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
- `--explain`: for each URL, list every removed parameter, which provider rule removed it, and a short description for **60** common tracking parameters (list in §7, explain)
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
- **Exit codes:** `0` success · `1` at least one invalid input line (everything else is still cleaned) · `2` usage error: unknown flag or missing flag value (the Unix convention and Go's `flag` default) · `3` the rules could not be loaded (nothing is cleaned; stderr says why). 0–2 follow the common convention; 3 is decrumb's own, so scripts can tell a broken `--rules` file apart from a bad input line.
- Run with no URL arguments while stdin is an interactive terminal (nothing piped in), decrumb prints the usage text to stderr and exits `2` instead of waiting silently for input.
- Empty or whitespace-only stdin lines are skipped (no output line for them).
- URLs given as arguments take priority; stdin is read only when there are none.
- **Line length limit: 2 MB** (Chrome's own maximum URL length), so every URL a browser can open is accepted and cleaned with the normal rules. A longer line is reported as invalid on stderr (exit code 1), reading continues with the next line, and stdout gets an **empty line** for it (not the junk itself) so output stays aligned. No chunked cleaning: rules match the whole URL, so splitting would give different results. Tests: a URL just under 2 MB cleans correctly; a line just over is rejected and the following line is still processed. (Measured on `main`: normal links ~34 µs; 2 MB URLs 0.1–0.6 s each on a slow 2-core machine, scaling linearly.)
- **Invalid input lines are passed through unchanged** (trimmed) on stdout, so output stays aligned with input line by line; the error goes to stderr naming the line, and the exit code is 1. In `--json` the entry has `"output": null` and `"error"` set.
- Windows line endings (`\r\n`) on stdin are accepted (a trailing `\r` is removed with the line ending); output always uses `\n`. Tests cover CRLF input explicitly.
- `-h` / `--help` prints a short usage text with the flags and an example, to **stdout**, exit `0`. An unknown flag prints Go's message plus the usage to stderr, exit `2`; `--rules ""` is also a usage error.
- **Flags are accepted anywhere** on the command line (`decrumb URL --json` works), like `git`, `curl` and `grep`. `--` ends flag parsing: everything after it is a URL.
- A **UTF-8 BOM** at the very start of stdin is dropped (Windows PowerShell 5.1 and older Notepad add one); a BOM anywhere else is left alone.
- **Error messages describe what the user typed**, never decrumb's internal normalization (no `%20` the user didn't type). E.g. `invalid URL "not a url ::": spaces are not allowed in the host`.
- An empty URL **argument** is invalid (exit `1`); empty stdin lines are skipped as above. A read error on stdin exits `1`, and `--json` output is still closed as a valid document. In `--json`, invalid lines are also reported on stderr, and a line over the length limit has `"input": ""` (the line isn't kept, so memory stays constant). `--version` always describes the bundled rules, even with `--rules`.
- No colors in v0.1 (plain text in every terminal and pipe); colors may come in a later release.
- `--json` always includes everything (removals with descriptions, unwrap hops); `--explain` only affects plain output.
- **`--json` is streamed:** `{"results":[` is written first, then each result as soon as it's cleaned, then `]}`. The output is the same single valid JSON document, but memory stays constant and results appear while it runs. (If interrupted, the document is incomplete; the exit code shows it.) A test parses the output with a standard JSON parser.
- `--explain` plain output shows each removed parameter with its provider and description (if any), raw-rule removals (e.g. `/ref=sr_1_1 (raw rule, amazon)`), each unwrap hop, and a note when the 5-hop limit was reached.

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
        {"param": "utm_source", "provider": "globalRules", "location": "query", "description": "Campaign source: which site or newsletter sent you"}
      ],
      "hop_limit_reached": false,
      "error": null
    }
  ]
}
```
`description` is omitted for parameters without a description (outside the 60). `unwrapped` lists each redirect hop's URL. `error` is a string for invalid input, in which case `output` is null.

- **Output is always the normalized URL**: input without a scheme comes out with `https://` added, in both plain and JSON output, even when nothing was cleaned.
- **`changed` is `true` only if something was removed or unwrapped.** Adding `https://` alone is normalization, not cleaning, so it leaves `changed` false.

