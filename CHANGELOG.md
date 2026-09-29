# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Community health files: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`,
  `SECURITY.md`, issue/PR templates and `CODEOWNERS`.
- CI: dedicated `lint` (gofmt/tidy) and `govulncheck` jobs, plus a CodeQL
  code-scanning workflow.
- Dependabot for Go modules and GitHub Actions.
- `.golangci.yml` linter configuration.

### Changed
- Pin CI Go toolchain to `1.26.6` (patched standard library).

### Security
- Bumped `golang.org/x/net` to `v0.55.0` and `golang.org/x/text` to `v0.39.0`
  to resolve known vulnerabilities reported by `govulncheck`.

## [0.1.0] - 2026-09-29

### Added
- Initial release of the confluent-identity-operator.
- `ConfluentIdentityPool` and `ConfluentRoleBinding` CRDs (API group
  `confluentoauth.io`).
- Controllers that reconcile identity pools (authN) and role bindings (authZ)
  against the Confluent Cloud IAM v2 API.
- Helm chart, multi-arch container image and Helm chart published to GHCR.
- Safety gates: `DRY_RUN` and `OWNED_PREFIX`.
- Unit, chart-lint and kind-based end-to-end tests (against a mock Confluent
  Cloud API — no real org is contacted).

[Unreleased]: https://github.com/emanuelbesliu/confluent-identity-operator/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/emanuelbesliu/confluent-identity-operator/releases/tag/v0.1.0
