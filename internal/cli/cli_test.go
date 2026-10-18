package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pinebench/decrumb/internal/explain"
	"github.com/pinebench/decrumb/internal/rules"
	"github.com/pinebench/decrumb/internal/rules/clearurls"
)

// fixture is a small rules file, so tests don't depend on upstream rules:
// globalRules removes utm_source and fbclid; "site" (site.example) removes
// sid and has a raw rule for /ref=...; "go" (go.example) is a redirect,
// /?u=<target>.
const fixture = "testdata/rules.json"

type output struct {
	code           int
	stdout, stderr string
}

// run calls Run with stdin as a pipe (not a terminal) and the given input.
func run(t *testing.T, stdin string, args ...string) output {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, strings.NewReader(stdin), &stdout, &stderr, Config{
		Version:         "1.2.3",
		StdinIsTerminal: func() bool { return false },
	})
	return output{code, stdout.String(), stderr.String()}
}

func describeOf(t *testing.T, name string) string {
	t.Helper()
	e, ok := explain.Describe(name, "globalRules")
	if !ok {
		t.Fatalf("no description for %q", name)
	}
	return e.Description
}

func TestPlain(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdin    string
		wantOut  string
		wantErr  string // substring of stderr; "" means stderr must be empty
		wantCode int
	}{
		{
			name:    "one argument",
			args:    []string{"https://example.com/?utm_source=x&id=7"},
			wantOut: "https://example.com/?id=7\n",
		},
		{
			name:    "several arguments, in order",
			args:    []string{"https://a.example/?fbclid=1", "b.example/x", "https://c.example/?sid=2"},
			wantOut: "https://a.example/\nhttps://b.example/x\nhttps://c.example/?sid=2\n",
		},
		{
			name:    "arguments take priority over stdin",
			args:    []string{"https://a.example/"},
			stdin:   "https://ignored.example/\n",
			wantOut: "https://a.example/\n",
		},
		{
			name:    "stdin, one URL per line",
			stdin:   "https://a.example/?utm_source=x\nhttps://b.example/?id=1&fbclid=2\n",
			wantOut: "https://a.example/\nhttps://b.example/?id=1\n",
		},
		{
			name:    "stdin without a final newline",
			stdin:   "https://a.example/\nhttps://b.example/?utm_source=x",
			wantOut: "https://a.example/\nhttps://b.example/\n",
		},
		{
			name:    "empty and whitespace-only lines are skipped",
			stdin:   "\n   \nhttps://a.example/\n\t\n\nhttps://b.example/\n \u00a0 \n",
			wantOut: "https://a.example/\nhttps://b.example/\n",
		},
		{
			name:    "surrounding whitespace is trimmed",
			stdin:   "   https://a.example/?utm_source=x  \t\n",
			wantOut: "https://a.example/\n",
		},
		{
			name:    "CRLF line endings",
			stdin:   "https://a.example/?id=1&utm_source=x\r\nhttps://b.example/\r\n\r\nhttps://c.example/?fbclid=1\r\n",
			wantOut: "https://a.example/?id=1\nhttps://b.example/\nhttps://c.example/\n",
		},
		{
			name:    "CRLF without a final line ending",
			stdin:   "https://a.example/\r\nhttps://b.example/?fbclid=1",
			wantOut: "https://a.example/\nhttps://b.example/\n",
		},
		{
			name:    "a site rule applies only on its site",
			stdin:   "https://www.site.example/?sid=1&id=2\nhttps://other.example/?sid=1\n",
			wantOut: "https://www.site.example/?id=2\nhttps://other.example/?sid=1\n",
		},
		{
			name:    "redirect is unwrapped",
			args:    []string{"https://go.example/?u=https%3A%2F%2Fwww.site.example%2Fa%3Futm_source%3Dx"},
			wantOut: "https://www.site.example/a\n",
		},
		{
			name:    "non-http input is passed through unchanged",
			stdin:   " mailto:someone@example.com?utm_source=x \n",
			wantOut: "mailto:someone@example.com?utm_source=x\n",
		},
		{
			name:     "invalid line is passed through, trimmed, and reported",
			stdin:    "https://a.example/?utm_source=x\n  http://  \nhttps://b.example/\n",
			wantOut:  "https://a.example/\nhttp://\nhttps://b.example/\n",
			wantErr:  `decrumb: line 2: invalid URL "  http://  ": no host`,
			wantCode: 1,
		},
		{
			name:     "line numbers count skipped lines",
			stdin:    "\n\nhttps://exa mple.com/\n",
			wantOut:  "https://exa mple.com/\n",
			wantErr:  "decrumb: line 3: invalid URL",
			wantCode: 1,
		},
		{
			name:     "invalid argument names the argument",
			args:     []string{"https://a.example/", "https://"},
			wantOut:  "https://a.example/\nhttps://\n",
			wantErr:  `decrumb: argument 2: invalid URL "https://": no host`,
			wantCode: 1,
		},
		{
			name:    "empty stdin gives no output",
			stdin:   "",
			wantOut: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, tt.stdin, append([]string{"--rules", fixture}, tt.args...)...)
			if got.stdout != tt.wantOut {
				t.Errorf("stdout:\n got %q\nwant %q", got.stdout, tt.wantOut)
			}
			if tt.wantErr == "" && got.stderr != "" {
				t.Errorf("stderr = %q, want nothing", got.stderr)
			}
			if !strings.Contains(got.stderr, tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", got.stderr, tt.wantErr)
			}
			if got.code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", got.code, tt.wantCode)
			}
		})
	}
}

