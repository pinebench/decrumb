// Package scheme answers "does this text start with a URL scheme?" the same
// way everywhere: for input URLs in the cleaner (spec §6, step 1) and for
// redirect targets in decode (spec §6, "Redirect target decoding").
package scheme

import "strings"

// HTTPLen returns the length of the http:// or https:// prefix of s,
// ignoring case, or 0 if s has neither.
func HTTPLen(s string) int {
	for _, scheme := range []string{"http://", "https://"} {
		if len(s) >= len(scheme) && strings.EqualFold(s[:len(scheme)], scheme) {
			return len(scheme)
		}
	}
	return 0
}

// Has reports whether s starts with a scheme such as https:, mailto: or
// javascript:. A name followed by a colon and a port number, as in
// example.com:8080/path, is a host and port, not a scheme.
func Has(s string) bool {
	i := 0
	for i < len(s) && isSchemeByte(s[i]) {
		i++
	}
	if i == 0 || i == len(s) || s[i] != ':' {
		return false
	}
	return !startsWithPort(s[i+1:])
}

func isSchemeByte(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' ||
		c == '+' || c == '-' || c == '.'
}

// startsWithPort reports whether s starts with a port number: one or more
// digits, then the end of s or the start of a path, query or fragment.
func startsWithPort(s string) bool {
	i := 0
	for i < len(s) && '0' <= s[i] && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return false
	}
	return i == len(s) || strings.IndexByte("/?#", s[i]) >= 0
}
