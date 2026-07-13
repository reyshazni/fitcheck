package autoscaler

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AutoscalerStatus holds the autoscaler state for a single nodepool.
type AutoscalerStatus struct {
	ScaleUpTriggered   bool
	ScaleUpFailed      bool
	InventoryUnhealthy bool
	Message            string
}

// StatusReader reads autoscaler status for a set of nodepools.
type StatusReader interface {
	ReadStatus(ctx context.Context, cl client.Client, pod *corev1.Pod, nodepoolIDs []string) (map[string]AutoscalerStatus, error)
}

const kubeSystemNamespace = "kube-system"

// Detect checks for known autoscaler ConfigMaps and returns the
// appropriate StatusReader. Returns nil if no supported autoscaler
// is detected.
func Detect(ctx context.Context, cl client.Client) (StatusReader, error) {
	found, err := configMapExists(ctx, cl, "cluster-autoscaler-status")
	if err != nil {
		return nil, fmt.Errorf("checking cluster-autoscaler-status: %w", err)
	}

	if found {
		slog.Info("cluster-autoscaler detected but not yet supported")

		return nil, nil
	}

	return detectGOATScaler(ctx, cl)
}

func detectGOATScaler(ctx context.Context, cl client.Client) (StatusReader, error) {
	var cm corev1.ConfigMap

	key := types.NamespacedName{Name: "autoscaler-meta", Namespace: kubeSystemNamespace}
	if err := cl.Get(ctx, key, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("checking autoscaler-meta: %w", err)
	}

	if cm.Data["scaler-type"] == "goatscaler" {
		slog.Info("detected GOATScaler autoscaler")

		return NewGOATScalerReader(), nil
	}

	return nil, nil
}

func configMapExists(ctx context.Context, cl client.Client, name string) (bool, error) {
	var cm corev1.ConfigMap

	key := types.NamespacedName{Name: name, Namespace: kubeSystemNamespace}
	if err := cl.Get(ctx, key, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("getting ConfigMap %s: %w", name, err)
	}

	return true, nil
}
