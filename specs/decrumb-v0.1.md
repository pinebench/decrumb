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

- **Output is always the normalized URL**: input without a scheme comes out with `https://` added, in both plain and JSON output, even when nothing was cleaned.
- **`changed` is `true` only if something was removed or unwrapped.** Adding `https://` alone is normalization, not cleaning, so it leaves `changed` false.

## 6. Cleaning logic (per URL)

1. Trim leading and trailing whitespace. Then **percent-encode every whitespace character left inside the URL**, the way browsers do (ASCII space, tab, newline and Unicode whitespace such as U+00A0, using their UTF-8 bytes: space → `%20`, U+00A0 → `%C2%A0`). No other characters are changed. This keeps the meaning of values like `?a &utm=1` (→ `?a%20`), keeps pasted links like `my file.pdf` working, and makes cleaning idempotent. If the input starts with `//` (protocol-relative, e.g. copied from HTML), prepend `https:`. Otherwise, if there's no scheme, add `https://`. "Has a scheme" uses the same rule as redirect decoding (one shared helper): `mailto:x` has a scheme; `example.com:8080/path` is a host and port, so it gets `https://`. Whitespace encoding applies to http(s) URLs only. Parse; if it can't be parsed, has no host, or has whitespace in the host (including encoded, e.g. `%C2%A0`), it's **invalid**: report an error (exit code 1).
2. If the scheme is not `http`/`https`, return the input unchanged (only trimmed).
3. Collect the providers whose `urlPattern` matches (site-specific providers + `globalRules`). **Order:** site-specific providers first (sorted by name in byte order, so capitalized names come first), then `globalRules`. This order decides which provider is credited for a removal (the first one whose rule matches) and which redirection is tried first.
4. **Exceptions are per provider** (like ClearURLs): if a provider's **exception** matches the URL, that provider is skipped for this URL; every other matching provider still applies. A URL is left unchanged only if every matching provider is skipped.
5. If a **redirection** matches → take the captured target, decode it (see "Redirect target decoding" below), and restart from step 1 with it. Stop after **5 hops**: the last URL reached is still cleaned (steps 6–8), and the result reports that the limit was hit. Redirections are tried in the step 3 provider order; if a target can't be decoded into a valid http(s) URL, the **next** matching redirection is tried. If none works, don't unwrap: keep the current URL and continue cleaning it.
6. Apply **rawRules** (regexes applied to the URL string, removing every match). Raw rules run **before** parameter removal, in the same order as ClearURLs. **Safety check:** after each raw rule, re-parse the URL; if it's no longer a valid http(s) URL with the **same host**, discard that rule's change and continue with the next rule.
7. Remove query parameters whose **name** matches a provider **rule** or **referralMarketing** rule. Matching is case-insensitive and must match the whole parameter name. Rules are tested against the **percent-decoded** name (so `utm%5Fsource` is treated as `utm_source`, like ClearURLs), while the output keeps every remaining parameter's **original** encoding (step 8). If a name can't be decoded (malformed percent-encoding), test the raw name instead. Do the same for parameters in the fragment (`#a=1&b=2`), but **only if the fragment contains `=`**; otherwise the fragment is left byte-for-byte unchanged (`#section-2`, `#/inbox/42`). When it is treated as parameters: split on `&`, remove matching names, rejoin the rest in original order and encoding; a value containing `=` (e.g. `#:~:text=a=b`) is kept whole. If nothing is removed, the fragment is not rebuilt at all; if every fragment parameter is removed, the `#` is dropped (like the `?`). This is deliberately stricter than ClearURLs, which rebuilds every fragment and can drop values containing `=`, empty pieces, and the order of repeated names.
8. Keep remaining parameters in their **original order and original encoding**. Keep the fragment. Don't add or remove a trailing `?` unless all parameters were removed (then drop the `?`).
   - Repeated parameters: **every** matching occurrence is removed (`?utm_source=x&utm_source=y`).
   - Valueless and empty parameters (`?flag`, `a=1&&b=2`) are kept exactly as they are unless a rule matches them.
   - Host and path are never normalized (uppercase hosts stay uppercase); only parameters and raw-rule matches are removed.

