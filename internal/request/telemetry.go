package request

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TracerName is the OpenTelemetry tracer name for the request package.
	TracerName = "github.com/llm-optimization-gateway/internal/request"
	// MeterName is the OpenTelemetry meter name for the request package.
	MeterName = "github.com/llm-optimization-gateway/internal/request"
)

// TelemetryConfig controls telemetry behavior for the gateway.
type TelemetryConfig struct {
	// EnablePromptLogging enables logging of raw prompts and responses.
	// Default false — prompts are never logged unless explicitly enabled.
	EnablePromptLogging bool
	// TracerProvider overrides the default OpenTelemetry TracerProvider.
	// If nil, otel.GetTracerProvider() is used.
	TracerProvider trace.TracerProvider
	// MeterProvider overrides the default OpenTelemetry MeterProvider.
	// If nil, otel.GetMeterProvider() is used.
	MeterProvider metric.MeterProvider
}

// DefaultTelemetryConfig returns a TelemetryConfig with safe defaults.
func DefaultTelemetryConfig() *TelemetryConfig {
	return &TelemetryConfig{
		EnablePromptLogging: false,
	}
}

// RequestTelemetry tracks all telemetry data for a single request lifecycle.
// It is safe for concurrent access.
type RequestTelemetry struct {
	mu sync.RWMutex

	// RequestID is the unique identifier for this request.
	RequestID string
	// Model is the resolved model name.
	Model string
	// Provider is the resolved provider name.
	Provider string
	// ContextID is an optional correlation identifier across requests.
	ContextID string
	// InputFormat tracks the detected input format (openai, gemini).
	InputFormat string

	// CacheHit indicates whether the response was served from cache.
	CacheHit bool

	// TokensInput is the number of input tokens.
	TokensInput int64
	// TokensOutput is the number of output tokens.
	TokensOutput int64
	// TokensCached is the number of tokens served from cache.
	TokensCached int64

	// EstimatedCost is the estimated cost in USD for this request.
	EstimatedCost float64
	// CostAvoided is the estimated cost saved by caching in USD.
	CostAvoided float64

	// Phase timings track duration of each request phase.
	phases map[string]*PhaseTiming

	// StartTime is when the request processing began.
	StartTime time.Time
	// EndTime is when the request processing completed.
	EndTime time.Time

	// rootSpan holds the root span for the request.
	rootSpan trace.Span
	// config holds the telemetry configuration.
	config *TelemetryConfig
	// tracer is the OpenTelemetry tracer for creating spans.
	tracer trace.Tracer
	// meters holds the metrics instruments.
	meters *RequestMeters
}

