package request

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Phase name constants for span creation.
const (
	PhaseNormalization        = "normalization"
	PhaseProviderCall         = "provider_call"
	PhaseCacheLookup          = "cache_lookup"
	PhaseCacheWrite           = "cache_write"
	PhasePolicyEvaluation     = "policy_evaluation"
	PhaseResponseConstruction = "response_construction"
)

// startPhaseSpan creates a child span for a specific request processing phase.
func startPhaseSpan(ctx context.Context, tracer trace.Tracer, phase string, requestID string) (context.Context, trace.Span) {
	spanName := fmt.Sprintf("gateway.%s", phase)
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("request_id", requestID),
			attribute.String("phase", phase),
		),
	)
	return ctx, span
}

// endPhaseSpan ends a phase span, recording any error that occurred.
func endPhaseSpan(span trace.Span, phase string, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, fmt.Sprintf("%s failed: %s", phase, err.Error()))
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End()
}

// AddSpanEvent adds a named event with attributes to the current span in context.
func AddSpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetSpanAttributes sets attributes on the current span in context.
func SetSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(attrs...)
}

// SpanFromContext returns the span from the given context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// TraceIDFromContext extracts the trace ID from the current span context.
// Returns an empty string if no active span exists.
func TraceIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	sc := span.SpanContext()
	if !sc.HasTraceID() {
		return ""
	}
	return sc.TraceID().String()
}

// SpanIDFromContext extracts the span ID from the current span context.
// Returns an empty string if no active span exists.
func SpanIDFromContext(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span == nil {
		return ""
	}
	sc := span.SpanContext()
	if !sc.HasSpanID() {
		return ""
	}
	return sc.SpanID().String()
}

// StartCustomSpan creates a new child span with the given name and attributes.
// The caller must call the returned function to end the span.
func StartCustomSpan(ctx context.Context, tracer trace.Tracer, name string, attrs ...attribute.KeyValue) (context.Context, func(error)) {
	ctx, span := tracer.Start(ctx, name,
		trace.WithAttributes(attrs...),
	)
	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.End()
	}
}