### Rule fields: how each is treated
| Field | v0.1 behavior |
|---|---|
| `urlPattern` | Decides which providers apply to a URL |
| `rules` | Parameter names to remove |
| `referralMarketing` | Removed by default (matches ClearURLs' default) |
| `rawRules` | Regexes applied to the whole URL |
| `exceptions` | Skip **that provider** for a matching URL (per provider, §6 step 4) |
| `redirections` | Offline unwrap of the captured target |
| `completeProvider` | **Ignored.** In the browser it blocks whole ad domains; a CLI that cleans URLs has nothing to block. |
| `forceRedirection` | **Ignored.** Browser-only behavior. |

### Redirect target decoding (package `internal/decode`)
**Semantics: decode only as much as needed.** The captured target is a parameter value, so it's decoded once; it's decoded further only while it still doesn't start with `http://` or `https://` (case-insensitive) because it was wrapped more than once. ClearURLs decodes until nothing changes, which can corrupt the target (`q=salt%2526pepper` must become `q=salt%26pepper`, not `q=salt&pepper`); we deliberately don't.
- Let *k* be the number of decoding rounds needed for the scheme (`https://` or `http://`, case-insensitive) at the start to appear, with *k* ≥ 1. The result is the target after exactly *k* rounds. If no http(s) scheme appears at any depth, *k* = 1: decode one round.
- A "round" decodes every valid `%HH` (two hex digits) once, left to right. `+` is **not** turned into a space. Invalid escapes (`%`, `%G1`, `%4`) are left as they are. Work on bytes; the result must be valid UTF-8.
- **Accepting the result** (otherwise don't unwrap):
  1. Starts with `http://` or `https://` → use it, if it parses as a URL with a non-empty host.
  2. Starts with any other scheme (letters, digits, `+ . -`, then `:`, where what follows the `:` is not a port number) → not usable. This rejects `mailto:`, `javascript:`, `ftp:`, `data:` and the like.
  3. Starts with `//` (protocol-relative, like step 1) → prepend `https:`, then apply the same checks as rule 4 (`///example.com` and `//user@example.com` are rejected).
  4. Otherwise (no scheme, e.g. Google AMP's `amp/s/example.com/article`) → prepend `https://` and accept only if it parses with a non-empty host that contains a dot or is an IP address, and has **no userinfo** (so `mailto`-style text like `a@b.com` can't become `https://a@b.com`).
- Required cases also include: `example.com/article`, `example.com:8080/path` (a port, not a scheme), `mailto:a@b.com`, `javascript:alert(1)`, `foo` (no dot), `user:pass@example.com`.

**Algorithm: linear time, no round limit.** Decoding round by round costs length × rounds, so a deeply nested hostile input is a slow-input denial of service (measured in the prototype: 5.8 s at depth 8,000). Required instead:
- A single left-to-right pass with a stack of (byte, depth) pairs. After each push, while the top three entries form `%HH`, replace them with the decoded byte whose depth is `max(depths) + 1`. With a depth cap *K*, only reductions whose new depth is ≤ *K* are applied.
- Pass 1 (no cap) finds the depth of the bytes forming the scheme, which gives *k*. Pass 2 runs with cap *K = k*. Both passes are linear, because each byte is pushed once and each reduction removes two bytes for good.
- Why it's correct: escapes never overlap (the bytes after `%` are hex digits, never `%`), so the order of decoding doesn't change the result, and the depth of a byte equals the round in which round-by-round decoding would produce it.

**Required tests:**
- A simple round-by-round reference decoder, in test code only.
- Property tests and a fuzz test: for random and adversarial inputs, the fast decoder equals the reference, both fully and with a cap of *K* rounds for *K* = 0…6.
- A scaling test: on deeply nested hostile inputs (e.g. `%` + `25`×n + `41`), doubling n must not much more than double the time.
- Cases: single-, double- and triple-encoded targets; the `salt%26pepper` case; invalid escapes; `+` kept; uppercase `HTTPS://`; targets without a scheme; inputs that never become a URL.

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
3. **cleaner**: exceptions, redirections, rules, referral rules, raw rules; returns the clean URL plus a list of removals and unwrap hops. Includes the `internal/decode` package (§6, "Redirect target decoding"). Includes **fuzz tests** (`go test -fuzz`) asserting that for any input the cleaner never panics, always terminates, and is idempotent (cleaning an already-clean URL changes nothing), **except** when the 5-hop redirect limit was reached: the cleaner's result reports that the limit was hit, and those inputs are skipped by the idempotence check. Raw rules are **not** re-applied in a loop until nothing changes (that would reintroduce the slow-input risk); if fuzzing finds a raw-rule input that isn't idempotent, the example is reported for a decision rather than worked around in code.
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
- No network access anywhere in the program. A test asserts that no package built into the binary (`cmd/`, `internal/`) depends, directly or indirectly, on Go's `net` package, which is where all network I/O lives (`net/http`, `crypto/tls` and `golang.org/x/net` all depend on it). Those are also checked explicitly as a backstop. Pure parsing packages such as `net/url` and `net/netip` are allowed. Dev-time tools under `scripts/` are exempt.

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
