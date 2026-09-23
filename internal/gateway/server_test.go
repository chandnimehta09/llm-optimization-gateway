package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestServerIntegration(t *testing.T) {
	port := "18080"
	srv := NewServer(port)

	go func() {
		if err := srv.Start(); err != nil {
			t.Logf("Server stopped: %v", err)
		}
	}()

	// Wait briefly for the server to start.
	time.Sleep(100 * time.Millisecond)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	baseURL := fmt.Sprintf("http://localhost:%s", port)

	t.Run("health endpoint", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("failed to call /health: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		var body HealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if body.Status != "healthy" {
			t.Errorf("expected status 'healthy', got %q", body.Status)
		}
	})

	t.Run("ready endpoint", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/ready")
		if err != nil {
			t.Fatalf("failed to call /ready: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}

		var body ReadyResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !body.Ready {
			t.Errorf("expected ready to be true, got %v", body.Ready)
		}
	})
}

func TestServerGracefulShutdown(t *testing.T) {
	port := "18081"
	srv := NewServer(port)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait briefly for the server to start.
	time.Sleep(100 * time.Millisecond)

	// Verify server is running.
	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/health", port))
	if err != nil {
		t.Fatalf("server not running: %v", err)
	}
	resp.Body.Close()

	// Shutdown gracefully.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	// Verify Start() returned without error (graceful shutdown).
	if err := <-errCh; err != nil {
		t.Errorf("expected nil error from Start after graceful shutdown, got: %v", err)
	}
}