// TestBundledRules runs a few cases with the bundled rules and overrides.
func TestBundledRules(t *testing.T) {
	got := run(t, "https://www.amazon.com/dp/B000000000?tag=affiliate-20&th=1&psc=1\n"+
		"https://shop.example.com/?gbraid=0AAA&color=blue\n")
	want := "https://www.amazon.com/dp/B000000000?psc=1\nhttps://shop.example.com/?color=blue\n"
	if got.stdout != want || got.code != 0 || got.stderr != "" {
		t.Errorf("got %+v, want stdout %q, exit 0", got, want)
	}
}

// TestBOM checks that a UTF-8 byte order mark is dropped at the very start
// of stdin only (spec §5).
func TestBOM(t *testing.T) {
	const bom = "\xEF\xBB\xBF"
	tests := []struct {
		name    string
		stdin   string
		wantOut string
		wantErr string // substring of stderr; "" means stderr must be empty
	}{
		{"BOM and URL on line 1", bom + "https://a.example/?utm_source=x\nhttps://b.example/\n", "https://a.example/\nhttps://b.example/\n", ""},
		{"BOM with CRLF", bom + "https://a.example/?utm_source=x\r\n", "https://a.example/\n", ""},
		{"BOM only", bom, "", ""},
		{"BOM and a newline only", bom + "\n", "", ""},
		{"BOM then an empty first line", bom + "\nhttps://a.example/\n", "https://a.example/\n", ""},
		// A BOM anywhere else is left alone: it's part of the text, so the
		// cleaner sees no scheme and adds https:// in front of it.
		{
			name:    "BOM inside a later line is left alone",
			stdin:   "https://a.example/\n" + bom + "https://b.example/\n",
			wantOut: "https://a.example/\nhttps://" + bom + "https://b.example/\n",
		},
		{
			name:    "only one BOM is dropped",
			stdin:   bom + bom + "https://a.example/\n",
			wantOut: "https://" + bom + "https://a.example/\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, tt.stdin, "--rules", fixture)
			if got.stdout != tt.wantOut {
				t.Errorf("stdout = %q, want %q", got.stdout, tt.wantOut)
			}
			if tt.wantErr == "" && got.stderr != "" {
				t.Errorf("stderr = %q, want nothing", got.stderr)
			}
			if !strings.Contains(got.stderr, tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", got.stderr, tt.wantErr)
			}
		})
	}
}

