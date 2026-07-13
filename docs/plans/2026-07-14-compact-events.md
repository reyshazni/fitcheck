# Compact Event Summary Implementation Plan

> **For agentic workers:** REQUIRED: Follow the `test-driven-development` skill for every task. Use `coding:execute` to implement task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor event emission to produce 2 compact summary events per pod (one Normal, one Warning) instead of N separate events that get aggregated into one by events.k8s.io.

**Architecture:** Move formatting logic to `internal/diagnosis/summary.go`. Controller calls `FormatSummary()` and emits at most 2 events. Messages truncated to fit 1kB note limit.

**Tech Stack:** Go 1.26, controller-runtime v0.24.1

**Execution Status:** COMPLETE
**Started:** 2026-07-14 00:14 WIB
**Completed:** 2026-07-14 00:25 WIB
**Progress:** 7/7 tasks complete
**Reviews:** 0 completed

---

## Context

### Current behavior

`emitEvents` in `internal/controller/reconciler.go` calls `buildSummary` which filters diagnoses by verdict and joins `Message()` strings with newlines. `Message()` returns `nodepool/<name>: fits X/Y nodes` for Accepted or `nodepool/<name>: <reason>` for others.

### Target behavior

Two compact events per pod:

Normal event note:
```
[accepted] kf-control-plane(5/5), common-8cpu(3/3)
```

Warning event note (combines all non-accepted verdicts):
```
[rejected] nfs: taint workload_type=nfs, risk-score: taint project=gopay-score
[no-stock] gpu-pool: inventory unhealthy
[candidate] spot-pool: scale-up triggered
```

### Rules

- Each verdict group on its own line, prefixed with `[verdict]`
- Nodepools within a group comma-separated
- Accepted format: `name(fitting/total)`
- Rejected/candidate/nostock format: `name: reason`
- If total message > 1000 bytes, truncate the longest group and append `... +N more`
- Empty groups are omitted (no empty lines)

### Verdict label mapping

| Verdict     | Label in output |
| :---------- | :-------------- |
| `Accepted`  | `accepted`      |
| `Rejected`  | `rejected`      |
| `Candidate` | `candidate`     |
| `NoStock`   | `no-stock`      |

---

## New function signature

File: `internal/diagnosis/summary.go`

```go
const maxNoteBytes = 1000

func FormatSummary(diagnoses []NodepoolDiagnosis) (normal, warning string)
```

Returns two strings: `normal` for accepted nodepools (may be empty), `warning` for rejected/candidate/nostock (may be empty).

---

## Changes to reconciler

File: `internal/controller/reconciler.go`

Replace `emitEvents` + `buildSummary` with:

```go
func (r *PodReconciler) emitEvents(pod *corev1.Pod, diagnoses []diagnosis.NodepoolDiagnosis) {
	normal, warning := diagnosis.FormatSummary(diagnoses)

	if normal != "" {
		r.Recorder.Eventf(pod, nil, corev1.EventTypeNormal, "FitcheckDiagnosis", "diagnose", normal)
	}

	if warning != "" {
		r.Recorder.Eventf(pod, nil, corev1.EventTypeWarning, "FitcheckDiagnosis", "diagnose", warning)
	}
}
```

Delete `buildSummary` from reconciler.go. Remove the `"strings"` import (no longer needed in reconciler.go).

---

## Tasks

### Task 1: FormatSummary with accepted nodepools only

File: `internal/diagnosis/summary.go`
Test: `internal/diagnosis/summary_test.go`

- [ ] **Step 1: Write failing test (RED)**

```go
package diagnosis_test

import (
	"testing"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

func TestFormatSummary_AcceptedOnly(t *testing.T) {
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
		{
			NodepoolName: "pool-c",
			Verdict:      diagnosis.Accepted,
			FittingNodes: 5,
			TotalNodes:   5,
		},
	}

	normal, warning := diagnosis.FormatSummary(diagnoses)

	wantNormal := "[accepted] pool-a(2/3), pool-b(1/1), pool-c(5/5)"
	if normal != wantNormal {
		t.Errorf("normal = %q, want %q", normal, wantNormal)
	}

	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_AcceptedOnly -count=1`
Expected: compilation failure, `FormatSummary` undefined

- [ ] **Step 3: Write minimal implementation (GREEN)**

Create `internal/diagnosis/summary.go`:

