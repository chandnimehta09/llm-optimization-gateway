package request

import (
	"context"
	"encoding/json"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// RequestLogger provides structured logging for the request lifecycle.
// It attaches standard fields and respects the prompt-logging configuration.
type RequestLogger struct {
	logger *slog.Logger
	config *TelemetryConfig
}

// NewRequestLogger creates a RequestLogger with the given slog.Logger and config.
// If logger is nil, slog.Default() is used.
// If config is nil, DefaultTelemetryConfig() is used.
func NewRequestLogger(logger *slog.Logger, config *TelemetryConfig) *RequestLogger {
	if logger == nil {
		logger = slog.Default()
	}
	if config == nil {
		config = DefaultTelemetryConfig()
	}
	return &RequestLogger{
		logger: logger,
		config: config,
	}
}

// LogRequestStart logs the beginning of request processing with standard metadata.
func (rl *RequestLogger) LogRequestStart(ctx context.Context, rt *RequestTelemetry) {
	attrs := rl.baseAttrs(ctx, rt)
	attrs = append(attrs, slog.String("event", "request_start"))
	rl.logger.LogAttrs(ctx, slog.LevelInfo, "request processing started", attrs...)
}

// LogRequestComplete logs the completion of request processing with final metrics.
func (rl *RequestLogger) LogRequestComplete(ctx context.Context, rt *RequestTelemetry, err error) {
	attrs := rl.baseAttrs(ctx, rt)
	attrs = append(attrs,
		slog.String("event", "request_complete"),
		slog.Float64("latency_ms", float64(rt.GetTotalDuration().Milliseconds())),
		slog.Int64("tokens_input", rt.TokensInput),
		slog.Int64("tokens_output", rt.TokensOutput),
		slog.Int64("tokens_cached", rt.TokensCached),
		slog.Float64("estimated_cost", rt.EstimatedCost),
		slog.Float64("cost_avoided", rt.CostAvoided),
		slog.Bool("cache_hit", rt.CacheHit),
	)

	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		rl.logger.LogAttrs(ctx, slog.LevelError, "request processing failed", attrs...)
	} else {
		rl.logger.LogAttrs(ctx, slog.LevelInfo, "request processing completed", attrs...)
	}
}

// LogPhaseStart logs the start of a request phase.
func (rl *RequestLogger) LogPhaseStart(ctx context.Context, rt *RequestTelemetry, phase string) {
	attrs := rl.baseAttrs(ctx, rt)
	attrs = append(attrs,
		slog.String("event", "phase_start"),
		slog.String("phase", phase),
	)
	rl.logger.LogAttrs(ctx, slog.LevelDebug, "phase started", attrs...)
}

// LogPhaseComplete logs the completion of a request phase.
func (rl *RequestLogger) LogPhaseComplete(ctx context.Context, rt *RequestTelemetry, phase string, durationMs float64, err error) {
	attrs := rl.baseAttrs(ctx, rt)
	attrs = append(attrs,
		slog.String("event", "phase_complete"),
		slog.String("phase", phase),
		slog.Float64("phase_duration_ms", durationMs),
	)
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		rl.logger.LogAttrs(ctx, slog.LevelWarn, "phase failed", attrs...)
	} else {
		rl.logger.LogAttrs(ctx, slog.LevelDebug, "phase completed", attrs...)
	}
}

// LogCacheResult logs a cache lookup result.
func (rl *RequestLogger) LogCacheResult(ctx context.Context, rt *RequestTelemetry, hit bool) {
	attrs := rl.baseAttrs(ctx, rt)
	attrs = append(attrs,
		slog.String("event", "cache_result"),
		slog.Bool("cache_hit", hit),
	)
	rl.logger.LogAttrs(ctx, slog.LevelInfo, "cache lookup result", attrs...)
}

// LogPrompt logs the request prompt. This is only logged when prompt logging
// is explicitly enabled in the configuration. By default, prompts are never logged.
func (rl *RequestLogger) LogPrompt(ctx context.Context, rt *RequestTelemetry, prompt interface{}) {
	if !rl.config.EnablePromptLogging {
		return
	}
	attrs := rl.baseAttrs(ctx, rt)
	attrs = append(attrs, slog.String("event", "prompt_logged"))

	// Marshal prompt to JSON for structured logging.
	promptJSON, err := json.Marshal(prompt)
	if err != nil {
		attrs = append(attrs, slog.String("prompt_error", "failed to marshal prompt"))
	} else {
		attrs = append(attrs, slog.String("prompt", string(promptJSON)))
	}
	rl.logger.LogAttrs(ctx, slog.LevelDebug, "prompt content (debug logging enabled)", attrs...)
}

// LogResponse logs the response content. This is only logged when prompt logging
// is explicitly enabled in the configuration. By default, responses are never logged.
func (rl *RequestLogger) LogResponse(ctx context.Context, rt *RequestTelemetry, response interface{}) {
	if !rl.config.EnablePromptLogging {
		return
	}
	attrs := rl.baseAttrs(ctx, rt)
	attrs = append(attrs, slog.String("event", "response_logged"))

	responseJSON, err := json.Marshal(response)
	if err != nil {
		attrs = append(attrs, slog.String("response_error", "failed to marshal response"))
	} else {
		attrs = append(attrs, slog.String("response", string(responseJSON)))
	}
	rl.logger.LogAttrs(ctx, slog.LevelDebug, "response content (debug logging enabled)", attrs...)
}

// LogProviderCall logs details of a provider API call.
func (rl *RequestLogger) LogProviderCall(ctx context.Context, rt *RequestTelemetry, provider string, durationMs float64, err error) {
	attrs := rl.baseAttrs(ctx, rt)
	attrs = append(attrs,
		slog.String("event", "provider_call"),
		slog.String("provider", provider),
		slog.Float64("provider_latency_ms", durationMs),
	)
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		rl.logger.LogAttrs(ctx, slog.LevelWarn, "provider call failed", attrs...)
	} else {
		rl.logger.LogAttrs(ctx, slog.LevelInfo, "provider call completed", attrs...)
	}
}

// baseAttrs returns the standard set of log attributes for a request.
func (rl *RequestLogger) baseAttrs(ctx context.Context, rt *RequestTelemetry) []slog.Attr {
	attrs := []slog.Attr{
		slog.String("request_id", rt.RequestID),
	}

	if rt.Model != "" {
		attrs = append(attrs, slog.String("model", rt.Model))
	}
	if rt.Provider != "" {
		attrs = append(attrs, slog.String("provider", rt.Provider))
	}
	if rt.ContextID != "" {
		attrs = append(attrs, slog.String("context_id", rt.ContextID))
	}
	if rt.InputFormat != "" {
		attrs = append(attrs, slog.String("input_format", rt.InputFormat))
	}

	// Add trace correlation if available.
	span := trace.SpanFromContext(ctx)
	if span != nil {
		sc := span.SpanContext()
		if sc.HasTraceID() {
			attrs = append(attrs, slog.String("trace_id", sc.TraceID().String()))
		}
		if sc.HasSpanID() {
			attrs = append(attrs, slog.String("span_id", sc.SpanID().String()))
		}
	}

	return attrs
}

// PromptLoggingEnabled returns whether prompt logging is enabled for this logger.
func (rl *RequestLogger) PromptLoggingEnabled() bool {
	return rl.config.EnablePromptLogging
}
