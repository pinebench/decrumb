// Command update-rules downloads the ClearURLs rules file from one exact
// upstream commit and records where it came from in a SOURCE lock file.
//
// Usage:
//
//	go run ./scripts/update-rules --commit <40-char commit SHA>
//
// It must be run from the repository root. This is a dev-time tool: it uses
// the network and is never part of the decrumb binary.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	upstreamRepo = "https://github.com/ClearURLs/Rules"
	rawURLFormat = "https://raw.githubusercontent.com/ClearURLs/Rules/%s/data.min.json"
	outDir       = "internal/rules/clearurls"
	dataFile     = "data.min.json"
	sourceFile   = "SOURCE"

	// The rules file is about 200 KB; anything far larger is not what we expect.
	maxSize = 10 << 20
)

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func main() {
	commit := flag.String("commit", "", "full 40-character upstream commit SHA (required)")
	flag.Parse()

	if err := run(*commit); err != nil {
		fmt.Fprintln(os.Stderr, "update-rules:", err)
		os.Exit(1)
	}
}

func run(commit string) error {
	if !commitPattern.MatchString(commit) {
		return fmt.Errorf("--commit must be a full 40-character lowercase hex SHA, got %q", commit)
	}

	data, err := download(fmt.Sprintf(rawURLFormat, commit))
	if err != nil {
		return fmt.Errorf("downloading rules: %w", err)
	}
	if !json.Valid(data) {
		return fmt.Errorf("downloaded file is not valid JSON")
	}

	sum := sha256.Sum256(data)
	source := fmt.Sprintf(
		"repository: %s\ncommit: %s\nfile: %s\ndownloaded: %s\nsha256: %s\n",
		upstreamRepo, commit, dataFile,
		time.Now().UTC().Format("2006-01-02"),
		hex.EncodeToString(sum[:]),
	)

	if err := os.WriteFile(filepath.Join(outDir, dataFile), data, 0o644); err != nil {
		return fmt.Errorf("writing rules file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, sourceFile), []byte(source), 0o644); err != nil {
		return fmt.Errorf("writing SOURCE: %w", err)
	}

	fmt.Print(source)
	return nil
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("GET %s: response larger than %d bytes", url, maxSize)
	}
	return data, nil
}
