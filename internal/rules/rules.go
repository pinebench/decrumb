// Package rules loads, validates and compiles rules in the ClearURLs schema.
//
// Every pattern is compiled once, case-insensitively, when the rules are
// loaded. The result is read-only and safe to share between goroutines.
package rules

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"

	"github.com/pinebench/decrumb/internal/rules/clearurls"
)

// overridesJSON holds our own additions to the bundled rules (spec §8).
//
//go:embed overrides.json
var overridesJSON []byte

// Rules is a compiled rules set.
type Rules struct {
	// Providers are sorted by name.
	Providers []Provider
}

// Provider is one compiled provider, such as "amazon" or "globalRules".
//
// The upstream fields completeProvider and forceRedirection are read
// (so wrong types are reported) but not kept: decrumb ignores them.
type Provider struct {
	Name string

	// URLPattern decides whether this provider applies to a URL.
	URLPattern *regexp.Regexp

	// Rules and ReferralMarketing match query parameter names. They are
	// anchored, so they only match a whole name: "ref" matches "ref" and
	// "REF", not "refresh".
	Rules             []*regexp.Regexp
	ReferralMarketing []*regexp.Regexp

	// RawRules match anywhere in the URL string.
	RawRules []*regexp.Regexp

	// Exceptions match the whole URL; a match means "don't clean".
	Exceptions []*regexp.Regexp

	// Redirections match the whole URL; the first capture group is the
	// redirect target.
	Redirections []*regexp.Regexp
}

// providerJSON is one provider as written in a rules file.
type providerJSON struct {
	URLPattern        string   `json:"urlPattern"`
	CompleteProvider  bool     `json:"completeProvider"` // ignored (spec §6)
	Rules             []string `json:"rules"`
	ReferralMarketing []string `json:"referralMarketing"`
	RawRules          []string `json:"rawRules"`
	Exceptions        []string `json:"exceptions"`
	Redirections      []string `json:"redirections"`
	ForceRedirection  bool     `json:"forceRedirection"` // ignored (spec §6)
}

// LoadBundled returns the bundled ClearURLs rules with our overrides merged in.
func LoadBundled() (*Rules, error) {
	return loadWithOverrides(clearurls.Data, overridesJSON)
}

// LoadFile returns the rules in the file at path, used exactly as given:
// overrides are not applied (spec §8, --rules).
func LoadFile(path string) (*Rules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	providers, err := parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	r, err := compile(providers)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// loadWithOverrides parses base and overrides, merges them and compiles
// the result.
func loadWithOverrides(base, overrides []byte) (*Rules, error) {
	providers, err := parse(base)
	if err != nil {
		return nil, fmt.Errorf("bundled rules: %w", err)
	}
	extra, err := parse(overrides)
	if err != nil {
		return nil, fmt.Errorf("overrides: %w", err)
	}
	if err := merge(providers, extra); err != nil {
		return nil, fmt.Errorf("overrides: %w", err)
	}
	return compile(providers)
}

// parse decodes a rules document without compiling it.
func parse(data []byte) (map[string]providerJSON, error) {
	// Decode each provider separately so errors can name it.
	var doc struct {
		Providers map[string]json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if doc.Providers == nil {
		return nil, errors.New(`missing "providers" object`)
	}

	providers := make(map[string]providerJSON, len(doc.Providers))
	for name, raw := range doc.Providers {
		var p providerJSON
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		providers[name] = p
	}
	return providers, nil
}

// merge adds extra to base, extend-only (spec §8): new providers are added,
// and for existing providers the lists are appended. Nothing in base is
// removed or replaced.
func merge(base, extra map[string]providerJSON) error {
	for _, name := range slices.Sorted(maps.Keys(extra)) {
		o := extra[name]
		b, exists := base[name]
		if !exists {
			base[name] = o
			continue
		}
		if o.URLPattern != "" {
			return fmt.Errorf("provider %q: urlPattern: cannot change the urlPattern of an existing provider", name)
		}
		b.Rules = append(b.Rules, o.Rules...)
		b.ReferralMarketing = append(b.ReferralMarketing, o.ReferralMarketing...)
		b.RawRules = append(b.RawRules, o.RawRules...)
		b.Exceptions = append(b.Exceptions, o.Exceptions...)
		b.Redirections = append(b.Redirections, o.Redirections...)
		base[name] = b
	}
	return nil
}

// compile validates and compiles every provider.
func compile(providers map[string]providerJSON) (*Rules, error) {
	r := &Rules{Providers: make([]Provider, 0, len(providers))}
	for _, name := range slices.Sorted(maps.Keys(providers)) {
		p, err := compileProvider(name, providers[name])
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", name, err)
		}
		r.Providers = append(r.Providers, p)
	}
	return r, nil
}

func compileProvider(name string, pj providerJSON) (Provider, error) {
	p := Provider{Name: name}
	var err error

	if pj.URLPattern == "" {
		return p, errors.New("urlPattern: missing")
	}
	if p.URLPattern, err = compilePattern(pj.URLPattern, false); err != nil {
		return p, fmt.Errorf("urlPattern: %w", err)
	}
	if p.Rules, err = compileList("rules", pj.Rules, true); err != nil {
		return p, err
	}
	if p.ReferralMarketing, err = compileList("referralMarketing", pj.ReferralMarketing, true); err != nil {
		return p, err
	}
	if p.RawRules, err = compileList("rawRules", pj.RawRules, false); err != nil {
		return p, err
	}
	if p.Exceptions, err = compileList("exceptions", pj.Exceptions, false); err != nil {
		return p, err
	}
	if p.Redirections, err = compileList("redirections", pj.Redirections, false); err != nil {
		return p, err
	}
	return p, nil
}

func compileList(field string, patterns []string, wholeName bool) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for i, pattern := range patterns {
		re, err := compilePattern(pattern, wholeName)
		if err != nil {
			return nil, fmt.Errorf("%s[%d]: %w", field, i, err)
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

// compilePattern compiles a pattern case-insensitively, like ClearURLs.
// With wholeName set, the pattern must match an entire parameter name
// (spec §6, step 6).
func compilePattern(pattern string, wholeName bool) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, errors.New("empty pattern")
	}
	expr := "(?i)" + pattern
	if wholeName {
		expr = "(?i)^(?:" + pattern + ")$"
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid regex %q: %w", pattern, err)
	}
	return re, nil
}
