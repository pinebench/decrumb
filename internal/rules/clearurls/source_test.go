package clearurls

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

// TestBundledRulesMatchSource fails if data.min.json was changed by hand
// (or by Git line-ending conversion) instead of by scripts/update-rules.
func TestBundledRulesMatchSource(t *testing.T) {
	src, err := BundledSource()
	if err != nil {
		t.Fatal(err)
	}

	// Read the file from disk as well as the embedded copy, so the check
	// does not depend on which file go:embed picked up.
	onDisk, err := os.ReadFile(src.File)
	if err != nil {
		t.Fatalf("reading rules file: %v", err)
	}

	for name, data := range map[string][]byte{"on disk": onDisk, "embedded": Data} {
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != src.SHA256 {
			t.Errorf("SHA-256 of %s %s is %s, but SOURCE says %s", name, src.File, got, src.SHA256)
		}
	}
}

func TestBundledSourceCommit(t *testing.T) {
	src, err := BundledSource()
	if err != nil {
		t.Fatal(err)
	}
	if len(src.Commit) != 40 {
		t.Errorf("commit %q is not a full 40-character SHA", src.Commit)
	}
}

func TestParseSource(t *testing.T) {
	full := "repository: https://github.com/ClearURLs/Rules\n" +
		"commit: abc\n" +
		"file: data.min.json\n" +
		"downloaded: 2026-10-16\n" +
		"sha256: 0123\n"

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "complete", input: full},
		{name: "CRLF line endings", input: strings.ReplaceAll(full, "\n", "\r\n")},
		{name: "missing commit", input: strings.Replace(full, "commit: abc\n", "", 1), wantErr: `missing "commit"`},
		{name: "empty value", input: strings.Replace(full, "sha256: 0123", "sha256:", 1), wantErr: `missing "sha256"`},
		{name: "empty file", input: "", wantErr: "missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := ParseSource([]byte(tt.input))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want one containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if src.Repository != "https://github.com/ClearURLs/Rules" || src.Commit != "abc" || src.SHA256 != "0123" {
				t.Errorf("unexpected result: %+v", src)
			}
		})
	}
}
