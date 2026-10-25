# Releasing decrumb

Releasing = pushing a `v*` tag on `main` (spec §14). The
[Release workflow](.github/workflows/release.yml) then does everything:
tests → **your approval** → build, upload to a draft release, attest,
publish → smoke test on Linux, macOS and Windows. Nothing is uploaded by hand.

Immutable releases are on, and the tag ruleset forbids moving or deleting
`v*` tags: **once a tag is pushed, that version number is used up.** That's
why minor and major releases go through release candidates first.

## Before tagging

- Everything for the release is merged, and CI on `main` is green (including
  the GoReleaser snapshot build).
- Pick the version (spec §14): patch `v0.1.x` for fixes and rules updates,
  minor `v0.x.0` for features or changed output/flags.

```sh
git checkout main
git pull --ff-only
git log --oneline -1   # the commit you are about to tag
```

## 1. Release candidate

Required before the first release and every minor/major; optional for
patches.

```sh
git tag -a v0.1.0-rc.1 -m "v0.1.0-rc.1"
git push origin v0.1.0-rc.1
```

1. **Actions → Release →** the run for the tag. Wait for the `test` jobs
   (the full CI suite on all three OSes).
2. **Approve the `release` environment:** the run shows "Waiting for
   review" → **Review deployments** → tick `release` → **Approve and
   deploy**. If `test` failed, nothing waits for approval and nothing is
   published.
3. Wait for `release` and the three `smoke` jobs to go green.
4. **Check the release page** (see below).
5. **Manual install test** if required (see below). For an RC that will become
   a minor/major release, do it on the RC.

If anything is wrong: fix it in a normal PR, merge, and tag `v0.1.0-rc.2` on
the new `main`. Never reuse a tag.

## 2. Final release

Tag the **same commit** as the RC that passed:

```sh
git tag -a v0.1.0 -m "v0.1.0" "v0.1.0-rc.1^{commit}"
git rev-parse "v0.1.0^{commit}" "v0.1.0-rc.1^{commit}"   # both lines must be equal
git push origin v0.1.0
```

Then the same steps: approve `release`, wait for `smoke`, check the release
page, write the intro, and do the manual install test if required.

A patch release without an RC is the same: tag `vX.Y.Z` on `main` and
follow these steps.

## Check the release page

On **github.com/pinebench/decrumb/releases**, for the new release:

- [ ] Title and tag are right; it is **published** (not a draft); an RC is
      marked **Pre-release**, a final release is **Latest**.
- [ ] It shows as **Immutable**.
- [ ] 11 files under **Assets**: 8 from our workflow, the six archives
      (`decrumb_<version>_{windows_amd64,windows_arm64}.zip`,
      `decrumb_<version>_{darwin,linux}_{amd64,arm64}.tar.gz`),
      `checksums.txt` and `decrumb_<version>_attestation.sigstore.json`,
      plus 3 that GitHub adds: "Source code (zip)", "Source code (tar.gz)"
      and "Release attestation (json)".
- [ ] The notes are grouped (Security, Added, Changed, Fixed, Rules & data,
      Other) and each line is in the right group. A breaking change must be
      under **Changed**.
- [ ] The attestations are listed under **Actions → Attestations** (or
      github.com/pinebench/decrumb/attestations).
- [ ] The `smoke` jobs passed on all three OSes (downloads, checksum,
      attestations, archive contents, `--version`, test cases).

## Hand-written intro

Edit the release (pencil icon) and replace the HTML comment at the top of the
notes with a short intro, keeping the generated groups below it:

- What the release is about, in one or two sentences.
- **Changed:** breaking changes, marked *breaking*, with what users must do.
- **Removed / Deprecated**, if anything.
- **Rules & data:** the bundled ClearURLs commit (from
  `internal/rules/clearurls/SOURCE`), overrides and descriptions added or
  removed.
- **Upgrade notes**, when needed.
- For a security release: a **Security** section first (what was affected,
  which versions, what users should do), and publish the GitHub Security
  Advisory at the same time.
- If a required manual install test was skipped: the reason.

Immutable releases lock the files and the tag; the title and notes stay
editable.

## Manual install test

**Required** for `v0.1.0`, every minor or major release, and any release that
changes the README's install or verify sections or `.goreleaser.yaml`.
Optional for other patches.

Follow **only the README**, without Go installed, on at least one clean
environment:

- an Ubuntu VirtualBox VM restored from a snapshot without Go,
- a **new Windows user account** (empty PATH, no Go), or
- another person's machine.

Go through: the install walkthrough for that system → "Verify your download"
(checksum and attestation, both must succeed) → `decrumb --version` shows the
release version → `decrumb "https://example.com/?utm_source=test"` prints
`https://example.com/`. Note anything in the README that was unclear or wrong
and fix it in a `docs:` PR.

One passing environment is enough. Skip it only in exceptional, unavoidable
cases, relying on the smoke test, and say why in the release notes.

## If something fails

- **`test` fails:** nothing was built or published. Fix on `main`, tag the
  next RC (or the next patch version).
- **`release` fails before publishing:** the release is still a draft. If the
  cause was temporary (e.g. a GitHub outage), **Re-run failed jobs**: the
  leftover draft is replaced. Otherwise delete the draft, fix on `main`, and
  tag the next version.
- **`smoke` fails after publishing:** the release is public and can't be
  changed. Mark it broken: add a warning at the very top of its notes (what
  is broken, which version to use instead) and, for a final release, untick
  **Set as the latest release** if an earlier working release exists. Then
  fix on `main` and tag the next RC or patch.
