package explain

import (
	"slices"
	"strings"
	"testing"

	"github.com/pinebench/decrumb/internal/cleaner"
	"github.com/pinebench/decrumb/internal/rules"
)

// specList is the list of 60 from spec §7, in the spec's order. Some names
// have one entry per provider (si, s, t, ref_src), so the file has more
// entries than names; what must match is the set of names.
var specList = []string{
	// campaign tags
	"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id",
	// ad click IDs
	"fbclid", "gclid", "msclkid", "yclid", "dclid", "twclid", "cmpid", "s_cid", "wt_mc", "_openstat",
	// email
	"mc_cid", "mc_eid", "_hsenc", "mkt_tok", "hsCtaTracking", "oly_enc_id", "vero_id", "__s",
	// analytics
	"_ga", "_gl", "srsltid",
	// referral
	"ref",
	// social
	"igsh", "igshid", "si", "feature", "pp", "ref_src", "s", "t", "mibextid", "__tn__", "trk", "lipi", "share_id",
	// search
	"ved", "ei", "oq", "gs_lcrp", "sca_esv", "cvid",
	// shopping
	"tag", "qid", "sr", "crid", "pd_rd_*", "pf_rd_*", "linkCode", "ascsubtag", "otracker", "lid", "srno", "spm",
	// news
	"smid",
}

// TestParamsFile checks that the embedded params.json passes validation
// (required fields, no duplicate name and provider, "*" rules, https
// sources, description length) and covers exactly the spec's 60 names.
func TestParamsFile(t *testing.T) {
	entries, err := parse(paramsJSON)
	if err != nil {
		t.Fatal(err)
	}

	if len(specList) != 60 {
		t.Fatalf("specList has %d names, want 60", len(specList))
	}
	var names []string
	for _, e := range entries {
		if !slices.Contains(names, e.Name) {
			names = append(names, e.Name)
		}
	}
	for _, name := range specList {
		if !slices.Contains(names, name) {
			t.Errorf("spec name %q has no entry", name)
		}
	}
	for _, name := range names {
		if !slices.Contains(specList, name) {
			t.Errorf("entry %q is not in the spec's list", name)
		}
	}
}

// TestParseRejects checks each validation rule on small broken files.
func TestParseRejects(t *testing.T) {
	const ok = `"description": "D: d.", "source": "https://example.com/doc", "example": "https://example.com/?a=1"`
	for _, tc := range []struct {
		name, json, wantErr string
	}{
		{"not a list", `{}`, "parsing params"},
		{"unknown field", `[{"name": "a", "provider": null, "extra": 1, ` + ok + `}]`, "unknown field"},
		{"missing name", `[{"provider": null, ` + ok + `}]`, "name is missing"},
		{"star in middle", `[{"name": "a*b", "provider": null, ` + ok + `}]`, `"*"`},
		{"two stars", `[{"name": "a**", "provider": null, ` + ok + `}]`, `"*"`},
		{"only star", `[{"name": "*", "provider": null, ` + ok + `}]`, `only "*"`},
		{"empty provider", `[{"name": "a", "provider": "", ` + ok + `}]`, "use null"},
		{"missing description", `[{"name": "a", "provider": null, "source": "https://e.com", "example": "https://e.com/?a=1"}]`, "description is missing"},
		{"long description", `[{"name": "a", "provider": null, "description": "` + strings.Repeat("x", 101) + `", "source": "https://e.com", "example": "https://e.com/?a=1"}]`, "longer than 100"},
		{"missing source", `[{"name": "a", "provider": null, "description": "D: d.", "example": "https://e.com/?a=1"}]`, "source is missing"},
		{"http source", `[{"name": "a", "provider": null, "description": "D: d.", "source": "http://e.com", "example": "https://e.com/?a=1"}]`, "https://"},
		{"missing example", `[{"name": "a", "provider": null, "description": "D: d.", "source": "https://e.com"}]`, "example is missing"},
		{"duplicate", `[{"name": "a", "provider": "p", ` + ok + `}, {"name": "A", "provider": "p", ` + ok + `}]`, "duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parse([]byte(tc.json))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("parse error = %v, want one containing %q", err, tc.wantErr)
			}
		})
	}

	// The same name with and without a provider, or with two providers,
	// is allowed.
	_, err := parse([]byte(`[{"name": "a", "provider": null, ` + ok + `}, {"name": "a", "provider": "p", ` + ok + `}, {"name": "a", "provider": "q", ` + ok + `}]`))
	if err != nil {
		t.Errorf("same name, different providers: %v", err)
	}
}

