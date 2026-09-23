package request

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTestTelemetry creates a RequestTelemetry with in-memory trace and metric exporters.
func newTestTelemetry(t *testing.T, promptLogging bool) (context.Context, *RequestTelemetry, *tracetest.InMemoryExporter, *sdkmetric.ManualReader) {
	t.Helper()
	ResetGlobalMeters()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(spanExporter),
	)

	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(metricReader),
	)

	config := &TelemetryConfig{
		EnablePromptLogging: promptLogging,
		TracerProvider:      tp,
		MeterProvider:       mp,
	}

	ctx, rt := NewRequestTelemetry(context.Background(), "req_test_123", config)
	return ctx, rt, spanExporter, metricReader
}

// collectMetrics reads all metrics from the reader.
func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
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

// --- Span Creation Tests ---

func TestNewRequestTelemetry_CreatesRootSpan(t *testing.T) {
	ctx, rt, exporter, _ := newTestTelemetry(t, false)

	// Finish the telemetry to flush spans.
	rt.Finish(ctx, nil)

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span, got none")
	}

	rootSpan := spans[len(spans)-1]
	if rootSpan.Name != "gateway.request" {
		t.Errorf("expected root span name 'gateway.request', got %q", rootSpan.Name)
	}

	// Check request_id attribute.
	found := false
	for _, attr := range rootSpan.Attributes {
		if string(attr.Key) == "request_id" && attr.Value.AsString() == "req_test_123" {
			found = true
			break
		}
	}
	if !found {
		t.Error("root span missing request_id attribute")
	}
}

func TestStartPhase_CreatesChildSpans(t *testing.T) {
	ctx, rt, exporter, _ := newTestTelemetry(t, false)

	phases := []string{
		PhaseNormalization,
		PhaseProviderCall,
		PhaseCacheLookup,
		PhaseCacheWrite,
		PhasePolicyEvaluation,
		PhaseResponseConstruction,
	}

	for _, phase := range phases {
		phaseCtx, endPhase := rt.StartPhase(ctx, phase)
		_ = phaseCtx
		time.Sleep(1 * time.Millisecond)
		endPhase(nil)
	}

	rt.Finish(ctx, nil)

	spans := exporter.GetSpans()
	// Should have root span + 6 phase spans.
	if len(spans) < 7 {
		t.Fatalf("expected at least 7 spans (root + 6 phases), got %d", len(spans))
	}

	// Verify each phase span exists.
	spanNames := make(map[string]bool)
	for _, s := range spans {
		spanNames[s.Name] = true
	}

	for _, phase := range phases {
		expectedName := "gateway." + phase
		if !spanNames[expectedName] {
			t.Errorf("missing span for phase %q (expected name %q)", phase, expectedName)
		}
	}
}

func TestStartPhase_RecordsPhaseDuration(t *testing.T) {
	ctx, rt, _, _ := newTestTelemetry(t, false)

	_, endPhase := rt.StartPhase(ctx, PhaseNormalization)
	time.Sleep(5 * time.Millisecond)
	endPhase(nil)

	duration := rt.GetPhaseDuration(PhaseNormalization)
	if duration < 5*time.Millisecond {
		t.Errorf("expected phase duration >= 5ms, got %v", duration)
	}
}

func TestStartPhase_RecordsError(t *testing.T) {
	ctx, rt, exporter, _ := newTestTelemetry(t, false)

	testErr := errors.New("provider timeout")
	_, endPhase := rt.StartPhase(ctx, PhaseProviderCall)
	endPhase(testErr)

	rt.Finish(ctx, nil)

	spans := exporter.GetSpans()
	var providerSpan *tracetest.SpanStub
	for i, s := range spans {
		if s.Name == "gateway.provider_call" {
			providerSpan = &spans[i]
			break
		}
	}

	if providerSpan == nil {
		t.Fatal("provider_call span not found")
	}

	if providerSpan.Status.Code != codes.Error {
		t.Errorf("expected error status on span, got %v", providerSpan.Status.Code)
	}
}

