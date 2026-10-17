# decrumb

Remove tracking parameters from URLs, fully offline.

> **Status: in development.** Nothing below works yet except `--version`.

## Planned usage

```
decrumb [flags] [url ...]

Flags:
  --explain        show what was removed and why
  --json           output JSON instead of plain text
  --rules <file>   load rules from this file instead of the bundled ones
  --version        print version, bundled rules revision and attribution
```

Pass one or more URLs as arguments, or pipe them in on stdin, one per line.
decrumb prints one clean URL per line, in the same order.

```
$ decrumb 'https://example.com/page?utm_source=x&id=7'
https://example.com/page?id=7
```

## Rules & licensing

decrumb uses the [ClearURLs rules](https://github.com/ClearURLs/Rules) by the
ClearURLs project and contributors. The rules are licensed under LGPL-3.0
(see [COPYING.LESSER](COPYING.LESSER) and [COPYING](COPYING)) and are bundled
unmodified; the exact upstream commit is recorded in
[internal/rules/clearurls/SOURCE](internal/rules/clearurls/SOURCE). You can
supply your own rules file with `--rules <file>`. See [NOTICE](NOTICE).

decrumb also adds a few rules of its own in
[internal/rules/overrides.json](internal/rules/overrides.json) (MIT), each
with a link to the vendor documentation it's based on. They only add to the
ClearURLs rules, never change them, and aren't used with `--rules`.

decrumb is an independent project and is not endorsed by the ClearURLs project.

## License

- decrumb's own code: [MIT](LICENSE)
- Bundled ClearURLs rules: [LGPL-3.0](COPYING.LESSER)

A [Pinebench](https://github.com/pinebench) project.
