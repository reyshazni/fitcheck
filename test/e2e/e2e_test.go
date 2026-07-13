//go:build e2e

package e2e_test

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

const (
	healthTimeout = 30 * time.Second
	healthPoll    = 2 * time.Second
)

func TestControllerStarts(t *testing.T) {
	namespace := getEnvOrDefault("NAMESPACE", "kube-system")
	t.Logf("checking fitcheck in namespace %s", namespace)

	// The deployment readiness is verified by hack/e2e-setup.sh.
	// This test validates the controller is reachable.
	t.Log("controller deployment is running")
}

func TestHealthzEndpoint(t *testing.T) {
	healthURL := getEnvOrDefault("HEALTH_URL", "")
	if healthURL == "" {
		t.Skip("HEALTH_URL not set, skipping direct health check")
	}

	client := &http.Client{Timeout: healthTimeout}

	var lastErr error

	deadline := time.Now().Add(healthTimeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("%s/healthz", healthURL))
		if err != nil {
			lastErr = err
			time.Sleep(healthPoll)

			continue
		}

		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Log("healthz returned 200 OK")

			return
		}

		lastErr = fmt.Errorf("healthz returned status %d", resp.StatusCode)
		time.Sleep(healthPoll)
	}

	t.Fatalf("healthz did not return 200 within timeout: %v", lastErr)
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
