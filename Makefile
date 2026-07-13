BINARY     := fitcheck
IMAGE      := ghcr.io/reyshazni/fitcheck
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE       := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
  -X github.com/reyshazni/fitcheck/internal/version.buildVersion=$(VERSION) \
  -X github.com/reyshazni/fitcheck/internal/version.buildCommit=$(COMMIT) \
  -X github.com/reyshazni/fitcheck/internal/version.buildDate=$(DATE)

ENVTEST_ASSETS ?= $(shell setup-envtest use -p path 2>/dev/null)

.PHONY: build test lint run docker-build setup-envtest helm-lint verify fmt vet

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/

test: setup-envtest
	KUBEBUILDER_ASSETS="$(ENVTEST_ASSETS)" go test -race -coverprofile=coverage.out ./...

lint:
	go tool golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

run: build
	./bin/$(BINARY)

docker-build:
	docker build -t $(IMAGE):$(VERSION) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) .

setup-envtest:
	@which setup-envtest > /dev/null 2>&1 || \
		go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.22

helm-lint:
	helm lint charts/fitcheck/
	helm template fitcheck charts/fitcheck/ -n kube-system > /dev/null

verify: fmt vet lint test helm-lint
	@echo "All checks passed."
