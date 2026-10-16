// Package clearurls holds the bundled ClearURLs rules file (LGPL-3.0),
// unmodified, and the SOURCE lock file that records where it came from.
//
// Both files are written only by scripts/update-rules.
package clearurls

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"strings"
)

// Data is the bundled data.min.json, byte-for-byte as published upstream.
//
//go:embed data.min.json
var Data []byte

//go:embed SOURCE
var sourceFile []byte

// Source is the parsed SOURCE lock file.
type Source struct {
	Repository string // upstream repository URL
	Commit     string // upstream commit SHA the file was downloaded from
	File       string // name of the rules file, relative to this directory
	Downloaded string // download date, YYYY-MM-DD
	SHA256     string // hex SHA-256 of the rules file
}

// BundledSource returns the parsed SOURCE lock file of the bundled rules.
func BundledSource() (Source, error) {
	return ParseSource(sourceFile)
}

// ParseSource parses a SOURCE lock file made of "key: value" lines.
// Every field must be present and non-empty.
func ParseSource(b []byte) (Source, error) {
	fields := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if ok {
			fields[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return Source{}, fmt.Errorf("reading SOURCE: %w", err)
	}

	for _, key := range []string{"repository", "commit", "file", "downloaded", "sha256"} {
		if fields[key] == "" {
			return Source{}, fmt.Errorf("SOURCE is missing %q", key)
		}
	}

	return Source{
		Repository: fields["repository"],
		Commit:     fields["commit"],
		File:       fields["file"],
		Downloaded: fields["downloaded"],
		SHA256:     fields["sha256"],
	}, nil
}
