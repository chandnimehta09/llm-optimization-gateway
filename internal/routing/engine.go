package routing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
)

// EngineConfig configures the routing engine.
type EngineConfig struct {
	// Executor configures per-provider request execution.
	Executor ExecutorConfig `json:"executor"`
	// Fallback configures the fallback chain.
	Fallback FallbackChainConfig `json:"fallback"`
	// HealthCheck configures provider health checking.
	HealthCheck HealthCheckConfig `json:"health_check"`
}

// DefaultEngineConfig returns an EngineConfig with sensible defaults.
func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		Executor:    DefaultExecutorConfig(),
		Fallback:    DefaultFallbackChainConfig(),
		HealthCheck: DefaultHealthCheckConfig(),
	}
}

// RoutingMetrics records metrics about routing operations.
type RoutingMetrics struct {
	mu sync.Mutex
	// TotalRequests is the total number of requests routed.
	TotalRequests int64
	// SuccessfulRequests is the number of requests that succeeded.
	SuccessfulRequests int64
	// FailedRequests is the number of requests that failed after exhausting all providers.
	FailedRequests int64
	// FallbackCount is the total number of fallback events.
	FallbackCount int64
	// ProviderAttempts maps provider names to attempt counts.
	ProviderAttempts map[string]int64
	// ProviderErrors maps provider names to error counts.
	ProviderErrors map[string]int64
}

// NewRoutingMetrics creates a new RoutingMetrics.
func NewRoutingMetrics() *RoutingMetrics {
	return &RoutingMetrics{
		ProviderAttempts: make(map[string]int64),
		ProviderErrors:   make(map[string]int64),
	}
}

// RecordRequest records a request routing outcome.
func (rm *RoutingMetrics) RecordRequest(success bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.TotalRequests++
	if success {
		rm.SuccessfulRequests++
	} else {
		rm.FailedRequests++
	}
}

// RecordAttempt records a provider attempt.
func (rm *RoutingMetrics) RecordAttempt(provider string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.ProviderAttempts[provider]++
}

// RecordError records a provider error.
func (rm *RoutingMetrics) RecordError(provider string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.ProviderErrors[provider]++
}

// RecordFallback records a fallback event.
func (rm *RoutingMetrics) RecordFallback() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.FallbackCount++
}

// Snapshot returns a copy of the current metrics.
func (rm *RoutingMetrics) Snapshot() RoutingMetrics {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	snap := RoutingMetrics{
		TotalRequests:      rm.TotalRequests,
		SuccessfulRequests: rm.SuccessfulRequests,
		FailedRequests:     rm.FailedRequests,
		FallbackCount:      rm.FallbackCount,
		ProviderAttempts:   make(map[string]int64, len(rm.ProviderAttempts)),
		ProviderErrors:     make(map[string]int64, len(rm.ProviderErrors)),
	}
	for k, v := range rm.ProviderAttempts {
		snap.ProviderAttempts[k] = v
	}
	for k, v := range rm.ProviderErrors {
		snap.ProviderErrors[k] = v
	}
	return snap
}

// Engine is the main routing engine. It accepts policy decisions, selects
// providers from the registry, executes requests with fallback support,
// and records metrics.
type Engine struct {
	registry *Registry
	executor *Executor
	config   EngineConfig
	metrics  *RoutingMetrics
	onEvent  func(FallbackEvent)
	mu       sync.RWMutex
}

// NewEngine creates a new routing Engine.
func NewEngine(registry *Registry, config EngineConfig) *Engine {
	return &Engine{
		registry: registry,
		executor: NewExecutor(registry, config.Executor),
		config:   config,
		metrics:  NewRoutingMetrics(),
	}
}

// SetFallbackEventHandler sets a callback invoked on each fallback event.
func (e *Engine) SetFallbackEventHandler(handler func(FallbackEvent)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onEvent = handler
}

// Registry returns the engine's provider registry.
func (e *Engine) Registry() *Registry {
	return e.registry
}

// Metrics returns the engine's routing metrics.
func (e *Engine) Metrics() *RoutingMetrics {
	return e.metrics
}

