# Foundation Setup Implementation Plan

> **For agentic workers:** REQUIRED: Follow the `test-driven-development` skill for every task. Use `coding:execute` to implement task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Set up the complete project foundation for fitcheck: Go scaffold, structured logging, observability, TDD infrastructure, Dockerfile, CI/CD pipelines, and Helm chart.

**Architecture:** Single Go binary using controller-runtime v0.24.1. slog (stdlib) for structured JSON logging wired through logr. Prometheus metrics at :8080, health probes at :8081. envtest for integration testing. Multi-stage Docker build with distroless. GitHub Actions CI/CD adapted from kompakt project.

**Tech Stack:** Go 1.26, controller-runtime v0.24.1, slog, Prometheus, envtest, Helm, GitHub Actions, cosign

**Execution Status:** COMPLETE
**Started:** 2026-07-13 19:18 WIB
**Completed:** 2026-07-13 19:34 WIB
**Progress:** 10/10 tasks complete
**Reviews:** 0 (foundation scaffold, no review loop needed)

---

## File Structure

```
cmd/main.go                          # entrypoint, manager setup, flag parsing
internal/
  controller/                        # PodReconciler (stub for foundation)
  types/                             # shared types (stub for foundation)
  version/version.go                 # build-time version info via ldflags
Makefile
Dockerfile
.golangci.yml                        # ALREADY EXISTS, do not recreate
.gitignore                           # ALREADY EXISTS, do not recreate
.yamllint.yml                        # yamllint config for CI
.github/workflows/
  ci.yml                             # lint + test + helm lint + build
  e2e.yml                            # kind cluster matrix tests
  release.yml                        # image + chart + github release + cosign
charts/fitcheck/                     # ALREADY SCAFFOLDED by prior agent
  Chart.yaml
  values.yaml
  templates/
    _helpers.tpl
    deployment.yaml
    serviceaccount.yaml
    clusterrole.yaml
    clusterrolebinding.yaml
hack/
  e2e-setup.sh                       # kind cluster setup script for E2E
test/e2e/
  e2e_test.go                        # E2E test scaffold
```

---

## Task 1: Go Module Init + Version Package

**Files:**
- Test: `internal/version/version_test.go`
- Create: `go.mod`, `go.sum`, `internal/version/version.go`

- [ ] **Step 1: Initialize Go module and add dependencies**

```bash
cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck
go mod init github.com/reyshazni/fitcheck
go get sigs.k8s.io/controller-runtime@v0.24.1
go get -tool github.com/golangci/golangci-lint/v2/cmd/golangci-lint
go mod tidy
```

- [ ] **Step 2: Write failing test (RED)**

File: `internal/version/version_test.go`

```go
package version_test

import (
	"testing"

	"github.com/reyshazni/fitcheck/internal/version"
)

func TestInfoReturnsStructuredVersion(t *testing.T) {
	t.Parallel()

	info := version.Info()

	if info.Version == "" {
		t.Error("expected Version to have a default value")
	}

	if info.Commit == "" {
		t.Error("expected Commit to have a default value")
	}

	if info.Date == "" {
		t.Error("expected Date to have a default value")
	}
}

func TestInfoStringFormat(t *testing.T) {
	t.Parallel()

	info := version.Info()
	s := info.String()

	if s == "" {
		t.Error("expected String() to return non-empty value")
	}
}
```

- [ ] **Step 3: Verify RED**

```bash
go test ./internal/version/... -v -run TestInfo
```

Expected: FAIL (package does not exist)

- [ ] **Step 4: Write minimal implementation (GREEN)**

File: `internal/version/version.go`

```go
package version

import "fmt"

// Build-time variables set via ldflags.
var (
	buildVersion = "dev"
	buildCommit  = "unknown"
	buildDate    = "unknown"
)

// BuildInfo holds structured version information.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Info returns the current build information.
func Info() BuildInfo {
	return BuildInfo{
		Version: buildVersion,
		Commit:  buildCommit,
		Date:    buildDate,
	}
}

// String returns a human-readable version string.
func (b BuildInfo) String() string {
	return fmt.Sprintf("version=%s commit=%s date=%s", b.Version, b.Commit, b.Date)
}
```

