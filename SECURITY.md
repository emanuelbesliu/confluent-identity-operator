# Security Policy

## Supported versions

This project is pre-1.0. Security fixes are applied to the latest released
`0.x` version only.

| Version | Supported |
|---------|-----------|
| latest `0.x` | :white_check_mark: |
| older | :x: |

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report privately via GitHub's
[private vulnerability reporting](https://github.com/emanuelbesliu/confluent-identity-operator/security/advisories/new)
(Security tab → *Report a vulnerability*).

Please include:

- A description of the issue and its impact
- Steps to reproduce or a proof of concept
- Affected version(s) and configuration

You can expect an initial acknowledgement within a few days. Once confirmed, a
fix will be prepared and a coordinated disclosure / release arranged.

## Scope

This operator talks to the Confluent Cloud IAM API using an API key/secret and
manages identity pools and role bindings. Please pay particular attention to
issues that could lead to:

- Privilege escalation (creating unintended role bindings)
- Managing pools/bindings outside the configured `OWNED_PREFIX`
- Leaking Confluent Cloud credentials in logs or status

## Dependencies

Go module and GitHub Actions dependencies are monitored by Dependabot, and every
push/PR is scanned with `govulncheck` and CodeQL.

## Disclaimer

This is a personal, independent project and is **not** affiliated with, endorsed
by, or supported by Confluent, Inc. or any employer. It is provided "as is"
without warranty of any kind.
