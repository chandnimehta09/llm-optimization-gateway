// Package main provides the benchmark CLI for running gateway optimization benchmarks.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/llm-optimization-gateway/internal/benchmark"
	"github.com/llm-optimization-gateway/internal/benchmark/comparison"
	"github.com/llm-optimization-gateway/internal/benchmark/report"
	"github.com/llm-optimization-gateway/internal/benchmark/workloads"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("benchmark failed: %v", err)
	}
}

func run() error {
	// Parse flags.
	configFile := flag.String("config", "", "Path to benchmark config JSON file")
	providerEndpoint := flag.String("provider", "http://localhost:8081", "Provider endpoint URL")
	gatewayEndpoint := flag.String("gateway", "http://localhost:8080", "Gateway endpoint URL")
	model := flag.String("model", "gpt-4", "Model to benchmark")
	apiKey := flag.String("api-key", "", "API key for authentication")
	iterations := flag.Int("iterations", 10, "Number of iterations per workload")
	concurrency := flag.Int("concurrency", 1, "Number of concurrent requests")
	outputDir := flag.String("output", "./benchmark-results", "Output directory for reports")
	timeout := flag.Duration("timeout", 30*time.Second, "Per-request timeout")
	flag.Parse()

	// Build or load config.
	var cfg *benchmark.Config
	if *configFile != "" {
		data, err := os.ReadFile(*configFile)
		if err != nil {
			return fmt.Errorf("read config: %w", err)
		}
		cfg = &benchmark.Config{}
		if err := json.Unmarshal(data, cfg); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
	} else {
		cfg = benchmark.DefaultConfig()
		cfg.ProviderEndpoint = *providerEndpoint
		cfg.GatewayEndpoint = *gatewayEndpoint
		cfg.Model = *model
		cfg.APIKey = *apiKey
		cfg.Iterations = *iterations
		cfg.Concurrency = *concurrency
		cfg.OutputDir = *outputDir
		cfg.Timeout = *timeout
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	fmt.Printf("Starting benchmark: %s\n", cfg.Name)
	fmt.Printf("  Provider:    %s\n", cfg.ProviderEndpoint)
	fmt.Printf("  Gateway:     %s\n", cfg.GatewayEndpoint)
	fmt.Printf("  Model:       %s\n", cfg.Model)
	fmt.Printf("  Iterations:  %d\n", cfg.Iterations)
	fmt.Printf("  Concurrency: %d\n", cfg.Concurrency)
	fmt.Printf("  Workloads:   %d\n", len(cfg.Workloads))
	fmt.Println()

	// Create senders.
	baselineSender := benchmark.NewHTTPSender(benchmark.HTTPSenderConfig{
		Endpoint:      cfg.ProviderEndpoint,
		APIKey:        cfg.APIKey,
		Model:         cfg.Model,
		Mode:          benchmark.RunModeBaseline,
		Timeout:       cfg.Timeout,
		CostPerInput:  cfg.CostPerInputToken,
		CostPerOutput: cfg.CostPerOutputToken,
	})

	gatewaySender := benchmark.NewHTTPSender(benchmark.HTTPSenderConfig{
		Endpoint:      cfg.GatewayEndpoint,
		APIKey:        cfg.APIKey,
		Model:         cfg.Model,
		Mode:          benchmark.RunModeGateway,
		Timeout:       cfg.Timeout,
		CostPerInput:  cfg.CostPerInputToken,
		CostPerOutput: cfg.CostPerOutputToken,
	})

	// Create harness.
	harness, err := benchmark.NewHarness(cfg, baselineSender, gatewaySender)
	if err != nil {
		return fmt.Errorf("create harness: %w", err)
	}

	// Register workload generators.
	registry := workloads.NewRegistry()
	for _, wc := range cfg.Workloads {
		gen, err := registry.Create(wc)
		if err != nil {
			return fmt.Errorf("create generator for %q: %w", wc.Type, err)
		}
		harness.RegisterGenerator(gen)
	}

	// Run with cancellation support.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		fmt.Println("\nInterrupted, stopping benchmark...")
		cancel()
	}()

	fmt.Println("Running benchmark...")
	results, err := harness.Run(ctx)
	if err != nil {
		return fmt.Errorf("run benchmark: %w", err)
	}

	// Generate report.
	gen := report.NewGenerator()
	rpt, err := gen.Generate(results)
	if err != nil {
		return fmt.Errorf("generate report: %w", err)
	}

	// Print text report.
	fmt.Println(report.FormatText(rpt))

	// Run comparison analysis.
	engine := comparison.NewEngine()
	comp, err := engine.Compare(rpt)
	if err != nil {
		return fmt.Errorf("comparison analysis: %w", err)
	}

	analysis := comparison.NewAnalysisReport(comp)
	fmt.Println(analysis.Format())

	// Export reports.
	exporter := report.NewExporter(cfg.OutputDir)
	if err := exporter.ExportAll(rpt, results); err != nil {
		return fmt.Errorf("export reports: %w", err)
	}

	fmt.Printf("Reports exported to %s/\n", cfg.OutputDir)
	return nil
}