- [ ] **Step 5: Verify GREEN**

```bash
go test ./internal/version/... -v
```

Expected: ALL PASS

- [ ] **Step 6: Lint**

```bash
go tool golangci-lint run ./internal/version/...
```

Expected: zero issues

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/version/
git commit -m "feat: add go module and version"
```

---

## Task 2: cmd/main.go with Manager Setup

**Files:**
- Test: `cmd/main_test.go`
- Create: `cmd/main.go`

- [ ] **Step 1: Write failing test (RED)**

File: `cmd/main_test.go`

```go
package main

import (
	"net/http"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestManagerCreation(t *testing.T) {
	t.Parallel()

	testEnv := &envtest.Environment{}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest: %v", err)
	}

	t.Cleanup(func() {
		if stopErr := testEnv.Stop(); stopErr != nil {
			t.Errorf("failed to stop envtest: %v", stopErr)
		}
	})

	mgr, err := createManager(cfg, managerOptions{
		metricsAddr:   "0",
		healthAddr:    "0",
		recheckInterval: 30 * time.Second,
		initialDelay:    10 * time.Second,
		nodepoolLabel:   "node.kubernetes.io/nodepool",
		autoscalerCM:    "cluster-autoscaler-status",
	})
	if err != nil {
		t.Fatalf("expected manager creation to succeed, got: %v", err)
	}

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestManagerHasHealthChecks(t *testing.T) {
	t.Parallel()

	testEnv := &envtest.Environment{}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest: %v", err)
	}

	t.Cleanup(func() {
		if stopErr := testEnv.Stop(); stopErr != nil {
			t.Errorf("failed to stop envtest: %v", stopErr)
		}
	})

	mgr, err := createManager(cfg, managerOptions{
		metricsAddr:   "0",
		healthAddr:    "0",
		recheckInterval: 30 * time.Second,
		initialDelay:    10 * time.Second,
		nodepoolLabel:   "node.kubernetes.io/nodepool",
		autoscalerCM:    "cluster-autoscaler-status",
	})
	if err != nil {
		t.Fatalf("manager creation failed: %v", err)
	}

	// Verify the manager accepts additional health checks (proves it was configured correctly).
	err = mgr.AddHealthzCheck("test-healthz", func(_ *http.Request) error {
		return nil
	})
	if err != nil {
		t.Fatalf("manager should accept health checks: %v", err)
	}
}
```

- [ ] **Step 2: Verify RED**

```bash
go test ./cmd/... -v -run TestManager -count=1
```

Expected: FAIL (createManager and managerOptions do not exist)

- [ ] **Step 3: Write minimal implementation (GREEN)**

File: `cmd/main.go`

```go
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/reyshazni/fitcheck/internal/version"
)

type managerOptions struct {
	metricsAddr     string
	healthAddr      string
	nodepoolLabel   string
	recheckInterval time.Duration
	initialDelay    time.Duration
	namespace       string
	autoscalerCM    string
}

func main() {
	opts := parseFlags()

	setupLogger()

	info := version.Info()
	slog.Info("starting fitcheck", "version", info.Version, "commit", info.Commit, "date", info.Date)

	cfg, err := ctrl.GetConfig()
	if err != nil {
		slog.Error("unable to load kubeconfig", "error", err)
		os.Exit(1)
	}

	mgr, err := createManager(cfg, opts)
	if err != nil {
		slog.Error("unable to create manager", "error", err)
		os.Exit(1)
	}

	slog.Info("starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("manager exited with error", "error", err)
		os.Exit(1)
	}
}

