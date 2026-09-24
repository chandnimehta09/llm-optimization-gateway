package benchmark

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RunMode indicates whether the runner targets the provider directly or the gateway.
type RunMode string

const (
	// RunModeBaseline sends requests directly to the provider (no gateway).
	RunModeBaseline RunMode = "baseline"
	// RunModeGateway sends requests through the optimization gateway.
	RunModeGateway RunMode = "gateway"
)

// RequestResult holds the outcome of a single benchmark request.
type RequestResult struct {
	// WorkloadType identifies the workload that generated this request.
	WorkloadType string `json:"workload_type"`
	// Mode indicates baseline or gateway.
	Mode RunMode `json:"mode"`
	// Iteration is the iteration number.
	Iteration int `json:"iteration"`
	// Latency is the round-trip time for the request.
	Latency time.Duration `json:"latency_ns"`
	// InputTokens is the number of input tokens.
	InputTokens int `json:"input_tokens"`
	// OutputTokens is the number of output tokens.
	OutputTokens int `json:"output_tokens"`
	// CachedTokens is the number of tokens served from cache.
	CachedTokens int `json:"cached_tokens"`
	// Cost is the estimated cost in USD.
	Cost float64 `json:"cost"`
	// StatusCode is the HTTP status code returned.
	StatusCode int `json:"status_code"`
	// Error holds any error message.
	Error string `json:"error,omitempty"`
	// Timestamp is when the request was made.
	Timestamp time.Time `json:"timestamp"`
}

// WorkloadResults aggregates results for a single workload.
type WorkloadResults struct {
	// WorkloadType identifies the workload.
	WorkloadType string `json:"workload_type"`
	// BaselineResults holds results from the baseline runner.
	BaselineResults []RequestResult `json:"baseline_results"`
	// GatewayResults holds results from the gateway runner.
	GatewayResults []RequestResult `json:"gateway_results"`
}

// BenchmarkResults holds the full results of a benchmark run.
type BenchmarkResults struct {
	// Config is the benchmark configuration used.
	Config *Config `json:"config"`
	// StartTime is when the benchmark started.
	StartTime time.Time `json:"start_time"`
	// EndTime is when the benchmark finished.
	EndTime time.Time `json:"end_time"`
	// Duration is the total benchmark duration.
	Duration time.Duration `json:"duration_ns"`
	// WorkloadResults holds per-workload results.
	WorkloadResults []WorkloadResults `json:"workload_results"`
}

// WorkloadGenerator is the interface that workload generators must implement.
type WorkloadGenerator interface {
	// Name returns the workload type name.
	Name() string
	// Generate produces a sequence of prompts for the benchmark.
	Generate(iteration int) WorkloadRequest
}

// WorkloadRequest represents a single generated request for a workload.
type WorkloadRequest struct {
	// SystemPrompt is the system-level instruction.
	SystemPrompt string `json:"system_prompt,omitempty"`
	// Messages is the conversation history.
	Messages []WorkloadMessage `json:"messages"`
	// Model overrides the benchmark model if non-empty.
	Model string `json:"model,omitempty"`
}

// WorkloadMessage is a simplified message for workload generation.
type WorkloadMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Sender sends a request and returns the result. This abstraction allows
// the harness to target either the provider directly or the gateway.
type Sender interface {
	// Send executes a single benchmark request.
	Send(ctx context.Context, req WorkloadRequest) RequestResult
}

// Harness orchestrates benchmark execution across workloads and modes.
type Harness struct {
	config     *Config
	generators map[string]WorkloadGenerator
	baseline   Sender
	gateway    Sender
	mu         sync.Mutex
}

// NewHarness creates a new benchmark harness.
func NewHarness(config *Config, baseline, gateway Sender) (*Harness, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if baseline == nil {
		return nil, fmt.Errorf("baseline sender is required")
	}
	if gateway == nil {
		return nil, fmt.Errorf("gateway sender is required")
	}
	return &Harness{
		config:     config,
		generators: make(map[string]WorkloadGenerator),
		baseline:   baseline,
		gateway:    gateway,
	}, nil
}

// RegisterGenerator registers a workload generator.
func (h *Harness) RegisterGenerator(gen WorkloadGenerator) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.generators[gen.Name()] = gen
}

// Run executes the full benchmark suite.
func (h *Harness) Run(ctx context.Context) (*BenchmarkResults, error) {
	if err := h.config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	results := &BenchmarkResults{
		Config:    h.config,
		StartTime: time.Now(),
	}

	for _, wc := range h.config.Workloads {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		gen, ok := h.generators[wc.Type]
		if !ok {
			return nil, fmt.Errorf("no generator registered for workload type %q", wc.Type)
		}

		wr, err := h.runWorkload(ctx, gen, wc)
		if err != nil {
			return nil, fmt.Errorf("workload %q failed: %w", wc.Type, err)
		}
		results.WorkloadResults = append(results.WorkloadResults, *wr)

		// Cooldown between workloads.
		if h.config.CooldownBetweenWorkloads > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(h.config.CooldownBetweenWorkloads):
			}
		}
	}

	results.EndTime = time.Now()
	results.Duration = results.EndTime.Sub(results.StartTime)
	return results, nil
}

// runWorkload runs a single workload against both baseline and gateway.
func (h *Harness) runWorkload(ctx context.Context, gen WorkloadGenerator, wc WorkloadConfig) (*WorkloadResults, error) {
	wr := &WorkloadResults{
		WorkloadType: wc.Type,
	}

	// Warmup phase for baseline.
	for i := 0; i < h.config.WarmupIterations; i++ {
		req := gen.Generate(i)
		h.baseline.Send(ctx, req)
	}

	// Measured baseline phase.
	baselineResults, err := h.runIterations(ctx, h.baseline, gen, RunModeBaseline)
	if err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	wr.BaselineResults = baselineResults

	// Warmup phase for gateway.
	for i := 0; i < h.config.WarmupIterations; i++ {
		req := gen.Generate(i)
		h.gateway.Send(ctx, req)
	}

	// Measured gateway phase.
	gatewayResults, err := h.runIterations(ctx, h.gateway, gen, RunModeGateway)
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}
	wr.GatewayResults = gatewayResults

	return wr, nil
}

// runIterations runs a set of iterations using the given sender.
func (h *Harness) runIterations(ctx context.Context, sender Sender, gen WorkloadGenerator, mode RunMode) ([]RequestResult, error) {
	iterations := h.config.Iterations
	results := make([]RequestResult, 0, iterations)

	if h.config.Concurrency <= 1 {
		// Sequential execution.
		for i := 0; i < iterations; i++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			req := gen.Generate(i)
			result := sender.Send(ctx, req)
			result.WorkloadType = gen.Name()
			result.Mode = mode
			result.Iteration = i
			results = append(results, result)
		}
		return results, nil
	}

	// Concurrent execution.
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, h.config.Concurrency)
	errCh := make(chan error, 1)

	for i := 0; i < iterations; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err := <-errCh:
			return nil, err
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(iteration int) {
			defer wg.Done()
			defer func() { <-sem }()

			req := gen.Generate(iteration)
			result := sender.Send(ctx, req)
			result.WorkloadType = gen.Name()
			result.Mode = mode
			result.Iteration = iteration

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	return results, nil
}
