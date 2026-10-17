package cleaner

import "testing"

// benchURLs is a realistic mix: mostly ordinary links, some with trackers,
// some site-specific, a few redirects and a long URL.
var benchURLs = []string{
	"https://example.com/",
	"https://en.wikipedia.org/wiki/Uniform_Resource_Locator",
	"https://go.dev/doc/effective_go#names",
	"https://news.ycombinator.com/item?id=41234567",
	"https://github.com/pinebench/decrumb/pull/5?utm_source=x",
	"https://www.youtube.com/watch?v=dQw4w9WgXcQ&si=AbCdEfGh&feature=share",
	"https://youtu.be/dQw4w9WgXcQ?si=AbCdEfGh",
	"https://www.nytimes.com/2026/10/16/technology/ai.html?smid=tw-share&utm_source=twitter",
	"https://medium.com/@someone/a-post-1a2b3c?source=email-abc&utm_campaign=digest",
	"https://shop.example.com/item/42?color=red&size=m&gclid=Cj0KCQjw&utm_medium=cpc",
	"https://www.amazon.com/Some-Book/dp/B000000000/ref=sr_1_1?crid=2ABC&keywords=go&qid=1700000000&sr=8-1",
	"https://www.amazon.de/dp/B000000000?tag=affiliate-21&th=1&psc=1",
	"https://www.google.com/search?q=decrumb&ei=abc&ved=0ah&oq=decrumb&gs_lcrp=xyz&sourceid=chrome&ie=UTF-8",
	"https://www.google.com/url?q=https://example.com/page%3Futm_source%3Dx%26id%3D7&sa=D&ust=1",
	"https://l.facebook.com/l.php?u=https%3A%2F%2Fexample.com%2Fpage%3Fid%3D7%26fbclid%3DIwAR1&h=AT0abc",
	"https://www.facebook.com/somepage?__tn__=K-R&eid=ARB&ref=bookmarks",
	"https://www.linkedin.com/posts/someone_activity-123?utm_source=share&utm_medium=member_desktop",
	"https://twitter.com/someone/status/1234567890?s=20&t=abcdef",
	"https://www.reddit.com/r/golang/comments/abc/title/?share_id=xyz&utm_content=1",
	"example.com/page?fbclid=IwAR0abc",
	"https://docs.example.com/guide/getting-started#install",
	"https://example.com/search?q=caf%C3%A9+au+lait&page=3&utm_source=newsletter&utm_medium=email&utm_campaign=autumn&utm_term=coffee&utm_content=link1&mc_eid=abc123&_hsenc=p2ANqtz",
}

// BenchmarkClean cleans the mix; one op is one URL. Spec §11 asks for
// 10,000 URLs in under a second, that is under 100µs per URL.
func BenchmarkClean(b *testing.B) {
	c := bundled(b)
	i := 0
	for b.Loop() {
		c.Clean(benchURLs[i%len(benchURLs)])
		i++
	}
}

// BenchmarkClean10k cleans 10,000 URLs per op, the spec §11 workload.
func BenchmarkClean10k(b *testing.B) {
	c := bundled(b)
	for b.Loop() {
		for i := range 10_000 {
			c.Clean(benchURLs[i%len(benchURLs)])
		}
	}
}
