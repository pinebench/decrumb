// Package cli is decrumb's command-line interface (spec §5): flags,
// arguments or stdin, plain or JSON output, and exit codes. cmd/decrumb only
// connects Run to the real process; everything else lives here, so it can be
// tested without building a binary.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/pinebench/decrumb/internal/cleaner"
	"github.com/pinebench/decrumb/internal/rules"
	"github.com/pinebench/decrumb/internal/rules/clearurls"
)

// Exit codes (spec §5).
const (
	exitOK      = 0 // success
	exitInvalid = 1 // at least one invalid input line
	exitUsage   = 2 // unknown flag, missing flag value, or nothing to read
	exitRules   = 3 // the rules could not be loaded
)

const usage = `Usage: decrumb [flags] [url ...]

Removes tracking parameters from URLs, offline. URLs are read from the
arguments or, if there are none, from stdin, one per line.

Flags:
  --explain        show what was removed and why
  --json           output JSON instead of plain text
  --rules <file>   load rules from this file instead of the bundled ones
  --version        print version, bundled rules revision and attribution
  -h, --help       show this help

Example:
  $ decrumb 'https://example.com/page?utm_source=newsletter&id=7'
  https://example.com/page?id=7

Exit codes: 0 success, 1 invalid input line, 2 usage error,
3 rules could not be loaded.
`

// Config is what Run needs from the process around it.
type Config struct {
	// Version is printed by --version.
	Version string

	// StdinIsTerminal reports whether stdin is an interactive terminal
	// rather than a pipe or a file. With no URL arguments, Run then prints
	// the usage instead of waiting for input that won't come.
	StdinIsTerminal func() bool
}

// Run runs decrumb with the given arguments (without the program name) and
// returns the exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, cfg Config) int {
	fs := flag.NewFlagSet("decrumb", flag.ContinueOnError)
	fs.SetOutput(stderr) // where flag reports a bad flag
	fs.Usage = func() {} // we print the usage ourselves, below
	explainFlag := fs.Bool("explain", false, "")
	jsonFlag := fs.Bool("json", false, "")
	rulesFlag := fs.String("rules", "", "")
	versionFlag := fs.Bool("version", false, "")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) { // -h or --help
			io.WriteString(stdout, usage)
			return exitOK
		}
		// flag has already written the reason, e.g. "flag provided but
		// not defined: -x".
		io.WriteString(stderr, usage)
		return exitUsage
	}
	rulesSet := false
	fs.Visit(func(f *flag.Flag) { rulesSet = rulesSet || f.Name == "rules" })
	if rulesSet && *rulesFlag == "" {
		fmt.Fprintln(stderr, "decrumb: --rules needs a file name")
		io.WriteString(stderr, usage)
		return exitUsage
	}

	if *versionFlag {
		return printVersion(stdout, stderr, cfg.Version)
	}

	urls := fs.Args()
	if len(urls) == 0 && cfg.StdinIsTerminal != nil && cfg.StdinIsTerminal() {
		io.WriteString(stderr, usage)
		return exitUsage
	}

	r, err := loadRules(*rulesFlag)
	if err != nil {
		fmt.Fprintf(stderr, "decrumb: %v\n", err)
		return exitRules
	}
	c := cleaner.New(r)

	p := &printer{w: stdout, json: *jsonFlag, explain: *explainFlag}
	invalid := false
	report := func(where string, res cleaner.Result) {
		if res.Err != nil {
			invalid = true
			fmt.Fprintf(stderr, "decrumb: %s: %v\n", where, res.Err)
		}
		p.print(res)
	}

	p.start()
	defer p.end() // close the JSON document even if reading stdin fails
	if len(urls) > 0 {
		for i, u := range urls {
			report(fmt.Sprintf("argument %d", i+1), c.Clean(u))
		}
	} else if err := cleanLines(stdin, c, report); err != nil {
		fmt.Fprintf(stderr, "decrumb: reading stdin: %v\n", err)
		invalid = true
	}

	if invalid {
		return exitInvalid
	}
	return exitOK
}

// errTooLong is the error for an input line over maxLineLen.
var errTooLong = fmt.Errorf("line is longer than 2 MB (%d bytes)", maxLineLen)

// cleanLines cleans every line of r and passes each result to report.
// Empty and whitespace-only lines are skipped. A line that is too long is
// reported with errTooLong and an empty Input, so plain output gets an
// empty line for it.
func cleanLines(r io.Reader, c *cleaner.Cleaner, report func(string, cleaner.Result)) error {
	lr := newLineReader(r, maxLineLen)
	for n := 1; ; n++ {
		line, tooLong, err := lr.next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		where := fmt.Sprintf("line %d", n)
		if tooLong {
			report(where, cleaner.Result{Err: errTooLong})
			continue
		}
		s := string(line)
		if strings.TrimSpace(s) == "" {
			continue
		}
		report(where, c.Clean(s))
	}
}

// loadRules loads the rules in path, or the bundled rules (with our
// overrides) if path is "".
func loadRules(path string) (*rules.Rules, error) {
	if path == "" {
		r, err := rules.LoadBundled()
		if err != nil {
			return nil, fmt.Errorf("loading the bundled rules: %w", err)
		}
		return r, nil
	}
	return rules.LoadFile(path) // its errors already name the file
}

// printVersion writes the --version text, including the attribution that
// the rules' license requires (spec §8).
func printVersion(stdout, stderr io.Writer, version string) int {
	src, err := clearurls.BundledSource()
	if err != nil {
		fmt.Fprintf(stderr, "decrumb: %v\n", err)
		return exitRules
	}
	overrides, err := rules.OverrideRuleCount()
	if err != nil {
		fmt.Fprintf(stderr, "decrumb: %v\n", err)
		return exitRules
	}
	fmt.Fprintf(stdout, `decrumb %s
rules:     ClearURLs rules, commit %s
overrides: %d rules added by decrumb (MIT)

decrumb uses the ClearURLs rules (%s)
by the ClearURLs project and contributors. The rules are licensed under
LGPL-3.0 and bundled unmodified. See NOTICE, COPYING.LESSER and COPYING,
included with the source code and every release.
`, version, src.Commit, overrides, src.Repository)
	return exitOK
}
