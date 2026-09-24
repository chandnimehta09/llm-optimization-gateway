package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"

	"github.com/llm-optimization-gateway/internal/cache"
)

// InstrumentedCache wraps a Redis Cache and records Prometheus-style metrics
// on every operation using the OpenTelemetry metrics API.
type InstrumentedCache struct {
	inner   *Cache
	metrics *Metrics
}

// compile-time interface check.
var _ cache.Cache = (*InstrumentedCache)(nil)

// NewInstrumented wraps an existing Redis Cache with metrics instrumentation.
// If mp is nil, the global OTel MeterProvider is used.
func NewInstrumented(inner *Cache, mp otelmetric.MeterProvider) *InstrumentedCache {
	return &InstrumentedCache{
		inner:   inner,
		metrics: NewMetrics(mp),
	}
}

// NewInstrumentedWithMetrics wraps an existing Redis Cache with a pre-built
// Metrics instance. Useful for sharing metrics across multiple caches or for testing.
func NewInstrumentedWithMetrics(inner *Cache, m *Metrics) *InstrumentedCache {
	return &InstrumentedCache{
		inner:   inner,
		metrics: m,
	}
}

// Get retrieves a cache entry by key, recording operation metrics.
func (ic *InstrumentedCache) Get(ctx context.Context, key string) (*cache.Entry, bool, error) {
	start := time.Now()

	entry, found, err := ic.inner.Get(ctx, key)
	duration := time.Since(start)

	operation := "get"
	status := ic.statusLabel(found, err)

	ic.recordOperation(ctx, operation, status, duration)

	if err == nil {
		if found {
			ic.metrics.recordHit()
		} else {
			ic.metrics.recordMiss()
		}
		// Update the hit rate gauge.
		ic.metrics.hitRate.Record(ctx, ic.metrics.HitRate())
	}

	// Record the size of the returned entry.
	if entry != nil {
		if data, serErr := ic.inner.serializer.Marshal(entry); serErr == nil {
			ic.metrics.entrySizeBytes.Record(ctx, float64(len(data)),
				otelmetric.WithAttributes(attribute.String("operation", operation)),
			)
		}
	}

	return entry, found, err
}

// Set stores a cache entry, recording operation metrics.
func (ic *InstrumentedCache) Set(ctx context.Context, key string, entry *cache.Entry, opts ...cache.Option) error {
	start := time.Now()

	err := ic.inner.Set(ctx, key, entry, opts...)
	duration := time.Since(start)

	operation := "set"
	status := "ok"
	if err != nil {
		status = "error"
	}

	ic.recordOperation(ctx, operation, status, duration)

	// Record entry size on successful set.
	if err == nil && entry != nil {
		if data, serErr := ic.inner.serializer.Marshal(entry); serErr == nil {
			ic.metrics.entrySizeBytes.Record(ctx, float64(len(data)),
				otelmetric.WithAttributes(attribute.String("operation", operation)),
			)
		}
	}

	return err
}

// Delete removes a cache entry, recording operation metrics.
func (ic *InstrumentedCache) Delete(ctx context.Context, key string) error {
	start := time.Now()

	err := ic.inner.Delete(ctx, key)
	duration := time.Since(start)

	operation := "delete"
	status := "ok"
	if err != nil {
		status = "error"
	}

	ic.recordOperation(ctx, operation, status, duration)
	return err
}

// Exists checks whether a key is present, recording operation metrics.
func (ic *InstrumentedCache) Exists(ctx context.Context, key string) (bool, error) {
	start := time.Now()

	exists, err := ic.inner.Exists(ctx, key)
	duration := time.Since(start)

	operation := "exists"
	status := "ok"
	if err != nil {
		status = "error"
	}

	ic.recordOperation(ctx, operation, status, duration)
	return exists, err
}

// Close releases resources, recording any connection errors.
func (ic *InstrumentedCache) Close() error {
	err := ic.inner.Close()
	if err != nil {
		ic.metrics.connectionErrorsTotal.Add(context.Background(), 1)
	}
	return err
}

// Health returns the most recent health check report.
func (ic *InstrumentedCache) Health() HealthReport {
	return ic.inner.Health()
}

// CheckHealth performs a health check and records metrics.
func (ic *InstrumentedCache) CheckHealth(ctx context.Context) HealthReport {
	start := time.Now()

	report := ic.inner.CheckHealth(ctx)
	duration := time.Since(start)

	// Record health check as a command.
	ic.metrics.commandDuration.Record(ctx, duration.Seconds(),
		otelmetric.WithAttributes(attribute.String("command", "ping")),
	)

	// Record connection errors if health check fails.
	if report.Status == HealthStatusDown {
		ic.metrics.connectionErrorsTotal.Add(ctx, 1)
	}

	return report
}

// CollectPoolStats collects connection pool statistics and records them as metrics.
// This should be called periodically (e.g., via a ticker) to keep pool metrics current.
func (ic *InstrumentedCache) CollectPoolStats(ctx context.Context) {
	// Only *redis.Client supports PoolStats() directly.
	client, ok := ic.inner.client.(*goredis.Client)
	if !ok {
		return
	}

	stats := client.PoolStats()
	if stats == nil {
		return
	}

	// ActiveConns = TotalConns - IdleConns
	active := int64(stats.TotalConns) - int64(stats.IdleConns)
	if active < 0 {
		active = 0
	}
	ic.metrics.connectionsActive.Record(ctx, active)
}

// Metrics returns the underlying metrics instance. Useful for tests and introspection.
func (ic *InstrumentedCache) Metrics() *Metrics {
	return ic.metrics
}

// Inner returns the underlying unwrapped Cache. Useful for operations that
// need direct access.
func (ic *InstrumentedCache) Inner() *Cache {
	return ic.inner
}

// recordOperation records operation-level metrics (total count and duration).
func (ic *InstrumentedCache) recordOperation(ctx context.Context, operation, status string, duration time.Duration) {
	attrs := otelmetric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("status", status),
	)

	ic.metrics.operationsTotal.Add(ctx, 1, attrs)
	ic.metrics.operationDuration.Record(ctx, duration.Seconds(),
		otelmetric.WithAttributes(attribute.String("operation", operation)),
	)

	// Record as a Redis command as well.
	ic.metrics.commandDuration.Record(ctx, duration.Seconds(),
		otelmetric.WithAttributes(attribute.String("command", operation)),
	)

	// Track connection errors.
	if status == "error" {
		ic.metrics.connectionErrorsTotal.Add(ctx, 1)
	}
}

// statusLabel returns a status label string based on the result of an operation.
func (ic *InstrumentedCache) statusLabel(found bool, err error) string {
	if err != nil {
		if cache.IsCacheCorrupted(err) {
			return "corrupted"
		}
		if cache.IsCacheExpired(err) {
			return "expired"
		}
		return "error"
	}
	if found {
		return "hit"
	}
	return "miss"
}

// String implements fmt.Stringer for InstrumentedCache for debugging.
func (ic *InstrumentedCache) String() string {
	return fmt.Sprintf("InstrumentedCache(hitRate=%.2f, total=%d)", ic.metrics.HitRate(), ic.metrics.Total())
}
