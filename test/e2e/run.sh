#!/usr/bin/env bash
# End-to-end test for confluent-identity-operator against a kind cluster and an
# in-cluster mock Confluent Cloud API (no real CC org is touched).
#
# Prereqs: docker, kind, kubectl, helm.
set -euo pipefail

CLUSTER=cio-e2e
CTX=kind-${CLUSTER}
NS=confluent-operator
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

export DOCKER_BUILDKIT=1

echo "==> Build images"
docker build -t confluent-identity-operator:e2e .
docker build -f Dockerfile.mockcc -t mockcc:e2e .

echo "==> Create kind cluster"
kind get clusters | grep -q "^${CLUSTER}$" || kind create cluster --name "$CLUSTER" --wait 90s
kind load docker-image confluent-identity-operator:e2e mockcc:e2e --name "$CLUSTER"

echo "==> Deploy mock Confluent Cloud"
kubectl --context "$CTX" create namespace "$NS" --dry-run=client -o yaml | kubectl --context "$CTX" apply -f -
kubectl --context "$CTX" apply -f test/e2e/mockcc.yaml
kubectl --context "$CTX" -n "$NS" rollout status deploy/mockcc --timeout=90s

echo "==> Install operator chart"
helm --kube-context "$CTX" upgrade --install cio charts/confluent-identity-operator \
  --namespace "$NS" \
  --set clusterName=aks-e2e-cluster \
  --set image.repository=confluent-identity-operator --set image.tag=e2e --set image.pullPolicy=IfNotPresent \
  --set confluent.apiBase=http://mockcc.${NS}.svc.cluster.local \
  --set confluent.identityProviderId=op-e2e123 \
  --set confluent.audience=aud-11111111 \
  --set confluent.credentials.apiKey=test-key --set confluent.credentials.apiSecret=test-secret \
  --set leaderElection=false
kubectl --context "$CTX" -n "$NS" rollout status deploy/cio-confluent-identity-operator --timeout=90s

echo "==> Apply workload identity ConfigMap + ConfluentIdentityPool + ConfluentRoleBinding"
kubectl --context "$CTX" create namespace orders --dry-run=client -o yaml | kubectl --context "$CTX" apply -f -
kubectl --context "$CTX" -n orders apply -f - <<'EOF'
apiVersion: v1
kind: ConfigMap
metadata:
  name: orders-service-identity-config
data:
  clientId: "cccccccc-1111-2222-3333-444444444444"
  principalId: "oooooooo-5555-6666-7777-888888888888"
---
apiVersion: confluentoauth.io/v1alpha1
kind: ConfluentIdentityPool
metadata:
  name: orders-service
spec:
  identityConfigRef:
    name: orders-service-identity-config
  audience: "aud-11111111"
  displayName: "aks-e2e-cluster-orders-orders-service"
---
apiVersion: confluentoauth.io/v1alpha1
kind: ConfluentRoleBinding
metadata:
  name: orders-service-read
spec:
  poolRef:
    name: orders-service
  bindings:
    - roleName: DeveloperRead
      crnPattern: "crn://confluent.cloud/kafka=lkc-xxx/topic=orders-*"
EOF

echo "==> Assert pool + binding exist in mock CC"
kubectl --context "$CTX" -n "$NS" port-forward svc/mockcc 18080:80 >/tmp/cio-pf.log 2>&1 &
PF_PID=$!
trap 'kill "$PF_PID" 2>/dev/null || true' EXIT
sleep 3

# The role-binding controller requeues after 30s while it waits for the pool's
# status.poolId to be populated, so poll (up to ~2min) instead of assuming a
# fixed delay.
STATE=""
for i in $(seq 1 40); do
  STATE=$(curl -s http://localhost:18080/_debug/state)
  if echo "$STATE" | python3 -c 'import sys,json; d=json.load(sys.stdin); sys.exit(0 if len(d["pools"])==1 and len(d["roleBindings"])==1 else 1)' 2>/dev/null; then
    break
  fi
  sleep 3
done

echo "$STATE" | python3 -m json.tool
echo "$STATE" | python3 -c 'import sys,json; d=json.load(sys.stdin); assert len(d["pools"])==1, "expected 1 pool"; assert len(d["roleBindings"])==1, "expected 1 binding"; print("ASSERT OK: 1 pool, 1 role binding")'

echo "==> E2E PASSED"
