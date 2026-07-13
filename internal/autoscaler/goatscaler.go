package autoscaler

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GOATScalerReader reads autoscaler status from GOATScaler events.
type GOATScalerReader struct{}

const (
	reasonProvisionNode          = "ProvisionNode"
	reasonNotTriggerScaleUp      = "NotTriggerScaleUp"
	reasonProvisionNodeFailed    = "ProvisionNodeFailed"
	reasonInventoryStatusChanged = "InstanceInventoryStatusChanged"
	componentGOATScaler          = "goatscaler"
	kindACKNodePool              = "ACKNodePool"
)

// NewGOATScalerReader creates a new GOATScalerReader.
func NewGOATScalerReader() *GOATScalerReader {
	return &GOATScalerReader{}
}

// ReadStatus reads GOATScaler events related to the given pod and nodepools.
func (g *GOATScalerReader) ReadStatus(
	ctx context.Context,
	cl client.Client,
	pod *corev1.Pod,
	nodepoolIDs []string,
) (map[string]AutoscalerStatus, error) {
	statuses := make(map[string]AutoscalerStatus)

	if err := g.readPodEvents(ctx, cl, pod, statuses); err != nil {
		return nil, err
	}

	if err := g.readNodepoolEvents(ctx, cl, nodepoolIDs, statuses); err != nil {
		return nil, err
	}

	return statuses, nil
}

func (g *GOATScalerReader) readPodEvents(
	ctx context.Context,
	cl client.Client,
	pod *corev1.Pod,
	statuses map[string]AutoscalerStatus,
) error {
	var events corev1.EventList
	if err := cl.List(ctx, &events, client.InNamespace(pod.Namespace)); err != nil {
		return fmt.Errorf("listing pod events: %w", err)
	}

	for i := range events.Items {
		ev := &events.Items[i]
		if !isPodEvent(ev, pod) {
			continue
		}

		processPodEvent(ev, statuses)
	}

	return nil
}

func isPodEvent(ev *corev1.Event, pod *corev1.Pod) bool {
	return ev.InvolvedObject.Kind == "Pod" &&
		ev.InvolvedObject.Name == pod.Name &&
		ev.InvolvedObject.UID == pod.UID &&
		ev.Source.Component == componentGOATScaler
}

func processPodEvent(ev *corev1.Event, statuses map[string]AutoscalerStatus) {
	switch ev.Reason {
	case reasonProvisionNode:
		s := statuses[ev.InvolvedObject.Name]
		s.ScaleUpTriggered = true
		s.Message = ev.Message
		statuses[ev.InvolvedObject.Name] = s
	case reasonNotTriggerScaleUp, reasonProvisionNodeFailed:
		s := statuses[ev.InvolvedObject.Name]
		s.ScaleUpFailed = true
		s.Message = ev.Message
		statuses[ev.InvolvedObject.Name] = s
	}
}

func (g *GOATScalerReader) readNodepoolEvents(
	ctx context.Context,
	cl client.Client,
	nodepoolIDs []string,
	statuses map[string]AutoscalerStatus,
) error {
	idSet := make(map[string]bool, len(nodepoolIDs))
	for _, id := range nodepoolIDs {
		idSet[id] = true
	}

	var events corev1.EventList
	if err := cl.List(ctx, &events); err != nil {
		return fmt.Errorf("listing nodepool events: %w", err)
	}

	for i := range events.Items {
		ev := &events.Items[i]
		if ev.InvolvedObject.Kind != kindACKNodePool {
			continue
		}

		if ev.Reason != reasonInventoryStatusChanged {
			continue
		}

		poolID := ev.InvolvedObject.Name
		if !idSet[poolID] {
			continue
		}

		if strings.Contains(ev.Message, "Healthy to UnHealthy") {
			s := statuses[poolID]
			s.InventoryUnhealthy = true
			s.Message = ev.Message
			statuses[poolID] = s
		}
	}

	return nil
}
