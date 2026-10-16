// Package clearurls holds the bundled ClearURLs rules file (LGPL-3.0) and
// its SOURCE lock file. This test checks that the two agree.
package clearurls

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// TestBundledRulesMatchSource fails if data.min.json was changed by hand
// (or by Git line-ending conversion) instead of by scripts/update-rules.
func TestBundledRulesMatchSource(t *testing.T) {
	source, err := os.ReadFile("SOURCE")
	if err != nil {
		t.Fatalf("reading SOURCE: %v", err)
	}
	fields := parseSource(source)

	for _, key := range []string{"repository", "commit", "file", "downloaded", "sha256"} {
		if fields[key] == "" {
			t.Fatalf("SOURCE is missing %q", key)
		}
	}

	data, err := os.ReadFile(fields["file"])
	if err != nil {
		t.Fatalf("reading rules file: %v", err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])

	if got != fields["sha256"] {
		t.Errorf("SHA-256 of %s is %s, but SOURCE says %s", fields["file"], got, fields["sha256"])
	}
}

// parseSource reads "key: value" lines into a map.
func parseSource(b []byte) map[string]string {
	fields := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if ok {
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return fields
}
