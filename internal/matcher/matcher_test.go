package matcher

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/pinebench/decrumb/internal/rules"
)

// bundled loads the real bundled rules, failing the test on error.
func bundled(tb testing.TB) *rules.Rules {
	tb.Helper()
	r, err := rules.LoadBundled()
	if err != nil {
		tb.Fatal(err)
	}
	return r
}

// names returns the provider names, in order.
func names(providers []*rules.Provider) []string {
	var out []string
	for _, p := range providers {
		out = append(out, p.Name)
	}
	return out
}

func TestMatchBundled(t *testing.T) {
	m := New(bundled(t))

	tests := []struct {
		name string
		url  string
		want []string
	}{
		// Subdomains.
		{"facebook mobile subdomain", "https://m.facebook.com/story.php?id=1", []string{"facebook", GlobalRules}},
		{"facebook no subdomain", "https://facebook.com/page", []string{"facebook", GlobalRules}},
		{"google www", "https://www.google.com/search?q=go", []string{"google", GlobalRules}},
		{"youtube short domain", "https://youtu.be/abc?si=x", []string{"youtube", GlobalRules}},

		// Country domains.
		{"amazon.co.uk", "https://www.amazon.co.uk/dp/B000000000", []string{"amazon", GlobalRules}},
		{"amazon.de", "https://www.amazon.de/dp/B000000000", []string{"amazon", GlobalRules}},
		{"google.co.in", "https://www.google.co.in/search?q=go", []string{"google", GlobalRules}},

		// Several site-specific providers: alphabetical, then globalRules.
		{"amazon search page", "https://www.amazon.com/s?k=book", []string{"amazon", "amazon search", GlobalRules}},
		{"linkedin learning", "https://www.linkedin.com/learning/go", []string{"LinkedIn", "LinkedIn Learning", GlobalRules}},
		{"youtube pagead", "https://www.youtube.com/pagead/x", []string{"youtube", "youtube_pagead", GlobalRules}},

		// No site-specific provider.
		{"unknown site", "https://example.com/page?utm_source=x", []string{GlobalRules}},
		{"http scheme", "http://example.org/", []string{GlobalRules}},

		// Case-insensitive: patterns are compiled with (?i).
		{"uppercase scheme and host", "HTTPS://WWW.AMAZON.COM/dp/B000000000", []string{"amazon", GlobalRules}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(m.Match(tt.url))
			if !slices.Equal(got, tt.want) {
				t.Errorf("Match(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// TestMatchLookalikeHosts records what the upstream patterns do with hosts
// that only look like a known site. These are not decrumb's choices: the
// patterns are applied exactly as upstream wrote them (spec Â§7, matcher).
func TestMatchLookalikeHosts(t *testing.T) {
	m := New(bundled(t))

	tests := []struct {
		name string
		url  string
		want []string
	}{
		// "amazon" must start the host or follow a dot, so this is not amazon.
		{"prefix before amazon", "https://notamazon.example/dp/1", []string{GlobalRules}},
		// Patterns have no end boundary: amazon(?:\.[a-z]{2,}){1,} matches
		// "amazon.com.example.org", so this is treated as amazon.
		{"amazon.com as subdomain", "https://amazon.com.example.org/dp/1", []string{"amazon", GlobalRules}},
		// Any amazon.<letters> counts, not just real Amazon domains.
		{"amazon on any tld", "https://amazon.example/dp/1", []string{"amazon", GlobalRules}},
		// The text before "@" is userinfo, not the host, but the pattern
		// only sees the string.
		{"amazon in userinfo", "https://amazon.com@example.org/", []string{"amazon", GlobalRules}},
		// github\.com is not followed by a boundary.
		{"github.community", "https://github.community/t/1", []string{"github.com", GlobalRules}},
		// x's pattern is x.com with an unescaped dot and no end boundary.
		{"x pattern unescaped dot", "https://xbcom.example/", []string{"x", GlobalRules}},
		// shutterstock's pattern has no ^, so it matches inside the query.
		{"shutterstock in query", "https://example.com/?u=https://www.shutterstock.com/a", []string{"shutterstock.com", GlobalRules}},
		// ...and it requires a subdomain.
		{"shutterstock without subdomain", "https://shutterstock.com/a", []string{GlobalRules}},
		// vk.com allows no subdomain at all.
		{"vk mobile subdomain", "https://m.vk.com/id1", []string{GlobalRules}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(m.Match(tt.url))
			if !slices.Equal(got, tt.want) {
				t.Errorf("Match(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// TestMatchCaseInsensitive checks that uppercase URLs match the same
// providers as their lowercase form.
func TestMatchCaseInsensitive(t *testing.T) {
	m := New(bundled(t))

	for _, url := range []string{
		"https://www.amazon.com/dp/B000000000",
		"https://www.amazon.com/s?k=book",
		"https://m.facebook.com/story.php?id=1",
		"https://www.google.co.in/search?q=go",
		"https://www.linkedin.com/learning/go",
	} {
		upper := strings.ToUpper(url)
		got := names(m.Match(upper))
		want := names(m.Match(url))
		if !slices.Equal(got, want) {
			t.Errorf("Match(%q) = %q, want %q (same as lowercase)", upper, got, want)
		}
	}
}

// TestMatchOrderBundled checks the order on the real rules: globalRules is
// always present and last, and the providers before it are sorted.
func TestMatchOrderBundled(t *testing.T) {
	m := New(bundled(t))

	for _, url := range benchURLs {
		got := names(m.Match(url))
		if len(got) == 0 || got[len(got)-1] != GlobalRules {
			t.Errorf("Match(%q) = %q, want globalRules last", url, got)
			continue
		}
		site := got[:len(got)-1]
		if !slices.IsSorted(site) {
			t.Errorf("Match(%q) = %q, site-specific providers not sorted", url, got)
		}
		if slices.Contains(site, GlobalRules) {
			t.Errorf("Match(%q) = %q, globalRules listed twice", url, got)
		}
	}
}

// TestMatchFixture tests the ordering logic on a small hand-written rules
// set, independently of the upstream file. The providers are deliberately
// not sorted, and globalRules is not last.
func TestMatchFixture(t *testing.T) {
	provider := func(name, pattern string) rules.Provider {
		return rules.Provider{Name: name, URLPattern: regexp.MustCompile(pattern)}
	}
	fixture := &rules.Rules{Providers: []rules.Provider{
		provider("zeta", `^https://a\.test`),
		provider(GlobalRules, `.*`),
		provider("beta", `^https://a\.test/b`),
		provider("alpha", `^https://a\.test`),
		provider("Zulu", `^https://a\.test`),
		provider("other", `^https://other\.test`),
	}}
	noGlobal := &rules.Rules{Providers: []rules.Provider{
		provider("site", `^https://a\.test`),
	}}
	globalNotMatching := &rules.Rules{Providers: []rules.Provider{
		provider(GlobalRules, `^https://only\.test`),
		provider("site", `^https://a\.test`),
	}}

	tests := []struct {
		name  string
		rules *rules.Rules
		url   string
		want  []string
	}{
		// Names sort by byte order, so uppercase comes before lowercase.
		{"several providers", fixture, "https://a.test/b", []string{"Zulu", "alpha", "beta", "zeta", GlobalRules}},
		{"one provider", fixture, "https://other.test/", []string{"other", GlobalRules}},
		{"only global", fixture, "https://none.test/", []string{GlobalRules}},
		{"no globalRules provider", noGlobal, "https://a.test/", []string{"site"}},
		{"globalRules pattern is tested too", globalNotMatching, "https://a.test/", []string{"site"}},
		{"nothing matches", noGlobal, "https://none.test/", nil},
		{"empty rules", &rules.Rules{}, "https://a.test/", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(New(tt.rules).Match(tt.url))
			if !slices.Equal(got, tt.want) {
				t.Errorf("Match(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// benchURLs is a mix of URLs people share: big sites with their own
// providers, country domains, and sites only globalRules covers.
var benchURLs = []string{
	"https://www.google.com/search?q=golang+regexp&oq=golang&sourceid=chrome&ie=UTF-8",
	"https://www.google.co.in/url?q=https%3A%2F%2Fexample.com%2F&sa=D&ust=1",
	"https://www.amazon.com/dp/B08N5WRWNW?ref_=nav_custrec_signin&pd_rd_w=abc&th=1",
	"https://www.amazon.co.uk/s?k=headphones&crid=2X&sprefix=head",
	"https://www.amazon.de/gp/product/B07XJ8C8F5/?tag=foo-21&linkCode=ll1",
	"https://www.youtube.com/watch?v=dQw4w9WgXcQ&si=abcdef&feature=share",
	"https://youtu.be/dQw4w9WgXcQ?si=abcdef",
	"https://m.facebook.com/story.php?story_fbid=123&id=456&mibextid=abc",
	"https://www.facebook.com/l.php?u=https%3A%2F%2Fexample.com%2F&h=AT0",
	"https://twitter.com/user/status/1?s=20&t=abc",
	"https://x.com/user/status/1?s=46",
	"https://www.instagram.com/p/abc/?igshid=xyz",
	"https://www.linkedin.com/posts/someone_activity-1?utm_source=share&utm_medium=member_desktop",
	"https://www.reddit.com/r/golang/comments/abc/title/?utm_source=share&utm_medium=web2x",
	"https://open.spotify.com/track/abc?si=def",
	"https://github.com/golang/go/issues/1?utm_source=newsletter",
	"https://www.nytimes.com/2026/10/01/technology/article.html?smid=nytcore-ios-share",
	"https://www.theguardian.com/world/2026/oct/01/story?CMP=share_btn_link",
	"https://www.ebay.co.uk/itm/123?_trkparms=abc&mkcid=16",
	"https://www.bing.com/search?q=go&form=QBLH",
	"https://example.com/blog/post?utm_source=newsletter&utm_medium=email&utm_campaign=oct",
	"https://news.example.org/a/b/c?fbclid=IwAR0abc&gclid=xyz",
	"https://docs.example.dev/guide#section-2",
	"https://shop.example.net/product/42?ref=homepage&color=blue&size=m",
	"https://blog.example.io/2026/10/hello-world/",
}

// matchAll is the plain version of Match: it runs every regex, with no
// literal shortcut. The tests compare Match against it.
func matchAll(r *rules.Rules, rawURL string) []string {
	var site []string
	global := false
	for _, p := range r.Providers {
		if !p.URLPattern.MatchString(rawURL) {
			continue
		}
		if p.Name == GlobalRules {
			global = true
			continue
		}
		site = append(site, p.Name)
	}
	slices.Sort(site)
	if global {
		site = append(site, GlobalRules)
	}
	return site
}

// TestMatchSameAsPlain checks that the literal shortcut never changes the
// result, on the real rules.
func TestMatchSameAsPlain(t *testing.T) {
	r := bundled(t)
	m := New(r)

	urls := slices.Clone(benchURLs)
	urls = append(urls,
		"https://amazon.com.example.org/dp/1",
		"https://amazon.com@example.org/",
		"https://example.com/?u=https://www.shutterstock.com/a",
		"https://www.linkedin.com/learning/go",
		"HTTPS://WWW.AMAZON.CO.UK/S?K=BOOK",
		// Non-ASCII: (?i)s matches "ſ" and (?i)k matches the Kelvin sign,
		// so these must match although their lowercase form doesn't
		// contain "shutterstock" or "kahoot".
		"https://www.ſhutterstock.com/a",
		"https://img.ſhutterſtock.com/a",
		"https://Kahoot.it/challenge",
		"https://www.amazon.de/dp/été",
		"https://bücher.example/",
	)
	// And the uppercase form of each.
	for _, url := range urls {
		urls = append(urls, strings.ToUpper(url))
	}

	for _, url := range urls {
		got := names(m.Match(url))
		want := matchAll(r, url)
		if !slices.Equal(got, want) {
			t.Errorf("Match(%q) = %q, plain match gives %q", url, got, want)
		}
	}

	// The non-ASCII cases really do match through case folding.
	for _, tc := range []struct{ url, provider string }{
		{"https://www.ſhutterstock.com/a", "shutterstock.com"},
		{"https://Kahoot.it/challenge", "kahoot.it"},
	} {
		if !slices.Contains(names(m.Match(tc.url)), tc.provider) {
			t.Errorf("Match(%q) does not include %q", tc.url, tc.provider)
		}
	}
}

func TestRequiredLiteral(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{`(?i)^https?:\/\/(?:[a-z0-9-]+\.)*?amazon(?:\.[a-z]{2,}){1,}`, "amazon"},
		{`(?i)https?:\/\/([a-z0-9-.]*\.)shutterstock\.com`, "shutterstock.com"},
		{`(?i)^https?:\/\/(?:[a-z0-9-]+\.)*?youtube\.com\/pagead`, "youtube.com/pagead"},
		// The site name is inside a group, so only "http" is found.
		{`(?i)^https?:\/\/(?:[a-z0-9-]+\.)*?(youtube\.com|youtu\.be)`, "http"},
		// "." is a wildcard, so it splits the literal.
		{`(?i)^https?:\/\/(?:[a-z0-9-]+\.)*?netflix.com`, "netflix"},
		{`(?i)AbC`, "abc"},
		{`(?i).*`, ""},
		{`(?i)a|b`, ""},
		// Non-ASCII literals are not used.
		{`(?i)^https?://bücher\.example`, "http"},
	}

	for _, tt := range tests {
		got := requiredLiteral(regexp.MustCompile(tt.pattern))
		if got != tt.want {
			t.Errorf("requiredLiteral(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
	}
}

// BenchmarkMatch reports the matching time per URL on the bundled rules.
func BenchmarkMatch(b *testing.B) {
	m := New(bundled(b))
	i := 0
	for b.Loop() {
		m.Match(benchURLs[i%len(benchURLs)])
		i++
	}
}

// BenchmarkMatchPlain is matchAll, for comparison: every regex, every URL.
func BenchmarkMatchPlain(b *testing.B) {
	r := bundled(b)
	i := 0
	for b.Loop() {
		matchAll(r, benchURLs[i%len(benchURLs)])
		i++
	}
}
