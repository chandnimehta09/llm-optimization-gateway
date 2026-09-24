package prefix

import (
	"context"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	// MeterName is the OpenTelemetry meter name for the prefix cache package.
	MeterName = "github.com/llm-optimization-gateway/internal/prefix"
)

// Telemetry tracks prefix cache metrics for observability.
type Telemetry struct {
	// Counters for computing aggregate statistics.
	totalRequests     atomic.Int64
	cacheHitRequests  atomic.Int64
	totalPrefixTokens atomic.Int64
	totalCachedTokens atomic.Int64

	// OTel instruments.
	prefixTokensCounter  metric.Int64Counter
	cachedTokensCounter  metric.Int64Counter
	cacheHitCounter      metric.Int64Counter
	cacheMissCounter     metric.Int64Counter
	requestDuration      metric.Float64Histogram
	latencySavings       metric.Float64Histogram
	prefixCacheHitRate   metric.Float64Histogram
	requestsTotal        metric.Int64Counter
	prefixLengthHist     metric.Float64Histogram

	// meterProvider is kept for testing.
	meterProvider metric.MeterProvider
}

// NewTelemetry creates a Telemetry instance using the given MeterProvider.
// If provider is nil, the global OTel MeterProvider is used.
func NewTelemetry(provider metric.MeterProvider) (*Telemetry, error) {
	if provider == nil {
		provider = otel.GetMeterProvider()
	}

	meter := provider.Meter(MeterName)

	prefixTokens, err := meter.Int64Counter("prefix_cache.prefix_tokens",
		metric.WithDescription("Total prefix tokens sent to vLLM"),
		metric.WithUnit("{tokens}"))
	if err != nil {
		return nil, err
	}

	cachedTokens, err := meter.Int64Counter("prefix_cache.cached_tokens",
		metric.WithDescription("Total tokens served from vLLM prefix cache"),
		metric.WithUnit("{tokens}"))
	if err != nil {
		return nil, err
	}

	cacheHit, err := meter.Int64Counter("prefix_cache.hits",
		metric.WithDescription("Number of requests with prefix cache hits"),
		metric.WithUnit("{requests}"))
	if err != nil {
		return nil, err
	}

	cacheMiss, err := meter.Int64Counter("prefix_cache.misses",
		metric.WithDescription("Number of requests with prefix cache misses"),
		metric.WithUnit("{requests}"))
	if err != nil {
		return nil, err
	}

	reqDuration, err := meter.Float64Histogram("prefix_cache.request_duration",
		metric.WithDescription("Duration of vLLM requests in milliseconds"),
		metric.WithUnit("ms"))
	if err != nil {
		return nil, err
	}

	latSavings, err := meter.Float64Histogram("prefix_cache.latency_savings",
		metric.WithDescription("Estimated latency savings from prefix caching in milliseconds"),
		metric.WithUnit("ms"))
	if err != nil {
		return nil, err
	}

	hitRate, err := meter.Float64Histogram("prefix_cache.hit_rate",
		metric.WithDescription("Per-request prefix cache hit rate (0.0-1.0)"))
	if err != nil {
		return nil, err
	}

	reqTotal, err := meter.Int64Counter("prefix_cache.requests_total",
		metric.WithDescription("Total requests processed by prefix cache backend"),
		metric.WithUnit("{requests}"))
	if err != nil {
		return nil, err
	}

	prefixLen, err := meter.Float64Histogram("prefix_cache.prefix_length",
		metric.WithDescription("Length of constructed prefix in estimated tokens"),
		metric.WithUnit("{tokens}"))
	if err != nil {
		return nil, err
	}

	return &Telemetry{
		prefixTokensCounter: prefixTokens,
		cachedTokensCounter: cachedTokens,
		cacheHitCounter:     cacheHit,
		cacheMissCounter:    cacheMiss,
		requestDuration:     reqDuration,
		latencySavings:      latSavings,
		prefixCacheHitRate:  hitRate,
		requestsTotal:       reqTotal,
		prefixLengthHist:    prefixLen,
		meterProvider:       provider,
	}, nil
}

// RequestResult holds the outcome of a single prefix-cache request for recording.
type RequestResult struct {
	// Model is the model that was called.
	Model string
	// PrefixTokens is the number of prefix tokens in the request.
	PrefixTokens int64
	// CachedTokens is the number of tokens served from the prefix cache.
	CachedTokens int64
	// TotalPromptTokens is the total prompt tokens reported by vLLM.
	TotalPromptTokens int64
	// Duration is the total request duration.
	Duration time.Duration
	// CacheHit is true if any tokens were served from the prefix cache.
	CacheHit bool
	// Error is non-nil if the request failed.
	Error error
}

