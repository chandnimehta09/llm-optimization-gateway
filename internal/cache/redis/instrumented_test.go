package redis

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/alicebob/miniredis/v2"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/llm-optimization-gateway/internal/cache"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestMetrics creates a Metrics instance with an in-memory metric reader.
func newTestMetrics(t *testing.T) (*Metrics, *sdkmetric.ManualReader) {
	t.Helper()
	ResetGlobalMetrics()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	m := NewMetrics(mp)
	return m, reader
}

// newInstrumentedTestCache creates an InstrumentedCache backed by miniredis.
func newInstrumentedTestCache(t *testing.T) (*InstrumentedCache, *miniredis.Miniredis, *sdkmetric.ManualReader) {
	t.Helper()
	ResetGlobalMetrics()

	mr := miniredis.RunT(t)

	cfg := Config{
		Mode:                 ModeStandalone,
		Addr:                 mr.Addr(),
		PoolSize:             5,
		MinIdleConns:         1,
		DialTimeout:          1 * time.Second,
		ReadTimeout:          1 * time.Second,
		WriteTimeout:         1 * time.Second,
		MaxRetries:           1,
		MinRetryBackoff:      8 * time.Millisecond,
		MaxRetryBackoff:      64 * time.Millisecond,
		DefaultTTL:           0,
		KeyPrefix:            "test:",
		EnableCompression:    false,
		CompressionThreshold: 1024,
		HealthCheckInterval:  0,
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:         mr.Addr(),
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		MaxRetries:   cfg.MaxRetries,
	})

	inner := NewWithClient(client, cfg)

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	m := NewMetrics(mp)

	ic := NewInstrumentedWithMetrics(inner, m)
	return ic, mr, reader
}

// collectTestMetrics reads all metrics from the reader.
func collectTestMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	err := reader.Collect(context.Background(), &rm)
	if err != nil {
		t.Fatalf("failed to collect metrics: %v", err)
	}
	return rm
}

// findMetricByName searches for a metric by name in the collected data.
func findMetricByName(rm metricdata.ResourceMetrics, name string) *metricdata.Metrics {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return &m
			}
		}
	}
	return nil
}

// sumInt64Counter returns the total value of an Int64Counter metric.
func sumInt64Counter(m *metricdata.Metrics) int64 {
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		return 0
	}
	var total int64
	for _, dp := range sum.DataPoints {
		total += dp.Value
	}
	return total
}

// countHistogramDataPoints returns the total count across all histogram data points.
func countHistogramDataPoints(m *metricdata.Metrics) uint64 {
	if m == nil {
		return 0
	}
	hist, ok := m.Data.(metricdata.Histogram[float64])
	if !ok {
		return 0
	}
	var total uint64
	for _, dp := range hist.DataPoints {
		total += dp.Count
	}
	return total
}