func TestSpanAttributes_IncludeModelProviderCacheHit(t *testing.T) {
	ctx, rt, exporter, _ := newTestTelemetry(t, false)

	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	rt.SetCacheHit(true)

	rt.Finish(ctx, nil)

	spans := exporter.GetSpans()
	rootSpan := spans[len(spans)-1]

	attrs := make(map[string]interface{})
	for _, attr := range rootSpan.Attributes {
		switch attr.Value.Type() {
		case attribute.STRING:
			attrs[string(attr.Key)] = attr.Value.AsString()
		case attribute.BOOL:
			attrs[string(attr.Key)] = attr.Value.AsBool()
		}
	}

	if attrs["model"] != "gpt-4o" {
		t.Errorf("expected model=gpt-4o, got %v", attrs["model"])
	}
	if attrs["provider"] != "openai" {
		t.Errorf("expected provider=openai, got %v", attrs["provider"])
	}
	if attrs["cache_hit"] != true {
		t.Errorf("expected cache_hit=true, got %v", attrs["cache_hit"])
	}
}

func TestTraceCorrelation_TraceIDAndSpanID(t *testing.T) {
	ctx, rt, _, _ := newTestTelemetry(t, false)

	traceID := TraceIDFromContext(ctx)
	spanID := SpanIDFromContext(ctx)

	if traceID == "" {
		t.Error("expected non-empty trace ID from context")
	}
	if spanID == "" {
		t.Error("expected non-empty span ID from context")
	}

	rt.Finish(ctx, nil)
}

func TestChildSpan_SharesParentTraceID(t *testing.T) {
	ctx, rt, exporter, _ := newTestTelemetry(t, false)

	phaseCtx, endPhase := rt.StartPhase(ctx, PhaseNormalization)
	_ = phaseCtx
	endPhase(nil)

	rt.Finish(ctx, nil)

	spans := exporter.GetSpans()
	if len(spans) < 2 {
		t.Fatal("expected at least 2 spans")
	}

	// All spans should share the same trace ID.
	traceID := spans[0].SpanContext.TraceID()
	for _, s := range spans {
		if s.SpanContext.TraceID() != traceID {
			t.Errorf("span %q has different trace ID: %s vs %s", s.Name, s.SpanContext.TraceID(), traceID)
		}
	}
}

// --- Metric Recording Tests ---

func TestMetrics_RequestsTotalIncremented(t *testing.T) {
	ctx, rt, _, reader := newTestTelemetry(t, false)

	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	rt.Finish(ctx, nil)

	rm := collectMetrics(t, reader)
	m := findMetricByName(rm, "gateway_requests_total")
	if m == nil {
		t.Fatal("metric gateway_requests_total not found")
	}

	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", m.Data)
	}

	var total int64
	for _, dp := range sum.DataPoints {
		total += dp.Value
	}
	if total != 1 {
		t.Errorf("expected gateway_requests_total=1, got %d", total)
	}
}

func TestMetrics_RequestDurationRecorded(t *testing.T) {
	ctx, rt, _, reader := newTestTelemetry(t, false)

	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	time.Sleep(5 * time.Millisecond)
	rt.Finish(ctx, nil)

	rm := collectMetrics(t, reader)
	m := findMetricByName(rm, "gateway_request_duration_seconds")
	if m == nil {
		t.Fatal("metric gateway_request_duration_seconds not found")
	}

	hist, ok := m.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("expected Histogram[float64], got %T", m.Data)
	}

	if len(hist.DataPoints) == 0 {
		t.Fatal("expected at least one data point for request duration")
	}

	if hist.DataPoints[0].Count == 0 {
		t.Error("expected non-zero count for request duration histogram")
	}
}

func TestMetrics_TokenCountsRecorded(t *testing.T) {
	ctx, rt, _, reader := newTestTelemetry(t, false)

	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	rt.SetTokenCounts(100, 50, 25)
	rt.Finish(ctx, nil)

	rm := collectMetrics(t, reader)

	inputMetric := findMetricByName(rm, "gateway_tokens_input_total")
	if inputMetric == nil {
		t.Fatal("metric gateway_tokens_input_total not found")
	}
	inputSum, _ := inputMetric.Data.(metricdata.Sum[int64])
	var inputTotal int64
	for _, dp := range inputSum.DataPoints {
		inputTotal += dp.Value
	}
	if inputTotal != 100 {
		t.Errorf("expected tokens_input=100, got %d", inputTotal)
	}

	outputMetric := findMetricByName(rm, "gateway_tokens_output_total")
	if outputMetric == nil {
		t.Fatal("metric gateway_tokens_output_total not found")
	}
	outputSum, _ := outputMetric.Data.(metricdata.Sum[int64])
	var outputTotal int64
	for _, dp := range outputSum.DataPoints {
		outputTotal += dp.Value
	}
	if outputTotal != 50 {
		t.Errorf("expected tokens_output=50, got %d", outputTotal)
	}

	cachedMetric := findMetricByName(rm, "gateway_tokens_cached_total")
	if cachedMetric == nil {
		t.Fatal("metric gateway_tokens_cached_total not found")
	}
	cachedSum, _ := cachedMetric.Data.(metricdata.Sum[int64])
	var cachedTotal int64
	for _, dp := range cachedSum.DataPoints {
		cachedTotal += dp.Value
	}
	if cachedTotal != 25 {
		t.Errorf("expected tokens_cached=25, got %d", cachedTotal)
	}
}

