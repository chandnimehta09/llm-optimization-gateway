package telemetry

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestInitMetrics_AllMetricsRegistered(t *testing.T) {
	cfg := Config{ServiceName: "test-service"}
	m, handler, err := InitMetrics(cfg)
	if err != nil {
		t.Fatalf("InitMetrics() error = %v", err)
	}
	if handler == nil {
		t.Fatal("InitMetrics() returned nil handler")
	}
	if m == nil {
		t.Fatal("InitMetrics() returned nil metrics")
	}

	// Increment counters and set gauge to ensure they appear in Gather.
	m.RequestsTotal.Inc()
	m.CacheHitsTotal.Inc()
	m.CacheMissesTotal.Inc()
	m.ActiveRequests.Set(1)
	m.ProviderCallsTotal.WithLabelValues("openai").Inc()
	m.ProviderErrorsTotal.WithLabelValues("openai", "timeout").Inc()
	m.RequestDuration.Observe(0.5)

	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	expected := map[string]dto.MetricType{
		"gateway_requests_total":           dto.MetricType_COUNTER,
		"gateway_request_duration_seconds": dto.MetricType_HISTOGRAM,
		"gateway_active_requests":          dto.MetricType_GAUGE,
		"gateway_provider_calls_total":     dto.MetricType_COUNTER,
		"gateway_provider_errors_total":    dto.MetricType_COUNTER,
		"gateway_cache_hits_total":         dto.MetricType_COUNTER,
		"gateway_cache_misses_total":       dto.MetricType_COUNTER,
	}

	found := make(map[string]bool)
	for _, f := range families {
		found[f.GetName()] = true
		if expType, ok := expected[f.GetName()]; ok {
			if f.GetType() != expType {
				t.Errorf("metric %s: expected type %v, got %v", f.GetName(), expType, f.GetType())
			}
		}
	}

	for name := range expected {
		if !found[name] {
			t.Errorf("expected metric %q not found in gathered metrics", name)
		}
	}
}

func TestInitMetrics_CounterIncrements(t *testing.T) {
	cfg := Config{ServiceName: "test-service"}
	m, _, err := InitMetrics(cfg)
	if err != nil {
		t.Fatalf("InitMetrics() error = %v", err)
	}

	m.RequestsTotal.Inc()
	m.RequestsTotal.Inc()
	m.RequestsTotal.Inc()

	val := counterValue(t, m.RequestsTotal)
	if val != 3 {
		t.Errorf("RequestsTotal = %v, want 3", val)
	}
}

func TestInitMetrics_ProviderLabels(t *testing.T) {
	cfg := Config{ServiceName: "test-service"}
	m, _, err := InitMetrics(cfg)
	if err != nil {
		t.Fatalf("InitMetrics() error = %v", err)
	}

	m.ProviderCallsTotal.WithLabelValues("openai").Inc()
	m.ProviderCallsTotal.WithLabelValues("anthropic").Inc()
	m.ProviderCallsTotal.WithLabelValues("openai").Inc()

	openaiVal := counterVecValue(t, m.ProviderCallsTotal, "openai")
	if openaiVal != 2 {
		t.Errorf("ProviderCallsTotal{provider=openai} = %v, want 2", openaiVal)
	}

	anthropicVal := counterVecValue(t, m.ProviderCallsTotal, "anthropic")
	if anthropicVal != 1 {
		t.Errorf("ProviderCallsTotal{provider=anthropic} = %v, want 1", anthropicVal)
	}
}

func TestInitMetrics_ProviderErrorLabels(t *testing.T) {
	cfg := Config{ServiceName: "test-service"}
	m, _, err := InitMetrics(cfg)
	if err != nil {
		t.Fatalf("InitMetrics() error = %v", err)
	}

	m.ProviderErrorsTotal.WithLabelValues("openai", "timeout").Inc()
	m.ProviderErrorsTotal.WithLabelValues("openai", "rate_limit").Inc()
	m.ProviderErrorsTotal.WithLabelValues("openai", "timeout").Inc()

	timeoutVal := counterVecValueMulti(t, m.ProviderErrorsTotal, "openai", "timeout")
	if timeoutVal != 2 {
		t.Errorf("ProviderErrorsTotal{openai,timeout} = %v, want 2", timeoutVal)
	}

	rateLimitVal := counterVecValueMulti(t, m.ProviderErrorsTotal, "openai", "rate_limit")
	if rateLimitVal != 1 {
		t.Errorf("ProviderErrorsTotal{openai,rate_limit} = %v, want 1", rateLimitVal)
	}
}

func TestInitMetrics_HistogramObservation(t *testing.T) {
	cfg := Config{ServiceName: "test-service"}
	m, _, err := InitMetrics(cfg)
	if err != nil {
		t.Fatalf("InitMetrics() error = %v", err)
	}

	m.RequestDuration.Observe(0.1)
	m.RequestDuration.Observe(0.5)
	m.RequestDuration.Observe(1.0)

	var metric dto.Metric
	if err := m.RequestDuration.(prometheus.Metric).Write(&metric); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if metric.GetHistogram().GetSampleCount() != 3 {
		t.Errorf("RequestDuration sample count = %v, want 3", metric.GetHistogram().GetSampleCount())
	}
}

func TestInitMetrics_GaugeSetAndGet(t *testing.T) {
	cfg := Config{ServiceName: "test-service"}
	m, _, err := InitMetrics(cfg)
	if err != nil {
		t.Fatalf("InitMetrics() error = %v", err)
	}

	m.ActiveRequests.Set(5)
	var metric dto.Metric
	if err := m.ActiveRequests.(prometheus.Metric).Write(&metric); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if metric.GetGauge().GetValue() != 5 {
		t.Errorf("ActiveRequests = %v, want 5", metric.GetGauge().GetValue())
	}

	m.ActiveRequests.Inc()
	if err := m.ActiveRequests.(prometheus.Metric).Write(&metric); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if metric.GetGauge().GetValue() != 6 {
		t.Errorf("ActiveRequests after Inc = %v, want 6", metric.GetGauge().GetValue())
	}
}

func TestInitMetrics_Registry(t *testing.T) {
	cfg := Config{ServiceName: "test-service"}
	m, _, err := InitMetrics(cfg)
	if err != nil {
		t.Fatalf("InitMetrics() error = %v", err)
	}
	if m.Registry() == nil {
		t.Error("Registry() returned nil")
	}
}

// Helper functions

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	var metric dto.Metric
	if err := c.(prometheus.Metric).Write(&metric); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	return metric.GetCounter().GetValue()
}

func counterVecValue(t *testing.T, cv *prometheus.CounterVec, labelVal string) float64 {
	t.Helper()
	var metric dto.Metric
	c := cv.WithLabelValues(labelVal)
	if err := c.(prometheus.Metric).Write(&metric); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	return metric.GetCounter().GetValue()
}

func counterVecValueMulti(t *testing.T, cv *prometheus.CounterVec, labelVals ...string) float64 {
	t.Helper()
	var metric dto.Metric
	c := cv.WithLabelValues(labelVals...)
	if err := c.(prometheus.Metric).Write(&metric); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	return metric.GetCounter().GetValue()
}
