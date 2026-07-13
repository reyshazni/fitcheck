# fitcheck: Implementation Research

## Provider Nodepool Labels

| Provider | Label Key | Value | Confidence |
|---|---|---|---|
| GKE | cloud.google.com/gke-nodepool | pool name | 1.0 |
| ACK | alibabacloud.com/nodepool-id | nodepool ID | 0.95 |
| TKE | tke.cloud.tencent.com/nodepool-id | nodepool ID (e.g. np-4nk83f9v) | 1.0 |
| EKS (managed) | eks.amazonaws.com/nodegroup | nodegroup name | 0.95 |
| EKS (eksctl) | alpha.eksctl.io/nodegroup-name | nodegroup name | 0.95 |
| EKS (Karpenter) | karpenter.sh/nodepool | NodePool name | 0.95 |

### TKE extra mapping

TKE's autoscaler ConfigMap keys nodegroups by ASG ID, not nodepool ID. Nodes carry both labels:
- `tke.cloud.tencent.com/nodepool-id` (nodepool identity)
- `cloud.tencent.com/auto-scaling-group-id` (ASG identity, used in ConfigMap)

The controller needs to map nodepool ID -> ASG ID to correlate with autoscaler status.

### EKS dual autoscaler

EKS clusters may run Cluster Autoscaler or Karpenter. They use different status sources:
- Cluster Autoscaler: standard `cluster-autoscaler-status` ConfigMap
- Karpenter: NodePool `.status` and NodeClaim `.status.conditions` CRDs

The controller should detect which is active and adapt.

## Autoscaler Status Reporting

All 4 providers use the standard upstream `cluster-autoscaler-status` ConfigMap in kube-system (when using Cluster Autoscaler). No provider-specific deviations in ConfigMap format.

### ConfigMap structure

```
Cluster-wide:
  Health:      Healthy (ready=N unready=0 ...)
  ScaleUp:     NoActivity | InProgress | Backoff
  ScaleDown:   NoCandidates | CandidatesPresent

NodeGroups:
  Name:        <nodegroup-identifier>
  Health:      Healthy (ready=N ...)
  ScaleUp:     NoActivity | InProgress | Backoff (ready=N, cloudProviderTarget=M ...)
  ScaleDown:   NoCandidates | CandidatesPresent
```

### ACK extra ConfigMap

ACK also has an `autoscaler-meta` ConfigMap in kube-system with ACK-specific metadata.

## Stock Exhaustion Error Signals

| Provider | Error Code / Message | Where it surfaces |
|---|---|---|
| GKE | ZONE_RESOURCE_POOL_EXHAUSTED, QUOTA_EXCEEDED, GCE_STOCKOUT | ScaleUpFailed event on pod, ConfigMap shows Backoff |
| ACK | OperationDenied.NoStock ("resource is out of stock in the specified zone") | FailedScaleUp event on pod, ESS ScaleOutError |
| TKE | ResourceInsufficient (CVM stock) via AS API | ScaleUpFailed event, ConfigMap shows Backoff |
| EKS (CA) | InsufficientInstanceCapacity (EC2 error) | FailedScaleUp event on pod, ConfigMap shows Backoff |
| EKS (Karpenter) | InsufficientInstanceCapacity | Event on NodeClaim, NodeClaim condition shows failure |

### Common pattern

All providers follow the same flow on stock exhaustion:
1. Autoscaler attempts scale-up
2. Cloud provider API returns capacity error
3. Autoscaler emits FailedScaleUp event on the pod
4. Autoscaler enters backoff for that nodegroup (up to 30min after repeated failures)
5. ConfigMap reflects Backoff status for the nodegroup

The controller reads the ConfigMap + events to surface this per nodepool.

## Provider-Specific Scheduling Quirks

### GKE Autopilot

- Nodepools exist but are fully Google-managed (same label applies)
- Users control scheduling via ComputeClasses, not nodepool config
- Node-level access restricted (no SSH, no privileged DaemonSets)
- Scheduling diagnosis is less meaningful since users don't control nodepools

### TKE Super Nodes

- Serverless scheduling layer (pods run on managed infra)
- Appear as Node objects with special types if enabled
- Not currently enabled on Rey's clusters
- Have configurable taints/labels, pods auto-overflow when CVM nodes full

### ACK Managed vs Self-managed

- Managed: ACK installs cluster-autoscaler automatically, supports ~20 autoscaled nodepools
- Self-managed: deploy cluster-autoscaler yourself, integrate with ESS directly
- Both use ESS scaling groups, same `alibabacloud.com/nodepool-id` label

### EKS Karpenter

- Replaces nodegroup concept with NodePool CRD
- No ASG underneath; Karpenter provisions EC2 instances directly
- Status via NodePool/NodeClaim CRDs, not ConfigMap
- Controller needs separate code path for Karpenter clusters

## RBAC

Standard Kubernetes RBAC works identically across all 4 providers. No provider-specific quirks for reading pods, nodes, configmaps, events.

### Required ClusterRole

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: fitcheck
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
    # scope to kube-system via resourceNames or namespace restriction
  - apiGroups: [""]
    resources: [events]
    verbs: [create, patch]
