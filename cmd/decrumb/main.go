// Command decrumb removes tracking parameters from URLs, fully offline.
//
// This is a placeholder: for now it only supports --version.
package main

import (
	"flag"
	"fmt"
	"os"
)

// version is overridden at release time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("decrumb %s\n", version)
		return
	}

	fmt.Fprintln(os.Stderr, "decrumb: not implemented yet (only --version works)")
	os.Exit(1)
}
