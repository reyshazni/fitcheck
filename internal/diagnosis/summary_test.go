package diagnosis_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

const (
	summaryPoolAlpha   = "alpha"
	summaryPoolBravo   = "bravo"
	summaryPoolCharlie = "charlie"
	summaryPoolDelta   = "delta"
	summaryPoolEcho    = "echo"
	summaryPoolFoxtrot = "foxtrot"
	summaryPoolGolf    = "golf"
)

func TestFormatSummary_AcceptedOnly(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: summaryPoolAlpha,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 2,
			TotalNodes:   3,
		},
		{
			NodepoolName: summaryPoolBravo,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 1,
			TotalNodes:   1,
		},
		{
			NodepoolName: summaryPoolCharlie,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 5,
			TotalNodes:   5,
		},
	}

	normal, warning := diagnosis.FormatSummary(diagnoses)

	wantNormal := "[accepted] alpha(2/3), bravo(1/1), charlie(5/5)"
	if normal != wantNormal {
		t.Errorf("normal = %q, want %q", normal, wantNormal)
	}

	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
}

func TestFormatSummary_RejectedOnly(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: summaryPoolAlpha,
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryTaint,
				Reason:   "taint workload_type=nfs",
			},
			TotalNodes: 2,
		},
		{
			NodepoolName: summaryPoolBravo,
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryTaint,
				Reason:   "taint nvidia:NoSchedule",
			},
			TotalNodes: 1,
		},
	}

	normal, warning := diagnosis.FormatSummary(diagnoses)

	if normal != "" {
		t.Errorf("normal = %q, want empty", normal)
	}

	wantWarning := "[rejected] alpha: taint workload_type=nfs, bravo: taint nvidia:NoSchedule"
	if warning != wantWarning {
		t.Errorf("warning = %q, want %q", warning, wantWarning)
	}
}

func TestFormatSummary_MixedVerdicts(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: summaryPoolAlpha,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 2,
			TotalNodes:   2,
		},
		{
			NodepoolName: summaryPoolBravo,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 1,
			TotalNodes:   1,
		},
		{
			NodepoolName: summaryPoolCharlie,
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryTaint,
				Reason:   "taint X",
			},
			TotalNodes: 3,
		},
		{
			NodepoolName: summaryPoolDelta,
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryNodeSelector,
				Reason:   "selector Y",
			},
			TotalNodes: 1,
		},
		{
			NodepoolName: summaryPoolEcho,
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryResources,
				Reason:   "resources",
			},
			TotalNodes: 2,
		},
		{
			NodepoolName: summaryPoolFoxtrot,
			Verdict:      diagnosis.NoStock,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryResources,
				Reason:   "inventory unhealthy",
			},
			TotalNodes: 0,
		},
		{
			NodepoolName: summaryPoolGolf,
			Verdict:      diagnosis.Candidate,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryResources,
				Reason:   "scale-up triggered",
			},
			TotalNodes: 1,
		},
	}

	normal, warning := diagnosis.FormatSummary(diagnoses)

	wantNormal := "[accepted] alpha(2/2), bravo(1/1)"
	if normal != wantNormal {
		t.Errorf("normal = %q, want %q", normal, wantNormal)
	}

	wantWarning := "[rejected] charlie: taint X, delta: selector Y, echo: resources\n" +
		"[no-stock] foxtrot: inventory unhealthy\n" +
		"[candidate] golf: scale-up triggered"
	if warning != wantWarning {
		t.Errorf("warning = %q, want %q", warning, wantWarning)
	}
}

func TestFormatSummary_TruncationAt1kB(t *testing.T) {
	diagnoses := make([]diagnosis.NodepoolDiagnosis, 0, 50)

	for i := range 50 {
		diagnoses = append(diagnoses, diagnosis.NodepoolDiagnosis{
			NodepoolName: fmt.Sprintf("long-nodepool-name-%03d", i),
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryTaint,
				Reason: fmt.Sprintf(
					"taint workload_type=special-value-%03d:NoSchedule not tolerated", i,
				),
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
		tail := warning[len(warning)-30:]
		t.Errorf("warning should end with %q, got tail: %q",
			wantSuffix, tail)
	}
}

func TestFormatSummary_EmptyDiagnoses(t *testing.T) {
	normal, warning := diagnosis.FormatSummary(nil)

	if normal != "" {
		t.Errorf("normal = %q, want empty", normal)
	}

	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
}