// newMetricsTestEntry creates a cache entry for testing.
func newMetricsTestEntry(content string, ttl time.Duration) *cache.Entry {
	return &cache.Entry{
		Response: cache.CompletionResponse{
			ID:           "resp-001",
			Model:        "gpt-4",
			Content:      content,
			FinishReason: "stop",
			Usage: cache.UsageInfo{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       ttl,
			HitCount:  0,
			Tokens:    30,
			Cost:      0.001,
		},
		ContextHash: "hash123",
		Provider:    "openai",
		Model:       "gpt-4",
	}
}

// ---------------------------------------------------------------------------
// Metrics unit tests
// ---------------------------------------------------------------------------

func TestMetrics_NewMetrics_NilProvider(t *testing.T) {
	ResetGlobalMetrics()
	m := NewMetrics(nil)
	if m == nil {
		t.Fatal("NewMetrics(nil) returned nil")
	}
	if m.operationsTotal == nil {
		t.Error("operationsTotal is nil")
	}
	if m.operationDuration == nil {
		t.Error("operationDuration is nil")
	}
	if m.connectionsActive == nil {
		t.Error("connectionsActive is nil")
	}
	if m.connectionErrorsTotal == nil {
		t.Error("connectionErrorsTotal is nil")
	}
	if m.commandDuration == nil {
		t.Error("commandDuration is nil")
	}
	if m.entrySizeBytes == nil {
		t.Error("entrySizeBytes is nil")
	}
	if m.hitRate == nil {
		t.Error("hitRate is nil")
	}
}

func TestMetrics_HitRate_NoOperations(t *testing.T) {
	m, _ := newTestMetrics(t)
	if got := m.HitRate(); got != 0 {
		t.Errorf("HitRate() = %f, want 0", got)
	}
}

func TestMetrics_HitRate_AllHits(t *testing.T) {
	m, _ := newTestMetrics(t)
	for i := 0; i < 10; i++ {
		m.recordHit()
	}
	if got := m.HitRate(); got != 1.0 {
		t.Errorf("HitRate() = %f, want 1.0", got)
	}
}

func TestMetrics_HitRate_AllMisses(t *testing.T) {
	m, _ := newTestMetrics(t)
	for i := 0; i < 10; i++ {
		m.recordMiss()
	}
	if got := m.HitRate(); got != 0 {
		t.Errorf("HitRate() = %f, want 0", got)
	}
}

func TestMetrics_HitRate_Mixed(t *testing.T) {
	m, _ := newTestMetrics(t)
	for i := 0; i < 5; i++ {
		m.recordHit()
	}
	for i := 0; i < 5; i++ {
		m.recordMiss()
	}
	if got := m.HitRate(); got != 0.5 {
		t.Errorf("HitRate() = %f, want 0.5", got)
	}
}

func TestMetrics_Hits_And_Total(t *testing.T) {
	m, _ := newTestMetrics(t)
	m.recordHit()
	m.recordHit()
	m.recordMiss()

	if got := m.Hits(); got != 2 {
		t.Errorf("Hits() = %d, want 2", got)
	}
	if got := m.Total(); got != 3 {
		t.Errorf("Total() = %d, want 3", got)
	}
}

func TestMetrics_GetOrCreateMetrics_Singleton(t *testing.T) {
	ResetGlobalMetrics()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	m1 := GetOrCreateMetrics(mp)
	m2 := GetOrCreateMetrics(mp)

	if m1 != m2 {
		t.Error("GetOrCreateMetrics should return the same instance")
	}

	ResetGlobalMetrics()
}

func TestMetrics_ResetGlobalMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	m1 := GetOrCreateMetrics(mp)
	ResetGlobalMetrics()
	m2 := GetOrCreateMetrics(mp)

	if m1 == m2 {
		t.Error("After ResetGlobalMetrics, GetOrCreateMetrics should return a new instance")
	}
}

// ---------------------------------------------------------------------------
// Instrumented cache tests
// ---------------------------------------------------------------------------

