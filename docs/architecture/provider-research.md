# fitcheck: Implementation Research

## Provider Nodepool Labels

| Provider | Label Key | Value | Source |
|---|---|---|---|
| ACK | `alibabacloud.com/nodepool-id` | nodepool ID (e.g. `np917725...`) | [ACK docs](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/schedule-an-application-pod-to-a-specific-node-pool), verified on live clusters |
| GKE | `cloud.google.com/gke-nodepool` | pool name | [GKE docs](https://docs.cloud.google.com/kubernetes-engine/docs/concepts/node-pools) |
| TKE | `tke.cloud.tencent.com/nodepool-id` | nodepool ID | [TKE docs](https://intl.cloud.tencent.com/ind/document/product/457/65833) |
| EKS (managed) | `eks.amazonaws.com/nodegroup` | nodegroup name | [EKS docs](https://docs.aws.amazon.com/eks/latest/userguide/managed-node-groups.html) |
| EKS (Karpenter) | `karpenter.sh/nodepool` | NodePool name | [Karpenter docs](https://karpenter.sh/docs/concepts/nodepools/) |

### ACK node labels: what comes from where

Verified on live ACK clusters. Each label is categorized by origin so fitcheck only depends on guaranteed ACK defaults.

#### ACK platform labels (safe to depend on)

These labels are applied by ACK to every node automatically.

| Label | Source |
|---|---|
| `alibabacloud.com/nodepool-id` | [ACK docs](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/schedule-an-application-pod-to-a-specific-node-pool): "automatically creates a globally unique label" |
| `node.alibabacloud.com/nodepool-id` | Undocumented, identical value to above. Likely newer prefix. |
| `alibabacloud.com/ecs-instance-id` | ACK node init |
| `node.alibabacloud.com/instance-charge-type` | ACK: `PostPaid`, `PrePaid` |
| `node.alibabacloud.com/spot-strategy` | ACK: `NoSpot`, `SpotWithPriceLimit`, etc. |
| `ack.aliyun.com` | ACK cluster ID |

#### ACK GPU labels (GPU nodes only, from ACK device plugin)

| Label | Source |
|---|---|
| `aliyun.accelerator/nvidia_name` | ACK GPU plugin: e.g. `NVIDIA-L20`, `NVIDIA-A10` |
| `aliyun.accelerator/nvidia_count` | GPU count per node |
| `aliyun.accelerator/nvidia_mem` | GPU memory in MiB |
| `ack.node.gpu.schedule` | ACK GPU scheduling mode |

#### Standard Kubernetes labels

| Label | Source |
|---|---|
| `node.kubernetes.io/instance-type` | kubelet: ECS instance type (e.g. `ecs.c8i.xlarge`) |
| `kubernetes.io/arch`, `kubernetes.io/os` | kubelet |
| `topology.kubernetes.io/region`, `topology.kubernetes.io/zone` | cloud provider |

#### GOATScaler labels (present when GOATScaler is active)

| Label | Source |
|---|---|
| `goatscaler.io/managed` | GOATScaler: `true` on all managed nodes |
| `goatscaler.io/provision-task-id` | GOATScaler: scale-out task ID |

#### User-configured labels (NOT from ACK)

These are set per nodepool by the cluster operator. They vary between organizations and MUST NOT be hardcoded in fitcheck.

Common patterns observed:
- `name`: human-readable nodepool name. Configured in nodepool settings, not an ACK default.
- `environment`, `team`, `workload`, `profile`: workload routing labels specific to each organization.

**fitcheck implications:**
- Use `alibabacloud.com/nodepool-id` as the nodepool grouping key (guaranteed by ACK).
- For human-readable nodepool names in event messages, use the `name` label if present. Fall back to the nodepool ID if `name` is not set. Do not require `name` to exist.
- Do not hardcode any org-specific label keys.

**Source:** [ACK docs](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/schedule-an-application-pod-to-a-specific-node-pool), verified on live ACK clusters.

## ACK Autoscaler: Two Modes

ACK offers two mutually exclusive node autoscalers. Only one runs per cluster.

| Feature | cluster-autoscaler | GOATScaler |
|---|---|---|
| Type | Round-robin polling | Event-driven |
| Scale-out latency | 60s (standard), 50s (swift) | 35-55s |
| Success rate | ~97% | 99% |
| Status ConfigMap | `cluster-autoscaler-status` (standard format) | `autoscaler-meta` (JSON) |
| Scaler detection | Check for `cluster-autoscaler-status` CM | Check `autoscaler-meta` CM, `scaler-type` field |
| Pod events | `TriggeredScaleUp`, `FailedScaleUp`, `NotTriggerScaleUp` | `ProvisionNode`, `ProvisionNodeFailed`, `NotTriggerScaleUp`, `ResetPod` |
| Nodepool events | None | `InstanceInventoryStatusChanged` on `ACKNodePool` objects |
| Node labels | Standard | `goatscaler.io/managed=true`, `goatscaler.io/provision-task-id` |

**Source:** [ACK Node Scaling Overview](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/overview-of-node-scaling)

### GOATScaler (verified on live clusters)

All inspected ACK clusters use GOATScaler. Key findings:

**ConfigMap `autoscaler-meta`** in kube-system contains JSON with:
- `scaler-type`: `"goatscaler"`
- `scan_interval`, `expander`, `scale_up_from_zero`, etc.
- `scaling_configurations`: map of ASG ID to nodepool config (labels, taints, min/max size, instance types)

**Events observed across clusters:**

| Event Reason | Source | Target | Message Pattern |
|---|---|---|---|
| `NotTriggerScaleUp` | `GOATScaler` | Pod | `pod didn't trigger scale-up due to missing matching nodepool: N <reason>` |
| `ProvisionNode` | `GOATScaler` | Pod | `Provision node <task-id> in Zone: <zone> with InstanceType: <type>` |
| `NodePoolInventoryStatusChanged` | `GOATScaler` | ACKNodePool | `nodepool <id> inventory phase changed from <X> to <Y>` |

**`cluster-autoscaler-status` ConfigMap does NOT exist** on any GOATScaler cluster (verified across 10 clusters).

**Source:** [ACK Instant Elasticity docs](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/instant-elasticity), [GOATScaler FAQ](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/faq-about-node-instant-scaling), live cluster inspection.

### Detecting which autoscaler is active

```
1. Try Get ConfigMap "cluster-autoscaler-status" in kube-system
   -> exists: standard cluster-autoscaler
2. Try Get ConfigMap "autoscaler-meta" in kube-system
   -> exists and scaler-type == "goatscaler": GOATScaler
3. Neither exists: no autoscaler (or unknown)
```

### Impact on fitcheck

For v0.0.1 (ACK only), fitcheck must handle GOATScaler:

- **NodepoolNoStock**: read `InstanceInventoryStatusChanged` events on ACKNodePool objects (phase changed to UnHealthy)
- **NodepoolCandidate**: read `ProvisionNode` events on pods (scale-up was triggered for this pod)
- **Scale-up failure**: read `NotTriggerScaleUp` and `ProvisionNodeFailed` events on pods
- **No ConfigMap parsing needed** for autoscaler state in GOATScaler mode. Events on pods and ACKNodePool objects provide sufficient signal.

If standard cluster-autoscaler is detected instead, fall back to parsing `cluster-autoscaler-status` ConfigMap.

## Stock Exhaustion Signals

### ACK with GOATScaler (verified)

GOATScaler emits `InstanceInventoryStatusChanged` events on ACKNodePool objects when ECS inventory becomes unavailable. The event message contains the phase transition (e.g. `Healthy to UnHealthy`).

GOATScaler limitations from official docs:
- Only simulates CPU, memory, ephemeral storage, GPU for scheduling fit
- Cannot account for pod-level storage constraints or zone-specific requirements
- Cannot verify preemptible instance inventory

**Source:** [GOATScaler FAQ](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/faq-about-node-instant-scaling)

### ACK with cluster-autoscaler

Standard flow: `FailedScaleUp` event on pod, `cluster-autoscaler-status` ConfigMap shows Backoff. ACK-specific error: `OperationDenied.NoStock` from ESS API.

**Source:** [ACK Autoscaling FAQ](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/faq-about-node-auto-scaling), [upstream alicloud provider](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/alicloud)

### Other providers

| Provider | Error Signal | Where |
|---|---|---|
| GKE | `ZONE_RESOURCE_POOL_EXHAUSTED`, `GCE_STOCKOUT` | `ScaleUpFailed` event on pod |
| TKE | `ResourceInsufficient` | `ScaleUpFailed` event, ConfigMap Backoff |
| EKS (CA) | `InsufficientInstanceCapacity` | `FailedScaleUp` event on pod |
| EKS (Karpenter) | `InsufficientInstanceCapacity` | Event on NodeClaim |

## Node Taints (verified on live ACK clusters)

GPU nodes typically have taints. Non-GPU nodes typically have zero custom taints.

### Standard taints (from K8s or NVIDIA device plugin)

| Taint | Effect | Source |
|---|---|---|
| `nvidia.com/gpu=present` | NoSchedule | NVIDIA device plugin (standard, any K8s cluster with GPUs) |
| `node.kubernetes.io/disk-pressure` | NoSchedule | kubelet (automatic, transient) |
| `node.kubernetes.io/unschedulable` | NoSchedule | kubelet (cordoned nodes) |

### User-configured taints (NOT from ACK)

Cluster operators commonly add custom taints to GPU nodepools or specialized nodepools for workload isolation (e.g. team-scoped, workload-type, GPU profile). These vary per organization.

**fitcheck implications:**
- fitcheck checks ALL taints on nodes against ALL tolerations on pods. It does not need to know which taints are standard vs custom.
- The taint/toleration check is purely mechanical: for each taint on the node, does the pod have a matching toleration?
- ACK does not auto-apply GPU taints. The `nvidia.com/gpu=present:NoSchedule` taint comes from the NVIDIA device plugin, which is standard across any K8s cluster with GPU nodes.

## GPU Resources (verified on live ACK clusters)

GPU availability is in `node.status.allocatable`:

```json
{
    "cpu": "7488m",
    "memory": "61715Mi",
    "nvidia.com/gpu": "1",
    "pods": "128"
}
```

fitcheck checks `pod.spec.containers[].resources.requests` against `node.status.allocatable` for cpu, memory, and `nvidia.com/gpu`.

## RBAC

Standard Kubernetes RBAC works identically across all providers.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: fitcheck
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

Notes:
- Pods need `patch` for writing the `fitcheck.io/diagnosis` annotation.
- The `events.k8s.io` group is needed for the newer Events API.
- Events need `get, list` (not just `create, patch`) so fitcheck can read GOATScaler events on pods.

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
recorder := mgr.GetEventRecorderFor("fitcheck")

// Usage in reconcile (single event per reconcile):
r.Recorder.Eventf(&pod, corev1.EventTypeWarning, "FitcheckDiagnosis",
    "2/13 nodepools fit | rejected: 8 taint, 2 affinity | no-stock: 2 | candidate: 1")
```

Full per-nodepool detail is written to the `fitcheck.io/diagnosis` annotation on the pod rather than emitted as separate events.

### Event deduplication

The EventRecorder auto-deduplicates via EventCorrelator in client-go. Matching on Source + InvolvedObject + Type + Reason + Message. Identical events increment `.Count` and update `.LastTimestamp`. No manual dedup tracking needed.

### ConfigMap cache scoping

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
- Error during reconcile: return error (triggers exponential backoff)

## Configuration Matrix

Provider is auto-detected from cluster node labels at startup. No flag needed.

| Provider | Nodepool label | Autoscaler status source |
|---|---|---|
| ACK | `alibabacloud.com/nodepool-id` | GOATScaler events or `cluster-autoscaler-status` CM (auto-detect) |
| GKE | `cloud.google.com/gke-nodepool` | `cluster-autoscaler-status` CM |
| TKE | `tke.cloud.tencent.com/nodepool-id` | `cluster-autoscaler-status` CM (keyed by ASG ID) |
| EKS | `eks.amazonaws.com/nodegroup` | `cluster-autoscaler-status` CM |
| Karpenter | `karpenter.sh/nodepool` | NodePool/NodeClaim CRDs |

## Unknowns

- **GOATScaler `ProvisionNodeFailed` event**: documented but not observed on live clusters (no stock exhaustion occurred during inspection). Message format unknown.
- **ACK with standard cluster-autoscaler**: no test clusters available with this config. Behavior assumed to match upstream.
- TKE stock error exact message: confidence 0.5. Needs real observation.
- Karpenter support: significant complexity (CRD watches). Consider v2.

## Sources

### ACK
- https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/overview-of-node-scaling
- https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/instant-elasticity
- https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/faq-about-node-instant-scaling
- https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/schedule-an-application-pod-to-a-specific-node-pool
- https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/auto-scaling-of-nodes
- https://github.com/AliyunContainerService/GOATScaler-Samples
- https://registry.terraform.io/providers/aliyun/alicloud/latest/docs/resources/cs_autoscaling_config
- https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/alicloud

### GKE
- https://docs.cloud.google.com/kubernetes-engine/docs/concepts/node-pools
- https://docs.cloud.google.com/kubernetes-engine/docs/concepts/cluster-autoscaler

### TKE
- https://intl.cloud.tencent.com/ind/document/product/457/65833
- https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler/cloudprovider/tencentcloud

### EKS
- https://docs.aws.amazon.com/eks/latest/userguide/managed-node-groups.html
- https://karpenter.sh/docs/concepts/nodepools/

### Controller-Runtime
- https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/reconcile
- https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/predicate
- https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md
