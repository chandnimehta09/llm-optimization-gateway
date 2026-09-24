// Package report generates benchmark result reports in multiple formats.
package report

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/llm-optimization-gateway/internal/benchmark"
)

// SummaryMetrics holds aggregated metrics for a set of results.
type SummaryMetrics struct {
	// Count is the number of requests.
	Count int `json:"count"`
	// SuccessCount is the number of successful requests.
	SuccessCount int `json:"success_count"`
	// ErrorCount is the number of failed requests.
	ErrorCount int `json:"error_count"`
	// MeanLatency is the average latency.
	MeanLatency time.Duration `json:"mean_latency_ns"`
	// MedianLatency is the p50 latency.
	MedianLatency time.Duration `json:"median_latency_ns"`
	// P95Latency is the 95th percentile latency.
	P95Latency time.Duration `json:"p95_latency_ns"`
	// P99Latency is the 99th percentile latency.
	P99Latency time.Duration `json:"p99_latency_ns"`
	// MinLatency is the minimum latency.
	MinLatency time.Duration `json:"min_latency_ns"`
	// MaxLatency is the maximum latency.
	MaxLatency time.Duration `json:"max_latency_ns"`
	// TotalInputTokens is the sum of input tokens.
	TotalInputTokens int `json:"total_input_tokens"`
	// TotalOutputTokens is the sum of output tokens.
	TotalOutputTokens int `json:"total_output_tokens"`
	// TotalCachedTokens is the sum of cached tokens.
	TotalCachedTokens int `json:"total_cached_tokens"`
	// TotalCost is the sum of costs in USD.
	TotalCost float64 `json:"total_cost"`
	// MeanInputTokens is the average input tokens per request.
	MeanInputTokens float64 `json:"mean_input_tokens"`
	// MeanOutputTokens is the average output tokens per request.
	MeanOutputTokens float64 `json:"mean_output_tokens"`
	// CacheHitRate is the fraction of tokens served from cache.
	CacheHitRate float64 `json:"cache_hit_rate"`
}

// WorkloadReport holds metrics for a single workload.
type WorkloadReport struct {
	// WorkloadType identifies the workload.
	WorkloadType string `json:"workload_type"`
	// Baseline holds baseline metrics.
	Baseline SummaryMetrics `json:"baseline"`
	// Gateway holds gateway metrics.
	Gateway SummaryMetrics `json:"gateway"`
	// LatencyImprovement is the percentage improvement in mean latency.
	LatencyImprovement float64 `json:"latency_improvement_pct"`
	// CostSavings is the percentage savings in cost.
	CostSavings float64 `json:"cost_savings_pct"`
	// TokenSavings is the percentage of tokens saved via caching.
	TokenSavings float64 `json:"token_savings_pct"`
}

// Report is the full benchmark report.
type Report struct {
	// Name is the benchmark name.
	Name string `json:"name"`
	// Description is the benchmark description.
	Description string `json:"description,omitempty"`
	// GeneratedAt is when the report was generated.
	GeneratedAt time.Time `json:"generated_at"`
	// Duration is the total benchmark duration.
	Duration time.Duration `json:"duration_ns"`
	// Overall holds aggregated metrics across all workloads.
	OverallBaseline SummaryMetrics `json:"overall_baseline"`
	// OverallGateway holds aggregated gateway metrics.
	OverallGateway SummaryMetrics `json:"overall_gateway"`
	// Workloads holds per-workload reports.
	Workloads []WorkloadReport `json:"workloads"`
	// OverallLatencyImprovement is the overall latency improvement percentage.
	OverallLatencyImprovement float64 `json:"overall_latency_improvement_pct"`
	// OverallCostSavings is the overall cost savings percentage.
	OverallCostSavings float64 `json:"overall_cost_savings_pct"`
}

// Generator produces reports from benchmark results.
type Generator struct{}

