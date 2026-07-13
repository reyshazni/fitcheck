package controller_test

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/reyshazni/fitcheck/internal/controller"
	"github.com/reyshazni/fitcheck/internal/provider/ack"
)

const (
	defaultNamespace = "default"
	pendingPodName   = "pending-pod"
	runningPodName   = "running-pod"
	gonePodName      = "gone-pod"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)

	return s
}

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
		if event == "" {
			t.Error("expected event, got empty string")
		}
	default:
		t.Error("expected at least one event to be emitted")
	}
}

func TestReconcile_NonPendingPod(t *testing.T) {
	scheme := testScheme()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      runningPodName,
			Namespace: defaultNamespace,
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod).
		Build()

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}

	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        ack.New(),
		RecheckInterval: 30 * time.Second,
		InitialDelay:    10 * time.Second,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: runningPodName, Namespace: defaultNamespace}}
	result, err := r.Reconcile(context.Background(), req)

	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0 (no requeue)", result.RequeueAfter)
	}

	select {
	case event := <-recorder.Events:
		t.Errorf("expected no events, got %q", event)
	default:
		// expected
	}
}

func TestReconcile_PodDeleted(t *testing.T) {
	scheme := testScheme()

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		Build()

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}

	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        ack.New(),
		RecheckInterval: 30 * time.Second,
		InitialDelay:    10 * time.Second,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: gonePodName, Namespace: defaultNamespace}}
	result, err := r.Reconcile(context.Background(), req)

	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}
