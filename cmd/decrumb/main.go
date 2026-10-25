// Command decrumb removes tracking parameters from URLs, fully offline.
//
// All the work happens in internal/cli; main only connects it to the real
// process (arguments, stdin, stdout, stderr and the exit code).
package main

import (
	"os"
	"regexp"
	"runtime/debug"

	"github.com/pinebench/decrumb/internal/cli"
)

// version is overridden at release time with -ldflags "-X main.version=...".
var version = "dev"

// pseudoVersion matches the ending shared by all three forms of a Go
// pseudo-version (v0.0.0-20261025093727-c252c0c87f27, v0.1.1-0.2026…-…,
// v0.2.0-rc.1.0.2026…-…): a 14-digit UTC timestamp and a 12-character
// commit, plus "+dirty" when the checkout had uncommitted changes.
var pseudoVersion = regexp.MustCompile(`[-.](\d{14})-([0-9a-f]{12})(\+dirty)?$`)

func main() {
	info, _ := debug.ReadBuildInfo()
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cli.Config{
		Version:         resolveVersion(version, info),
		StdinIsTerminal: stdinIsTerminal,
	}))
}

// resolveVersion returns the version --version shows (spec §14). A version
// injected with -ldflags wins. Otherwise (go install) it is the module
// version Go recorded in the binary, e.g. v0.1.0, unless that is missing,
// empty or "(devel)". A pseudo-version is shown as "dev (<commit>, <date>)".
// info is nil when the binary has no build info.
func resolveVersion(injected string, info *debug.BuildInfo) string {
	if injected != "dev" {
		return injected
	}
	if info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	v := info.Main.Version
	m := pseudoVersion.FindStringSubmatch(v)
	if m == nil {
		return v
	}
	timestamp, commit, dirty := m[1], m[2], m[3]
	date := timestamp[0:4] + "-" + timestamp[4:6] + "-" + timestamp[6:8]
	if dirty != "" {
		return "dev (" + commit[:7] + ", " + date + ", modified)"
	}
	return "dev (" + commit[:7] + ", " + date + ")"
}

// stdinIsTerminal reports whether stdin is an interactive terminal (a
// "character device") rather than a pipe or a file.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
