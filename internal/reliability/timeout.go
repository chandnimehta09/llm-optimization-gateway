package reliability

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// TimeoutConfig configures timeouts for request phases.
type TimeoutConfig struct {
	// Phases maps each timeout phase to its duration.
	Phases map[TimeoutPhase]time.Duration `json:"phases"`
	// ProviderOverrides maps provider names to per-provider timeout overrides.
	ProviderOverrides map[string]map[TimeoutPhase]time.Duration `json:"provider_overrides,omitempty"`
	// ModelOverrides maps model names to per-model timeout overrides.
	ModelOverrides map[string]map[TimeoutPhase]time.Duration `json:"model_overrides,omitempty"`
}

// DefaultTimeoutConfig returns a TimeoutConfig with sensible defaults.
func DefaultTimeoutConfig() TimeoutConfig {
	return TimeoutConfig{
		Phases:            DefaultPhaseTimeouts(),
		ProviderOverrides: make(map[string]map[TimeoutPhase]time.Duration),
		ModelOverrides:    make(map[string]map[TimeoutPhase]time.Duration),
	}
}

// Validate checks the timeout configuration for errors.
func (c TimeoutConfig) Validate() error {
	var errs []error
	for phase, d := range c.Phases {
		if !phase.IsValid() {
			errs = append(errs, fmt.Errorf("unknown phase: %s", phase))
		}
		if d < 0 {
			errs = append(errs, fmt.Errorf("phase %s: timeout must be >= 0", phase))
		}
	}
	for provider, overrides := range c.ProviderOverrides {
		for phase, d := range overrides {
			if !phase.IsValid() {
				errs = append(errs, fmt.Errorf("provider %s: unknown phase %s", provider, phase))
			}
			if d < 0 {
				errs = append(errs, fmt.Errorf("provider %s phase %s: timeout must be >= 0", provider, phase))
			}
		}
	}
	for model, overrides := range c.ModelOverrides {
		for phase, d := range overrides {
			if !phase.IsValid() {
				errs = append(errs, fmt.Errorf("model %s: unknown phase %s", model, phase))
			}
			if d < 0 {
				errs = append(errs, fmt.Errorf("model %s phase %s: timeout must be >= 0", model, phase))
			}
		}
	}
	return errors.Join(errs...)
}

// ResolveTimeout resolves the effective timeout for a phase, considering overrides.
// Priority: request-level > model > provider > default.
func (c TimeoutConfig) ResolveTimeout(phase TimeoutPhase, provider, model string, requestOverride *time.Duration) time.Duration {
	// Request-level override takes highest priority.
	if requestOverride != nil {
		return *requestOverride
	}

	// Model override.
	if model != "" {
		if overrides, ok := c.ModelOverrides[model]; ok {
			if d, ok := overrides[phase]; ok {
				return d
			}
		}
	}

	// Provider override.
	if provider != "" {
		if overrides, ok := c.ProviderOverrides[provider]; ok {
			if d, ok := overrides[phase]; ok {
				return d
			}
		}
	}

	// Default.
	if d, ok := c.Phases[phase]; ok {
		return d
	}

	return 0
}

// TimeoutManager applies and tracks timeouts for request phases.
type TimeoutManager struct {
	config TimeoutConfig
	mu     sync.RWMutex
	events []TimeoutEvent
	count  int64
}

// NewTimeoutManager creates a new timeout manager.
func NewTimeoutManager(config TimeoutConfig) *TimeoutManager {
	return &TimeoutManager{
		config: config,
		events: make([]TimeoutEvent, 0),
	}
}

// WithTimeout wraps a context with the appropriate timeout for the given phase.
// It returns the derived context, a cancel function, and the configured timeout duration.
func (m *TimeoutManager) WithTimeout(ctx context.Context, phase TimeoutPhase, provider, model string, requestOverride *time.Duration) (context.Context, context.CancelFunc, time.Duration) {
	timeout := m.config.ResolveTimeout(phase, provider, model, requestOverride)
	if timeout <= 0 {
		return ctx, func() {}, 0
	}

	// Respect existing deadline if it's sooner.
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	return ctx, cancel, timeout
}

// Execute runs an operation with the configured timeout for the given phase.
// If the operation exceeds the timeout, a TimeoutError is returned.
func (m *TimeoutManager) Execute(ctx context.Context, phase TimeoutPhase, provider, model string, requestOverride *time.Duration, op func(ctx context.Context) error) error {
	timeout := m.config.ResolveTimeout(phase, provider, model, requestOverride)
	if timeout <= 0 {
		return op(ctx)
	}

	// Respect existing deadline if it's sooner.
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := op(ctx)
	elapsed := time.Since(start)

	if err != nil && ctx.Err() == context.DeadlineExceeded {
		event := TimeoutEvent{
			Provider:   provider,
			Phase:      phase,
			Configured: timeout,
			Elapsed:    elapsed,
			Timestamp:  time.Now(),
		}
		m.recordEvent(event)
		atomic.AddInt64(&m.count, 1)
		return &TimeoutError{
			Provider: provider,
			Phase:    phase,
			Timeout:  timeout,
			Err:      err,
		}
	}

	return err
}

func (m *TimeoutManager) recordEvent(event TimeoutEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
}

// Events returns all recorded timeout events.
func (m *TimeoutManager) Events() []TimeoutEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	events := make([]TimeoutEvent, len(m.events))
	copy(events, m.events)
	return events
}

// TimeoutCount returns the number of timeouts that have occurred.
func (m *TimeoutManager) TimeoutCount() int64 {
	return atomic.LoadInt64(&m.count)
}

// ClearEvents clears all recorded timeout events.
func (m *TimeoutManager) ClearEvents() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = m.events[:0]
}

// UpdateConfig replaces the timeout configuration.
func (m *TimeoutManager) UpdateConfig(config TimeoutConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
}
