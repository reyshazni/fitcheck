package provider

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/reyshazni/fitcheck/internal/autoscaler"
)

// Provider abstracts cloud-specific configuration for nodepool discovery
// and autoscaler integration. Adding a new cloud means implementing this
// interface and registering it in the providers slice.
type Provider interface {
	// Name returns the provider identifier (e.g. "ack", "gke", "eks").
	Name() string

	// NodepoolLabelKey returns the node label used to group nodes into nodepools.
	NodepoolLabelKey() string

	// NameLabelKey returns the node label used for human-readable nodepool names.
	NameLabelKey() string

	// Detect checks whether this provider's nodes exist in the cluster.
	Detect(ctx context.Context, cl client.Client) (bool, error)

	// NewStatusReader creates an autoscaler status reader, or nil if no
	// supported autoscaler is detected on the cluster.
	NewStatusReader(ctx context.Context, cl client.Client) (autoscaler.StatusReader, error)
}

// registry is the list of known providers set via Register.
var registry []Provider

// Register adds a provider to the global registry.
// Call this from an init() function in each provider package.
func Register(p Provider) {
	registry = append(registry, p)
}

// DetectProvider checks the cluster and returns the first provider whose
// nodes are found. Returns an error if no provider is detected.
func DetectProvider(ctx context.Context, cl client.Client) (Provider, error) {
	for _, p := range registry {
		found, err := p.Detect(ctx, cl)
		if err != nil {
			return nil, fmt.Errorf("detecting provider %s: %w", p.Name(), err)
		}

		if found {
			slog.Info("provider detected", "provider", p.Name())

			return p, nil
		}
	}

	return nil, fmt.Errorf("no supported provider detected: check that nodes have a recognized nodepool label")
}
