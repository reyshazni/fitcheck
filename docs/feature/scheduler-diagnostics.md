# fitcheck

## Problem

Debugging why pods are Pending on Alicloud ACK is high cognitive load. The scheduler's FailedScheduling event only shows aggregated rejection counts, not per-nodepool breakdowns. The autoscaler's status is in a separate ConfigMap. Autoscaler events are scattered across pods and nodes. There is no single view that answers: "for this pod, why was it rejected by each nodepool, and what's the autoscaler doing about it?"

No existing kubectl plugin or tool solves this.

## Goal

When I run `kubectl describe pod <pending-pod>`, I see per-nodepool Events that tell me:
- Which nodepools can accept this pod and which can't
- The specific reason each nodepool rejected the pod
- The autoscaler's scaling state for each relevant nodepool (backoff, stock unavailable, max size reached)
- Which nodepools would fit the pod if they could scale up

Zero client-side tooling required. The information appears natively in pod Events for anyone on the cluster.

## Solution

A Kubernetes controller running in kube-system that watches Pending pods, diagnoses scheduling fit per nodepool, and writes the results as Events on the pod.

## Event Format

The controller emits a single `FitcheckDiagnosis` event per reconcile on Pending pods:

| Reason | Type | When |
|---|---|---|
| FitcheckDiagnosis | Normal | At least one nodepool is Accepted, Initializing, or Candidate |
| FitcheckDiagnosis | Warning | All nodepools are Rejected or NoStock |

The event message is a one-line summary:

```
2/13 nodepools fit | rejected: 8 taint, 2 affinity | no-stock: 2 | candidate: 1 | initializing: 1
```

## Annotation Diagnostics

Full per-nodepool detail is written to the `fitcheck.io/diagnosis` annotation on the pod:

```json
{
  "timestamp": "2026-07-13T17:55:31Z",
  "summary": "2/13 nodepools fit | rejected: 8 taint, 2 affinity | no-stock: 2",
  "nodepools": [
    {"name": "general-pool", "verdict": "accepted", "fitting": 3, "total": 5},
    {"name": "gpu-pool", "verdict": "rejected", "reason": "taint nvidia.com/gpu=present:NoSchedule not tolerated", "category": "taint"}
  ]
}
```

Query the annotation:

```bash
kubectl get pod <name> -o jsonpath='{.metadata.annotations.fitcheck\.io/diagnosis}' | jq .
```

The annotation is automatically removed when the pod leaves Pending state.

## Rejection Categories

The controller checks these scheduling dimensions per nodepool, in order:

1. Taint/toleration mismatch
   - Startup taints: if the only untolerated taints are known startup taints and the node is within `--startup-timeout` of creation, verdict is `Initializing` instead of `Rejected`
2. NodeSelector not matched
3. Node affinity not matched
4. Insufficient resources (cpu, memory, gpu)
5. Pod anti-affinity conflict
6. Topology spread constraint violation
7. Autoscaler state: max size reached, backoff, FailedScaleUp, stock unavailable

The event message reports the first matching rejection reason (most specific).

## Behavior

- Only processes Pending pods (ignores Running, Succeeded, Failed)
- Waits 10s after a pod enters Pending before first diagnosis (let the scheduler attempt first)
- Re-evaluates every 30s while pod remains Pending
- Only emits new events when diagnosis changes (no spam)
- Stops processing once pod is Scheduled or deleted
- Detects active autoscaler (GOATScaler or cluster-autoscaler) from ConfigMaps in kube-system
- Reads autoscaler events on the pod (ProvisionNode, ProvisionNodeFailed, NotTriggerScaleUp for GOATScaler; TriggeredScaleUp, FailedScaleUp for cluster-autoscaler)
- Reads InstanceInventoryStatusChanged events on ACKNodePool objects for stock status (GOATScaler)

## Example

```
$ kubectl describe pod my-ml-job

...
Events:
  Type     Reason              Age  From               Message
  ----     ------              ---  ----               -------
  Warning  FailedScheduling    5m   default-scheduler  0/12 nodes available: 4 had taint, 5 Insufficient cpu, 3 node(s) didn't match node selector
  Warning  FitcheckDiagnosis   4m   fitcheck           2/6 nodepools fit | rejected: 2 taint, 1 affinity, 1 resource | no-stock: 1
```

Full per-nodepool breakdown is available via the `fitcheck.io/diagnosis` annotation:

```bash
kubectl get pod my-ml-job -o jsonpath='{.metadata.annotations.fitcheck\.io/diagnosis}' | jq .
```

## Configuration

| Flag | Default | Purpose |
|---|---|---|
| `--metrics-addr` | `:8080` | Prometheus metrics bind address |
| `--health-addr` | `:8081` | Health probe bind address (/healthz, /readyz) |
| `--recheck-interval` | `30s` | Re-evaluation interval for pending pods |
| `--initial-delay` | `10s` | Delay before first diagnosis |
| `--namespace` | (all) | Restrict to specific namespace |
| `--startup-timeout` | `10m` | Startup taint detection timeout |

## Deployment

- Runs in kube-system as a single-replica Deployment
- Go binary using controller-runtime
- Minimal RBAC: read pods/nodes/configmaps, patch pods (annotation writes), create/patch events (core and events.k8s.io groups)
- Low resource footprint (only processes Pending pods)

## Distribution

### Helm chart (primary install method)

```
charts/fitcheck/
  Chart.yaml
  values.yaml
  templates/
    deployment.yaml
    serviceaccount.yaml
    clusterrole.yaml
    clusterrolebinding.yaml
```

Overridable values:
- image.repository, image.tag
- namespace
- resources.requests/limits
- tolerations, nodeSelector (for running on control-plane nodes)
- nodepoolLabel (default: alibabacloud.com/nodepool-id)
- recheckInterval, initialDelay

Production install:

```bash
helm install fitcheck oci://ghcr.io/helmi/charts/fitcheck
```

### Raw install.yaml (quick-start)

Single-file GitHub release asset for evaluation. Not for production.

```bash
kubectl apply -f https://github.com/helmi/fitcheck/releases/latest/download/install.yaml
```

### Container image

- Registry: ghcr.io (no rate limits for public images)
- Multi-arch: amd64 + arm64
- Tags: semver (e.g. v0.1.0), never latest in production
- CI: GitHub Actions builds image + packages Helm chart as OCI artifact

### Not shipping

- Kustomize base: community can contribute later if needed
- OLM bundle: OpenShift niche, not worth the effort
- Operator Hub: no CRDs, not applicable

## Non-goals

- Does not modify scheduling decisions
- Does not replace the scheduler or autoscaler
- Does not process non-Pending pods
- Does not implement its own scaling logic
