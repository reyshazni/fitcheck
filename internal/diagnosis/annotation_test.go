package diagnosis_test

import (
	"testing"
	"time"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

const (
	gpuPoolA          = "gpu-pool-a"
	gpuPoolB          = "gpu-pool-b"
	armPool           = "arm-pool"
	spotPool          = "spot-pool"
	scalingPool       = "scaling-pool"
	generalPool       = "general-pool"
	highmemPool       = "highmem-pool"
	reasonNvidiaTaint = "taint nvidia.com/gpu=present:NoSchedule not tolerated"
	reasonScaleUp     = "scale-up triggered"
	verdictTaint      = "taint"
	mixedSummary      = "2/7 nodepools fit | rejected: 2 taint, 1 affinity | no-stock: 1 | candidate: 1"
)

func mixedDiagnoses() []diagnosis.NodepoolDiagnosis {
	return []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: generalPool,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 3,
			TotalNodes:   5,
		},
		{
			NodepoolName: highmemPool,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 2,
			TotalNodes:   2,
		},
		{
			NodepoolName: gpuPoolA,
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryTaint,
				Reason:   reasonNvidiaTaint,
			},
			TotalNodes: 4,
		},
		{
			NodepoolName: gpuPoolB,
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryTaint,
				Reason:   "taint dedicated=gpu:NoSchedule not tolerated",
			},
			TotalNodes: 2,
		},
		{
			NodepoolName: armPool,
			Verdict:      diagnosis.Rejected,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryAffinity,
				Reason:   "required node affinity not matched",
			},
			TotalNodes: 3,
		},
		{
			NodepoolName: spotPool,
			Verdict:      diagnosis.NoStock,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryResources,
				Reason:   reasonInvUnhealthy,
			},
			TotalNodes: 0,
		},
		{
			NodepoolName: scalingPool,
			Verdict:      diagnosis.Candidate,
			Rejection: &diagnosis.Rejection{
				Category: diagnosis.CategoryResources,
				Reason:   reasonScaleUp,
			},
			TotalNodes: 1,
		},
	}
}

func TestBuildReport_MixedTimestamp(t *testing.T) {
	report := diagnosis.BuildReport(mixedDiagnoses())

	ts, err := time.Parse(time.RFC3339, report.Timestamp)
	if err != nil {
		t.Fatalf("timestamp parse error: %v", err)
	}

	if time.Since(ts) > 5*time.Second {
		t.Errorf("timestamp too old: %v", report.Timestamp)
	}
}

func TestBuildReport_MixedSummary(t *testing.T) {
	report := diagnosis.BuildReport(mixedDiagnoses())

	wantSummary := mixedSummary
	if report.Summary != wantSummary {
		t.Errorf("summary = %q, want %q", report.Summary, wantSummary)
	}

	if len(report.Nodepools) != 7 {
		t.Fatalf("nodepools count = %d, want 7", len(report.Nodepools))
	}
}

func TestBuildReport_MixedAcceptedFields(t *testing.T) {
	report := diagnosis.BuildReport(mixedDiagnoses())

	np0 := report.Nodepools[0]
	if np0.Name != generalPool {
		t.Errorf("nodepools[0].Name = %q, want %q", np0.Name, generalPool)
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
}

func TestBuildReport_MixedRejectedFields(t *testing.T) {
	report := diagnosis.BuildReport(mixedDiagnoses())

	np2 := report.Nodepools[2]
	if np2.Name != gpuPoolA {
		t.Errorf("nodepools[2].Name = %q, want %q", np2.Name, gpuPoolA)
	}

	if np2.Verdict != "rejected" {
		t.Errorf("nodepools[2].Verdict = %q, want %q", np2.Verdict, "rejected")
	}

	if np2.Reason != reasonNvidiaTaint {
		t.Errorf("nodepools[2].Reason = %q", np2.Reason)
	}

	if np2.Category != verdictTaint {
		t.Errorf("nodepools[2].Category = %q, want %q", np2.Category, verdictTaint)
	}
}

func TestBuildReport_MixedNoStockAndCandidate(t *testing.T) {
	report := diagnosis.BuildReport(mixedDiagnoses())

	np5 := report.Nodepools[5]
	if np5.Verdict != "no-stock" {
		t.Errorf("nodepools[5].Verdict = %q, want %q", np5.Verdict, "no-stock")
	}

	np6 := report.Nodepools[6]
	if np6.Verdict != "candidate" {
		t.Errorf("nodepools[6].Verdict = %q, want %q", np6.Verdict, "candidate")
	}
}

func TestBuildReport_AllAccepted(t *testing.T) {
	diagnoses := []diagnosis.NodepoolDiagnosis{
		{
			NodepoolName: poolA,
			Verdict:      diagnosis.Accepted,
			FittingNodes: 2,
			TotalNodes:   3,
		},
		{
			NodepoolName: poolB,
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
