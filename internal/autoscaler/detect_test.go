package autoscaler_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/reyshazni/fitcheck/internal/autoscaler"
)

const testNSKubeSystem = "kube-system"

func TestDetect_GOATScaler(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "autoscaler-meta",
			Namespace: testNSKubeSystem,
		},
		Data: map[string]string{
			"scaler-type": testGOATScaler,
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(cm).
		Build()

	reader, err := autoscaler.Detect(context.Background(), cl)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if reader == nil {
		t.Error("Detect() = nil, want GOATScalerReader")
	}
}

func TestDetect_NoConfigMaps(t *testing.T) {
	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		Build()

	reader, err := autoscaler.Detect(context.Background(), cl)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if reader != nil {
		t.Errorf("Detect() = %v, want nil", reader)
	}
}

func TestDetect_ClusterAutoscalerExists(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-autoscaler-status",
			Namespace: testNSKubeSystem,
		},
		Data: map[string]string{
			"status": "running",
		},
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(testScheme()).
		WithObjects(cm).
		Build()

	reader, err := autoscaler.Detect(context.Background(), cl)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if reader != nil {
		t.Errorf("Detect() = %v, want nil (CA not yet supported)", reader)
	}
}
