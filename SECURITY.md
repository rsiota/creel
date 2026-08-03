# Security Policy

## Reporting a vulnerability

`creel` handles database credentials, OS keychain entries, and SSH keys, so
security reports are taken seriously.

**Please do not open a public GitHub issue for security problems.** Instead,
use GitHub's private vulnerability reporting: open the
[Security tab](https://github.com/rsiota/creel/security) and choose
**Report a vulnerability**.

Please include:

- A description of the issue and its potential impact
- Steps to reproduce, or a proof of concept
- Affected versions — the output of `creel -version`

We aim to acknowledge reports within a few days and to coordinate a fix and
disclosure timeline with you before any public disclosure.

## Scope

This policy covers the `main` branch of [`rsiota/creel`](https://github.com/rsiota/creel),
including the bundled code (the pure-Go SQLite driver, keychain/secret storage,
and SSH tunnel handling). Vulnerabilities in third-party dependencies should be
reported to their upstream maintainers — though we welcome a heads-up so we can
track and bump the dependency.

## Supported versions

Only the latest release line receives security fixes.
