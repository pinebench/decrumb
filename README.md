# decrumb

Remove tracking parameters from URLs, fully offline.

decrumb is site-aware: it uses the ClearURLs rules, so a parameter is removed
only where it is known to be tracking, and unknown parameters are kept.
Redirect links such as `google.com/url?q=…` are unwrapped offline. decrumb
never opens a network connection and never writes anything to disk.

## Install

From source, with [Go](https://go.dev/dl/) installed:

```
go install github.com/pinebench/decrumb/cmd/decrumb@latest
```

This puts `decrumb` in `$(go env GOPATH)/bin`. Prebuilt releases will follow.

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
