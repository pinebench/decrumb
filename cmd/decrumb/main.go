// Command decrumb removes tracking parameters from URLs, fully offline.
//
// All the work happens in internal/cli; main only connects it to the real
// process (arguments, stdin, stdout, stderr and the exit code).
package main

import (
	"os"

	"github.com/pinebench/decrumb/internal/cli"
)

// version is overridden at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cli.Config{
		Version:         version,
		StdinIsTerminal: stdinIsTerminal,
	}))
}

// stdinIsTerminal reports whether stdin is an interactive terminal (a
// "character device") rather than a pipe or a file.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
