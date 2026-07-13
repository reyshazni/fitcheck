package nodepool_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/reyshazni/fitcheck/internal/nodepool"
)

const (
	testLabelKey     = "node.kubernetes.io/nodepool"
	testNameLabelKey = "name"
	testNodeName     = "node-1"
	testPoolGeneral  = "general"
	testPoolNoName   = "pool-no-name"
	testPoolMyPool   = "my-pool"
	testPoolA        = "pool-a"
)

func TestCollector_GroupsByLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testNodeName,
				Labels: map[string]string{testLabelKey: testPoolA, testNameLabelKey: testPoolGeneral},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-2",
				Labels: map[string]string{testLabelKey: testPoolA, testNameLabelKey: testPoolGeneral},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-3",
				Labels: map[string]string{testLabelKey: "pool-b", testNameLabelKey: "gpu"},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("8"),
				},
			},
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&nodes[0], &nodes[1], &nodes[2]).
		Build()

	c := nodepool.Collector{}
	pools, err := c.Collect(context.Background(), cl, testLabelKey, testNameLabelKey)

	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(pools) != 2 {
		t.Fatalf("len(pools) = %d, want 2", len(pools))
	}

	poolMap := make(map[string]int)
	for _, p := range pools {
		poolMap[p.ID] = len(p.Nodes)
	}

	if poolMap[testPoolA] != 2 {
		t.Errorf("pool-a nodes = %d, want 2", poolMap[testPoolA])
	}

	if poolMap["pool-b"] != 1 {
		t.Errorf("pool-b nodes = %d, want 1", poolMap["pool-b"])
	}
}

func TestCollector_SkipsNodesWithoutLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	nodes := []corev1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   testNodeName,
				Labels: map[string]string{testLabelKey: testPoolA},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "node-unlabeled",
				Labels: map[string]string{"other": "label"},
			},
			Status: corev1.NodeStatus{
				Allocatable: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&nodes[0], &nodes[1]).
		Build()

	c := nodepool.Collector{}
	pools, err := c.Collect(context.Background(), cl, testLabelKey, testNameLabelKey)

	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(pools) != 1 {
		t.Fatalf("len(pools) = %d, want 1", len(pools))
	}

	if pools[0].ID != testPoolA {
		t.Errorf("pool ID = %q, want %q", pools[0].ID, testPoolA)
	}
}

func TestCollector_NameLabelFallback(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testNodeName,
			Labels: map[string]string{testLabelKey: testPoolNoName},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			},
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&node).
		Build()

	c := nodepool.Collector{}
	pools, err := c.Collect(context.Background(), cl, testLabelKey, testNameLabelKey)

	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if len(pools) != 1 {
		t.Fatalf("len(pools) = %d, want 1", len(pools))
	}

	if pools[0].Name != testPoolNoName {
		t.Errorf("Name = %q, want %q (fallback to ID)", pools[0].Name, testPoolNoName)
	}
}

func TestCollector_NameLabelPresent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testNodeName,
			Labels: map[string]string{testLabelKey: "pool-x", testNameLabelKey: testPoolMyPool},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("4"),
			},
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&node).
		Build()

	c := nodepool.Collector{}
	pools, err := c.Collect(context.Background(), cl, testLabelKey, testNameLabelKey)

	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	if pools[0].Name != testPoolMyPool {
		t.Errorf("Name = %q, want %q", pools[0].Name, testPoolMyPool)
	}
}