// TestDescribeOrder checks the spec §7 lookup order on a small fixture.
func TestDescribeOrder(t *testing.T) {
	fixture, err := parse([]byte(`[
		{"name": "x_*",   "provider": null,   "description": "family, any provider",   "source": "https://e.com", "example": "https://e.com/?x_1=a"},
		{"name": "x_*",   "provider": "shop", "description": "family, shop",           "source": "https://e.com", "example": "https://e.com/?x_1=a"},
		{"name": "x_one", "provider": null,   "description": "exact, any provider",    "source": "https://e.com", "example": "https://e.com/?x_one=a"},
		{"name": "x_one", "provider": "shop", "description": "exact, shop",            "source": "https://e.com", "example": "https://e.com/?x_one=a"},
		{"name": "only",  "provider": "shop", "description": "exact, shop only",       "source": "https://e.com", "example": "https://e.com/?only=a"}
	]`))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, provider string
		want           string // description, or "" for no match
	}{
		{"x_one", "shop", "exact, shop"},           // 1. exact name + provider
		{"x_one", "other", "exact, any provider"},  // 2. exact name without provider
		{"x_two", "shop", "family, shop"},          // 3. family + provider
		{"x_two", "other", "family, any provider"}, // 4. family without provider
		{"X_ONE", "shop", "exact, shop"},           // names ignore case
		{"X_Two", "other", "family, any provider"}, // also for families
		{"x_", "other", "family, any provider"},    // the bare prefix is in the family
		{"only", "shop", "exact, shop only"},       // scoped name, its provider
		{"only", "other", ""},                      // scoped name, another provider: no match
		{"only", "", ""},                           // scoped name, no provider: no match
		{"x_one", "Shop", "exact, any provider"},   // providers match exactly
		{"y_one", "shop", ""},                      // not described
	} {
		got, ok := fixture.describe(tc.name, tc.provider)
		if got.Description != tc.want || ok != (tc.want != "") {
			t.Errorf("describe(%q, %q) = %q, %v; want %q", tc.name, tc.provider, got.Description, ok, tc.want)
		}
	}
}

// TestExamplesRemoved checks that the bundled rules remove every described
// parameter (spec §7): cleaning each entry's example removes a parameter
// matching the entry, credited to the entry's provider when it has one,
// and Describe returns this entry for that removal.
func TestExamplesRemoved(t *testing.T) {
	r, err := rules.LoadBundled()
	if err != nil {
		t.Fatal(err)
	}
	c := cleaner.New(r)

	for _, e := range bundled {
		t.Run(e.Name+"/"+e.Provider, func(t *testing.T) {
			res := c.Clean(e.Example)
			if res.Err != nil {
				t.Fatalf("example %q: %v", e.Example, res.Err)
			}
			for _, rm := range res.Removed {
				if rm.From == cleaner.RawRule || !matches(e.Name, rm.Name) {
					continue
				}
				if e.Provider != "" && rm.Provider != e.Provider {
					t.Fatalf("%s removed by %q, want %q", rm.Name, rm.Provider, e.Provider)
				}
				got, ok := Describe(rm.Name, rm.Provider)
				if !ok || got != e {
					t.Fatalf("Describe(%q, %q) = %+v, %v; want this entry", rm.Name, rm.Provider, got, ok)
				}
				return
			}
			t.Errorf("cleaning %q did not remove %s; removed %+v", e.Example, e.Name, res.Removed)
		})
	}
}
