# confluent-identity-operator

A Kubernetes operator that auto-provisions **Confluent Cloud workload identity**
for workloads that authenticate to Confluent Cloud via **OAuth/OIDC**
(SASL_SSL / OAUTHBEARER) — replacing static, long-lived API keys with federated
identity.

It is designed for clusters where pods obtain OIDC tokens from a cloud identity
provider (e.g. federated managed identities / workload identity) and you want the
matching Confluent Cloud **identity pools** and **role bindings** to be created
declaratively from Kubernetes, reviewed through GitOps.

> Runs **one instance per cluster**, co-located with the workloads it serves.

## Why

Static Confluent Cloud API keys are hard to rotate, easy to leak, and decouple
access from workload identity. Confluent Cloud supports OAuth identity pools that
trust an external OIDC issuer and pin a token to a specific workload via a CEL
filter. This operator turns that setup into two small CRDs so app teams request
access through pull requests instead of ticket-driven key handouts.

## Model

| Layer | Resource | Meaning |
|-------|----------|---------|
| **Authentication** | `ConfluentIdentityPool` (rendered by the app's chart) | Creates a Confluent identity pool with **zero role bindings** — the workload can authenticate but every topic operation is `403`. |
| **Authorization** | `ConfluentRoleBinding` (lives in a GitOps permission repo) | Grants role bindings to the pool's principal. The **PR review is the approval gate**. |

A pool's CEL filter pins the token to exactly one workload, for example:

```
claims.aud == "<audience>" && claims.oid == "<principalId>" && claims.azp == "<clientId>"
```

## Layout

```
api/v1alpha1/            CRD types (ConfluentIdentityPool, ConfluentRoleBinding)
internal/confluent/      minimal Confluent Cloud IAM v2 REST client
internal/controller/     reconcilers for both CRDs
cmd/main.go              manager wiring + env config
config/crd/bases/        generated CRDs
config/samples/          example CRs
charts/                  per-cluster Helm chart
test/                    mock Confluent Cloud API + kind-based e2e
```

## Build

```sh
make generate manifests   # deepcopy + CRDs + RBAC (controller-gen)
make build                # go build
make test                 # go test ./...
make docker-build IMG=<registry>/confluent-identity-operator:0.1.0
```

## Container image

Released multi-arch images (`linux/amd64`, `linux/arm64`) are published to GitHub
Container Registry on every `v*` tag:

```
ghcr.io/emanuelbesliu/confluent-identity-operator:<version>   # e.g. 0.1.0
ghcr.io/emanuelbesliu/confluent-identity-operator:latest
```

The Helm chart is also pushed as an OCI artifact:

```sh
helm install cio oci://ghcr.io/emanuelbesliu/charts/confluent-identity-operator \
  --version 0.1.0 ...
```

## Deploy (per cluster)

```sh
helm install cio charts/confluent-identity-operator \
  --namespace confluent-operator --create-namespace \
  --set clusterName=<cluster-name> \
  --set confluent.identityProviderId=op-xxxx \
  --set confluent.audience=<audience-client-id> \
  --set image.repository=ghcr.io/emanuelbesliu/confluent-identity-operator \
  --set confluent.credentials.existingSecret=cc-operator-creds
```

`cc-operator-creds` is a `Secret` with keys `apiKey` / `apiSecret` holding an
org-scoped Confluent Cloud credential used for identity-pool and role-binding
CRUD. Manage it however you prefer (sealed-secrets, external-secrets, a CSI
secret store, or plain `kubectl`). For dev/PoC you can inline the values via
`confluent.credentials.apiKey` / `apiSecret`, but prefer `existingSecret` in real
environments.

## Operator configuration (env)

| Env | Required | Description |
|-----|----------|-------------|
| `CLUSTER_NAME` | yes | This cluster's name (pool naming / ownership). |
| `CONFLUENT_IDENTITY_PROVIDER_ID` | yes | Confluent identity provider id (`op-xxxx`). |
| `CONFLUENT_API_KEY` / `CONFLUENT_API_SECRET` | yes | Confluent Cloud credential. |
| `CONFLUENT_API_BASE` | no | Defaults to `https://api.confluent.cloud`. |
| `CONFLUENT_AUDIENCE` | no | Default `claims.aud` when a CR omits `spec.audience`. |
| `CONFLUENT_CRN_SCOPE` | no | Narrows role-binding lookups. |
| `LEADER_ELECT` | no | Defaults to `true`. |
| `DRY_RUN` | no | When `true`, every mutating Confluent Cloud call becomes a logged no-op. |
| `OWNED_PREFIX` | no | The operator only manages pools whose display name starts with this prefix. Empty = no restriction. |
| `LOG_LEVEL` | no | `debug`, `info`, or `error`. |

## Safety gates

When testing against a real Confluent Cloud org, set:

- `safety.dryRun=true` (Helm) / `DRY_RUN=true` — no mutations are sent.
- `safety.ownedPrefix=<prefix>` / `OWNED_PREFIX=<prefix>` — restrict the operator
  to pools it owns, so it never touches pools created by other tools.

## Testing

Every test phase runs against local tooling only — **no real Confluent Cloud
organization is contacted at any point**. All three phases run in CI on every
push and pull request (see `.github/workflows/ci.yml`).

| Phase | Command | What it does | External deps |
|-------|---------|--------------|---------------|
| Unit | `make test` (`go test ./... -race`) | Static analysis (`go vet`), race-enabled unit tests and a compile. | none |
| Chart lint | `make helm-lint` | Renders the Helm chart with required values. | none |
| End-to-end | `test/e2e/run.sh` | Spins up a `kind` cluster and an in-cluster **mock Confluent Cloud API**, installs the operator, applies sample CRs and asserts a pool + role binding were created. | `docker`, `kind`, `kubectl`, `helm` |

The mock Confluent Cloud API (`test/mockcc`) is an in-memory reimplementation of
just the IAM v2 identity-pool and role-binding endpoints the operator calls, so
the e2e path exercises the real controller reconcile loop without any external
account or credentials.

## License

[MIT](./LICENSE)

## Disclaimer

This is a personal, independent open-source project. It is **not** affiliated
with, endorsed by, or supported by Confluent, Inc. or any employer. "Confluent"
and "Confluent Cloud" are trademarks of Confluent, Inc. Provided "as is", without
warranty of any kind.
