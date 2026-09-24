package request

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RequestMeters holds all OpenTelemetry metric instruments for the gateway.
type RequestMeters struct {
	// requestsTotal counts total requests processed.
	requestsTotal metric.Int64Counter
	// requestDuration records total request duration.
	requestDuration metric.Float64Histogram
	// phaseDuration records duration of each request phase.
	phaseDuration metric.Float64Histogram
	// tokensInput records input token counts.
	tokensInput metric.Int64Counter
	// tokensOutput records output token counts.
	tokensOutput metric.Int64Counter
	// tokensCached records cached token counts.
	tokensCached metric.Int64Counter
	// cacheHits counts cache hit events.
	cacheHits metric.Int64Counter
	// cacheMisses counts cache miss events.
	cacheMisses metric.Int64Counter
	// providerLatency records per-provider latency.
	providerLatency metric.Float64Histogram
	// modelUsage counts requests per model.
	modelUsage metric.Int64Counter
	// estimatedCost records estimated cost per request.
	estimatedCost metric.Float64Counter
	// costAvoided records cost avoided through caching.
	costAvoided metric.Float64Counter
	// requestErrors counts request errors.
	requestErrors metric.Int64Counter
}

var (
	globalMeters     *RequestMeters
	globalMetersOnce sync.Once
	globalMetersMu   sync.Mutex
)

