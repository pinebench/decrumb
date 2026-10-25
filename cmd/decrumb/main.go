// Command decrumb removes tracking parameters from URLs, fully offline.
//
// All the work happens in internal/cli; main only connects it to the real
// process (arguments, stdin, stdout, stderr and the exit code).
package main

import (
	"os"
	"runtime/debug"

	"github.com/pinebench/decrumb/internal/cli"
)

// version is overridden at release time with -ldflags "-X main.version=...".
var version = "dev"

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
// empty or "(devel)". info is nil when the binary has no build info.
func resolveVersion(injected string, info *debug.BuildInfo) string {
	if injected != "dev" {
		return injected
	}
	if info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}

// stdinIsTerminal reports whether stdin is an interactive terminal (a
// "character device") rather than a pipe or a file.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
