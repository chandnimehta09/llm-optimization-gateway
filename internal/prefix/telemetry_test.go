package prefix

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func newTestTelemetry(t *testing.T) (*Telemetry, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tel, err := NewTelemetry(provider)
	if err != nil {
		t.Fatalf("NewTelemetry() error = %v", err)
	}
	return tel, reader
}

func collectTestMetrics(t *testing.T, reader *sdkmetric.ManualReader) *metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	return &rm
}

func findTestMetric(rm *metricdata.ResourceMetrics, name string) *metricdata.Metrics {
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

func TestNewTelemetry_NilProvider(t *testing.T) {
	// Should use global provider, no error.
	tel, err := NewTelemetry(nil)
	if err != nil {
		t.Fatalf("NewTelemetry(nil) error = %v", err)
	}
	if tel == nil {
		t.Fatal("NewTelemetry(nil) returned nil")
	}
}

func TestNewTelemetry_ValidProvider(t *testing.T) {
	tel, _ := newTestTelemetry(t)
	if tel == nil {
		t.Fatal("NewTelemetry() returned nil")
	}
}

func TestTelemetry_RecordRequest_Success(t *testing.T) {
	tel, reader := newTestTelemetry(t)

	tel.RecordRequest(context.Background(), &RequestResult{
		Model:             "llama-3",
		PrefixTokens:      50,
		CachedTokens:      40,
		TotalPromptTokens: 100,
		Duration:          200 * time.Millisecond,
		CacheHit:          true,
	})

	rm := collectTestMetrics(t, reader)

	// Check requests total.
	m := findTestMetric(rm, "prefix_cache.requests_total")
	if m == nil {
		t.Fatal("prefix_cache.requests_total metric not found")
	}

	// Check prefix tokens.
	m = findTestMetric(rm, "prefix_cache.prefix_tokens")
	if m == nil {
		t.Fatal("prefix_cache.prefix_tokens metric not found")
	}

	// Check cached tokens.
	m = findTestMetric(rm, "prefix_cache.cached_tokens")
	if m == nil {
		t.Fatal("prefix_cache.cached_tokens metric not found")
	}

	// Check cache hits.
	m = findTestMetric(rm, "prefix_cache.hits")
	if m == nil {
		t.Fatal("prefix_cache.hits metric not found")
	}

	// Check hit rate.
	m = findTestMetric(rm, "prefix_cache.hit_rate")
	if m == nil {
		t.Fatal("prefix_cache.hit_rate metric not found")
	}

	// Check latency savings.
	m = findTestMetric(rm, "prefix_cache.latency_savings")
	if m == nil {
		t.Fatal("prefix_cache.latency_savings metric not found")
	}
}

func TestTelemetry_RecordRequest_CacheMiss(t *testing.T) {
	tel, reader := newTestTelemetry(t)

	tel.RecordRequest(context.Background(), &RequestResult{
		Model:             "llama-3",
		PrefixTokens:      50,
		CachedTokens:      0,
		TotalPromptTokens: 100,
		Duration:          300 * time.Millisecond,
		CacheHit:          false,
	})

	rm := collectTestMetrics(t, reader)

	m := findTestMetric(rm, "prefix_cache.misses")
	if m == nil {
		t.Fatal("prefix_cache.misses metric not found")
	}
}

func TestTelemetry_RecordRequest_Error(t *testing.T) {
	tel, reader := newTestTelemetry(t)

	tel.RecordRequest(context.Background(), &RequestResult{
		Model:    "llama-3",
		Duration: 100 * time.Millisecond,
		Error:    &APIError{StatusCode: 500, Body: "error"},
	})

	rm := collectTestMetrics(t, reader)

	m := findTestMetric(rm, "prefix_cache.requests_total")
	if m == nil {
		t.Fatal("prefix_cache.requests_total metric not found")
	}

	// Prefix tokens should NOT be recorded for errors.
	snap := tel.Snapshot()
	if snap.TotalPrefixTokens != 0 {
		t.Errorf("TotalPrefixTokens = %d, want 0 after error", snap.TotalPrefixTokens)
	}
}

func TestTelemetry_RecordRequest_NilResult(t *testing.T) {
	tel, _ := newTestTelemetry(t)
	// Should not panic.
	tel.RecordRequest(context.Background(), nil)
}

func TestTelemetry_RecordPrefixLength(t *testing.T) {
	tel, reader := newTestTelemetry(t)

	tel.RecordPrefixLength(context.Background(), "llama-3", 42.0)

	rm := collectTestMetrics(t, reader)
	m := findTestMetric(rm, "prefix_cache.prefix_length")
	if m == nil {
		t.Fatal("prefix_cache.prefix_length metric not found")
	}
}

func TestTelemetry_Snapshot(t *testing.T) {
	tel, _ := newTestTelemetry(t)

	// Record several requests.
	ctx := context.Background()
	tel.RecordRequest(ctx, &RequestResult{
		Model:             "llama-3",
		PrefixTokens:      50,
		CachedTokens:      40,
		TotalPromptTokens: 100,
		Duration:          200 * time.Millisecond,
		CacheHit:          true,
	})
	tel.RecordRequest(ctx, &RequestResult{
		Model:             "llama-3",
		PrefixTokens:      50,
		CachedTokens:      0,
		TotalPromptTokens: 100,
		Duration:          300 * time.Millisecond,
		CacheHit:          false,
	})
	tel.RecordRequest(ctx, &RequestResult{
		Model:             "llama-3",
		PrefixTokens:      50,
		CachedTokens:      30,
		TotalPromptTokens: 100,
		Duration:          250 * time.Millisecond,
		CacheHit:          true,
	})

	snap := tel.Snapshot()

	if snap.TotalRequests != 3 {
		t.Errorf("TotalRequests = %d, want 3", snap.TotalRequests)
	}
	if snap.CacheHitRequests != 2 {
		t.Errorf("CacheHitRequests = %d, want 2", snap.CacheHitRequests)
	}
	if snap.TotalPrefixTokens != 150 {
		t.Errorf("TotalPrefixTokens = %d, want 150", snap.TotalPrefixTokens)
	}
	if snap.TotalCachedTokens != 70 {
		t.Errorf("TotalCachedTokens = %d, want 70", snap.TotalCachedTokens)
	}
}

func TestTelemetrySnapshot_HitRate(t *testing.T) {
	tests := []struct {
		name    string
		snap    TelemetrySnapshot
		want    float64
	}{
		{"no requests", TelemetrySnapshot{}, 0},
		{"all hits", TelemetrySnapshot{TotalRequests: 10, CacheHitRequests: 10}, 1.0},
		{"half hits", TelemetrySnapshot{TotalRequests: 10, CacheHitRequests: 5}, 0.5},
		{"no hits", TelemetrySnapshot{TotalRequests: 10, CacheHitRequests: 0}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.snap.HitRate()
			if got != tt.want {
				t.Errorf("HitRate() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestTelemetrySnapshot_TokenCacheRate(t *testing.T) {
	tests := []struct {
		name string
		snap TelemetrySnapshot
		want float64
	}{
		{"no tokens", TelemetrySnapshot{}, 0},
		{"all cached", TelemetrySnapshot{TotalPrefixTokens: 100, TotalCachedTokens: 100}, 1.0},
		{"half cached", TelemetrySnapshot{TotalPrefixTokens: 100, TotalCachedTokens: 50}, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.snap.TokenCacheRate()
			if got != tt.want {
				t.Errorf("TokenCacheRate() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestRequestResult_HitRate(t *testing.T) {
	tests := []struct {
		name   string
		result RequestResult
		want   float64
	}{
		{"zero total", RequestResult{TotalPromptTokens: 0}, 0},
		{"full hit", RequestResult{CachedTokens: 100, TotalPromptTokens: 100}, 1.0},
		{"partial hit", RequestResult{CachedTokens: 50, TotalPromptTokens: 100}, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.HitRate()
			if got != tt.want {
				t.Errorf("HitRate() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestRequestResult_EstimatedLatencySavingsMs(t *testing.T) {
	tests := []struct {
		name   string
		result RequestResult
		wantGT float64
	}{
		{"no tokens", RequestResult{Duration: 100 * time.Millisecond}, 0},
		{"no cached tokens", RequestResult{
			TotalPromptTokens: 100,
			CachedTokens:      0,
			Duration:          100 * time.Millisecond,
		}, 0},
		{"has savings", RequestResult{
			TotalPromptTokens: 100,
			CachedTokens:      50,
			Duration:          100 * time.Millisecond,
		}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.EstimatedLatencySavingsMs()
			if got < tt.wantGT {
				t.Errorf("EstimatedLatencySavingsMs() = %f, want > %f", got, tt.wantGT)
			}
		})
	}

	// Verify positive savings when cached.
	r := RequestResult{
		TotalPromptTokens: 100,
		CachedTokens:      80,
		Duration:          200 * time.Millisecond,
	}
	got := r.EstimatedLatencySavingsMs()
	if got <= 0 {
		t.Errorf("Expected positive latency savings for cached request, got %f", got)
	}
}
