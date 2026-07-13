# fitcheck

Kubernetes controller that diagnoses why pods are stuck in Pending. Watches Pending pods, evaluates scheduling fit per nodepool, and emits diagnostic Events visible via `kubectl describe pod`, no client-side tooling required.

## The problem

The scheduler's `FailedScheduling` event only shows aggregated rejection counts:

```
0/12 nodes available: 4 had taint, 5 Insufficient cpu, 3 node(s) didn't match node selector
```

This tells you nothing about which nodepools rejected the pod, why each one rejected it, or what the autoscaler is doing about it.

## What fitcheck does

```
$ kubectl describe pod my-ml-job

Events:
  Type     Reason             Age  From                  Message
  ----     ------             ---  ----                  -------
  Warning  FailedScheduling   5m   default-scheduler     0/12 nodes available: ...
  Normal   NodepoolAccepted   4m   scheduler-debugger    nodepool/dsp-general: 2 of 3 nodes fit (cpu=2/8 avail, mem=4Gi/16Gi avail)
  Warning  NodepoolRejected   4m   scheduler-debugger    nodepool/ml-training: taint {team=ml:NoSchedule} not tolerated
  Warning  NodepoolRejected   4m   scheduler-debugger    nodepool/system: nodeSelector {team=dsp} not matched
  Warning  NodepoolRejected   4m   scheduler-debugger    nodepool/dsp-highmem: Insufficient cpu (requested=2, max allocatable=1.5)
  Warning  NodepoolNoStock    4m   scheduler-debugger    nodepool/dsp-spot: autoscaler backoff - FailedScaleUp (insufficient China-East-2 inventory)
  Normal   NodepoolCandidate  4m   scheduler-debugger    nodepool/dsp-general: would fit on new node, but autoscaler in backoff
```

Per-nodepool Events that tell you:
- Which nodepools can accept the pod and which can't
- The specific reason each nodepool rejected it
- The autoscaler's scaling state per nodepool (backoff, stock unavailable, max size)
- Which nodepools would fit if they could scale up

## Tech stack

- **Language**: Go 1.26
- **Framework**: controller-runtime v0.24.1
- **Logging**: slog (stdlib) with JSON output, wired to controller-runtime via logr
- **Metrics**: Prometheus at `/metrics` (controller-runtime built-in)
- **Testing**: envtest (integration), Go standard testing (unit)
- **Distribution**: Helm chart (OCI), container image (ghcr.io)
- **CI/CD**: GitHub Actions (lint, test, e2e, release with cosign signing)

## Install

### Helm (recommended)

```bash
helm install fitcheck oci://ghcr.io/reyshazni/charts/fitcheck -n kube-system
```

### Quick-start (evaluation only)

```bash
kubectl apply -f https://github.com/reyshazni/fitcheck/releases/latest/download/install.yaml
```

## Configuration

| Flag | Default | Purpose |
|---|---|---|
| `--nodepool-label` | `node.kubernetes.io/nodepool` | Label key for grouping nodes into nodepools |
| `--recheck-interval` | `30s` | Re-evaluation interval for pending pods |
| `--initial-delay` | `10s` | Delay before first diagnosis |
| `--namespace` | (all) | Restrict to specific namespace |
| `--autoscaler-configmap` | `cluster-autoscaler-status` | ConfigMap name for autoscaler status |
| `--metrics-addr` | `:8080` | Metrics bind address |
| `--health-addr` | `:8081` | Health probe bind address |

## Provider support

v0.0.1 targets **Alibaba ACK** only. Multi-provider support is planned for future releases.

| Provider | Nodepool label | Status |
|---|---|---|
| ACK (Alibaba) | `alibabacloud.com/nodepool-id` | v0.0.1 |
| GKE | `cloud.google.com/gke-nodepool` | planned |
| EKS (managed) | `eks.amazonaws.com/nodegroup` | planned |
| EKS (Karpenter) | `karpenter.sh/nodepool` | planned |
| TKE (Tencent) | `tke.cloud.tencent.com/nodepool-id` | planned |

## Scheduling dimensions checked

The controller evaluates these dimensions per nodepool, in order:

1. Taint/toleration mismatch
2. NodeSelector not matched
3. Node affinity not matched
4. Insufficient resources (cpu, memory, gpu)
5. Pod anti-affinity conflict
6. Topology spread constraint violation
7. Autoscaler state (max size, backoff, FailedScaleUp, stock unavailable)

## RBAC

Minimal permissions: read pods/nodes/configmaps, create events.

```yaml
rules:
  - apiGroups: [""]
    resources: [pods]
    verbs: [get, list, watch]
  - apiGroups: [""]
    resources: [nodes]
    verbs: [get, list]
  - apiGroups: [""]
    resources: [configmaps]
    verbs: [get]
  - apiGroups: [""]
    resources: [events]
    verbs: [create, patch]
```

## Development

```bash
make build          # compile to bin/fitcheck (with ldflags version info)
make test           # run all tests with envtest + race detector
make lint           # golangci-lint (strict config in .golangci.yml)
make fmt            # go fmt
make vet            # go vet
make run            # build and run
make docker-build   # build container image
make helm-lint      # lint and template Helm chart
make verify         # run all checks (fmt, vet, lint, test, helm-lint)
```

## Repository layout

```
cmd/                            # main.go, controller entrypoint
internal/
  controller/                   # PodReconciler, core reconcile loop
  types/                        # shared types (Verdict, NodepoolDiagnosis, AutoscalerStatus)
  version/                      # build-time version info via ldflags
charts/
  fitcheck/                     # Helm chart
    templates/
      deployment.yaml
      serviceaccount.yaml
      clusterrole.yaml
      clusterrolebinding.yaml
.github/
  workflows/
    ci.yml                      # lint, test, helm lint, build
    e2e.yml                     # kind cluster matrix (K8s 1.30, 1.31)
    release.yml                 # image push, cosign, SBOM, Helm OCI, GitHub release
hack/
  e2e-setup.sh                  # kind cluster setup for E2E tests
test/
  e2e/                          # E2E test suite
docs/
  architecture/                 # architecture deep-dive, provider research
  feature/                      # feature specs
  plans/                        # implementation plans
```

## Non-goals

- Does not modify scheduling decisions
- Does not replace the scheduler or autoscaler
- Does not process non-Pending pods
- Does not implement its own scaling logic

## Documentation

| Location | Content |
|---|---|
| `docs/architecture/overview.md` | Architecture, reconciler lifecycle, provider support |
| `docs/architecture/provider-research.md` | Provider labels, autoscaler ConfigMap format, stock errors |
| `docs/feature/scheduler-diagnostics.md` | Feature spec: event types, message format, rejection categories |
| `docs/plans/` | Implementation plans |