func TestInstrumented_Get_Hit(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	entry := newMetricsTestEntry("hello", 60*time.Second)
	if err := ic.Set(ctx, "key1", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, err := ic.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if got.Response.Content != "hello" {
		t.Errorf("content = %q, want %q", got.Response.Content, "hello")
	}

	// Verify metrics.
	rm := collectTestMetrics(t, reader)
	opTotal := findMetricByName(rm, "cache_redis_operations_total")
	if opTotal == nil {
		t.Fatal("cache_redis_operations_total metric not found")
	}
	totalOps := sumInt64Counter(opTotal)
	if totalOps < 2 { // 1 set + 1 get minimum
		t.Errorf("operations total = %d, want >= 2", totalOps)
	}

	// Verify hit rate.
	if got := ic.Metrics().HitRate(); got != 1.0 {
		t.Errorf("HitRate() = %f, want 1.0", got)
	}
}

func TestInstrumented_Get_Miss(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	_, found, err := ic.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if found {
		t.Fatal("expected found=false")
	}

	// Verify metrics.
	rm := collectTestMetrics(t, reader)
	opTotal := findMetricByName(rm, "cache_redis_operations_total")
	if opTotal == nil {
		t.Fatal("cache_redis_operations_total metric not found")
	}
	if got := sumInt64Counter(opTotal); got != 1 {
		t.Errorf("operations total = %d, want 1", got)
	}

	// Verify miss recorded.
	if got := ic.Metrics().HitRate(); got != 0 {
		t.Errorf("HitRate() = %f, want 0", got)
	}
}

func TestInstrumented_AcceptanceCriteria_HitMissRatio(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	// Set up 5 entries that will be hits.
	for i := 0; i < 5; i++ {
		key := "hit-key-" + string(rune('a'+i))
		entry := newMetricsTestEntry("content", 60*time.Second)
		if err := ic.Set(ctx, key, entry); err != nil {
			t.Fatalf("Set() error: %v", err)
		}
	}

	// Perform 5 Gets that hit.
	for i := 0; i < 5; i++ {
		key := "hit-key-" + string(rune('a'+i))
		_, found, err := ic.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get(%q) error: %v", key, err)
		}
		if !found {
			t.Fatalf("Get(%q) expected hit, got miss", key)
		}
	}

	// Perform 5 Gets that miss.
	for i := 0; i < 5; i++ {
		key := "miss-key-" + string(rune('a'+i))
		_, found, err := ic.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get(%q) error: %v", key, err)
		}
		if found {
			t.Fatalf("Get(%q) expected miss, got hit", key)
		}
	}

	// Verify total operations: 5 set + 10 get = 15.
	rm := collectTestMetrics(t, reader)
	opTotal := findMetricByName(rm, "cache_redis_operations_total")
	if opTotal == nil {
		t.Fatal("cache_redis_operations_total metric not found")
	}
	totalOps := sumInt64Counter(opTotal)
	if totalOps != 15 {
		t.Errorf("operations total = %d, want 15 (5 set + 10 get)", totalOps)
	}

	// Verify hit rate = 50%.
	hitRate := ic.Metrics().HitRate()
	if hitRate != 0.5 {
		t.Errorf("HitRate() = %f, want 0.5", hitRate)
	}

	// Verify hit/miss counters.
	if got := ic.Metrics().Hits(); got != 5 {
		t.Errorf("Hits() = %d, want 5", got)
	}
	if got := ic.Metrics().Total(); got != 10 {
		t.Errorf("Total() = %d, want 10", got)
	}
}

func TestInstrumented_AcceptanceCriteria_OperationDuration(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	entry := newMetricsTestEntry("hello", 60*time.Second)
	if err := ic.Set(ctx, "key1", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	_, _, err := ic.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}

	rm := collectTestMetrics(t, reader)
	durMetric := findMetricByName(rm, "cache_redis_operation_duration_seconds")
	if durMetric == nil {
		t.Fatal("cache_redis_operation_duration_seconds metric not found")
	}

	count := countHistogramDataPoints(durMetric)
	if count < 2 { // 1 set + 1 get
		t.Errorf("operation duration data points = %d, want >= 2", count)
	}
}

