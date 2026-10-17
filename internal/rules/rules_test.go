package rules

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/pinebench/decrumb/internal/rules/clearurls"
)

// findProvider returns the provider with the given name, or nil.
func findProvider(r *Rules, name string) *Provider {
	for i := range r.Providers {
		if r.Providers[i].Name == name {
			return &r.Providers[i]
		}
	}
	return nil
}

// TestBundledPatternsCompile compiles the bundled file on its own (without
// overrides), so an upstream update with a pattern Go can't compile fails
// here (spec §6, regex compatibility).
func TestBundledPatternsCompile(t *testing.T) {
	providers, err := parse(clearurls.Data)
	if err != nil {
		t.Fatal(err)
	}
	r, err := compile(providers)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, p := range r.Providers {
		count += 1 + len(p.Rules) + len(p.ReferralMarketing) + len(p.RawRules) +
			len(p.Exceptions) + len(p.Redirections)
	}
	t.Logf("compiled %d patterns in %d providers", count, len(r.Providers))
}

func TestLoadBundled(t *testing.T) {
	r, err := LoadBundled()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"globalRules", "google", "amazon"} {
		if findProvider(r, name) == nil {
			t.Errorf("provider %q not found", name)
		}
	}

	amazon := findProvider(r, "amazon")
	if amazon == nil {
		t.FailNow()
	}
	if !amazon.URLPattern.MatchString("https://www.amazon.co.uk/dp/123") {
		t.Error("amazon urlPattern does not match an amazon.co.uk URL")
	}
	if len(amazon.Rules) == 0 || len(amazon.RawRules) == 0 || len(amazon.Exceptions) == 0 {
		t.Error("amazon is missing rules, rawRules or exceptions")
	}
}

func TestBundledOverridesFileIsValid(t *testing.T) {
	if _, err := parse(overridesJSON); err != nil {
		t.Fatal(err)
	}
}