```go
package diagnosis

import (
	"fmt"
	"strings"
)

const maxNoteBytes = 1000

type verdictGroup struct {
	label   string
	entries []string
}

func FormatSummary(diagnoses []NodepoolDiagnosis) (normal, warning string) {
	accepted := collectAccepted(diagnoses)
	rejected := collectByVerdict(diagnoses, Rejected, "rejected")
	noStock := collectByVerdict(diagnoses, NoStock, "no-stock")
	candidate := collectByVerdict(diagnoses, Candidate, "candidate")

	normal = formatGroup(accepted)
	warning = joinWarningGroups(rejected, noStock, candidate)

	return normal, warning
}

func collectAccepted(diagnoses []NodepoolDiagnosis) verdictGroup {
	g := verdictGroup{label: "accepted"}

	for i := range diagnoses {
		if diagnoses[i].Verdict == Accepted {
			entry := fmt.Sprintf("%s(%d/%d)",
				diagnoses[i].NodepoolName,
				diagnoses[i].FittingNodes,
				diagnoses[i].TotalNodes,
			)
			g.entries = append(g.entries, entry)
		}
	}

	return g
}

func collectByVerdict(diagnoses []NodepoolDiagnosis, v Verdict, label string) verdictGroup {
	g := verdictGroup{label: label}

	for i := range diagnoses {
		if diagnoses[i].Verdict == v {
			entry := fmt.Sprintf("%s: %s",
				diagnoses[i].NodepoolName,
				diagnoses[i].Rejection.Reason,
			)
			g.entries = append(g.entries, entry)
		}
	}

	return g
}

func formatGroup(g verdictGroup) string {
	if len(g.entries) == 0 {
		return ""
	}

	return fmt.Sprintf("[%s] %s", g.label, strings.Join(g.entries, ", "))
}

func joinWarningGroups(groups ...verdictGroup) string {
	var lines []string

	for _, g := range groups {
		if line := formatGroup(g); line != "" {
			lines = append(lines, line)
		}
	}

	result := strings.Join(lines, "\n")

	if len(result) <= maxNoteBytes {
		return result
	}

	return truncateWarning(lines)
}

func truncateWarning(lines []string) string {
	for {
		longest, longestLen := 0, 0

		for i, line := range lines {
			if len(line) > longestLen {
				longest = i
				longestLen = len(line)
			}
		}

		total := totalLen(lines)
		if total <= maxNoteBytes {
			return strings.Join(lines, "\n")
		}

		lines[longest] = truncateLine(lines[longest])
	}
}

func truncateLine(line string) string {
	// Find the bracket-prefix end: "[verdict] "
	prefixEnd := strings.Index(line, "] ")
	if prefixEnd < 0 {
		return line
	}

	prefix := line[:prefixEnd+2]
	body := line[prefixEnd+2:]

	parts := strings.Split(body, ", ")
	if len(parts) <= 1 {
		return line
	}

	kept := parts[:len(parts)-1]
	dropped := 1

	// Check if last kept entry already has "+ N more" suffix
	last := kept[len(kept)-1]
	if strings.HasSuffix(last, " more") {
		// Remove the old "... +N more" and count its dropped
		kept = kept[:len(kept)-1]
		dropped++
	}

	return fmt.Sprintf("%s%s, ... +%d more", prefix, strings.Join(kept, ", "), dropped)
}

func totalLen(lines []string) int {
	n := 0

	for i, line := range lines {
		n += len(line)
		if i > 0 {
			n++ // newline separator
		}
	}

	return n
}
```

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_AcceptedOnly -count=1`
Expected: PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/diagnosis/...`

- [ ] **Step 6: Commit**

Run: `git commit -m "feat: add FormatSummary accepted path"`

---

### Task 2: FormatSummary with rejected nodepools only

File: `internal/diagnosis/summary_test.go` (append)

- [ ] **Step 1: Write failing test (RED)**

```go
func TestFormatSummary_RejectedOnly(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: "nfs",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryTaint, Reason: "taint workload_type=nfs"},
			TotalNodes:   2,
		},
		{
			NodepoolName: "gpu",
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryTaint, Reason: "taint nvidia:NoSchedule"},
			TotalNodes:   1,
		},
	}

	normal, warning := diagnosis.FormatSummary(diagnoses)

	if normal != "" {
		t.Errorf("normal = %q, want empty", normal)
	}

	wantWarning := "[rejected] nfs: taint workload_type=nfs, gpu: taint nvidia:NoSchedule"
	if warning != wantWarning {
		t.Errorf("warning = %q, want %q", warning, wantWarning)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_RejectedOnly -count=1`
Expected: PASS (implementation already handles this from Task 1)

Note: If this passes immediately, no GREEN step needed. Proceed to lint.

- [ ] **Step 3: Write minimal implementation (GREEN)**

