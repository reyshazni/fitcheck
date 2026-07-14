# fitcheck

Kubernetes controller that diagnoses why pods are stuck in Pending. Per-nodepool scheduling breakdown visible via `kubectl describe pod`, no client-side tooling required.

## The problem

The scheduler's `FailedScheduling` event only shows aggregated rejection counts:

```
0/26 nodes available: 4 had untolerated taint {project: growth-dge}, 7 had untolerated taint
{workload_type: nfs}, 8 didn't match Pod's node affinity/selector...
```

This tells you nothing about which nodepools rejected the pod, why each one rejected it, or what the autoscaler is doing about it.

## What fitcheck does

```
$ kubectl describe pod my-pending-job

Events:
  Type     Reason              Age  From               Message
  ----     ------              ---  ----               -------
  Warning  FailedScheduling    5m   default-scheduler   0/26 nodes available: ...
  Warning  FitcheckDiagnosis   4m   fitcheck            2/13 nodepools fit | rejected: 8 taint, 2 affinity | no-stock: 2 | candidate: 1 | initializing: 1
```

One compact event per reconcile:
- **Normal** if all nodepools fit, **Warning** if any are rejected, no-stock, or candidate
- Single `FitcheckDiagnosis` reason with a one-line summary

### Full diagnosis (annotation)

`kubectl describe pod` truncates long annotations. To see the full per-nodepool breakdown:

```bash
kubectl get pod -n <namespace> <pod_name> -o jsonpath='{.metadata.annotations.fitcheck\.io/diagnosis}' | jq .
```

```json
{
  "timestamp": "2026-07-14T04:04:58Z",
  "summary": "1/15 nodepools fit | rejected: 12 taint, 2 affinity",
  "nodepools": [
    {
      "name": "general-pool",
      "verdict": "accepted",
      "fitting": 1,
      "total": 1
    },
    {
      "name": "gpu-pool",
      "verdict": "rejected",
      "reason": "taint nvidia.com/gpu=present:NoSchedule not tolerated",
      "category": "taint"
    },
    {
      "name": "nfs",
      "verdict": "rejected",
      "reason": "taint workload_type=nfs:NoSchedule not tolerated",
      "category": "taint"
    }
  ]
}
```

Filter for specific verdicts:

```bash
# Show only rejected nodepools
kubectl get pod -n <namespace> <pod_name> -o jsonpath='{.metadata.annotations.fitcheck\.io/diagnosis}' | jq '.nodepools[] | select(.verdict == "rejected")'

# Show only accepted nodepools
kubectl get pod -n <namespace> <pod_name> -o jsonpath='{.metadata.annotations.fitcheck\.io/diagnosis}' | jq '.nodepools[] | select(.verdict == "accepted")'
```

The annotation is automatically removed when the pod leaves Pending state.

## How it works

fitcheck reads only from the Kubernetes API. Zero cloud SDK dependencies.

1. Watches Pending pods via controller-runtime
2. Groups cluster nodes by nodepool label (auto-detected per provider)
3. Evaluates scheduling fit per nodepool: taints, nodeSelector, node affinity, resources (cpu, memory, gpu)
4. Reads autoscaler events (GOATScaler) to determine scaling state per nodepool
5. Emits compact summary events on the pod

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
| `--metrics-addr` | `:8080` | Prometheus metrics bind address |
| `--health-addr` | `:8081` | Health probe bind address (/healthz, /readyz) |
| `--recheck-interval` | `30s` | Re-evaluation interval for pending pods |
| `--initial-delay` | `10s` | Delay before first diagnosis (let scheduler attempt first) |
| `--namespace` | (all) | Restrict to specific namespace |
| `--startup-timeout` | `10m` | Startup taint detection timeout |

Provider is auto-detected from cluster node labels at startup. No `--provider` flag needed.

## Provider support

v0.0.1 targets **Alibaba ACK** with GOATScaler. Multi-provider support is planned.

| Provider | Nodepool label | Status |
|---|---|---|
| ACK (Alibaba) | `alibabacloud.com/nodepool-id` | v0.0.1 |
| GKE | `cloud.google.com/gke-nodepool` | planned |
| EKS (managed) | `eks.amazonaws.com/nodegroup` | planned |
| EKS (Karpenter) | `karpenter.sh/nodepool` | planned |
| TKE (Tencent) | `tke.cloud.tencent.com/nodepool-id` | planned |

Adding a provider means implementing the `Provider` interface and registering it. See `internal/provider/ack/` for reference.

## Scheduling dimensions checked

Per nodepool, in order:

1. Taint/toleration mismatch (startup taints yield `Initializing` verdict during `--startup-timeout` window)
2. NodeSelector not matched
3. Node affinity not matched
4. Insufficient resources (cpu, memory, nvidia.com/gpu)

**Verdicts**: `accepted`, `rejected`, `candidate` (scale-up triggered), `no-stock` (inventory unhealthy), `initializing` (node blocked only by startup taints, still joining the cluster).

Autoscaler integration (GOATScaler):
- Reads `ProvisionNode`, `NotTriggerScaleUp`, `ProvisionNodeFailed` events on pods
- Reads `InstanceInventoryStatusChanged` events for nodepool stock status
- Upgrades rejected verdicts to `candidate` (scale-up triggered) or `no-stock` (inventory unhealthy)

## RBAC

```yaml
rules:
  - apiGroups: [""]
    resources: [pods]
    verbs: [get, list, watch, patch]
  - apiGroups: [""]
    resources: [nodes]
    verbs: [get, list, watch]
  - apiGroups: [""]
    resources: [configmaps]
    verbs: [get, list, watch]
  - apiGroups: [""]
    resources: [events]
    verbs: [get, list, create, patch]
  - apiGroups: ["events.k8s.io"]
    resources: [events]
    verbs: [get, list, create, patch]
```

Pods require `patch` for writing the `fitcheck.io/diagnosis` annotation. The `events.k8s.io` group is needed for the newer Events API.

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
cmd/
  main.go                       # thin entry point: parse flags, app.Run()
internal/
  app/                          # manager creation, health checks, controller wiring
  controller/                   # PodReconciler, event emission
  diagnosis/                    # scheduling checks (pure functions), compact summary formatting
  provider/                     # Provider interface (plug-and-play per cloud)
  provider/ack/                 # ACK provider: labels, GOATScaler integration
  nodepool/                     # node collector, groups nodes by label
  autoscaler/                   # StatusReader interface, GOATScaler event reader
  log/                          # structured logging setup (slog + logr)
  version/                      # build-time version info
charts/
  fitcheck/                     # Helm chart
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
| `docs/architecture/provider-research.md` | ACK labels, GOATScaler events, autoscaler research |
| `docs/feature/scheduler-diagnostics.md` | Feature spec: event types, message format, behavior |
| `docs/plans/` | Implementation plans |