// TestFlagsAnywhere checks that flags may come before, between or after
// URLs, and that "--" ends the flags (spec §5).
func TestFlagsAnywhere(t *testing.T) {
	const a, b = "https://a.example/?utm_source=x", "https://b.example/?fbclid=y"
	jsonOf := func(urls ...string) string {
		return run(t, "", append([]string{"--json", "--rules", fixture}, urls...)...).stdout
	}
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
	}{
		{"flags before URLs", []string{"--rules", fixture, "--json", a, b}, jsonOf(a, b), exitOK},
		{"flags after URLs", []string{a, b, "--json", "--rules", fixture}, jsonOf(a, b), exitOK},
		{"flags between URLs", []string{a, "--json", b, "--rules", fixture}, jsonOf(a, b), exitOK},
		{"single-dash flag after a URL", []string{a, "-json", "-rules", fixture}, jsonOf(a), exitOK},
		{"--rules=file after a URL", []string{a, "--rules=" + fixture}, "https://a.example/\n", exitOK},
		{
			name:     "-- ends the flags: what follows is a URL even if it looks like a flag",
			args:     []string{"--rules", fixture, a, "--", "--json", "-x"},
			wantOut:  "https://a.example/\nhttps://--json\nhttps://-x\n",
			wantCode: exitOK,
		},
		{
			name:     "a second -- is a URL",
			args:     []string{"--rules", fixture, "--", "--", a},
			wantOut:  "https://--\nhttps://a.example/\n",
			wantCode: exitOK,
		},
		{
			name:     "-- as the last argument",
			args:     []string{"--rules", fixture, a, "--"},
			wantOut:  "https://a.example/\n",
			wantCode: exitOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "", tt.args...)
			if got.stdout != tt.wantOut || got.code != tt.wantCode {
				t.Errorf("got exit %d, stdout:\n%s\nwant exit %d, stdout:\n%s\nstderr: %s", got.code, got.stdout, tt.wantCode, tt.wantOut, got.stderr)
			}
		})
	}

	t.Run("--version after a URL", func(t *testing.T) {
		got := run(t, "", a, "--version")
		if !strings.HasPrefix(got.stdout, "decrumb 1.2.3\n") || got.code != exitOK {
			t.Errorf("got %+v, want the version", got)
		}
	})
}

// TestFlagsAnywhereUsageErrors checks that a bad flag after a URL is still
// a usage error, and that "--" as the value of --rules isn't the end of
// the flags.
func TestFlagsAnywhereUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
		want    int
	}{
		{"unknown flag after a URL", []string{"https://a.example/", "--nope"}, "flag provided but not defined: -nope\n", exitUsage},
		{"unknown flag between URLs", []string{"https://a.example/", "-x", "https://b.example/"}, "flag provided but not defined: -x\n", exitUsage},
		{"missing value at the end", []string{"https://a.example/", "--rules"}, "flag needs an argument: -rules\n", exitUsage},
		{"bad bool value after a URL", []string{"https://a.example/", "--json=maybe"}, "invalid boolean value", exitUsage},
		{"-- as the value of --rules", []string{"--rules", "--", "https://a.example/"}, "decrumb: --: file not found\n", exitRules},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "", tt.args...)
			if got.code != tt.want {
				t.Errorf("exit code = %d, want %d", got.code, tt.want)
			}
			if !strings.Contains(got.stderr, tt.wantErr) {
				t.Errorf("stderr = %q, want %q", got.stderr, tt.wantErr)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want nothing", got.stdout)
			}
		})
	}
}