func TestMetrics_CacheHitAndMiss(t *testing.T) {
	// Test cache hit.
	ctx, rt, _, reader := newTestTelemetry(t, false)
	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	rt.SetCacheHit(true)
	rt.Finish(ctx, nil)

	rm := collectMetrics(t, reader)
	hitMetric := findMetricByName(rm, "gateway_cache_hits_total")
	if hitMetric == nil {
		t.Fatal("metric gateway_cache_hits_total not found")
	}
	hitSum, _ := hitMetric.Data.(metricdata.Sum[int64])
	var hitTotal int64
	for _, dp := range hitSum.DataPoints {
		hitTotal += dp.Value
	}
	if hitTotal != 1 {
		t.Errorf("expected cache_hits=1, got %d", hitTotal)
	}

	// Test cache miss.
	ResetGlobalMeters()
	ctx2, rt2, _, reader2 := newTestTelemetry(t, false)
	rt2.SetModel("gpt-4o")
	rt2.SetProvider("openai")
	rt2.SetCacheHit(false)
	rt2.Finish(ctx2, nil)

	rm2 := collectMetrics(t, reader2)
	missMetric := findMetricByName(rm2, "gateway_cache_misses_total")
	if missMetric == nil {
		t.Fatal("metric gateway_cache_misses_total not found")
	}
	missSum, _ := missMetric.Data.(metricdata.Sum[int64])
	var missTotal int64
	for _, dp := range missSum.DataPoints {
		missTotal += dp.Value
	}
	if missTotal != 1 {
		t.Errorf("expected cache_misses=1, got %d", missTotal)
	}
}

func TestMetrics_PhaseDurationRecorded(t *testing.T) {
	ctx, rt, _, reader := newTestTelemetry(t, false)

	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")

	_, endPhase := rt.StartPhase(ctx, PhaseNormalization)
	time.Sleep(2 * time.Millisecond)
	endPhase(nil)

	rt.Finish(ctx, nil)

	rm := collectMetrics(t, reader)
	m := findMetricByName(rm, "gateway_phase_duration_seconds")
	if m == nil {
		t.Fatal("metric gateway_phase_duration_seconds not found")
	}

	hist, ok := m.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("expected Histogram[float64], got %T", m.Data)
	}

	if len(hist.DataPoints) == 0 {
		t.Fatal("expected at least one data point for phase duration")
	}
}

func TestMetrics_CostRecorded(t *testing.T) {
	ctx, rt, _, reader := newTestTelemetry(t, false)

	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	rt.SetCost(0.005, 0.003)
	rt.Finish(ctx, nil)

	rm := collectMetrics(t, reader)

	costMetric := findMetricByName(rm, "gateway_estimated_cost_dollars")
	if costMetric == nil {
		t.Fatal("metric gateway_estimated_cost_dollars not found")
	}
	costSum, _ := costMetric.Data.(metricdata.Sum[float64])
	var costTotal float64
	for _, dp := range costSum.DataPoints {
		costTotal += dp.Value
	}
	if costTotal < 0.004 || costTotal > 0.006 {
		t.Errorf("expected estimated_cost~0.005, got %f", costTotal)
	}

	avoidedMetric := findMetricByName(rm, "gateway_cost_avoided_dollars")
	if avoidedMetric == nil {
		t.Fatal("metric gateway_cost_avoided_dollars not found")
	}
	avoidedSum, _ := avoidedMetric.Data.(metricdata.Sum[float64])
	var avoidedTotal float64
	for _, dp := range avoidedSum.DataPoints {
		avoidedTotal += dp.Value
	}
	if avoidedTotal < 0.002 || avoidedTotal > 0.004 {
		t.Errorf("expected cost_avoided~0.003, got %f", avoidedTotal)
	}
}