// getOrCreateMeters returns the singleton RequestMeters instance, creating it if necessary.
func getOrCreateMeters(config *TelemetryConfig) *RequestMeters {
	globalMetersMu.Lock()
	defer globalMetersMu.Unlock()

	if globalMeters != nil {
		return globalMeters
	}

	mp := config.MeterProvider
	if mp == nil {
		mp = otel.GetMeterProvider()
	}

	meter := mp.Meter(MeterName)
	m := &RequestMeters{}

	var err error

	m.requestsTotal, err = meter.Int64Counter("gateway_requests_total",
		metric.WithDescription("Total number of requests processed by the gateway"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		m.requestsTotal, _ = meter.Int64Counter("gateway_requests_total_fallback")
	}

	m.requestDuration, err = meter.Float64Histogram("gateway_request_duration_seconds",
		metric.WithDescription("Duration of request processing in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		m.requestDuration, _ = meter.Float64Histogram("gateway_request_duration_seconds_fallback")
	}

	m.phaseDuration, err = meter.Float64Histogram("gateway_phase_duration_seconds",
		metric.WithDescription("Duration of individual request phases in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		m.phaseDuration, _ = meter.Float64Histogram("gateway_phase_duration_seconds_fallback")
	}

	m.tokensInput, err = meter.Int64Counter("gateway_tokens_input_total",
		metric.WithDescription("Total number of input tokens processed"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		m.tokensInput, _ = meter.Int64Counter("gateway_tokens_input_total_fallback")
	}

	m.tokensOutput, err = meter.Int64Counter("gateway_tokens_output_total",
		metric.WithDescription("Total number of output tokens generated"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		m.tokensOutput, _ = meter.Int64Counter("gateway_tokens_output_total_fallback")
	}

	m.tokensCached, err = meter.Int64Counter("gateway_tokens_cached_total",
		metric.WithDescription("Total number of tokens served from cache"),
		metric.WithUnit("{token}"),
	)
	if err != nil {
		m.tokensCached, _ = meter.Int64Counter("gateway_tokens_cached_total_fallback")
	}

	m.cacheHits, err = meter.Int64Counter("gateway_cache_hits_total",
		metric.WithDescription("Total number of cache hits"),
		metric.WithUnit("{hit}"),
	)
	if err != nil {
		m.cacheHits, _ = meter.Int64Counter("gateway_cache_hits_total_fallback")
	}

	m.cacheMisses, err = meter.Int64Counter("gateway_cache_misses_total",
		metric.WithDescription("Total number of cache misses"),
		metric.WithUnit("{miss}"),
	)
	if err != nil {
		m.cacheMisses, _ = meter.Int64Counter("gateway_cache_misses_total_fallback")
	}

	m.providerLatency, err = meter.Float64Histogram("gateway_provider_latency_seconds",
		metric.WithDescription("Latency of provider calls in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		m.providerLatency, _ = meter.Float64Histogram("gateway_provider_latency_seconds_fallback")
	}

	m.modelUsage, err = meter.Int64Counter("gateway_model_usage_total",
		metric.WithDescription("Total number of requests per model"),
		metric.WithUnit("{request}"),
	)
	if err != nil {
		m.modelUsage, _ = meter.Int64Counter("gateway_model_usage_total_fallback")
	}

	m.estimatedCost, err = meter.Float64Counter("gateway_estimated_cost_dollars",
		metric.WithDescription("Estimated cost of requests in USD"),
		metric.WithUnit("USD"),
	)
	if err != nil {
		m.estimatedCost, _ = meter.Float64Counter("gateway_estimated_cost_dollars_fallback")
	}

	m.costAvoided, err = meter.Float64Counter("gateway_cost_avoided_dollars",
		metric.WithDescription("Estimated cost avoided through caching in USD"),
		metric.WithUnit("USD"),
	)
	if err != nil {
		m.costAvoided, _ = meter.Float64Counter("gateway_cost_avoided_dollars_fallback")
	}

	m.requestErrors, err = meter.Int64Counter("gateway_request_errors_total",
		metric.WithDescription("Total number of request errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		m.requestErrors, _ = meter.Int64Counter("gateway_request_errors_total_fallback")
	}

	globalMeters = m
	return m
}

// ResetGlobalMeters resets the global meters singleton. Used for testing.
func ResetGlobalMeters() {
	globalMetersMu.Lock()
	defer globalMetersMu.Unlock()
	globalMeters = nil
	globalMetersOnce = sync.Once{}
}

// RecordRequest records a completed request.
func (m *RequestMeters) RecordRequest(ctx context.Context, model, provider string, cacheHit bool, err error) {
	attrs := []attribute.KeyValue{
		attribute.String("model", model),
		attribute.String("provider", provider),
		attribute.Bool("cache_hit", cacheHit),
	}
	if err != nil {
		attrs = append(attrs, attribute.String("status", "error"))
	} else {
		attrs = append(attrs, attribute.String("status", "ok"))
	}

	m.requestsTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.modelUsage.Add(ctx, 1, metric.WithAttributes(
		attribute.String("model", model),
		attribute.String("provider", provider),
	))

	if err != nil {
		m.requestErrors.Add(ctx, 1, metric.WithAttributes(
			attribute.String("model", model),
			attribute.String("provider", provider),
		))
	}
}

// RecordDuration records the total request duration.
func (m *RequestMeters) RecordDuration(ctx context.Context, duration time.Duration, model, provider string) {
	m.requestDuration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(
			attribute.String("model", model),
			attribute.String("provider", provider),
		),
	)
}

// RecordPhaseDuration records the duration of a specific request phase.
func (m *RequestMeters) RecordPhaseDuration(ctx context.Context, phase string, duration time.Duration, model, provider string) {
	m.phaseDuration.Record(ctx, duration.Seconds(),
		metric.WithAttributes(
			attribute.String("phase", phase),
			attribute.String("model", model),
			attribute.String("provider", provider),
		),
	)
}

// RecordTokens records token counts for a request.
func (m *RequestMeters) RecordTokens(ctx context.Context, input, output, cached int64, model, provider string) {
	attrs := metric.WithAttributes(
		attribute.String("model", model),
		attribute.String("provider", provider),
	)
	if input > 0 {
		m.tokensInput.Add(ctx, input, attrs)
	}
	if output > 0 {
		m.tokensOutput.Add(ctx, output, attrs)
	}
	if cached > 0 {
		m.tokensCached.Add(ctx, cached, attrs)
	}
}

// RecordCacheHit records a cache hit event.
func (m *RequestMeters) RecordCacheHit(ctx context.Context, model, provider string) {
	m.cacheHits.Add(ctx, 1, metric.WithAttributes(
		attribute.String("model", model),
		attribute.String("provider", provider),
	))
}

// RecordCacheMiss records a cache miss event.
func (m *RequestMeters) RecordCacheMiss(ctx context.Context, model, provider string) {
	m.cacheMisses.Add(ctx, 1, metric.WithAttributes(
		attribute.String("model", model),
		attribute.String("provider", provider),
	))
}

// RecordProviderLatency records the latency of a provider call.
func (m *RequestMeters) RecordProviderLatency(ctx context.Context, duration time.Duration, model, provider string) {
	m.providerLatency.Record(ctx, duration.Seconds(),
		metric.WithAttributes(
			attribute.String("model", model),
			attribute.String("provider", provider),
		),
	)
}

// RecordCost records estimated cost and cost avoided.
func (m *RequestMeters) RecordCost(ctx context.Context, estimated, avoided float64, model, provider string) {
	attrs := metric.WithAttributes(
		attribute.String("model", model),
		attribute.String("provider", provider),
	)
	if estimated > 0 {
		m.estimatedCost.Add(ctx, estimated, attrs)
	}
	if avoided > 0 {
		m.costAvoided.Add(ctx, avoided, attrs)
	}
}
