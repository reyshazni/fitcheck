# Annotation Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED: Follow the `test-driven-development` skill for every task. Use `coding:execute` to implement task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two-layer diagnostic output: compact one-line event summary + full structured JSON annotation on pods. Pattern follows kube-scheduler-simulator (official K8s SIG project).

**Architecture:** `internal/diagnosis/annotation.go` builds a `DiagnosisReport` struct and serializes to JSON. `internal/diagnosis/summary.go` is rewritten to produce a one-line summary from the report. Reconciler writes the annotation via strategic merge PATCH (non-fatal on failure) and emits the summary event. Annotation is cleaned up when pod leaves Pending state.

**Tech Stack:** Go 1.26, controller-runtime v0.24.1

---

## Annotation key

`fitcheck.io/diagnosis`

## Annotation JSON format

```json
{
  "timestamp": "2026-07-13T17:55:31Z",
  "summary": "2/13 nodepools fit | rejected: 8 taint, 2 affinity | no-stock: 2 | candidate: 1",
  "nodepools": [
    {"name": "general-pool", "verdict": "accepted", "fitting": 3, "total": 5},
    {"name": "gpu-pool", "verdict": "rejected", "reason": "taint nvidia.com/gpu=present:NoSchedule not tolerated", "category": "taint"},
    {"name": "spot-pool", "verdict": "no-stock", "reason": "inventory unhealthy"},
    {"name": "highmem-pool", "verdict": "candidate", "reason": "scale-up triggered"}
  ]
}
```

## Event format (replaces current FormatSummary)

Single event, one line:
```
2/13 nodepools fit | rejected: 8 taint, 2 affinity | no-stock: 2 | candidate: 1
```

If all fit: `13/13 nodepools fit`
If none fit: `0/13 nodepools fit | rejected: 8 taint, 3 affinity, 2 resource`

## New types in `internal/diagnosis/annotation.go`

```go
const AnnotationKey = "fitcheck.io/diagnosis"

type DiagnosisReport struct {
	Timestamp string           `json:"timestamp"`
	Summary   string           `json:"summary"`
	Nodepools []NodepoolResult `json:"nodepools"`
}

type NodepoolResult struct {
	Name     string `json:"name"`
	Verdict  string `json:"verdict"`
	Fitting  int    `json:"fitting,omitempty"`
	Total    int    `json:"total,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Category string `json:"category,omitempty"`
}
```

## Functions

**`internal/diagnosis/annotation.go`:**
```go
func BuildReport(diagnoses []NodepoolDiagnosis) DiagnosisReport
func MarshalReport(report DiagnosisReport) (string, error)
```

**`internal/diagnosis/summary.go` (rewrite):**
```go
func FormatEventSummary(diagnoses []NodepoolDiagnosis) string
```
Returns: `"2/13 nodepools fit | rejected: 8 taint, 2 affinity | no-stock: 2 | candidate: 1"`

The old `FormatSummary` returning `(normal, warning string)` is deleted. The new function returns a single string used for both Normal and Warning events. Event type is determined by whether any non-accepted verdicts exist.

**`internal/controller/reconciler.go` changes:**
```go
func (r *PodReconciler) writeAnnotation(ctx context.Context, pod *corev1.Pod, diagnoses []diagnosis.NodepoolDiagnosis) {
	report := diagnosis.BuildReport(diagnoses)
	data, err := diagnosis.MarshalReport(report)
	if err != nil {
		slog.Warn("failed to marshal diagnosis report", "error", err, "pod", pod.Name)
		return
	}

	patch := client.MergeFrom(pod.DeepCopy())
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[diagnosis.AnnotationKey] = data

	if err := r.Client.Patch(ctx, pod, patch); err != nil {
		slog.Warn("failed to write diagnosis annotation", "error", err, "pod", pod.Name)
	}
}

