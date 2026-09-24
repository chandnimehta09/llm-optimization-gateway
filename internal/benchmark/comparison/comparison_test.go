package comparison

import (
	"math"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/benchmark/report"
)

func TestNewEngine(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Error("expected non-nil engine")
	}
}

func TestEngine_Compare_NilReport(t *testing.T) {
	e := NewEngine()
	_, err := e.Compare(nil)
	if err == nil {
		t.Error("expected error for nil report")
	}
}

func TestEngine_Compare_Success(t *testing.T) {
	e := NewEngine()
	r := makeTestReport()

	result, err := e.Compare(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
	if len(result.PerWorkload) != 2 {
		t.Errorf("expected 2 per-workload comparisons, got %d", len(result.PerWorkload))
	}
	if result.Summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestEngine_Compare_LatencyImprovement(t *testing.T) {
	e := NewEngine()
	r := makeTestReport()

	result, err := e.Compare(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Gateway should show improvement (lower latency).
	if !result.Overall.LatencyAnalysis.Improved {
		t.Error("expected latency improvement")
	}
	if result.Overall.LatencyAnalysis.PercentChange >= 0 {
		t.Error("expected negative percent change for latency improvement")
	}
}

func TestEngine_Compare_CostSavings(t *testing.T) {
	e := NewEngine()
	r := makeTestReport()

	result, err := e.Compare(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Overall.CostAnalysis.Improved {
		t.Error("expected cost improvement")
	}
}

func TestEngine_Compare_Recommendations(t *testing.T) {
	e := NewEngine()
	r := makeTestReport()

	result, err := e.Compare(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Recommendations) == 0 {
		t.Error("expected at least one recommendation")
	}

	// Check that recommendations have required fields.
	for i, rec := range result.Recommendations {
		if rec.Category == "" {
			t.Errorf("recommendation %d: missing category", i)
		}
		if rec.Priority == "" {
			t.Errorf("recommendation %d: missing priority", i)
		}
		if rec.Message == "" {
			t.Errorf("recommendation %d: missing message", i)
		}
	}
}

func TestEngine_Compare_PerWorkloadTypes(t *testing.T) {
	e := NewEngine()
	r := makeTestReport()

	result, err := e.Compare(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTypes := map[string]bool{"repetitive": false, "random": false}
	for _, pw := range result.PerWorkload {
		if _, ok := expectedTypes[pw.WorkloadType]; ok {
			expectedTypes[pw.WorkloadType] = true
		}
	}
	for k, found := range expectedTypes {
		if !found {
			t.Errorf("expected workload type %q in per-workload comparisons", k)
		}
	}
}

func TestEngine_Compare_NoImprovement(t *testing.T) {
	e := NewEngine()
	r := &report.Report{
		OverallBaseline: report.SummaryMetrics{
			Count:       10,
			MeanLatency: 50 * time.Millisecond,
			TotalCost:   0.001,
		},
		OverallGateway: report.SummaryMetrics{
			Count:       10,
			MeanLatency: 100 * time.Millisecond,
			TotalCost:   0.002,
		},
	}

	result, err := e.Compare(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Overall.LatencyAnalysis.Improved {
		t.Error("should not show latency improvement")
	}
	if result.Overall.CostAnalysis.Improved {
		t.Error("should not show cost improvement")
	}
}

func TestMetricComparison_PercentChange(t *testing.T) {
	mc := newMetricComparison(100.0, 80.0, true)

	if !mc.Improved {
		t.Error("expected improvement when gateway < baseline")
	}
	if mc.PercentChange != -20.0 {
		t.Errorf("expected -20%% change, got %.1f%%", mc.PercentChange)
	}
	if mc.AbsoluteChange != -20.0 {
		t.Errorf("expected -20 absolute change, got %.1f", mc.AbsoluteChange)
	}
}

func TestMetricComparison_ZeroBaseline(t *testing.T) {
	mc := newMetricComparison(0.0, 50.0, true)
	if mc.PercentChange != 0 {
		t.Errorf("expected 0%% change for zero baseline, got %.1f%%", mc.PercentChange)
	}
}

func TestAnalysisReport_Format(t *testing.T) {
	e := NewEngine()
	r := makeTestReport()
	result, _ := e.Compare(r)

	report := NewAnalysisReport(result)
	text := report.Format()

	if text == "" {
		t.Error("expected non-empty analysis report")
	}
	// Check for expected sections.
	if !containsStr(text, "COMPARISON ANALYSIS") {
		t.Error("missing COMPARISON ANALYSIS header")
	}
	if !containsStr(text, "RECOMMENDATIONS") {
		t.Error("missing RECOMMENDATIONS section")
	}
	if !containsStr(text, "SUMMARY") {
		t.Error("missing SUMMARY section")
	}
}

func TestCostBreakdown(t *testing.T) {
	e := NewEngine()
	r := makeTestReport()
	result, _ := e.Compare(r)

	text := CostBreakdown(result)
	if text == "" {
		t.Error("expected non-empty cost breakdown")
	}
	if !containsStr(text, "COST BREAKDOWN") {
		t.Error("missing COST BREAKDOWN header")
	}
}

func TestMean(t *testing.T) {
	tests := []struct {
		sample []float64
		want   float64
	}{
		{nil, 0},
		{[]float64{}, 0},
		{[]float64{5}, 5},
		{[]float64{1, 2, 3, 4, 5}, 3},
		{[]float64{10, 20}, 15},
	}

	for _, tt := range tests {
		got := Mean(tt.sample)
		if got != tt.want {
			t.Errorf("Mean(%v) = %f, want %f", tt.sample, got, tt.want)
		}
	}
}

func TestVariance(t *testing.T) {
	tests := []struct {
		sample []float64
		want   float64
	}{
		{nil, 0},
		{[]float64{5}, 0},
		{[]float64{2, 4, 4, 4, 5, 5, 7, 9}, 4.571428571428571},
	}

	for _, tt := range tests {
		got := Variance(tt.sample)
		if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("Variance(%v) = %f, want %f", tt.sample, got, tt.want)
		}
	}
}

func TestStdDev(t *testing.T) {
	sample := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	got := StdDev(sample)
	want := math.Sqrt(4.571428571428571)
	if math.Abs(got-want) > 0.0001 {
		t.Errorf("StdDev = %f, want %f", got, want)
	}
}

func TestPercentile(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	tests := []struct {
		p    float64
		want float64
	}{
		{0.0, 1.0},
		{0.5, 5.5},
		{1.0, 10.0},
		{0.25, 3.25},
	}

	for _, tt := range tests {
		got := Percentile(sorted, tt.p)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("Percentile(p=%.2f) = %f, want %f", tt.p, got, tt.want)
		}
	}
}

func TestPercentile_Empty(t *testing.T) {
	got := Percentile(nil, 0.5)
	if got != 0 {
		t.Errorf("expected 0 for empty slice, got %f", got)
	}
}

func TestWelchTTest_DifferentSamples(t *testing.T) {
	sample1 := []float64{100, 110, 105, 95, 108, 102, 99, 107, 103, 101}
	sample2 := []float64{50, 55, 48, 52, 51, 53, 49, 54, 50, 52}

	significant, pValue := WelchTTest(sample1, sample2, 0.05)
	if !significant {
		t.Error("expected significant difference")
	}
	if pValue >= 0.05 {
		t.Errorf("expected p-value < 0.05, got %f", pValue)
	}
}

func TestWelchTTest_SimilarSamples(t *testing.T) {
	sample1 := []float64{100, 101, 99, 100, 101}
	sample2 := []float64{100, 102, 99, 101, 100}

	significant, _ := WelchTTest(sample1, sample2, 0.05)
	if significant {
		t.Error("expected no significant difference for similar samples")
	}
}

func TestWelchTTest_TooFewSamples(t *testing.T) {
	significant, pValue := WelchTTest([]float64{1}, []float64{2}, 0.05)
	if significant {
		t.Error("expected no significance with single samples")
	}
	if pValue != 1.0 {
		t.Errorf("expected p=1.0 for insufficient samples, got %f", pValue)
	}
}

func TestWelchTTest_IdenticalSamples(t *testing.T) {
	sample := []float64{5, 5, 5, 5, 5}
	significant, pValue := WelchTTest(sample, sample, 0.05)
	if significant {
		t.Error("expected no significant difference for identical samples")
	}
	if pValue != 1.0 {
		t.Errorf("expected p=1.0 for identical samples, got %f", pValue)
	}
}

func TestEffectSize(t *testing.T) {
	sample1 := []float64{100, 110, 105, 95, 108}
	sample2 := []float64{50, 55, 48, 52, 51}

	d := EffectSize(sample1, sample2)
	if d <= 0 {
		t.Errorf("expected positive effect size, got %f", d)
	}
}

func TestEffectSize_Small(t *testing.T) {
	sample1 := []float64{100, 101, 99, 100, 101, 100, 99, 101, 100, 100}
	sample2 := []float64{100, 100, 99, 100, 100, 100, 99, 100, 100, 100}

	d := EffectSize(sample1, sample2)
	label := EffectSizeLabel(d)
	if label != "negligible" && label != "small" {
		t.Errorf("expected small/negligible effect, got %q (d=%.3f)", label, d)
	}
}

func TestEffectSizeLabel(t *testing.T) {
	tests := []struct {
		d    float64
		want string
	}{
		{0.0, "negligible"},
		{0.1, "negligible"},
		{0.3, "small"},
		{0.6, "medium"},
		{1.0, "large"},
		{-0.9, "large"},
	}

	for _, tt := range tests {
		got := EffectSizeLabel(tt.d)
		if got != tt.want {
			t.Errorf("EffectSizeLabel(%.1f) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestConfidenceInterval(t *testing.T) {
	sample1 := []float64{100, 110, 105, 95, 108}
	sample2 := []float64{50, 55, 48, 52, 51}

	lower, upper := ConfidenceInterval(sample1, sample2, 0.95)
	if lower >= upper {
		t.Errorf("expected lower < upper, got [%.2f, %.2f]", lower, upper)
	}

	diff := Mean(sample1) - Mean(sample2)
	if diff < lower || diff > upper {
		t.Error("mean difference should be within confidence interval")
	}
}

func TestConfidenceInterval_TooFewSamples(t *testing.T) {
	lower, upper := ConfidenceInterval([]float64{1}, []float64{2}, 0.95)
	if lower != 0 || upper != 0 {
		t.Errorf("expected [0,0] for insufficient samples, got [%.2f, %.2f]", lower, upper)
	}
}

// Helper functions.

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func makeTestReport() *report.Report {
	return &report.Report{
		Name:        "test-benchmark",
		GeneratedAt: time.Now(),
		Duration:    5 * time.Second,
		OverallBaseline: report.SummaryMetrics{
			Count:             20,
			SuccessCount:      20,
			MeanLatency:       100 * time.Millisecond,
			MedianLatency:     95 * time.Millisecond,
			P95Latency:        150 * time.Millisecond,
			P99Latency:        200 * time.Millisecond,
			MinLatency:        50 * time.Millisecond,
			MaxLatency:        250 * time.Millisecond,
			TotalInputTokens:  2000,
			TotalOutputTokens: 1000,
			TotalCost:         0.09,
		},
		OverallGateway: report.SummaryMetrics{
			Count:             20,
			SuccessCount:      20,
			MeanLatency:       60 * time.Millisecond,
			MedianLatency:     55 * time.Millisecond,
			P95Latency:        90 * time.Millisecond,
			P99Latency:        120 * time.Millisecond,
			MinLatency:        30 * time.Millisecond,
			MaxLatency:        150 * time.Millisecond,
			TotalInputTokens:  2000,
			TotalOutputTokens: 1000,
			TotalCachedTokens: 1200,
			TotalCost:         0.054,
			CacheHitRate:      0.6,
		},
		Workloads: []report.WorkloadReport{
			{
				WorkloadType: "repetitive",
				Baseline: report.SummaryMetrics{
					Count:            10,
					SuccessCount:     10,
					MeanLatency:      100 * time.Millisecond,
					TotalInputTokens: 1000,
					TotalCost:        0.045,
				},
				Gateway: report.SummaryMetrics{
					Count:             10,
					SuccessCount:      10,
					MeanLatency:       40 * time.Millisecond,
					TotalInputTokens:  1000,
					TotalCachedTokens: 900,
					TotalCost:         0.018,
					CacheHitRate:      0.9,
				},
				LatencyImprovement: 60,
				CostSavings:        60,
				TokenSavings:       90,
			},
			{
				WorkloadType: "random",
				Baseline: report.SummaryMetrics{
					Count:            10,
					SuccessCount:     10,
					MeanLatency:      100 * time.Millisecond,
					TotalInputTokens: 1000,
					TotalCost:        0.045,
				},
				Gateway: report.SummaryMetrics{
					Count:             10,
					SuccessCount:      10,
					MeanLatency:       80 * time.Millisecond,
					TotalInputTokens:  1000,
					TotalCachedTokens: 300,
					TotalCost:         0.036,
					CacheHitRate:      0.3,
				},
				LatencyImprovement: 20,
				CostSavings:        20,
				TokenSavings:       30,
			},
		},
	}
}