func TestInstrumented_Set_RecordsMetrics(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	entry := newMetricsTestEntry("test content", 60*time.Second)
	if err := ic.Set(ctx, "key1", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	rm := collectTestMetrics(t, reader)

	// Verify operations_total.
	opTotal := findMetricByName(rm, "cache_redis_operations_total")
	if opTotal == nil {
		t.Fatal("cache_redis_operations_total metric not found")
	}
	if got := sumInt64Counter(opTotal); got != 1 {
		t.Errorf("operations total = %d, want 1", got)
	}

	// Verify entry size recorded.
	sizeMetric := findMetricByName(rm, "cache_redis_entries_size_bytes")
	if sizeMetric == nil {
		t.Fatal("cache_redis_entries_size_bytes metric not found")
	}
	sizeCount := countHistogramDataPoints(sizeMetric)
	if sizeCount != 1 {
		t.Errorf("entry size data points = %d, want 1", sizeCount)
	}
}

func TestInstrumented_Delete_RecordsMetrics(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	// Set then delete.
	entry := newMetricsTestEntry("to delete", 60*time.Second)
	if err := ic.Set(ctx, "key1", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}
	if err := ic.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	rm := collectTestMetrics(t, reader)
	opTotal := findMetricByName(rm, "cache_redis_operations_total")
	if opTotal == nil {
		t.Fatal("cache_redis_operations_total metric not found")
	}
	totalOps := sumInt64Counter(opTotal)
	if totalOps != 2 { // 1 set + 1 delete
		t.Errorf("operations total = %d, want 2", totalOps)
	}
}

func TestInstrumented_Exists_RecordsMetrics(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	entry := newMetricsTestEntry("exists test", 60*time.Second)
	if err := ic.Set(ctx, "key1", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}
	exists, err := ic.Exists(ctx, "key1")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !exists {
		t.Fatal("expected exists=true")
	}

	rm := collectTestMetrics(t, reader)
	opTotal := findMetricByName(rm, "cache_redis_operations_total")
	if opTotal == nil {
		t.Fatal("cache_redis_operations_total metric not found")
	}
	totalOps := sumInt64Counter(opTotal)
	if totalOps != 2 { // 1 set + 1 exists
		t.Errorf("operations total = %d, want 2", totalOps)
	}
}

func TestInstrumented_HealthCheck_RecordsMetrics(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	report := ic.CheckHealth(ctx)
	if report.Status != HealthStatusUp {
		t.Errorf("expected status up, got %s", report.Status)
	}

	rm := collectTestMetrics(t, reader)
	cmdDur := findMetricByName(rm, "cache_redis_command_duration_seconds")
	if cmdDur == nil {
		t.Fatal("cache_redis_command_duration_seconds metric not found")
	}
	if count := countHistogramDataPoints(cmdDur); count < 1 {
		t.Errorf("command duration data points = %d, want >= 1", count)
	}
}

func TestInstrumented_HealthCheck_Down_RecordsConnectionError(t *testing.T) {
	ic, mr, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	// Close miniredis to simulate connection failure.
	mr.Close()

	report := ic.CheckHealth(ctx)
	if report.Status != HealthStatusDown {
		t.Errorf("expected status down, got %s", report.Status)
	}

	rm := collectTestMetrics(t, reader)
	connErrors := findMetricByName(rm, "cache_redis_connection_errors_total")
	if connErrors == nil {
		t.Fatal("cache_redis_connection_errors_total metric not found")
	}
	if got := sumInt64Counter(connErrors); got < 1 {
		t.Errorf("connection errors = %d, want >= 1", got)
	}
}

func TestInstrumented_CommandDuration_Recorded(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	entry := newMetricsTestEntry("cmd test", 60*time.Second)
	_ = ic.Set(ctx, "key1", entry)
	_, _, _ = ic.Get(ctx, "key1")
	_ = ic.Delete(ctx, "key1")

	rm := collectTestMetrics(t, reader)
	cmdDur := findMetricByName(rm, "cache_redis_command_duration_seconds")
	if cmdDur == nil {
		t.Fatal("cache_redis_command_duration_seconds metric not found")
	}
	if count := countHistogramDataPoints(cmdDur); count < 3 {
		t.Errorf("command duration data points = %d, want >= 3", count)
	}
}

func TestInstrumented_EntrySizeBytes_Recorded(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	entry := newMetricsTestEntry("size tracking test", 60*time.Second)
	if err := ic.Set(ctx, "key1", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	_, _, _ = ic.Get(ctx, "key1")

	rm := collectTestMetrics(t, reader)
	sizeMetric := findMetricByName(rm, "cache_redis_entries_size_bytes")
	if sizeMetric == nil {
		t.Fatal("cache_redis_entries_size_bytes metric not found")
	}
	// Should have size recorded for both set and get.
	if count := countHistogramDataPoints(sizeMetric); count < 2 {
		t.Errorf("entry size data points = %d, want >= 2", count)
	}
}

func TestInstrumented_CollectPoolStats(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	// Force some connection activity.
	entry := newMetricsTestEntry("pool test", 60*time.Second)
	_ = ic.Set(ctx, "key1", entry)

	ic.CollectPoolStats(ctx)

	rm := collectTestMetrics(t, reader)
	connActive := findMetricByName(rm, "cache_redis_connections_active")
	if connActive == nil {
		t.Fatal("cache_redis_connections_active metric not found")
	}
}

func TestInstrumented_String(t *testing.T) {
	ic, _, _ := newInstrumentedTestCache(t)
	defer ic.Close()

	s := ic.String()
	if s == "" {
		t.Error("String() returned empty string")
	}
}

func TestInstrumented_Inner(t *testing.T) {
	ic, _, _ := newInstrumentedTestCache(t)
	defer ic.Close()

	inner := ic.Inner()
	if inner == nil {
		t.Error("Inner() returned nil")
	}
}

func TestInstrumented_StatusLabel_Corrupted(t *testing.T) {
	ic, _, _ := newInstrumentedTestCache(t)
	defer ic.Close()

	corruptedErr := &cache.CacheCorruptedError{Key: "k", Reason: "bad data"}
	if got := ic.statusLabel(false, corruptedErr); got != "corrupted" {
		t.Errorf("statusLabel for corrupted = %q, want %q", got, "corrupted")
	}
}

func TestInstrumented_StatusLabel_Expired(t *testing.T) {
	ic, _, _ := newInstrumentedTestCache(t)
	defer ic.Close()

	expiredErr := &cache.CacheExpiredError{Key: "k", ExpiredAt: "2021-01-01"}
	if got := ic.statusLabel(false, expiredErr); got != "expired" {
		t.Errorf("statusLabel for expired = %q, want %q", got, "expired")
	}
}

// ---------------------------------------------------------------------------
// Integration test: all metrics are exposed
// ---------------------------------------------------------------------------

func TestIntegration_AllMetricsExposed(t *testing.T) {
	ic, mr, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	// Perform operations to generate all metric types.
	entry := newMetricsTestEntry("integration test", 60*time.Second)
	_ = ic.Set(ctx, "key1", entry)
	_, _, _ = ic.Get(ctx, "key1")    // hit
	_, _, _ = ic.Get(ctx, "missing") // miss
	_, _ = ic.Exists(ctx, "key1")
	_ = ic.Delete(ctx, "key1")
	ic.CheckHealth(ctx)
	ic.CollectPoolStats(ctx)

	// Close miniredis to trigger a connection error, which populates the counter.
	mr.Close()
	ic.CheckHealth(ctx)

	rm := collectTestMetrics(t, reader)

	expectedMetrics := []string{
		"cache_redis_operations_total",
		"cache_redis_operation_duration_seconds",
		"cache_redis_connections_active",
		"cache_redis_connection_errors_total",
		"cache_redis_command_duration_seconds",
		"cache_redis_entries_size_bytes",
		"cache_redis_hit_rate",
	}

	for _, name := range expectedMetrics {
		if m := findMetricByName(rm, name); m == nil {
			t.Errorf("expected metric %q not found", name)
		}
	}
}

func TestIntegration_MetricValues(t *testing.T) {
	ic, _, reader := newInstrumentedTestCache(t)
	defer ic.Close()
	ctx := context.Background()

	entry := newMetricsTestEntry("value test", 60*time.Second)

	// 2 sets.
	_ = ic.Set(ctx, "key1", entry)
	_ = ic.Set(ctx, "key2", entry)

	// 2 gets: 1 hit, 1 miss.
	_, _, _ = ic.Get(ctx, "key1")
	_, _, _ = ic.Get(ctx, "key999")

	rm := collectTestMetrics(t, reader)

	// Operations total = 4 (2 set + 2 get).
	opTotal := findMetricByName(rm, "cache_redis_operations_total")
	if got := sumInt64Counter(opTotal); got != 4 {
		t.Errorf("operations total = %d, want 4", got)
	}

	// Hit rate = 50% (1 hit / 2 gets).
	if got := ic.Metrics().HitRate(); got != 0.5 {
		t.Errorf("HitRate() = %f, want 0.5", got)
	}
}

// ---------------------------------------------------------------------------
// Benchmark tests
// ---------------------------------------------------------------------------

func BenchmarkInstrumented_Get_Hit(b *testing.B) {
	ResetGlobalMetrics()
	mr := miniredis.RunT(b)

	cfg := Config{
		Mode:                 ModeStandalone,
		Addr:                 mr.Addr(),
		PoolSize:             5,
		MinIdleConns:         1,
		DialTimeout:          1 * time.Second,
		ReadTimeout:          1 * time.Second,
		WriteTimeout:         1 * time.Second,
		MaxRetries:           1,
		KeyPrefix:            "bench:",
		EnableCompression:    false,
		CompressionThreshold: 1024,
		HealthCheckInterval:  0,
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:     mr.Addr(),
		PoolSize: cfg.PoolSize,
	})
	inner := NewWithClient(client, cfg)
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	m := NewMetrics(mp)
	ic := NewInstrumentedWithMetrics(inner, m)
	defer ic.Close()

	ctx := context.Background()
	entry := newMetricsTestEntry("benchmark", 60*time.Second)
	_ = ic.Set(ctx, "bench-key", entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = ic.Get(ctx, "bench-key")
	}
}

func BenchmarkInstrumented_Get_Miss(b *testing.B) {
	ResetGlobalMetrics()
	mr := miniredis.RunT(b)

	cfg := Config{
		Mode:                 ModeStandalone,
		Addr:                 mr.Addr(),
		PoolSize:             5,
		MinIdleConns:         1,
		DialTimeout:          1 * time.Second,
		ReadTimeout:          1 * time.Second,
		WriteTimeout:         1 * time.Second,
		MaxRetries:           1,
		KeyPrefix:            "bench:",
		EnableCompression:    false,
		CompressionThreshold: 1024,
		HealthCheckInterval:  0,
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:     mr.Addr(),
		PoolSize: cfg.PoolSize,
	})
	inner := NewWithClient(client, cfg)
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	m := NewMetrics(mp)
	ic := NewInstrumentedWithMetrics(inner, m)
	defer ic.Close()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = ic.Get(ctx, "nonexistent")
	}
}