func (r *PodReconciler) removeAnnotation(ctx context.Context, pod *corev1.Pod) {
	if _, ok := pod.Annotations[diagnosis.AnnotationKey]; !ok {
		return
	}

	patch := client.MergeFrom(pod.DeepCopy())
	delete(pod.Annotations, diagnosis.AnnotationKey)

	if err := r.Client.Patch(ctx, pod, patch); err != nil {
		slog.Warn("failed to remove diagnosis annotation", "error", err, "pod", pod.Name)
	}
}
```

Update `emitEvents` to use single event:
```go
func (r *PodReconciler) emitEvents(pod *corev1.Pod, diagnoses []diagnosis.NodepoolDiagnosis) {
	summary := diagnosis.FormatEventSummary(diagnoses)
	if summary == "" {
		return
	}

	eventType := corev1.EventTypeNormal
	for _, d := range diagnoses {
		if d.Verdict != diagnosis.Accepted {
			eventType = corev1.EventTypeWarning
			break
		}
	}

	r.Recorder.Eventf(pod, nil, eventType, "FitcheckDiagnosis", "diagnose", summary)
}
```

Update `Reconcile` to add annotation write + cleanup:
```go
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod corev1.Pod
	if err := r.Client.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("getting pod: %w", err)
	}

	if pod.Status.Phase != corev1.PodPending {
		r.removeAnnotation(ctx, &pod)
		return ctrl.Result{}, nil
	}

	if remaining := r.remainingDelay(&pod); remaining > 0 {
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	diagnoses, err := r.diagnose(ctx, &pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.writeAnnotation(ctx, &pod, diagnoses)
	r.emitEvents(&pod, diagnoses)

	return ctrl.Result{RequeueAfter: r.RecheckInterval}, nil
}
```

## RBAC change

Add `patch` to pods in ClusterRole:
```yaml
- apiGroups: [""]
  resources: [pods]
  verbs: [get, list, watch, patch]
```

---

## Tasks

### Task 1: DiagnosisReport types and BuildReport

File: `internal/diagnosis/annotation.go`
Test: `internal/diagnosis/annotation_test.go`

- [ ] **Step 1: Write failing test (RED)**

```go
// internal/diagnosis/annotation_test.go
package diagnosis_test

import (
	"testing"
	"time"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

func TestBuildReport_MixedVerdicts(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: "general-pool",
			Verdict:      diagnosis.Accepted,
			FittingNodes: 3,
			TotalNodes:   5,
		},
		{
			NodepoolName: "highmem-pool",
			Verdict:      diagnosis.Accepted,
			FittingNodes: 2,
			TotalNodes:   2,
		},
		{
			NodepoolName: "gpu-pool-a",
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryTaint,
				Reason:   "taint nvidia.com/gpu=present:NoSchedule not tolerated",
			},
			TotalNodes: 4,
		},
		{
			NodepoolName: "gpu-pool-b",
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryTaint,
				Reason:   "taint dedicated=gpu:NoSchedule not tolerated",
			},
			TotalNodes: 2,
		},
		{
			NodepoolName: "arm-pool",
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryAffinity,
				Reason:   "required node affinity not matched",
			},
			TotalNodes: 3,
		},
		{
			NodepoolName: "spot-pool",
			Verdict:      diagnosis.NoStock,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryResources,
				Reason:   "inventory unhealthy",
			},
			TotalNodes: 0,
		},
		{
			NodepoolName: "scaling-pool",
			Verdict:      diagnosis.Candidate,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryResources,
				Reason:   "scale-up triggered",
			},
			TotalNodes: 1,
		},
	}

	report := diagnosis.BuildReport(diagnoses)

	// Verify timestamp is recent (within last 5 seconds).
	ts, err := time.Parse(time.RFC3339, report.Timestamp)
	if err != nil {
		t.Fatalf("timestamp parse error: %v", err)
	}

	if time.Since(ts) > 5*time.Second {
		t.Errorf("timestamp too old: %v", report.Timestamp)
	}

	// Verify summary is generated.
	wantSummary := "2/7 nodepools fit | rejected: 2 taint, 1 affinity | no-stock: 1 | candidate: 1"
	if report.Summary != wantSummary {
		t.Errorf("summary = %q, want %q", report.Summary, wantSummary)
	}

	// Verify nodepool count.
	if len(report.Nodepools) != 7 {
		t.Fatalf("nodepools count = %d, want 7", len(report.Nodepools))
	}

	// Verify accepted nodepool fields.
	np0 := report.Nodepools[0]
	if np0.Name != "general-pool" {
		t.Errorf("nodepools[0].Name = %q, want %q", np0.Name, "general-pool")
	}
	if np0.Verdict != "accepted" {
		t.Errorf("nodepools[0].Verdict = %q, want %q", np0.Verdict, "accepted")
	}
	if np0.Fitting != 3 {
		t.Errorf("nodepools[0].Fitting = %d, want 3", np0.Fitting)
	}
	if np0.Total != 5 {
		t.Errorf("nodepools[0].Total = %d, want 5", np0.Total)
	}
	if np0.Reason != "" {
		t.Errorf("nodepools[0].Reason = %q, want empty", np0.Reason)
	}
	if np0.Category != "" {
		t.Errorf("nodepools[0].Category = %q, want empty", np0.Category)
	}

	// Verify rejected nodepool fields.
	np2 := report.Nodepools[2]
	if np2.Name != "gpu-pool-a" {
		t.Errorf("nodepools[2].Name = %q, want %q", np2.Name, "gpu-pool-a")
	}
	if np2.Verdict != "rejected" {
		t.Errorf("nodepools[2].Verdict = %q, want %q", np2.Verdict, "rejected")
	}
	if np2.Reason != "taint nvidia.com/gpu=present:NoSchedule not tolerated" {
		t.Errorf("nodepools[2].Reason = %q", np2.Reason)
	}
	if np2.Category != "taint" {
		t.Errorf("nodepools[2].Category = %q, want %q", np2.Category, "taint")
	}

	// Verify no-stock nodepool fields.
	np5 := report.Nodepools[5]
	if np5.Verdict != "no-stock" {
		t.Errorf("nodepools[5].Verdict = %q, want %q", np5.Verdict, "no-stock")
	}

	// Verify candidate nodepool fields.
	np6 := report.Nodepools[6]
	if np6.Verdict != "candidate" {
		t.Errorf("nodepools[6].Verdict = %q, want %q", np6.Verdict, "candidate")
	}
}

func TestBuildReport_AllAccepted(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: "pool-a",
			Verdict:      diagnosis.Accepted,
			FittingNodes: 2,
			TotalNodes:   3,
		},
		{
			NodepoolName: "pool-b",
			Verdict:      diagnosis.Accepted,
			FittingNodes: 1,
			TotalNodes:   1,
		},
	}

	report := diagnosis.BuildReport(diagnoses)

	wantSummary := "2/2 nodepools fit"
	if report.Summary != wantSummary {
		t.Errorf("summary = %q, want %q", report.Summary, wantSummary)
	}
}

