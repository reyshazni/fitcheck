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

## Event Types

The controller emits these Events on Pending pods:

| Reason | Type | When |
|---|---|---|
| NodepoolAccepted | Normal | Pod fits on existing node(s) in this nodepool |
| NodepoolCandidate | Normal | Pod would fit if nodepool scales up |
| NodepoolRejected | Warning | Pod cannot schedule on this nodepool |
| NodepoolNoStock | Warning | Autoscaler cannot scale this nodepool (backoff, stock, max size) |

## Event Message Format

Each event message starts with `nodepool/<name>:` followed by the reason.

```
nodepool/dsp-general: 2 of 3 nodes fit (cpu=2/8 avail, mem=4Gi/16Gi avail)
nodepool/ml-training: taint {team=ml:NoSchedule} not tolerated
nodepool/system: nodeSelector {team=dsp} not matched
nodepool/dsp-highmem: Insufficient cpu (requested=2, max allocatable=1.5)
nodepool/dsp-spot: autoscaler backoff - FailedScaleUp (insufficient China-East-2 inventory)
nodepool/dsp-general: would fit on new node, but autoscaler in backoff
```

## Rejection Categories

The controller checks these scheduling dimensions per nodepool, in order:

1. Taint/toleration mismatch
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
  Type     Reason             Age  From                  Message
  ----     ------             ---  ----                  -------
  Warning  FailedScheduling   5m   default-scheduler     0/12 nodes available: 4 had taint, 5 Insufficient cpu, 3 node(s) didn't match node selector
  Normal   NodepoolAccepted   4m   fitcheck    nodepool/dsp-general: 2 of 3 nodes fit (cpu=2/8 avail, mem=4Gi/16Gi avail)
  Warning  NodepoolRejected   4m   fitcheck    nodepool/ml-training: taint {team=ml:NoSchedule} not tolerated
  Warning  NodepoolRejected   4m   fitcheck    nodepool/system: nodeSelector {team=dsp} not matched
  Warning  NodepoolRejected   4m   fitcheck    nodepool/dsp-highmem: Insufficient cpu (requested=2, max allocatable=1.5)
  Warning  NodepoolNoStock    4m   fitcheck    nodepool/dsp-spot: autoscaler backoff - FailedScaleUp (insufficient China-East-2 inventory)
  Normal   NodepoolCandidate  4m   fitcheck    nodepool/dsp-general: would fit on new node, but autoscaler in backoff
```

## Configuration

| Flag | Default | Purpose |
|---|---|---|
| --nodepool-label | node.kubernetes.io/nodepool | Label key for grouping nodes into nodepools |
| --recheck-interval | 30s | Re-evaluation interval for pending pods |
| --initial-delay | 10s | Delay before first diagnosis |
| --namespace | (all) | Restrict to specific namespace |
| --autoscaler-configmap | cluster-autoscaler-status | ConfigMap name for autoscaler status |

## Deployment

- Runs in kube-system as a single-replica Deployment
- Go binary using controller-runtime
- Minimal RBAC: read pods/nodes/configmaps, create events
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
