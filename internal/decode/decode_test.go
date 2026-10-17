package decode

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"
)

// refRound is the reference for one decoding round: it decodes every valid
// %HH once, left to right, and leaves everything else as it is. It is
// deliberately simple and shares no code with the package.
func refRound(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '%' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// refDecode is the reference for decodeRounds: limit rounds of refRound, or
// rounds until nothing changes if limit is noLimit.
func refDecode(s string, limit int) string {
	for round := 0; limit == noLimit || round < limit; round++ {
		next := refRound(s)
		if next == s {
			break
		}
		s = next
	}
	return s
}

// refTarget is the reference for Target, written straight from the spec:
// decode round by round until the text starts with http(s)://; if it never
// does, decode once.
func refTarget(s string) (string, bool) {
	k := 1
	cur := s
	for round := 1; ; round++ {
		next := refRound(cur)
		lower := strings.ToLower(next)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			k = round
			break
		}
		if next == cur {
			break // fully decoded, no scheme: k stays 1
		}
		cur = next
	}
	return usable(refDecode(s, k))
}

func TestTarget(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string // "" means not usable
	}{
		// How many rounds.
		{"plain target is decoded once", "https://example.com/?q=a%2Bb", "https://example.com/?q=a+b"},
		{"single-encoded", "https%3A%2F%2Fexample.com%2Fpage%3Fid%3D7", "https://example.com/page?id=7"},
		{"double-encoded", "https%253A%252F%252Fexample.com%252Fpage%253Fid%253D7", "https://example.com/page?id=7"},
		{"triple-encoded", "https%25253A%25252F%25252Fexample.com%25252Fpage", "https://example.com/page"},
		{"lowercase hex", "https%3a%2f%2fexample.com%2fpage", "https://example.com/page"},
		{"http scheme", "http%3A%2F%2Fexample.com%2F", "http://example.com/"},
		{"only one scheme byte encoded", "%68ttps://example.com/", "https://example.com/"},
		{"scheme byte encoded twice", "h%2574tps://example.com/", "https://example.com/"},
		{"deeply nested", nest("https://example.com/page", 50), "https://example.com/page"},

		// Decode only as much as needed.
		{"salt%26pepper stays encoded", "https%3A%2F%2Fexample.com%2Fsearch%3Fq%3Dsalt%2526pepper", "https://example.com/search?q=salt%26pepper"},
		{"salt%26pepper double-wrapped", "https%253A%252F%252Fexample.com%252Fsearch%253Fq%253Dsalt%252526pepper", "https://example.com/search?q=salt%26pepper"},
		{"rest of the target deeper than the scheme", "https%3A%2F%2Fexample.com%2F%3Fq%3D%25252F", "https://example.com/?q=%252F"},

		// Bytes left alone.
		{"plus is kept", "https%3A%2F%2Fexample.com%2F%3Fq%3Da+b", "https://example.com/?q=a+b"},
		{"invalid escapes kept in the query", "https%3A%2F%2Fexample.com%2F%3Fa%3D100%%26b%3D%G1%26c%3D%4", "https://example.com/?a=100%&b=%G1&c=%4"},
		{"invalid escape in the path does not parse", "https%3A%2F%2Fexample.com%2F100%", ""},
		{"uppercase scheme", "HTTPS%3A%2F%2FEXAMPLE.COM%2FPage", "HTTPS://EXAMPLE.COM/Page"},
		{"mixed-case scheme", "HtTp%3A%2F%2Fexample.com", "HtTp://example.com"},

		// No scheme: https:// is added.
		{"google amp target", "example.com/article", "https://example.com/article"},
		{"encoded target without scheme", "example.com%2Farticle%3Fid%3D1", "https://example.com/article?id=1"},
		{"no scheme decoded only once", "example.com%2F%253F", "https://example.com/%3F"},
		{"host and port", "example.com:8080/path", "https://example.com:8080/path"},
		{"host and port only", "example.com:8080", "https://example.com:8080"},
		{"IPv4 address", "192.168.0.1/admin", "https://192.168.0.1/admin"},
		{"IPv6 address", "[2001:db8::1]:8080/x", "https://[2001:db8::1]:8080/x"},
		{"IPv4-mapped IPv6 address", "[::ffff:192.0.2.1]/x", "https://[::ffff:192.0.2.1]/x"},

		// Protocol-relative: https: is added.
		{"encoded protocol-relative", "%2F%2Fexample.com%2Fa", "https://example.com/a"},
		{"protocol-relative without path", "//example.com", "https://example.com"},
		{"protocol-relative, no host", "///example.com", ""},
		{"protocol-relative with user info", "//user@example.com", ""},
		{"protocol-relative, no dot in host", "//foo/x", ""},

		// Not usable.
		{"mailto", "mailto:a@b.com", ""},
		{"javascript", "javascript:alert(1)", ""},
		{"encoded javascript", "javascript%3Aalert(1)", ""},
		{"ftp", "ftp%3A%2F%2Fexample.com%2F", ""},
		{"data", "data:text/html,hi", ""},
		{"no dot in host", "foo", ""},
		{"no dot in host with path", "localhost:8080/x", ""},
		{"user and password", "user:pass@example.com", ""},
		{"user only", "user@example.com", ""},
		{"empty", "", ""},
		{"lone percent", "%", ""},
		{"only percents", "%%%", ""},
		{"spaces", "not a url", ""},
		{"never becomes a URL", "%" + strings.Repeat("25", 10) + "41", ""},
		{"scheme without host", "https%3A%2F%2F", ""},
		{"scheme only, unencoded", "https://", ""},
		{"invalid UTF-8", "https%3A%2F%2Fexample.com%2F%FF", ""},
		{"brackets that are not IPv6", "[zz]/x", ""},
		{"brackets that look like IPv6", "[1:2]/x", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Target(tt.target)
			if ok != (tt.want != "") || got != tt.want {
				t.Errorf("Target(%q) = %q, %v; want %q, %v", tt.target, got, ok, tt.want, tt.want != "")
			}
		})
	}
}