No changes expected. The `collectByVerdict` function from Task 1 already handles rejected.

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_RejectedOnly -count=1`
Expected: PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/diagnosis/...`

- [ ] **Step 6: Commit**

Run: `git commit -m "feat: add FormatSummary rejected test"`

---

### Task 3: FormatSummary with mixed verdicts

File: `internal/diagnosis/summary_test.go` (append)

- [ ] **Step 1: Write failing test (RED)**

```go
func TestFormatSummary_MixedVerdicts(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: "pool-a",
			Verdict:      diagnosis.Accepted,
			FittingNodes: 2,
			TotalNodes:   2,
		},
		{
			NodepoolName: "pool-b",
			Verdict:      diagnosis.Accepted,
			FittingNodes: 1,
			TotalNodes:   1,
		},
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

	normal, warning := diagnosis.FormatSummary(diagnoses)

	wantNormal := "[accepted] pool-a(2/2), pool-b(1/1)"
	if normal != wantNormal {
		t.Errorf("normal = %q, want %q", normal, wantNormal)
	}

	wantWarning := "[rejected] pool-c: taint X, pool-d: selector Y, pool-e: resources\n" +
		"[no-stock] pool-f: inventory unhealthy\n" +
		"[candidate] pool-g: scale-up triggered"
	if warning != wantWarning {
		t.Errorf("warning = %q, want %q", warning, wantWarning)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_MixedVerdicts -count=1`
Expected: PASS (implementation already handles this from Task 1)

- [ ] **Step 3: Write minimal implementation (GREEN)**

No changes expected. Task 1 implementation covers this.

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_MixedVerdicts -count=1`
Expected: PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/diagnosis/...`

- [ ] **Step 6: Commit**

Run: `git commit -m "feat: add FormatSummary mixed test"`

---

### Task 4: FormatSummary truncation at 1kB

File: `internal/diagnosis/summary_test.go` (append)

- [ ] **Step 1: Write failing test (RED)**

```go
import "fmt"

func TestFormatSummary_TruncationAt1kB(t *testing.T) {
	var diagnoses []diagnosis.NodepoolDiagnosis

	for i := range 50 {
		diagnoses = append(diagnoses, diagnosis.NodepoolDiagnosis{
			NodepoolName: fmt.Sprintf("long-nodepool-name-%03d", i),
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryTaint,
				Reason:   fmt.Sprintf("taint workload_type=special-value-%03d:NoSchedule not tolerated", i),
			},
			TotalNodes: 3,
		})
	}

	_, warning := diagnosis.FormatSummary(diagnoses)

	if len(warning) > 1000 {
		t.Errorf("warning length = %d, want <= 1000", len(warning))
	}

	if warning == "" {
		t.Error("warning should not be empty with 50 rejected pools")
	}

	wantSuffix := "more"
	if !strings.HasSuffix(warning, wantSuffix) {
		t.Errorf("warning should end with %q, got tail: %q", wantSuffix, warning[len(warning)-30:])
	}
}
```

