package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestIntegration_MetricsEndpoint(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ServiceName: "test-gateway",
		LogLevel:    "info",
	}

	tel, shutdown, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			t.Errorf("shutdown error = %v", err)
		}
	}()

	// Increment some metrics so they appear in the output.
	tel.Metrics.RequestsTotal.Inc()
	tel.Metrics.CacheHitsTotal.Inc()
	tel.Metrics.CacheMissesTotal.Inc()
	tel.Metrics.ActiveRequests.Set(3)
	tel.Metrics.ProviderCallsTotal.WithLabelValues("openai").Inc()
	tel.Metrics.ProviderErrorsTotal.WithLabelValues("openai", "timeout").Inc()
	tel.Metrics.RequestDuration.Observe(0.123)

	// Serve the metrics handler.
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	tel.MetricsHandler.ServeHTTP(rec, req)

	resp := rec.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	bodyStr := string(body)

	expectedMetrics := []string{
		"gateway_requests_total",
		"gateway_request_duration_seconds",
		"gateway_active_requests",
		"gateway_provider_calls_total",
		"gateway_provider_errors_total",
		"gateway_cache_hits_total",
		"gateway_cache_misses_total",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(bodyStr, metric) {
			t.Errorf("metrics output missing %q", metric)
		}
	}

	// Verify specific values.
	if !strings.Contains(bodyStr, "gateway_requests_total 1") {
		t.Error("expected gateway_requests_total to be 1")
	}
	if !strings.Contains(bodyStr, "gateway_active_requests 3") {
		t.Error("expected gateway_active_requests to be 3")
	}
	if !strings.Contains(bodyStr, `gateway_provider_calls_total{provider="openai"} 1`) {
		t.Error("expected gateway_provider_calls_total{provider=openai} to be 1")
	}
	if !strings.Contains(bodyStr, `gateway_provider_errors_total{error_type="timeout",provider="openai"} 1`) {
		t.Error("expected gateway_provider_errors_total{error_type=timeout,provider=openai} to be 1")
	}
}

func TestIntegration_MetricsEndpoint_ContentType(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ServiceName: "test-gateway",
		LogLevel:    "info",
	}

	tel, shutdown, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	tel.Metrics.RequestsTotal.Inc()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	tel.MetricsHandler.ServeHTTP(rec, req)

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}
}

func TestIntegration_TracingNoopWithoutEndpoint(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ServiceName: "test-gateway",
		LogLevel:    "info",
	}

	_, shutdown, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	// The global tracer provider should be set.
	tracer := otel.Tracer("test")
	_, span := tracer.Start(ctx, "test-span")
	// Noop spans are not recording.
	if span.IsRecording() {
		t.Error("expected noop span to not be recording")
	}
	span.End()
}

func TestIntegration_TracingWithEndpoint(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ServiceName:    "test-gateway",
		ServiceVersion: "1.0.0-test",
		OTLPEndpoint:   "localhost:4317",
		LogLevel:       "info",
	}

	// InitTracing should succeed even if the collector is not running.
	// The OTLP exporter uses async batching and won't fail on init.
	shutdownFn, err := InitTracing(ctx, cfg)
	if err != nil {
		t.Fatalf("InitTracing() error = %v", err)
	}

	tracer := otel.Tracer("test")
	_, span := tracer.Start(ctx, "test-span")

	// With a real tracer provider, spans should be recording.
	if !span.IsRecording() {
		t.Error("expected span to be recording with OTLP endpoint configured")
	}
	span.End()

	// Shutdown should succeed (even without a running collector).
	if err := shutdownFn(ctx); err != nil {
		// Connection error expected without a running collector.
		t.Logf("shutdown returned error (expected without collector): %v", err)
	}
}

func TestIntegration_TelemetryInit_FullStack(t *testing.T) {
	ctx := context.Background()
	cfg := Config{
		ServiceName: "integration-test",
		LogLevel:    "debug",
	}

	tel, shutdown, err := Init(ctx, cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() {
		_ = shutdown(ctx)
	}()

	if tel.Logger == nil {
		t.Fatal("expected Logger to be non-nil")
	}
	if tel.Metrics == nil {
		t.Fatal("expected Metrics to be non-nil")
	}
	if tel.MetricsHandler == nil {
		t.Fatal("expected MetricsHandler to be non-nil")
	}

	// Logger with request ID should work.
	ctx = WithRequestID(ctx, "test-req-001")
	logger := LoggerFromContext(ctx, tel.Logger)
	logger.Info("integration test log")
}