func TestDecodeRounds(t *testing.T) {
	tests := []struct {
		in    string
		limit int
		want  string
	}{
		{"%2541", 0, "%2541"},
		{"%2541", 1, "%41"},
		{"%2541", 2, "A"},
		{"%2541", noLimit, "A"},
		{"%252541", 2, "%41"},
		{"%%34%31", 1, "%41"}, // round 1 makes "4" and "1", which then form an escape
		{"%%34%31", 2, "A"},
		{"%41%42", 1, "AB"},
		{"a+b", noLimit, "a+b"},
		{"%", noLimit, "%"},
		{"%4", noLimit, "%4"},
		{"%G1", noLimit, "%G1"},
		{"%%", noLimit, "%%"},
		{"%%41", noLimit, "%A"},
		{"100%", noLimit, "100%"},
		{"%e2%82%ac", noLimit, "€"},
		{"%FF", noLimit, "\xff"}, // bytes, not runes
		{"", noLimit, ""},
	}
	for _, tt := range tests {
		if got := decodeRounds(tt.in, tt.limit); got != tt.want {
			t.Errorf("decodeRounds(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
		}
	}
}

// TestDecodeMatchesReference compares the fast decoder with the round-by-
// round reference on many random and adversarial inputs, both fully and
// with a limit of 0 to 6 rounds, and compares Target with refTarget.
func TestDecodeMatchesReference(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2)) // fixed seed: failures are reproducible
	for i := 0; i < 20000; i++ {
		s := randomInput(r)
		checkAgainstReference(t, s)
		if t.Failed() {
			return // one input is enough to debug
		}
	}
}

func checkAgainstReference(t *testing.T, s string) {
	t.Helper()
	for limit := noLimit; limit <= 6; limit++ {
		if got, want := decodeRounds(s, limit), refDecode(s, limit); got != want {
			t.Errorf("decodeRounds(%q, %d) = %q, reference %q", s, limit, got, want)
		}
	}
	got, ok := Target(s)
	want, wantOK := refTarget(s)
	if got != want || ok != wantOK {
		t.Errorf("Target(%q) = %q, %v; reference %q, %v", s, got, ok, want, wantOK)
	}
}