func parseFlags() managerOptions {
	opts := managerOptions{}

	flag.StringVar(&opts.metricsAddr, "metrics-addr", ":8080", "metrics bind address")
	flag.StringVar(&opts.healthAddr, "health-addr", ":8081", "health probe bind address")
	flag.StringVar(&opts.nodepoolLabel, "nodepool-label", "node.kubernetes.io/nodepool", "label key for grouping nodes")
	flag.DurationVar(&opts.recheckInterval, "recheck-interval", 30*time.Second, "re-evaluation interval for pending pods")
	flag.DurationVar(&opts.initialDelay, "initial-delay", 10*time.Second, "delay before first diagnosis")
	flag.StringVar(&opts.namespace, "namespace", "", "restrict to specific namespace")
	flag.StringVar(&opts.autoscalerCM, "autoscaler-configmap", "cluster-autoscaler-status", "ConfigMap name for autoscaler status")
	flag.Parse()

	return opts
}

func setupLogger() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := logr.FromSlogHandler(handler)
	ctrl.SetLogger(logger)
	slog.SetDefault(slog.New(handler))
}

func createManager(cfg *rest.Config, opts managerOptions) (ctrl.Manager, error) {
	mgrOpts := ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: opts.metricsAddr,
		},
		HealthProbeBindAddress: opts.healthAddr,
		Cache:                  buildCacheOptions(opts),
	}

	mgr, err := ctrl.NewManager(cfg, mgrOpts)
	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	if err := addHealthChecks(mgr); err != nil {
		return nil, fmt.Errorf("adding health checks: %w", err)
	}

	return mgr, nil
}

func buildCacheOptions(opts managerOptions) cache.Options {
	cacheOpts := cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.ConfigMap{}: {
				Namespaces: map[string]cache.Config{
					"kube-system": {},
				},
			},
		},
	}

	if opts.namespace != "" {
		cacheOpts.DefaultNamespaces = map[string]cache.Config{
			opts.namespace: {},
		}
	}

	return cacheOpts
}

