package decode

import (
	"strings"
	"testing"
)

// FuzzDecode checks, for any input, that decoding never panics, matches the
// round-by-round reference (fully and with a limit of 0 to 6 rounds, and for
// Target), and terminates: the fuzzer reports an input that hangs.
//
// go test runs only the seed inputs below. To search for new failures:
//
//	go test -run '^$' -fuzz FuzzDecode -fuzztime 60s ./internal/decode
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		"",
		"%",
		"%%34%31",
		"%2541",
		"https%3A%2F%2Fexample.com%2F%3Fq%3Dsalt%2526pepper",
		"https%253A%252F%252Fexample.com",
		"h%2574tps://example.com/",
		"HTTPS%3A%2F%2FEXAMPLE.COM",
		"example.com/article",
		"example.com:8080/path",
		"mailto:a@b.com",
		"user:pass@example.com",
		"%G1%4%+",
		"%" + strings.Repeat("25", 20) + "41",
		nest("https://example.com/a?b=c", 4),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		checkAgainstReference(t, s)
	})
}