func TestMetrics_ModelUsageRecorded(t *testing.T) {
	ctx, rt, _, reader := newTestTelemetry(t, false)

	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	rt.Finish(ctx, nil)

	rm := collectMetrics(t, reader)
	m := findMetricByName(rm, "gateway_model_usage_total")
	if m == nil {
		t.Fatal("metric gateway_model_usage_total not found")
	}

	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", m.Data)
	}

	var total int64
	for _, dp := range sum.DataPoints {
		total += dp.Value
	}
	if total != 1 {
		t.Errorf("expected model_usage=1, got %d", total)
	}
}

func TestMetrics_ErrorsRecorded(t *testing.T) {
	ctx, rt, _, reader := newTestTelemetry(t, false)

	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	rt.Finish(ctx, errors.New("test error"))

	rm := collectMetrics(t, reader)
	m := findMetricByName(rm, "gateway_request_errors_total")
	if m == nil {
		t.Fatal("metric gateway_request_errors_total not found")
	}

	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("expected Sum[int64], got %T", m.Data)
	}

	var total int64
	for _, dp := range sum.DataPoints {
		total += dp.Value
	}
	if total != 1 {
		t.Errorf("expected errors=1, got %d", total)
	}
}

// --- Integration Tests ---

func TestIntegration_TraceExport(t *testing.T) {
	ctx, rt, exporter, _ := newTestTelemetry(t, false)

	// Simulate full request lifecycle.
	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	rt.SetInputFormat("openai")

	// Normalization phase.
	_, endNorm := rt.StartPhase(ctx, PhaseNormalization)
	time.Sleep(1 * time.Millisecond)
	endNorm(nil)

	// Cache lookup phase.
	_, endCache := rt.StartPhase(ctx, PhaseCacheLookup)
	time.Sleep(1 * time.Millisecond)
	rt.SetCacheHit(false)
	endCache(nil)

	// Provider call phase.
	_, endProvider := rt.StartPhase(ctx, PhaseProviderCall)
	time.Sleep(2 * time.Millisecond)
	endProvider(nil)

	// Cache write phase.
	_, endWrite := rt.StartPhase(ctx, PhaseCacheWrite)
	time.Sleep(1 * time.Millisecond)
	endWrite(nil)

	// Response construction phase.
	_, endResponse := rt.StartPhase(ctx, PhaseResponseConstruction)
	time.Sleep(1 * time.Millisecond)
	endResponse(nil)

	rt.SetTokenCounts(100, 50, 0)
	rt.SetCost(0.005, 0.0)
	rt.Finish(ctx, nil)

	spans := exporter.GetSpans()
	if len(spans) < 6 {
		t.Fatalf("expected at least 6 spans, got %d", len(spans))
	}

	// Verify trace ID is consistent across all spans.
	traceID := spans[0].SpanContext.TraceID()
	for _, s := range spans {
		if s.SpanContext.TraceID() != traceID {
			t.Errorf("inconsistent trace ID: span %q has %s, expected %s",
				s.Name, s.SpanContext.TraceID(), traceID)
		}
	}

	// Verify root span has all expected attributes.
	rootSpan := spans[len(spans)-1]
	attrMap := make(map[string]interface{})
	for _, attr := range rootSpan.Attributes {
		switch attr.Value.Type() {
		case attribute.STRING:
			attrMap[string(attr.Key)] = attr.Value.AsString()
		case attribute.BOOL:
			attrMap[string(attr.Key)] = attr.Value.AsBool()
		case attribute.INT64:
			attrMap[string(attr.Key)] = attr.Value.AsInt64()
		case attribute.FLOAT64:
			attrMap[string(attr.Key)] = attr.Value.AsFloat64()
		}
	}

	if attrMap["request_id"] != "req_test_123" {
		t.Errorf("root span missing request_id, attrs: %v", attrMap)
	}
	if attrMap["model"] != "gpt-4o" {
		t.Errorf("root span missing model, attrs: %v", attrMap)
	}
	if attrMap["provider"] != "openai" {
		t.Errorf("root span missing provider, attrs: %v", attrMap)
	}
}

