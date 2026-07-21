package metrics

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace     = "default"
	testAPIAppsV1     = "apps/v1"
	testAPIBatchV1    = "batch/v1"
	kindJob           = "Job"
	kindStatefulSet   = "StatefulSet"
	kindDaemonSet     = "DaemonSet"
	kindDeployment    = "Deployment"
	testStandaloneRS  = "standalone-rs"
	testDeploymentWeb = "web"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)

	return s
}

func newPodWithOwner(name, apiVersion, kind, ownerName, uid string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: apiVersion,
					Kind:       kind,
					Name:       ownerName,
					UID:        types.UID(uid),
				},
			},
		},
	}
}

func TestResolveOwner_DirectOwners(t *testing.T) {
	tests := []struct {
		name       string
		podName    string
		apiVersion string
		ownerKind  string
		ownerName  string
		uid        string
	}{
		{
			name:       kindJob,
			podName:    "job-pod",
			apiVersion: testAPIBatchV1,
			ownerKind:  kindJob,
			ownerName:  "my-job",
			uid:        "job-uid",
		},
		{
			name:       kindStatefulSet,
			podName:    "ss-pod",
			apiVersion: testAPIAppsV1,
			ownerKind:  kindStatefulSet,
			ownerName:  "my-sts",
			uid:        "sts-uid",
		},
		{
			name:       kindDaemonSet,
			podName:    "ds-pod",
			apiVersion: testAPIAppsV1,
			ownerKind:  kindDaemonSet,
			ownerName:  "my-ds",
			uid:        "ds-uid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pod := newPodWithOwner(tc.podName, tc.apiVersion, tc.ownerKind, tc.ownerName, tc.uid)
			cl := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
			cache := make(map[types.UID]ownerInfo)

			info := resolveOwner(context.Background(), cl, pod, cache)

			if info.Kind != tc.ownerKind {
				t.Errorf("Kind = %q, want %q", info.Kind, tc.ownerKind)
			}

			if info.Name != tc.ownerName {
				t.Errorf("Name = %q, want %q", info.Name, tc.ownerName)
			}
		})
	}
}

func TestResolveOwner_ReplicaSetOwnedByDeployment(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-abc123",
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

	pod := newPodWithOwner("web-abc123-xyz", testAPIAppsV1, kindReplicaSet, "web-abc123", "rs-uid")
	cl := fakeclient.NewClientBuilder().WithScheme(testScheme()).WithObjects(rs).Build()
	cache := make(map[types.UID]ownerInfo)

	info := resolveOwner(context.Background(), cl, pod, cache)

	if info.Kind != kindDeployment {
		t.Errorf("Kind = %q, want %q", info.Kind, kindDeployment)
	}

	if info.Name != testDeploymentWeb {
		t.Errorf("Name = %q, want %q", info.Name, testDeploymentWeb)
	}
}

func TestResolveOwner_ReplicaSetWithoutDeployment(t *testing.T) {
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testStandaloneRS,
			Namespace: testNamespace,
			UID:       types.UID("rs-uid-2"),
		},
	}

	pod := newPodWithOwner("standalone-rs-pod", testAPIAppsV1, kindReplicaSet, testStandaloneRS, "rs-uid-2")
	cl := fakeclient.NewClientBuilder().WithScheme(testScheme()).WithObjects(rs).Build()
	cache := make(map[types.UID]ownerInfo)

	info := resolveOwner(context.Background(), cl, pod, cache)

	if info.Kind != kindReplicaSet {
		t.Errorf("Kind = %q, want %q", info.Kind, kindReplicaSet)
	}

	if info.Name != testStandaloneRS {
		t.Errorf("Name = %q, want %q", info.Name, testStandaloneRS)
	}
}

func TestResolveOwner_ReplicaSetGetFails(t *testing.T) {
	pod := newPodWithOwner("rs-missing-pod", testAPIAppsV1, kindReplicaSet, "missing-rs", "rs-uid-3")
	cl := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
	cache := make(map[types.UID]ownerInfo)

	info := resolveOwner(context.Background(), cl, pod, cache)

	if info.Kind != kindReplicaSet {
		t.Errorf("Kind = %q, want %q", info.Kind, kindReplicaSet)
	}

	if info.Name != "missing-rs" {
		t.Errorf("Name = %q, want %q", info.Name, "missing-rs")
	}
}

func TestResolveOwner_ReplicaSetCache(t *testing.T) {
	const cachedDeploy = "cached-deploy"

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cached-rs",
			Namespace: testNamespace,
			UID:       types.UID("rs-uid-4"),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: testAPIAppsV1,
					Kind:       kindDeployment,
					Name:       cachedDeploy,
					UID:        types.UID("deploy-uid-4"),
				},
			},
		},
	}

	pod1 := newPodWithOwner("cached-pod-1", testAPIAppsV1, kindReplicaSet, "cached-rs", "rs-uid-4")
	pod2 := newPodWithOwner("cached-pod-2", testAPIAppsV1, kindReplicaSet, "cached-rs", "rs-uid-4")

	cl := fakeclient.NewClientBuilder().WithScheme(testScheme()).WithObjects(rs).Build()
	cache := make(map[types.UID]ownerInfo)

	info1 := resolveOwner(context.Background(), cl, pod1, cache)
	info2 := resolveOwner(context.Background(), cl, pod2, cache)

	if info1.Kind != kindDeployment || info1.Name != cachedDeploy {
		t.Errorf("pod1: got %+v, want {%s, %s}", info1, kindDeployment, cachedDeploy)
	}

	if info2.Kind != kindDeployment || info2.Name != cachedDeploy {
		t.Errorf("pod2: got %+v, want {%s, %s}", info2, kindDeployment, cachedDeploy)
	}

	if len(cache) != 1 {
		t.Errorf("cache size = %d, want 1", len(cache))
	}
}

func TestResolveOwner_StandalonePod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone-pod",
			Namespace: testNamespace,
		},
	}

	cl := fakeclient.NewClientBuilder().WithScheme(testScheme()).Build()
	cache := make(map[types.UID]ownerInfo)

	info := resolveOwner(context.Background(), cl, pod, cache)

	if info.Kind != "" {
		t.Errorf("Kind = %q, want empty", info.Kind)
	}

	if info.Name != "" {
		t.Errorf("Name = %q, want empty", info.Name)
	}
}
