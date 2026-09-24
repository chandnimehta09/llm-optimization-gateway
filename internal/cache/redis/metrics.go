package redis

import (
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const (
	// MeterName is the OpenTelemetry meter name for redis cache metrics.
	MeterName = "github.com/llm-optimization-gateway/internal/cache/redis"
)

// Metrics holds all OpenTelemetry metric instruments for the Redis cache.
type Metrics struct {
	// operationsTotal counts total cache operations by operation and status.
	operationsTotal metric.Int64Counter

	// operationDuration records duration of cache operations in seconds.
	operationDuration metric.Float64Histogram

	// connectionsActive tracks the number of active connections in the pool.
	connectionsActive metric.Int64Gauge

	// connectionErrorsTotal counts connection-level errors.
	connectionErrorsTotal metric.Int64Counter

	// commandDuration records duration of individual Redis commands in seconds.
	commandDuration metric.Float64Histogram

	// entrySizeBytes records the size of cache entries in bytes.
	entrySizeBytes metric.Float64Histogram

	// hitRate tracks the current cache hit rate as a gauge (0.0 to 1.0).
	hitRate metric.Float64Gauge

	// hits tracks the total number of cache hits (used to compute hitRate).
	hits int64
	// total tracks the total number of get operations (used to compute hitRate).
	total int64
	// mu protects hits and total.
	mu sync.Mutex
}

var (
	globalMetrics   *Metrics
	globalMetricsMu sync.Mutex
)

// NewMetrics creates a new Metrics instance using the provided MeterProvider.
// If mp is nil, the global OTel MeterProvider is used.
func NewMetrics(mp metric.MeterProvider) *Metrics {
	if mp == nil {
		mp = otel.GetMeterProvider()
	}

	meter := mp.Meter(MeterName)
	m := &Metrics{}

	var err error

	m.operationsTotal, err = meter.Int64Counter("cache_redis_operations_total",
		metric.WithDescription("Total number of Redis cache operations"),
		metric.WithUnit("{operation}"),
	)
	if err != nil {
		m.operationsTotal, _ = meter.Int64Counter("cache_redis_operations_total_fallback")
	}

	m.operationDuration, err = meter.Float64Histogram("cache_redis_operation_duration_seconds",
		metric.WithDescription("Duration of Redis cache operations in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		m.operationDuration, _ = meter.Float64Histogram("cache_redis_operation_duration_seconds_fallback")
	}

	m.connectionsActive, err = meter.Int64Gauge("cache_redis_connections_active",
		metric.WithDescription("Number of active connections in the Redis connection pool"),
		metric.WithUnit("{connection}"),
	)
	if err != nil {
		m.connectionsActive, _ = meter.Int64Gauge("cache_redis_connections_active_fallback")
	}

	m.connectionErrorsTotal, err = meter.Int64Counter("cache_redis_connection_errors_total",
		metric.WithDescription("Total number of Redis connection errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		m.connectionErrorsTotal, _ = meter.Int64Counter("cache_redis_connection_errors_total_fallback")
	}

	m.commandDuration, err = meter.Float64Histogram("cache_redis_command_duration_seconds",
		metric.WithDescription("Duration of individual Redis commands in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		m.commandDuration, _ = meter.Float64Histogram("cache_redis_command_duration_seconds_fallback")
	}

	m.entrySizeBytes, err = meter.Float64Histogram("cache_redis_entries_size_bytes",
		metric.WithDescription("Size of Redis cache entries in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		m.entrySizeBytes, _ = meter.Float64Histogram("cache_redis_entries_size_bytes_fallback")
	}

	m.hitRate, err = meter.Float64Gauge("cache_redis_hit_rate",
		metric.WithDescription("Current cache hit rate (0.0 to 1.0)"),
		metric.WithUnit("1"),
	)
	if err != nil {
		m.hitRate, _ = meter.Float64Gauge("cache_redis_hit_rate_fallback")
	}

	return m
}

// GetOrCreateMetrics returns the global Metrics singleton, creating it if necessary.
func GetOrCreateMetrics(mp metric.MeterProvider) *Metrics {
	globalMetricsMu.Lock()
	defer globalMetricsMu.Unlock()

	if globalMetrics != nil {
		return globalMetrics
	}

	globalMetrics = NewMetrics(mp)
	return globalMetrics
}

// ResetGlobalMetrics resets the global metrics singleton. Used for testing.
func ResetGlobalMetrics() {
	globalMetricsMu.Lock()
	defer globalMetricsMu.Unlock()
	globalMetrics = nil
}

// recordHit updates the internal hit/total counters and recomputes the hit rate.
func (m *Metrics) recordHit() {
	m.mu.Lock()
	m.hits++
	m.total++
	m.mu.Unlock()
}

// recordMiss updates the internal total counter and recomputes the hit rate.
func (m *Metrics) recordMiss() {
	m.mu.Lock()
	m.total++
	m.mu.Unlock()
}

// HitRate returns the current hit rate (0.0 to 1.0). Returns 0 if no operations
// have been recorded.
func (m *Metrics) HitRate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.total == 0 {
		return 0
	}
	return float64(m.hits) / float64(m.total)
}

// Hits returns the total number of cache hits.
func (m *Metrics) Hits() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hits
}

// Total returns the total number of get operations.
func (m *Metrics) Total() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.total
}