func TestExitCodes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.json")
	tests := []struct {
		name string
		args []string
		in   string
		want int
	}{
		{"success", []string{"https://a.example/"}, "", exitOK},
		{"success from stdin", nil, "https://a.example/\n", exitOK},
		{"non-http input is not an error", []string{"mailto:a@example.com"}, "", exitOK},
		{"invalid input line", nil, "https://a.example/\nhttp://\n", exitInvalid},
		{"too long line", nil, strings.Repeat("a", maxLineLen+1) + "\n", exitInvalid},
		{"unknown flag", []string{"--nope"}, "", exitUsage},
		{"unknown short flag", []string{"-x", "https://a.example/"}, "", exitUsage},
		{"missing flag value", []string{"--rules"}, "", exitUsage},
		{"empty --rules value", []string{"--rules="}, "", exitUsage},
		{"bool flag with a bad value", []string{"--json=maybe"}, "", exitUsage},
		{"rules file can't be loaded", []string{"--rules", missing, "https://a.example/"}, "", exitRules},
		{"help", []string{"--help"}, "", exitOK},
		{"short help", []string{"-h"}, "", exitOK},
		{"version", []string{"--version"}, "", exitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(t, tt.in, tt.args...); got.code != tt.want {
				t.Errorf("exit code = %d, want %d (stderr %q)", got.code, tt.want, got.stderr)
			}
		})
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"unknown flag", []string{"--nope"}, "flag provided but not defined: -nope\n"},
		{"missing flag value", []string{"--rules"}, "flag needs an argument: -rules\n"},
		{"empty --rules value", []string{"--rules", ""}, "decrumb: --rules needs a file name\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "https://a.example/\n", tt.args...)
			if got.code != exitUsage {
				t.Errorf("exit code = %d, want %d", got.code, exitUsage)
			}
			if want := tt.wantErr + usage; got.stderr != want {
				t.Errorf("stderr:\n got %q\nwant %q", got.stderr, want)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want nothing", got.stdout)
			}
		})
	}
}

func TestHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "-help"} {
		t.Run(flag, func(t *testing.T) {
			got := run(t, "", flag)
			if got.stdout != usage || got.stderr != "" || got.code != exitOK {
				t.Fatalf("got %+v, want usage on stdout and exit 0", got)
			}
		})
	}
	for _, want := range []string{"--explain", "--json", "--rules <file>", "--version", "-h, --help", "Example:", "$ decrumb '"} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage does not mention %q", want)
		}
	}
}

func TestInteractiveTerminal(t *testing.T) {
	cfg := Config{StdinIsTerminal: func() bool { return true }}

	t.Run("no arguments: usage, exit 2, stdin not read", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		stdin := &failingReader{t: t}
		code := Run(nil, stdin, &stdout, &stderr, cfg)
		if code != exitUsage || stderr.String() != usage || stdout.Len() != 0 {
			t.Errorf("got exit %d, stdout %q, stderr %q; want exit 2 and usage on stderr", code, stdout.String(), stderr.String())
		}
	})

	t.Run("with arguments: cleans them", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"https://a.example/?utm_source=x"}, &failingReader{t: t}, &stdout, &stderr, cfg)
		if code != exitOK || stdout.String() != "https://a.example/\n" {
			t.Errorf("got exit %d, stdout %q", code, stdout.String())
		}
	})
}

// failingReader fails the test if it is read.
type failingReader struct{ t *testing.T }

func (r *failingReader) Read([]byte) (int, error) {
	r.t.Error("stdin was read")
	return 0, io.EOF
}

func TestVersion(t *testing.T) {
	got := run(t, "", "--version", "https://ignored.example/")
	if got.code != exitOK || got.stderr != "" {
		t.Fatalf("got exit %d, stderr %q", got.code, got.stderr)
	}
	src, err := clearurls.BundledSource()
	if err != nil {
		t.Fatal(err)
	}
	n, err := rules.OverrideRuleCount()
	if err != nil {
		t.Fatal(err)
	}
	wants := []string{
		"decrumb 1.2.3\n",
		"commit " + src.Commit,
		fmt.Sprintf("%d rules added by decrumb", n),
		"ClearURLs rules (https://github.com/ClearURLs/Rules)",
		"by the ClearURLs project and contributors",
		"LGPL-3.0",
		"NOTICE, COPYING.LESSER and COPYING",
	}
	for _, want := range wants {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("--version output does not contain %q:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(strings.ToLower(got.stdout), "endorse") {
		t.Errorf("--version output mentions endorsement:\n%s", got.stdout)
	}
}

func TestExplain(t *testing.T) {
	utm, fbclid := describeOf(t, "utm_source"), describeOf(t, "fbclid")
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "parameters, raw rule and fragment",
			in:   "https://www.site.example/item/ref=abc?sid=1&utm_source=mail&id=7#fbclid=x&y=2",
			want: "https://www.site.example/item?id=7#y=2\n" +
				"  removed    /ref=abc    site         (raw rule)\n" +
				"  removed    sid         site\n" +
				"  removed    utm_source  globalRules  " + utm + "\n" +
				"  removed    fbclid      globalRules  (in fragment) " + fbclid + "\n",
		},
		{
			name: "unwrap hop",
			in:   "https://go.example/?u=https%3A%2F%2Fwww.site.example%2Fa%3Futm_source%3Dx",
			want: "https://www.site.example/a\n" +
				"  unwrapped  https://www.site.example/a?utm_source=x\n" +
				"  removed    utm_source  globalRules  " + utm + "\n",
		},
		{
			name: "hop limit",
			in:   nestedRedirect(7),
			want: nestedRedirect(2) + "\n" +
				"  unwrapped  " + nestedRedirect(6) + "\n" +
				"  unwrapped  " + nestedRedirect(5) + "\n" +
				"  unwrapped  " + nestedRedirect(4) + "\n" +
				"  unwrapped  " + nestedRedirect(3) + "\n" +
				"  unwrapped  " + nestedRedirect(2) + "\n" +
				"  note       stopped after 5 redirect hops; the last URL reached was cleaned\n",
		},
		{
			name: "nothing removed",
			in:   "https://a.example/?id=1",
			want: "https://a.example/?id=1\n",
		},
		{
			name: "invalid input: just the line",
			in:   "http://",
			want: "http://\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, tt.in+"\n", "--explain", "--rules", fixture)
			if got.stdout != tt.want {
				t.Errorf("stdout:\n got:\n%s\nwant:\n%s", got.stdout, tt.want)
			}
		})
	}
}