func TestIntegration_MetricPresence(t *testing.T) {
	ctx, rt, _, reader := newTestTelemetry(t, false)

	rt.SetModel("gpt-4o-mini")
	rt.SetProvider("openai")
	rt.SetCacheHit(true)
	rt.SetTokenCounts(200, 100, 200)
	rt.SetCost(0.001, 0.009)

	_, endNorm := rt.StartPhase(ctx, PhaseNormalization)
	endNorm(nil)

	rt.Finish(ctx, nil)

	rm := collectMetrics(t, reader)

	expectedMetrics := []string{
		"gateway_requests_total",
		"gateway_request_duration_seconds",
		"gateway_tokens_input_total",
		"gateway_tokens_output_total",
		"gateway_tokens_cached_total",
		"gateway_cache_hits_total",
		"gateway_model_usage_total",
		"gateway_estimated_cost_dollars",
		"gateway_cost_avoided_dollars",
		"gateway_phase_duration_seconds",
	}

	for _, name := range expectedMetrics {
		m := findMetricByName(rm, name)
		if m == nil {
			t.Errorf("expected metric %q to be present", name)
		}
	}
}

// --- Logging Tests ---

func TestLogging_PromptsNotLoggedByDefault(t *testing.T) {
	ResetGlobalMeters()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := DefaultTelemetryConfig()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	config.TracerProvider = tp
	config.MeterProvider = mp

	ctx, rt := NewRequestTelemetry(context.Background(), "req_log_test", config)
	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")

	rl := NewRequestLogger(logger, config)

	// Log start and complete events.
	rl.LogRequestStart(ctx, rt)
	rl.LogRequestComplete(ctx, rt, nil)

	// Attempt to log prompts — should be suppressed.
	rl.LogPrompt(ctx, rt, map[string]string{"role": "user", "content": "secret prompt"})
	rl.LogResponse(ctx, rt, map[string]string{"content": "secret response"})

	rt.Finish(ctx, nil)

	output := buf.String()

	// Verify prompts and responses are NOT in the output.
	if strings.Contains(output, "secret prompt") {
		t.Error("prompt content was logged when prompt logging is disabled")
	}
	if strings.Contains(output, "secret response") {
		t.Error("response content was logged when prompt logging is disabled")
	}

	// Verify metadata IS in the output.
	if !strings.Contains(output, "req_log_test") {
		t.Error("request_id not found in log output")
	}
	if !strings.Contains(output, "gpt-4o") {
		t.Error("model not found in log output")
	}
	if !strings.Contains(output, "openai") {
		t.Error("provider not found in log output")
	}
}

func TestLogging_PromptsLoggedWhenEnabled(t *testing.T) {
	ResetGlobalMeters()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := &TelemetryConfig{
		EnablePromptLogging: true,
	}

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	config.TracerProvider = tp
	config.MeterProvider = mp

	ctx, rt := NewRequestTelemetry(context.Background(), "req_log_prompt", config)
	rt.SetModel("gpt-4o")

	rl := NewRequestLogger(logger, config)

	promptData := map[string]string{"role": "user", "content": "debug prompt"}
	responseData := map[string]string{"content": "debug response"}

	rl.LogPrompt(ctx, rt, promptData)
	rl.LogResponse(ctx, rt, responseData)

	rt.Finish(ctx, nil)

	output := buf.String()

	if !strings.Contains(output, "debug prompt") {
		t.Error("prompt content was NOT logged when prompt logging is enabled")
	}
	if !strings.Contains(output, "debug response") {
		t.Error("response content was NOT logged when prompt logging is enabled")
	}
}

