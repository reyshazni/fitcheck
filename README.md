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

- **Language**: Go
- **Framework**: controller-runtime
- **Testing**: envtest
- **Distribution**: Helm chart (OCI), container image (ghcr.io)
- **CI/CD**: GitHub Actions

## Install

### Helm (recommended)

```bash
helm install fitcheck oci://ghcr.io/helmi/charts/fitcheck
```

### Quick-start (evaluation only)

```bash
kubectl apply -f https://github.com/helmi/fitcheck/releases/latest/download/install.yaml
```

## Configuration

| Flag | Default | Purpose |
|---|---|---|
| `--nodepool-label` | `node.kubernetes.io/nodepool` | Label key for grouping nodes into nodepools |
| `--recheck-interval` | `30s` | Re-evaluation interval for pending pods |
| `--initial-delay` | `10s` | Delay before first diagnosis |
| `--namespace` | (all) | Restrict to specific namespace |
| `--autoscaler-configmap` | `cluster-autoscaler-status` | ConfigMap name for autoscaler status |

## Supported providers

| Provider | Nodepool label | Autoscaler source |
|---|---|---|
| ACK (Alibaba) | `alibabacloud.com/nodepool-id` | ConfigMap |
| GKE | `cloud.google.com/gke-nodepool` | ConfigMap |
| EKS (managed) | `eks.amazonaws.com/nodegroup` | ConfigMap |
| EKS (Karpenter) | `karpenter.sh/nodepool` | NodePool/NodeClaim CRDs |
| TKE (Tencent) | `tke.cloud.tencent.com/nodepool-id` | ConfigMap |

Auto-detection available via `--provider=auto`.

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
make build          # compile to bin/fitcheck
make test           # run all tests
make lint           # golangci-lint
make run            # build and run
make docker-build   # build container image
```

## Repository layout

```
cmd/                        # main.go, controller entrypoint
internal/
  controller/               # PodReconciler, core reconcile loop
  types/                    # shared types (Verdict, NodepoolDiagnosis, AutoscalerStatus)
charts/
  fitcheck/                 # Helm chart
    templates/
      deployment.yaml
      serviceaccount.yaml
      clusterrole.yaml
      clusterrolebinding.yaml
docs/
  architecture/
    overview.md             # architecture deep-dive, reconciler lifecycle
    provider-research.md    # provider-specific labels, autoscaler behavior, quirks
  feature/
    scheduler-diagnostics.md  # feature spec (event types, message format, behavior)
  plans/
    v0.0.1-implementation.md  # TDD implementation plan
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
| `docs/plans/v0.0.1-implementation.md` | Step-by-step TDD implementation plan |