// nestedRedirect returns a go.example redirect wrapped n times around
// https://end.example/?id=1 (nestedRedirect(0) is that URL itself).
func nestedRedirect(n int) string {
	s := "https://end.example/?id=1"
	for range n {
		s = "https://go.example/?u=" + s
	}
	return s
}

func TestExplainBundledAmazon(t *testing.T) {
	got := run(t, "", "--explain", "https://www.amazon.com/Some-Book/dp/B000000000/ref=sr_1_1?qid=1700000000&tag=affiliate-20")
	tag, ok := explain.Describe("tag", "amazon")
	if !ok {
		t.Fatal("no description for tag on amazon")
	}
	want := "https://www.amazon.com/Some-Book/dp/B000000000\n" +
		"  removed    /ref=sr_1_1  amazon  (raw rule)\n"
	if !strings.HasPrefix(got.stdout, want) {
		t.Errorf("stdout:\n%s\nwant it to start with:\n%s", got.stdout, want)
	}
	if !strings.Contains(got.stdout, "  removed    tag          amazon  "+tag.Description+"\n") {
		t.Errorf("stdout does not describe tag:\n%s", got.stdout)
	}
}

// jsonDoc is the --json document, decoded strictly enough to notice a
// missing field: pointers tell null from absent.
type jsonDoc struct {
	Results []struct {
		Input           *string  `json:"input"`
		Output          *string  `json:"output"`
		Changed         *bool    `json:"changed"`
		Unwrapped       []string `json:"unwrapped"`
		Removed         []map[string]string
		HopLimitReached *bool   `json:"hop_limit_reached"`
		Error           *string `json:"error"`
	} `json:"results"`
}

func parseJSON(t *testing.T, s string) jsonDoc {
	t.Helper()
	var doc jsonDoc
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("output is not the expected JSON: %v\n%s", err, s)
	}
	if dec.More() {
		t.Fatalf("more than one JSON document:\n%s", s)
	}
	return doc
}