func TestLogging_StructuredFields(t *testing.T) {
	ResetGlobalMeters()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := DefaultTelemetryConfig()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	config.TracerProvider = tp
	config.MeterProvider = mp

	ctx, rt := NewRequestTelemetry(context.Background(), "req_struct_test", config)
	rt.SetModel("gemini-1.5-pro")
	rt.SetProvider("gemini")
	rt.SetContextID("ctx_abc123")
	rt.SetCacheHit(true)
	rt.SetTokenCounts(500, 250, 500)
	rt.SetCost(0.01, 0.008)

	rl := NewRequestLogger(logger, config)
	rl.LogRequestStart(ctx, rt)

	time.Sleep(2 * time.Millisecond)
	rt.Finish(ctx, nil)
	rl.LogRequestComplete(ctx, rt, nil)

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Parse the completion log line and check fields.
	var completionLog map[string]interface{}
	for _, line := range lines {
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if msg, ok := entry["msg"]; ok && msg == "request processing completed" {
			completionLog = entry
			break
		}
	}

	if completionLog == nil {
		t.Fatal("could not find 'request processing completed' log entry")
	}

	expectedFields := map[string]interface{}{
		"request_id":     "req_struct_test",
		"model":          "gemini-1.5-pro",
		"provider":       "gemini",
		"context_id":     "ctx_abc123",
		"cache_hit":      true,
		"tokens_input":   float64(500),
		"tokens_output":  float64(250),
		"tokens_cached":  float64(500),
		"estimated_cost": float64(0.01),
		"cost_avoided":   float64(0.008),
	}

	for field, expected := range expectedFields {
		actual, ok := completionLog[field]
		if !ok {
			t.Errorf("missing field %q in log output", field)
			continue
		}
		switch exp := expected.(type) {
		case string:
			if actual != exp {
				t.Errorf("field %q: expected %q, got %v", field, exp, actual)
			}
		case bool:
			if actual != exp {
				t.Errorf("field %q: expected %v, got %v", field, exp, actual)
			}
		case float64:
			actualFloat, ok := actual.(float64)
			if !ok {
				t.Errorf("field %q: expected float64, got %T", field, actual)
				continue
			}
			if actualFloat != exp {
				t.Errorf("field %q: expected %v, got %v", field, exp, actualFloat)
			}
		}
	}

	// Verify latency_ms is present and > 0.
	if latency, ok := completionLog["latency_ms"]; ok {
		if l, ok := latency.(float64); ok {
			if l <= 0 {
				t.Errorf("expected latency_ms > 0, got %v", l)
			}
		}
	} else {
		t.Error("missing latency_ms in log output")
	}
}

func TestLogging_TraceCorrelation(t *testing.T) {
	ResetGlobalMeters()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := DefaultTelemetryConfig()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	config.TracerProvider = tp
	config.MeterProvider = mp

	ctx, rt := NewRequestTelemetry(context.Background(), "req_trace_corr", config)
	rt.SetModel("gpt-4o")

	rl := NewRequestLogger(logger, config)
	rl.LogRequestStart(ctx, rt)

	rt.Finish(ctx, nil)

	output := buf.String()

	// Verify trace_id is present in log output.
	if !strings.Contains(output, "trace_id") {
		t.Error("trace_id not found in log output")
	}
	if !strings.Contains(output, "span_id") {
		t.Error("span_id not found in log output")
	}
}

func TestLogging_PhaseEvents(t *testing.T) {
	ResetGlobalMeters()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := DefaultTelemetryConfig()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	config.TracerProvider = tp
	config.MeterProvider = mp

	ctx, rt := NewRequestTelemetry(context.Background(), "req_phase_log", config)
	rt.SetModel("gpt-4o")

	rl := NewRequestLogger(logger, config)
	rl.LogPhaseStart(ctx, rt, PhaseNormalization)
	rl.LogPhaseComplete(ctx, rt, PhaseNormalization, 5.0, nil)

	testErr := errors.New("policy denied")
	rl.LogPhaseComplete(ctx, rt, PhasePolicyEvaluation, 2.0, testErr)

	rt.Finish(ctx, nil)

	output := buf.String()
	if !strings.Contains(output, "phase_start") {
		t.Error("phase_start event not found in log output")
	}
	if !strings.Contains(output, "phase_complete") {
		t.Error("phase_complete event not found in log output")
	}
	if !strings.Contains(output, "policy denied") {
		t.Error("phase error not found in log output")
	}
}

// --- Telemetry Config Tests ---

func TestDefaultTelemetryConfig(t *testing.T) {
	config := DefaultTelemetryConfig()
	if config.EnablePromptLogging {
		t.Error("expected EnablePromptLogging to be false by default")
	}
	if config.TracerProvider != nil {
		t.Error("expected TracerProvider to be nil by default")
	}
	if config.MeterProvider != nil {
		t.Error("expected MeterProvider to be nil by default")
	}
}

func TestRequestTelemetry_GetPhases(t *testing.T) {
	ctx, rt, _, _ := newTestTelemetry(t, false)

	_, endNorm := rt.StartPhase(ctx, PhaseNormalization)
	endNorm(nil)

	_, endProvider := rt.StartPhase(ctx, PhaseProviderCall)
	endProvider(nil)

	phases := rt.GetPhases()
	if len(phases) != 2 {
		t.Errorf("expected 2 phases, got %d", len(phases))
	}

	rt.Finish(ctx, nil)
}

