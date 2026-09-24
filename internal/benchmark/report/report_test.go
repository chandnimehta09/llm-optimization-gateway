package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/benchmark"
)

func TestNewGenerator(t *testing.T) {
	g := NewGenerator()
	if g == nil {
		t.Error("expected non-nil generator")
	}
}

func TestGenerator_Generate_NilResults(t *testing.T) {
	g := NewGenerator()
	_, err := g.Generate(nil)
	if err == nil {
		t.Error("expected error for nil results")
	}
}

func TestGenerator_Generate_Success(t *testing.T) {
	g := NewGenerator()
	results := makeTestResults()

	rpt, err := g.Generate(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rpt.Name != "test" {
		t.Errorf("expected name 'test', got %q", rpt.Name)
	}
	if len(rpt.Workloads) != 1 {
		t.Errorf("expected 1 workload, got %d", len(rpt.Workloads))
	}
	if rpt.GeneratedAt.IsZero() {
		t.Error("generated_at should be set")
	}
}

func TestGenerator_Generate_Metrics(t *testing.T) {
	g := NewGenerator()
	results := makeTestResults()

	rpt, err := g.Generate(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w := rpt.Workloads[0]
	if w.Baseline.Count != 3 {
		t.Errorf("expected 3 baseline requests, got %d", w.Baseline.Count)
	}
	if w.Gateway.Count != 3 {
		t.Errorf("expected 3 gateway requests, got %d", w.Gateway.Count)
	}
	if w.Baseline.MeanLatency <= 0 {
		t.Error("baseline mean latency should be positive")
	}
	if w.Gateway.MeanLatency <= 0 {
		t.Error("gateway mean latency should be positive")
	}
}

func TestGenerator_Generate_CacheMetrics(t *testing.T) {
	g := NewGenerator()
	results := makeTestResults()

	rpt, err := g.Generate(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w := rpt.Workloads[0]
	if w.Gateway.TotalCachedTokens != 240 { // 80 * 3
		t.Errorf("expected 240 cached tokens, got %d", w.Gateway.TotalCachedTokens)
	}
	if w.Gateway.CacheHitRate <= 0 {
		t.Error("gateway cache hit rate should be positive")
	}
}

func TestGenerator_Generate_OverallMetrics(t *testing.T) {
	g := NewGenerator()
	results := makeTestResults()

	rpt, err := g.Generate(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rpt.OverallBaseline.Count != 3 {
		t.Errorf("expected 3 overall baseline requests, got %d", rpt.OverallBaseline.Count)
	}
	if rpt.OverallGateway.Count != 3 {
		t.Errorf("expected 3 overall gateway requests, got %d", rpt.OverallGateway.Count)
	}
}

func TestFormatText(t *testing.T) {
	g := NewGenerator()
	results := makeTestResults()
	rpt, _ := g.Generate(results)

	text := FormatText(rpt)
	if text == "" {
		t.Error("expected non-empty text report")
	}
	if !contains(text, "BENCHMARK REPORT") {
		t.Error("missing BENCHMARK REPORT header")
	}
	if !contains(text, "OVERALL SUMMARY") {
		t.Error("missing OVERALL SUMMARY section")
	}
}

func TestFormatMarkdown(t *testing.T) {
	g := NewGenerator()
	results := makeTestResults()
	rpt, _ := g.Generate(results)

	md := FormatMarkdown(rpt)
	if md == "" {
		t.Error("expected non-empty markdown report")
	}
	if !contains(md, "# Benchmark Report") {
		t.Error("missing markdown header")
	}
	if !contains(md, "## Overall Summary") {
		t.Error("missing overall summary section")
	}
}

func TestLatencyChart(t *testing.T) {
	g := NewGenerator()
	results := makeTestResults()
	rpt, _ := g.Generate(results)

	chart := LatencyChart(rpt)
	if chart == "" {
		t.Error("expected non-empty latency chart")
	}
	if !contains(chart, "Mean Latency") {
		t.Error("missing chart title")
	}
}

func TestCostChart(t *testing.T) {
	g := NewGenerator()
	results := makeTestResults()
	rpt, _ := g.Generate(results)

	chart := CostChart(rpt)
	if chart == "" {
		t.Error("expected non-empty cost chart")
	}
}

func TestImprovementChart(t *testing.T) {
	g := NewGenerator()
	results := makeTestResults()
	rpt, _ := g.Generate(results)

	chart := ImprovementChart(rpt)
	if chart == "" {
		t.Error("expected non-empty improvement chart")
	}
	if !contains(chart, "Improvement by Workload") {
		t.Error("missing chart title")
	}
}

func TestExporter_ExportAll(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGenerator()
	results := makeTestResults()
	rpt, _ := g.Generate(results)

	exporter := NewExporter(tmpDir)
	if err := exporter.ExportAll(rpt, results); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check all files were created.
	expectedFiles := []string{"report.json", "report.txt", "report.csv", "raw_results.json"}
	for _, f := range expectedFiles {
		path := filepath.Join(tmpDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", f)
		}
	}
}

func TestExporter_ExportJSON(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGenerator()
	results := makeTestResults()
	rpt, _ := g.Generate(results)

	exporter := NewExporter(tmpDir)
	if err := exporter.ExportJSON(rpt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "report.json"))
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}

func TestExporter_ExportCSV(t *testing.T) {
	tmpDir := t.TempDir()
	g := NewGenerator()
	results := makeTestResults()
	rpt, _ := g.Generate(results)

	exporter := NewExporter(tmpDir)
	if err := exporter.ExportCSV(rpt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "report.csv"))
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty CSV")
	}
	if !contains(string(data), "workload_type") {
		t.Error("CSV should contain header row")
	}
}

func TestComputeMetrics_Empty(t *testing.T) {
	m := computeMetrics(nil)
	if m.Count != 0 {
		t.Error("expected 0 count for empty results")
	}
}

func TestComputeMetrics_ErrorCounting(t *testing.T) {
	results := []benchmark.RequestResult{
		{Latency: 10 * time.Millisecond, StatusCode: 200},
		{Latency: 20 * time.Millisecond, StatusCode: 500, Error: "server error"},
		{Latency: 30 * time.Millisecond, StatusCode: 200},
	}

	m := computeMetrics(results)
	if m.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", m.SuccessCount)
	}
	if m.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", m.ErrorCount)
	}
}

