// Package matcher finds the providers that apply to a URL (spec §6, step 3).
package matcher

import (
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/pinebench/decrumb/internal/rules"
)

// GlobalRules is the name of the provider whose rules apply to every URL.
// It always comes last in the result of Match.
const GlobalRules = "globalRules"

// Matcher finds the providers for a URL. Build it once with New and reuse
// it; it is read-only and safe to share between goroutines.
type Matcher struct {
	// candidates are in result order: site-specific providers sorted by
	// name, then globalRules.
	candidates []candidate
}

type candidate struct {
	provider *rules.Provider

	// literal is lowercase ASCII text that every URL matching the
	// provider's URLPattern contains, or "" if none was found. It lets
	// Match skip most regexes with a cheap strings.Contains.
	literal string
}

// New returns a Matcher for the providers in r. The Matcher points into r,
// so r must not be modified afterwards.
func New(r *rules.Rules) *Matcher {
	var site []candidate
	var global *candidate

	for i := range r.Providers {
		p := &r.Providers[i]
		c := candidate{provider: p, literal: requiredLiteral(p.URLPattern)}
		if p.Name == GlobalRules {
			global = &c
			continue
		}
		site = append(site, c)
	}

	slices.SortFunc(site, func(a, b candidate) int {
		return strings.Compare(a.provider.Name, b.provider.Name)
	})
	if global != nil {
		site = append(site, *global)
	}
	return &Matcher{candidates: site}
}

// Match returns the providers whose URLPattern matches rawURL: site-specific
// providers sorted by name, then globalRules if it matched. It returns nil if
// no provider matches. The result points into the rules and must not be
// modified.
//
// rawURL must be an absolute, already-normalized URL; the cleaner trims
// whitespace, adds a missing scheme and rejects invalid input. Each pattern
// is tested against the full URL string, exactly as compiled, like ClearURLs.
func (m *Matcher) Match(rawURL string) []*rules.Provider {
	// The literal shortcut is only safe for ASCII URLs: with (?i), Go also
	// matches "s" against "ſ" and "k" against the Kelvin sign, which
	// strings.ToLower doesn't map to ASCII.
	ascii := isASCII(rawURL)
	lower := ""
	if ascii {
		lower = strings.ToLower(rawURL)
	}

	var matched []*rules.Provider
	for _, c := range m.candidates {
		if ascii && c.literal != "" && !strings.Contains(lower, c.literal) {
			continue // can't match: a required piece of text is missing
		}
		if c.provider.URLPattern.MatchString(rawURL) {
			matched = append(matched, c.provider)
		}
	}
	return matched
}

// requiredLiteral returns the longest ASCII literal that every match of re
// must contain, lowercased, or "" if there is none.
//
// It only looks at the top level of the pattern: in ^https?://amazon\.,
// "http", "://" and "amazon." are all required, because a concatenation
// must match every part in order. Literals inside groups, alternations or
// repetitions are ignored, which is always safe: missing a literal only
// means the regex runs.
func requiredLiteral(re *regexp.Regexp) string {
	tree, err := syntax.Parse(re.String(), syntax.Perl)
	if err != nil {
		return "" // can't happen for a compiled regex; just don't skip
	}

	parts := []*syntax.Regexp{tree}
	if tree.Op == syntax.OpConcat {
		parts = tree.Sub
	}

	longest := ""
	for _, part := range parts {
		if part.Op != syntax.OpLiteral {
			continue
		}
		lit := string(part.Rune)
		if isASCII(lit) && len(lit) > len(longest) {
			longest = lit
		}
	}
	return strings.ToLower(longest)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
