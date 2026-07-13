package ack

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/reyshazni/fitcheck/internal/autoscaler"
)

// ACKProvider implements the Provider interface for Alibaba Cloud
// Container Service for Kubernetes (ACK).
type ACKProvider struct{}

const (
	nodepoolLabel = "alibabacloud.com/nodepool-id"
	nameLabel     = "name"
)

// New creates a new ACKProvider.
func New() *ACKProvider { return &ACKProvider{} }

// Name returns the provider identifier.
func (p *ACKProvider) Name() string { return "ack" }

// NodepoolLabelKey returns the ACK nodepool label key.
func (p *ACKProvider) NodepoolLabelKey() string { return nodepoolLabel }

// NameLabelKey returns the ACK name label key.
func (p *ACKProvider) NameLabelKey() string { return nameLabel }

// Detect checks whether ACK nodes exist in the cluster by looking for
// the nodepool label.
func (p *ACKProvider) Detect(ctx context.Context, cl client.Client) (bool, error) {
	var nodes corev1.NodeList
	if err := cl.List(ctx, &nodes, client.Limit(1), client.HasLabels{nodepoolLabel}); err != nil {
		return false, fmt.Errorf("listing nodes: %w", err)
	}

	return len(nodes.Items) > 0, nil
}

// NewStatusReader creates an autoscaler status reader by detecting
// the autoscaler type from cluster ConfigMaps.
func (p *ACKProvider) NewStatusReader(ctx context.Context, cl client.Client) (autoscaler.StatusReader, error) {
	reader, err := autoscaler.Detect(ctx, cl)
	if err != nil {
		return nil, fmt.Errorf("detecting autoscaler: %w", err)
	}

	return reader, nil
}