func TestBuildReport_Empty(t *testing.T) {
	report := diagnosis.BuildReport(nil)

	if report.Summary != "" {
		t.Errorf("summary = %q, want empty", report.Summary)
	}

	if len(report.Nodepools) != 0 {
		t.Errorf("nodepools = %d, want 0", len(report.Nodepools))
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestBuildReport -count=1`
Expected: compilation error, `BuildReport` undefined

- [ ] **Step 3: Write minimal implementation (GREEN)**

```go
// internal/diagnosis/annotation.go
package diagnosis

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const AnnotationKey = "fitcheck.io/diagnosis"

type DiagnosisReport struct {
	Timestamp string           `json:"timestamp"`
	Summary   string           `json:"summary"`
	Nodepools []NodepoolResult `json:"nodepools"`
}

type NodepoolResult struct {
	Name     string `json:"name"`
	Verdict  string `json:"verdict"`
	Fitting  int    `json:"fitting,omitempty"`
	Total    int    `json:"total,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Category string `json:"category,omitempty"`
}

func BuildReport(diagnoses []NodepoolDiagnosis) DiagnosisReport {
	if len(diagnoses) == 0 {
		return DiagnosisReport{}
	}

	report := DiagnosisReport{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Nodepools: make([]NodepoolResult, len(diagnoses)),
	}

	for i, d := range diagnoses {
		report.Nodepools[i] = buildNodepoolResult(d)
	}

	report.Summary = FormatEventSummary(diagnoses)

	return report
}

func buildNodepoolResult(d NodepoolDiagnosis) NodepoolResult {
	r := NodepoolResult{
		Name:    d.NodepoolName,
		Verdict: verdictToString(d.Verdict),
	}

	if d.Verdict == Accepted {
		r.Fitting = d.FittingNodes
		r.Total = d.TotalNodes
	} else if d.Rejection != nil {
		r.Reason = d.Rejection.Reason
		r.Category = categoryToString(d.Rejection.Category)
	}

	return r
}

func verdictToString(v Verdict) string {
	switch v {
	case Accepted:
		return "accepted"
	case Rejected:
		return "rejected"
	case NoStock:
		return "no-stock"
	case Candidate:
		return "candidate"
	default:
		return strings.ToLower(string(v))
	}
}

func categoryToString(c RejectionCategory) string {
	switch c {
	case CategoryTaint:
		return "taint"
	case CategoryNodeSelector:
		return "selector"
	case CategoryAffinity:
		return "affinity"
	case CategoryResources:
		return "resource"
	default:
		return ""
	}
}
```

Note: `BuildReport` depends on `FormatEventSummary` from Task 3. During Task 1 implementation, add a temporary stub for `FormatEventSummary` in `summary.go` that returns `""`, or implement Task 3 first. Recommended order: implement Task 3 before Task 1.

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestBuildReport -count=1`
Expected: ALL PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/diagnosis/...`

- [ ] **Step 6: Commit**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && git add internal/diagnosis/annotation.go internal/diagnosis/annotation_test.go && git commit -m "feat: add diagnosis report types"`

---

### Task 2: MarshalReport

File: `internal/diagnosis/annotation.go`
Test: `internal/diagnosis/annotation_test.go`

- [ ] **Step 1: Write failing test (RED)**

Append to `internal/diagnosis/annotation_test.go`:

```go
func TestMarshalReport_RoundTrip(t *testing.T) {
	report := diagnosis.DiagnosisReport{
		Timestamp: "2026-07-13T17:55:31Z",
		Summary:   "1/2 nodepools fit | rejected: 1 taint",
		Nodepools: []diagnosis.NodepoolResult{
			{
				Name:    "general-pool",
				Verdict: "accepted",
				Fitting: 3,
				Total:   5,
			},
			{
				Name:     "gpu-pool",
				Verdict:  "rejected",
				Reason:   "taint nvidia:NoSchedule not tolerated",
				Category: "taint",
			},
		},
	}

	data, err := diagnosis.MarshalReport(report)
	if err != nil {
		t.Fatalf("MarshalReport() error: %v", err)
	}

	if data == "" {
		t.Fatal("MarshalReport() returned empty string")
	}

	// Verify it is valid JSON by unmarshaling back.
	var decoded diagnosis.DiagnosisReport
	if err := json.Unmarshal([]byte(data), &decoded); err != nil {
		t.Fatalf("round-trip unmarshal error: %v", err)
	}

	if decoded.Timestamp != report.Timestamp {
		t.Errorf("timestamp = %q, want %q", decoded.Timestamp, report.Timestamp)
	}

	if decoded.Summary != report.Summary {
		t.Errorf("summary = %q, want %q", decoded.Summary, report.Summary)
	}

	if len(decoded.Nodepools) != 2 {
		t.Fatalf("nodepools count = %d, want 2", len(decoded.Nodepools))
	}

	if decoded.Nodepools[0].Fitting != 3 {
		t.Errorf("nodepools[0].Fitting = %d, want 3", decoded.Nodepools[0].Fitting)
	}

	if decoded.Nodepools[1].Category != "taint" {
		t.Errorf("nodepools[1].Category = %q, want %q", decoded.Nodepools[1].Category, "taint")
	}

	// Verify omitempty: accepted pool should not have reason/category in JSON.
	if strings.Contains(data, `"reason":""`) {
		t.Error("JSON should omit empty reason field")
	}

	if strings.Contains(data, `"category":""`) {
		t.Error("JSON should omit empty category field")
	}
}
```

Add `"encoding/json"` and `"strings"` to the import block of `annotation_test.go`.

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestMarshalReport -count=1`
Expected: compilation error, `MarshalReport` undefined

- [ ] **Step 3: Write minimal implementation (GREEN)**

Add to `internal/diagnosis/annotation.go`:

```go
import "encoding/json"
```

```go
func MarshalReport(report DiagnosisReport) (string, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return "", fmt.Errorf("marshaling diagnosis report: %w", err)
	}

	return string(data), nil
}
```

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestMarshalReport -count=1`
Expected: ALL PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/diagnosis/...`

- [ ] **Step 6: Commit**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && git add internal/diagnosis/annotation.go internal/diagnosis/annotation_test.go && git commit -m "feat: add marshal diagnosis report"`

---

### Task 3: FormatEventSummary (rewrite summary.go)

File: `internal/diagnosis/summary.go` (rewrite)
Test: `internal/diagnosis/summary_test.go` (rewrite)

- [ ] **Step 1: Write failing test (RED)**

Replace entire contents of `internal/diagnosis/summary_test.go`:

```go
package diagnosis_test

import (
	"testing"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

func TestFormatEventSummary_AllAccepted(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{NodepoolName: "pool-a", Verdict: diagnosis.Accepted, FittingNodes: 2, TotalNodes: 3},
		{NodepoolName: "pool-b", Verdict: diagnosis.Accepted, FittingNodes: 1, TotalNodes: 1},
		{NodepoolName: "pool-c", Verdict: diagnosis.Accepted, FittingNodes: 5, TotalNodes: 5},
	}

	got := diagnosis.FormatEventSummary(diagnoses)
	want := "3/3 nodepools fit"

	if got != want {
		t.Errorf("FormatEventSummary() = %q, want %q", got, want)
	}
}

func TestFormatEventSummary_AllRejected(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: "pool-a",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryTaint, Reason: "taint X"},
			TotalNodes:   2,
		},
		{
			NodepoolName: "pool-b",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryAffinity, Reason: "affinity Y"},
			TotalNodes:   1,
		},
		{
			NodepoolName: "pool-c",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryAffinity, Reason: "affinity Z"},
			TotalNodes:   3,
		},
	}

	got := diagnosis.FormatEventSummary(diagnoses)
	want := "0/3 nodepools fit | rejected: 1 taint, 2 affinity"

	if got != want {
		t.Errorf("FormatEventSummary() = %q, want %q", got, want)
	}
}

