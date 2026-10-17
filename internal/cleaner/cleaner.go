// Package cleaner removes tracking parameters from URLs and unwraps redirect
// links, offline (spec §6). Build a Cleaner once with New, then call Clean
// for each URL.
package cleaner

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pinebench/decrumb/internal/decode"
	"github.com/pinebench/decrumb/internal/matcher"
	"github.com/pinebench/decrumb/internal/rules"
	"github.com/pinebench/decrumb/internal/scheme"
)

// MaxHops is the most redirects Clean unwraps for one URL (spec §6, step 5).
const MaxHops = 5

// ErrInvalidURL is wrapped by every error in Result.Err, so callers can
// check for it with errors.Is.
var ErrInvalidURL = errors.New("invalid URL")

// Cleaner cleans URLs with one rules set. It is read-only after New and
// safe to share between goroutines.
type Cleaner struct {
	matcher *matcher.Matcher
}

// New returns a Cleaner for r. The Cleaner points into r, so r must not be
// modified afterwards.
func New(r *rules.Rules) *Cleaner {
	return &Cleaner{matcher: matcher.New(r)}
}

// Result is the outcome of cleaning one URL.
type Result struct {
	// Input is the URL exactly as given to Clean.
	Input string

	// Output is the cleaned URL. It is always normalized: surrounding
	// whitespace is trimmed and https:// is added when the input had no
	// scheme. A URL whose scheme isn't http or https is returned as it is
	// (after trimming). Output is "" when Err is set.
	Output string

	// Changed is true if something was removed or unwrapped. Normalizing
	// alone (trimming, adding https://) does not count (spec §5).
	Changed bool

	// Removed lists what was removed, in the order it was removed.
	Removed []Removal

	// Unwrapped lists the URL reached by each redirect hop, in order.
	Unwrapped []string

	// HopLimitReached is true if a redirect was still left to unwrap after
	// MaxHops hops. Output is then the last URL reached, cleaned.
	HopLimitReached bool

	// Err is set, wrapping ErrInvalidURL, if the input is not a valid URL.
	Err error
}

// Removal is one thing removed from a URL.
type Removal struct {
	// Name is the removed parameter's name as tested against the rules,
	// that is percent-decoded (utm%5Fsource becomes utm_source). For a raw
	// rule, Name is the text the rule removed.
	Name string

	// Provider is the provider credited with the removal: the first one,
	// in the §6 step 3 order, whose rule matched.
	Provider string

	// From says where the removal happened.
	From Location
}

// Location is where in the URL a removal happened.
type Location string

const (
	Query    Location = "query"    // a query parameter (step 7)
	Fragment Location = "fragment" // a parameter in the fragment (step 7)
	RawRule  Location = "rawRule"  // text removed by a raw rule (step 6)
)

// Clean cleans one URL, following spec §6 steps 1 to 8.
func (c *Cleaner) Clean(input string) Result {
	res := Result{Input: input}

	// Steps 1 and 2: normalize and parse; leave non-http(s) URLs alone.
	s, isHTTP, err := normalize(input)
	if err != nil {
		res.Err = err
		return res
	}
	if !isHTTP {
		res.Output = s
		return res
	}

	// Steps 3 to 5, repeated for every redirect hop.
	var providers []*rules.Provider
	for {
		providers = c.activeProviders(s)
		target, ok := redirectTarget(s, providers)
		if !ok {
			break
		}
		if len(res.Unwrapped) == MaxHops {
			res.HopLimitReached = true
			break
		}
		res.Unwrapped = append(res.Unwrapped, target)
		s = target
	}

	// Step 6: raw rules.
	s, removed := applyRawRules(s, providers)
	res.Removed = append(res.Removed, removed...)

	// Steps 7 and 8: parameters in the query and the fragment.
	s, removed = removeParams(s, providers)
	res.Removed = append(res.Removed, removed...)

	res.Output = s
	res.Changed = len(res.Removed) > 0 || len(res.Unwrapped) > 0
	return res
}

// normalize is step 1: trim whitespace, add https:// if there's no scheme,
// percent-encode whitespace left inside the URL, and parse. isHTTP reports
// whether the scheme is http or https; only then is a host required, and
// only then is whitespace encoded (step 2 returns any other scheme as it
// is, after trimming).
func normalize(input string) (s string, isHTTP bool, err error) {
	s = strings.TrimSpace(input)
	if !scheme.Has(s) {
		s = "https://" + s
	}
	if scheme.HTTPLen(s) > 0 {
		s = encodeSpaces(s)
	}

	u, err := url.Parse(s)
	if err != nil {
		// url.Parse's error repeats the whole URL; keep only the reason.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return "", false, fmt.Errorf("%w %q: %v", ErrInvalidURL, input, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return s, false, nil
	}
	if u.Host == "" {
		return "", false, fmt.Errorf("%w %q: no host", ErrInvalidURL, input)
	}
	// url.Parse rejects %20 in a host, but accepts encoded Unicode
	// whitespace such as %C2%A0 (U+00A0). Reject both.
	if strings.ContainsFunc(u.Host, unicode.IsSpace) {
		return "", false, fmt.Errorf("%w %q: whitespace in host", ErrInvalidURL, input)
	}
	return s, true, nil
}