// HitRate returns the fraction of prompt tokens that were cached.
func (r *RequestResult) HitRate() float64 {
	if r.TotalPromptTokens == 0 {
		return 0
	}
	return float64(r.CachedTokens) / float64(r.TotalPromptTokens)
}

// EstimatedLatencySavingsMs estimates the latency saved by prefix caching.
// It uses a simple model: cached tokens are ~10x faster than computed tokens.
// The savings are: duration * (cached_ratio * 0.9) — assuming 90% of compute
// time for cached tokens is avoided.
func (r *RequestResult) EstimatedLatencySavingsMs() float64 {
	if r.TotalPromptTokens == 0 || r.CachedTokens == 0 {
		return 0
	}
	cachedRatio := float64(r.CachedTokens) / float64(r.TotalPromptTokens)
	// Estimate that prefix processing is roughly proportional to token count
	// and cached tokens avoid ~90% of the compute.
	return float64(r.Duration.Milliseconds()) * cachedRatio * 0.9
}

// RecordRequest records telemetry for a completed prefix-cache request.
func (t *Telemetry) RecordRequest(ctx context.Context, result *RequestResult) {
	if result == nil {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("model", result.Model),
	}
	if result.Error != nil {
		attrs = append(attrs, attribute.String("status", "error"))
	} else {
		attrs = append(attrs, attribute.String("status", "ok"))
	}
	attrSet := metric.WithAttributes(attrs...)

	t.requestsTotal.Add(ctx, 1, attrSet)
	t.totalRequests.Add(1)

	if result.Error != nil {
		return
	}

	t.prefixTokensCounter.Add(ctx, result.PrefixTokens, attrSet)
	t.cachedTokensCounter.Add(ctx, result.CachedTokens, attrSet)
	t.requestDuration.Record(ctx, float64(result.Duration.Milliseconds()), attrSet)
	t.prefixCacheHitRate.Record(ctx, result.HitRate(), attrSet)

	t.totalPrefixTokens.Add(result.PrefixTokens)
	t.totalCachedTokens.Add(result.CachedTokens)

	if result.CacheHit {
		t.cacheHitCounter.Add(ctx, 1, attrSet)
		t.cacheHitRequests.Add(1)
		savings := result.EstimatedLatencySavingsMs()
		t.latencySavings.Record(ctx, savings, attrSet)
	} else {
		t.cacheMissCounter.Add(ctx, 1, attrSet)
	}
}

// RecordPrefixLength records the length of a constructed prefix.
func (t *Telemetry) RecordPrefixLength(ctx context.Context, model string, estimatedTokens float64) {
	t.prefixLengthHist.Record(ctx, estimatedTokens,
		metric.WithAttributes(attribute.String("model", model)))
}

// Snapshot returns a point-in-time snapshot of aggregate prefix cache statistics.
func (t *Telemetry) Snapshot() TelemetrySnapshot {
	return TelemetrySnapshot{
		TotalRequests:     t.totalRequests.Load(),
		CacheHitRequests:  t.cacheHitRequests.Load(),
		TotalPrefixTokens: t.totalPrefixTokens.Load(),
		TotalCachedTokens: t.totalCachedTokens.Load(),
	}
}

// TelemetrySnapshot is a point-in-time view of aggregate prefix cache metrics.
type TelemetrySnapshot struct {
	// TotalRequests is the total number of requests processed.
	TotalRequests int64
	// CacheHitRequests is the number of requests where prefix cache was hit.
	CacheHitRequests int64
	// TotalPrefixTokens is the cumulative prefix tokens across all requests.
	TotalPrefixTokens int64
	// TotalCachedTokens is the cumulative cached tokens across all requests.
	TotalCachedTokens int64
}

// HitRate returns the fraction of requests that had a prefix cache hit.
func (s *TelemetrySnapshot) HitRate() float64 {
	if s.TotalRequests == 0 {
		return 0
	}
	return float64(s.CacheHitRequests) / float64(s.TotalRequests)
}

// TokenCacheRate returns the fraction of prefix tokens that were cached.
func (s *TelemetrySnapshot) TokenCacheRate() float64 {
	if s.TotalPrefixTokens == 0 {
		return 0
	}
	return float64(s.TotalCachedTokens) / float64(s.TotalPrefixTokens)
}
