# decrumb

Remove tracking parameters from URLs, fully offline.

decrumb is site-aware: it uses the ClearURLs rules, so a parameter is removed
only where it is known to be tracking, and unknown parameters are kept.
Redirect links such as `google.com/url?q=…` are unwrapped offline. decrumb
never opens a network connection and never writes anything to disk.

## Install

There are two ways to install decrumb:

- **[Download a release](#option-1-download-a-release)**: a ready-made
  program for Windows, macOS or Linux. You don't need anything else.
- **[`go install`](#option-2-go-install-for-developers-with-go)**: for
  developers who already have Go; builds decrumb from source.

The examples use version `v0.1.0`. For a newer release, replace `0.1.0` with
its version number everywhere.

### Option 1: Download a release

Open the [latest release](https://github.com/pinebench/decrumb/releases/latest).
On that page, scroll down to **Assets** and download only the one archive for
your system from this table:

| System | Archive |
|---|---|
| Windows, most PCs (Intel/AMD, "x64") | `decrumb_0.1.0_windows_amd64.zip` |
| Windows on Arm (e.g. Snapdragon laptops) | `decrumb_0.1.0_windows_arm64.zip` |
| macOS, Apple Silicon (M1 and later) | `decrumb_0.1.0_darwin_arm64.tar.gz` |
| macOS, Intel | `decrumb_0.1.0_darwin_amd64.tar.gz` |
| Linux, most PCs and servers (x86-64) | `decrumb_0.1.0_linux_amd64.tar.gz` |
| Linux on Arm (e.g. Raspberry Pi 4/5 with a 64-bit OS) | `decrumb_0.1.0_linux_arm64.tar.gz` |

Each archive contains the `decrumb` program, `README.md`, the license files
(`LICENSE`, `NOTICE`, `COPYING.LESSER`, `COPYING`) and the bundled ClearURLs
rules as `clearurls/data.min.json`. `checksums.txt` lists the SHA-256 of every
archive.

You don't need the other files under Assets:

- `checksums.txt` and `decrumb_0.1.0_attestation.sigstore.json` are only
  needed to [verify your download](#verify-your-download) (recommended).
- "Source code (zip)", "Source code (tar.gz)" and "Release attestation" are
  added by GitHub to every release. You don't need them.

Release candidates (test versions, marked "Pre-release") have `-rc.N` in their
file names, e.g. `decrumb_0.1.0-rc.1_linux_amd64.tar.gz`. You never need them.

#### Windows (PowerShell)

1. **Check your system.** Open PowerShell and run:

   ```powershell
   $env:PROCESSOR_ARCHITECTURE
   ```

   `AMD64` means use the `windows_amd64` archive; `ARM64` means use
   `windows_arm64` (replace `amd64` with `arm64` in the commands below).

2. **Download** the archive and `checksums.txt` into your Downloads folder:

   ```powershell
   cd $HOME\Downloads
   Invoke-WebRequest https://github.com/pinebench/decrumb/releases/download/v0.1.0/decrumb_0.1.0_windows_amd64.zip -OutFile decrumb_0.1.0_windows_amd64.zip
   Invoke-WebRequest https://github.com/pinebench/decrumb/releases/download/v0.1.0/checksums.txt -OutFile checksums.txt
   ```

   You can also download both files in your browser, from Assets on the
   [latest release](https://github.com/pinebench/decrumb/releases/latest).

3. **Verify the download** before you run anything from it: see
   [Verify your download](#verify-your-download).

4. **Extract** the archive:

   ```powershell
   Expand-Archive decrumb_0.1.0_windows_amd64.zip -DestinationPath decrumb_0.1.0
   ```

5. **Move `decrumb.exe`** to a folder for your own programs,
   `%USERPROFILE%\bin` (in PowerShell: `$HOME\bin`):

   ```powershell
   New-Item -ItemType Directory -Force "$HOME\bin" | Out-Null
   Move-Item decrumb_0.1.0\decrumb.exe "$HOME\bin\" -Force
   ```

6. **Add that folder to your PATH** (only needed once; only for your user
   account, no administrator rights needed):

   ```powershell
   $bin = "$HOME\bin"
   $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
   if (($userPath -split ";") -notcontains $bin) {
       $newPath = (@($userPath, $bin) | Where-Object { $_ }) -join ";"
       [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
   }
   ```

   Or without commands: open Start, search for **"Edit environment variables
   for your account"**, select **Path**, click **Edit → New**, enter
   `%USERPROFILE%\bin`, and click **OK** twice.

7. **Open a new PowerShell window** (the PATH change only applies to new
   windows) and check that it works:

   ```powershell
   decrumb --version
   ```

   The first line should read `decrumb v0.1.0`. If Windows shows a blue
   "Windows protected your PC" window instead, see
   [Security warnings on first run](#security-warnings-on-first-run).

8. **First test:**

   ```powershell
   decrumb "https://example.com/?utm_source=test"
   ```

   It prints `https://example.com/`.

#### macOS

1. **Check your Mac.** In Terminal, run `uname -m`. `arm64` means Apple
   Silicon: use the `darwin_arm64` archive. `x86_64` means Intel: use
   `darwin_amd64` (replace `arm64` with `amd64` in the commands below).

2. **Download** the archive and `checksums.txt`:

   ```sh
   cd ~/Downloads
   curl -LO https://github.com/pinebench/decrumb/releases/download/v0.1.0/decrumb_0.1.0_darwin_arm64.tar.gz
   curl -LO https://github.com/pinebench/decrumb/releases/download/v0.1.0/checksums.txt
   ```

   You can also download both files in your browser, from Assets on the
   [latest release](https://github.com/pinebench/decrumb/releases/latest).

3. **Verify the download** before you run anything from it: see
   [Verify your download](#verify-your-download).

4. **Extract** the archive:

   ```sh
   mkdir decrumb_0.1.0
   tar -xzf decrumb_0.1.0_darwin_arm64.tar.gz -C decrumb_0.1.0
   ```

5. **Clear the quarantine flag.** macOS marks files downloaded with a browser
   as "quarantined", and Gatekeeper then blocks unsigned programs such as
   decrumb (see [Security warnings on first run](#security-warnings-on-first-run)).
   After verifying the download, remove the flag:

   ```sh
   xattr -d com.apple.quarantine decrumb_0.1.0/decrumb 2>/dev/null || true
   ```

   (Files downloaded with `curl` aren't quarantined; then this does nothing.)

6. **Move `decrumb`** onto your PATH. Either for all users, in
   `/usr/local/bin` (asks for your password):

   ```sh
   sudo mkdir -p /usr/local/bin
   sudo mv decrumb_0.1.0/decrumb /usr/local/bin/
   ```

   or only for you, in `~/bin`:

   ```sh
   mkdir -p ~/bin
   mv decrumb_0.1.0/decrumb ~/bin/
   echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
   ```

   With `~/bin`, open a new Terminal window afterwards.

7. **Check that it works:**

   ```sh
   decrumb --version
   ```

   The first line should read `decrumb v0.1.0`.

8. **First test:**

   ```sh
   decrumb "https://example.com/?utm_source=test"
   ```

   It prints `https://example.com/`.

#### Linux

1. **Check your system:** run `uname -m`. `x86_64` means use the
   `linux_amd64` archive; `aarch64` means use `linux_arm64` (replace `amd64`
   with `arm64` in the commands below).

2. **Download** the archive and `checksums.txt`:

   ```sh
   cd ~/Downloads 2>/dev/null || cd ~
   curl -LO https://github.com/pinebench/decrumb/releases/download/v0.1.0/decrumb_0.1.0_linux_amd64.tar.gz
   curl -LO https://github.com/pinebench/decrumb/releases/download/v0.1.0/checksums.txt
   ```

   (No `curl`? Use `wget` with the same URLs.)

   You can also download both files in your browser, from Assets on the
   [latest release](https://github.com/pinebench/decrumb/releases/latest).

3. **Verify the download** before you run anything from it: see
   [Verify your download](#verify-your-download).

4. **Extract** the archive:

   ```sh
   mkdir decrumb_0.1.0
   tar -xzf decrumb_0.1.0_linux_amd64.tar.gz -C decrumb_0.1.0
   ```

5. **Move `decrumb`** to `~/.local/bin`:

   ```sh
   mkdir -p ~/.local/bin
   mv decrumb_0.1.0/decrumb ~/.local/bin/
   ```

6. **Check that it works:**

   ```sh
   decrumb --version
   ```

   The first line should read `decrumb v0.1.0`. If you get
   `decrumb: command not found`, `~/.local/bin` isn't on your PATH yet. Many
   distributions add it automatically at your next login; otherwise add it
   yourself and open a new terminal:

   ```sh
   echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
   ```

   (For zsh, use `~/.zshrc` instead of `~/.bashrc`.)

7. **First test:**

   ```sh
   decrumb "https://example.com/?utm_source=test"
   ```

   It prints `https://example.com/`.

### Option 2: `go install` (for developers with Go)

1. **Check your Go version:**

   ```sh
   go version
   ```

   decrumb needs Go 1.27.3 or newer. (Go 1.21 and later download the needed
   version automatically.) No Go yet? Use
   [Option 1](#option-1-download-a-release), or install Go from
   [go.dev/dl](https://go.dev/dl/).

2. **Build and install:**

   ```sh
   go install github.com/pinebench/decrumb/cmd/decrumb@v0.1.0
   ```

   Go downloads the source, checks it against Go's public checksum database,
   and builds `decrumb` on your machine.

3. **Where it lands:** in Go's `bin` folder, `$(go env GOPATH)/bin` (or
   `$(go env GOBIN)` if you've set that). By default that's `~/go/bin` on
   macOS and Linux, and `%USERPROFILE%\go\bin` on Windows. To see the exact
   folder:

   ```sh
   go env GOPATH
   ```

   (then add `/bin`, or `\bin` on Windows).

4. **Put it on your PATH**, if it isn't already (the Windows Go installer
   usually adds it for you):

   - macOS (zsh): `echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc`
   - Linux (bash): `echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.bashrc`
   - Windows: add `%USERPROFILE%\go\bin` to your Path as in
     [step 6 of the Windows walkthrough](#windows-powershell), replacing
     `$HOME\bin` with `$HOME\go\bin`.

   Then open a new terminal.

5. **Check that it works:**

   ```sh
   decrumb --version
   ```

   The first line should read `decrumb v0.1.0`, the same as a release
   download; the rules line below it matches the release too.

6. **First test:**

   ```sh
   decrumb "https://example.com/?utm_source=test"
   ```

   It prints `https://example.com/`.

You don't need the next two sections for `go install`: Go verifies the source
itself, and a program you built yourself doesn't trigger SmartScreen or
Gatekeeper.

## Verify your download

Two checks, both recommended before you run a downloaded decrumb:

1. **Checksum:** the archive's SHA-256 matches the one in `checksums.txt`, so
   the file wasn't damaged or changed on the way.
2. **Attestation:** GitHub confirms that the archive and `checksums.txt` were
   built by decrumb's own release workflow in
   [pinebench/decrumb](https://github.com/pinebench/decrumb). This also
   catches a swapped `checksums.txt`.

Run both in the folder you downloaded to, with your archive's name.

### 1. Checksum

**Windows (PowerShell):**

```powershell
$file = "decrumb_0.1.0_windows_amd64.zip"
$expected = ((Get-Content checksums.txt) -match "  $([regex]::Escape($file))$" -split "\s+")[0]
$actual = (Get-FileHash $file -Algorithm SHA256).Hash.ToLower()
if ($expected -and $expected -eq $actual) { "OK: checksum matches" } else { "FAILED: checksum does not match" }
```

**macOS:**

```sh
grep " decrumb_0.1.0_darwin_arm64.tar.gz$" checksums.txt | shasum -a 256 -c
```

**Linux:**

```sh
grep " decrumb_0.1.0_linux_amd64.tar.gz$" checksums.txt | sha256sum -c
```

**Good result:** `OK: checksum matches` (Windows) or a line ending in `: OK`,
e.g. `decrumb_0.1.0_linux_amd64.tar.gz: OK` (macOS, Linux).

**Bad result:** `FAILED: checksum does not match` (Windows), or `FAILED` and
`WARNING: 1 computed checksum did NOT match` (macOS, Linux). No output on
macOS/Linux means the file name wasn't found in `checksums.txt`; check the
spelling. Either way: don't run the file; delete it and download it again
from Assets on the
[latest release](https://github.com/pinebench/decrumb/releases/latest). If
it still fails, please
[report it privately](https://github.com/pinebench/decrumb/security).

### 2. Attestation

This needs the [GitHub CLI](https://cli.github.com/) (`gh`), signed in once
with `gh auth login`. The commands are the same on Windows, macOS and Linux:

```sh
gh attestation verify decrumb_0.1.0_linux_amd64.tar.gz -o pinebench
gh attestation verify checksums.txt -o pinebench
```

(Use your archive's name in the first line.)

**Good result,** for each file:

```text
✓ Verification succeeded!

The following 1 attestation matched the policy criteria

- Attestation #1
  - Build repo:..... pinebench/decrumb
  - Build workflow:. .github/workflows/release.yml@refs/tags/v0.1.0
  ...
```

Check that **Build repo** is `pinebench/decrumb` and the workflow ends in
`@refs/tags/v0.1.0`, the version you downloaded.

**Bad result:** a message that verification failed, or that no attestations
were found for the file. Don't run it: the file wasn't built by decrumb's
release workflow. Delete it, download it again from Assets on the
[latest release](https://github.com/pinebench/decrumb/releases/latest), and if it
still fails, please [report it privately](https://github.com/pinebench/decrumb/security).

Each release also contains the attestation file itself,
`decrumb_0.1.0_attestation.sigstore.json`, for checking without asking
GitHub's API: add `--bundle decrumb_0.1.0_attestation.sigstore.json` to the
commands above.

## Security warnings on first run

decrumb's release builds aren't signed with a paid Microsoft or Apple
certificate yet, so Windows and macOS may warn you the first time you run a
**downloaded** copy. Verify your download first (above); if both checks pass,
it's the genuine release and you can continue as described here.
[`go install`](#option-2-go-install-for-developers-with-go) builds decrumb on
your own machine and avoids these warnings entirely.

### Windows SmartScreen

**What you see:** a blue window, "Windows protected your PC: Microsoft
Defender SmartScreen prevented an unrecognized app from starting", with only a
**Don't run** button. This mostly happens with files downloaded in a browser.

**How to proceed:** click **More info**, check that the app is `decrumb.exe`
(the publisher shows as "Unknown publisher"), then click **Run anyway**.
Windows remembers this. Or, before the first run, remove the "downloaded from
the internet" mark in PowerShell:

```powershell
Unblock-File "$HOME\bin\decrumb.exe"
```

### macOS Gatekeeper

**What you see:** a dialog saying that "decrumb" can't be opened because
Apple can't check it for malicious software, or that the developer can't be
verified, with buttons such as **Done** and **Move to Trash**. This happens
when the file still has the quarantine flag from a browser download.

**How to proceed:** don't click **Move to Trash**. Click **Done**, then
either remove the quarantine flag as in
[step 5 of the macOS walkthrough](#macos) (with the path where decrumb is
now, e.g. `xattr -d com.apple.quarantine /usr/local/bin/decrumb`), or open
**System Settings → Privacy & Security**, scroll down to the message about
"decrumb", click **Open Anyway**, and confirm. Then run decrumb again.

## Usage

```
decrumb [flags] [url ...] (flags may appear anywhere)

Flags:
  --explain        show what was removed and why
  --json           output JSON instead of plain text
  --rules <file>   load rules from this file instead of the bundled ones
  --version        print version, bundled rules revision and attribution
  -h, --help       show this help
```

Pass one or more URLs as arguments, or pipe them in on stdin, one per line.
decrumb prints one clean URL per line, in the same order. Flags may come
before, between or after the URLs (`decrumb URL --json` works); after `--`,
every argument is a URL, even one that starts with `-`.

```
$ decrumb 'https://example.com/page?utm_source=newsletter&id=7'
https://example.com/page?id=7

$ decrumb 'https://www.google.com/url?q=https%3A%2F%2Fexample.com%2Farticle%3Futm_source%3Dgoogle%26id%3D7&sa=D&ved=2ahUKE'
https://example.com/article?id=7
```

Quote URLs in the shell: `&` and `?` mean something to it.

### From stdin

```
$ cat links.txt
https://www.amazon.com/dp/B000000000?tag=affiliate-20&th=1&psc=1

example.com/?fbclid=IwAR0abc
http://
$ decrumb < links.txt
https://www.amazon.com/dp/B000000000?psc=1
https://example.com/
decrumb: line 4: invalid URL "http://": no host
http://
```

- Empty and whitespace-only lines are skipped. Windows line endings (`\r\n`)
  are fine; output always uses `\n`. A UTF-8 byte order mark at the very
  start of stdin (added by Windows PowerShell 5.1 and older Notepad) is
  ignored.
- A URL without a scheme gets `https://`.
- An invalid line is passed through unchanged (trimmed) and reported on
  stderr with its line number and the reason, e.g.
  `decrumb: line 3: invalid URL "not a url ::": spaces are not allowed in the host`,
  so output lines stay aligned with input lines.
- A host containing invisible characters (zero-width spaces, soft hyphens, a
  byte order mark) is invalid: such lookalike domains are a phishing trick,
  so decrumb warns instead of handing the link back as clean. International
  domain names such as `пример.рф` are fine. Ports must be 0–65535.
- Lines longer than 2 MB (Chrome's maximum URL length) are reported on stderr
  and get an empty output line; reading continues.
- Non-web URLs such as `mailto:` are passed through unchanged.

### `--explain`

```
$ decrumb --explain 'https://www.amazon.com/Some-Book/dp/B000000000/ref=sr_1_1?qid=1700000000&tag=affiliate-20'
https://www.amazon.com/Some-Book/dp/B000000000
  removed    /ref=sr_1_1  amazon  (raw rule)
  removed    qid          amazon  Amazon search timestamp: undocumented; appears to record when the search that led here ran.
  removed    tag          amazon  Amazon Associates tracking ID: credits the affiliate who shared the link for purchases.
```

Under each URL: every redirect that was unwrapped, then every removed
parameter with the rules provider that removed it and, for 60 common tracking
parameters, a short description. `(raw rule)` marks text removed by a rule
that matches the URL itself rather than a parameter, and `(in fragment)` a
parameter removed after the `#`. A redirect chain is followed for at most 5
hops; a note says when that limit was reached.

### `--json`

```
$ decrumb --json 'https://example.com/page?utm_source=x&id=7'
{"results":[
{"input":"https://example.com/page?utm_source=x&id=7","output":"https://example.com/page?id=7","changed":true,"unwrapped":[],"removed":[{"param":"utm_source","provider":"globalRules","location":"query","description":"Campaign source: names the site, newsletter or ad network that sent you."}],"hop_limit_reached":false,"error":null}
]}
```

One JSON document per run, with one entry per input line, in order:

| Field | Meaning |
|---|---|
| `input` | the line as given |
| `output` | the clean URL, or `null` if the input was invalid |
| `changed` | `true` if something was removed or unwrapped (adding `https://` alone doesn't count) |
| `unwrapped` | the URL reached by each redirect hop |
| `removed` | `param`, `provider`, `location` (`query`, `fragment` or `rawRule`) and, when there is one, `description` |
| `hop_limit_reached` | `true` if the 5-hop redirect limit was reached |
| `error` | why the input is invalid, or `null` |

The document is streamed: each result is written as soon as it is cleaned, so
long inputs use constant memory. `--json` always includes everything;
`--explain` has no effect on it.

### `--rules <file>`

```
$ decrumb --rules my-rules.json 'https://example.com/?utm_source=x'
```

Uses your rules file, in the same format as the
[ClearURLs rules](https://github.com/ClearURLs/Rules), instead of the bundled
rules. It is used exactly as given: decrumb's own additions
(below) are not applied. If the file can't be loaded, decrumb says why and
cleans nothing:

```
$ decrumb --rules my-rules.json 'https://example.com/'
decrumb: my-rules.json: provider "p": "rules" must be a list of strings
```

### Exit codes

| Code | Meaning |
|---|---|
| `0` | success |
| `1` | at least one invalid input line (everything else is still cleaned) |
| `2` | usage error: unknown flag, missing flag value, or no URLs and nothing piped in |
| `3` | the rules could not be loaded (nothing is cleaned) |

## Rules & licensing

decrumb uses the [ClearURLs rules](https://github.com/ClearURLs/Rules) by the
ClearURLs project and contributors. The rules are licensed under LGPL-3.0
(see [COPYING.LESSER](COPYING.LESSER) and [COPYING](COPYING)) and are bundled
unmodified; the exact upstream commit is recorded in
[internal/rules/clearurls/SOURCE](internal/rules/clearurls/SOURCE) and shown
by `decrumb --version`. You can supply your own rules file with
`--rules <file>`. See [NOTICE](NOTICE).

decrumb also adds a few rules of its own in
[internal/rules/overrides.json](internal/rules/overrides.json) (MIT), each
with a link to the vendor documentation it's based on. They only add to the
ClearURLs rules, never change them, and aren't used with `--rules`.

decrumb is an independent project and is not endorsed by the ClearURLs project.

## License

- decrumb's own code: [MIT](LICENSE)
- Bundled ClearURLs rules: [LGPL-3.0](COPYING.LESSER)

A [Pinebench](https://github.com/pinebench) project.
