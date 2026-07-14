package main

import (
	"flag"
	"log/slog"
	"os"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/reyshazni/fitcheck/internal/app"
	fclog "github.com/reyshazni/fitcheck/internal/log"
	"github.com/reyshazni/fitcheck/internal/version"
)

func main() {
	metricsAddr, healthAddr, opts := parseFlags()

	fclog.Setup()

	info := version.Info()
	slog.Info("starting fitcheck",
		"version", info.Version,
		"commit", info.Commit,
		"date", info.Date,
	)

	cfg, err := ctrl.GetConfig()
	if err != nil {
		slog.Error("unable to load kubeconfig", "error", err)
		os.Exit(1)
	}

	if err := app.Run(cfg, metricsAddr, healthAddr, opts); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func parseFlags() (string, string, app.Options) {
	var (
		metricsAddr string
		healthAddr  string
		opts        app.Options
	)

	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "metrics bind address")
	flag.StringVar(&healthAddr, "health-addr", ":8081", "health probe bind address")
	flag.DurationVar(&opts.RecheckInterval, "recheck-interval", 30*time.Second, "re-evaluation interval")
	flag.DurationVar(&opts.InitialDelay, "initial-delay", 10*time.Second, "delay before first diagnosis")
	flag.StringVar(&opts.Namespace, "namespace", "", "restrict to namespace")
	flag.DurationVar(&opts.StartupTimeout, "startup-timeout", 10*time.Minute, "node startup taint timeout")
	flag.Parse()

	return metricsAddr, healthAddr, opts
}