func TestRequestTelemetry_GetTotalDuration(t *testing.T) {
	ctx, rt, _, _ := newTestTelemetry(t, false)

	time.Sleep(5 * time.Millisecond)
	rt.Finish(ctx, nil)

	duration := rt.GetTotalDuration()
	if duration < 5*time.Millisecond {
		t.Errorf("expected total duration >= 5ms, got %v", duration)
	}
}

func TestRequestTelemetry_NilConfig(t *testing.T) {
	ResetGlobalMeters()

	// NewRequestTelemetry with nil config should use defaults.
	ctx, rt := NewRequestTelemetry(context.Background(), "req_nil_config", nil)

	// Should not panic.
	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")
	rt.Finish(ctx, nil)

	if rt.PromptLoggingEnabled() {
		t.Error("expected prompt logging to be disabled with nil config")
	}
}

// --- Span Helper Tests ---

func TestAddSpanEvent(t *testing.T) {
	ctx, rt, exporter, _ := newTestTelemetry(t, false)

	AddSpanEvent(ctx, "test_event",
		attribute.String("key", "value"),
	)

	rt.Finish(ctx, nil)

	spans := exporter.GetSpans()
	rootSpan := spans[len(spans)-1]

	found := false
	for _, event := range rootSpan.Events {
		if event.Name == "test_event" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected test_event to be recorded on span")
	}
}

func TestSetSpanAttributes(t *testing.T) {
	ctx, rt, exporter, _ := newTestTelemetry(t, false)

	SetSpanAttributes(ctx,
		attribute.String("custom_key", "custom_value"),
	)

	rt.Finish(ctx, nil)

	spans := exporter.GetSpans()
	rootSpan := spans[len(spans)-1]

	found := false
	for _, attr := range rootSpan.Attributes {
		if string(attr.Key) == "custom_key" && attr.Value.AsString() == "custom_value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected custom_key attribute on span")
	}
}

func TestStartCustomSpan(t *testing.T) {
	ctx, rt, exporter, _ := newTestTelemetry(t, false)

	childCtx, endSpan := StartCustomSpan(ctx, rt.Tracer(), "custom.operation",
		attribute.String("operation", "test"),
	)
	_ = childCtx
	endSpan(nil)

	rt.Finish(ctx, nil)

	spans := exporter.GetSpans()
	found := false
	for _, s := range spans {
		if s.Name == "custom.operation" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected custom.operation span")
	}
}

func TestRequestLogger_NilLogger(t *testing.T) {
	ResetGlobalMeters()

	// Should use slog.Default() and not panic.
	rl := NewRequestLogger(nil, nil)
	if rl.PromptLoggingEnabled() {
		t.Error("expected prompt logging disabled with nil config")
	}
}

func TestLogging_ProviderCall(t *testing.T) {
	ResetGlobalMeters()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := DefaultTelemetryConfig()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	config.TracerProvider = tp
	config.MeterProvider = mp

	ctx, rt := NewRequestTelemetry(context.Background(), "req_prov_log", config)
	rt.SetModel("gpt-4o")
	rt.SetProvider("openai")

	rl := NewRequestLogger(logger, config)
	rl.LogProviderCall(ctx, rt, "openai", 150.5, nil)

	rt.Finish(ctx, nil)

	output := buf.String()
	if !strings.Contains(output, "provider_call") {
		t.Error("provider_call event not found in log output")
	}
	if !strings.Contains(output, "provider_latency_ms") {
		t.Error("provider_latency_ms not found in log output")
	}
}

func TestLogging_CacheResult(t *testing.T) {
	ResetGlobalMeters()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	config := DefaultTelemetryConfig()

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))
	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	config.TracerProvider = tp
	config.MeterProvider = mp

	ctx, rt := NewRequestTelemetry(context.Background(), "req_cache_log", config)
	rt.SetModel("gpt-4o")

	rl := NewRequestLogger(logger, config)
	rl.LogCacheResult(ctx, rt, true)

	rt.Finish(ctx, nil)

	output := buf.String()
	if !strings.Contains(output, "cache_result") {
		t.Error("cache_result event not found in log output")
	}
	if !strings.Contains(output, "cache_hit") {
		t.Error("cache_hit not found in log output")
	}
}
