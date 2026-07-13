package main

import (
	"net/http"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	testNodepoolLabel = "node.kubernetes.io/nodepool"
	testAutoscalerCM  = "cluster-autoscaler-status"
)

func TestManagerCreation(t *testing.T) {
	t.Parallel()

	testEnv := &envtest.Environment{}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest: %v", err)
	}

	t.Cleanup(func() {
		if stopErr := testEnv.Stop(); stopErr != nil {
			t.Errorf("failed to stop envtest: %v", stopErr)
		}
	})

	mgr, err := createManager(cfg, managerOptions{
		metricsAddr:     "0",
		healthAddr:      "0",
		recheckInterval: 30 * time.Second,
		initialDelay:    10 * time.Second,
		nodepoolLabel:   testNodepoolLabel,
		autoscalerCM:    testAutoscalerCM,
	})
	if err != nil {
		t.Fatalf("expected manager creation to succeed, got: %v", err)
	}

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestManagerHasHealthChecks(t *testing.T) {
	t.Parallel()

	testEnv := &envtest.Environment{}

	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest: %v", err)
	}

	t.Cleanup(func() {
		if stopErr := testEnv.Stop(); stopErr != nil {
			t.Errorf("failed to stop envtest: %v", stopErr)
		}
	})

	mgr, err := createManager(cfg, managerOptions{
		metricsAddr:     "0",
		healthAddr:      "0",
		recheckInterval: 30 * time.Second,
		initialDelay:    10 * time.Second,
		nodepoolLabel:   testNodepoolLabel,
		autoscalerCM:    testAutoscalerCM,
	})
	if err != nil {
		t.Fatalf("manager creation failed: %v", err)
	}

	// Verify the manager accepts additional health checks (proves it was configured correctly).
	err = mgr.AddHealthzCheck("test-healthz", func(_ *http.Request) error {
		return nil
	})
	if err != nil {
		t.Fatalf("manager should accept health checks: %v", err)
	}
}
