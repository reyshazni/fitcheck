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

Verified across multiple ACK clusters. Each label is categorized by origin so fitcheck only depends on guaranteed ACK defaults.

#### ACK platform labels (present on every ACK node, safe to depend on)

| Label | Present | Source |
|---|---|---|
| `alibabacloud.com/nodepool-id` | 24/24 | [ACK docs](https://www.alibabacloud.com/help/en/ack/ack-managed-and-ack-dedicated/user-guide/schedule-an-application-pod-to-a-specific-node-pool): "automatically creates a globally unique label" |
| `node.alibabacloud.com/nodepool-id` | 24/24 | Undocumented, identical value to above. Likely newer prefix. |
| `alibabacloud.com/ecs-instance-id` | 24/24 | ACK node init |
| `node.alibabacloud.com/instance-charge-type` | 24/24 | ACK: `PostPaid`, `PrePaid` |
| `node.alibabacloud.com/spot-strategy` | 24/24 | ACK: `NoSpot`, `SpotWithPriceLimit`, etc. |
| `ack.aliyun.com` | 24/24 | ACK cluster ID |
| `k8s.aliyun.com` | 24/24 | ACK marker |

#### ACK GPU labels (present on GPU nodes, from ACK device plugin)

| Label | Present | Source |
|---|---|---|
| `aliyun.accelerator/nvidia_name` | 3/24 | ACK GPU plugin: `NVIDIA-L20`, `NVIDIA-A10` |
| `aliyun.accelerator/nvidia_count` | 2/24 | GPU count per node |
| `aliyun.accelerator/nvidia_mem` | 2/24 | GPU memory (e.g. `46068MiB`) |
| `ack.node.gpu.schedule` | 3/24 | ACK GPU scheduling mode: `default` |

#### Standard Kubernetes labels (present on every node)

| Label | Source |
|---|---|
| `node.kubernetes.io/instance-type` | kubelet: ECS instance type (e.g. `ecs.c8i.xlarge`) |
| `kubernetes.io/arch`, `kubernetes.io/os` | kubelet |
| `topology.kubernetes.io/region`, `topology.kubernetes.io/zone` | cloud provider |

#### GOATScaler labels (present when GOATScaler is active, from ACK)

| Label | Present | Source |
|---|---|---|
| `goatscaler.io/managed` | 24/24 | GOATScaler: `true` on all managed nodes |
| `goatscaler.io/provision-task-id` | 24/24 | GOATScaler: scale-out task ID |

#### User-configured labels (org-specific, NOT from ACK)

These are set per nodepool by the cluster operator. They vary between organizations and MUST NOT be hardcoded in fitcheck.

| Label | Example | Notes |
|---|---|---|
| `name` | `compute-optimized-nodepool-01` | User-defined nodepool name. Configured in nodepool settings, NOT an ACK default. Present on all nodes but value is org-specific. |
| `environment` | `staging`, `production` | Org-specific |
| `stream`, `pod`, `policy` | varies | Org-specific workload routing |
| `component` | `kubernetes-nodepool` | Org-specific |
| `node.gopay.sh/*` | `lifecycle=on_demand` | Org-specific |
| `caraml/*` | `nvidia-l20=enabled` | Org-specific ML platform |
| `team`, `workload`, `profile` | varies | Org-specific workload isolation |

**fitcheck implications:**
- Use `alibabacloud.com/nodepool-id` as the nodepool grouping key (guaranteed by ACK).
- For human-readable nodepool names in event messages, use the `name` label if present. Fall back to the nodepool ID if `name` is not set. Do not require `name` to exist.
- Do not hardcode any org-specific label keys.

**Source:** Live cluster inspection across ACK clusters, cross-referenced with `autoscaler-meta` ConfigMap `scaling_configurations.labels` field.

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

Only 3/24 nodes (GPU nodes) have custom taints. Non-GPU nodes have zero custom taints.

### Standard taints (from K8s or NVIDIA device plugin)

| Taint | Effect | Source |
|---|---|---|
| `nvidia.com/gpu=present` | NoSchedule | NVIDIA device plugin (standard, any K8s cluster with GPUs) |
| `node.kubernetes.io/disk-pressure` | NoSchedule | kubelet (automatic, transient) |
| `node.kubernetes.io/unschedulable` | NoSchedule | kubelet (cordoned nodes) |

### User-configured taints (org-specific, NOT from ACK)

These are set per nodepool by the cluster operator. They vary between organizations.

| Taint | Example | Org |
|---|---|---|
| `node.gopay.sh/gpu=nvidia` | NoSchedule | org-specific |
| `caraml/nvidia-l20=enabled` | NoSchedule | org-specific |
| `profile=l20-1x` | NoSchedule | org-specific |
| `team=ds-identity` | NoSchedule | org-specific |
| `workload=model` | NoSchedule | org-specific |
| `node_type=memory-optimized-amd` | NoSchedule | org-specific |

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
    verbs: [get, list, watch]
  - apiGroups: [""]
    resources: [nodes]
    verbs: [get, list]
  - apiGroups: [""]
    resources: [configmaps]
    verbs: [get]
  - apiGroups: [""]
    resources: [events]
    verbs: [get, list, create, patch]
```

Note: events now need `get, list` (not just `create, patch`) so fitcheck can read GOATScaler events on pods.

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

// Usage in reconcile:
r.Recorder.Eventf(&pod, corev1.EventTypeWarning, "NodepoolRejected",
    "nodepool/%s: taint {%s:%s} not tolerated", nodepoolName, taintKey, taintEffect)
```

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

| --provider | Nodepool label | Autoscaler status source |
|---|---|---|
| ack | `alibabacloud.com/nodepool-id` | GOATScaler events or `cluster-autoscaler-status` CM (auto-detect) |
| gke | `cloud.google.com/gke-nodepool` | `cluster-autoscaler-status` CM |
| tke | `tke.cloud.tencent.com/nodepool-id` | `cluster-autoscaler-status` CM (keyed by ASG ID) |
| eks | `eks.amazonaws.com/nodegroup` | `cluster-autoscaler-status` CM |
| karpenter | `karpenter.sh/nodepool` | NodePool/NodeClaim CRDs |
| auto | detect from node labels | detect from cluster state |

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