// randomInput returns a short string that is likely to contain escapes,
// nested escapes, broken escapes and schemes.
func randomInput(r *rand.Rand) string {
	const alphabet = "%%%%2255aAfF0749Gg+/:.htps"
	var s string
	switch r.IntN(3) {
	case 0: // random bytes from a small alphabet heavy in % and hex digits
		b := make([]byte, r.IntN(24))
		for i := range b {
			b[i] = alphabet[r.IntN(len(alphabet))]
		}
		s = string(b)
	case 1: // a URL, partly encoded a few times over
		s = "https://example.com/p?q=salt%26pepper+x"
		for range r.IntN(5) {
			s = encodeSome(r, s)
		}
	default: // a target without scheme, partly encoded, with junk around
		s = encodeSome(r, "example.com/a?b=%G1%4") + alphabet[r.IntN(len(alphabet)):]
	}
	return s
}

// encodeSome percent-encodes each byte of s with probability 1/2, and every
// % always (so the result decodes back to s).
func encodeSome(r *rand.Rand, s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' || r.IntN(2) == 0 {
			fmt.Fprintf(&b, "%%%02X", s[i])
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// nest percent-encodes every special byte of s, n times over.
func nest(s string, n int) string {
	for range n {
		var b strings.Builder
		for i := 0; i < len(s); i++ {
			if strings.IndexByte("%:/?=&", s[i]) >= 0 {
				fmt.Fprintf(&b, "%%%02X", s[i])
			} else {
				b.WriteByte(s[i])
			}
		}
		s = b.String()
	}
	return s
}

// hostileInputs are deeply nested inputs that make round-by-round decoding
// take len × depth time. The size n is about the nesting depth.
var hostileInputs = map[string]func(n int) string{
	// %252525...41 decodes one level per round, to "A".
	"nested escape": func(n int) string {
		return "%" + strings.Repeat("25", n) + "41"
	},
	// A URL whose scheme only appears after n rounds, so Target's pass 2
	// runs with a large limit too.
	"nested URL": func(n int) string {
		return "https%" + strings.Repeat("25", n) + "3A%" + strings.Repeat("25", n) + "2F%" +
			strings.Repeat("25", n) + "2Fexample.com"
	},
}

// TestScaling checks that doubling the depth of a hostile input about
// doubles the time Target takes (linear), instead of quadrupling it (what
// round-by-round decoding would do).
//
// To stay reliable on slow or busy CI machines, it compares ratios between
// two sizes on the same machine, never absolute times, takes the fastest
// of several runs for each size, and retries a pair a few times before
// failing. A quadratic decoder gives a ratio near 4 every time; the limit
// is 3.
func TestScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	const (
		small    = 1 << 17 // input of about 256 KB
		maxRatio = 3.0
		attempts = 3
	)
	for name, input := range hostileInputs {
		t.Run(name, func(t *testing.T) {
			a, b := input(small), input(2*small)
			if got, ok := Target(a); name == "nested URL" && (!ok || got != "https://example.com") {
				t.Fatalf("Target(nested URL) = %q, %v", got, ok)
			}
			var ratios []string
			for range attempts {
				ta, tb := fastest(a), fastest(b)
				ratio := float64(tb) / float64(ta)
				ratios = append(ratios, fmt.Sprintf("%.2f (%v → %v)", ratio, ta, tb))
				if ratio < maxRatio {
					t.Logf("n=%d → 2n: ratio %s", small, ratios[len(ratios)-1])
					return
				}
			}
			t.Errorf("doubling the input size multiplied the time by more than %.1f: %v", maxRatio, ratios)
		})
	}
}

// fastest returns the fastest of several runs of Target on s. The fastest
// run is the one least disturbed by the garbage collector or other programs.
func fastest(s string) time.Duration {
	best := time.Duration(1<<63 - 1)
	for range 7 {
		start := time.Now()
		Target(s)
		best = min(best, time.Since(start))
	}
	return best
}

func BenchmarkTargetHostile(b *testing.B) {
	for name, input := range hostileInputs {
		for _, n := range []int{1 << 12, 1 << 14, 1 << 16} {
			s := input(n)
			b.Run(fmt.Sprintf("%s/n=%d", name, n), func(b *testing.B) {
				b.SetBytes(int64(len(s)))
				for b.Loop() {
					Target(s)
				}
			})
		}
	}
}
