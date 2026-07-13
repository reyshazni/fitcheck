package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/reyshazni/fitcheck/internal/version"
)

type managerOptions struct {
	metricsAddr     string
	healthAddr      string
	nodepoolLabel   string
	recheckInterval time.Duration
	initialDelay    time.Duration
	namespace       string
	autoscalerCM    string
}

func main() {
	opts := parseFlags()

	setupLogger()

	info := version.Info()
	slog.Info("starting fitcheck", "version", info.Version, "commit", info.Commit, "date", info.Date)

	cfg, err := ctrl.GetConfig()
	if err != nil {
		slog.Error("unable to load kubeconfig", "error", err)
		os.Exit(1)
	}

	mgr, err := createManager(cfg, opts)
	if err != nil {
		slog.Error("unable to create manager", "error", err)
		os.Exit(1)
	}

	slog.Info("starting manager")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		slog.Error("manager exited with error", "error", err)
		os.Exit(1)
	}
}

func parseFlags() managerOptions {
	opts := managerOptions{}

	flag.StringVar(&opts.metricsAddr, "metrics-addr", ":8080", "metrics bind address")
	flag.StringVar(&opts.healthAddr, "health-addr", ":8081", "health probe bind address")
	flag.StringVar(&opts.nodepoolLabel, "nodepool-label", "node.kubernetes.io/nodepool", "label key for grouping nodes")
	flag.DurationVar(&opts.recheckInterval, "recheck-interval", 30*time.Second, "re-evaluation interval for pending pods")
	flag.DurationVar(&opts.initialDelay, "initial-delay", 10*time.Second, "delay before first diagnosis")
	flag.StringVar(&opts.namespace, "namespace", "", "restrict to specific namespace")
	flag.StringVar(&opts.autoscalerCM, "autoscaler-configmap", "cluster-autoscaler-status", "ConfigMap name for autoscaler status")
	flag.Parse()

	return opts
}

func setupLogger() {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := logr.FromSlogHandler(handler)
	ctrl.SetLogger(logger)
	slog.SetDefault(slog.New(handler))
}

func createManager(cfg *rest.Config, opts managerOptions) (ctrl.Manager, error) {
	mgrOpts := ctrl.Options{
		Metrics: metricsserver.Options{
			BindAddress: opts.metricsAddr,
		},
		HealthProbeBindAddress: opts.healthAddr,
		Cache:                  buildCacheOptions(opts),
	}

	mgr, err := ctrl.NewManager(cfg, mgrOpts)
	if err != nil {
		return nil, fmt.Errorf("creating manager: %w", err)
	}

	if err := addHealthChecks(mgr); err != nil {
		return nil, fmt.Errorf("adding health checks: %w", err)
	}

	return mgr, nil
}

func buildCacheOptions(opts managerOptions) cache.Options {
	cacheOpts := cache.Options{
		ByObject: map[client.Object]cache.ByObject{
			&corev1.ConfigMap{}: {
				Namespaces: map[string]cache.Config{
					"kube-system": {},
				},
			},
		},
	}

	if opts.namespace != "" {
		cacheOpts.DefaultNamespaces = map[string]cache.Config{
			opts.namespace: {},
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