func TestFormatEventSummary_Mixed(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{NodepoolName: "pool-a", Verdict: diagnosis.Accepted, FittingNodes: 2, TotalNodes: 2},
		{NodepoolName: "pool-b", Verdict: diagnosis.Accepted, FittingNodes: 1, TotalNodes: 1},
		{
			NodepoolName: "pool-c",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryTaint, Reason: "taint X"},
			TotalNodes:   3,
		},
		{
			NodepoolName: "pool-d",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryNodeSelector, Reason: "selector Y"},
			TotalNodes:   1,
		},
		{
			NodepoolName: "pool-e",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: "resources"},
			TotalNodes:   2,
		},
		{
			NodepoolName: "pool-f",
			Verdict:      diagnosis.NoStock,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: "inventory unhealthy"},
			TotalNodes:   0,
		},
		{
			NodepoolName: "pool-g",
			Verdict:      diagnosis.Candidate,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: "scale-up triggered"},
			TotalNodes:   1,
		},
	}

	got := diagnosis.FormatEventSummary(diagnoses)
	want := "2/7 nodepools fit | rejected: 1 taint, 1 selector, 1 resource | no-stock: 1 | candidate: 1"

	if got != want {
		t.Errorf("FormatEventSummary() = %q, want %q", got, want)
	}
}

func TestFormatEventSummary_Empty(t *testing.T) {
	got := diagnosis.FormatEventSummary(nil)

	if got != "" {
		t.Errorf("FormatEventSummary() = %q, want empty", got)
	}
}

func TestFormatEventSummary_SingleAccepted(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{NodepoolName: "pool-a", Verdict: diagnosis.Accepted, FittingNodes: 1, TotalNodes: 1},
	}

	got := diagnosis.FormatEventSummary(diagnoses)
	want := "1/1 nodepools fit"

	if got != want {
		t.Errorf("FormatEventSummary() = %q, want %q", got, want)
	}
}

func TestFormatEventSummary_SingleRejected(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: "pool-a",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: "cpu insufficient"},
			TotalNodes:   3,
		},
	}

	got := diagnosis.FormatEventSummary(diagnoses)
	want := "0/1 nodepools fit | rejected: 1 resource"

	if got != want {
		t.Errorf("FormatEventSummary() = %q, want %q", got, want)
	}
}

func TestFormatEventSummary_NoStockOnly(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: "pool-a",
			Verdict:      diagnosis.NoStock,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: "inventory unhealthy"},
			TotalNodes:   0,
		},
		{
			NodepoolName: "pool-b",
			Verdict:      diagnosis.NoStock,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: "inventory unhealthy"},
			TotalNodes:   0,
		},
	}

	got := diagnosis.FormatEventSummary(diagnoses)
	want := "0/2 nodepools fit | no-stock: 2"

	if got != want {
		t.Errorf("FormatEventSummary() = %q, want %q", got, want)
	}
}

