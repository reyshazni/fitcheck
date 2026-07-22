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
	metricName     = "fitcheck_pending_pod_count"
	testRSName     = "web-abc123"
	testPodName    = "web-abc123-xyz"
	testTimestamp  = "2026-07-21T00:00:00Z"
	testSummaryFit = "1/1 nodepools fit"
	testPoolGen    = "general"
	testAccepted   = "accepted"
	testJobTrain1  = "train-1"
	testGPUPool    = "gpu-pool"
	testNoStock    = "no-stock"
	testCatTaint   = "taint"
)

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

	collector := NewPendingPodCollector(cl, cl)

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

	if !strings.Contains(desc, metricName) {
		t.Errorf("descriptor %q does not contain %q", desc, metricName)
	}

	expectedLabels := []string{labelNamespace, labelOwnerKind, labelOwnerName, labelVerdict, labelCategory}
	for _, label := range expectedLabels {
		if !strings.Contains(desc, label) {
			t.Errorf("descriptor missing label %q: %s", label, desc)
		}
	}
}

func TestCollect_SinglePendingPod(t *testing.T) {
	report := diagnosis.DiagnosisReport{
		Timestamp: testTimestamp,
		Summary:   testSummaryFit,
		Nodepools: []diagnosis.NodepoolResult{
			{Name: testPoolGen, Verdict: testAccepted, Fitting: 2, Total: 3},
			{Name: testGPUPool, Verdict: verdictRejected, Reason: testCatTaint, Category: testCatTaint},
		},
	}

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testRSName,
			Namespace: testNamespace,
			UID:       types.UID("rs-uid"),
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: testAPIAppsV1, Kind: kindDeployment, Name: testDeploymentWeb, UID: types.UID("deploy-uid")},
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
				{APIVersion: testAPIAppsV1, Kind: kindReplicaSet, Name: testRSName, UID: types.UID("rs-uid")},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(pod, rs).
		Build()

	collector := NewPendingPodCollector(cl, cl)

	expected := `
# HELP fitcheck_pending_pod_count Number of pending pods grouped by owner and scheduling verdict
# TYPE fitcheck_pending_pod_count gauge
fitcheck_pending_pod_count{category="",namespace="default",owner_kind="Deployment",owner_name="web",verdict="accepted"} 1
`

	err := testutil.CollectAndCompare(collector, strings.NewReader(expected), metricName)
	if err != nil {
		t.Errorf("metric mismatch: %v", err)
	}
}

func TestCollect_BestVerdictPicked(t *testing.T) {
	report := diagnosis.DiagnosisReport{
		Timestamp: testTimestamp,
		Summary:   "0/2 nodepools fit",
		Nodepools: []diagnosis.NodepoolResult{
			{Name: "pool-a", Verdict: verdictRejected, Category: testCatTaint},
			{Name: "pool-b", Verdict: testNoStock, Category: ""},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nostock-pod",
			Namespace: testNamespace,
			Annotations: map[string]string{
				diagnosis.AnnotationKey: annotationJSON(report),
			},
			OwnerReferences: []metav1.OwnerReference{
				{Kind: kindJob, Name: "batch-job", UID: types.UID("j-1")},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(pod).
		Build()

	collector := NewPendingPodCollector(cl, cl)

	expected := `
# HELP fitcheck_pending_pod_count Number of pending pods grouped by owner and scheduling verdict
# TYPE fitcheck_pending_pod_count gauge
fitcheck_pending_pod_count{category="",namespace="default",owner_kind="Job",owner_name="batch-job",verdict="no-stock"} 1
`

	err := testutil.CollectAndCompare(collector, strings.NewReader(expected), metricName)
	if err != nil {
		t.Errorf("metric mismatch: %v", err)
	}
}

func TestCollect_AggregatesMultiplePods(t *testing.T) {
	report := diagnosis.DiagnosisReport{
		Timestamp: testTimestamp,
		Summary:   "0/1 nodepools fit | rejected: 1 resource",
		Nodepools: []diagnosis.NodepoolResult{
			{Name: testGPUPool, Verdict: verdictRejected, Reason: "insufficient cpu", Category: "resource"},
		},
	}

	ann := annotationJSON(report)
	batchNS := "batch"

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "job-pod-1", Namespace: batchNS,
			Annotations:     map[string]string{diagnosis.AnnotationKey: ann},
			OwnerReferences: []metav1.OwnerReference{{Kind: kindJob, Name: testJobTrain1, UID: types.UID("job-1")}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "job-pod-2", Namespace: batchNS,
			Annotations:     map[string]string{diagnosis.AnnotationKey: ann},
			OwnerReferences: []metav1.OwnerReference{{Kind: kindJob, Name: testJobTrain1, UID: types.UID("job-1")}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	pod3 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other-pod", Namespace: batchNS,
			Annotations:     map[string]string{diagnosis.AnnotationKey: ann},
			OwnerReferences: []metav1.OwnerReference{{Kind: kindJob, Name: "train-2", UID: types.UID("job-2")}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(pod1, pod2, pod3).
		Build()

	collector := NewPendingPodCollector(cl, cl)

	expected := `
# HELP fitcheck_pending_pod_count Number of pending pods grouped by owner and scheduling verdict
# TYPE fitcheck_pending_pod_count gauge
fitcheck_pending_pod_count{category="resource",namespace="batch",owner_kind="Job",owner_name="train-1",verdict="rejected"} 2
fitcheck_pending_pod_count{category="resource",namespace="batch",owner_kind="Job",owner_name="train-2",verdict="rejected"} 1
`

	err := testutil.CollectAndCompare(collector, strings.NewReader(expected), metricName)
	if err != nil {
		t.Errorf("metric mismatch: %v", err)
	}
}

func TestCollect_ZeroPendingPods(t *testing.T) {
	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		Build()

	collector := NewPendingPodCollector(cl, cl)

	count := testutil.CollectAndCount(collector, metricName)
	if count != 0 {
		t.Errorf("series count = %d, want 0", count)
	}
}

func TestCollect_RunningPodsIgnored(t *testing.T) {
	report := diagnosis.DiagnosisReport{
		Timestamp: testTimestamp,
		Summary:   testSummaryFit,
		Nodepools: []diagnosis.NodepoolResult{
			{Name: testPoolGen, Verdict: testAccepted},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "running-pod", Namespace: testNamespace,
			Annotations: map[string]string{diagnosis.AnnotationKey: annotationJSON(report)},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(pod).
		Build()

	collector := NewPendingPodCollector(cl, cl)

	count := testutil.CollectAndCount(collector, metricName)
	if count != 0 {
		t.Errorf("series count = %d, want 0", count)
	}
}

func TestCollect_MalformedAnnotationSkipped(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bad-ann-pod", Namespace: testNamespace,
			Annotations: map[string]string{diagnosis.AnnotationKey: "not valid json{{{"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(pod).
		Build()

	collector := NewPendingPodCollector(cl, cl)

	count := testutil.CollectAndCount(collector, metricName)
	if count != 0 {
		t.Errorf("series count = %d, want 0", count)
	}
}

func TestCollect_PendingPodWithoutAnnotation(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "no-ann-pod", Namespace: testNamespace,
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(pod).
		Build()

	collector := NewPendingPodCollector(cl, cl)

	count := testutil.CollectAndCount(collector, metricName)
	if count != 0 {
		t.Errorf("series count = %d, want 0", count)
	}
}
