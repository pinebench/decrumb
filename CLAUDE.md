# CLAUDE.md — decrumb

`decrumb` is an offline Go CLI that removes tracking parameters from URLs using the ClearURLs rules.
It's the first product of **Pinebench** (`github.com/pinebench`): small, useful, open-source tools.

## Source of truth

- The approved spec is **`specs/decrumb-v0.1.md`**. Build exactly what it says.
- If the spec is ambiguous, contradicts itself, or seems wrong, **stop and ask**. Don't silently pick an interpretation or "improve" the scope. Spec changes are decided outside this repo and then written back into the spec file.
- Anything listed as out of scope stays out, even if it's easy to add.
- **Never merge your own PRs** (no `gh pr merge`, no pushing to `main`). The owner merges only after CI is green and the module has been reviewed in the Claude project against the spec.

## How we work

- Build what the spec says; for spec'd modules, one module per branch (`module/<name>`) and PR.
- Process, branches, commits and releases follow the workspace rules.
- The owner is learning Go. Write simple, idiomatic, readable Go over clever Go, and briefly explain non-obvious idioms in the module summary (not in long code comments).

## Non-negotiables

1. **No network.** No package built into the binary (`cmd/` and `internal/`) may depend, directly or indirectly, on Go's `net` package, which is where all network I/O lives. Pure parsing packages like `net/url` and `net/netip` are fine. A test enforces this (`nonetwork_test.go`). The dev-time tools in `scripts/` are exempt.
2. **Nothing stored.** The binary never writes files, caches, config or history.
3. **Rules file is never modified.** `internal/rules/clearurls/data.min.json` is LGPL-3.0 and must stay byte-for-byte identical to the upstream commit recorded in `SOURCE`. Only the update script changes it.
4. **Overrides are written from scratch.** Never copy an upstream rule and tweak it.
5. **No copied code.** Read other projects (ClearURLs, other cleaners) to understand them, then write our own implementation. Don't paste their code.
6. **Never commit secrets.** No keys, tokens or `.env` files.

## Layout

```
cmd/decrumb/            main package (thin: calls cli.Run); main_test.go builds the binary (end-to-end)
internal/cli/           the command line (spec §5): flags, args/stdin, plain/--explain/--json output, exit codes
internal/rules/         load + validate + compile rules; merge overrides
internal/rules/overrides.json  our own additions (MIT, extend-only, spec §8)
internal/rules/clearurls/  data.min.json (LGPL-3.0, go:embed), SOURCE lock file, license copies
nonetwork_test.go       repo-wide test: nothing in cmd/ or internal/ imports networking packages
internal/matcher/       providers for a URL
internal/scheme/        "has a scheme?" check shared by decode and cleaner (spec §6 step 1)
internal/decode/        redirect target decoding (linear time, spec §6)
internal/cleaner/       the cleaning pipeline (spec §6 steps 1–8); fuzz seeds in its testdata/fuzz/
internal/explain/       descriptions for the 60 parameter names (64 entries) in spec §7; Describe(name, provider)
internal/explain/params.json  the descriptions (MIT, go:embed; also read by the future extension)
testdata/cases.json     shared input → output cases (spec §10; also used by the future TS extension)
scripts/update-rules    dev-time rules update (download pinned commit, write SOURCE)
specs/                  specs
.goreleaser.yaml        release builds, archives, checksums, release notes groups (spec §14)
.github/workflows/      ci.yml (also called by release.yml), release.yml (v* tags only)
SECURITY.md, RELEASING.md  vulnerability reporting; the owner's release steps
```

## Go conventions

- Module path: `github.com/pinebench/decrumb`. Always lowercase.
- Go version: the latest stable release; set it in `go.mod` and use the same in CI.
- **Standard library first.** Use `flag` for CLI flags. Any third-party dependency needs a reason in the PR description.
- Wrap errors with context: `fmt.Errorf("loading rules: %w", err)`.
- Table-driven tests. Every bug fix comes with a test that fails without the fix.
- Compile regexes once at load time, never per URL.

## Commands

```
go build ./...
go test ./...
go test -race ./...
go vet ./...
gofmt -l .                      # must print nothing
staticcheck ./...               # honnef.co/go/tools, pinned version
govulncheck ./...               # golang.org/x/vuln
goreleaser check                # github.com/goreleaser/goreleaser/v2, pinned version (go run …@version)
goreleaser release --snapshot --clean   # local release build into dist/, nothing published
```

## CI & security

- CI runs on `ubuntu-latest`, `macos-latest` and `windows-latest`: build, test (with `-race` where supported), vet, gofmt check, staticcheck, govulncheck, and the rules SHA-256 check against `SOURCE`. A separate job runs `goreleaser check` and a snapshot build.
- Releases: `release.yml` (spec §14) is the only workflow with write permissions. Never create or push tags; the owner does (see `RELEASING.md`).
- **Every action is pinned to a full commit SHA** with the version as a comment, e.g. `uses: actions/checkout@<40-char-sha> # v5.0.0`. The org requires this; unpinned actions fail.
- Only actions from GitHub or Marketplace verified creators are allowed. If a step needs another action, prefer running the tool directly (`go run …@version`) instead.
- Dependabot: `gomod` and `github-actions` ecosystems, weekly.
- Secret scanning, push protection, code scanning and Dependabot alerts are enabled at the org level. Locally, `gitleaks` as a pre-commit hook is recommended.

## Commits & PRs

- PR description: what changed, which spec section it implements, and how it was tested.

## Naming

- Product: `decrumb`, lowercase everywhere, including at the start of sentences.
- Organization brand: **Pinebench**; handle `pinebench`.
- Say "uses the ClearURLs rules", never anything that implies ClearURLs endorses or makes this tool.

## Definition of done (per module)

1. Matches the spec, no scope creep
2. Tests for main paths and relevant edge cases from spec §9, passing
3. CI green on all three operating systems
4. README and this file updated if anything changed
5. Summary posted and reviewed before moving on