func TestComputeMetrics_Percentiles(t *testing.T) {
	results := make([]benchmark.RequestResult, 100)
	for i := 0; i < 100; i++ {
		results[i] = benchmark.RequestResult{
			Latency: time.Duration(i+1) * time.Millisecond,
		}
	}

	m := computeMetrics(results)

	if m.MinLatency != 1*time.Millisecond {
		t.Errorf("expected min 1ms, got %v", m.MinLatency)
	}
	if m.MaxLatency != 100*time.Millisecond {
		t.Errorf("expected max 100ms, got %v", m.MaxLatency)
	}
	if m.MedianLatency != 50*time.Millisecond {
		t.Errorf("expected median 50ms, got %v", m.MedianLatency)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Microsecond, "500µs"},
		{1500 * time.Microsecond, "1.5ms"},
		{1500 * time.Millisecond, "1.50s"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestSuccessRate(t *testing.T) {
	m := SummaryMetrics{Count: 10, SuccessCount: 8}
	rate := successRate(m)
	if rate != 80.0 {
		t.Errorf("expected 80%%, got %.1f%%", rate)
	}
}

func TestSuccessRate_ZeroCount(t *testing.T) {
	m := SummaryMetrics{Count: 0}
	rate := successRate(m)
	if rate != 0 {
		t.Errorf("expected 0%%, got %.1f%%", rate)
	}
}

// Helpers.

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func makeTestResults() *benchmark.BenchmarkResults {
	return &benchmark.BenchmarkResults{
		Config: &benchmark.Config{
			Name:  "test",
			Model: "gpt-4",
		},
		StartTime: time.Now().Add(-5 * time.Second),
		EndTime:   time.Now(),
		Duration:  5 * time.Second,
		WorkloadResults: []benchmark.WorkloadResults{
			{
				WorkloadType: "repetitive",
				BaselineResults: []benchmark.RequestResult{
					{Latency: 100 * time.Millisecond, InputTokens: 100, OutputTokens: 50, Cost: 0.006, StatusCode: 200},
					{Latency: 110 * time.Millisecond, InputTokens: 100, OutputTokens: 50, Cost: 0.006, StatusCode: 200},
					{Latency: 90 * time.Millisecond, InputTokens: 100, OutputTokens: 50, Cost: 0.006, StatusCode: 200},
				},
				GatewayResults: []benchmark.RequestResult{
					{Latency: 50 * time.Millisecond, InputTokens: 100, OutputTokens: 50, CachedTokens: 80, Cost: 0.003, StatusCode: 200},
					{Latency: 55 * time.Millisecond, InputTokens: 100, OutputTokens: 50, CachedTokens: 80, Cost: 0.003, StatusCode: 200},
					{Latency: 45 * time.Millisecond, InputTokens: 100, OutputTokens: 50, CachedTokens: 80, Cost: 0.003, StatusCode: 200},
				},
			},
		},
	}
}
