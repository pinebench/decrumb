// Package explain describes common tracking parameters (spec §7, explain).
// The descriptions live in params.json, which is embedded into the binary
// and is also read by the browser extension.
package explain

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// maxDescriptionLen is the longest description allowed, in characters
// (spec §7: "about 100 characters at most").
const maxDescriptionLen = 100

//go:embed params.json
var paramsJSON []byte

// bundled is the parsed params.json. A broken file is a build mistake that
// the tests catch, so loading it at startup can't fail in a release.
var bundled = mustParse(paramsJSON)

// Entry describes one parameter, or a family of parameters.
type Entry struct {
	// Name is an exact parameter name (fbclid), or a family ending in a
	// single trailing "*" (pd_rd_*).
	Name string

	// Provider is the rules provider the description is limited to, for
	// names that mean different things on different sites (t on x and
	// twitter). It is "" for names that mean the same thing everywhere.
	Provider string

	// Description is one plain sentence: "What it is: what it does".
	Description string

	// Source is the vendor documentation, or the best public evidence,
	// behind the description.
	Source string

	// Example is a realistic URL containing the parameter.
	Example string
}

// Describe returns the description for a parameter removed by provider.
// name is the decoded name the cleaner reports, and provider is the
// provider it credits. Names match case-insensitively; providers exactly.
//
// The first match wins, in this order (spec §7): exact name and provider,
// exact name without provider, family and provider, family without
// provider.
func Describe(name, provider string) (Entry, bool) {
	return bundled.describe(name, provider)
}

// table is a parsed params file.
type table []Entry

func (t table) describe(name, provider string) (Entry, bool) {
	steps := []struct {
		family   bool
		provider string
	}{
		{false, provider},
		{false, ""},
		{true, provider},
		{true, ""},
	}
	for _, step := range steps {
		for _, e := range t {
			if isFamily(e.Name) == step.family && e.Provider == step.provider && matches(e.Name, name) {
				return e, true
			}
		}
	}
	return Entry{}, false
}

// isFamily reports whether pattern is a family like pd_rd_*.
func isFamily(pattern string) bool {
	return strings.HasSuffix(pattern, "*")
}

// matches reports whether name matches pattern, ignoring case. A family
// matches every name that starts with the part before the "*".
func matches(pattern, name string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "*"); ok {
		return len(name) >= len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
	}
	return strings.EqualFold(pattern, name)
}

// entryJSON is one entry as written in params.json. Provider is a pointer
// so that null (no provider) can be told apart from "" (a mistake).
type entryJSON struct {
	Name        string  `json:"name"`
	Provider    *string `json:"provider"`
	Description string  `json:"description"`
	Source      string  `json:"source"`
	Example     string  `json:"example"`
}

// parse reads and validates a params file.
func parse(data []byte) (table, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // our own file: an unknown field is a typo
	var raw []entryJSON
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("parsing params: %w", err)
	}

	t := make(table, 0, len(raw))
	seen := make(map[string]bool)
	for i, r := range raw {
		e := Entry{
			Name:        r.Name,
			Description: r.Description,
			Source:      r.Source,
			Example:     r.Example,
		}
		if r.Provider != nil {
			e.Provider = *r.Provider
		}
		if err := validate(e, r.Provider != nil); err != nil {
			return nil, fmt.Errorf("params entry %d (%q): %w", i, r.Name, err)
		}
		key := strings.ToLower(e.Name) + "\x00" + e.Provider
		if seen[key] {
			return nil, fmt.Errorf("params entry %d (%q): duplicate name and provider %q", i, r.Name, e.Provider)
		}
		seen[key] = true
		t = append(t, e)
	}
	return t, nil
}

// validate checks one entry. hasProvider is true if the file gave a
// provider string rather than null.
func validate(e Entry, hasProvider bool) error {
	switch {
	case e.Name == "":
		return errors.New("name is missing")
	case strings.Contains(strings.TrimSuffix(e.Name, "*"), "*"):
		return errors.New(`"*" is only allowed once, as the last character`)
	case e.Name == "*":
		return errors.New(`name can't be only "*"`)
	case hasProvider && e.Provider == "":
		return errors.New(`provider is "": use null for no provider`)
	case e.Description == "":
		return errors.New("description is missing")
	case utf8.RuneCountInString(e.Description) > maxDescriptionLen:
		return fmt.Errorf("description is longer than %d characters", maxDescriptionLen)
	case e.Source == "":
		return errors.New("source is missing")
	case !strings.HasPrefix(e.Source, "https://"):
		return errors.New("source must start with https://")
	case e.Example == "":
		return errors.New("example is missing")
	}
	return nil
}

func mustParse(data []byte) table {
	t, err := parse(data)
	if err != nil {
		panic(err)
	}
	return t
}
