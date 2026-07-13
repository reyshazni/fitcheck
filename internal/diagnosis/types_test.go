package diagnosis_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

func TestNodepoolDiagnosis_Accepted(t *testing.T) {
	d := diagnosis.NodepoolDiagnosis{
		NodepoolID:   testPoolID1,
		NodepoolName: "gpu-pool",
		Verdict:      diagnosis.Accepted,
		FittingNodes: 2,
		TotalNodes:   3,
	}

	if d.EventType() != corev1.EventTypeNormal {
		t.Errorf("EventType() = %q, want %q", d.EventType(), corev1.EventTypeNormal)
	}

	if d.EventReason() != "NodepoolAccepted" {
		t.Errorf("EventReason() = %q, want %q", d.EventReason(), "NodepoolAccepted")
	}

	want := "nodepool/gpu-pool: fits 2/3 nodes"
	if d.Message() != want {
		t.Errorf("Message() = %q, want %q", d.Message(), want)
	}
}

func TestNodepoolDiagnosis_WarningVerdicts(t *testing.T) {
	tests := []struct {
		name       string
		diag       diagnosis.NodepoolDiagnosis
		wantReason string
		wantMsg    string
	}{
		{
			name: "Rejected",
			diag: diagnosis.NodepoolDiagnosis{
				NodepoolID:   "pool-2",
				NodepoolName: "cpu-pool",
				Verdict:      diagnosis.Rejected,
				Rejection: &diagnosis.Rejection{
					Category: diagnosis.CategoryTaint,
					Reason:   "taint node.kubernetes.io/not-ready not tolerated",
				},
				TotalNodes: 2,
			},
			wantReason: "NodepoolRejected",
			wantMsg:    "nodepool/cpu-pool: taint node.kubernetes.io/not-ready not tolerated",
		},
		{
			name: "Candidate",
			diag: diagnosis.NodepoolDiagnosis{
				NodepoolID:   "pool-3",
				NodepoolName: "spot-pool",
				Verdict:      diagnosis.Candidate,
				Rejection: &diagnosis.Rejection{
					Category: diagnosis.CategoryResources,
					Reason:   "cpu requested 4, allocatable 2",
				},
				TotalNodes: 1,
			},
			wantReason: "NodepoolCandidate",
			wantMsg:    "nodepool/spot-pool: cpu requested 4, allocatable 2",
		},
		{
			name: "NoStock",
			diag: diagnosis.NodepoolDiagnosis{
				NodepoolID:   "pool-4",
				NodepoolName: "highmem-pool",
				Verdict:      diagnosis.NoStock,
				Rejection: &diagnosis.Rejection{
					Category: diagnosis.CategoryResources,
					Reason:   "inventory unhealthy, scaling blocked",
				},
				TotalNodes: 0,
			},
			wantReason: "NodepoolNoStock",
			wantMsg:    "nodepool/highmem-pool: inventory unhealthy, scaling blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.diag.EventType() != corev1.EventTypeWarning {
				t.Errorf("EventType() = %q, want %q", tt.diag.EventType(), corev1.EventTypeWarning)
			}

			if tt.diag.EventReason() != tt.wantReason {
				t.Errorf("EventReason() = %q, want %q", tt.diag.EventReason(), tt.wantReason)
			}

			if tt.diag.Message() != tt.wantMsg {
				t.Errorf("Message() = %q, want %q", tt.diag.Message(), tt.wantMsg)
			}
		})
	}
}
