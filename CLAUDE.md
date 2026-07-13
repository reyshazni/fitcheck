# CLAUDE.md

## Project

fitcheck is a Kubernetes controller that watches Pending pods and emits per-nodepool diagnostic Events. It evaluates scheduling fit (taints, nodeSelector, affinity, resources, anti-affinity, topology spread) and autoscaler state per nodepool, then writes the results as Events on the pod, visible via `kubectl describe pod` with zero client-side tooling.

## Tech Stack

- **Language**: Go
- **Controller framework**: controller-runtime
- **Testing**: envtest (integration), Go standard testing (unit)
- **Distribution**: Helm chart (OCI via ghcr.io), container image (ghcr.io, multi-arch amd64+arm64)
- **CI/CD**: GitHub Actions (build image + package Helm chart)
- **Container**: Multi-stage Docker build

## Architecture

Single binary, one reconciler. Watches Pending pods, groups cluster nodes by nodepool label, checks scheduling dimensions in order, reads cluster-autoscaler-status ConfigMap for scaling state, emits Events on the pod.

**PodReconciler** is the core loop:
1. Pod enters Pending, which triggers reconcile
2. Wait `--initial-delay` (10s) for scheduler to attempt first
3. List nodes, group by nodepool label
4. Per nodepool: check taints, nodeSelector, affinity, resources, anti-affinity, topology spread
5. Read autoscaler ConfigMap for scaling state per nodegroup
6. Emit Events: NodepoolAccepted, NodepoolRejected, NodepoolCandidate, NodepoolNoStock
7. Requeue every `--recheck-interval` (30s) while pod remains Pending
8. Stop when pod is Scheduled or deleted

Event deduplication handled by standard Kubernetes EventRecorder (EventCorrelator in client-go).

## Repository Layout

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
    provider-research.md    # provider labels, autoscaler behavior, stock errors
  feature/
    scheduler-diagnostics.md  # feature spec (event types, message format, behavior)
  plans/                    # implementation plans
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

## Provider Support

| Provider | Nodepool label | Autoscaler source |
|---|---|---|
| ACK (Alibaba) | `alibabacloud.com/nodepool-id` | ConfigMap |
| GKE | `cloud.google.com/gke-nodepool` | ConfigMap |
| EKS (managed) | `eks.amazonaws.com/nodegroup` | ConfigMap |
| EKS (Karpenter) | `karpenter.sh/nodepool` | NodePool/NodeClaim CRDs |
| TKE (Tencent) | `tke.cloud.tencent.com/nodepool-id` | ConfigMap |

Auto-detection via `--provider=auto`.

## Commands

```bash
make build          # compile to bin/fitcheck
make test           # all tests with envtest
make lint           # golangci-lint run ./...
make run            # build and run
make docker-build   # build container image
```

## RBAC

Minimal: read pods/nodes/configmaps, create events. Runs in kube-system as a single-replica Deployment.

## Deployment

- Runs in kube-system as a single-replica Deployment
- Low resource footprint (only processes Pending pods)
- Helm chart is the primary install method
- `install.yaml` release asset for quick evaluation

## Code Conventions

### Linting

golangci-lint with `.golangci.yml` is mandatory. All code must pass `make lint` with zero issues.

- Zero `//nolint` directives. If a function is too long or complex, split it.
- `funlen`: max 80 lines, 50 statements per function.
- `cyclop`: max cyclomatic complexity 15.
- `gocognit`: max cognitive complexity 20.
- `interfacebloat`: max 8 methods per interface.
- `gosec`: all rules except G104 (unhandled errors on deferred Close).
- `exhaustive`: all switch statements on typed enums must cover every case. No implicit default fallthrough.
- `wrapcheck`: errors from external packages must be wrapped with `fmt.Errorf("context: %w", err)`. Never return a bare external error.
- `dupl`: duplicate code blocks over 100 tokens are rejected. Extract shared logic into a reusable function.
- `goconst`: repeated string/number literals (3+ chars, 2+ occurrences) must be extracted to named constants.

### Code Reuse

- Never copy-paste logic. If the same pattern exists in two places, extract it into a shared function or package.
- When adding code that resembles existing code, find the existing code first and extract a common helper.
- Shared helpers go in the most specific package that both callers can import. If no such package exists, create one under `internal/`.

### Style

- Follow standard Go conventions (gofmt, goimports).
- Imports: stdlib, blank line, external, blank line, internal.
- Receiver names: 1-2 letters, consistent across methods.
- Declaration order: type, const, var, func (enforced by `decorder`).

### Error handling

- Handle an error exactly once: return it OR log it. Never both.
- Happy path at left margin. No else after error return.
- **HARD RULE: zero 500 Internal Server Error responses.** Every HTTP handler must catch all possible errors and return the correct HTTP status code. No error may bubble up uncaught into a generic 500.
- Every code path that can fail must have an explicit error check with a specific HTTP response code (400, 401, 403, 404, 409, 422, 503, etc.).
- Error responses must include a structured JSON body with a human-readable message. Never return a bare status code.
- If an external dependency fails (upstream API, ConfigMap read), return 502 or 503 with context, not 500.
- Test coverage for error paths is mandatory. Every handler test must include cases for each error branch, verifying the exact status code returned.

### Formatting

- No em dashes (U+2014/U+2013) anywhere.
- No double hyphens as punctuation. Use commas, semicolons, periods, or rewrite.
- The only acceptable `--` is CLI flags and required code syntax.

## Non-goals

- Does not modify scheduling decisions
- Does not replace the scheduler or autoscaler
- Does not process non-Pending pods
- Does not implement its own scaling logic

## Documentation

| Location | Content |
|---|---|
| `README.md` | Project overview, quick start, configuration |
| `docs/architecture/overview.md` | Architecture, reconciler lifecycle, provider support |
| `docs/architecture/provider-research.md` | Provider labels, autoscaler ConfigMap format, stock errors |
| `docs/feature/scheduler-diagnostics.md` | Feature spec: event types, message format, rejection categories |
| `docs/plans/` | Implementation plans (TDD, step-by-step) |