// Route executes a request based on the given policy decision. It selects
// the provider, executes the request, handles fallback on failure, and
// records metrics.
func (e *Engine) Route(ctx context.Context, req *request.NormalizedRequest, decision *policy.PolicyDecision) (*ProviderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("routing engine: request must not be nil")
	}
	if decision == nil {
		return nil, fmt.Errorf("routing engine: decision must not be nil")
	}

	rc := NewRoutingContext(req, decision)
	ctx = WithRoutingContext(ctx, rc)

	primaryProvider := decision.SelectedProvider

	// Build fallback chain from registry's providers.
	chain := e.buildFallbackChain(primaryProvider)

	e.mu.RLock()
	onEvent := e.onEvent
	e.mu.RUnlock()

	// Create a fallback-aware event handler that also records metrics.
	eventHandler := func(event FallbackEvent) {
		e.metrics.RecordFallback()
		if onEvent != nil {
			onEvent(event)
		}
	}

	fallbackExec := NewFallbackExecutor(e.registry, chain, eventHandler)

	result, err := fallbackExec.Execute(ctx, rc, primaryProvider)

	// Record metrics for all attempts.
	for _, attempt := range rc.Attempts {
		e.metrics.RecordAttempt(attempt.Provider)
		if attempt.Error != nil {
			e.metrics.RecordError(attempt.Provider)
		}
	}

	if err != nil {
		e.metrics.RecordRequest(false)
		rc.Finish(err)
		return nil, err
	}

	e.metrics.RecordRequest(true)
	rc.Finish(nil)

	if result != nil && result.Response != nil {
		return result.Response, nil
	}

	return nil, fmt.Errorf("routing engine: no response received")
}

// buildFallbackChain constructs a FallbackChain with the primary provider
// first, followed by other healthy registered providers.
func (e *Engine) buildFallbackChain(primaryProvider string) *FallbackChain {
	providers := e.registry.ListProviders()

	// Build priorities: primary first (priority 0), others in order.
	priorities := make([]ProviderPriority, 0, len(providers))
	idx := 1
	for _, name := range providers {
		if name == primaryProvider {
			priorities = append(priorities, ProviderPriority{
				Provider: name,
				Priority: 0,
			})
		} else {
			priorities = append(priorities, ProviderPriority{
				Provider: name,
				Priority: idx,
			})
			idx++
		}
	}

	// If primary wasn't in the list, add it at the front.
	hasPrimary := false
	for _, p := range priorities {
		if p.Provider == primaryProvider {
			hasPrimary = true
			break
		}
	}
	if !hasPrimary && primaryProvider != "" {
		priorities = append([]ProviderPriority{{
			Provider: primaryProvider,
			Priority: 0,
		}}, priorities...)
	}

	pl := NewPriorityList(priorities)
	return NewFallbackChain(pl, e.config.Fallback)
}

// RouteSimple executes a request on a single provider without fallback.
func (e *Engine) RouteSimple(ctx context.Context, req *request.NormalizedRequest, providerName string) (*ProviderResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("routing engine: request must not be nil")
	}
	if providerName == "" {
		return nil, fmt.Errorf("routing engine: provider name must not be empty")
	}

	rc := NewRoutingContext(req, &policy.PolicyDecision{
		SelectedProvider: providerName,
		SelectedModel:    req.Model,
		PolicyVersion:    "simple",
		Confidence:       1.0,
		EvaluatedAt:      time.Now(),
	})
	ctx = WithRoutingContext(ctx, rc)

	e.metrics.RecordAttempt(providerName)
	resp, err := e.executor.Execute(ctx, providerName, req)
	if err != nil {
		e.metrics.RecordError(providerName)
		e.metrics.RecordRequest(false)
		rc.Finish(err)
		return nil, err
	}

	e.metrics.RecordRequest(true)
	rc.Finish(nil)
	return resp, nil
}

// String returns a summary of the engine state.
func (e *Engine) String() string {
	snap := e.metrics.Snapshot()
	return fmt.Sprintf("routing_engine: providers=%d total_requests=%d success=%d failed=%d fallbacks=%d",
		e.registry.Size(), snap.TotalRequests, snap.SuccessfulRequests, snap.FailedRequests, snap.FallbackCount)
}
