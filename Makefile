CONTROLLER_GEN ?= $(shell go env GOPATH)/bin/controller-gen
IMG ?= confluent-identity-operator:latest

.PHONY: generate manifests build vet fmt test docker-build helm-lint

## generate: deepcopy code
generate:
	$(CONTROLLER_GEN) object paths=./api/...

## manifests: CRDs + RBAC
manifests:
	$(CONTROLLER_GEN) crd paths=./api/... output:crd:artifacts:config=config/crd/bases
	$(CONTROLLER_GEN) rbac:roleName=manager-role paths=./internal/... output:rbac:artifacts:config=config/rbac
	cp config/crd/bases/*.yaml charts/confluent-identity-operator/crds/

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

build: generate fmt vet
	go build -o bin/manager ./cmd

docker-build:
	docker build -t $(IMG) .

helm-lint:
	helm lint charts/confluent-identity-operator \
	  --set clusterName=ci --set confluent.identityProviderId=op-x \
	  --set image.repository=x/y --set confluent.credentials.existingSecret=s
