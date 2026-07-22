package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/reyshazni/fitcheck/internal/autoscaler"
	"github.com/reyshazni/fitcheck/internal/controller"
	fitmetrics "github.com/reyshazni/fitcheck/internal/metrics"
	"github.com/reyshazni/fitcheck/internal/provider"
	_ "github.com/reyshazni/fitcheck/internal/provider/ack" // register ACK provider
)

// Options holds business configuration for the application.
// Infrastructure addresses (bind ports) are passed separately by the caller.
type Options struct {
	RecheckInterval time.Duration
	InitialDelay    time.Duration
	Namespace       string
	StartupTimeout  time.Duration
}

type unauthorizedRoundTripper struct {
	rt http.RoundTripper
}

// Run creates a controller-runtime manager and starts it.
func Run(cfg *rest.Config, metricsAddr, healthAddr string, opts Options) error {
	cfg.Wrap(exitOnUnauthorized)

	mgr, err := CreateManager(cfg, metricsAddr, healthAddr, opts)
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	ctx := ctrl.SetupSignalHandler()

	// Use a direct (non-cached) client for pre-start operations and metrics.
	// mgr.GetClient() is cached and dynamically starts informers on first access,
	// which blocks the metrics endpoint on large clusters.
	directClient, err := client.New(cfg, client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return fmt.Errorf("creating direct client: %w", err)
	}

	crmetrics.Registry.MustRegister(fitmetrics.NewPendingPodCollector(mgr.GetClient(), directClient))

	prov, err := provider.DetectProvider(ctx, directClient)
	if err != nil {
		return fmt.Errorf("detecting provider: %w", err)
	}

	reader, err := prov.NewStatusReader(ctx, directClient)
	if err != nil {
		return fmt.Errorf("detecting autoscaler: %w", err)
	}

	if err := setupReconciler(mgr, opts, prov, reader); err != nil {
		return err
	}

	slog.Info("starting manager", "provider", prov.Name())

	if err := mgr.Start(ctx); err != nil {
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

func (t *unauthorizedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.rt.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("round trip: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		slog.Error("API server returned 401, exiting to trigger pod restart")
		os.Exit(1)
	}

	return resp, nil
}

func exitOnUnauthorized(rt http.RoundTripper) http.RoundTripper {
	return &unauthorizedRoundTripper{rt: rt}
}

func setupReconciler(
	mgr ctrl.Manager,
	opts Options,
	prov provider.Provider,
	reader autoscaler.StatusReader,
) error {
	reconciler := &controller.PodReconciler{
		Client:          mgr.GetClient(),
		Recorder:        mgr.GetEventRecorder("fitcheck"),
		Provider:        prov,
		RecheckInterval: opts.RecheckInterval,
		InitialDelay:    opts.InitialDelay,
		StatusReader:    reader,
		StartupTimeout:  opts.StartupTimeout,
	}

	if err := controller.SetupWithManager(mgr, reconciler); err != nil {
		return fmt.Errorf("setting up reconciler: %w", err)
	}

	return nil
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
