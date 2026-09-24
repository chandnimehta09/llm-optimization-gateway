package report

import (
	"fmt"
	"math"
	"strings"
)

// ChartData holds data for chart generation.
type ChartData struct {
	Labels []string
	Series []ChartSeries
}

// ChartSeries holds a named data series.
type ChartSeries struct {
	Name   string
	Values []float64
}

// LatencyChart generates an ASCII bar chart comparing baseline vs gateway latency.
func LatencyChart(r *Report) string {
	data := ChartData{
		Labels: make([]string, 0, len(r.Workloads)),
		Series: []ChartSeries{
			{Name: "Baseline", Values: make([]float64, 0, len(r.Workloads))},
			{Name: "Gateway", Values: make([]float64, 0, len(r.Workloads))},
		},
	}

	for _, w := range r.Workloads {
		data.Labels = append(data.Labels, w.WorkloadType)
		data.Series[0].Values = append(data.Series[0].Values, float64(w.Baseline.MeanLatency)/float64(1e6))
		data.Series[1].Values = append(data.Series[1].Values, float64(w.Gateway.MeanLatency)/float64(1e6))
	}

	return renderBarChart("Mean Latency (ms)", data)
}

// CostChart generates an ASCII bar chart comparing baseline vs gateway costs.
func CostChart(r *Report) string {
	data := ChartData{
		Labels: make([]string, 0, len(r.Workloads)),
		Series: []ChartSeries{
			{Name: "Baseline", Values: make([]float64, 0, len(r.Workloads))},
			{Name: "Gateway", Values: make([]float64, 0, len(r.Workloads))},
		},
	}

	for _, w := range r.Workloads {
		data.Labels = append(data.Labels, w.WorkloadType)
		data.Series[0].Values = append(data.Series[0].Values, w.Baseline.TotalCost*1000000) // microdollars
		data.Series[1].Values = append(data.Series[1].Values, w.Gateway.TotalCost*1000000)
	}

	return renderBarChart("Total Cost (µ$)", data)
}

// ImprovementChart generates an ASCII chart showing improvement percentages.
func ImprovementChart(r *Report) string {
	var sb strings.Builder
	sb.WriteString("Improvement by Workload\n")
	sb.WriteString(strings.Repeat("-", 70) + "\n")

	maxLabel := 0
	for _, w := range r.Workloads {
		if len(w.WorkloadType) > maxLabel {
			maxLabel = len(w.WorkloadType)
		}
	}

	for _, w := range r.Workloads {
		latBar := renderHorizontalBar(w.LatencyImprovement, 40)
		costBar := renderHorizontalBar(w.CostSavings, 40)
		sb.WriteString(fmt.Sprintf("  %-*s  Latency: %s %.1f%%\n", maxLabel, w.WorkloadType, latBar, w.LatencyImprovement))
		sb.WriteString(fmt.Sprintf("  %-*s  Cost:    %s %.1f%%\n", maxLabel, "", costBar, w.CostSavings))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderBarChart renders a simple ASCII bar chart.
func renderBarChart(title string, data ChartData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s\n", title))
	sb.WriteString(strings.Repeat("-", 70) + "\n")

	maxVal := 0.0
	for _, s := range data.Series {
		for _, v := range s.Values {
			if v > maxVal {
				maxVal = v
			}
		}
	}

	if maxVal == 0 {
		maxVal = 1
	}

	maxLabel := 0
	for _, l := range data.Labels {
		if len(l) > maxLabel {
			maxLabel = len(l)
		}
	}

	barWidth := 40
	for i, label := range data.Labels {
		for _, s := range data.Series {
			if i < len(s.Values) {
				width := int(math.Round(s.Values[i] / maxVal * float64(barWidth)))
				if width < 0 {
					width = 0
				}
				bar := strings.Repeat("█", width) + strings.Repeat("░", barWidth-width)
				sb.WriteString(fmt.Sprintf("  %-*s [%-8s] %s %.2f\n",
					maxLabel, label, s.Name, bar, s.Values[i]))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderHorizontalBar renders a single horizontal bar for a percentage value.
func renderHorizontalBar(pct float64, maxWidth int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	width := int(math.Round(pct / 100 * float64(maxWidth)))
	return strings.Repeat("█", width) + strings.Repeat("░", maxWidth-width)
}