// TestOverridesHaveSources checks spec §8: every pattern in overrides.json
// has a source URL in its provider's "sources" map, and every source belongs
// to a pattern.
func TestOverridesHaveSources(t *testing.T) {
	var doc struct {
		Providers map[string]struct {
			Rules             []string          `json:"rules"`
			ReferralMarketing []string          `json:"referralMarketing"`
			RawRules          []string          `json:"rawRules"`
			Exceptions        []string          `json:"exceptions"`
			Redirections      []string          `json:"redirections"`
			Sources           map[string]string `json:"sources"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(overridesJSON, &doc); err != nil {
		t.Fatal(err)
	}

	for name, p := range doc.Providers {
		patterns := slices.Concat(p.Rules, p.ReferralMarketing, p.RawRules, p.Exceptions, p.Redirections)
		for _, pattern := range patterns {
			if !strings.HasPrefix(p.Sources[pattern], "https://") {
				t.Errorf("provider %q: pattern %q has no https source", name, pattern)
			}
		}
		for pattern := range p.Sources {
			if !slices.Contains(patterns, pattern) {
				t.Errorf("provider %q: source for unknown pattern %q", name, pattern)
			}
		}
	}
}

func TestPatternsAreCaseInsensitive(t *testing.T) {
	r := mustLoad(t, `{"providers": {"p": {
		"urlPattern": "^https://example\\.com",
		"rules": ["utm_source"],
		"rawRules": ["/ref=[a-z]+"]
	}}}`)
	p := r.Providers[0]

	if !p.URLPattern.MatchString("HTTPS://EXAMPLE.COM/") {
		t.Error("urlPattern should match regardless of case")
	}
	if !p.Rules[0].MatchString("UTM_Source") {
		t.Error("rules should match regardless of case")
	}
	if !p.RawRules[0].MatchString("/REF=ABC") {
		t.Error("rawRules should match regardless of case")
	}
}

func TestParameterRulesMatchWholeName(t *testing.T) {
	r := mustLoad(t, `{"providers": {"p": {
		"urlPattern": ".*",
		"rules": ["ref", "a|b"],
		"referralMarketing": ["tag"]
	}}}`)
	p := r.Providers[0]

	tests := []struct {
		name string
		re   int
		in   string
		want bool
	}{
		{"exact", 0, "ref", true},
		{"prefix only", 0, "refresh", false},
		{"suffix only", 0, "pref", false},
		{"alternation first", 1, "a", true},
		{"alternation second", 1, "b", true},
		{"alternation is anchored as a whole", 1, "ab", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.Rules[tt.re].MatchString(tt.in); got != tt.want {
				t.Errorf("rules[%d].MatchString(%q) = %v, want %v", tt.re, tt.in, got, tt.want)
			}
		})
	}

	if p.ReferralMarketing[0].MatchString("tags") {
		t.Error("referralMarketing should match whole names only")
	}
}

func TestProvidersAreSortedByName(t *testing.T) {
	r := mustLoad(t, `{"providers": {
		"zeta":  {"urlPattern": "z"},
		"alpha": {"urlPattern": "a"},
		"mid":   {"urlPattern": "m"}
	}}`)
	var names []string
	for _, p := range r.Providers {
		names = append(names, p.Name)
	}
	if got := strings.Join(names, ","); got != "alpha,mid,zeta" {
		t.Errorf("got order %s, want alpha,mid,zeta", got)
	}
}

func TestInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr []string // all must appear in the error
	}{
		{
			name:    "malformed JSON",
			input:   `{"providers": {`,
			wantErr: []string{"invalid JSON"},
		},
		{
			name:    "not an object",
			input:   `[]`,
			wantErr: []string{"invalid JSON"},
		},
		{
			name:    "missing providers",
			input:   `{}`,
			wantErr: []string{`missing "providers"`},
		},
		{
			name:    "null providers",
			input:   `{"providers": null}`,
			wantErr: []string{`missing "providers"`},
		},
		{
			name:    "providers is a list",
			input:   `{"providers": []}`,
			wantErr: []string{"invalid JSON"},
		},
		{
			name:    "missing urlPattern",
			input:   `{"providers": {"amazon": {"rules": ["a"]}}}`,
			wantErr: []string{`provider "amazon"`, "urlPattern: missing"},
		},
		{
			name:    "bad regex in urlPattern",
			input:   `{"providers": {"amazon": {"urlPattern": "(unclosed"}}}`,
			wantErr: []string{`provider "amazon"`, "urlPattern", "invalid regex"},
		},
		{
			name:    "bad regex in rules",
			input:   `{"providers": {"amazon": {"urlPattern": "a", "rules": ["a", "b", "c", "x(?=y)"]}}}`,
			wantErr: []string{`provider "amazon": rules[3]: invalid regex "x(?=y)"`},
		},
		{
			name:    "bad regex in referralMarketing",
			input:   `{"providers": {"p": {"urlPattern": "a", "referralMarketing": ["[a-"]}}}`,
			wantErr: []string{`provider "p": referralMarketing[0]: invalid regex`},
		},
		{
			name:    "bad regex in rawRules",
			input:   `{"providers": {"p": {"urlPattern": "a", "rawRules": ["*"]}}}`,
			wantErr: []string{`provider "p": rawRules[0]: invalid regex`},
		},
		{
			name:    "bad regex in exceptions",
			input:   `{"providers": {"p": {"urlPattern": "a", "exceptions": ["ok", "(?<=x)"]}}}`,
			wantErr: []string{`provider "p": exceptions[1]: invalid regex`},
		},
		{
			name:    "bad regex in redirections",
			input:   `{"providers": {"p": {"urlPattern": "a", "redirections": ["\\"]}}}`,
			wantErr: []string{`provider "p": redirections[0]: invalid regex`},
		},
		{
			name:    "empty pattern",
			input:   `{"providers": {"p": {"urlPattern": "a", "rules": [""]}}}`,
			wantErr: []string{`provider "p": rules[0]: empty pattern`},
		},
		{
			name:    "rules is a string",
			input:   `{"providers": {"p": {"urlPattern": "a", "rules": "utm_source"}}}`,
			wantErr: []string{`provider "p"`, "rules"},
		},
		{
			name:    "urlPattern is a number",
			input:   `{"providers": {"p": {"urlPattern": 42}}}`,
			wantErr: []string{`provider "p"`, "urlPattern"},
		},
		{
			name:    "completeProvider is a string",
			input:   `{"providers": {"p": {"urlPattern": "a", "completeProvider": "yes"}}}`,
			wantErr: []string{`provider "p"`, "completeProvider"},
		},
		{
			name:    "provider is not an object",
			input:   `{"providers": {"p": "a"}}`,
			wantErr: []string{`provider "p"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFile(writeTemp(t, tt.input))
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestLoadFileMissing(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "nope.json"))
	if err == nil || !strings.Contains(err.Error(), "nope.json") {
		t.Errorf("got %v, want an error naming the file", err)
	}
}

func TestLoadFileIgnoresOverridesAndUnusedFields(t *testing.T) {
	r := mustLoad(t, `{"providers": {"only": {
		"urlPattern": "x",
		"completeProvider": true,
		"forceRedirection": true,
		"somethingNew": [1, 2]
	}}}`)
	if len(r.Providers) != 1 || r.Providers[0].Name != "only" {
		t.Errorf("got %d providers, want just %q", len(r.Providers), "only")
	}
}

func TestOverridesMerge(t *testing.T) {
	const base = `{"providers": {
		"site": {
			"urlPattern": "^https://site\\.example",
			"rules": ["a"],
			"referralMarketing": ["ref1"],
			"rawRules": ["/raw1"],
			"exceptions": ["/exc1"],
			"redirections": ["/go\\?to=([^&]+)"]
		},
		"globalRules": {"urlPattern": ".*", "rules": ["utm_source"]}
	}}`

	t.Run("empty overrides change nothing", func(t *testing.T) {
		r, err := loadWithOverrides([]byte(base), []byte(`{"providers": {}}`))
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Providers) != 2 {
			t.Fatalf("got %d providers, want 2", len(r.Providers))
		}
		if got := patternStrings(findProvider(r, "site").Rules); got != "(?i)^(?:a)$" {
			t.Errorf("site rules = %s", got)
		}
	})

	t.Run("new provider is added", func(t *testing.T) {
		r, err := loadWithOverrides([]byte(base), []byte(`{"providers": {
			"newsite": {"urlPattern": "^https://new\\.example", "rules": ["trk"]}
		}}`))
		if err != nil {
			t.Fatal(err)
		}
		p := findProvider(r, "newsite")
		if p == nil {
			t.Fatal("newsite not added")
		}
		if !p.Rules[0].MatchString("trk") {
			t.Error("newsite rule not compiled")
		}
		if len(r.Providers) != 3 {
			t.Errorf("got %d providers, want 3", len(r.Providers))
		}
	})

	t.Run("lists of an existing provider are appended", func(t *testing.T) {
		r, err := loadWithOverrides([]byte(base), []byte(`{"providers": {
			"site": {
				"rules": ["b"],
				"referralMarketing": ["ref2"],
				"rawRules": ["/raw2"],
				"exceptions": ["/exc2"],
				"redirections": ["/out\\?u=([^&]+)"]
			}
		}}`))
		if err != nil {
			t.Fatal(err)
		}
		p := findProvider(r, "site")

		checks := []struct {
			field string
			got   string
			want  string
		}{
			{"urlPattern", p.URLPattern.String(), `(?i)^https://site\.example`},
			{"rules", patternStrings(p.Rules), "(?i)^(?:a)$ (?i)^(?:b)$"},
			{"referralMarketing", patternStrings(p.ReferralMarketing), "(?i)^(?:ref1)$ (?i)^(?:ref2)$"},
			{"rawRules", patternStrings(p.RawRules), "(?i)/raw1 (?i)/raw2"},
			{"exceptions", patternStrings(p.Exceptions), "(?i)/exc1 (?i)/exc2"},
			{"redirections", patternStrings(p.Redirections), `(?i)/go\?to=([^&]+) (?i)/out\?u=([^&]+)`},
		}
		for _, c := range checks {
			if c.got != c.want {
				t.Errorf("%s = %s, want %s", c.field, c.got, c.want)
			}
		}

		// Other providers are untouched.
		if got := patternStrings(findProvider(r, "globalRules").Rules); got != "(?i)^(?:utm_source)$" {
			t.Errorf("globalRules rules = %s", got)
		}
	})

	t.Run("urlPattern on an existing provider is an error", func(t *testing.T) {
		_, err := loadWithOverrides([]byte(base), []byte(`{"providers": {
			"site": {"urlPattern": ".*", "rules": ["b"]}
		}}`))
		if err == nil || !strings.Contains(err.Error(), `overrides: provider "site": urlPattern`) {
			t.Errorf("got %v, want a urlPattern error for provider site", err)
		}
	})

	t.Run("new provider without urlPattern is an error", func(t *testing.T) {
		_, err := loadWithOverrides([]byte(base), []byte(`{"providers": {
			"newsite": {"rules": ["b"]}
		}}`))
		if err == nil || !strings.Contains(err.Error(), `provider "newsite": urlPattern: missing`) {
			t.Errorf("got %v, want a missing urlPattern error for newsite", err)
		}
	})

	t.Run("bad regex in overrides is an error", func(t *testing.T) {
		_, err := loadWithOverrides([]byte(base), []byte(`{"providers": {
			"site": {"rules": ["("]}
		}}`))
		if err == nil || !strings.Contains(err.Error(), `provider "site": rules[1]: invalid regex`) {
			t.Errorf("got %v, want an invalid regex error", err)
		}
	})

	t.Run("malformed overrides", func(t *testing.T) {
		_, err := loadWithOverrides([]byte(base), []byte(`{`))
		if err == nil || !strings.Contains(err.Error(), "overrides: invalid JSON") {
			t.Errorf("got %v, want an overrides JSON error", err)
		}
	})
}

func mustLoad(t *testing.T, doc string) *Rules {
	t.Helper()
	r, err := LoadFile(writeTemp(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// writeTemp writes content to a file in a per-test temporary directory,
// which the testing package deletes afterwards.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rules.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func patternStrings(res []*regexp.Regexp) string {
	s := make([]string, len(res))
	for i, re := range res {
		s[i] = re.String()
	}
	return strings.Join(s, " ")
}