func BenchmarkInstrumented_Set(b *testing.B) {
	ResetGlobalMetrics()
	mr := miniredis.RunT(b)

	cfg := Config{
		Mode:                 ModeStandalone,
		Addr:                 mr.Addr(),
		PoolSize:             5,
		MinIdleConns:         1,
		DialTimeout:          1 * time.Second,
		ReadTimeout:          1 * time.Second,
		WriteTimeout:         1 * time.Second,
		MaxRetries:           1,
		KeyPrefix:            "bench:",
		EnableCompression:    false,
		CompressionThreshold: 1024,
		HealthCheckInterval:  0,
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:     mr.Addr(),
		PoolSize: cfg.PoolSize,
	})
	inner := NewWithClient(client, cfg)
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	m := NewMetrics(mp)
	ic := NewInstrumentedWithMetrics(inner, m)
	defer ic.Close()

	ctx := context.Background()
	entry := newMetricsTestEntry("benchmark set", 60*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ic.Set(ctx, "bench-key", entry)
	}
}

func BenchmarkInstrumented_MetricOverhead(b *testing.B) {
	// Measure the overhead of instrumentation by comparing
	// instrumented vs non-instrumented cache.
	ResetGlobalMetrics()
	mr := miniredis.RunT(b)

	cfg := Config{
		Mode:                 ModeStandalone,
		Addr:                 mr.Addr(),
		PoolSize:             5,
		MinIdleConns:         1,
		DialTimeout:          1 * time.Second,
		ReadTimeout:          1 * time.Second,
		WriteTimeout:         1 * time.Second,
		MaxRetries:           1,
		KeyPrefix:            "bench:",
		EnableCompression:    false,
		CompressionThreshold: 1024,
		HealthCheckInterval:  0,
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:     mr.Addr(),
		PoolSize: cfg.PoolSize,
	})
	inner := NewWithClient(client, cfg)
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	m := NewMetrics(mp)
	ic := NewInstrumentedWithMetrics(inner, m)
	defer ic.Close()

	ctx := context.Background()
	entry := newMetricsTestEntry("overhead test", 60*time.Second)
	_ = ic.Set(ctx, "overhead-key", entry)

	b.Run("instrumented_get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, _ = ic.Get(ctx, "overhead-key")
		}
	})

	b.Run("raw_get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, _ = inner.Get(ctx, "overhead-key")
		}
	})
}
