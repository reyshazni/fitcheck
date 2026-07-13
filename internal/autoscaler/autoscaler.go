package autoscaler

import (
	"context"

	corev1 "k8s.io/api/core/v1"
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