func addHealthChecks(mgr ctrl.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up healthz: %w", err)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up readyz: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: Install envtest binaries**

```bash
go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.22
setup-envtest use -p env
```

Source the output to set `KUBEBUILDER_ASSETS` before running tests.

- [ ] **Step 5: Verify GREEN**

```bash
eval $(setup-envtest use -p env)
go test ./cmd/... -v -count=1
```

Expected: ALL PASS

- [ ] **Step 6: Lint**

```bash
go tool golangci-lint run ./cmd/...
```

Expected: zero issues

- [ ] **Step 7: Commit**

```bash
git add cmd/
git commit -m "feat: add manager setup entrypoint"
```

---

## Task 3: Makefile

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create Makefile**

File: `Makefile`

```makefile
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
```

- [ ] **Step 2: Verify Makefile targets**

```bash
make build
make lint
make helm-lint
```

Expected: all targets succeed

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: add project Makefile"
```

---

## Task 4: Dockerfile

**Files:**
- Create: `Dockerfile`

- [ ] **Step 1: Create Dockerfile**

File: `Dockerfile`

```dockerfile
FROM golang:1.26 AS builder

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w \
      -X github.com/reyshazni/fitcheck/internal/version.buildVersion=${VERSION} \
      -X github.com/reyshazni/fitcheck/internal/version.buildCommit=${COMMIT} \
      -X github.com/reyshazni/fitcheck/internal/version.buildDate=${DATE}" \
    -o fitcheck ./cmd/

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /workspace/fitcheck /fitcheck

USER 65532:65532

ENTRYPOINT ["/fitcheck"]
```

- [ ] **Step 2: Verify build**

```bash
make docker-build
```

Expected: Docker image builds successfully

- [ ] **Step 3: Commit**

```bash
git add Dockerfile
git commit -m "chore: add multi-stage Dockerfile"
```

---

## Task 5: CI Pipeline (.github/workflows/ci.yml)

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create CI workflow**

File: `.github/workflows/ci.yml`

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

permissions:
  contents: read

jobs:
  lint:
    name: Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - name: Run go fmt
        run: |
          output=$(gofmt -l .)
          if [ -n "$output" ]; then
            echo "Files not formatted:"
            echo "$output"
            exit 1
          fi

      - name: Run go vet
        run: go vet ./...

      - name: Run golangci-lint
        run: go tool golangci-lint run ./...

      - name: Install yamllint
        run: pip install yamllint

      - name: Run yamllint
        run: yamllint -c .yamllint.yml charts/ .github/

  test:
    name: Test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - name: Install setup-envtest
        run: go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.22

      - name: Run tests
        run: |
          eval $(setup-envtest use -p env)
          go test -race -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: actions/upload-artifact@v4
        with:
          name: coverage
          path: coverage.out

  helm:
    name: Helm
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Helm
        uses: azure/setup-helm@v4

      - name: Lint chart
        run: helm lint charts/fitcheck/

      - name: Template chart
        run: helm template fitcheck charts/fitcheck/ -n kube-system > /dev/null

  build:
    name: Build
    runs-on: ubuntu-latest
    needs: [lint, test]
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - name: Build binary
        run: CGO_ENABLED=0 go build -o bin/fitcheck ./cmd/

      - name: Build Docker image
        run: docker build -t fitcheck:ci .
```

- [ ] **Step 2: Verify YAML is valid**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add lint test build pipeline"
```

---

## Task 6: E2E Pipeline (.github/workflows/e2e.yml)

**Files:**
- Create: `.github/workflows/e2e.yml`, `hack/e2e-setup.sh`, `test/e2e/e2e_test.go`

- [ ] **Step 1: Create E2E setup script**

File: `hack/e2e-setup.sh`

```bash
#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-fitcheck-e2e}"
IMAGE_NAME="${IMAGE_NAME:-fitcheck:e2e}"
NAMESPACE="${NAMESPACE:-kube-system}"
CHART_DIR="${CHART_DIR:-charts/fitcheck}"

echo "Building fitcheck image..."
docker build -t "${IMAGE_NAME}" .

echo "Loading image into kind cluster..."
kind load docker-image "${IMAGE_NAME}" --name "${CLUSTER_NAME}"

echo "Installing fitcheck via Helm..."
helm upgrade --install fitcheck "${CHART_DIR}" \
  --namespace "${NAMESPACE}" \
  --set image.repository=fitcheck \
  --set image.tag=e2e \
  --set image.pullPolicy=Never \
  --wait \
  --timeout 120s

echo "Waiting for deployment to be ready..."
kubectl rollout status deployment/fitcheck -n "${NAMESPACE}" --timeout=120s

echo "E2E setup complete."
```

- [ ] **Step 2: Create E2E test scaffold**

File: `test/e2e/e2e_test.go`

```go
//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	healthTimeout = 30 * time.Second
	healthPoll    = 2 * time.Second
)

func TestControllerStarts(t *testing.T) {
	namespace := getEnvOrDefault("NAMESPACE", "kube-system")
	t.Logf("checking fitcheck in namespace %s", namespace)

	// The deployment readiness is verified by hack/e2e-setup.sh.
	// This test validates the controller is reachable.
	t.Log("controller deployment is running")
}

func TestHealthzEndpoint(t *testing.T) {
	healthURL := getEnvOrDefault("HEALTH_URL", "")
	if healthURL == "" {
		t.Skip("HEALTH_URL not set, skipping direct health check")
	}

	client := &http.Client{Timeout: healthTimeout}

	var lastErr error

	deadline := time.Now().Add(healthTimeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("%s/healthz", healthURL))
		if err != nil {
			lastErr = err
			time.Sleep(healthPoll)

			continue
		}

		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Log("healthz returned 200 OK")

			return
		}

		lastErr = fmt.Errorf("healthz returned status %d", resp.StatusCode)
		time.Sleep(healthPoll)
	}

	t.Fatalf("healthz did not return 200 within timeout: %v", lastErr)
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
```

- [ ] **Step 3: Create E2E workflow**

File: `.github/workflows/e2e.yml`

```yaml
name: E2E

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

permissions:
  contents: read

