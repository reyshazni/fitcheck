package controller_test

import (
	"context"
	"encoding/json"
	"strings"
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
	"github.com/reyshazni/fitcheck/internal/diagnosis"
	"github.com/reyshazni/fitcheck/internal/provider/ack"
)

const (
	defaultNamespace  = "default"
	pendingPodName    = "pending-pod"
	runningPodName    = "running-pod"
	gonePodName       = "gone-pod"
	annotatedPodName  = "annotated-pod"
	wasPendingPodName = "was-pending-pod"
	cleanRunningPod   = "clean-running-pod"
	containerName     = "main"
	nameLabelKey      = "name"
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
					Name: containerName,
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
			Labels: map[string]string{prov.NodepoolLabelKey(): "pool-a", nameLabelKey: "general"},
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

	// Expect exactly one event with the new summary format.
	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, "nodepools fit") {
			t.Errorf("expected event with 'nodepools fit', got %q", event)
		}
	default:
		t.Error("expected at least one event to be emitted")
	}

	// No second event.
	select {
	case event := <-recorder.Events:
		t.Errorf("expected exactly one event, got extra: %q", event)
	default:
		// expected
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

func newPendingPod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         defaultNamespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Minute)),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: containerName,
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
}

func newNode(name, poolLabelKey, poolID, poolName string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{poolLabelKey: poolID, nameLabelKey: poolName},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			},
		},
	}
}

func TestReconcile_PendingPod_WritesAnnotation(t *testing.T) {
	prov := ack.New()
	pod := newPendingPod(annotatedPodName)
	node := newNode("node-ann-1", prov.NodepoolLabelKey(), "pool-ann", "ann-pool")

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(pod, node).
		WithStatusSubresource(pod).
		Build()

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}
	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        prov,
		RecheckInterval: 30 * time.Second,
		InitialDelay:    10 * time.Second,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: annotatedPodName, Namespace: defaultNamespace}}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var updated corev1.Pod
	if err := cl.Get(context.Background(), req.NamespacedName, &updated); err != nil {
		t.Fatalf("getting updated pod: %v", err)
	}

	ann, ok := updated.Annotations[diagnosis.AnnotationKey]
	if !ok {
		t.Fatalf("expected annotation %q to be set", diagnosis.AnnotationKey)
	}

	var report diagnosis.DiagnosisReport
	if err := json.Unmarshal([]byte(ann), &report); err != nil {
		t.Fatalf("unmarshaling annotation: %v", err)
	}

	if report.Timestamp == "" {
		t.Error("report.Timestamp is empty")
	}
	if report.Summary == "" {
		t.Error("report.Summary is empty")
	}
	if len(report.Nodepools) == 0 {
		t.Error("report.Nodepools is empty")
	}
}

func TestReconcile_RunningPod_RemovesAnnotation(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      wasPendingPodName,
			Namespace: defaultNamespace,
			Annotations: map[string]string{
				diagnosis.AnnotationKey: `{"timestamp":"2026-07-13T00:00:00Z","summary":"test","nodepools":[]}`,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
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

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: wasPendingPodName, Namespace: defaultNamespace}}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	var updated corev1.Pod
	if err := cl.Get(context.Background(), req.NamespacedName, &updated); err != nil {
		t.Fatalf("getting updated pod: %v", err)
	}

	if _, ok := updated.Annotations[diagnosis.AnnotationKey]; ok {
		t.Errorf("expected annotation %q to be removed", diagnosis.AnnotationKey)
	}
}

func TestReconcile_RunningPod_NoAnnotation_NoPatch(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cleanRunningPod,
			Namespace: defaultNamespace,
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
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

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: cleanRunningPod, Namespace: defaultNamespace}}
	result, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter != 0 {
		t.Errorf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}
