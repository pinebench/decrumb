// Package decode turns a redirect target captured from a URL into the URL it
// points to (spec §6, "Redirect target decoding").
package decode

import (
	"net/netip"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/pinebench/decrumb/internal/scheme"
)

// Target decodes a redirect target captured by a redirection rule and
// reports whether the result is a usable http(s) URL. If ok is false, the
// returned string is "" and the caller should not unwrap.
//
// The target is a parameter value, so it is decoded once. It is decoded
// further only as long as it was wrapped more than once: the result is the
// target after the fewest rounds (at least one) that make it start with
// http:// or https://, ignoring case. Decoding the rest away could corrupt
// the target (q=salt%2526pepper must stay q=salt%26pepper).
//
// A target that never gets an http(s) scheme is decoded once. If it then has
// no scheme at all, https:// is added, but only when the result has a host
// with a dot in it (or an IP address) and no user info. Other schemes
// (mailto:, javascript:, ftp:, ...) are never usable.
//
// Target runs in time linear in len(target), however deeply it is nested.
func Target(target string) (string, bool) {
	// Pass 1: decode everything, to learn in which round the scheme
	// appears. That round is k.
	k := 1
	if depth, ok := schemeDepth(reduce(target, noLimit)); ok {
		k = max(depth, 1)
	}

	// Pass 2: decode exactly k rounds.
	return usable(decodeRounds(target, k))
}

// noLimit is the limit for decoding until no escapes are left.
const noLimit = -1

// entry is one byte on the decoding stack.
type entry struct {
	b byte

	// depth is the round of round-by-round decoding in which b appears:
	// 0 for bytes of the input, 1 for bytes produced by the first round,
	// and so on.
	depth int
}

// reduce decodes s with a single left-to-right pass over its bytes and
// returns the decoded bytes with their depths. Only escapes that round-by-
// round decoding would decode within limit rounds are decoded; with noLimit,
// every escape is.
//
// Round-by-round decoding repeats "decode every %HH once" and costs
// len(s) × rounds, which a hostile, deeply nested input can make very slow.
// reduce gives the same result in linear time. Each byte is pushed once, and
// each decoding replaces three stack entries with one, so there are fewer
// decodings than bytes.
//
// Why the result is the same as round by round:
//
//   - Escapes never overlap. The two bytes after a % are hex digits, never %,
//     so a byte can belong to at most one %HH at a time. That means the order
//     in which escapes are decoded doesn't change the outcome, so decoding as
//     soon as an escape is complete (here: on the top of the stack) is fine.
//   - Depth equals round. A %HH can be decoded in round r exactly when its
//     three bytes all exist after round r-1, that is when the deepest of
//     them has depth r-1. So the new byte gets depth max(depths)+1.
//   - With a limit, an escape that would need a later round stays as its
//     three bytes. None of them can ever join another escape (see the first
//     point), so nothing below them on the stack changes later either.
func reduce(s string, limit int) []entry {
	stack := make([]entry, 0, len(s))
	for i := 0; i < len(s); i++ {
		stack = append(stack, entry{b: s[i]})

		// A new byte can complete a %HH. The decoded byte can complete
		// another one (as its last hex digit), so keep checking.
		for len(stack) >= 3 {
			n := len(stack)
			pct, hi, lo := stack[n-3], stack[n-2], stack[n-1]
			if pct.b != '%' || !isHex(hi.b) || !isHex(lo.b) {
				break
			}
			depth := max(pct.depth, hi.depth, lo.depth) + 1
			if limit != noLimit && depth > limit {
				break
			}
			stack = append(stack[:n-3], entry{
				b:     unhex(hi.b)<<4 | unhex(lo.b),
				depth: depth,
			})
		}
	}
	return stack
}

// decodeRounds returns s after limit rounds of decoding, or after decoding
// every escape if limit is noLimit.
func decodeRounds(s string, limit int) string {
	stack := reduce(s, limit)
	b := make([]byte, len(stack))
	for i, e := range stack {
		b[i] = e.b
	}
	return string(b)
}

// schemeDepth reports whether the fully decoded stack starts with http:// or
// https:// (ignoring case) and, if so, the round in which that whole prefix
// first appears: the greatest depth among its bytes.
//
// This works because none of the bytes in "http://" or "https://" (in either
// case) is % or a hex digit, so once produced they never change again. After
// round r the text starts with exactly those stack entries whose depth is at
// most r, and the scheme can't appear any earlier: until its deepest byte is
// produced, a % stands in its place.
func schemeDepth(stack []entry) (int, bool) {
	prefix := make([]byte, 0, len("https://"))
	for _, e := range stack[:min(len(stack), len("https://"))] {
		prefix = append(prefix, e.b)
	}

	n := scheme.HTTPLen(string(prefix))
	if n == 0 {
		return 0, false
	}
	depth := 0
	for _, e := range stack[:n] {
		depth = max(depth, e.depth)
	}
	return depth, true
}

// usable checks a decoded target and returns the URL to unwrap to.
func usable(s string) (string, bool) {
	if !utf8.ValidString(s) {
		return "", false
	}

	if scheme.HTTPLen(s) > 0 {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return "", false
		}
		return s, true
	}

	// Any other scheme (mailto:, javascript:, ftp:, ...) is not usable.
	if scheme.Has(s) {
		return "", false
	}

	// No scheme at all, like Google AMP's amp/s/example.com/article.
	s = "https://" + s
	u, err := url.Parse(s)
	if err != nil || u.User != nil || !plausibleHost(u) {
		return "", false
	}
	return s, true
}

// plausibleHost reports whether u's host looks like a real host: a name
// with a dot in it or an IP address. It rejects single words like "foo",
// which are rarely meant as a host.
func plausibleHost(u *url.URL) bool {
	host := u.Hostname() // without port, and without brackets around IPv6
	if strings.Contains(host, ".") {
		return true
	}
	_, err := netip.ParseAddr(host)
	return err == nil
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

// unhex returns the value of the hex digit c, which must satisfy isHex.
func unhex(c byte) byte {
	switch {
	case c <= '9':
		return c - '0'
	case c <= 'F':
		return c - 'A' + 10
	default:
		return c - 'a' + 10
	}
}