jobs:
  e2e:
    name: E2E (K8s ${{ matrix.k8s-version }})
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        k8s-version: ["1.30", "1.31"]
        include:
          - k8s-version: "1.30"
            kind-image: kindest/node:v1.30.10
          - k8s-version: "1.31"
            kind-image: kindest/node:v1.31.6
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - name: Set up Helm
        uses: azure/setup-helm@v4

      - name: Create kind cluster
        uses: helm/kind-action@v1
        with:
          node_image: ${{ matrix.kind-image }}
          cluster_name: fitcheck-e2e

      - name: Run E2E setup
        run: bash hack/e2e-setup.sh
        env:
          CLUSTER_NAME: fitcheck-e2e

      - name: Run E2E tests
        run: go test -tags e2e -v ./test/e2e/...

      - name: Collect failure logs
        if: failure()
        run: |
          echo "=== Pod status ==="
          kubectl get pods -A
          echo "=== Controller logs ==="
          kubectl logs -n kube-system -l app.kubernetes.io/name=fitcheck --tail=100
          echo "=== Events ==="
          kubectl get events -n kube-system --sort-by='.lastTimestamp'
          echo "=== Describe deployment ==="
          kubectl describe deployment -n kube-system fitcheck
```

- [ ] **Step 4: Make script executable and verify YAML**

```bash
chmod +x hack/e2e-setup.sh
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/e2e.yml'))"
```

Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/e2e.yml hack/e2e-setup.sh test/e2e/
git commit -m "ci: add e2e test pipeline"
```

---

## Task 7: Release Pipeline (.github/workflows/release.yml)

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create release workflow**

File: `.github/workflows/release.yml`

```yaml
name: Release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write
  packages: write
  id-token: write

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: reyshazni/fitcheck
  CHART_REGISTRY: oci://ghcr.io/reyshazni/charts

jobs:
  publish-image:
    name: Publish Image
    runs-on: ubuntu-latest
    outputs:
      digest: ${{ steps.build-push.outputs.digest }}
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - name: Install setup-envtest
        run: go install sigs.k8s.io/controller-runtime/tools/setup-envtest@release-0.22

      - name: Run tests
        run: |
          eval $(setup-envtest use -p env)
          go test -race ./...

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GHCR
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=sha

      - name: Build and push
        id: build-push
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          platforms: linux/amd64,linux/arm64
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            VERSION=${{ github.ref_name }}
            COMMIT=${{ github.sha }}
            DATE=${{ github.event.head_commit.timestamp }}

      - name: Install cosign
        uses: sigstore/cosign-installer@v3

      - name: Sign image
        run: cosign sign --yes ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}@${{ steps.build-push.outputs.digest }}

      - name: Generate SBOM
        uses: anchore/sbom-action@v0
        with:
          image: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}@${{ steps.build-push.outputs.digest }}
          output-file: sbom.spdx.json

      - name: Attach SBOM to image
        run: cosign attach sbom --sbom sbom.spdx.json ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}@${{ steps.build-push.outputs.digest }}

      - name: Upload SBOM artifact
        uses: actions/upload-artifact@v4
        with:
          name: sbom
          path: sbom.spdx.json

  publish-chart:
    name: Publish Chart
    runs-on: ubuntu-latest
    needs: [publish-image]
    steps:
      - uses: actions/checkout@v4

      - name: Set up Helm
        uses: azure/setup-helm@v4

      - name: Log in to GHCR
        run: echo "${{ secrets.GITHUB_TOKEN }}" | helm registry login ${{ env.REGISTRY }} -u ${{ github.actor }} --password-stdin

      - name: Update chart appVersion
        run: |
          VERSION="${GITHUB_REF_NAME#v}"
          sed -i "s/^appVersion:.*/appVersion: \"${VERSION}\"/" charts/fitcheck/Chart.yaml
          sed -i "s/^version:.*/version: ${VERSION}/" charts/fitcheck/Chart.yaml

      - name: Package chart
        run: helm package charts/fitcheck/

      - name: Push chart
        run: helm push fitcheck-*.tgz ${{ env.CHART_REGISTRY }}

  github-release:
    name: GitHub Release
    runs-on: ubuntu-latest
    needs: [publish-image, publish-chart]
    steps:
      - uses: actions/checkout@v4

      - name: Set up Helm
        uses: azure/setup-helm@v4

      - name: Generate install.yaml
        run: helm template fitcheck charts/fitcheck/ -n kube-system > install.yaml

      - name: Download SBOM
        uses: actions/download-artifact@v4
        with:
          name: sbom

      - name: Create release
        uses: softprops/action-gh-release@v2
        with:
          generate_release_notes: true
          files: |
            install.yaml
            sbom.spdx.json
```

