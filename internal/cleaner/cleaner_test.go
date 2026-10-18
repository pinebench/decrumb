package cleaner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pinebench/decrumb/internal/rules"
)

// bundled returns a Cleaner for the real bundled rules.
func bundled(tb testing.TB) *Cleaner {
	tb.Helper()
	r, err := rules.LoadBundled()
	if err != nil {
		tb.Fatal(err)
	}
	return New(r)
}

// fixture returns a Cleaner for a small rules set written for one test.
// It goes through rules.LoadFile, so patterns are compiled exactly as for
// real rules.
func fixture(tb testing.TB, rulesJSON string) *Cleaner {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "rules.json")
	if err := os.WriteFile(path, []byte(rulesJSON), 0o600); err != nil {
		tb.Fatal(err)
	}
	r, err := rules.LoadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	return New(r)
}

// TestSharedCases runs testdata/cases.json (spec §10).
func TestSharedCases(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Input  string `json:"input"`
		Output string `json:"output"`
		Note   string `json:"note"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no cases")
	}

	c := bundled(t)
	for _, tc := range cases {
		t.Run(tc.Note, func(t *testing.T) {
			res := c.Clean(tc.Input)
			if res.Err != nil {
				t.Fatalf("Clean(%q): %v", tc.Input, res.Err)
			}
			if res.Output != tc.Output {
				t.Errorf("Clean(%q)\n got  %q\n want %q", tc.Input, res.Output, tc.Output)
			}
		})
	}
}

// Steps 1 and 2.
func TestNormalize(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		isHTTP  bool
		invalid bool
	}{
		{"https://example.com/", "https://example.com/", true, false},
		{"  http://example.com/a \t\n", "http://example.com/a", true, false},
		{"HTTPS://EXAMPLE.COM", "HTTPS://EXAMPLE.COM", true, false},
		{"example.com/page", "https://example.com/page", true, false},
		{"example.com:8080/path", "https://example.com:8080/path", true, false},
		{"localhost:3000", "https://localhost:3000", true, false},
		{"//example.com/x", "https://example.com/x", true, false},
		{" //example.com ", "https://example.com", true, false},
		{"//example.com/a b", "https://example.com/a%20b", true, false},
		{"mailto:x", "mailto:x", false, false},
		{"mailto:a@b.com", "mailto:a@b.com", false, false},
		{"ftp://example.com/f", "ftp://example.com/f", false, false},
		{"javascript:alert(1)", "javascript:alert(1)", false, false},

		// Whitespace inside an http(s) URL is percent-encoded, as browsers do.
		{"https://e.com/my file.pdf", "https://e.com/my%20file.pdf", true, false},
		{" example.com/a b ", "https://example.com/a%20b", true, false},
		{"https://e.com/a\tb\nc", "https://e.com/a%09b%0Ac", true, false},
		{"https://e.com/a b", "https://e.com/a%C2%A0b", true, false},
		{"https://e.com/a　b?q=x y", "https://e.com/a%E3%80%80b?q=x%E2%80%A8y", true, false},
		{"https://e.com/café\xff b", "https://e.com/café\xff%20b", true, false},
		{"https://user name@e.com/", "https://user%20name@e.com/", true, false},
		{"mailto:a b@example.com", "mailto:a b@example.com", false, false}, // other schemes: as given

		// Invalid: can't be parsed, or http(s) without a host.
		{"", "", false, true},
		{"   ", "", false, true},
		{"https://", "", false, true},
		{"http:example.com", "", false, true},
		{"https://exa mple.com/", "", false, true},
		{"exa mple.com", "", false, true},
		{"https://exa\tmple.com/", "", false, true},
		{"https://exa mple.com/", "", false, true},
		{"https://example.com　/", "", false, true},
		{"https://example.com:port/", "", false, true},
		{"///example.com", "", false, true}, // protocol-relative, but no host
	}
	for _, tt := range tests {
		got, isHTTP, err := normalize(tt.input)
		if tt.invalid {
			if !errors.Is(err, ErrInvalidURL) {
				t.Errorf("normalize(%q) error = %v, want ErrInvalidURL", tt.input, err)
			}
			continue
		}
		if err != nil || got != tt.want || isHTTP != tt.isHTTP {
			t.Errorf("normalize(%q) = %q, %v, %v; want %q, %v, nil", tt.input, got, isHTTP, err, tt.want, tt.isHTTP)
		}
	}
}

func TestCleanInvalid(t *testing.T) {
	c := bundled(t)
	res := c.Clean("https://exa mple.com/")
	if !errors.Is(res.Err, ErrInvalidURL) {
		t.Fatalf("Err = %v, want ErrInvalidURL", res.Err)
	}
	if res.Output != "" || res.Changed || res.Input != "https://exa mple.com/" {
		t.Errorf("unexpected result for invalid input: %+v", res)
	}
	if strings.Count(res.Err.Error(), "exa mple") != 1 {
		t.Errorf("error should name the input once: %v", res.Err)
	}
}

// TestInvalidMessages checks the exact error for each kind of invalid
// input. Messages describe what the user typed, never our normalization:
// no "%20" the user didn't type, no Go package names (spec §5).
func TestInvalidMessages(t *testing.T) {
	c := bundled(t)
	tests := []struct {
		input  string
		reason string
	}{
		// Whitespace in the host, typed or percent-encoded.
		{"not a url ::", "spaces are not allowed in the host"},
		{"exa mple.com", "spaces are not allowed in the host"},
		{"https://exa mple.com/a b", "spaces are not allowed in the host"},
		{"https://exa%20mple.com/", "spaces are not allowed in the host"},
		{"https://a.example:80 80/", "spaces are not allowed in the host"},
		{"https://exa\tmple.com/", "whitespace is not allowed in the host"},
		{"https://exa mple.com/", "whitespace is not allowed in the host"},
		{"https://exa%C2%A0mple.com/", "whitespace is not allowed in the host"},
		// No host.
		{"http://", "no host"},
		{"https:///path", "no host"},
		{"//", "no host"},
		{":", "no host"},
		{"https://:80/", "no host"},
		{"", "empty"},
		{"   ", "empty"},
		// Input url.Parse can't parse.
		{"https://[::1", `missing ']' in host`},
		{"https://a.example:port/", `invalid port ":port" after host`},
		{"https://ex%41mple.com/", `"%41" is not allowed in the host`},
		{"http://a.example/%zz", `invalid percent-encoding "%zz"`},
		{"https://example.com/%", `invalid percent-encoding "%"`},
		{"https://a\x00b.example/", "control characters are not allowed"},
		{"https://a.example/\x7f", "control characters are not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res := c.Clean(tt.input)
			want := fmt.Sprintf("invalid URL %q: %s", tt.input, tt.reason)
			if res.Err == nil || res.Err.Error() != want {
				t.Errorf("Clean(%q) error\n got  %v\n want %s", tt.input, res.Err, want)
			}
			if !errors.Is(res.Err, ErrInvalidURL) {
				t.Error("error does not wrap ErrInvalidURL")
			}
		})
	}

	// Whitespace outside the host is fine: it's encoded, as browsers do.
	for _, in := range []string{"https://example.com/a b", "https://a b@example.com/"} {
		if res := c.Clean(in); res.Err != nil {
			t.Errorf("Clean(%q): %v", in, res.Err)
		}
	}
}

// TestChanged checks spec §5: changed is true only if something was removed
// or unwrapped; normalizing alone doesn't count.
func TestChanged(t *testing.T) {
	c := bundled(t)
	tests := []struct {
		input   string
		changed bool
	}{
		{"https://example.com/?id=7", false},
		{"example.com/?id=7", false},
		{"  https://example.com/  ", false},
		{"https://example.com/my file.pdf", false},
		{"mailto:a@b.com", false},
		{"https://example.com/?utm_source=x", true},
		{"https://www.google.com/amp/s/example.com/a", true},
		{"https://www.amazon.com/dp/B000000000/ref=x", true},
	}
	for _, tt := range tests {
		if got := c.Clean(tt.input).Changed; got != tt.changed {
			t.Errorf("Clean(%q).Changed = %v, want %v", tt.input, got, tt.changed)
		}
	}
}

// TestRemovedCredit checks which provider is credited for a removal (§6
// step 3): the first matching provider in order, site-specific before
// globalRules.
func TestRemovedCredit(t *testing.T) {
	c := bundled(t)
	tests := []struct {
		name  string
		input string
		want  []Removal
	}{
		{
			"global only",
			"https://example.com/?utm_source=x&id=1&fbclid=y",
			[]Removal{
				{"utm_source", "globalRules", Query},
				{"fbclid", "globalRules", Query},
			},
		},
		{
			// ref is in both amazon's rules and globalRules' referralMarketing.
			"site provider first",
			"https://www.amazon.com/dp/B000000000?ref=x&utm_source=y",
			[]Removal{
				{"ref", "amazon", Query},
				{"utm_source", "globalRules", Query},
			},
		},
		{
			// amazon is skipped by its exception, amazon search applies.
			"skipped provider is not credited",
			"https://www.amazon.com/s?k=go&ref=x",
			[]Removal{{"ref", "amazon search", Query}},
		},
		{
			"decoded name",
			"https://example.com/?utm%5Fsource=x",
			[]Removal{{"utm_source", "globalRules", Query}},
		},
		{
			"raw rule, then parameters",
			"https://www.amazon.com/dp/B000000000/ref=sr_1_1?qid=1",
			[]Removal{
				{"/ref=sr_1_1", "amazon", RawRule},
				{"qid", "amazon", Query},
			},
		},
		{
			"query, then fragment",
			"https://example.com/?gclid=1#utm_medium=x&a=b",
			[]Removal{
				{"gclid", "globalRules", Query},
				{"utm_medium", "globalRules", Fragment},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Clean(tt.input).Removed
			if !slices.Equal(got, tt.want) {
				t.Errorf("Removed = %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

// Step 4: exceptions are per provider.
func TestExceptionsPerProvider(t *testing.T) {
	c := fixture(t, `{"providers": {
		"site": {
			"urlPattern": "^https?://example\\.com",
			"rules": ["site_id"],
			"rawRules": ["/track/[0-9]+"],
			"exceptions": ["^https?://example\\.com/keep"],
			"redirections": ["^https?://example\\.com/(?:keep/)?go\\?to=([^&]+)"]
		},
		"globalRules": {
			"urlPattern": ".*",
			"rules": ["utm_source"],
			"exceptions": ["^https?://example\\.com/raw"]
		}
	}}`)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no exception: both apply",
			"https://example.com/a?site_id=1&utm_source=2&x=3",
			"https://example.com/a?x=3"},
		{"site skipped: globalRules still applies",
			"https://example.com/keep?site_id=1&utm_source=2&x=3",
			"https://example.com/keep?site_id=1&x=3"},
		{"site skipped: its raw rules don't run",
			"https://example.com/keep/track/42?utm_source=2",
			"https://example.com/keep/track/42"},
		{"site skipped: its redirections don't run",
			"https://example.com/keep/go?to=https%3A%2F%2Fother.org%2F&utm_source=1",
			"https://example.com/keep/go?to=https%3A%2F%2Fother.org%2F"},
		{"globalRules skipped: site still applies",
			"https://example.com/raw?site_id=1&utm_source=2",
			"https://example.com/raw?utm_source=2"},
		{"every provider skipped on one URL is left alone",
			"https://example.com/keep/raw?site_id=1",
			"https://example.com/keep/raw?site_id=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.Clean(tt.input).Output; got != tt.want {
				t.Errorf("Clean(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Step 5: redirections.
func TestRedirections(t *testing.T) {
	c := fixture(t, `{"providers": {
		"a": {
			"urlPattern": "^https?://r\\.example/",
			"redirections": ["^https?://r\\.example/a\\?u=([^&]+)"]
		},
		"b": {
			"urlPattern": "^https?://r\\.example/",
			"redirections": ["^https?://r\\.example/.*?&u=([^&]+)", "^https?://r\\.example/nogroup"]
		},
		"globalRules": {"urlPattern": ".*", "rules": ["utm_source"]}
	}}`)

	tests := []struct {
		name      string
		input     string
		want      string
		unwrapped []string
	}{
		{"simple unwrap, then clean",
			"https://r.example/a?u=https%3A%2F%2Fdest.org%2F%3Futm_source%3Dx%26id%3D1",
			"https://dest.org/?id=1",
			[]string{"https://dest.org/?utm_source=x&id=1"}},
		{"provider a's redirection is tried first",
			"https://r.example/a?u=https%3A%2F%2Ffirst.org%2F&u=https%3A%2F%2Fsecond.org%2F",
			"https://first.org/",
			[]string{"https://first.org/"}},
		{"target can't be decoded: next redirection is tried",
			"https://r.example/a?u=mailto%3Ax%40y.com&u=https%3A%2F%2Fok.org%2F",
			"https://ok.org/",
			[]string{"https://ok.org/"}},
		{"no usable target: keep and clean the current URL",
			"https://r.example/a?u=javascript%3Aalert(1)&utm_source=x",
			"https://r.example/a?u=javascript%3Aalert(1)",
			nil},
		{"redirection without a capture group is ignored",
			"https://r.example/nogroup?utm_source=x",
			"https://r.example/nogroup",
			nil},
		{"target without a scheme gets https://",
			"https://r.example/a?u=dest.org%2Fpage",
			"https://dest.org/page",
			[]string{"https://dest.org/page"}},
		{"target whitespace is trimmed (restart from step 1)",
			"https://r.example/a?u=https%3A%2F%2Fdest.org%2Fp%20",
			"https://dest.org/p",
			[]string{"https://dest.org/p"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := c.Clean(tt.input)
			if res.Output != tt.want || !slices.Equal(res.Unwrapped, tt.unwrapped) || res.HopLimitReached {
				t.Errorf("Clean(%q)\n got  %q %q limit=%v\n want %q %q", tt.input, res.Output, res.Unwrapped, res.HopLimitReached, tt.want, tt.unwrapped)
			}
		})
	}
}

// hopChain returns a URL that redirects n times through h1.example,
// h2.example, ... before reaching h(n+1).example.
func hopChain(n int) string {
	s := ""
	for i := n + 1; i >= 1; i-- {
		if s == "" {
			s = "https://h" + strconv.Itoa(i) + ".example/end"
			continue
		}
		s = "https://h" + strconv.Itoa(i) + ".example/?next=" + s
	}
	return s
}

// Step 5: stop after MaxHops hops.
func TestHopLimit(t *testing.T) {
	c := fixture(t, `{"providers": {
		"hops": {
			"urlPattern": "^https?://h[0-9]\\.example/",
			"redirections": ["^https?://h[0-9]\\.example/\\?next=(.+)"]
		},
		"loop": {
			"urlPattern": "^https?://loop\\.example/",
			"redirections": ["^https?://loop\\.example/\\?self=(.+)"]
		}
	}}`)

	t.Run("chain of 7 stops after 5 hops", func(t *testing.T) {
		res := c.Clean(hopChain(7))
		if !res.HopLimitReached {
			t.Error("HopLimitReached = false, want true")
		}
		if len(res.Unwrapped) != MaxHops {
			t.Fatalf("unwrapped %d hops, want %d: %q", len(res.Unwrapped), MaxHops, res.Unwrapped)
		}
		for i, hop := range res.Unwrapped {
			want := "https://h" + strconv.Itoa(i+2) + ".example/"
			if !strings.HasPrefix(hop, want) {
				t.Errorf("hop %d = %q, want prefix %q", i+1, hop, want)
			}
		}
		if want := "https://h6.example/?next=https://h7.example/?next=https://h8.example/end"; res.Output != want {
			t.Errorf("Output = %q, want %q (the last URL reached)", res.Output, want)
		}
	})

	t.Run("chain of exactly 5 is followed to the end", func(t *testing.T) {
		res := c.Clean(hopChain(5))
		if res.HopLimitReached || len(res.Unwrapped) != 5 || res.Output != "https://h6.example/end" {
			t.Errorf("got %q, %d hops, limit=%v", res.Output, len(res.Unwrapped), res.HopLimitReached)
		}
	})

	t.Run("redirect loop terminates", func(t *testing.T) {
		res := c.Clean("https://loop.example/?self=https://loop.example/?self=https://loop.example/?self=https://loop.example/?self=https://loop.example/?self=https://loop.example/?self=https://loop.example/?self=x")
		if !res.HopLimitReached || len(res.Unwrapped) != MaxHops {
			t.Errorf("got %d hops, limit=%v", len(res.Unwrapped), res.HopLimitReached)
		}
	})
}

// Step 6: raw rules and their safety check.
func TestRawRules(t *testing.T) {
	c := fixture(t, `{"providers": {
		"breaker": {
			"urlPattern": "example\\.com",
			"rawRules": [
				"^https://",
				"example\\.com",
				"evil\\.",
				":8080",
				"/track/[0-9]+",
				"x*",
				";sid=[a-z]+"
			]
		}
	}}`)

	tests := []struct {
		name    string
		input   string
		want    string
		removed []Removal
	}{
		{"rule removing the scheme is discarded, later rules still run",
			"https://example.com/a/track/1",
			"https://example.com/a",
			[]Removal{{"/track/1", "breaker", RawRule}}},
		{"rule removing the host is discarded",
			"https://example.com/",
			"https://example.com/",
			nil},
		{"rule changing the host is discarded",
			"https://evil.example.com/track/7",
			"https://evil.example.com",
			[]Removal{{"/track/7", "breaker", RawRule}}},
		{"rule changing the port is discarded",
			"https://example.com:8080/p",
			"https://example.com:8080/p",
			nil},
		{"every match is removed",
			"https://example.com/p;sid=abc/q;sid=def",
			"https://example.com/p/q",
			[]Removal{{";sid=abc", "breaker", RawRule}, {";sid=def", "breaker", RawRule}}},
		{"empty matches remove nothing",
			"https://example.com/",
			"https://example.com/",
			nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := c.Clean(tt.input)
			if res.Output != tt.want || !slices.Equal(res.Removed, tt.removed) {
				t.Errorf("Clean(%q)\n got  %q %+v\n want %q %+v", tt.input, res.Output, res.Removed, tt.want, tt.removed)
			}
		})
	}
}

// Steps 7 and 8.
func TestRemoveParams(t *testing.T) {
	c := fixture(t, `{"providers": {
		"globalRules": {
			"urlPattern": ".*",
			"rules": ["utm_[a-z]+", "fbclid"],
			"referralMarketing": ["ref"]
		}
	}}`)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"whole name only", "https://e.com/?utm_source=1&xutm_source=2&utm_source3=3", "https://e.com/?xutm_source=2&utm_source3=3"},
		{"case-insensitive", "https://e.com/?FBCLID=1&UTM_Source=2", "https://e.com/"},
		{"referralMarketing", "https://e.com/?ref=1&refresh=2", "https://e.com/?refresh=2"},
		{"repeated", "https://e.com/?fbclid=1&a=1&fbclid=2&a=2", "https://e.com/?a=1&a=2"},
		{"valueless kept", "https://e.com/?flag&fbclid=1", "https://e.com/?flag"},
		{"valueless removed", "https://e.com/?fbclid&flag", "https://e.com/?flag"},
		{"empty pieces kept", "https://e.com/?&a=1&&fbclid=1&", "https://e.com/?&a=1&&"},
		{"empty query kept", "https://e.com/?", "https://e.com/?"},
		{"all removed drops ?", "https://e.com/p?fbclid=1&ref=2", "https://e.com/p"},
		{"all removed, fragment kept", "https://e.com/p?fbclid=1#top", "https://e.com/p#top"},
		{"encoded name", "https://e.com/?utm%5Fsource=1&%66bclid=2", "https://e.com/"},
		{"plus is not a space", "https://e.com/?utm+source=1", "https://e.com/?utm+source=1"},
		{"malformed escape: raw name tested", "https://e.com/?fbclid%=1&fbclid%2=2", "https://e.com/?fbclid%=1&fbclid%2=2"},
		{"encoding of the rest kept", "https://e.com/?a=%41%2b+&fbclid=1&b=%E2%82%AC", "https://e.com/?a=%41%2b+&b=%E2%82%AC"},
		{"value with = kept", "https://e.com/?a=b=c&fbclid=1", "https://e.com/?a=b=c"},
		{"? inside the fragment is not a query", "https://e.com/p#x?fbclid=1", "https://e.com/p#x?fbclid=1"},
		{"fragment without =", "https://e.com/#fbclid", "https://e.com/#fbclid"},
		{"fragment params", "https://e.com/#a=1&fbclid=2&a=3", "https://e.com/#a=1&a=3"},
		{"fragment, nothing removed: not rebuilt", "https://e.com/#a=1&&b", "https://e.com/#a=1&&b"},
		{"fragment, all removed drops #", "https://e.com/?a=1#fbclid=2", "https://e.com/?a=1"},
		{"text fragment", "https://e.com/#:~:text=a=b&ref=x", "https://e.com/#:~:text=a=b"},
		{"host and path untouched", "https://E.COM/A%2fB/?fbclid=1", "https://E.COM/A%2fB/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.Clean(tt.input).Output; got != tt.want {
				t.Errorf("Clean(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestNonHTTPUnchanged checks step 2: other schemes are returned as given
// (after trimming), even when a provider would match them.
func TestNonHTTPUnchanged(t *testing.T) {
	c := fixture(t, `{"providers": {"globalRules": {"urlPattern": ".*", "rules": ["utm_source"], "rawRules": ["x"]}}}`)
	for _, in := range []string{
		"mailto:x@example.com?utm_source=1",
		"ftp://example.com/x?utm_source=1",
		"wss://example.com/?utm_source=1",
		"data:text/plain,x?utm_source=1",
	} {
		res := c.Clean(" " + in + " ")
		if res.Err != nil || res.Output != in || res.Changed || len(res.Removed) > 0 {
			t.Errorf("Clean(%q) = %+v, want %q unchanged", in, res, in)
		}
	}
}

// TestConcurrent runs the cleaner from several goroutines; with -race it
// checks that a Cleaner is safe to share.
func TestConcurrent(t *testing.T) {
	c := bundled(t)
	done := make(chan bool)
	for range 4 {
		go func() {
			for _, u := range benchURLs {
				c.Clean(u)
			}
			done <- true
		}()
	}
	for range 4 {
		<-done
	}
}
