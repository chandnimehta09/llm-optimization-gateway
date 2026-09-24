// Package comparison provides statistical comparison between baseline and
// gateway benchmark results.
package comparison

import (
	"fmt"
	"time"

	"github.com/llm-optimization-gateway/internal/benchmark/report"
)

// ComparisonResult holds the full comparison analysis.
type ComparisonResult struct {
	// Timestamp is when the comparison was performed.
	Timestamp time.Time `json:"timestamp"`
	// Overall holds the overall comparison across all workloads.
	Overall WorkloadComparison `json:"overall"`
	// PerWorkload holds per-workload comparisons.
	PerWorkload []WorkloadComparison `json:"per_workload"`
	// Recommendations holds optimization recommendations.
	Recommendations []Recommendation `json:"recommendations"`
	// Summary is a human-readable summary of the comparison.
	Summary string `json:"summary"`
}

// WorkloadComparison holds comparison data for a single workload (or overall).
type WorkloadComparison struct {
	// WorkloadType identifies the workload (empty for overall).
	WorkloadType string `json:"workload_type,omitempty"`
	// LatencyAnalysis compares latency metrics.
	LatencyAnalysis MetricComparison `json:"latency_analysis"`
	// CostAnalysis compares cost metrics.
	CostAnalysis MetricComparison `json:"cost_analysis"`
	// TokenAnalysis compares token usage.
	TokenAnalysis TokenComparison `json:"token_analysis"`
	// StatisticalSignificance indicates whether the difference is statistically significant.
	StatisticalSignificance bool `json:"statistically_significant"`
	// ConfidenceLevel is the confidence level for the significance test.
	ConfidenceLevel float64 `json:"confidence_level"`
}

// MetricComparison compares a single metric between baseline and gateway.
type MetricComparison struct {
	// BaselineValue is the baseline metric value.
	BaselineValue float64 `json:"baseline_value"`
	// GatewayValue is the gateway metric value.
	GatewayValue float64 `json:"gateway_value"`
	// AbsoluteChange is gateway - baseline.
	AbsoluteChange float64 `json:"absolute_change"`
	// PercentChange is the percentage change.
	PercentChange float64 `json:"percent_change"`
	// Improved indicates whether the gateway performed better.
	Improved bool `json:"improved"`
}

// TokenComparison holds token-specific comparison data.
type TokenComparison struct {
	// BaselineInputTokens total from baseline.
	BaselineInputTokens int `json:"baseline_input_tokens"`
	// GatewayInputTokens total from gateway.
	GatewayInputTokens int `json:"gateway_input_tokens"`
	// GatewayCachedTokens total cached from gateway.
	GatewayCachedTokens int `json:"gateway_cached_tokens"`
	// CacheHitRate is the fraction of input tokens served from cache.
	CacheHitRate float64 `json:"cache_hit_rate"`
	// TokenSavingsPercent is the percentage of tokens saved via caching.
	TokenSavingsPercent float64 `json:"token_savings_pct"`
}

// Recommendation is an optimization recommendation based on the comparison.
type Recommendation struct {
	// Category groups the recommendation (e.g., "caching", "cost", "latency").
	Category string `json:"category"`
	// Priority is the priority level (high, medium, low).
	Priority string `json:"priority"`
	// Message describes the recommendation.
	Message string `json:"message"`
	// Impact is the estimated impact of following the recommendation.
	Impact string `json:"impact,omitempty"`
}

// Engine performs comparison analysis between benchmark results.
type Engine struct{}

// NewEngine creates a new comparison engine.
func NewEngine() *Engine {
	return &Engine{}
}

// Compare performs a full comparison analysis.
func (e *Engine) Compare(r *report.Report) (*ComparisonResult, error) {
	if r == nil {
		return nil, fmt.Errorf("report is required")
	}

	result := &ComparisonResult{
		Timestamp: time.Now(),
	}

	// Overall comparison.
	result.Overall = e.compareMetrics("", r.OverallBaseline, r.OverallGateway)

	// Per-workload comparisons.
	for _, w := range r.Workloads {
		wc := e.compareMetrics(w.WorkloadType, w.Baseline, w.Gateway)
		result.PerWorkload = append(result.PerWorkload, wc)
	}

	// Generate recommendations.
	result.Recommendations = e.generateRecommendations(result)
	result.Summary = e.generateSummary(result)

	return result, nil
}

// compareMetrics compares baseline and gateway summary metrics.
func (e *Engine) compareMetrics(workloadType string, baseline, gateway report.SummaryMetrics) WorkloadComparison {
	baselineLatency := float64(baseline.MeanLatency) / float64(time.Millisecond)
	gatewayLatency := float64(gateway.MeanLatency) / float64(time.Millisecond)

	wc := WorkloadComparison{
		WorkloadType:    workloadType,
		LatencyAnalysis: newMetricComparison(baselineLatency, gatewayLatency, true),
		CostAnalysis:    newMetricComparison(baseline.TotalCost, gateway.TotalCost, true),
		TokenAnalysis: TokenComparison{
			BaselineInputTokens: baseline.TotalInputTokens,
			GatewayInputTokens:  gateway.TotalInputTokens,
			GatewayCachedTokens: gateway.TotalCachedTokens,
			CacheHitRate:        gateway.CacheHitRate,
			TokenSavingsPercent: tokenSavings(gateway),
		},
	}

	// Extract latency values for statistical tests.
	baselineLatencies := []float64{baselineLatency}
	gatewayLatencies := []float64{gatewayLatency}

	if baseline.Count > 1 && gateway.Count > 1 {
		// With real per-request data, we'd have full distributions.
		// Approximate with available percentile data.
		baselineLatencies = approximateDistribution(baseline)
		gatewayLatencies = approximateDistribution(gateway)
	}

	sig, conf := WelchTTest(baselineLatencies, gatewayLatencies, 0.05)
	wc.StatisticalSignificance = sig
	wc.ConfidenceLevel = conf

	return wc
}

