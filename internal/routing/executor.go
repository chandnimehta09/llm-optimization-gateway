package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/llm-optimization-gateway/internal/request"
)

// ExecutorConfig configures the request executor.
type ExecutorConfig struct {
	// Timeout is the per-provider request timeout.
	Timeout time.Duration `json:"timeout"`
}

// DefaultExecutorConfig returns an ExecutorConfig with sensible defaults.
func DefaultExecutorConfig() ExecutorConfig {
	return ExecutorConfig{
		Timeout: 30 * time.Second,
	}
}

// Executor handles sending requests to providers, with optional timeout
// enforcement. It wraps the lower-level Provider.Send with timeout and
// error handling.
type Executor struct {
	registry *Registry
	config   ExecutorConfig
}

// NewExecutor creates a new Executor.
func NewExecutor(registry *Registry, config ExecutorConfig) *Executor {
	return &Executor{
		registry: registry,
		config:   config,
	}
}

// Execute sends a request to the named provider with timeout enforcement.
func (e *Executor) Execute(ctx context.Context, providerName string, req *request.NormalizedRequest) (*ProviderResponse, error) {
	provider, err := e.registry.Get(providerName)
	if err != nil {
		return nil, fmt.Errorf("executor: %w", err)
	}

	// Apply timeout if configured.
	execCtx := ctx
	if e.config.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, e.config.Timeout)
		defer cancel()
	}

	resp, err := provider.Send(execCtx, req)
	if err != nil {
		return nil, fmt.Errorf("executor: provider %s: %w", providerName, err)
	}

	return resp, nil
}

// ExecuteWithAttempt sends a request to the named provider and records the
// attempt in the routing context.
func (e *Executor) ExecuteWithAttempt(ctx context.Context, rc *RoutingContext, providerName string, attemptNum int, isFallback bool) (*ProviderResponse, error) {
	attempt := ProviderAttempt{
		Provider:      providerName,
		AttemptNumber: attemptNum,
		StartTime:     time.Now(),
		IsFallback:    isFallback,
	}

	resp, err := e.Execute(ctx, providerName, rc.Request)

	attempt.EndTime = time.Now()
	attempt.Duration = attempt.EndTime.Sub(attempt.StartTime)
	if resp != nil {
		attempt.StatusCode = resp.StatusCode
	}
	if err != nil {
		attempt.Error = err
		attempt.ErrorMessage = err.Error()
	}

	rc.RecordAttempt(attempt)

	return resp, err
}
