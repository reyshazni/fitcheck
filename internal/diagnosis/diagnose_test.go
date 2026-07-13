package diagnosis_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

func newTestPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "main",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
		},
	}
}

func newFittingNode(name string) diagnosis.NodeInfo {
	return diagnosis.NodeInfo{
		Name:   name,
		Labels: map[string]string{},
		Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		},
	}
}

func TestDiagnoseNodepool_AllFit(t *testing.T) {
	pod := newTestPod()
	np := diagnosis.NodepoolInfo{
		ID:    testPoolID1,
		Name:  "general",
		Nodes: []diagnosis.NodeInfo{newFittingNode(testNodeName1), newFittingNode("node-2")},
	}

	d := diagnosis.DiagnoseNodepool(pod, np)

	if d.Verdict != diagnosis.Accepted {
		t.Errorf("Verdict = %q, want %q", d.Verdict, diagnosis.Accepted)
	}

	if d.FittingNodes != 2 {
		t.Errorf("FittingNodes = %d, want 2", d.FittingNodes)
	}

	if d.TotalNodes != 2 {
		t.Errorf("TotalNodes = %d, want 2", d.TotalNodes)
	}
}

func TestDiagnoseNodepool_SomeFit(t *testing.T) {
	pod := newTestPod()
	np := diagnosis.NodepoolInfo{
		ID:   testPoolID1,
		Name: "mixed",
		Nodes: []diagnosis.NodeInfo{
			newFittingNode(testNodeName1),
			{
				Name:   "node-2",
				Labels: map[string]string{},
				Taints: []corev1.Taint{
					{Key: taintKeyDedicated, Value: taintKeySpecial, Effect: corev1.TaintEffectNoSchedule},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("4"),
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			},
		},
	}

	d := diagnosis.DiagnoseNodepool(pod, np)

	if d.Verdict != diagnosis.Accepted {
		t.Errorf("Verdict = %q, want %q", d.Verdict, diagnosis.Accepted)
	}

	if d.FittingNodes != 1 {
		t.Errorf("FittingNodes = %d, want 1", d.FittingNodes)
	}
}

func TestDiagnoseNodepool_AllRejectTaint(t *testing.T) {
	pod := newTestPod()
	np := diagnosis.NodepoolInfo{
		ID:   testPoolID1,
		Name: testValueTainted,
		Nodes: []diagnosis.NodeInfo{
			{
				Name:   testNodeName1,
				Labels: map[string]string{},
				Taints: []corev1.Taint{
					{Key: taintKeyDedicated, Value: testValueGPU, Effect: corev1.TaintEffectNoSchedule},
				},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
		},
	}

	d := diagnosis.DiagnoseNodepool(pod, np)

	if d.Verdict != diagnosis.Rejected {
		t.Errorf("Verdict = %q, want %q", d.Verdict, diagnosis.Rejected)
	}

	if d.Rejection == nil {
		t.Fatal("Rejection is nil, want non-nil")
	}
}

func TestDiagnoseNodepool_AllRejectResources(t *testing.T) {
	pod := newTestPod()
	np := diagnosis.NodepoolInfo{
		ID:   testPoolID1,
		Name: "tiny",
		Nodes: []diagnosis.NodeInfo{
			{
				Name:   testNodeName1,
				Labels: map[string]string{},
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
		},
	}

	d := diagnosis.DiagnoseNodepool(pod, np)

	if d.Verdict != diagnosis.Rejected {
		t.Errorf("Verdict = %q, want %q", d.Verdict, diagnosis.Rejected)
	}

	if d.Rejection == nil {
		t.Fatal("Rejection is nil, want non-nil")
	}

	if d.Rejection.Category != diagnosis.CategoryResources {
		t.Errorf("Category = %d, want %d", d.Rejection.Category, diagnosis.CategoryResources)
	}
}

func TestDiagnoseNodepool_ZeroNodes(t *testing.T) {
	pod := newTestPod()
	np := diagnosis.NodepoolInfo{
		ID:    testPoolID1,
		Name:  "empty",
		Nodes: []diagnosis.NodeInfo{},
	}

	d := diagnosis.DiagnoseNodepool(pod, np)

	if d.Verdict != diagnosis.Rejected {
		t.Errorf("Verdict = %q, want %q", d.Verdict, diagnosis.Rejected)
	}

	if d.Rejection == nil {
		t.Fatal("Rejection is nil, want non-nil")
	}
}