// approximateDistribution creates an approximate distribution from summary metrics.
func approximateDistribution(m report.SummaryMetrics) []float64 {
	// Use available percentile data to approximate the distribution.
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	return []float64{
		ms(m.MinLatency),
		ms(m.MedianLatency),
		ms(m.MeanLatency),
		ms(m.P95Latency),
		ms(m.P99Latency),
		ms(m.MaxLatency),
	}
}

// newMetricComparison creates a MetricComparison. lowerIsBetter indicates
// whether a lower value is an improvement (true for latency and cost).
func newMetricComparison(baseline, gateway float64, lowerIsBetter bool) MetricComparison {
	mc := MetricComparison{
		BaselineValue:  baseline,
		GatewayValue:   gateway,
		AbsoluteChange: gateway - baseline,
	}
	if baseline != 0 {
		mc.PercentChange = (gateway - baseline) / baseline * 100
	}
	if lowerIsBetter {
		mc.Improved = gateway < baseline
	} else {
		mc.Improved = gateway > baseline
	}
	return mc
}

func tokenSavings(m report.SummaryMetrics) float64 {
	if m.TotalInputTokens == 0 {
		return 0
	}
	return float64(m.TotalCachedTokens) / float64(m.TotalInputTokens) * 100
}

// generateRecommendations produces recommendations based on comparison results.
func (e *Engine) generateRecommendations(result *ComparisonResult) []Recommendation {
	var recs []Recommendation

	// Check overall latency.
	if result.Overall.LatencyAnalysis.Improved {
		recs = append(recs, Recommendation{
			Category: "latency",
			Priority: "low",
			Message:  fmt.Sprintf("Gateway reduces mean latency by %.1f%%. Optimization is effective.", -result.Overall.LatencyAnalysis.PercentChange),
			Impact:   "Confirmed positive impact on response times.",
		})
	} else if result.Overall.LatencyAnalysis.PercentChange > 10 {
		recs = append(recs, Recommendation{
			Category: "latency",
			Priority: "high",
			Message:  fmt.Sprintf("Gateway increases mean latency by %.1f%%. Review gateway overhead.", result.Overall.LatencyAnalysis.PercentChange),
			Impact:   "Gateway overhead may outweigh optimization benefits for this workload mix.",
		})
	}

	// Check cost savings.
	if result.Overall.CostAnalysis.Improved {
		recs = append(recs, Recommendation{
			Category: "cost",
			Priority: "low",
			Message:  fmt.Sprintf("Gateway reduces cost by %.1f%%.", -result.Overall.CostAnalysis.PercentChange),
			Impact:   "Token caching provides measurable cost reduction.",
		})
	} else {
		recs = append(recs, Recommendation{
			Category: "cost",
			Priority: "medium",
			Message:  "Gateway does not reduce cost for this workload. Consider tuning cache configuration.",
			Impact:   "Adjusting cache TTL or prefix matching may improve cost efficiency.",
		})
	}

	// Check cache hit rate.
	if result.Overall.TokenAnalysis.CacheHitRate < 0.1 {
		recs = append(recs, Recommendation{
			Category: "caching",
			Priority: "high",
			Message:  "Cache hit rate is below 10%. Review prefix detection and cache key strategy.",
			Impact:   "Improving cache hit rate is the primary lever for cost and latency reduction.",
		})
	} else if result.Overall.TokenAnalysis.CacheHitRate > 0.5 {
		recs = append(recs, Recommendation{
			Category: "caching",
			Priority: "low",
			Message:  fmt.Sprintf("Cache hit rate is %.0f%%. Caching strategy is working well.", result.Overall.TokenAnalysis.CacheHitRate*100),
		})
	}

	// Per-workload recommendations.
	for _, pw := range result.PerWorkload {
		if pw.WorkloadType == "repetitive" && pw.TokenAnalysis.CacheHitRate < 0.5 {
			recs = append(recs, Recommendation{
				Category: "caching",
				Priority: "high",
				Message:  "Repetitive workload has low cache hit rate. Exact-match caching may not be working.",
				Impact:   "Repetitive prompts should achieve near-100% cache hit rate.",
			})
		}
		if pw.WorkloadType == "static_prefix" && pw.TokenAnalysis.CacheHitRate < 0.3 {
			recs = append(recs, Recommendation{
				Category: "caching",
				Priority: "medium",
				Message:  "Static prefix workload has low cache hit rate. Review prefix detection configuration.",
				Impact:   "Prefix caching should provide significant savings for static prefix workloads.",
			})
		}
	}

	return recs
}

// generateSummary creates a human-readable summary.
func (e *Engine) generateSummary(result *ComparisonResult) string {
	overall := result.Overall
	latPct := overall.LatencyAnalysis.PercentChange
	costPct := overall.CostAnalysis.PercentChange
	cacheRate := overall.TokenAnalysis.CacheHitRate * 100

	var verdict string
	if overall.LatencyAnalysis.Improved && overall.CostAnalysis.Improved {
		verdict = "The gateway optimization provides clear benefits in both latency and cost."
	} else if overall.LatencyAnalysis.Improved {
		verdict = "The gateway improves latency but does not reduce cost for this workload."
	} else if overall.CostAnalysis.Improved {
		verdict = "The gateway reduces cost but adds latency overhead."
	} else {
		verdict = "The gateway does not provide measurable benefits for this workload mix. Review configuration."
	}

	return fmt.Sprintf(
		"Latency: %.1f%% change | Cost: %.1f%% change | Cache hit rate: %.0f%% | %s",
		latPct, costPct, cacheRate, verdict,
	)
}
