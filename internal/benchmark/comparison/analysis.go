package comparison

import (
	"fmt"
	"strings"
)

// AnalysisReport generates a detailed textual analysis from a comparison result.
type AnalysisReport struct {
	result *ComparisonResult
}

// NewAnalysisReport creates a new analysis report.
func NewAnalysisReport(result *ComparisonResult) *AnalysisReport {
	return &AnalysisReport{result: result}
}

// Format produces a formatted analysis string.
func (a *AnalysisReport) Format() string {
	var sb strings.Builder

	sb.WriteString("COMPARISON ANALYSIS\n")
	sb.WriteString(strings.Repeat("=", 70) + "\n\n")

	// Overall.
	sb.WriteString("Overall Results\n")
	sb.WriteString(strings.Repeat("-", 50) + "\n")
	a.writeComparison(&sb, a.result.Overall)
	sb.WriteString("\n")

	// Per workload.
	for _, pw := range a.result.PerWorkload {
		sb.WriteString(fmt.Sprintf("Workload: %s\n", pw.WorkloadType))
		sb.WriteString(strings.Repeat("-", 50) + "\n")
		a.writeComparison(&sb, pw)
		sb.WriteString("\n")
	}

	// Recommendations.
	sb.WriteString("RECOMMENDATIONS\n")
	sb.WriteString(strings.Repeat("=", 70) + "\n\n")
	for i, rec := range a.result.Recommendations {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s: %s\n", i+1,
			strings.ToUpper(rec.Priority), rec.Category, rec.Message))
		if rec.Impact != "" {
			sb.WriteString(fmt.Sprintf("   Impact: %s\n", rec.Impact))
		}
	}
	sb.WriteString("\n")

	// Summary.
	sb.WriteString("SUMMARY\n")
	sb.WriteString(strings.Repeat("-", 50) + "\n")
	sb.WriteString(a.result.Summary + "\n")

	return sb.String()
}

// writeComparison writes a single workload comparison.
func (a *AnalysisReport) writeComparison(sb *strings.Builder, wc WorkloadComparison) {
	sb.WriteString(fmt.Sprintf("  Latency:  Baseline=%.2fms  Gateway=%.2fms  Change=%.1f%%  %s\n",
		wc.LatencyAnalysis.BaselineValue,
		wc.LatencyAnalysis.GatewayValue,
		wc.LatencyAnalysis.PercentChange,
		improvementLabel(wc.LatencyAnalysis.Improved)))

	sb.WriteString(fmt.Sprintf("  Cost:     Baseline=$%.8f  Gateway=$%.8f  Change=%.1f%%  %s\n",
		wc.CostAnalysis.BaselineValue,
		wc.CostAnalysis.GatewayValue,
		wc.CostAnalysis.PercentChange,
		improvementLabel(wc.CostAnalysis.Improved)))

	sb.WriteString(fmt.Sprintf("  Tokens:   Baseline=%d  Gateway=%d  Cached=%d  HitRate=%.1f%%\n",
		wc.TokenAnalysis.BaselineInputTokens,
		wc.TokenAnalysis.GatewayInputTokens,
		wc.TokenAnalysis.GatewayCachedTokens,
		wc.TokenAnalysis.CacheHitRate*100))

	if wc.StatisticalSignificance {
		sb.WriteString(fmt.Sprintf("  Statistical Significance: YES (confidence=%.2f)\n", wc.ConfidenceLevel))
	} else {
		sb.WriteString(fmt.Sprintf("  Statistical Significance: NO (confidence=%.2f)\n", wc.ConfidenceLevel))
	}
}

func improvementLabel(improved bool) string {
	if improved {
		return "✓ IMPROVED"
	}
	return "✗ NO IMPROVEMENT"
}

// CostBreakdown produces a detailed cost comparison.
func CostBreakdown(result *ComparisonResult) string {
	var sb strings.Builder
	sb.WriteString("COST BREAKDOWN\n")
	sb.WriteString(strings.Repeat("-", 60) + "\n")

	overall := result.Overall
	sb.WriteString(fmt.Sprintf("  Baseline Total Cost:       $%.8f\n", overall.CostAnalysis.BaselineValue))
	sb.WriteString(fmt.Sprintf("  Gateway Total Cost:        $%.8f\n", overall.CostAnalysis.GatewayValue))
	sb.WriteString(fmt.Sprintf("  Absolute Savings:          $%.8f\n", -overall.CostAnalysis.AbsoluteChange))
	sb.WriteString(fmt.Sprintf("  Percent Savings:           %.1f%%\n", -overall.CostAnalysis.PercentChange))
	sb.WriteString(fmt.Sprintf("  Tokens Cached:             %d\n", overall.TokenAnalysis.GatewayCachedTokens))
	sb.WriteString(fmt.Sprintf("  Cache Hit Rate:            %.1f%%\n", overall.TokenAnalysis.CacheHitRate*100))
	sb.WriteString("\n")

	// Per-workload breakdown.
	for _, pw := range result.PerWorkload {
		sb.WriteString(fmt.Sprintf("  %-20s  Baseline=$%.8f  Gateway=$%.8f  Savings=%.1f%%\n",
			pw.WorkloadType,
			pw.CostAnalysis.BaselineValue,
			pw.CostAnalysis.GatewayValue,
			-pw.CostAnalysis.PercentChange))
	}

	return sb.String()
}
