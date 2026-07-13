package app

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// Options holds business configuration for the application.
// Infrastructure addresses (bind ports) are passed separately by the caller.
type Options struct {
	RecheckInterval time.Duration
	InitialDelay    time.Duration
	Namespace       string
}

// Run creates a controller-runtime manager and starts it.
func Run(cfg *rest.Config, metricsAddr, healthAddr string, opts Options) error {
	mgr, err := CreateManager(cfg, metricsAddr, healthAddr, opts)
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	// controller wiring will be added in Task 13

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("running manager: %w", err)
	}

	return nil
}

// CreateManager builds a controller-runtime manager with metrics, health probes, and cache options.
func CreateManager(cfg *rest.Config, metricsAddr, healthAddr string, opts Options) (ctrl.Manager, error) {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: healthAddr,
		Cache:                  buildCacheOptions(opts),
	})
	if err != nil {
		return nil, fmt.Errorf("initializing manager: %w", err)
	}

	if err := addHealthChecks(mgr); err != nil {
		return nil, err
	}

	return mgr, nil
}

func buildCacheOptions(opts Options) cache.Options {
	cacheOpts := cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.ConfigMap{}: {
				Namespaces: map[string]cache.Config{
					"kube-system": {},
				},
			},
		},
	}

	if opts.Namespace != "" {
		cacheOpts.DefaultNamespaces = map[string]cache.Config{
			opts.Namespace: {},
		}
	}

	return cacheOpts
}

func addHealthChecks(mgr ctrl.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up healthz: %w", err)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up readyz: %w", err)
	}

	return nil
}
