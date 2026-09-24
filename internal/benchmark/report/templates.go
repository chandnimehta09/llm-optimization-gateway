package report

import (
	"fmt"
	"strings"
	"time"
)

// FormatText produces a human-readable text report.
func FormatText(r *Report) string {
	var sb strings.Builder

	sb.WriteString(strings.Repeat("=", 80) + "\n")
	sb.WriteString(fmt.Sprintf("  BENCHMARK REPORT: %s\n", r.Name))
	if r.Description != "" {
		sb.WriteString(fmt.Sprintf("  %s\n", r.Description))
	}
	sb.WriteString(fmt.Sprintf("  Generated: %s\n", r.GeneratedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("  Duration:  %s\n", r.Duration.Round(time.Millisecond)))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	// Overall summary.
	sb.WriteString("OVERALL SUMMARY\n")
	sb.WriteString(strings.Repeat("-", 60) + "\n")
	sb.WriteString(fmt.Sprintf("  %-30s %15s %15s\n", "Metric", "Baseline", "Gateway"))
	sb.WriteString(strings.Repeat("-", 60) + "\n")
	writeMetricRow(&sb, "Mean Latency",
		formatDuration(r.OverallBaseline.MeanLatency),
		formatDuration(r.OverallGateway.MeanLatency))
	writeMetricRow(&sb, "P95 Latency",
		formatDuration(r.OverallBaseline.P95Latency),
		formatDuration(r.OverallGateway.P95Latency))
	writeMetricRow(&sb, "P99 Latency",
		formatDuration(r.OverallBaseline.P99Latency),
		formatDuration(r.OverallGateway.P99Latency))
	writeMetricRow(&sb, "Total Input Tokens",
		fmt.Sprintf("%d", r.OverallBaseline.TotalInputTokens),
		fmt.Sprintf("%d", r.OverallGateway.TotalInputTokens))
	writeMetricRow(&sb, "Total Output Tokens",
		fmt.Sprintf("%d", r.OverallBaseline.TotalOutputTokens),
		fmt.Sprintf("%d", r.OverallGateway.TotalOutputTokens))
	writeMetricRow(&sb, "Total Cached Tokens",
		fmt.Sprintf("%d", r.OverallBaseline.TotalCachedTokens),
		fmt.Sprintf("%d", r.OverallGateway.TotalCachedTokens))
	writeMetricRow(&sb, "Total Cost (USD)",
		fmt.Sprintf("$%.6f", r.OverallBaseline.TotalCost),
		fmt.Sprintf("$%.6f", r.OverallGateway.TotalCost))
	writeMetricRow(&sb, "Cache Hit Rate",
		fmt.Sprintf("%.1f%%", r.OverallBaseline.CacheHitRate*100),
		fmt.Sprintf("%.1f%%", r.OverallGateway.CacheHitRate*100))
	writeMetricRow(&sb, "Success Rate",
		fmt.Sprintf("%.1f%%", successRate(r.OverallBaseline)),
		fmt.Sprintf("%.1f%%", successRate(r.OverallGateway)))
	sb.WriteString(strings.Repeat("-", 60) + "\n")
	sb.WriteString(fmt.Sprintf("  Latency Improvement: %.1f%%\n", r.OverallLatencyImprovement))
	sb.WriteString(fmt.Sprintf("  Cost Savings:        %.1f%%\n", r.OverallCostSavings))
	sb.WriteString("\n")

	// Per-workload details.
	for _, w := range r.Workloads {
		sb.WriteString(fmt.Sprintf("WORKLOAD: %s\n", strings.ToUpper(w.WorkloadType)))
		sb.WriteString(strings.Repeat("-", 60) + "\n")
		sb.WriteString(fmt.Sprintf("  %-30s %15s %15s\n", "Metric", "Baseline", "Gateway"))
		sb.WriteString(strings.Repeat("-", 60) + "\n")
		writeMetricRow(&sb, "Requests",
			fmt.Sprintf("%d", w.Baseline.Count),
			fmt.Sprintf("%d", w.Gateway.Count))
		writeMetricRow(&sb, "Mean Latency",
			formatDuration(w.Baseline.MeanLatency),
			formatDuration(w.Gateway.MeanLatency))
		writeMetricRow(&sb, "Median Latency",
			formatDuration(w.Baseline.MedianLatency),
			formatDuration(w.Gateway.MedianLatency))
		writeMetricRow(&sb, "P95 Latency",
			formatDuration(w.Baseline.P95Latency),
			formatDuration(w.Gateway.P95Latency))
		writeMetricRow(&sb, "Total Cost (USD)",
			fmt.Sprintf("$%.6f", w.Baseline.TotalCost),
			fmt.Sprintf("$%.6f", w.Gateway.TotalCost))
		writeMetricRow(&sb, "Cache Hit Rate",
			fmt.Sprintf("%.1f%%", w.Baseline.CacheHitRate*100),
			fmt.Sprintf("%.1f%%", w.Gateway.CacheHitRate*100))
		sb.WriteString(strings.Repeat("-", 60) + "\n")
		sb.WriteString(fmt.Sprintf("  Latency Improvement: %.1f%%\n", w.LatencyImprovement))
		sb.WriteString(fmt.Sprintf("  Cost Savings:        %.1f%%\n", w.CostSavings))
		sb.WriteString(fmt.Sprintf("  Token Savings:       %.1f%%\n", w.TokenSavings))
		sb.WriteString("\n")
	}

	return sb.String()
}

// FormatMarkdown produces a markdown-formatted report.
func FormatMarkdown(r *Report) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Benchmark Report: %s\n\n", r.Name))
	if r.Description != "" {
		sb.WriteString(fmt.Sprintf("> %s\n\n", r.Description))
	}
	sb.WriteString(fmt.Sprintf("**Generated:** %s  \n", r.GeneratedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Duration:** %s\n\n", r.Duration.Round(time.Millisecond)))

	// Overall summary table.
	sb.WriteString("## Overall Summary\n\n")
	sb.WriteString("| Metric | Baseline | Gateway | Improvement |\n")
	sb.WriteString("|--------|----------|---------|-------------|\n")
	sb.WriteString(fmt.Sprintf("| Mean Latency | %s | %s | %.1f%% |\n",
		formatDuration(r.OverallBaseline.MeanLatency),
		formatDuration(r.OverallGateway.MeanLatency),
		r.OverallLatencyImprovement))
	sb.WriteString(fmt.Sprintf("| P95 Latency | %s | %s | - |\n",
		formatDuration(r.OverallBaseline.P95Latency),
		formatDuration(r.OverallGateway.P95Latency)))
	sb.WriteString(fmt.Sprintf("| Total Cost | $%.6f | $%.6f | %.1f%% |\n",
		r.OverallBaseline.TotalCost,
		r.OverallGateway.TotalCost,
		r.OverallCostSavings))
	sb.WriteString(fmt.Sprintf("| Cache Hit Rate | %.1f%% | %.1f%% | - |\n",
		r.OverallBaseline.CacheHitRate*100,
		r.OverallGateway.CacheHitRate*100))
	sb.WriteString("\n")

	// Per-workload tables.
	sb.WriteString("## Workload Details\n\n")
	for _, w := range r.Workloads {
		sb.WriteString(fmt.Sprintf("### %s\n\n", w.WorkloadType))
		sb.WriteString("| Metric | Baseline | Gateway |\n")
		sb.WriteString("|--------|----------|---------|\n")
		sb.WriteString(fmt.Sprintf("| Requests | %d | %d |\n", w.Baseline.Count, w.Gateway.Count))
		sb.WriteString(fmt.Sprintf("| Mean Latency | %s | %s |\n",
			formatDuration(w.Baseline.MeanLatency),
			formatDuration(w.Gateway.MeanLatency)))
		sb.WriteString(fmt.Sprintf("| P95 Latency | %s | %s |\n",
			formatDuration(w.Baseline.P95Latency),
			formatDuration(w.Gateway.P95Latency)))
		sb.WriteString(fmt.Sprintf("| Total Cost | $%.6f | $%.6f |\n",
			w.Baseline.TotalCost, w.Gateway.TotalCost))
		sb.WriteString(fmt.Sprintf("| Latency Improvement | %.1f%% | - |\n", w.LatencyImprovement))
		sb.WriteString(fmt.Sprintf("| Cost Savings | %.1f%% | - |\n", w.CostSavings))
		sb.WriteString(fmt.Sprintf("| Token Savings | %.1f%% | - |\n", w.TokenSavings))
		sb.WriteString("\n")
	}

	return sb.String()
}

// writeMetricRow writes a formatted row in the text table.
func writeMetricRow(sb *strings.Builder, name, baseline, gateway string) {
	sb.WriteString(fmt.Sprintf("  %-30s %15s %15s\n", name, baseline, gateway))
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.0fµs", float64(d)/float64(time.Microsecond))
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%.2fs", float64(d)/float64(time.Second))
}

// successRate returns the success rate as a percentage.
func successRate(m SummaryMetrics) float64 {
	if m.Count == 0 {
		return 0
	}
	return float64(m.SuccessCount) / float64(m.Count) * 100
}
