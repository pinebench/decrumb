# Security policy

## Reporting a vulnerability

Please report security problems **privately**, not in a public issue:

1. Go to the [Security tab](https://github.com/pinebench/decrumb/security) of this repository.
2. Click **Report a vulnerability** and fill in the form.

Only the maintainer can see the report. Helpful details: the decrumb version
(`decrumb --version`), your operating system, the exact input, what decrumb
did and what you expected. Examples of security problems: a crafted URL that
makes decrumb output a different or lookalike host, unwrap a redirect to a
non-web scheme, hang, or crash.

If you've already posted a problem publicly by mistake, that's fine: we'll
move it to a private advisory and edit out the details.

## Supported versions

Below version 1.0, only the **latest release** receives security fixes.
Please upgrade to it before reporting, if you can.

| Version | Supported |
|---|---|
| Latest release | Yes |
| Older releases | No |

## What happens next

- You'll get an acknowledgement **within 7 days**.
- We'll work on the fix privately and keep you updated in the advisory.
- The fix ships as **its own release**, as soon as it's ready, together with a
  published **GitHub Security Advisory** describing what was affected, which
  versions, and what to do (with a CVE when appropriate). Advisories feed Go's
  vulnerability database, so `govulncheck` warns users of affected versions.
