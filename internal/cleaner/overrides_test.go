package cleaner

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/pinebench/decrumb/internal/rules"
)

// TestOverrides checks our own globalRules additions (spec §8): the bundled
// upstream rules alone don't remove these parameters, and with our
// overrides each one is removed and credited to globalRules.
func TestOverrides(t *testing.T) {
	// rules.LoadFile uses a file exactly as given, without overrides.
	upstreamRules, err := rules.LoadFile(filepath.Join("..", "rules", "clearurls", "data.min.json"))
	if err != nil {
		t.Fatal(err)
	}
	upstream := New(upstreamRules)
	withOverrides := bundled(t)

	for _, name := range []string{
		"gbraid", "wbraid", "gad_source", "gad_campaignid", "gclsrc", // Google Ads
		"ttclid",           // TikTok
		"li_fat_id",        // LinkedIn
		"epik",             // Pinterest
		"ScCid",            // Snapchat
		"_hsmi",            // HubSpot
		"_branch_match_id", // Branch
	} {
		t.Run(name, func(t *testing.T) {
			input := "https://example.com/page?" + name + "=abc123&id=7"

			if got := upstream.Clean(input); got.Output != input {
				t.Errorf("upstream rules alone already remove %s: %q", name, got.Output)
			}

			got := withOverrides.Clean(input)
			want := []Removal{{Name: name, Provider: "globalRules", From: Query}}
			if got.Output != "https://example.com/page?id=7" || !slices.Equal(got.Removed, want) {
				t.Errorf("with overrides: %q, removed %+v; want %s removed by globalRules", got.Output, got.Removed, name)
			}
		})
	}
}
