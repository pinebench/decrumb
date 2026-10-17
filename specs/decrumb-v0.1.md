# decrumb v0.1 — Spec

Status: **Approved** (2026-10-15)
Home: `github.com/pinebench/decrumb` (also the Go module path, fixed from the first commit)

## 1. Summary

`decrumb` is a Go command-line tool that removes tracking parameters from URLs, fully offline. Cleaning is site-aware: a parameter is removed only where it is known to be tracking, so links don't break.

## 2. Users

- People who share links and don't want to pass on tracking IDs
- Developers and power users who clean URLs in scripts and pipelines

## 3. Principles (non-negotiable)

1. **No network, ever.** The binary never opens a network connection. Everything it needs ships inside it.
2. **Never break a link by default.** Unknown parameters are kept.
3. **Nothing stored.** No history, no cache, no config or files written to disk.

## 4. Scope

### In scope (v0.1)
- Clean one or more URLs passed as arguments
- Read URLs from stdin, one per line, when no URL arguments are given
- Site-aware cleaning using the ClearURLs rules (global rules + per-site rules and exceptions)
- Offline redirect unwrapping (e.g. `google.com/url?q=…`, Facebook `l.php?u=…`), then clean the result
- `--explain`: for each URL, list every removed parameter, which provider rule removed it, and a short description for the **top 30** most common tracking parameters
- `--json`: machine-readable output
- `--rules <file>`: load rules from this file instead of the bundled ones
- `--version`: version, bundled rules revision, and rules attribution

### Out of scope (v0.1)
- Browser extension (v0.2)
- Go library API and bulk file input (v0.3)
- Following real redirects or expanding short links (bit.ly, t.co): needs network
- Clipboard watching
- A "strict" mode that removes unknown parameters

## 5. Command interface

```
decrumb [flags] [url ...]

Flags:
  --explain        show what was removed and why
  --json           output JSON instead of plain text
  --rules <file>   load rules from this file instead of the bundled ones
  --version        print version, bundled rules revision and attribution
```

- **Plain output:** one clean URL per line, same order as input.
- **Errors** go to stderr. An invalid URL does not stop processing of the others.
- **Exit codes:** `0` all good · `1` at least one invalid URL · `2` rules could not be loaded.
- Empty or whitespace-only stdin lines are skipped (no output line for them).

### `--explain` plain output (shape, final wording at implementer's discretion)
```
https://example.com/page
  removed fbclid   (rule: globalRules)  Facebook click ID: links your visit to your Facebook account
  removed utm_source (rule: globalRules)  Campaign source: which site or newsletter sent you
```

### `--json` output (one JSON document per run)
```json
{
  "results": [
    {
      "input": "https://example.com/page?utm_source=x&id=7",
      "output": "https://example.com/page?id=7",
      "changed": true,
      "unwrapped": [],
      "removed": [
        {"param": "utm_source", "provider": "globalRules", "description": "Campaign source: which site or newsletter sent you"}
      ],
      "error": null
    }
  ]
}
```
`description` is omitted for parameters outside the top 30. `unwrapped` lists each redirect hop's URL. `error` is a string for invalid input, in which case `output` is null.

## 6. Cleaning logic (per URL)

1. Trim whitespace. If there's no scheme, add `https://`. Parse; if it's still invalid, report an error.
2. If the scheme is not `http`/`https`, return the input unchanged.
3. Collect the providers whose `urlPattern` matches (site-specific providers + `globalRules`). **Order:** site-specific providers first (alphabetical among themselves), then `globalRules`. This order decides which provider is credited for a removal (the first one whose rule matches) and which redirection is tried first.
4. If any matching provider's **exception** matches → return the URL unchanged (stop).
5. If a **redirection** matches → take the captured target, URL-decode it, and restart from step 1 with it. Stop after **5 hops** (return the last URL reached).
6. Remove query parameters whose **name** matches a provider **rule** or **referralMarketing** rule. Matching is case-insensitive and must match the whole parameter name. Rules are tested against the **percent-decoded** name (so `utm%5Fsource` is treated as `utm_source`, like ClearURLs), while the output keeps every remaining parameter's **original** encoding (step 8). If a name can't be decoded (malformed percent-encoding), test the raw name instead. Do the same for parameters in the fragment (`#a=1&b=2`).
7. Apply **rawRules** (regexes applied to the URL string, removing matches).
8. Keep remaining parameters in their **original order and original encoding**. Keep the fragment. Don't add or remove a trailing `?` unless all parameters were removed (then drop the `?`).