// NewGenerator creates a new report generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// Generate creates a report from benchmark results.
func (g *Generator) Generate(results *benchmark.BenchmarkResults) (*Report, error) {
	if results == nil {
		return nil, fmt.Errorf("results are required")
	}

	report := &Report{
		Name:        results.Config.Name,
		Description: results.Config.Description,
		GeneratedAt: time.Now(),
		Duration:    results.Duration,
	}

	var allBaseline, allGateway []benchmark.RequestResult

	for _, wr := range results.WorkloadResults {
		baselineMetrics := computeMetrics(wr.BaselineResults)
		gatewayMetrics := computeMetrics(wr.GatewayResults)

		workloadReport := WorkloadReport{
			WorkloadType:       wr.WorkloadType,
			Baseline:           baselineMetrics,
			Gateway:            gatewayMetrics,
			LatencyImprovement: percentImprovement(baselineMetrics.MeanLatency, gatewayMetrics.MeanLatency),
			CostSavings:        percentImprovement64(baselineMetrics.TotalCost, gatewayMetrics.TotalCost),
			TokenSavings:       tokenSavingsPercent(gatewayMetrics),
		}
		report.Workloads = append(report.Workloads, workloadReport)

		allBaseline = append(allBaseline, wr.BaselineResults...)
		allGateway = append(allGateway, wr.GatewayResults...)
	}

	report.OverallBaseline = computeMetrics(allBaseline)
	report.OverallGateway = computeMetrics(allGateway)
	report.OverallLatencyImprovement = percentImprovement(
		report.OverallBaseline.MeanLatency,
		report.OverallGateway.MeanLatency,
	)
	report.OverallCostSavings = percentImprovement64(
		report.OverallBaseline.TotalCost,
		report.OverallGateway.TotalCost,
	)

	return report, nil
}

// computeMetrics calculates summary metrics from a list of request results.
func computeMetrics(results []benchmark.RequestResult) SummaryMetrics {
	if len(results) == 0 {
		return SummaryMetrics{}
	}

	m := SummaryMetrics{
		Count:      len(results),
		MinLatency: time.Duration(math.MaxInt64),
	}

	latencies := make([]time.Duration, 0, len(results))
	var totalLatency time.Duration

	for _, r := range results {
		if r.Error != "" {
			m.ErrorCount++
		} else {
			m.SuccessCount++
		}
		latencies = append(latencies, r.Latency)
		totalLatency += r.Latency
		m.TotalInputTokens += r.InputTokens
		m.TotalOutputTokens += r.OutputTokens
		m.TotalCachedTokens += r.CachedTokens
		m.TotalCost += r.Cost
		if r.Latency < m.MinLatency {
			m.MinLatency = r.Latency
		}
		if r.Latency > m.MaxLatency {
			m.MaxLatency = r.Latency
		}
	}

	m.MeanLatency = totalLatency / time.Duration(m.Count)

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})
	m.MedianLatency = percentile(latencies, 0.5)
	m.P95Latency = percentile(latencies, 0.95)
	m.P99Latency = percentile(latencies, 0.99)

	m.MeanInputTokens = float64(m.TotalInputTokens) / float64(m.Count)
	m.MeanOutputTokens = float64(m.TotalOutputTokens) / float64(m.Count)

	if m.TotalInputTokens > 0 {
		m.CacheHitRate = float64(m.TotalCachedTokens) / float64(m.TotalInputTokens)
	}

	return m
}

// percentile returns the value at the given percentile from a sorted slice.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// percentImprovement calculates the percentage improvement from baseline to gateway.
func percentImprovement(baseline, gateway time.Duration) float64 {
	if baseline == 0 {
		return 0
	}
	return (float64(baseline) - float64(gateway)) / float64(baseline) * 100
}

// percentImprovement64 calculates percentage improvement for float64 values.
func percentImprovement64(baseline, gateway float64) float64 {
	if baseline == 0 {
		return 0
	}
	return (baseline - gateway) / baseline * 100
}

// tokenSavingsPercent calculates the percentage of tokens saved via caching.
func tokenSavingsPercent(metrics SummaryMetrics) float64 {
	if metrics.TotalInputTokens == 0 {
		return 0
	}
	return float64(metrics.TotalCachedTokens) / float64(metrics.TotalInputTokens) * 100
}
