# Contributing

Thanks for your interest in improving **confluent-identity-operator**! This is a
personal, independent open-source project and contributions of all kinds are
welcome: bug reports, feature requests, documentation and code.

## Ground rules

- Be respectful. This project follows the [Code of Conduct](./CODE_OF_CONDUCT.md).
- Keep pull requests focused and small where possible.
- Discuss large or breaking changes in an issue first.

## Development environment

You need:

- Go (see the version in [`go.mod`](./go.mod))
- `docker`, `kind`, `kubectl` and `helm` (for the end-to-end test)
- `make`

Common tasks:

```bash
make generate manifests   # regenerate deepcopy, CRDs and RBAC (controller-gen)
make build                # compile
make vet                  # go vet
make test                 # unit tests (go test ./...)
make helm-lint            # lint the Helm chart
bash test/e2e/run.sh      # full e2e against kind + a mock Confluent Cloud API
```

## Testing

Every test phase runs against local tooling only and **never contacts a real
Confluent Cloud organization**:

| Phase | Command | External deps |
|-------|---------|---------------|
| Unit | `make test` | none |
| Chart lint | `make helm-lint` | none |
| End-to-end | `bash test/e2e/run.sh` | `docker`, `kind`, `kubectl`, `helm` |

All three phases also run in CI on every pull request.

## Commit messages

Please use [Conventional Commits](https://www.conventionalcommits.org/), e.g.:

```
feat(controller): requeue role bindings until the pool id is known
fix(chart): default imagePullPolicy to IfNotPresent
docs: clarify the OAuth trust setup
```

## Developer Certificate of Origin (DCO)

By contributing you certify the [DCO](https://developercertificate.org/). Sign
off every commit with `-s`:

```bash
git commit -s -m "fix: ..."
```

This adds a `Signed-off-by:` trailer with your name and email.

## Pull request checklist

- [ ] `make vet test` passes
- [ ] `gofmt` clean (`gofmt -l .` prints nothing)
- [ ] CRDs/RBAC regenerated if you changed `api/` (`make manifests`)
- [ ] Docs / `README.md` updated if behaviour changed
- [ ] `CHANGELOG.md` updated under `Unreleased`
- [ ] Commits are signed off (DCO)

## Releasing (maintainers)

Releases are cut by pushing a `vX.Y.Z` tag. The release workflow builds and
pushes a multi-arch image and the Helm chart to GHCR and creates a GitHub
Release. Remember to move the `CHANGELOG.md` `Unreleased` section under the new
version first.
