package diagnosis_test

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

func TestDiagnoseAll(t *testing.T) {
	pod := newTestPod()

	nodepools := []diagnosis.NodepoolInfo{
		{
			ID:   poolA,
			Name: "fits",
			Nodes: []diagnosis.NodeInfo{
				{
					Name:   testNodeName1,
					Labels: map[string]string{},
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			},
		},
		{
			ID:   poolB,
			Name: "too-small",
			Nodes: []diagnosis.NodeInfo{
				{
					Name:   testNodeName2,
					Labels: map[string]string{},
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
			},
		},
	}

	results := diagnosis.DiagnoseAll(pod, nodepools, 10*time.Minute)

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	if results[0].Verdict != diagnosis.Accepted {
		t.Errorf("pool-a Verdict = %q, want %q", results[0].Verdict, diagnosis.Accepted)
	}

	if results[1].Verdict != diagnosis.Rejected {
		t.Errorf("pool-b Verdict = %q, want %q", results[1].Verdict, diagnosis.Rejected)
	}
}