func TestJSON(t *testing.T) {
	in := strings.Join([]string{
		"https://www.site.example/item/ref=abc?sid=1&utm_source=mail&id=7#fbclid=x",
		"example.com/plain",
		"http://",
		"https://go.example/?u=https%3A%2F%2Fwww.site.example%2Fa%3Futm_source%3Dx",
		nestedRedirect(7),
	}, "\n")
	got := run(t, in, "--json", "--rules", fixture)
	if got.code != exitInvalid {
		t.Errorf("exit code = %d, want %d", got.code, exitInvalid)
	}
	if !strings.Contains(got.stderr, "decrumb: line 3: invalid URL") {
		t.Errorf("stderr = %q, want the line 3 error", got.stderr)
	}
	if !strings.HasPrefix(got.stdout, "{\"results\":[\n") || !strings.HasSuffix(got.stdout, "\n]}\n") {
		t.Errorf("document is not framed as {\"results\":[ ... ]}:\n%s", got.stdout)
	}
	if strings.Contains(got.stdout, `\u0026`) {
		t.Error("& in URLs is escaped as \\u0026")
	}
	doc := parseJSON(t, got.stdout)
	if len(doc.Results) != 5 {
		t.Fatalf("got %d results, want 5", len(doc.Results))
	}

	// Every field is present in every result.
	for i, r := range doc.Results {
		if r.Input == nil || r.Changed == nil || r.HopLimitReached == nil || r.Unwrapped == nil || r.Removed == nil {
			t.Errorf("result %d is missing a field: %+v", i, r)
		}
	}

	r := doc.Results[0]
	if *r.Output != "https://www.site.example/item?id=7" || !*r.Changed || r.Error != nil {
		t.Errorf("result 0: output %q, changed %v, error %v", *r.Output, *r.Changed, r.Error)
	}
	wantRemoved := []map[string]string{
		{"param": "/ref=abc", "provider": "site", "location": "rawRule"},
		{"param": "sid", "provider": "site", "location": "query"},
		{"param": "utm_source", "provider": "globalRules", "location": "query", "description": describeOf(t, "utm_source")},
		{"param": "fbclid", "provider": "globalRules", "location": "fragment", "description": describeOf(t, "fbclid")},
	}
	if fmt.Sprint(r.Removed) != fmt.Sprint(wantRemoved) {
		t.Errorf("result 0 removed:\n got %v\nwant %v", r.Removed, wantRemoved)
	}

	r = doc.Results[1]
	if *r.Output != "https://example.com/plain" || *r.Changed {
		t.Errorf("result 1: adding https:// is normalization: output %q, changed %v", *r.Output, *r.Changed)
	}
	if len(r.Unwrapped) != 0 || len(r.Removed) != 0 {
		t.Errorf("result 1: unwrapped %v, removed %v, want empty lists", r.Unwrapped, r.Removed)
	}

	r = doc.Results[2]
	if r.Output != nil || r.Error == nil || *r.Error != `invalid URL "http://": no host` || *r.Input != "http://" {
		t.Errorf("result 2: invalid input should have output null and error set: %+v", r)
	}

	r = doc.Results[3]
	if len(r.Unwrapped) != 1 || r.Unwrapped[0] != "https://www.site.example/a?utm_source=x" || *r.HopLimitReached {
		t.Errorf("result 3: unwrapped %v, hop limit %v", r.Unwrapped, *r.HopLimitReached)
	}

	r = doc.Results[4]
	if !*r.HopLimitReached || len(r.Unwrapped) != 5 || *r.Output != nestedRedirect(2) {
		t.Errorf("result 4: hop limit %v, %d hops, output %q", *r.HopLimitReached, len(r.Unwrapped), *r.Output)
	}

	// --explain doesn't change --json output.
	if again := run(t, in, "--json", "--explain", "--rules", fixture); again.stdout != got.stdout {
		t.Errorf("--json --explain differs from --json:\n%s", again.stdout)
	}
}

func TestJSONEmpty(t *testing.T) {
	got := run(t, "\n\n", "--json")
	if got.stdout != "{\"results\":[\n]}\n" {
		t.Errorf("stdout = %q", got.stdout)
	}
	if doc := parseJSON(t, got.stdout); len(doc.Results) != 0 {
		t.Errorf("got %d results, want 0", len(doc.Results))
	}
}

// recordingWriter keeps every Write separately. It is safe to use from
// two goroutines.
type recordingWriter struct {
	mu     sync.Mutex
	writes []string
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes = append(w.writes, string(p))
	return len(p), nil
}

func (w *recordingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.Join(w.writes, "")
}