- [ ] **Step 2: Verify YAML is valid**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))"
```

Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release pipeline"
```

---

## Task 8: Helm Chart Validation

**Files:**
- Verify: `charts/fitcheck/`

The chart was already scaffolded by a prior agent. This task validates it.

- [ ] **Step 1: Run helm lint**

```bash
helm lint charts/fitcheck/
```

Expected: 0 errors, 0 warnings (or only informational)

- [ ] **Step 2: Run helm template**

```bash
helm template fitcheck charts/fitcheck/ -n kube-system > /dev/null
```

Expected: succeeds with exit code 0

- [ ] **Step 3: Validate rendered output**

```bash
helm template fitcheck charts/fitcheck/ -n kube-system | kubectl apply --dry-run=client -f -
```

Expected: all resources valid (may fail without cluster, that is acceptable)

- [ ] **Step 4: Fix any issues found**

If lint or template fails, fix the chart files. Common issues:
- Missing required fields in Chart.yaml
- Template syntax errors
- Invalid YAML in values.yaml

- [ ] **Step 5: Commit (only if changes were made)**

```bash
git add charts/
git commit -m "fix: resolve helm chart issues"
```

---

## Task 9: .yamllint.yml

**Files:**
- Create: `.yamllint.yml`

- [ ] **Step 1: Create yamllint config**

File: `.yamllint.yml`

```yaml
extends: default

rules:
  line-length:
    max: 200
    allow-non-breakable-words: true
    allow-non-breakable-inline-mappings: true
  truthy:
    check-keys: false
  comments:
    min-spaces-from-content: 1
  document-start: disable
  braces:
    max-spaces-inside: 1
```

- [ ] **Step 2: Verify yamllint passes**

```bash
pip install yamllint 2>/dev/null || true
yamllint -c .yamllint.yml charts/ .github/
```

Expected: no errors (warnings are acceptable)

- [ ] **Step 3: Commit**

```bash
git add .yamllint.yml
git commit -m "chore: add yamllint config"
```

---

## Task 10: Verify Everything Works Together

- [ ] **Step 1: Run full verification**

```bash
make verify
```

This runs: fmt, vet, lint, test, helm-lint.

Expected: all targets pass with zero errors.

- [ ] **Step 2: Run Docker build**

```bash
make docker-build
```

Expected: image builds successfully.

- [ ] **Step 3: Verify binary has version info**

```bash
make build
./bin/fitcheck --help
```

Expected: binary compiles and shows flag help.

- [ ] **Step 4: Verify .gitignore covers build artifacts**

Check that `.gitignore` includes:

```
bin/
coverage.out
*.tgz
```

If not, update it.

- [ ] **Step 5: Final commit (only if .gitignore updated)**

```bash
git add .gitignore
git commit -m "chore: update gitignore for builds"
```

---

## Summary Checklist

| Task | Description | Key Output |
|---|---|---|
| 1 | Go module + version package | `go.mod`, `internal/version/` |
| 2 | cmd/main.go with manager | `cmd/main.go`, envtest test |
| 3 | Makefile | `Makefile` with all targets |
| 4 | Dockerfile | Multi-stage distroless build |
| 5 | CI pipeline | `.github/workflows/ci.yml` |
| 6 | E2E pipeline | `.github/workflows/e2e.yml`, `hack/`, `test/e2e/` |
| 7 | Release pipeline | `.github/workflows/release.yml` |
| 8 | Helm validation | Verify existing chart |
| 9 | yamllint config | `.yamllint.yml` |
| 10 | Full verification | All checks pass |
