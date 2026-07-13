package diagnosis_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

func TestCheckNodeAffinity_Nil(t *testing.T) {
	got := diagnosis.CheckNodeAffinity(nil, map[string]string{testLabelZone: testZoneEast})
	if got != nil {
		t.Errorf("CheckNodeAffinity(nil) = %v, want nil", got)
	}
}

func TestCheckNodeAffinity_SingleTermMatches(t *testing.T) {
	affinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: testValueGPU, Operator: corev1.NodeSelectorOpIn, Values: []string{testValueTrue}},
					},
				},
			},
		},
	}
	labels := map[string]string{testValueGPU: testValueTrue}

	got := diagnosis.CheckNodeAffinity(affinity, labels)
	if got != nil {
		t.Errorf("CheckNodeAffinity() = %v, want nil", got)
	}
}

func TestCheckNodeAffinity_SingleTermFails(t *testing.T) {
	affinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: testValueGPU, Operator: corev1.NodeSelectorOpIn, Values: []string{testValueTrue}},
					},
				},
			},
		},
	}
	labels := map[string]string{testValueGPU: testValueFalse}

	got := diagnosis.CheckNodeAffinity(affinity, labels)
	if got == nil {
		t.Fatal("CheckNodeAffinity() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryAffinity {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryAffinity)
	}
}

func TestCheckNodeAffinity_MultipleTermsOneMatches(t *testing.T) {
	affinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: testLabelZone, Operator: corev1.NodeSelectorOpIn, Values: []string{testZoneWest}},
					},
				},
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: testLabelZone, Operator: corev1.NodeSelectorOpIn, Values: []string{testZoneEast}},
					},
				},
			},
		},
	}
	labels := map[string]string{testLabelZone: testZoneEast}

	got := diagnosis.CheckNodeAffinity(affinity, labels)
	if got != nil {
		t.Errorf("CheckNodeAffinity() = %v, want nil (OR semantics)", got)
	}
}

func TestCheckNodeAffinity_ANDLogicOneFails(t *testing.T) {
	affinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: testValueGPU, Operator: corev1.NodeSelectorOpIn, Values: []string{testValueTrue}},
						{Key: testLabelZone, Operator: corev1.NodeSelectorOpIn, Values: []string{testZoneWest}},
					},
				},
			},
		},
	}
	labels := map[string]string{testValueGPU: testValueTrue, testLabelZone: testZoneEast}

	got := diagnosis.CheckNodeAffinity(affinity, labels)
	if got == nil {
		t.Fatal("CheckNodeAffinity() = nil, want Rejection (AND within term)")
	}
}

func TestCheckNodeAffinity_OperatorExists(t *testing.T) {
	affinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: testValueGPU, Operator: corev1.NodeSelectorOpExists},
					},
				},
			},
		},
	}
	labels := map[string]string{testValueGPU: "any-value"}

	got := diagnosis.CheckNodeAffinity(affinity, labels)
	if got != nil {
		t.Errorf("CheckNodeAffinity(Exists) = %v, want nil", got)
	}
}

func TestCheckNodeAffinity_OperatorDoesNotExist(t *testing.T) {
	affinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: "tainted", Operator: corev1.NodeSelectorOpDoesNotExist},
					},
				},
			},
		},
	}
	labels := map[string]string{testValueGPU: testValueTrue}

	got := diagnosis.CheckNodeAffinity(affinity, labels)
	if got != nil {
		t.Errorf("CheckNodeAffinity(DoesNotExist) = %v, want nil", got)
	}
}

func TestCheckNodeAffinity_OperatorNotIn(t *testing.T) {
	affinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: testLabelZone, Operator: corev1.NodeSelectorOpNotIn, Values: []string{testZoneWest, "eu-central"}},
					},
				},
			},
		},
	}
	labels := map[string]string{testLabelZone: testZoneEast}

	got := diagnosis.CheckNodeAffinity(affinity, labels)
	if got != nil {
		t.Errorf("CheckNodeAffinity(NotIn) = %v, want nil", got)
	}
}
