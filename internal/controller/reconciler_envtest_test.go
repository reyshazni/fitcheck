//go:build envtest

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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/reyshazni/fitcheck/internal/controller"
	"github.com/reyshazni/fitcheck/internal/diagnosis"
	"github.com/reyshazni/fitcheck/internal/provider/ack"
)

const (
	envtestNamespace = "test-ns"
	envtestPodName   = "test-pod"
	gpuPoolID        = "gpu-pool"
	cpuPoolID        = "cpu-pool"
	nodepoolLabelKey = "alibabacloud.com/nodepool-id"
)

func TestReconciler_Envtest(t *testing.T) {
	testEnv := &envtest.Environment{}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("starting envtest: %v", err)
	}

	t.Cleanup(func() {
		if stopErr := testEnv.Stop(); stopErr != nil {
			t.Errorf("stopping envtest: %v", stopErr)
		}
	})

	cl, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}

	ctx := context.Background()

	createNamespace(t, cl, ctx)
	createNodes(t, cl, ctx)
	createPendingPod(t, cl, ctx)

	recorder := &events.FakeRecorder{Events: make(chan string, 10)}

	r := &controller.PodReconciler{
		Client:          cl,
		Recorder:        recorder,
		Provider:        ack.New(),
		RecheckInterval: 30 * time.Second,
		InitialDelay:    0,
		StartupTimeout:  10 * time.Minute,
	}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: envtestPodName, Namespace: envtestNamespace}}
	result, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if result.RequeueAfter != 30*time.Second {
		t.Errorf("RequeueAfter = %v, want 30s", result.RequeueAfter)
	}

	verifyAnnotation(t, cl, ctx, req.NamespacedName)
	verifyEvents(t, recorder)
}

func createNamespace(t *testing.T, cl client.Client, ctx context.Context) {
	t.Helper()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: envtestNamespace}}
	if err := cl.Create(ctx, ns); err != nil {
		t.Fatalf("creating namespace: %v", err)
	}
}

func createNodes(t *testing.T, cl client.Client, ctx context.Context) {
	t.Helper()

	gpuNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "gpu-node-1",
			Labels: map[string]string{nodepoolLabelKey: gpuPoolID, "name": "gpu-pool-name"},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{
				{Key: "dedicated", Value: "gpu", Effect: corev1.TaintEffectNoSchedule},
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("8"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
			},
		},
	}

	cpuNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "cpu-node-1",
			Labels: map[string]string{nodepoolLabelKey: cpuPoolID, "name": "cpu-pool-name"},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
		},
	}

	for _, node := range []*corev1.Node{gpuNode, cpuNode} {
		savedTaints := node.Spec.Taints
		status := node.Status.DeepCopy()

		if err := cl.Create(ctx, node); err != nil {
			t.Fatalf("creating node %s: %v", node.Name, err)
		}

		node.Status = *status
		if err := cl.Status().Update(ctx, node); err != nil {
			t.Fatalf("updating node status %s: %v", node.Name, err)
		}

		// Re-fetch and overwrite taints to remove any auto-added taints.
		if err := cl.Get(ctx, types.NamespacedName{Name: node.Name}, node); err != nil {
			t.Fatalf("re-fetching node %s: %v", node.Name, err)
		}

		node.Spec.Taints = savedTaints
		if err := cl.Update(ctx, node); err != nil {
			t.Fatalf("clearing taints on %s: %v", node.Name, err)
		}
	}
}

func createPendingPod(t *testing.T, cl client.Client, ctx context.Context) {
	t.Helper()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      envtestPodName,
			Namespace: envtestNamespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "main",
					Image: "busybox",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
		},
	}

	if err := cl.Create(ctx, pod); err != nil {
		t.Fatalf("creating pod: %v", err)
	}

	pod.Status.Phase = corev1.PodPending
	if err := cl.Status().Update(ctx, pod); err != nil {
		t.Fatalf("updating pod status: %v", err)
	}
}

func verifyAnnotation(
	t *testing.T,
	cl client.Client,
	ctx context.Context,
	key types.NamespacedName,
) {
	t.Helper()

	var pod corev1.Pod
	if err := cl.Get(ctx, key, &pod); err != nil {
		t.Fatalf("getting pod for annotation check: %v", err)
	}

	ann, ok := pod.Annotations[diagnosis.AnnotationKey]
	if !ok {
		t.Fatalf("expected annotation %q to be set", diagnosis.AnnotationKey)
	}

	var report diagnosis.DiagnosisReport
	if err := json.Unmarshal([]byte(ann), &report); err != nil {
		t.Fatalf("invalid annotation JSON: %v", err)
	}

	if report.Timestamp == "" {
		t.Error("report.Timestamp is empty")
	}

	if !strings.Contains(report.Summary, "nodepools fit") {
		t.Errorf("summary = %q, expected 'nodepools fit'", report.Summary)
	}

	if len(report.Nodepools) != 2 {
		t.Errorf("nodepools count = %d, want 2", len(report.Nodepools))
	}

	hasAccepted := false
	hasRejected := false

	for _, np := range report.Nodepools {
		if np.Verdict == "accepted" && np.Name == "cpu-pool-name" {
			hasAccepted = true
		}

		if np.Verdict == "rejected" && np.Name == "gpu-pool-name" {
			hasRejected = true
		}
	}

	if !hasAccepted {
		t.Errorf("expected accepted nodepool cpu-pool-name, got: %s", ann)
	}

	if !hasRejected {
		t.Errorf("expected rejected nodepool gpu-pool-name, got: %s", ann)
	}
}

func verifyEvents(t *testing.T, recorder *events.FakeRecorder) {
	t.Helper()

	collected := drainEvents(recorder)

	if len(collected) != 1 {
		t.Fatalf("expected 1 event, got %d: %v", len(collected), collected)
	}

	event := collected[0]

	if !strings.Contains(event, "nodepools fit") {
		t.Errorf("expected event with 'nodepools fit', got: %q", event)
	}
}

func drainEvents(recorder *events.FakeRecorder) []string {
	collected := make([]string, 0)

	for {
		select {
		case e := <-recorder.Events:
			collected = append(collected, e)
		default:
			return collected
		}
	}
}
