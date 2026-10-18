package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestBinary builds the real decrumb binary and runs it on the shared test
// cases (spec §10) through stdin, the way a user's pipeline would.
func TestBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	bin := filepath.Join(t.TempDir(), "decrumb")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-ldflags", "-X main.version=e2e-test", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Input  string `json:"input"`
		Output string `json:"output"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}

	t.Run("cases.json through stdin", func(t *testing.T) {
		var in, want strings.Builder
		for _, c := range cases {
			in.WriteString(c.Input + "\n")
			want.WriteString(c.Output + "\n")
		}
		stdout, stderr, code := runBinary(t, bin, in.String())
		if code != 0 || stderr != "" {
			t.Fatalf("exit code %d, stderr %q", code, stderr)
		}
		gotLines := strings.Split(stdout, "\n")
		wantLines := strings.Split(want.String(), "\n")
		if len(gotLines) != len(wantLines) {
			t.Fatalf("got %d lines, want %d", len(gotLines), len(wantLines))
		}
		for i := range wantLines {
			if gotLines[i] != wantLines[i] {
				t.Errorf("line %d:\n got %s\nwant %s", i+1, gotLines[i], wantLines[i])
			}
		}
	})

	t.Run("CRLF, an invalid line and --json", func(t *testing.T) {
		stdout, stderr, code := runBinary(t, bin, cases[0].Input+"\r\nhttp://\r\n", "--json")
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr, "line 2: invalid URL") {
			t.Errorf("stderr = %q", stderr)
		}
		var doc struct {
			Results []struct {
				Output *string `json:"output"`
			} `json:"results"`
		}
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("invalid JSON: %v\n%s", err, stdout)
		}
		if len(doc.Results) != 2 || *doc.Results[0].Output != cases[0].Output || doc.Results[1].Output != nil {
			t.Errorf("unexpected results:\n%s", stdout)
		}
	})

	t.Run("version set at build time", func(t *testing.T) {
		stdout, _, code := runBinary(t, bin, "", "--version")
		if code != 0 || !strings.HasPrefix(stdout, "decrumb e2e-test\n") {
			t.Errorf("exit code %d, output:\n%s", code, stdout)
		}
	})

	t.Run("broken --rules file exits 3", func(t *testing.T) {
		_, stderr, code := runBinary(t, bin, "", "--rules", filepath.Join(t.TempDir(), "none.json"), "https://example.com/")
		if code != 3 || !strings.HasSuffix(stderr, "none.json: file not found\n") {
			t.Errorf("exit code %d, stderr %q", code, stderr)
		}
	})
}

// runBinary runs bin with stdin as a pipe carrying input.
func runBinary(t *testing.T, bin, input string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(input)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	case err != nil:
		t.Fatalf("running %s: %v", bin, err)
	}
	return out.String(), errOut.String(), code
}