```

### Cloud API access (optional, for richer stock error detail)

If the controller wants to query cloud APIs for detailed capacity errors:
- GKE: Workload Identity (GKE SA -> GCP SA)
- ACK: RRSA (RAM Roles for Service Accounts via OIDC)
- TKE: Standard RBAC sufficient for K8s-level data
- EKS: Pod Identity (recommended) or IRSA

This is optional; the ConfigMap + events provide enough signal without cloud API calls.

## Controller-Runtime Implementation

### Reconciler pattern

```go
type PodReconciler struct {
    client.Client
    Recorder record.EventRecorder
}

func (r *PodReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
    var pod corev1.Pod
    if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
        return reconcile.Result{}, client.IgnoreNotFound(err)
    }

    if pod.Status.Phase != corev1.PodPending {
        return reconcile.Result{}, nil // not pending, stop
    }

    // diagnose and emit events...

    return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
}
```

### Wiring

```go
ctrl.NewControllerManagedBy(mgr).
    For(&corev1.Pod{}).
    WithEventFilter(predicate.Funcs{
        CreateFunc: func(e event.CreateEvent) bool {
            pod := e.Object.(*corev1.Pod)
            return pod.Status.Phase == corev1.PodPending
        },
        UpdateFunc: func(e event.UpdateEvent) bool {
            pod := e.ObjectNew.(*corev1.Pod)
            return pod.Status.Phase == corev1.PodPending
        },
        DeleteFunc:  func(_ event.DeleteEvent) bool { return false },
        GenericFunc: func(_ event.GenericEvent) bool { return false },
    }).
    Complete(&PodReconciler{...})
```

### EventRecorder

```go
recorder := mgr.GetEventRecorderFor("scheduler-debugger")

// Usage in reconcile:
r.Recorder.Eventf(&pod, corev1.EventTypeWarning, "NodepoolRejected",
    "nodepool/%s: taint {%s:%s} not tolerated", nodepoolName, taintKey, taintEffect)
```

### Event deduplication

The EventRecorder auto-deduplicates via EventCorrelator in client-go. Matching on Source + InvolvedObject + Type + Reason + Message. Identical events increment `.Count` and update `.LastTimestamp`. EventAggregator kicks in at 10 identical events in 10 minutes. No manual dedup tracking needed.

### ConfigMap cache scoping

To avoid caching all ConfigMaps cluster-wide, scope the cache:

```go
mgr, _ := ctrl.NewManager(cfg, ctrl.Options{
    Cache: cache.Options{
        ByObject: map[client.Object]cache.ByObject{
            &corev1.ConfigMap{}: {
                Namespaces: map[string]cache.Config{
                    "kube-system": {},
                },
            },
        },
    },
})
```

### Requeue strategy

- Pod still Pending: `return reconcile.Result{RequeueAfter: 30 * time.Second}, nil`
- Pod scheduled/deleted: `return reconcile.Result{}, nil` (stops requeue)
- Error during reconcile: return error (triggers exponential backoff, ignores RequeueAfter)

## Configuration Matrix

The controller needs a `--provider` flag or auto-detection to select the right nodepool label:

| --provider | Nodepool label used | Autoscaler status source |
|---|---|---|
| gke | cloud.google.com/gke-nodepool | ConfigMap |
| ack | alibabacloud.com/nodepool-id | ConfigMap |
| tke | tke.cloud.tencent.com/nodepool-id | ConfigMap (keyed by ASG ID) |
| eks | eks.amazonaws.com/nodegroup | ConfigMap |
| karpenter | karpenter.sh/nodepool | NodePool/NodeClaim CRDs |
| auto | detect from node labels | detect from cluster state |

Auto-detection: list nodes, check which provider label exists. For Karpenter: check if `karpenter.sh/nodepool` label exists on any node, or if NodePool CRD is registered.

## Unknowns

- TKE stock error exact message: confidence 0.5. Could not observe `ResourceInsufficient` directly. Need to trigger a real stock exhaustion on TKE to confirm the exact error string. Mitigation: parse the event message generically for "insufficient", "stock", "capacity" keywords.
- GKE Autopilot: scheduling diagnosis is less useful since nodepools are Google-managed. May want to skip or simplify output for Autopilot clusters.
- Karpenter support adds significant complexity (CRD watches, different status model). Consider making it a v2 feature.

## Sources

### GKE
- https://docs.cloud.google.com/kubernetes-engine/docs/concepts/node-pools
- https://docs.cloud.google.com/kubernetes-engine/docs/concepts/cluster-autoscaler
- https://cloud.google.com/kubernetes-engine/docs/how-to/cluster-autoscaler-visibility
- https://docs.cloud.google.com/kubernetes-engine/docs/troubleshooting/cluster-autoscaler-scale-up

### ACK
- https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/schedule-an-application-pod-to-a-specific-node-pool
- https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/faq-about-node-auto-scaling
- https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/auto-scaling-of-nodes
- https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/alicloud

### TKE
- https://intl.cloud.tencent.com/ind/document/product/457/65833
- https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/tencentcloud
- https://github.com/TencentCloud/karpenter-provider-tke

### EKS
- https://docs.aws.amazon.com/eks/latest/userguide/managed-node-groups.html
- https://karpenter.sh/docs/concepts/nodepools/
- https://docs.aws.amazon.com/eks/latest/best-practices/cas.html

### Controller-Runtime
- https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/reconcile
- https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/predicate
- https://book.kubebuilder.io/cronjob-tutorial/controller-implementation.html
- https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md