// encodeSpaces percent-encodes every whitespace character in s, as
// browsers do, using its UTF-8 bytes: a space becomes %20 and U+00A0
// becomes %C2%A0. Nothing else is changed.
func encodeSpaces(s string) string {
	if !strings.ContainsFunc(s, unicode.IsSpace) {
		return s
	}
	var b strings.Builder
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if unicode.IsSpace(r) {
			for i := range size {
				fmt.Fprintf(&b, "%%%02X", s[i])
			}
		} else {
			b.WriteString(s[:size]) // the original bytes, even if not valid UTF-8
		}
		s = s[size:]
	}
	return b.String()
}

// activeProviders is steps 3 and 4: the providers whose urlPattern matches
// s, in the §6 step 3 order (matcher.Match's order), without those that an
// exception skips for s. Exceptions are per provider: skipping one provider
// leaves the others in place.
func (c *Cleaner) activeProviders(s string) []*rules.Provider {
	var active []*rules.Provider
	for _, p := range c.matcher.Match(s) {
		if !anyMatch(p.Exceptions, s) {
			active = append(active, p)
		}
	}
	return active
}

// redirectTarget is step 5: the URL that s redirects to, if any. Providers
// and their redirections are tried in order; the first one whose captured
// target decodes into a valid http(s) URL wins. If none does, s is not
// unwrapped.
func redirectTarget(s string, providers []*rules.Provider) (string, bool) {
	for _, p := range providers {
		for _, re := range p.Redirections {
			m := re.FindStringSubmatch(s)
			if len(m) < 2 {
				continue // no match, or no capture group
			}
			target, ok := decode.Target(m[1])
			if !ok {
				continue
			}
			// "Restart from step 1": the target must pass step 1 too.
			target, isHTTP, err := normalize(target)
			if err != nil || !isHTTP {
				continue
			}
			return target, true
		}
	}
	return "", false
}

// applyRawRules is step 6: each raw rule removes every match from the URL
// string, provider by provider, in order. A rule's change is discarded if
// the result is no longer a valid http(s) URL with the same host.
func applyRawRules(s string, providers []*rules.Provider) (string, []Removal) {
	host, _ := httpHost(s)
	var removed []Removal
	for _, p := range providers {
		for _, re := range p.RawRules {
			out := re.ReplaceAllLiteralString(s, "")
			if out == s {
				continue
			}
			if h, ok := httpHost(out); !ok || h != host {
				continue // safety check failed: discard this rule's change
			}
			for _, text := range re.FindAllString(s, -1) {
				if text != "" {
					removed = append(removed, Removal{Name: text, Provider: p.Name, From: RawRule})
				}
			}
			s = out
		}
	}
	return s, removed
}

// httpHost parses s as it is (no trimming, no added scheme) and returns its
// host, with the port if any. ok is false unless s is a valid http(s) URL
// with a host.
func httpHost(s string) (host string, ok bool) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", false
	}
	return u.Host, true
}

// removeParams is steps 7 and 8: remove the query parameters, and the
// fragment parameters, whose name matches a provider's rules or
// referralMarketing. Everything else keeps its order and encoding.
func removeParams(s string, providers []*rules.Provider) (string, []Removal) {
	rest, fragment, hasFragment := strings.Cut(s, "#")
	base, query, hasQuery := strings.Cut(rest, "?")
	var removed []Removal

	if hasQuery {
		kept, r := filterParams(query, providers, Query)
		if len(r) > 0 {
			removed = append(removed, r...)
			query = kept
			hasQuery = kept != "" // all removed: drop the "?"
		}
	}

	// The fragment is only treated as parameters if it contains "=", so
	// #section-2 or #/inbox/42 are never touched.
	if hasFragment && strings.Contains(fragment, "=") {
		kept, r := filterParams(fragment, providers, Fragment)
		if len(r) > 0 {
			removed = append(removed, r...)
			fragment = kept
			hasFragment = kept != "" // all removed: drop the "#"
		}
	}

	if len(removed) == 0 {
		return s, nil // nothing removed: don't rebuild anything
	}
	out := base
	if hasQuery {
		out += "?" + query
	}
	if hasFragment {
		out += "#" + fragment
	}
	return out, removed
}

// filterParams splits list on "&", drops every piece whose name matches a
// rule and rejoins the rest unchanged. A piece's name is the text before its
// first "=", so a value may contain "=" (#:~:text=a=b keeps its value whole).
// If nothing is removed, list is returned as it was.
func filterParams(list string, providers []*rules.Provider, from Location) (string, []Removal) {
	var kept []string
	var removed []Removal
	for _, piece := range strings.Split(list, "&") {
		name, _, _ := strings.Cut(piece, "=")
		// Test the percent-decoded name; if it can't be decoded, the raw one.
		if decoded, err := url.PathUnescape(name); err == nil {
			name = decoded
		}
		if provider, ok := matchingProvider(name, providers); ok {
			removed = append(removed, Removal{Name: name, Provider: provider, From: from})
			continue
		}
		kept = append(kept, piece)
	}
	if len(removed) == 0 {
		return list, nil
	}
	return strings.Join(kept, "&"), removed
}

// matchingProvider returns the name of the first provider with a rules or
// referralMarketing pattern that matches the whole parameter name.
func matchingProvider(name string, providers []*rules.Provider) (string, bool) {
	for _, p := range providers {
		if anyMatch(p.Rules, name) || anyMatch(p.ReferralMarketing, name) {
			return p.Name, true
		}
	}
	return "", false
}

// anyMatch reports whether any of the patterns matches s.
func anyMatch(patterns []*regexp.Regexp, s string) bool {
	for _, re := range patterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
