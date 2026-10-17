package scheme

import "testing"

func TestHTTPLen(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"http://example.com", 7},
		{"https://example.com", 8},
		{"HTTPS://EXAMPLE.COM", 8},
		{"hTtP://x", 7},
		{"https:/example.com", 0},
		{"https:", 0},
		{"ftp://example.com", 0},
		{"example.com", 0},
		{"", 0},
	}
	for _, tt := range tests {
		if got := HTTPLen(tt.s); got != tt.want {
			t.Errorf("HTTPLen(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestHas(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		// Schemes.
		{"https://example.com", true},
		{"HTTP://example.com", true},
		{"mailto:a@b.com", true},
		{"mailto:x", true},
		{"javascript:alert(1)", true},
		{"ftp://example.com", true},
		{"data:text/plain,hi", true},
		{"svn+ssh://host/repo", true},
		{"localhost:abc", true}, // not a port, so "localhost" is read as a scheme

		// Host and port, not a scheme.
		{"example.com:8080/path", false},
		{"example.com:8080", false},
		{"example.com:8080?q=1", false},
		{"example.com:8080#top", false},
		{"localhost:3000", false},

		// No scheme at all.
		{"example.com/article", false},
		{"amp/s/example.com/article", false},
		{"user:pass@example.com", true}, // "user:" is followed by text, not a port
		{"a@b.com", false},
		{"foo", false},
		{":foo", false},
		{"foo:", true},
		{"//example.com", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := Has(tt.s); got != tt.want {
			t.Errorf("Has(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}