// PhaseTiming tracks the duration and status of a request processing phase.
type PhaseTiming struct {
	Phase     string        `json:"phase"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time,omitempty"`
	Duration  time.Duration `json:"duration,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// NewRequestTelemetry creates a new RequestTelemetry for tracking a request.
// It creates a root span and initializes all metrics instruments.
func NewRequestTelemetry(ctx context.Context, requestID string, config *TelemetryConfig) (context.Context, *RequestTelemetry) {
	if config == nil {
		config = DefaultTelemetryConfig()
	}

	tp := config.TracerProvider
	if tp == nil {
		tp = otel.GetTracerProvider()
	}

	tracer := tp.Tracer(TracerName)
	meters := getOrCreateMeters(config)

	ctx, rootSpan := tracer.Start(ctx, "gateway.request",
		trace.WithAttributes(
			attribute.String("request_id", requestID),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)

	rt := &RequestTelemetry{
		RequestID: requestID,
		StartTime: time.Now(),
		phases:    make(map[string]*PhaseTiming),
		rootSpan:  rootSpan,
		config:    config,
		tracer:    tracer,
		meters:    meters,
	}

	return ctx, rt
}

// SetModel sets the model name and updates span attributes.
func (rt *RequestTelemetry) SetModel(model string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.Model = model
	if rt.rootSpan != nil {
		rt.rootSpan.SetAttributes(attribute.String("model", model))
	}
}

// SetProvider sets the provider name and updates span attributes.
func (rt *RequestTelemetry) SetProvider(provider string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.Provider = provider
	if rt.rootSpan != nil {
		rt.rootSpan.SetAttributes(attribute.String("provider", provider))
	}
}

// SetContextID sets the context ID for cross-request correlation.
func (rt *RequestTelemetry) SetContextID(contextID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.ContextID = contextID
	if rt.rootSpan != nil {
		rt.rootSpan.SetAttributes(attribute.String("context_id", contextID))
	}
}

// SetInputFormat records the input format of the request.
func (rt *RequestTelemetry) SetInputFormat(format string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.InputFormat = format
	if rt.rootSpan != nil {
		rt.rootSpan.SetAttributes(attribute.String("input_format", format))
	}
}

// SetCacheHit records whether the response was served from cache.
func (rt *RequestTelemetry) SetCacheHit(hit bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.CacheHit = hit
	if rt.rootSpan != nil {
		rt.rootSpan.SetAttributes(attribute.Bool("cache_hit", hit))
	}
}

// SetTokenCounts records the token counts for the request.
func (rt *RequestTelemetry) SetTokenCounts(input, output, cached int64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.TokensInput = input
	rt.TokensOutput = output
	rt.TokensCached = cached
	if rt.rootSpan != nil {
		rt.rootSpan.SetAttributes(
			attribute.Int64("tokens.input", input),
			attribute.Int64("tokens.output", output),
			attribute.Int64("tokens.cached", cached),
		)
	}
}

// SetCost records the estimated cost and cost avoided.
func (rt *RequestTelemetry) SetCost(estimated, avoided float64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.EstimatedCost = estimated
	rt.CostAvoided = avoided
	if rt.rootSpan != nil {
		rt.rootSpan.SetAttributes(
			attribute.Float64("cost.estimated", estimated),
			attribute.Float64("cost.avoided", avoided),
		)
	}
}

// StartPhase begins tracking a named phase and returns a function to end it.
// The context returned carries the child span for the phase.
func (rt *RequestTelemetry) StartPhase(ctx context.Context, phase string) (context.Context, func(err error)) {
	rt.mu.Lock()
	timing := &PhaseTiming{
		Phase:     phase,
		StartTime: time.Now(),
	}
	rt.phases[phase] = timing
	rt.mu.Unlock()

	ctx, span := startPhaseSpan(ctx, rt.tracer, phase, rt.RequestID)

	return ctx, func(err error) {
		rt.mu.Lock()
		timing.EndTime = time.Now()
		timing.Duration = timing.EndTime.Sub(timing.StartTime)
		if err != nil {
			timing.Error = err.Error()
		}
		rt.mu.Unlock()

		endPhaseSpan(span, phase, err)

		if rt.meters != nil {
			rt.meters.RecordPhaseDuration(ctx, phase, timing.Duration, rt.Model, rt.Provider)
		}
	}
}

// Finish completes the request telemetry, recording final metrics and ending
// the root span. It should be called exactly once when request processing is done.
func (rt *RequestTelemetry) Finish(ctx context.Context, err error) {
	rt.mu.Lock()
	rt.EndTime = time.Now()
	totalDuration := rt.EndTime.Sub(rt.StartTime)
	rt.mu.Unlock()

	// Record final metrics.
	if rt.meters != nil {
		rt.meters.RecordRequest(ctx, rt.Model, rt.Provider, rt.CacheHit, err)
		rt.meters.RecordDuration(ctx, totalDuration, rt.Model, rt.Provider)
		rt.meters.RecordTokens(ctx, rt.TokensInput, rt.TokensOutput, rt.TokensCached, rt.Model, rt.Provider)
		rt.meters.RecordCost(ctx, rt.EstimatedCost, rt.CostAvoided, rt.Model, rt.Provider)

		if rt.CacheHit {
			rt.meters.RecordCacheHit(ctx, rt.Model, rt.Provider)
		} else {
			rt.meters.RecordCacheMiss(ctx, rt.Model, rt.Provider)
		}
	}

	// End root span.
	if rt.rootSpan != nil {
		if err != nil {
			rt.rootSpan.SetAttributes(attribute.String("error.message", err.Error()))
			rt.rootSpan.SetAttributes(attribute.Bool("error", true))
		}
		rt.rootSpan.SetAttributes(attribute.Float64("duration_ms", float64(totalDuration.Milliseconds())))
		rt.rootSpan.End()
	}
}

// GetPhaseDuration returns the duration of a completed phase.
func (rt *RequestTelemetry) GetPhaseDuration(phase string) time.Duration {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	if timing, ok := rt.phases[phase]; ok {
		return timing.Duration
	}
	return 0
}

// GetTotalDuration returns the total request duration.
func (rt *RequestTelemetry) GetTotalDuration() time.Duration {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	if rt.EndTime.IsZero() {
		return time.Since(rt.StartTime)
	}
	return rt.EndTime.Sub(rt.StartTime)
}

// GetPhases returns a copy of all phase timings.
func (rt *RequestTelemetry) GetPhases() []PhaseTiming {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	result := make([]PhaseTiming, 0, len(rt.phases))
	for _, timing := range rt.phases {
		result = append(result, *timing)
	}
	return result
}

// RootSpan returns the root span for the request, allowing callers to add
// custom attributes or events.
func (rt *RequestTelemetry) RootSpan() trace.Span {
	return rt.rootSpan
}

// Tracer returns the tracer used by this request telemetry.
func (rt *RequestTelemetry) Tracer() trace.Tracer {
	return rt.tracer
}

// PromptLoggingEnabled returns whether prompt logging is enabled.
func (rt *RequestTelemetry) PromptLoggingEnabled() bool {
	if rt.config == nil {
		return false
	}
	return rt.config.EnablePromptLogging
}
