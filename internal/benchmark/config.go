// Package benchmark provides a harness for measuring gateway optimization
// effectiveness against a baseline (direct-to-provider) implementation.
package benchmark

import (
	"errors"
	"fmt"
	"time"
)

// Config holds the full benchmark configuration.
type Config struct {
	// Name is a human-readable name for this benchmark run.
	Name string `json:"name"`
	// Description describes the benchmark scenario.
	Description string `json:"description,omitempty"`
	// ProviderEndpoint is the URL of the LLM provider (or mock).
	ProviderEndpoint string `json:"provider_endpoint"`
	// GatewayEndpoint is the URL of the optimization gateway.
	GatewayEndpoint string `json:"gateway_endpoint"`
	// APIKey is the API key for authentication.
	APIKey string `json:"api_key,omitempty"`
	// Model is the model to use for benchmark requests.
	Model string `json:"model"`
	// Workloads lists the workload types to execute.
	Workloads []WorkloadConfig `json:"workloads"`
	// Concurrency is the number of concurrent requests per workload.
	Concurrency int `json:"concurrency"`
	// WarmupIterations is the number of warmup iterations before measuring.
	WarmupIterations int `json:"warmup_iterations"`
	// Iterations is the number of measured iterations per workload.
	Iterations int `json:"iterations"`
	// Timeout is the per-request timeout.
	Timeout time.Duration `json:"timeout"`
	// CooldownBetweenWorkloads is the pause between different workloads.
	CooldownBetweenWorkloads time.Duration `json:"cooldown_between_workloads"`
	// OutputDir is the directory for writing reports.
	OutputDir string `json:"output_dir"`
	// CostPerInputToken is the cost per input token in USD.
	CostPerInputToken float64 `json:"cost_per_input_token"`
	// CostPerOutputToken is the cost per output token in USD.
	CostPerOutputToken float64 `json:"cost_per_output_token"`
}

// WorkloadConfig configures a single workload within the benchmark.
type WorkloadConfig struct {
	// Type is the workload type name.
	Type string `json:"type"`
	// Weight determines how many iterations this workload gets relative to others.
	Weight float64 `json:"weight"`
	// Params holds workload-specific parameters.
	Params map[string]interface{} `json:"params,omitempty"`
}

// Validate checks the config for required fields and constraints.
func (c *Config) Validate() error {
	var errs []error

	if c.Name == "" {
		errs = append(errs, errors.New("name is required"))
	}
	if c.ProviderEndpoint == "" {
		errs = append(errs, errors.New("provider_endpoint is required"))
	}
	if c.GatewayEndpoint == "" {
		errs = append(errs, errors.New("gateway_endpoint is required"))
	}
	if c.Model == "" {
		errs = append(errs, errors.New("model is required"))
	}
	if len(c.Workloads) == 0 {
		errs = append(errs, errors.New("at least one workload is required"))
	}
	if c.Concurrency < 1 {
		errs = append(errs, errors.New("concurrency must be >= 1"))
	}
	if c.Iterations < 1 {
		errs = append(errs, errors.New("iterations must be >= 1"))
	}
	if c.Timeout <= 0 {
		errs = append(errs, errors.New("timeout must be > 0"))
	}
	if c.CostPerInputToken < 0 {
		errs = append(errs, errors.New("cost_per_input_token must be >= 0"))
	}
	if c.CostPerOutputToken < 0 {
		errs = append(errs, errors.New("cost_per_output_token must be >= 0"))
	}

	for i, w := range c.Workloads {
		if w.Type == "" {
			errs = append(errs, fmt.Errorf("workload[%d]: type is required", i))
		}
		if w.Weight < 0 {
			errs = append(errs, fmt.Errorf("workload[%d]: weight must be >= 0", i))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation: %w", errors.Join(errs...))
	}
	return nil
}

// DefaultConfig returns a sensible default benchmark configuration.
func DefaultConfig() *Config {
	return &Config{
		Name:                     "default-benchmark",
		ProviderEndpoint:         "http://localhost:8081",
		GatewayEndpoint:          "http://localhost:8080",
		Model:                    "gpt-4",
		Concurrency:              1,
		WarmupIterations:         2,
		Iterations:               10,
		Timeout:                  30 * time.Second,
		CooldownBetweenWorkloads: 1 * time.Second,
		OutputDir:                "./benchmark-results",
		CostPerInputToken:        0.00003,
		CostPerOutputToken:       0.00006,
		Workloads: []WorkloadConfig{
			{Type: "repetitive", Weight: 1.0},
			{Type: "context_change", Weight: 1.0},
			{Type: "static_prefix", Weight: 1.0},
			{Type: "random", Weight: 1.0},
		},
	}
}