### Rule fields: how each is treated
| Field | v0.1 behavior |
|---|---|
| `urlPattern` | Decides which providers apply to a URL |
| `rules` | Parameter names to remove |
| `referralMarketing` | Removed by default (matches ClearURLs' default) |
| `rawRules` | Regexes applied to the whole URL |
| `exceptions` | Skip cleaning entirely |
| `redirections` | Offline unwrap of the captured target |
| `completeProvider` | **Ignored.** In the browser it blocks whole ad domains; a CLI that cleans URLs has nothing to block. |
| `forceRedirection` | **Ignored.** Browser-only behavior. |

### Loading behavior (decided in the rules module)
- Empty patterns are rejected. Unknown JSON fields are ignored, so new upstream fields don't break loading.
- Rule and referral patterns match the **whole** parameter name, compiled as `(?i)^(?:pattern)$`. This matches ClearURLs' own behavior (it tests `^rule$`). Known consequence: Amazon's `[^a-z%0-9]adId` matches `_adId` but not `adId`, exactly as in ClearURLs; we add an override only if a plain `adId` tracker is actually observed.
- Errors from a user's `--rules` file must be readable without knowing Go: the **cli** module turns JSON type errors into messages like `rules.json: provider "p": "rules" must be a list of strings`.

### Regex compatibility
The rules are written for JavaScript regexes; Go uses RE2. All 1,095 patterns in the current rules file (checked 2026-10-15) compile with Go's `regexp`. A unit test must compile every pattern in the bundled file, so a future rules update with an incompatible pattern fails CI instead of failing at runtime. Patterns are compiled case-insensitively (`(?i)`), matching ClearURLs.

## 7. Modules

Build in this order; each module is reviewed before the next starts.

1. **rules**: load and validate a rules file (bundled default via `go:embed`, or `--rules`), compile all patterns once, merge our overrides file into the bundled rules (§8). This module also adds the repo-wide no-network import test (§11), so it's enforced from the first module onwards.
2. **matcher**: find providers for a URL, in the §6 step 3 order (site-specific alphabetical, then `globalRules`). Each provider's `urlPattern` is tested against the full URL string, like ClearURLs; subdomains and country domains (`m.facebook.com`, `amazon.co.uk`) are handled by the patterns themselves and must be covered by tests.
3. **cleaner**: exceptions, redirections, rules, referral rules, raw rules; returns the clean URL plus a list of removals and unwrap hops. Includes **fuzz tests** (`go test -fuzz`) asserting that for any input the cleaner never panics, always terminates, and is idempotent (cleaning an already-clean URL changes nothing).
4. **explain**: our own descriptions for the top 30 tracking parameters (ClearURLs has none)
5. **cli**: flags, args/stdin, plain/JSON output, exit codes

All packages live under `internal/` in v0.1 so the API can change freely; v0.3 promotes a stable public package.

## 8. Rules data & licensing

- Our code: **MIT**.
- ClearURLs rules (`data.min.json`): **LGPL-3.0**, from github.com/ClearURLs/Rules, kept **byte-for-byte unmodified**.
- Location: next to the package that embeds it (Go's `go:embed` can only reach files in the package directory or below), e.g. `internal/rules/clearurls/data.min.json`, together with a copy of the LGPL-3.0 and GPL-3.0 texts.
- Our fixes go in a separate overrides file we write from scratch (MIT), never copied-and-tweaked from their entries.
- **Overrides file** (`internal/rules/overrides.json`, same JSON schema as ClearURLs, starts empty: `{"providers": {}}`):
  - **Extend only.** A provider with a new name is added. A provider with an existing name gets our `rules`, `referralMarketing`, `rawRules`, `exceptions` and `redirections` **appended** to upstream's. Overrides can never remove or replace upstream entries. Its `urlPattern`, if given for an existing provider, is an error.
  - Entries are added only for real, observed misses (our own use or user reports), each with a test case, and proposed upstream to ClearURLs. Once upstream accepts a fix, our override entry is removed.
  - **`--rules <file>` disables overrides.** The user's file is used exactly as given, alone.
- **Updating rules** is a dev-time script, never done by the tool: download `data.min.json` from a **specific upstream commit**, and record that commit SHA and the file's SHA-256 in a lock file, `internal/rules/clearurls/SOURCE`, as `key: value` lines: `repository`, `commit`, `file`, `downloaded`, `sha256`. CI verifies the bundled file's SHA-256 matches the lock file. `--version` prints the commit SHA as the rules revision.

### LGPL-3.0 compliance checklist (§4, Combined Works)
- [ ] Rules file embedded byte-for-byte unmodified (no minifying or converting at build time)
- [ ] Overrides file written from scratch
- [ ] Repo and every release archive contain: `LICENSE` (MIT), `COPYING.LESSER` (LGPL-3.0), `COPYING` (GPL-3.0), `NOTICE`, and the rules file
- [ ] Prominent notice in README and NOTICE: uses the ClearURLs rules, covered by LGPL-3.0, with a link to the upstream repo; credited to "the ClearURLs project and contributors"
- [ ] `--version` output includes the rules attribution and points to the license files
- [ ] Source stays public and buildable with a modified rules file (§4d0); `--rules` swaps rules at runtime (§4d1)
- [ ] Wording never implies endorsement: "uses ClearURLs rules", not "ClearURLs for the command line"
- [ ] If upstream test cases are reused, they live in their own testdata folder with the same notice

## 9. Edge cases to test

- Subdomains and country domains
- URLs without a scheme; uppercase hosts and schemes; trailing `?` or `&`
- Encoded and double-encoded parameters; redirect targets that are themselves encoded
- Parameters in the fragment (`#…`)
- Repeated parameters (`?a=1&a=2`); parameters with no value (`?flag`)
- Redirect loops and chains (stop at 5 hops)
- Non-http(s) input (`mailto:`, `ftp:`) → unchanged
- Empty lines and surrounding whitespace in stdin
- Very long URLs and very long stdin input

## 10. Shared test cases

`testdata/cases.json`: a language-neutral list of `{ "input": …, "output": …, "note": … }`. The Go tests run every case. The v0.2 TypeScript extension will run the same file, so the two implementations can't drift apart.

## 11. Non-functional

- Platforms: Windows, macOS, Linux (amd64 + arm64)
- Single static binary, no runtime dependencies
- Cleans 10,000 URLs from stdin in under 1 second on a typical laptop (benchmark test)
- No network access anywhere in the program. A test asserts that no package built into the binary (`cmd/`, `internal/`) imports `net`, `net/http` or other networking packages (`net/url` is fine: it only parses strings). Dev-time tools under `scripts/` are exempt.

## 12. Roadmap

- **v0.1**: this CLI
- **v0.2**: browser extension (TypeScript). Differentiator: shows what was removed and why. Must pass `testdata/cases.json`.
- **v0.3**: public Go library + bulk input (CSV, text files, bookmark exports)

## 13. Done when

Module done:
1. Matches this spec, no scope creep
2. Tests cover the main paths and the relevant edge cases above, and pass
3. CI green on Windows, macOS and Linux: build, test, vet, format check, lint, vulnerability check
4. README and CLAUDE.md updated
5. Reviewed in the Claude project against this spec (project tracking, e.g. Notion, is handled there, not in the repo)

Product v0.1 done:
- All modules done
- Release pipeline set up (GoReleaser) and the first release published through it, with the license files in every archive
- Someone else can install and use it from the README alone
