package ack_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/reyshazni/fitcheck/internal/provider"
	"github.com/reyshazni/fitcheck/internal/provider/ack"
)

const testNodepoolLabel = "alibabacloud.com/nodepool-id"

// Compile-time check: ACKProvider must implement Provider.
var _ provider.Provider = (*ack.ACKProvider)(nil)

func TestACKProvider_Name(t *testing.T) {
	p := ack.New()

	if got := p.Name(); got != "ack" {
		t.Errorf("Name() = %q, want %q", got, "ack")
	}
}

func TestACKProvider_NodepoolLabelKey(t *testing.T) {
	p := ack.New()

	if got := p.NodepoolLabelKey(); got != testNodepoolLabel {
		t.Errorf("NodepoolLabelKey() = %q, want %q", got, testNodepoolLabel)
	}
}

func TestACKProvider_NameLabelKey(t *testing.T) {
	p := ack.New()

	if got := p.NameLabelKey(); got != "name" {
		t.Errorf("NameLabelKey() = %q, want %q", got, "name")
	}
}

func TestACKProvider_Detect_Found(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ack-node",
			Labels: map[string]string{testNodepoolLabel: "pool-1"},
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		Build()

	p := ack.New()
	found, err := p.Detect(context.Background(), cl)

	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if !found {
		t.Error("Detect() = false, want true")
	}
}

func TestACKProvider_Detect_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "other-node",
			Labels: map[string]string{"other": "label"},
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		Build()

	p := ack.New()
	found, err := p.Detect(context.Background(), cl)

	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if found {
		t.Error("Detect() = true, want false")
	}
}
