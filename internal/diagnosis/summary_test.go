package diagnosis_test

import (
	"testing"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

const (
	poolA              = "pool-a"
	poolB              = "pool-b"
	poolC              = "pool-c"
	poolD              = "pool-d"
	poolE              = "pool-e"
	poolF              = "pool-f"
	poolG              = "pool-g"
	reasonTaintX       = "taint X"
	reasonInvUnhealthy = "inventory unhealthy"
)

func TestFormatEventSummary_AllAccepted(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{NodepoolName: poolA, Verdict: diagnosis.Accepted, FittingNodes: 2, TotalNodes: 3},
		{NodepoolName: poolB, Verdict: diagnosis.Accepted, FittingNodes: 1, TotalNodes: 1},
		{NodepoolName: poolC, Verdict: diagnosis.Accepted, FittingNodes: 5, TotalNodes: 5},
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
			NodepoolName: poolA,
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryTaint, Reason: reasonTaintX},
			TotalNodes:   2,
		},
		{
			NodepoolName: poolB,
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryAffinity, Reason: "affinity Y"},
			TotalNodes:   1,
		},
		{
			NodepoolName: poolC,
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
		{NodepoolName: poolA, Verdict: diagnosis.Accepted, FittingNodes: 2, TotalNodes: 2},
		{NodepoolName: poolB, Verdict: diagnosis.Accepted, FittingNodes: 1, TotalNodes: 1},
		{
			NodepoolName: poolC,
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryTaint, Reason: reasonTaintX},
			TotalNodes:   3,
		},
		{
			NodepoolName: poolD,
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryNodeSelector, Reason: "selector Y"},
			TotalNodes:   1,
		},
		{
			NodepoolName: poolE,
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: "resources"},
			TotalNodes:   2,
		},
		{
			NodepoolName: poolF,
			Verdict:      diagnosis.NoStock,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: reasonInvUnhealthy},
			TotalNodes:   0,
		},
		{
			NodepoolName: poolG,
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
		{NodepoolName: poolA, Verdict: diagnosis.Accepted, FittingNodes: 1, TotalNodes: 1},
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
			NodepoolName: poolA,
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
			NodepoolName: poolA,
			Verdict:      diagnosis.NoStock,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: reasonInvUnhealthy},
			TotalNodes:   0,
		},
		{
			NodepoolName: poolB,
			Verdict:      diagnosis.NoStock,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: reasonInvUnhealthy},
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
			NodepoolName: poolA,
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryResources, Reason: "cpu"},
			TotalNodes:   1,
		},
		{
			NodepoolName: poolB,
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryTaint, Reason: "taint"},
			TotalNodes:   1,
		},
		{
			NodepoolName: poolC,
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryAffinity, Reason: "affinity"},
			TotalNodes:   1,
		},
		{
			NodepoolName: poolD,
			Verdict:      diagnosis.Rejected,
			Rejection:    &diagnosis.Rejection{Category: diagnosis.CategoryNodeSelector, Reason: "selector"},
			TotalNodes:   1,
		},
	}

	got := diagnosis.FormatEventSummary(diagnoses)
	want := "0/4 nodepools fit | rejected: 1 taint, 1 selector, 1 affinity, 1 resource"

	if got != want {
		t.Errorf("FormatEventSummary() = %q, want %q", got, want)
	}
}
