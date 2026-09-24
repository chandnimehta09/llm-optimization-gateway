package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/llm-optimization-gateway/internal/benchmark"
)

// Exporter writes reports to various file formats.
type Exporter struct {
	outputDir string
}

// NewExporter creates a new report exporter.
func NewExporter(outputDir string) *Exporter {
	return &Exporter{outputDir: outputDir}
}

// ExportAll writes all report formats to the output directory.
func (e *Exporter) ExportAll(r *Report, results *benchmark.BenchmarkResults) error {
	if err := os.MkdirAll(e.outputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	if err := e.ExportJSON(r); err != nil {
		return fmt.Errorf("export JSON: %w", err)
	}
	if err := e.ExportText(r); err != nil {
		return fmt.Errorf("export text: %w", err)
	}
	if err := e.ExportCSV(r); err != nil {
		return fmt.Errorf("export CSV: %w", err)
	}
	if err := e.ExportRawResults(results); err != nil {
		return fmt.Errorf("export raw results: %w", err)
	}
	return nil
}

// ExportJSON writes the report as JSON.
func (e *Exporter) ExportJSON(r *Report) error {
	path := filepath.Join(e.outputDir, "report.json")
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// ExportText writes the report as formatted text.
func (e *Exporter) ExportText(r *Report) error {
	path := filepath.Join(e.outputDir, "report.txt")
	text := FormatText(r)
	text += "\n" + LatencyChart(r)
	text += "\n" + CostChart(r)
	text += "\n" + ImprovementChart(r)
	return os.WriteFile(path, []byte(text), 0o644)
}

// ExportCSV writes per-workload metrics as a CSV file.
func (e *Exporter) ExportCSV(r *Report) error {
	path := filepath.Join(e.outputDir, "report.csv")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create CSV: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Header.
	header := []string{
		"workload_type",
		"baseline_mean_latency_ms",
		"gateway_mean_latency_ms",
		"baseline_p95_latency_ms",
		"gateway_p95_latency_ms",
		"baseline_total_cost_usd",
		"gateway_total_cost_usd",
		"baseline_total_input_tokens",
		"gateway_total_input_tokens",
		"baseline_total_cached_tokens",
		"gateway_total_cached_tokens",
		"latency_improvement_pct",
		"cost_savings_pct",
		"token_savings_pct",
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, wr := range r.Workloads {
		row := []string{
			wr.WorkloadType,
			fmtMs(wr.Baseline.MeanLatency),
			fmtMs(wr.Gateway.MeanLatency),
			fmtMs(wr.Baseline.P95Latency),
			fmtMs(wr.Gateway.P95Latency),
			strconv.FormatFloat(wr.Baseline.TotalCost, 'f', 8, 64),
			strconv.FormatFloat(wr.Gateway.TotalCost, 'f', 8, 64),
			strconv.Itoa(wr.Baseline.TotalInputTokens),
			strconv.Itoa(wr.Gateway.TotalInputTokens),
			strconv.Itoa(wr.Baseline.TotalCachedTokens),
			strconv.Itoa(wr.Gateway.TotalCachedTokens),
			strconv.FormatFloat(wr.LatencyImprovement, 'f', 2, 64),
			strconv.FormatFloat(wr.CostSavings, 'f', 2, 64),
			strconv.FormatFloat(wr.TokenSavings, 'f', 2, 64),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// ExportRawResults writes the raw benchmark results as JSON.
func (e *Exporter) ExportRawResults(results *benchmark.BenchmarkResults) error {
	path := filepath.Join(e.outputDir, "raw_results.json")
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal raw results: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// fmtMs converts a duration to milliseconds as a string.
func fmtMs(d time.Duration) string {
	return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', 3, 64)
}
