package autoscaler_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/reyshazni/fitcheck/internal/autoscaler"
)

const (
	testPoolA      = "pool-a"
	testNSDefault  = "default"
	testKindPod    = "Pod"
	testGOATScaler = "goatscaler"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)

	return s
}

func testPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: testNSDefault,
			UID:       "pod-uid-123",
		},
	}
}

func TestGOATScaler_ProvisionNode(t *testing.T) {
	pod := testPod()
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "provision-event",
			Namespace: testNSDefault,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      testKindPod,
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       pod.UID,
		},
		Reason:  "ProvisionNode",
		Source:  corev1.EventSource{Component: testGOATScaler},
		Message: "Provisioning node in nodepool pool-a",
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(event).
		Build()

	reader := autoscaler.NewGOATScalerReader()
	statuses, err := reader.ReadStatus(context.Background(), cl, pod, []string{testPoolA})

	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}

	if len(statuses) == 0 {
		t.Fatal("statuses is empty, want at least one entry")
	}

	found := false
	for _, s := range statuses {
		if s.ScaleUpTriggered {
			found = true
		}
	}

	if !found {
		t.Error("expected ScaleUpTriggered = true for at least one status")
	}
}

func TestGOATScaler_NotTriggerScaleUp(t *testing.T) {
	pod := testPod()
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-trigger-event",
			Namespace: testNSDefault,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:      testKindPod,
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       pod.UID,
		},
		Reason:  "NotTriggerScaleUp",
		Source:  corev1.EventSource{Component: testGOATScaler},
		Message: "Pod does not match any nodepool",
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(event).
		Build()

	reader := autoscaler.NewGOATScalerReader()
	statuses, err := reader.ReadStatus(context.Background(), cl, pod, []string{testPoolA})

	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}

	found := false
	for _, s := range statuses {
		if s.ScaleUpFailed {
			found = true

			if s.Message == "" {
				t.Error("expected non-empty Message when ScaleUpFailed")
			}
		}
	}

	if !found {
		t.Error("expected ScaleUpFailed = true for at least one status")
	}
}

func TestGOATScaler_InventoryUnhealthy(t *testing.T) {
	pod := testPod()
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "inventory-event",
			Namespace: testNSDefault,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "ACKNodePool",
			Name: testPoolA,
		},
		Reason:  "InstanceInventoryStatusChanged",
		Message: "Instance inventory status changed from Healthy to UnHealthy",
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(event).
		Build()

	reader := autoscaler.NewGOATScalerReader()
	statuses, err := reader.ReadStatus(context.Background(), cl, pod, []string{testPoolA})

	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}

	s, ok := statuses[testPoolA]
	if !ok {
		t.Fatal("statuses missing pool-a")
	}

	if !s.InventoryUnhealthy {
		t.Error("expected InventoryUnhealthy = true")
	}
}

func TestGOATScaler_NoEvents(t *testing.T) {
	pod := testPod()

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		Build()

	reader := autoscaler.NewGOATScalerReader()
	statuses, err := reader.ReadStatus(context.Background(), cl, pod, []string{testPoolA})

	if err != nil {
		t.Fatalf("ReadStatus() error = %v", err)
	}

	if len(statuses) != 0 {
		t.Errorf("len(statuses) = %d, want 0", len(statuses))
	}
}
