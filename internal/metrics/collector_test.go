package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

const (
	metricName  = "fitcheck_pending_pods"
	testRSName  = "web-abc123"
	testPodName = "web-abc123-xyz"
)

func containsSubstring(s, substr string) bool {
	return strings.Contains(s, substr)
}

func annotationJSON(report diagnosis.DiagnosisReport) string {
	data, err := diagnosis.MarshalReport(report)
	if err != nil {
		panic(err)
	}

	return data
}

func TestDescribe_SendsOneDescriptor(t *testing.T) {
	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		Build()

	collector := NewPendingPodCollector(cl)

	ch := make(chan *prometheus.Desc, 10)
	collector.Describe(ch)
	close(ch)

	descs := make([]*prometheus.Desc, 0)
	for d := range ch {
		descs = append(descs, d)
	}

	if len(descs) != 1 {
		t.Fatalf("Describe sent %d descriptors, want 1", len(descs))
	}

	desc := descs[0].String()

	wantName := "fitcheck_pending_pods"
	if !containsSubstring(desc, wantName) {
		t.Errorf("descriptor %q does not contain %q", desc, wantName)
	}

	expectedLabels := []string{labelNamespace, labelOwnerKind, labelOwnerName, labelNodepool, labelVerdict, labelCategory}
	for _, label := range expectedLabels {
		if !strings.Contains(desc, label) {
			t.Errorf("descriptor missing label %q: %s", label, desc)
		}
	}
}

func TestCollect_SinglePendingPod(t *testing.T) {
	report := diagnosis.DiagnosisReport{
		Timestamp: "2026-07-21T00:00:00Z",
		Summary:   "1/1 nodepools fit",
		Nodepools: []diagnosis.NodepoolResult{
			{
				Name:    "general",
				Verdict: "accepted",
				Fitting: 2,
				Total:   3,
			},
		},
	}

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testRSName,
			Namespace: testNamespace,
			UID:       types.UID("rs-uid"),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: testAPIAppsV1,
					Kind:       kindDeployment,
					Name:       testDeploymentWeb,
					UID:        types.UID("deploy-uid"),
				},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				diagnosis.AnnotationKey: annotationJSON(report),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: testAPIAppsV1,
					Kind:       kindReplicaSet,
					Name:       testRSName,
					UID:        types.UID("rs-uid"),
				},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(pod, rs).
		Build()

	collector := NewPendingPodCollector(cl)

	expected := `
# HELP fitcheck_pending_pods Number of pending pods grouped by owner, nodepool, and scheduling verdict
# TYPE fitcheck_pending_pods gauge
fitcheck_pending_pods{category="",namespace="default",nodepool="general",owner_kind="Deployment",owner_name="web",verdict="accepted"} 1
`

	err := testutil.CollectAndCompare(collector, strings.NewReader(expected), metricName)
	if err != nil {
		t.Errorf("metric mismatch: %v", err)
	}
}