func TestFormatEventSummary_CategoryOrdering(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: "pool-a",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: "cpu"},
			TotalNodes:   1,
		},
		{
			NodepoolName: "pool-b",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryTaint, Reason: "taint"},
			TotalNodes:   1,
		},
		{
			NodepoolName: "pool-c",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryAffinity, Reason: "affinity"},
			TotalNodes:   1,
		},
		{
			NodepoolName: "pool-d",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryNodeSelector, Reason: "selector"},
			TotalNodes:   1,
		},
	}

	got := diagnosis.FormatEventSummary(diagnoses)
	// Categories should be ordered by RejectionCategory value: taint, selector, affinity, resource.
	want := "0/4 nodepools fit | rejected: 1 taint, 1 selector, 1 affinity, 1 resource"

	if got != want {
		t.Errorf("FormatEventSummary() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatEventSummary -count=1`
Expected: compilation error, `FormatEventSummary` undefined

- [ ] **Step 3: Write minimal implementation (GREEN)**

Replace entire contents of `internal/diagnosis/summary.go`:

```go
package diagnosis

import (
	"fmt"
	"sort"
	"strings"
)

func FormatEventSummary(diagnoses []NodepoolDiagnosis) string {
	if len(diagnoses) == 0 {
		return ""
	}

	accepted := countAccepted(diagnoses)
	total := len(diagnoses)

	parts := []string{fmt.Sprintf("%d/%d nodepools fit", accepted, total)}

	if rejectedPart := formatRejectedBreakdown(diagnoses); rejectedPart != "" {
		parts = append(parts, rejectedPart)
	}

	if noStockCount := countByVerdict(diagnoses, NoStock); noStockCount > 0 {
		parts = append(parts, fmt.Sprintf("no-stock: %d", noStockCount))
	}

	if candidateCount := countByVerdict(diagnoses, Candidate); candidateCount > 0 {
		parts = append(parts, fmt.Sprintf("candidate: %d", candidateCount))
	}

	return strings.Join(parts, " | ")
}

func countAccepted(diagnoses []NodepoolDiagnosis) int {
	n := 0

	for i := range diagnoses {
		if diagnoses[i].Verdict == Accepted {
			n++
		}
	}

	return n
}

func countByVerdict(diagnoses []NodepoolDiagnosis, v Verdict) int {
	n := 0

	for i := range diagnoses {
		if diagnoses[i].Verdict == v {
			n++
		}
	}

	return n
}

func formatRejectedBreakdown(diagnoses []NodepoolDiagnosis) string {
	counts := make(map[RejectionCategory]int)

	for i := range diagnoses {
		if diagnoses[i].Verdict == Rejected && diagnoses[i].Rejection != nil {
			counts[diagnoses[i].Rejection.Category]++
		}
	}

	if len(counts) == 0 {
		return ""
	}

	categories := make([]RejectionCategory, 0, len(counts))
	for c := range counts {
		categories = append(categories, c)
	}

	sort.Slice(categories, func(i, j int) bool {
		return int(categories[i]) < int(categories[j])
	})

	parts := make([]string, 0, len(categories))
	for _, c := range categories {
		parts = append(parts, fmt.Sprintf("%d %s", counts[c], categoryToString(c)))
	}

	return "rejected: " + strings.Join(parts, ", ")
}
```

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatEventSummary -count=1`
Expected: ALL PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/diagnosis/...`

- [ ] **Step 6: Commit**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && git add internal/diagnosis/summary.go internal/diagnosis/summary_test.go && git commit -m "feat: rewrite summary to one-line format"`

---

### Task 4: writeAnnotation in reconciler

File: `internal/controller/reconciler.go`
Test: `internal/controller/reconciler_test.go`

- [ ] **Step 1: Write failing test (RED)**

Add to `internal/controller/reconciler_test.go`:

```go
import (
	"encoding/json"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)
```

```go
func TestReconcile_PendingPod_WritesAnnotation(t *testing.T) {
	scheme := testScheme()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "annotated-pod",
			Namespace:         defaultNamespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Minute)),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	prov := ack.New()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-ann-1",
			Labels: map[string]string{prov.NodepoolLabelKey(): "pool-ann", "name": "ann-pool"},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			},
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod, node).
		WithStatusSubresource(pod).
		Build()

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}

	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        prov,
		RecheckInterval: 30 * time.Second,
		InitialDelay:    10 * time.Second,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "annotated-pod", Namespace: defaultNamespace}}
	_, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Re-fetch the pod and check annotation.
	var updated corev1.Pod
	if err := cl.Get(context.Background(), req.NamespacedName, &updated); err != nil {
		t.Fatalf("getting updated pod: %v", err)
	}

	ann, ok := updated.Annotations[diagnosis.AnnotationKey]
	if !ok {
		t.Fatalf("expected annotation %q to be set", diagnosis.AnnotationKey)
	}

	var report diagnosis.DiagnosisReport
	if err := json.Unmarshal([]byte(ann), &report); err != nil {
		t.Fatalf("unmarshaling annotation: %v", err)
	}

	if report.Timestamp == "" {
		t.Error("report.Timestamp is empty")
	}

	if report.Summary == "" {
		t.Error("report.Summary is empty")
	}

	if len(report.Nodepools) == 0 {
		t.Error("report.Nodepools is empty")
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile_PendingPod_WritesAnnotation -count=1`
Expected: FAIL, annotation not set on pod

- [ ] **Step 3: Write minimal implementation (GREEN)**

Add to `internal/controller/reconciler.go`:

```go
func (r *PodReconciler) writeAnnotation(
	ctx context.Context,
	pod *corev1.Pod,
	diagnoses []diagnosis.NodepoolDiagnosis,
) {
	report := diagnosis.BuildReport(diagnoses)

	data, err := diagnosis.MarshalReport(report)
	if err != nil {
		slog.Warn("failed to marshal diagnosis report", "error", err, "pod", pod.Name)
		return
	}

	patch := client.MergeFrom(pod.DeepCopy())

	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	pod.Annotations[diagnosis.AnnotationKey] = data

	if err := r.Client.Patch(ctx, pod, patch); err != nil {
		slog.Warn("failed to write diagnosis annotation", "error", err, "pod", pod.Name)
	}
}
```

Update `Reconcile` to call `writeAnnotation` before `emitEvents`:

```go
// In Reconcile, after diagnose() returns:
r.writeAnnotation(ctx, &pod, diagnoses)
r.emitEvents(&pod, diagnoses)
```

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile_PendingPod_WritesAnnotation -count=1`
Expected: ALL PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/controller/...`

- [ ] **Step 6: Commit**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && git add internal/controller/reconciler.go internal/controller/reconciler_test.go && git commit -m "feat: write diagnosis annotation on pods"`

---

### Task 5: removeAnnotation cleanup

File: `internal/controller/reconciler.go`
Test: `internal/controller/reconciler_test.go`

- [ ] **Step 1: Write failing test (RED)**

Add to `internal/controller/reconciler_test.go`:

```go
func TestReconcile_RunningPod_RemovesAnnotation(t *testing.T) {
	scheme := testScheme()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "was-pending-pod",
			Namespace: defaultNamespace,
			Annotations: map[string]string{
				diagnosis.AnnotationKey: `{"timestamp":"2026-07-13T00:00:00Z","summary":"test","nodepools":[]}`,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod).
		Build()

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}

	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        ack.New(),
		RecheckInterval: 30 * time.Second,
		InitialDelay:    10 * time.Second,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "was-pending-pod", Namespace: defaultNamespace}}
	_, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var updated corev1.Pod
	if err := cl.Get(context.Background(), req.NamespacedName, &updated); err != nil {
		t.Fatalf("getting updated pod: %v", err)
	}

	if _, ok := updated.Annotations[diagnosis.AnnotationKey]; ok {
		t.Errorf("expected annotation %q to be removed", diagnosis.AnnotationKey)
	}
}

func TestReconcile_RunningPod_NoAnnotation_NoPatch(t *testing.T) {
	scheme := testScheme()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "clean-running-pod",
			Namespace: defaultNamespace,
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod).
		Build()

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}

	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        ack.New(),
		RecheckInterval: 30 * time.Second,
		InitialDelay:    10 * time.Second,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "clean-running-pod", Namespace: defaultNamespace}}
	result, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile_RunningPod -count=1`
Expected: FAIL, annotation still present

- [ ] **Step 3: Write minimal implementation (GREEN)**

Add to `internal/controller/reconciler.go`:

```go
func (r *PodReconciler) removeAnnotation(ctx context.Context, pod *corev1.Pod) {
	if _, ok := pod.Annotations[diagnosis.AnnotationKey]; !ok {
		return
	}

	patch := client.MergeFrom(pod.DeepCopy())
	delete(pod.Annotations, diagnosis.AnnotationKey)

	if err := r.Client.Patch(ctx, pod, patch); err != nil {
		slog.Warn("failed to remove diagnosis annotation", "error", err, "pod", pod.Name)
	}
}
```

Update `Reconcile` to call `removeAnnotation` when pod is not Pending:

```go
if pod.Status.Phase != corev1.PodPending {
	r.removeAnnotation(ctx, &pod)
	return ctrl.Result{}, nil
}
```

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile_RunningPod -count=1`
Expected: ALL PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/controller/...`

- [ ] **Step 6: Commit**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && git add internal/controller/reconciler.go internal/controller/reconciler_test.go && git commit -m "feat: remove annotation on non-pending pods"`

---

### Task 6: Update emitEvents to single summary event

File: `internal/controller/reconciler.go`
Test: `internal/controller/reconciler_test.go`

- [ ] **Step 1: Write failing test (RED)**

Update the existing `TestReconcile_PendingPod` test expectations in `internal/controller/reconciler_test.go`:

```go
func TestReconcile_PendingPod(t *testing.T) {
	scheme := testScheme()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              pendingPodName,
			Namespace:         defaultNamespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Minute)),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	prov := ack.New()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-1",
			Labels: map[string]string{prov.NodepoolLabelKey(): "pool-a", "name": "general"},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			},
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod, node).
		WithStatusSubresource(pod).
		Build()

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}

	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        prov,
		RecheckInterval: 30 * time.Second,
		InitialDelay:    10 * time.Second,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: pendingPodName, Namespace: defaultNamespace}}
	result, err := r.Reconcile(context.Background(), req)

	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter != 30*time.Second {
		t.Errorf("RequeueAfter = %v, want 30s", result.RequeueAfter)
	}

	// Expect exactly one event with the new summary format.
	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, "nodepools fit") {
			t.Errorf("expected event with 'nodepools fit', got %q", event)
		}
	default:
		t.Error("expected at least one event to be emitted")
	}

	// No second event.
	select {
	case event := <-recorder.Events:
		t.Errorf("expected exactly one event, got extra: %q", event)
	default:
		// expected
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile_PendingPod$ -count=1`
Expected: FAIL, event format mismatch (old format still emitted)

- [ ] **Step 3: Write minimal implementation (GREEN)**

Replace `emitEvents` in `internal/controller/reconciler.go`:

```go
func (r *PodReconciler) emitEvents(pod *corev1.Pod, diagnoses []diagnosis.NodepoolDiagnosis) {
	summary := diagnosis.FormatEventSummary(diagnoses)
	if summary == "" {
		return
	}

	eventType := corev1.EventTypeNormal

	for _, d := range diagnoses {
		if d.Verdict != diagnosis.Accepted {
			eventType = corev1.EventTypeWarning
			break
		}
	}

	r.Recorder.Eventf(pod, nil, eventType, "FitcheckDiagnosis", "diagnose", summary)
}
```

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile_PendingPod$ -count=1`
Expected: ALL PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/controller/...`

- [ ] **Step 6: Commit**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && git add internal/controller/reconciler.go internal/controller/reconciler_test.go && git commit -m "feat: emit single summary event"`

---

### Task 7: Wire writeAnnotation into Reconcile

File: `internal/controller/reconciler.go`
Test: `internal/controller/reconciler_test.go`

Note: If Tasks 4 and 5 already wired the calls into `Reconcile`, this task verifies the full flow end-to-end.

- [ ] **Step 1: Write failing test (RED)**

Add to `internal/controller/reconciler_test.go`:

```go
func TestReconcile_FullFlow_AnnotationAndEvent(t *testing.T) {
	scheme := testScheme()

	prov := ack.New()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "full-flow-pod",
			Namespace:         defaultNamespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Minute)),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	acceptedNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "accepted-node",
			Labels: map[string]string{prov.NodepoolLabelKey(): "pool-ok", "name": "ok-pool"},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			},
		},
	}

	rejectedNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "rejected-node",
			Labels: map[string]string{prov.NodepoolLabelKey(): "pool-bad", "name": "bad-pool"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("8"),
			},
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod, acceptedNode, rejectedNode).
		WithStatusSubresource(pod).
		Build()

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}

	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        prov,
		RecheckInterval: 30 * time.Second,
		InitialDelay:    0,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: "full-flow-pod", Namespace: defaultNamespace}}
	_, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	// Verify annotation exists with valid JSON.
	var updated corev1.Pod
	if err := cl.Get(context.Background(), req.NamespacedName, &updated); err != nil {
		t.Fatalf("getting updated pod: %v", err)
	}

	ann, ok := updated.Annotations[diagnosis.AnnotationKey]
	if !ok {
		t.Fatal("annotation not set")
	}

	var report diagnosis.DiagnosisReport
	if err := json.Unmarshal([]byte(ann), &report); err != nil {
		t.Fatalf("invalid annotation JSON: %v", err)
	}

	if len(report.Nodepools) != 2 {
		t.Errorf("nodepools count = %d, want 2", len(report.Nodepools))
	}

	if !strings.Contains(report.Summary, "nodepools fit") {
		t.Errorf("summary = %q, expected to contain 'nodepools fit'", report.Summary)
	}

	// Verify exactly one event.
	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, "nodepools fit") {
			t.Errorf("event = %q, expected 'nodepools fit'", event)
		}
	default:
		t.Error("expected one event")
	}

	select {
	case extra := <-recorder.Events:
		t.Errorf("expected exactly one event, got extra: %q", extra)
	default:
		// expected
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile_FullFlow -count=1`
Expected: FAIL if wiring is not yet complete; PASS if Tasks 4-6 already wired everything

- [ ] **Step 3: Write minimal implementation (GREEN)**

Ensure `Reconcile` method looks like this (final form):

```go
func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var pod corev1.Pod
	if err := r.Client.Get(ctx, req.NamespacedName, &pod); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("getting pod: %w", err)
	}

	if pod.Status.Phase != corev1.PodPending {
		r.removeAnnotation(ctx, &pod)
		return ctrl.Result{}, nil
	}

	if remaining := r.remainingDelay(&pod); remaining > 0 {
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	diagnoses, err := r.diagnose(ctx, &pod)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.writeAnnotation(ctx, &pod, diagnoses)
	r.emitEvents(&pod, diagnoses)

	return ctrl.Result{RequeueAfter: r.RecheckInterval}, nil
}
```

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile -count=1`
Expected: ALL PASS (all reconciler tests)

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/controller/...`

- [ ] **Step 6: Commit**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && git add internal/controller/reconciler.go internal/controller/reconciler_test.go && git commit -m "feat: wire annotation into reconcile loop"`

---

### Task 8: Update RBAC in Helm chart

File: `charts/fitcheck/templates/clusterrole.yaml`

- [ ] **Step 1: Write failing test (RED)**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && helm template fitcheck charts/fitcheck/ -n kube-system | grep -A2 'resources: \[pods\]'`
Expected: output shows `verbs: [get, list, watch]` (no `patch`)

- [ ] **Step 2: Update clusterrole.yaml**

Change the pods rule in `charts/fitcheck/templates/clusterrole.yaml`:

```yaml
  - apiGroups: [""]
    resources: [pods]
    verbs: [get, list, watch, patch]
```

- [ ] **Step 3: Verify**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && helm lint charts/fitcheck/ && helm template fitcheck charts/fitcheck/ -n kube-system | grep -A2 'resources: \[pods\]'`
Expected: lint passes, output shows `verbs: [get, list, watch, patch]`

- [ ] **Step 4: Commit**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && git add charts/fitcheck/templates/clusterrole.yaml && git commit -m "chore: add patch verb for pods"`

---

### Task 9: Update envtest integration test

File: `internal/controller/reconciler_envtest_test.go`

- [ ] **Step 1: Write failing test (RED)**

Replace `verifyEvents` and update test to also verify annotation. Replace the relevant parts of `internal/controller/reconciler_envtest_test.go`:

```go
import (
	"encoding/json"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)
```

Update `TestReconciler_Envtest` to verify annotation after reconcile:

```go
func TestReconciler_Envtest(t *testing.T) {
	testEnv := &envtest.Environment{}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("starting envtest: %v", err)
	}

	t.Cleanup(func() {
		if stopErr := testEnv.Stop(); stopErr != nil {
			t.Errorf("stopping envtest: %v", stopErr)
		}
	})

	cl, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	ctx := context.Background()

	createNamespace(t, cl, ctx)
	createNodes(t, cl, ctx)
	createPendingPod(t, cl, ctx)

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}

	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        ack.New(),
		RecheckInterval: 30 * time.Second,
		InitialDelay:    0,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: envtestPodName, Namespace: envtestNamespace}}
	result, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter != 30*time.Second {
		t.Errorf("RequeueAfter = %v, want 30s", result.RequeueAfter)
	}

	verifyAnnotation(t, cl, ctx, req.NamespacedName)
	verifyEvents(t, recorder)
}
```

Replace `verifyEvents`:

```go
func verifyEvents(t *testing.T, recorder *events.FakeRecorder) {
	t.Helper()

	collected := drainEvents(recorder)

	if len(collected) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(collected), collected)
	}

	event := collected[0]

	if !strings.Contains(event, "nodepools fit") {
		t.Errorf("expected event with 'nodepools fit', got: %q", event)
	}
}
```

Add `verifyAnnotation`:

```go
func verifyAnnotation(t *testing.T, cl client.Client, ctx context.Context, key types.NamespacedName) {
	t.Helper()

	var pod corev1.Pod
	if err := cl.Get(ctx, key, &pod); err != nil {
		t.Fatalf("getting pod for annotation check: %v", err)
	}

	ann, ok := pod.Annotations[diagnosis.AnnotationKey]
	if !ok {
		t.Fatalf("expected annotation %q to be set", diagnosis.AnnotationKey)
	}

	var report diagnosis.DiagnosisReport
	if err := json.Unmarshal([]byte(ann), &report); err != nil {
		t.Fatalf("invalid annotation JSON: %v", err)
	}

	if report.Timestamp == "" {
		t.Error("report.Timestamp is empty")
	}

	if !strings.Contains(report.Summary, "nodepools fit") {
		t.Errorf("summary = %q, expected 'nodepools fit'", report.Summary)
	}

	if len(report.Nodepools) != 2 {
		t.Errorf("nodepools count = %d, want 2", len(report.Nodepools))
	}

	hasAccepted := false
	hasRejected := false

	for _, np := range report.Nodepools {
		if np.Verdict == "accepted" && np.Name == "cpu-pool-name" {
			hasAccepted = true
		}

		if np.Verdict == "rejected" && np.Name == "gpu-pool-name" {
			hasRejected = true
		}
	}

	if !hasAccepted {
		t.Errorf("expected accepted nodepool cpu-pool-name in annotation, got: %s", ann)
	}

	if !hasRejected {
		t.Errorf("expected rejected nodepool gpu-pool-name in annotation, got: %s", ann)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test -tags envtest ./internal/controller/ -run TestReconciler_Envtest -count=1 -timeout 60s`
Expected: FAIL (annotation check fails because envtest runs against real API server but annotation write is not yet wired, or PASS if Tasks 4-7 already completed)

- [ ] **Step 3: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test -tags envtest ./internal/controller/ -run TestReconciler_Envtest -count=1 -timeout 60s`
Expected: ALL PASS

- [ ] **Step 4: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/controller/...`

- [ ] **Step 5: Commit**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && git add internal/controller/reconciler_envtest_test.go && git commit -m "feat: verify annotation in envtest"`

---

## Code conventions

- Import order: stdlib, blank line, external, blank line, internal
- Declaration order: type, const, var, func
- funlen max 80, cyclop 15, gocognit 20
- Zero `//nolint`
- Commit: `prefix: max 5 words` with `-m` only, no trailers
- Error handling: PATCH failures are logged and skipped, never returned as reconcile errors

## Execution order

Tasks 3 and 1 share a dependency (`BuildReport` calls `FormatEventSummary`). Execute in this order:

1. Task 3 (FormatEventSummary)
2. Task 1 (BuildReport + types)
3. Task 2 (MarshalReport)
4. Task 4 (writeAnnotation)
5. Task 5 (removeAnnotation)
6. Task 6 (emitEvents rewrite)
7. Task 7 (full wiring verification)
8. Task 8 (RBAC)
9. Task 9 (envtest)