## 6. Cleaning logic (per URL)

1. Trim leading and trailing whitespace. Then **percent-encode every whitespace character left inside the URL**, the way browsers do (ASCII space, tab, newline and Unicode whitespace such as U+00A0, using their UTF-8 bytes: space → `%20`, U+00A0 → `%C2%A0`). No other characters are changed. This keeps the meaning of values like `?a &utm=1` (→ `?a%20`), keeps pasted links like `my file.pdf` working, and makes cleaning idempotent. If the input starts with `//` (protocol-relative, e.g. copied from HTML), prepend `https:`. Otherwise, if there's no scheme, add `https://`. "Has a scheme" uses the same rule as redirect decoding (one shared helper): `mailto:x` has a scheme; `example.com:8080/path` is a host and port, so it gets `https://`. Whitespace encoding applies to http(s) URLs only. Parse; if it can't be parsed, has no host, has whitespace in the host (including encoded, e.g. `%C2%A0`), has **invisible characters in the host** (Unicode format characters, category Cf, raw or percent-encoded: the BOM U+FEFF, zero-width spaces such as U+200B, the soft hyphen U+00AD, …), or has a **port outside 0–65535**, it's **invalid**: report an error (exit code 1). Invisible characters make lookalike domains (`pay​pal.com`), a classic phishing trick, so decrumb warns instead of handing such a link back as clean. International domain names (e.g. `пример.рф`) are unaffected. The same checks apply to redirect targets (step 5), so decrumb never unwraps into such a host.
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
- Errors from a user's `--rules` file must be readable without knowing Go, e.g. `rules.json: provider "p": "rules" must be a list of strings`. They're produced in the **rules** package, where the error happens; the cli prints them and exits `3`.

### Regex compatibility
The rules are written for JavaScript regexes; Go uses RE2. All 1,095 patterns in the current rules file (checked 2026-10-15) compile with Go's `regexp`. A unit test must compile every pattern in the bundled file, so a future rules update with an incompatible pattern fails CI instead of failing at runtime. Patterns are compiled case-insensitively (`(?i)`), matching ClearURLs.

## 7. Modules

Build in this order; each module is reviewed before the next starts.

