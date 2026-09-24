package benchmark_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/benchmark"
	"github.com/llm-optimization-gateway/internal/benchmark/comparison"
	"github.com/llm-optimization-gateway/internal/benchmark/report"
	"github.com/llm-optimization-gateway/internal/benchmark/workloads"
)

// TestIntegration_MockProvider tests the full benchmark pipeline with a mock HTTP provider.
func TestIntegration_MockProvider(t *testing.T) {
	// Create a mock provider that returns realistic responses.
	requestCount := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		resp := map[string]interface{}{
			"id":      fmt.Sprintf("chatcmpl-%d", requestCount),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "This is a mock response for benchmarking.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     100,
				"completion_tokens": 20,
				"total_tokens":      120,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer provider.Close()

	// Create a mock gateway that simulates caching.
	gatewayRequestCount := 0
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayRequestCount++
		cachedTokens := 0
		if gatewayRequestCount > 2 {
			// Simulate cache hits after warmup.
			cachedTokens = 80
		}
		resp := map[string]interface{}{
			"id":      fmt.Sprintf("chatcmpl-gw-%d", gatewayRequestCount),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "This is a cached response.",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     100,
				"completion_tokens": 20,
				"total_tokens":      120,
				"cached_tokens":     cachedTokens,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer gateway.Close()

	// Configure benchmark.
	cfg := &benchmark.Config{
		Name:                     "integration-test",
		ProviderEndpoint:         provider.URL,
		GatewayEndpoint:          gateway.URL,
		Model:                    "gpt-4",
		Concurrency:              1,
		WarmupIterations:         1,
		Iterations:               3,
		Timeout:                  5 * time.Second,
		CooldownBetweenWorkloads: 0,
		OutputDir:                t.TempDir(),
		CostPerInputToken:        0.00003,
		CostPerOutputToken:       0.00006,
		Workloads: []benchmark.WorkloadConfig{
			{Type: "repetitive", Weight: 1.0},
		},
	}

	// Create senders.
	baselineSender := benchmark.NewHTTPSender(benchmark.HTTPSenderConfig{
		Endpoint:      cfg.ProviderEndpoint,
		Model:         cfg.Model,
		Mode:          benchmark.RunModeBaseline,
		Timeout:       cfg.Timeout,
		CostPerInput:  cfg.CostPerInputToken,
		CostPerOutput: cfg.CostPerOutputToken,
	})
	gatewaySender := benchmark.NewHTTPSender(benchmark.HTTPSenderConfig{
		Endpoint:      cfg.GatewayEndpoint,
		Model:         cfg.Model,
		Mode:          benchmark.RunModeGateway,
		Timeout:       cfg.Timeout,
		CostPerInput:  cfg.CostPerInputToken,
		CostPerOutput: cfg.CostPerOutputToken,
	})

	// Create and configure harness.
	harness, err := benchmark.NewHarness(cfg, baselineSender, gatewaySender)
	if err != nil {
		t.Fatalf("failed to create harness: %v", err)
	}

	registry := workloads.NewRegistry()
	for _, wc := range cfg.Workloads {
		gen, err := registry.Create(wc)
		if err != nil {
			t.Fatalf("failed to create generator: %v", err)
		}
		harness.RegisterGenerator(gen)
	}

	// Run benchmark.
	results, err := harness.Run(context.Background())
	if err != nil {
		t.Fatalf("benchmark run failed: %v", err)
	}

	// Verify results.
	if len(results.WorkloadResults) != 1 {
		t.Fatalf("expected 1 workload result, got %d", len(results.WorkloadResults))
	}

	wr := results.WorkloadResults[0]
	if len(wr.BaselineResults) != 3 {
		t.Errorf("expected 3 baseline results, got %d", len(wr.BaselineResults))
	}
	if len(wr.GatewayResults) != 3 {
		t.Errorf("expected 3 gateway results, got %d", len(wr.GatewayResults))
	}

	// Check baseline results.
	for i, r := range wr.BaselineResults {
		if r.StatusCode != 200 {
			t.Errorf("baseline[%d]: expected status 200, got %d", i, r.StatusCode)
		}
		if r.InputTokens != 100 {
			t.Errorf("baseline[%d]: expected 100 input tokens, got %d", i, r.InputTokens)
		}
		if r.Latency <= 0 {
			t.Errorf("baseline[%d]: expected positive latency", i)
		}
		if r.Cost <= 0 {
			t.Errorf("baseline[%d]: expected positive cost", i)
		}
	}

	// Generate report.
	gen := report.NewGenerator()
	rpt, err := gen.Generate(results)
	if err != nil {
		t.Fatalf("report generation failed: %v", err)
	}

	if rpt.Name != "integration-test" {
		t.Errorf("expected report name 'integration-test', got %q", rpt.Name)
	}

	// Verify text and markdown report generation.
	text := report.FormatText(rpt)
	if text == "" {
		t.Error("text report should not be empty")
	}
	md := report.FormatMarkdown(rpt)
	if md == "" {
		t.Error("markdown report should not be empty")
	}

	// Run comparison.
	engine := comparison.NewEngine()
	comp, err := engine.Compare(rpt)
	if err != nil {
		t.Fatalf("comparison failed: %v", err)
	}

	if comp.Summary == "" {
		t.Error("comparison summary should not be empty")
	}

	// Export reports.
	exporter := report.NewExporter(cfg.OutputDir)
	if err := exporter.ExportAll(rpt, results); err != nil {
		t.Fatalf("export failed: %v", err)
	}
}

// TestIntegration_AllWorkloadTypes tests that all workload generators produce valid requests.
func TestIntegration_AllWorkloadTypes(t *testing.T) {
	registry := workloads.NewRegistry()

	workloadConfigs := []benchmark.WorkloadConfig{
		{Type: "repetitive", Weight: 1.0},
		{Type: "context_change", Weight: 1.0},
		{Type: "static_prefix", Weight: 1.0},
		{Type: "repository_context", Weight: 1.0},
		{Type: "random", Weight: 1.0},
	}

	for _, wc := range workloadConfigs {
		t.Run(wc.Type, func(t *testing.T) {
			gen, err := registry.Create(wc)
			if err != nil {
				t.Fatalf("failed to create generator for %q: %v", wc.Type, err)
			}

			if gen.Name() != wc.Type {
				t.Errorf("expected name %q, got %q", wc.Type, gen.Name())
			}

			// Generate several iterations.
			for i := 0; i < 5; i++ {
				req := gen.Generate(i)
				if len(req.Messages) == 0 {
					t.Errorf("iteration %d: expected non-empty messages", i)
				}
				// Verify last message is from user.
				lastMsg := req.Messages[len(req.Messages)-1]
				if lastMsg.Role != "user" {
					t.Errorf("iteration %d: expected last message role 'user', got %q", i, lastMsg.Role)
				}
				if lastMsg.Content == "" {
					t.Errorf("iteration %d: expected non-empty content", i)
				}
			}
		})
	}
}

// TestIntegration_FullPipeline tests the complete pipeline: run, report, compare, export.
func TestIntegration_FullPipeline(t *testing.T) {
	cfg := &benchmark.Config{
		Name:                     "full-pipeline-test",
		Description:              "Integration test of the full benchmark pipeline",
		ProviderEndpoint:         "http://localhost:9999",
		GatewayEndpoint:          "http://localhost:9998",
		Model:                    "gpt-4",
		Concurrency:              1,
		WarmupIterations:         0,
		Iterations:               5,
		Timeout:                  5 * time.Second,
		CooldownBetweenWorkloads: 0,
		OutputDir:                t.TempDir(),
		CostPerInputToken:        0.00003,
		CostPerOutputToken:       0.00006,
		Workloads: []benchmark.WorkloadConfig{
			{Type: "repetitive", Weight: 1.0},
			{Type: "random", Weight: 1.0},
		},
	}

	// Use mock senders.
	baseline := benchmark.NewMockSender(benchmark.MockSenderConfig{
		Mode:          benchmark.RunModeBaseline,
		Latency:       100 * time.Millisecond,
		InputTokens:   200,
		OutputTokens:  50,
		CostPerInput:  cfg.CostPerInputToken,
		CostPerOutput: cfg.CostPerOutputToken,
	})
	gw := benchmark.NewMockSender(benchmark.MockSenderConfig{
		Mode:          benchmark.RunModeGateway,
		Latency:       60 * time.Millisecond,
		InputTokens:   200,
		OutputTokens:  50,
		CachedTokens:  150,
		CostPerInput:  cfg.CostPerInputToken,
		CostPerOutput: cfg.CostPerOutputToken,
	})

	harness, err := benchmark.NewHarness(cfg, baseline, gw)
	if err != nil {
		t.Fatalf("harness creation failed: %v", err)
	}

	registry := workloads.NewRegistry()
	for _, wc := range cfg.Workloads {
		gen, _ := registry.Create(wc)
		harness.RegisterGenerator(gen)
	}

	// Step 1: Run.
	results, err := harness.Run(context.Background())
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(results.WorkloadResults) != 2 {
		t.Fatalf("expected 2 workload results, got %d", len(results.WorkloadResults))
	}

	// Step 2: Report.
	rptGen := report.NewGenerator()
	rpt, err := rptGen.Generate(results)
	if err != nil {
		t.Fatalf("report failed: %v", err)
	}

	if rpt.OverallLatencyImprovement <= 0 {
		t.Error("expected positive latency improvement")
	}

	// Step 3: Compare.
	engine := comparison.NewEngine()
	comp, err := engine.Compare(rpt)
	if err != nil {
		t.Fatalf("comparison failed: %v", err)
	}

	if !comp.Overall.LatencyAnalysis.Improved {
		t.Error("expected latency improvement in comparison")
	}
	if len(comp.Recommendations) == 0 {
		t.Error("expected recommendations")
	}

	// Step 4: Export.
	exporter := report.NewExporter(cfg.OutputDir)
	if err := exporter.ExportAll(rpt, results); err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// Verify analysis report generation.
	analysis := comparison.NewAnalysisReport(comp)
	text := analysis.Format()
	if text == "" {
		t.Error("analysis report should not be empty")
	}

	costText := comparison.CostBreakdown(comp)
	if costText == "" {
		t.Error("cost breakdown should not be empty")
	}
}
