package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/pinebench/decrumb/internal/cleaner"
	"github.com/pinebench/decrumb/internal/explain"
)

// printer writes results to stdout as they come: plain text, plain text
// with explanations, or a JSON document that is streamed result by result
// (spec §5). Each result is written with a single Write call.
type printer struct {
	w       io.Writer
	json    bool
	explain bool // plain output only: --json ignores it
	count   int  // results written so far
}

// start writes what comes before the first result.
func (p *printer) start() {
	if p.json {
		io.WriteString(p.w, "{\"results\":[\n")
	}
}

// print writes one result. For an invalid input, plain output repeats the
// input, trimmed, so output lines stay aligned with input lines.
func (p *printer) print(res cleaner.Result) {
	var b bytes.Buffer
	switch {
	case p.json:
		if p.count > 0 {
			b.WriteString(",\n")
		}
		writeJSON(&b, res)
	case res.Err != nil:
		b.WriteString(strings.TrimSpace(res.Input))
		b.WriteByte('\n')
	default:
		b.WriteString(res.Output)
		b.WriteByte('\n')
		if p.explain {
			writeExplanation(&b, res)
		}
	}
	p.w.Write(b.Bytes())
	p.count++
}

// end writes what comes after the last result.
func (p *printer) end() {
	if !p.json {
		return
	}
	if p.count > 0 {
		io.WriteString(p.w, "\n]}\n")
	} else {
		io.WriteString(p.w, "]}\n")
	}
}

// jsonResult is one entry of the --json "results" list (spec §5). Output
// and Error are pointers so they can be null.
type jsonResult struct {
	Input           string        `json:"input"`
	Output          *string       `json:"output"`
	Changed         bool          `json:"changed"`
	Unwrapped       []string      `json:"unwrapped"`
	Removed         []jsonRemoval `json:"removed"`
	HopLimitReached bool          `json:"hop_limit_reached"`
	Error           *string       `json:"error"`
}

type jsonRemoval struct {
	Param       string `json:"param"`
	Provider    string `json:"provider"`
	Location    string `json:"location"`
	Description string `json:"description,omitempty"`
}

// writeJSON writes res as one line of compact JSON, without a newline.
func writeJSON(b *bytes.Buffer, res cleaner.Result) {
	jr := jsonResult{
		Input:           res.Input,
		Changed:         res.Changed,
		Unwrapped:       res.Unwrapped,
		Removed:         []jsonRemoval{},
		HopLimitReached: res.HopLimitReached,
	}
	if jr.Unwrapped == nil {
		jr.Unwrapped = []string{} // [] rather than null
	}
	if res.Err != nil {
		msg := res.Err.Error()
		jr.Error = &msg
	} else {
		jr.Output = &res.Output
	}
	for _, rm := range res.Removed {
		jr.Removed = append(jr.Removed, jsonRemoval{
			Param:       rm.Name,
			Provider:    rm.Provider,
			Location:    string(rm.From),
			Description: describe(rm),
		})
	}

	enc := json.NewEncoder(b)
	enc.SetEscapeHTML(false) // keep & < > readable in URLs instead of &
	if err := enc.Encode(jr); err != nil {
		// Can't happen: every field is a string, bool or list of those.
		panic(err)
	}
	b.Truncate(b.Len() - 1) // Encode adds a newline
}

// describe returns the description of a removed parameter, or "" if there
// is none. Raw-rule removals are URL text, not parameters, so they have none.
func describe(rm cleaner.Removal) string {
	if rm.From == cleaner.RawRule {
		return ""
	}
	if e, ok := explain.Describe(rm.Name, rm.Provider); ok {
		return e.Description
	}
	return ""
}

// maxNameColumn caps the width of the name column in --explain output, so
// one long raw-rule match doesn't push every other line far to the right.
const maxNameColumn = 24

// writeExplanation writes the --explain lines for res, under its URL:
//
//	unwrapped  https://example.com/article?utm_source=x
//	removed    utm_source  globalRules  Campaign source: ...
//	removed    /ref=sr_1_1 amazon       (raw rule)
func writeExplanation(b *bytes.Buffer, res cleaner.Result) {
	for _, u := range res.Unwrapped {
		fmt.Fprintf(b, "  unwrapped  %s\n", u)
	}
	if res.HopLimitReached {
		fmt.Fprintf(b, "  note       stopped after %d redirect hops; the last URL reached was cleaned\n", cleaner.MaxHops)
	}

	nameWidth, providerWidth := 0, 0
	for _, rm := range res.Removed {
		nameWidth = max(nameWidth, min(utf8.RuneCountInString(rm.Name), maxNameColumn))
		providerWidth = max(providerWidth, utf8.RuneCountInString(rm.Provider))
	}
	for _, rm := range res.Removed {
		var about string
		switch rm.From {
		case cleaner.RawRule:
			about = "(raw rule)"
		case cleaner.Fragment:
			about = strings.TrimSpace("(in fragment) " + describe(rm))
		default:
			about = describe(rm)
		}
		// %-*s pads to a width given as an argument; fmt counts runes.
		line := fmt.Sprintf("  removed    %-*s %-*s  %s", nameWidth+1, rm.Name, providerWidth, rm.Provider, about)
		b.WriteString(strings.TrimRight(line, " "))
		b.WriteByte('\n')
	}
}