Note: Add `"strings"` and `"fmt"` to the import block.

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_TruncationAt1kB -count=1`
Expected: PASS or FAIL depending on truncation logic correctness from Task 1. If FAIL, proceed to fix.

- [ ] **Step 3: Write minimal implementation (GREEN)**

If the truncation loop in `truncateWarning` has bugs (e.g. infinite loop when a single entry exceeds the limit), fix them. The `truncateLine` function should handle the edge case where removing entries still leaves > 1000 bytes by continuing to remove from the longest line.

Potential fix if needed: add a guard in `truncateWarning` to break if no progress is made (all lines are already truncated to minimum).

```go
func truncateWarning(lines []string) string {
	for {
		total := totalLen(lines)
		if total <= maxNoteBytes {
			return strings.Join(lines, "\n")
		}

		longest, longestLen := 0, 0
		for i, line := range lines {
			if len(line) > longestLen {
				longest = i
				longestLen = len(line)
			}
		}

		shortened := truncateLine(lines[longest])
		if shortened == lines[longest] {
			// Cannot shorten further; hard-truncate
			result := strings.Join(lines, "\n")
			return result[:maxNoteBytes]
		}

		lines[longest] = shortened
	}
}
```

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_TruncationAt1kB -count=1`
Expected: PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/diagnosis/...`

- [ ] **Step 6: Commit**

Run: `git commit -m "feat: add FormatSummary truncation logic"`

---

### Task 5: FormatSummary empty diagnoses

File: `internal/diagnosis/summary_test.go` (append)

- [ ] **Step 1: Write failing test (RED)**

```go
func TestFormatSummary_EmptyDiagnoses(t *testing.T) {
	normal, warning := diagnosis.FormatSummary(nil)

	if normal != "" {
		t.Errorf("normal = %q, want empty", normal)
	}

	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_EmptyDiagnoses -count=1`
Expected: PASS (nil slice produces no entries, empty groups produce empty strings)

- [ ] **Step 3: Write minimal implementation (GREEN)**

No changes expected.

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/diagnosis/ -run TestFormatSummary_EmptyDiagnoses -count=1`
Expected: PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/diagnosis/...`

- [ ] **Step 6: Commit**

Run: `git commit -m "feat: add FormatSummary empty test"`

---

### Task 6: Update reconciler emitEvents

Files:
- `internal/controller/reconciler.go`
- `internal/controller/reconciler_test.go`

- [ ] **Step 1: Write failing test (RED)**

Update `TestReconcile_PendingPod` in `internal/controller/reconciler_test.go` to verify the new compact event format:

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

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, "[accepted]") {
			t.Errorf("expected compact event with [accepted], got %q", event)
		}
		if !strings.Contains(event, "general(1/1)") {
			t.Errorf("expected general(1/1) in event, got %q", event)
		}
	default:
		t.Error("expected at least one event to be emitted")
	}
}
```

Note: Add `"strings"` to the import block.

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile_PendingPod -count=1`
Expected: FAIL because current events use old format (`nodepool/general: fits 1/1 nodes`)

- [ ] **Step 3: Write minimal implementation (GREEN)**

Edit `internal/controller/reconciler.go`:

1. Remove the `"strings"` import
2. Replace `emitEvents` with:

```go
func (r *PodReconciler) emitEvents(pod *corev1.Pod, diagnoses []diagnosis.NodepoolDiagnosis) {
	normal, warning := diagnosis.FormatSummary(diagnoses)

	if normal != "" {
		r.Recorder.Eventf(pod, nil, corev1.EventTypeNormal, "FitcheckDiagnosis", "diagnose", normal)
	}

	if warning != "" {
		r.Recorder.Eventf(pod, nil, corev1.EventTypeWarning, "FitcheckDiagnosis", "diagnose", warning)
	}
}
```

3. Delete `buildSummary` function entirely (lines 143-158)

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -run TestReconcile -count=1`
Expected: ALL PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/controller/...`

- [ ] **Step 6: Commit**

Run: `git commit -m "feat: use FormatSummary in reconciler"`

---

### Task 7: Update envtest integration test

File: `internal/controller/reconciler_envtest_test.go`

- [ ] **Step 1: Write failing test (RED)**

Replace `verifyEvents` function:

```go
func verifyEvents(t *testing.T, recorder *record.FakeRecorder) {
	t.Helper()

	events := drainEvents(recorder)

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(events), events)
	}

	hasAccepted := false
	hasRejected := false

	for _, e := range events {
		if strings.Contains(e, "[accepted]") && strings.Contains(e, "cpu-pool-name(1/1)") {
			hasAccepted = true
		}

		if strings.Contains(e, "[rejected]") && strings.Contains(e, "gpu-pool-name") {
			hasRejected = true
		}
	}

	if !hasAccepted {
		t.Errorf("expected [accepted] event with cpu-pool-name(1/1), got: %v", events)
	}

	if !hasRejected {
		t.Errorf("expected [rejected] event with gpu-pool-name, got: %v", events)
	}
}
```

- [ ] **Step 2: Verify RED**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -tags=envtest -run TestReconciler_Envtest -count=1`
Expected: PASS (the new format should already be emitted from Task 6)

Note: This requires envtest binaries. If not available locally, verify by reading the test and confirming logical consistency with the new event format.

- [ ] **Step 3: Write minimal implementation (GREEN)**

No production code changes. Test-only update.

- [ ] **Step 4: Verify GREEN**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go test ./internal/controller/ -tags=envtest -run TestReconciler_Envtest -count=1`
Expected: PASS

- [ ] **Step 5: Lint**

Run: `cd /Users/muhammad.helmi/Desktop/Non-work/repo/fitcheck && go tool golangci-lint run ./internal/controller/...`

- [ ] **Step 6: Commit**

Run: `git commit -m "chore: update envtest for compact events"`

---

## Code conventions

- Import order: stdlib, blank line, external, blank line, internal
- Declaration order: type, const, var, func
- funlen max 80, cyclop 15, gocognit 20
- Zero `//nolint`
- Commit: `prefix: max 5 words` with `-m` only, no trailers
- Test package: `diagnosis_test` (external test package)
- Test constants: reuse `testPoolID1` from `taint_test.go` where applicable
