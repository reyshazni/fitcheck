package diagnosis_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

const (
	taintKeyDedicated = "dedicated"
	taintKeySpecial   = "special"
	testValueGPU      = "gpu"
	testValueTrue     = "true"
	testValueFalse    = "false"
	testValueTainted  = "tainted"
	testZoneEast1a    = "us-east-1a"
	testLabelZone     = "zone"
	testZoneEast      = "us-east"
	testZoneWest      = "us-west"
	testPoolID1       = "pool-1"
	testNodeName1     = "node-1"
)

func TestCheckTaints_AllTolerated(t *testing.T) {
	taints := []corev1.Taint{
		{Key: taintKeyDedicated, Value: testValueGPU, Effect: corev1.TaintEffectNoSchedule},
	}
	tolerations := []corev1.Toleration{
		{Key: taintKeyDedicated, Operator: corev1.TolerationOpEqual, Value: testValueGPU, Effect: corev1.TaintEffectNoSchedule},
	}

	got := diagnosis.CheckTaints(tolerations, taints)
	if got != nil {
		t.Errorf("CheckTaints() = %v, want nil", got)
	}
}

func TestCheckTaints_MissingToleration(t *testing.T) {
	taints := []corev1.Taint{
		{Key: taintKeyDedicated, Value: testValueGPU, Effect: corev1.TaintEffectNoSchedule},
	}

	got := diagnosis.CheckTaints(nil, taints)
	if got == nil {
		t.Fatal("CheckTaints() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryTaint {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryTaint)
	}
}

func TestCheckTaints_EmptyTaints(t *testing.T) {
	got := diagnosis.CheckTaints(nil, nil)
	if got != nil {
		t.Errorf("CheckTaints() = %v, want nil", got)
	}
}

func TestCheckTaints_WildcardToleration(t *testing.T) {
	taints := []corev1.Taint{
		{Key: taintKeyDedicated, Value: testValueGPU, Effect: corev1.TaintEffectNoSchedule},
		{Key: taintKeySpecial, Value: testValueTrue, Effect: corev1.TaintEffectNoExecute},
	}
	tolerations := []corev1.Toleration{
		{Operator: corev1.TolerationOpExists},
	}

	got := diagnosis.CheckTaints(tolerations, taints)
	if got != nil {
		t.Errorf("CheckTaints() = %v, want nil", got)
	}
}

func TestCheckTaints_NoScheduleNotTolerated(t *testing.T) {
	taints := []corev1.Taint{
		{Key: "node.kubernetes.io/not-ready", Effect: corev1.TaintEffectNoSchedule},
	}

	got := diagnosis.CheckTaints(nil, taints)
	if got == nil {
		t.Fatal("CheckTaints() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryTaint {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryTaint)
	}
}

func TestCheckTaints_NoExecuteNotTolerated(t *testing.T) {
	taints := []corev1.Taint{
		{Key: "node.kubernetes.io/unreachable", Effect: corev1.TaintEffectNoExecute},
	}

	got := diagnosis.CheckTaints(nil, taints)
	if got == nil {
		t.Fatal("CheckTaints() = nil, want Rejection")
	}

	if got.Category != diagnosis.CategoryTaint {
		t.Errorf("Category = %d, want %d", got.Category, diagnosis.CategoryTaint)
	}
}

func TestCheckTaints_PreferNoScheduleIgnored(t *testing.T) {
	taints := []corev1.Taint{
		{Key: "prefer-zone", Value: testZoneEast, Effect: corev1.TaintEffectPreferNoSchedule},
	}

	got := diagnosis.CheckTaints(nil, taints)
	if got != nil {
		t.Errorf("CheckTaints() = %v, want nil (PreferNoSchedule should be ignored)", got)
	}
}