// TestJSONIsStreamed checks that each result is written as soon as it is
// cleaned, not all together at the end (spec §5).
func TestJSONIsStreamed(t *testing.T) {
	t.Run("one write per result, in order", func(t *testing.T) {
		const n = 500
		var in strings.Builder
		for i := range n {
			fmt.Fprintf(&in, "https://example.com/%d?utm_source=x\n", i)
		}
		var stdout recordingWriter
		code := Run([]string{"--json", "--rules", fixture}, strings.NewReader(in.String()), &stdout, io.Discard, Config{})
		if code != exitOK {
			t.Fatalf("exit code = %d", code)
		}
		// The opening, one write per result, and the closing.
		if len(stdout.writes) != n+2 {
			t.Fatalf("got %d writes, want %d", len(stdout.writes), n+2)
		}
		for i := range n {
			if want := fmt.Sprintf(`"output":"https://example.com/%d"`, i); !strings.Contains(stdout.writes[i+1], want) {
				t.Fatalf("write %d = %q, want result %d", i+1, stdout.writes[i+1], i)
			}
		}
		parseJSON(t, stdout.String())
	})

	t.Run("a result appears before the input ends", func(t *testing.T) {
		stdinR, stdinW := io.Pipe()
		var stdout recordingWriter
		done := make(chan int)
		go func() {
			done <- Run([]string{"--json", "--rules", fixture}, stdinR, &stdout, io.Discard, Config{})
		}()

		io.WriteString(stdinW, "https://a.example/?utm_source=x\n")
		waitFor(t, &stdout, `"output":"https://a.example/"`)

		io.WriteString(stdinW, "https://b.example/?fbclid=y\n")
		waitFor(t, &stdout, `"output":"https://b.example/"`)

		stdinW.Close()
		if code := <-done; code != exitOK {
			t.Errorf("exit code = %d", code)
		}
		if doc := parseJSON(t, stdout.String()); len(doc.Results) != 2 {
			t.Errorf("got %d results, want 2", len(doc.Results))
		}
	})
}