1. **rules**: load and validate a rules file (bundled default via `go:embed`, or `--rules`), compile all patterns once, merge our overrides file into the bundled rules (§8). This module also adds the repo-wide no-network import test (§11), so it's enforced from the first module onwards.
2. **matcher**: find providers for a URL, in the §6 step 3 order (site-specific alphabetical, then `globalRules`). Each provider's `urlPattern` is tested against the full URL string, like ClearURLs; subdomains and country domains (`m.facebook.com`, `amazon.co.uk`) are handled by the patterns themselves and must be covered by tests.
3. **cleaner**: exceptions, redirections, rules, referral rules, raw rules; returns the clean URL plus a list of removals and unwrap hops. Includes the `internal/decode` package (§6, "Redirect target decoding"). Includes **fuzz tests** (`go test -fuzz`) asserting that for any input the cleaner never panics, always terminates, and is idempotent (cleaning an already-clean URL changes nothing), **except** when the 5-hop redirect limit was reached: the cleaner's result reports that the limit was hit, and those inputs are skipped by the idempotence check. Raw rules are **not** re-applied in a loop until nothing changes (that would reintroduce the slow-input risk); if fuzzing finds a raw-rule input that isn't idempotent, the example is reported for a decision rather than worked around in code.
4. **explain**: our own descriptions for **60** tracking parameter names (ClearURLs has none); names whose meaning differs per site get one entry per provider, so there are more entries than names. More can be added in later releases.
   - **The 60:** campaign tags `utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, `utm_content`, `utm_id`; ad click IDs `fbclid`, `gclid`, `msclkid`, `yclid`, `dclid`, `twclid`, `cmpid`, `s_cid`, `wt_mc`, `_openstat`; email `mc_cid`, `mc_eid`, `_hsenc`, `mkt_tok`, `hsCtaTracking`, `oly_enc_id`, `vero_id`, `__s`; analytics `_ga`, `_gl`, `srsltid`, `gad_source` (removed via our overrides); referral `ref`; social `igsh`, `igshid`, `si`, `feature`, `pp`, `ref_src`, `s`, `t`, `mibextid`, `__tn__`, `trk`, `share_id`; search `ved`, `ei`, `oq`, `gs_lcrp`, `sca_esv`, `cvid`; shopping `tag`, `qid`, `sr`, `crid`, `pd_rd_*`, `pf_rd_*`, `linkCode`, `ascsubtag`, `otracker`, `lid`, `srno`, `spm`; news `smid`. (Flipkart's `pid` is deliberately excluded until we've checked whether it selects the product variant.)
   - **Style:** one sentence, about 100 characters at most, shaped "What it is: what it does" (e.g. `fbclid`: "Facebook click ID: lets Facebook connect this visit to the account that clicked."). Plain, factual language: no fear words, no downplaying. English only in v0.1.
   - **Every description has a source**: a link to vendor documentation, or, where none exists, wording that says so honestly, keeping the style's shape: "What it is: undocumented; appears to …". No guesses presented as fact.
   - **Storage:** a data file, `internal/explain/params.json` (inside the package so `go:embed` can reach it), embedded into the binary. It's the single source for descriptions; the v0.2 browser extension reads the same file. Our own writing, MIT-licensed. Each entry: `{"name": …, "provider": … or null, "description": …, "source": …, "example": …}`, where `example` is a realistic URL containing the parameter (for a family, one concrete member), used by the removal test below and reusable by the extension. A test validates it: required fields present, no duplicate (name, provider) pairs, `*` only as a single trailing character.
   - **Matching a description to a removed parameter.** Each entry has a `name` (an exact name, or a family ending in a single trailing `*`, e.g. `pd_rd_*`; no other wildcards) and an optional `provider`. With a provider, the description applies only when that provider was credited with the removal (so `t` is described only on `x`/`twitter`, never elsewhere). Lookup order: exact name + matching provider → exact name without provider → family + matching provider → family without provider. Name matching ignores case, like the rules; it's done on the decoded name the cleaner reports. `utm_*` stays six separate entries, since each means something different.
   - **A test checks that every described parameter is actually removed by the bundled rules** (cleaning each entry's `example` removes that parameter, credited to the entry's provider when one is set), so a rules update that stops removing one makes CI flag the stale description.
5. **cli**: flags, args/stdin, plain/JSON output, exit codes

All packages live under `internal/` in v0.1 so the API can change freely; v0.3 promotes a stable public package.

## 8. Rules data & licensing

- Our code: **MIT**.
- ClearURLs rules (`data.min.json`): **LGPL-3.0**, from github.com/ClearURLs/Rules, kept **byte-for-byte unmodified**.
- Location: next to the package that embeds it (Go's `go:embed` can only reach files in the package directory or below), e.g. `internal/rules/clearurls/data.min.json`, together with a copy of the LGPL-3.0 and GPL-3.0 texts.
- Our fixes go in a separate overrides file we write from scratch (MIT), never copied-and-tweaked from their entries.
- **Overrides file** (`internal/rules/overrides.json`, same JSON schema as ClearURLs, starts empty: `{"providers": {}}`):
  - **Extend only.** A provider with a new name is added. A provider with an existing name gets our `rules`, `referralMarketing`, `rawRules`, `exceptions` and `redirections` **appended** to upstream's. Overrides can never remove or replace upstream entries. Its `urlPattern`, if given for an existing provider, is an error.
  - Entries are added only for real misses: **observed** in real links (our own use or user reports), **or documented by the vendor itself** as a click or tracking identifier. Each entry has a source, a test case in `testdata/cases.json`, and is proposed upstream to ClearURLs. Once upstream accepts a fix, our override entry is removed.
  - **Sources live in the file:** each provider object in `overrides.json` may carry a `"sources"` map from pattern to URL (the loader ignores unknown fields, so this stays compatible with the ClearURLs schema). A test checks that every override pattern has a source.
  - **First entries (v0.1):** added to `globalRules`: `gbraid`, `wbraid`, `gad_source`, `gad_campaignid`, `gclsrc` (Google Ads), `ttclid` (TikTok), `li_fat_id` (LinkedIn), `epik` (Pinterest), `ScCid` (Snapchat), `_hsmi` (HubSpot), `_branch_match_id` (Branch). All vendor-documented click or tracking IDs that the bundled rules (commit `11086f4`) don't remove. `rdt_cid` (Reddit) is pending: it's added once a readable Reddit page documenting it is confirmed as its source.
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

## 14. Release (v0.1.0)

### What gets built
- **Six binaries:** `windows`, `darwin` (macOS), `linux`, each for `amd64` and `arm64`.
- **Archives:** `.zip` for Windows, `.tar.gz` for macOS and Linux, named `decrumb_<version>_<os>_<arch>.<ext>`.
- **Each archive contains:** the `decrumb` binary, `README.md`, `LICENSE`, `NOTICE`, `COPYING.LESSER`, `COPYING` and the bundled rules file as `clearurls/data.min.json`, byte-for-byte identical to the embedded copy (the last five are required by the LGPL-3.0 checklist in §8; the rules file also gives users an editable copy to pass to `--rules`).
- **Build settings:** `CGO_ENABLED=0` (static, no system dependencies), `-trimpath`, and `-ldflags "-s -w -X main.version=<version>"`: `-s -w` strips the debug symbol tables (smaller binaries; Go's embedded build info stays, so `govulncheck` can still scan them), and `-X` injects the version, with its `v` (`v{{ .Version }}`, e.g. `v0.1.0`, the same as the tag and as `go install` builds), so `decrumb --version` shows the release version. Archive names keep GoReleaser's form without the `v` (`decrumb_0.1.0_linux_amd64.tar.gz`).

### Trust and verification
- **Checksums:** `checksums.txt` with the SHA-256 of every archive.
- **Build provenance:** GitHub artifact attestations (`actions/attest-build-provenance`) for **every archive and for `checksums.txt` itself**, so a swapped checksum file also fails verification. Verify with `gh attestation verify <file> -o pinebench`.
- **Immutable releases** enabled in the repo settings if GitHub offers it for this repo (a published release's files and tag can't be changed afterwards), plus a **tag ruleset**: only the owner can create `v*` tags, and they can't be updated or deleted.
- **No paid code signing in v0.1.** Windows SmartScreen and macOS Gatekeeper may warn on first run.
- **README sections:** "Install" (download a release, or `go install … @<version>` for developers who have Go, as two separate options); "Verify your download" (checksum and attestation checks with copy-paste commands for PowerShell, macOS and Linux); "Security warnings on first run" (what SmartScreen and Gatekeeper show and how to proceed, and that `go install` avoids them).

### Where it's published
- **GitHub Releases:** the six archives, `checksums.txt` and the attestations.
- **`go install github.com/pinebench/decrumb/cmd/decrumb@<version>`:** works once the tag exists; builds from source. With no version injected by `-ldflags`, `decrumb --version` falls back to the module version Go records in the binary (`runtime/debug.ReadBuildInfo`), e.g. `decrumb v0.1.0`, the same as release builds. Only when Go records no usable version (empty or `(devel)`) does it show `dev`. A build that isn't from a release tag (a plain `go build` in a git checkout, or `go install …@main`) gets a Go pseudo-version such as `v0.0.0-20261025093727-c252c0c87f27+dirty`; that is shown in readable form instead: `decrumb dev (c252c0c, 2026-10-25)`, the short commit and its UTC date, with `, modified` added when Go marked the build `+dirty` (uncommitted changes). Recognized by the pseudo-version's ending (a 14-digit timestamp and 12-character commit, optionally `+dirty`), without new dependencies.
- **Package managers (Homebrew, Scoop, winget): later** (v0.1.x or v0.2), starting with whichever early users ask for; each gets its own README walkthrough when added.
- **README install walkthroughs, step by step for every option:**
  - Release download on **Windows** (PowerShell: download, `Expand-Archive`, move `decrumb.exe` to e.g. `%USERPROFILE%\bin`, add it to the PATH, new terminal, `decrumb --version`, SmartScreen note); **macOS** (choosing Apple Silicon vs Intel, `tar`, move to `/usr/local/bin` or `~/bin`, clear the Gatekeeper quarantine, `decrumb --version`); **Linux** (amd64 vs arm64, `tar`, move to `~/.local/bin`, `decrumb --version`).
  - **`go install`:** check `go version`, run the command, where the binary lands and how to put it on the PATH, `decrumb --version`.
  - Each ends with a pointer to "Verify your download" (not needed for `go install`) and a first test: `decrumb "https://example.com/?utm_source=test"`.

### Versions and release notes
- **Semantic versioning**, starting at `v0.1.0`. **Patch:** fixes, ClearURLs rules updates, new overrides and descriptions. **Minor:** new features, or changes to output or flags. **`v1.0.0`** once the command interface is stable and has had real users.
- **Security fixes** are patch releases (minor if the fix must change behavior), released **on their own, immediately**, with a **Security** section at the top of the notes (what was affected, which versions, what users should do), and a **GitHub Security Advisory** published with the release (CVE when appropriate; feeds Go's vulnerability database, so `govulncheck` warns users of affected versions). Reports come in through private vulnerability reporting.
- **`SECURITY.md`** in the repo: how to report privately (the advisory form), which versions get fixes (below 1.0: only the latest release), and expected response time.
- **Releasing = pushing a `v*` tag on `main`**; the workflow builds, attests and publishes everything. Nothing is uploaded by hand.
- **Release notes replace a CHANGELOG.md** and must contain everything one would, in this order: **Security**, **Added**, **Changed** (breaking changes marked), **Fixed**, **Removed / Deprecated** (when relevant), **Rules & data** (bundled ClearURLs commit; overrides and descriptions added or removed), **Upgrade notes** (when needed). Commit groups are generated from conventional-commit titles (`feat:` → Added, `fix:` → Fixed, …); a hand-written intro adds context, breaking changes and upgrade steps.

### Release workflow
- **`.github/workflows/release.yml`, triggered only by pushing a `v*` tag.** The CI workflow stays read-only.
- **Permissions, in this workflow only:** `contents: write` (create the release), `id-token: write` and `attestations: write` (provenance). Nothing else.
- **Tests first:** the full test suite runs on the tagged commit before building; any failure stops the release.
- **GoReleaser** via `go run github.com/goreleaser/goreleaser/v2@<pinned version>` (no third-party Action; verified by Go's checksum database), configured in `.goreleaser.yaml`.
- **Manual approval:** the publishing job uses a GitHub environment `release` with the owner as required reviewer, so nothing is published until it's approved.
- **The release config is tested on every PR:** CI runs `goreleaser check` and a `--snapshot` build (nothing published).

### Testing the release before v0.1.0
- **Release candidate first:** the first tag is `v0.1.0-rc.1`, published as a GitHub *pre-release* through the full pipeline. Problems are fixed and a new RC (`rc.2`, …) is cut. Only when an RC passes everything is the **same commit** tagged `v0.1.0`.
- **Automatic smoke test (mandatory):** after publishing, a job on fresh `ubuntu-latest`, `macos-latest` and `windows-latest` runners downloads the published archive like a user would, verifies its checksum **and** its attestation, extracts it, and runs `decrumb --version` plus a few `testdata/cases.json` inputs. A failure fails the workflow; the owner then marks the release as broken by hand (a warning at the top of its notes; see `RELEASING.md`) and fixes it in the next RC or patch. Marking is deliberately manual, so the smoke job needs no write permission.
- **Manual install test:** required for `v0.1.0`, every minor or major release, and any release that changes the README's install or verify sections or `.goreleaser.yaml`; optional for other patches. When required: following only the README, without Go installed, on at least **one** clean environment: an Ubuntu VirtualBox VM (from a snapshot without Go), a **new Windows user account** (empty PATH, no Go), or another person's machine. One passing manual test is enough to proceed. In exceptional, unavoidable cases it may be skipped and the automatic smoke test relied on, with the reason noted in the release notes.
