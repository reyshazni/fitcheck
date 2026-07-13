# Architecture Overview

## What fitcheck does

fitcheck is a Kubernetes controller that watches Pending pods and emits per-nodepool diagnostic Events explaining why each nodepool accepted or rejected the pod. The results appear natively in `kubectl describe pod`, no client-side tooling required.

## How it works

```
Pending Pod (watch)
       |
       v
  PodReconciler
       |
       +-- List nodes, group by nodepool label
       +-- For each nodepool:
       |     +-- Check taint/toleration
       |     +-- Check nodeSelector
       |     +-- Check node affinity
       |     +-- Check resource fit (cpu, memory, gpu)
       |     +-- Check pod anti-affinity
       |     +-- Check topology spread
       |     +-- Check autoscaler state (ConfigMap)
       |     +-- Determine verdict: Accepted / Rejected / Candidate / NoStock
       |
       +-- Emit single FitcheckDiagnosis event (one-line summary)
       +-- Write fitcheck.io/diagnosis annotation (full JSON detail)
       |
       v
  Event + annotation on Pod (visible via kubectl describe / kubectl get -o jsonpath)
```

## Reconciler lifecycle

1. Pod enters Pending state, which triggers reconcile
2. Controller waits `--initial-delay` (default 10s) to let the scheduler attempt first
3. Diagnoses scheduling fit per nodepool
4. Emits a single `FitcheckDiagnosis` event with a one-line summary on the pod
5. Writes full per-nodepool JSON to the `fitcheck.io/diagnosis` annotation on the pod
6. Requeues every `--recheck-interval` (default 30s) while pod remains Pending
7. Stops processing once pod is Scheduled or deleted (annotation is cleaned up when pod leaves Pending)

## Scheduling dimensions checked (in order)

1. Taint/toleration mismatch
2. NodeSelector not matched
3. Node affinity not matched
4. Insufficient resources (cpu, memory, gpu)
5. Pod anti-affinity conflict
6. Topology spread constraint violation
7. Autoscaler state (backoff, stock unavailable, max size reached)

The event message reports the first matching rejection reason.

## Autoscaler integration

The controller reads the `cluster-autoscaler-status` ConfigMap in kube-system to determine scaling state per nodegroup. It also correlates autoscaler events (TriggeredScaleUp, NotTriggerScaleUp, FailedScaleUp) on the pod.

This enables NodepoolNoStock and NodepoolCandidate verdicts, answering "would this nodepool work if it could scale up?" and "why can't it scale up?"

## Provider support

| Provider | Nodepool label | Autoscaler source |
|---|---|---|
| ACK (Alibaba) | `alibabacloud.com/nodepool-id` | ConfigMap |
| GKE | `cloud.google.com/gke-nodepool` | ConfigMap |
| EKS (managed) | `eks.amazonaws.com/nodegroup` | ConfigMap |
| EKS (Karpenter) | `karpenter.sh/nodepool` | NodePool/NodeClaim CRDs |
| TKE (Tencent) | `tke.cloud.tencent.com/nodepool-id` | ConfigMap (keyed by ASG ID) |

Provider is auto-detected from cluster node labels at startup. No flag needed.

## Event deduplication

Uses the standard Kubernetes EventRecorder which auto-deduplicates via EventCorrelator in client-go. Identical events increment `.Count` and update `.LastTimestamp`. No manual dedup needed.