// waitFor waits until w's output contains want, or fails after a while.
func waitFor(t *testing.T, w *recordingWriter, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(w.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("output never contained %q; got:\n%s", want, w.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestLineLengthLimit checks the 2 MB line limit (spec §5).
func TestLineLengthLimit(t *testing.T) {
	// longURL returns a URL of exactly size bytes that ends in a tracking
	// parameter, so a cleaned result is recognizable.
	longURL := func(size int) string {
		const head, tail = "https://example.com/?a=", "&utm_source=x"
		return head + strings.Repeat("b", size-len(head)-len(tail)) + tail
	}
	cleaned := func(size int) string {
		return strings.TrimSuffix(longURL(size), "&utm_source=x")
	}

	t.Run("a line of exactly 2 MB is cleaned", func(t *testing.T) {
		got := run(t, longURL(maxLineLen)+"\nhttps://next.example/\n", "--rules", fixture)
		want := cleaned(maxLineLen) + "\nhttps://next.example/\n"
		if got.stdout != want || got.code != exitOK || got.stderr != "" {
			t.Errorf("exit %d, stderr %q, stdout matches: %v", got.code, got.stderr, got.stdout == want)
		}
	})

	t.Run("2 MB plus CRLF is still accepted", func(t *testing.T) {
		got := run(t, longURL(maxLineLen)+"\r\n", "--rules", fixture)
		if got.stdout != cleaned(maxLineLen)+"\n" || got.code != exitOK {
			t.Errorf("exit %d, stderr %q", got.code, got.stderr)
		}
	})

	for _, ending := range []string{"\n", "\r\n", ""} {
		t.Run(fmt.Sprintf("one byte over is rejected, ending %q", ending), func(t *testing.T) {
			in := "https://first.example/?utm_source=x\n" + longURL(maxLineLen+1) + ending
			if ending != "" {
				in += "https://next.example/?fbclid=y\n"
			}
			got := run(t, in, "--rules", fixture)

			want := "https://first.example/\n\n"
			if ending != "" {
				want += "https://next.example/\n"
			}
			if got.stdout != want {
				t.Errorf("stdout = %.200q, want %q", got.stdout, want)
			}
			if wantErr := "decrumb: line 2: line is longer than 2 MB (2097152 bytes)\n"; got.stderr != wantErr {
				t.Errorf("stderr = %.200q, want %q", got.stderr, wantErr)
			}
			if got.code != exitInvalid {
				t.Errorf("exit code = %d, want %d", got.code, exitInvalid)
			}
		})
	}

	t.Run("in JSON", func(t *testing.T) {
		got := run(t, longURL(maxLineLen+1)+"\n", "--json", "--rules", fixture)
		doc := parseJSON(t, got.stdout)
		if len(doc.Results) != 1 {
			t.Fatalf("got %d results", len(doc.Results))
		}
		r := doc.Results[0]
		if r.Output != nil || r.Error == nil || *r.Error != errTooLong.Error() || *r.Input != "" {
			t.Errorf("got %+v", r)
		}
	})
}

func TestLineReader(t *testing.T) {
	const max = 5
	tests := []struct {
		name string
		in   string
		want []string // "!" marks a line that was too long
	}{
		{"LF", "a\nbb\n", []string{"a", "bb"}},
		{"CRLF", "a\r\nbb\r\n", []string{"a", "bb"}},
		{"no final newline", "a\nbb", []string{"a", "bb"}},
		{"empty lines are returned", "\n\na\n", []string{"", "", "a"}},
		{"lone CR inside a line is kept", "a\rb\n", []string{"a\rb"}},
		{"exactly max", "12345\n", []string{"12345"}},
		{"exactly max with CRLF", "12345\r\n", []string{"12345"}},
		{"one over", "123456\nok\n", []string{"!", "ok"}},
		{"one over with CRLF", "123456\r\nok\n", []string{"!", "ok"}},
		{"far over", strings.Repeat("x", 1000) + "\nok", []string{"!", "ok"}},
		{"over at the end", "ok\n123456", []string{"ok", "!"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr := newLineReader(strings.NewReader(tt.in), max)
			var got []string
			for {
				line, tooLong, err := lr.next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if tooLong {
					got = append(got, "!")
				} else {
					got = append(got, string(line))
				}
			}
			if fmt.Sprintf("%q", got) != fmt.Sprintf("%q", tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRulesFile checks --rules with a good file and with broken ones: the
// message must be readable without knowing Go (spec §6, "Loading behavior").
func TestRulesFile(t *testing.T) {
	t.Run("valid file, used alone without overrides", func(t *testing.T) {
		// gbraid is removed only by our overrides, which --rules disables.
		got := run(t, "", "--rules", fixture, "https://a.example/?gbraid=1&utm_source=x")
		if got.stdout != "https://a.example/?gbraid=1\n" || got.code != exitOK {
			t.Errorf("got %+v", got)
		}
		bundled := run(t, "", "https://a.example/?gbraid=1&utm_source=x")
		if bundled.stdout != "https://a.example/\n" {
			t.Errorf("bundled rules: got %q", bundled.stdout)
		}
	})

	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	missing := filepath.Join(dir, "missing.json")
	malformed := write("malformed.json", "{\"providers\": {\n  \"p\": {\"urlPattern\": \"a\",}\n}}")
	wrongType := write("wrongtype.json", `{"providers": {"p": {"urlPattern": "a", "rules": "utm_source"}}}`)
	badRegex := write("badregex.json", `{"providers": {"p": {"urlPattern": "a", "rules": ["(unclosed"]}}}`)

	tests := []struct {
		name string
		path string
		want string
	}{
		{"missing file", missing, missing + ": file not found"},
		{"malformed JSON", malformed, malformed + ": invalid JSON at line 2, column 27: invalid character '}' looking for beginning of object key string"},
		{"wrong type", wrongType, wrongType + `: provider "p": "rules" must be a list of strings`},
		{"bad regex", badRegex, badRegex + `: provider "p": rules[0]: invalid regex "(unclosed": error parsing regexp: missing closing ): ` + "`(?i)^(?:(unclosed)$`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "https://a.example/\n", "--rules", tt.path)
			if got.code != exitRules {
				t.Errorf("exit code = %d, want %d", got.code, exitRules)
			}
			if want := "decrumb: " + tt.want + "\n"; got.stderr != want {
				t.Errorf("stderr:\n got %s\nwant %s", got.stderr, want)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want nothing (nothing is cleaned)", got.stdout)
			}
		})
	}
}
