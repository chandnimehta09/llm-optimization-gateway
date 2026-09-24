package reliability

import (
	"fmt"
	"sync"
	"time"
)

// CircuitBreaker implements the circuit breaker pattern for a single provider.
type CircuitBreaker struct {
	mu                  sync.RWMutex
	provider            string
	config              CircuitBreakerConfig
	state               CircuitState
	consecutiveFailures int
	halfOpenSuccesses   int
	halfOpenAttempts    int
	totalSuccesses      int64
	totalFailures       int64
	totalRejected       int64
	lastFailureTime     time.Time
	lastSuccessTime     time.Time
	lastStateChange     time.Time
	transitions         []StateTransition
	nowFunc             func() time.Time // for testing
}

// NewCircuitBreaker creates a new circuit breaker for the given provider.
func NewCircuitBreaker(provider string, config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = DefaultFailureThreshold
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = DefaultResetTimeout
	}
	if config.HalfOpenMaxAttempts <= 0 {
		config.HalfOpenMaxAttempts = DefaultHalfOpenMaxAttempts
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = DefaultSuccessThreshold
	}

	return &CircuitBreaker{
		provider:        provider,
		config:          config,
		state:           CircuitClosed,
		lastStateChange: time.Now(),
		transitions:     make([]StateTransition, 0),
		nowFunc:         time.Now,
	}
}

// State returns the current circuit breaker state, transitioning from open
// to half-open if the reset timeout has elapsed.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.stateLocked()
}

func (cb *CircuitBreaker) stateLocked() CircuitState {
	if cb.state == CircuitOpen {
		if cb.nowFunc().Sub(cb.lastStateChange) >= cb.config.ResetTimeout {
			cb.transitionLocked(CircuitHalfOpen, "reset timeout elapsed")
		}
	}
	return cb.state
}

// Allow checks whether a request is allowed through. Returns false if the circuit
// is open; increments the rejected counter.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.stateLocked()
	switch state {
	case CircuitOpen:
		cb.totalRejected++
		return false
	case CircuitHalfOpen:
		// Allow limited traffic in half-open state.
		if cb.halfOpenAttempts >= cb.config.HalfOpenMaxAttempts {
			cb.totalRejected++
			return false
		}
		cb.halfOpenAttempts++
		return true
	default:
		return true
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.totalSuccesses++
	cb.consecutiveFailures = 0
	cb.lastSuccessTime = cb.nowFunc()

	if cb.state == CircuitHalfOpen {
		cb.halfOpenSuccesses++
		if cb.halfOpenSuccesses >= cb.config.SuccessThreshold {
			cb.transitionLocked(CircuitClosed, fmt.Sprintf("successful probes (%d) reached threshold (%d)",
				cb.halfOpenSuccesses, cb.config.SuccessThreshold))
			cb.halfOpenSuccesses = 0
			cb.halfOpenAttempts = 0
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.totalFailures++
	cb.consecutiveFailures++
	cb.lastFailureTime = cb.nowFunc()

	switch cb.state {
	case CircuitClosed:
		if cb.consecutiveFailures >= cb.config.FailureThreshold {
			cb.transitionLocked(CircuitOpen, fmt.Sprintf("consecutive failures (%d) reached threshold (%d)",
				cb.consecutiveFailures, cb.config.FailureThreshold))
		}
	case CircuitHalfOpen:
		cb.transitionLocked(CircuitOpen, "failure during half-open probe")
		cb.halfOpenSuccesses = 0
		cb.halfOpenAttempts = 0
	}
}

func (cb *CircuitBreaker) transitionLocked(to CircuitState, reason string) {
	from := cb.state
	if from == to {
		return
	}
	transition := StateTransition{
		Provider:  cb.provider,
		From:      from,
		To:        to,
		Timestamp: cb.nowFunc(),
		Reason:    reason,
	}
	cb.transitions = append(cb.transitions, transition)
	cb.state = to
	cb.lastStateChange = cb.nowFunc()

	// Reset half-open counters when entering half-open state.
	if to == CircuitHalfOpen {
		cb.halfOpenAttempts = 0
		cb.halfOpenSuccesses = 0
	}

	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.provider, from, to)
	}
}

// Reset manually resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state != CircuitClosed {
		cb.transitionLocked(CircuitClosed, "manual reset")
	}
	cb.consecutiveFailures = 0
	cb.halfOpenSuccesses = 0
	cb.halfOpenAttempts = 0
}

// ForceOpen forces the circuit into open state.
func (cb *CircuitBreaker) ForceOpen() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state != CircuitOpen {
		cb.transitionLocked(CircuitOpen, "forced open")
	}
}

// ForceClose forces the circuit into closed state.
func (cb *CircuitBreaker) ForceClose() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state != CircuitClosed {
		cb.transitionLocked(CircuitClosed, "forced close")
	}
	cb.consecutiveFailures = 0
	cb.halfOpenSuccesses = 0
	cb.halfOpenAttempts = 0
}

// Provider returns the provider name.
func (cb *CircuitBreaker) Provider() string {
	return cb.provider
}

// Snapshot returns the current state snapshot.
func (cb *CircuitBreaker) Snapshot() CircuitSnapshot {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return CircuitSnapshot{
		Provider:            cb.provider,
		State:               cb.state,
		ConsecutiveFailures: cb.consecutiveFailures,
		TotalSuccesses:      cb.totalSuccesses,
		TotalFailures:       cb.totalFailures,
		TotalRejected:       cb.totalRejected,
		LastFailureTime:     cb.lastFailureTime,
		LastSuccessTime:     cb.lastSuccessTime,
		LastStateChange:     cb.lastStateChange,
		ResetTimeout:        cb.config.ResetTimeout,
		Timestamp:           cb.nowFunc(),
	}
}

// Transitions returns all recorded state transitions.
func (cb *CircuitBreaker) Transitions() []StateTransition {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	r := make([]StateTransition, len(cb.transitions))
	copy(r, cb.transitions)
	return r
}

// Counters returns (successes, failures, rejected).
func (cb *CircuitBreaker) Counters() (int64, int64, int64) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.totalSuccesses, cb.totalFailures, cb.totalRejected
}

// ConsecutiveFailures returns the current consecutive failure count.
func (cb *CircuitBreaker) ConsecutiveFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.consecutiveFailures
}

// CircuitBreakerRegistry manages per-provider circuit breakers.
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
}

// NewCircuitBreakerRegistry creates a new registry with the given default config.
func NewCircuitBreakerRegistry(config CircuitBreakerConfig) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// Get returns the circuit breaker for a provider, creating one if it doesn't exist.
func (r *CircuitBreakerRegistry) Get(provider string) *CircuitBreaker {
	r.mu.RLock()
	cb, ok := r.breakers[provider]
	r.mu.RUnlock()
	if ok {
		return cb
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock.
	if cb, ok = r.breakers[provider]; ok {
		return cb
	}

	cb = NewCircuitBreaker(provider, r.config)
	r.breakers[provider] = cb
	return cb
}

// Allow checks whether the given provider's circuit breaker allows a request.
func (r *CircuitBreakerRegistry) Allow(provider string) bool {
	return r.Get(provider).Allow()
}

// RecordSuccess records a successful request for the given provider.
func (r *CircuitBreakerRegistry) RecordSuccess(provider string) {
	r.Get(provider).RecordSuccess()
}

// RecordFailure records a failed request for the given provider.
func (r *CircuitBreakerRegistry) RecordFailure(provider string) {
	r.Get(provider).RecordFailure()
}

// Reset resets the circuit breaker for the given provider.
func (r *CircuitBreakerRegistry) Reset(provider string) {
	r.mu.RLock()
	cb, ok := r.breakers[provider]
	r.mu.RUnlock()
	if ok {
		cb.Reset()
	}
}

// ResetAll resets all circuit breakers.
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, cb := range r.breakers {
		cb.Reset()
	}
}

// ForceOpen forces the circuit breaker for a provider into open state.
func (r *CircuitBreakerRegistry) ForceOpen(provider string) {
	r.Get(provider).ForceOpen()
}

// ForceClose forces the circuit breaker for a provider into closed state.
func (r *CircuitBreakerRegistry) ForceClose(provider string) {
	r.Get(provider).ForceClose()
}

// Snapshots returns a snapshot for every registered provider.
func (r *CircuitBreakerRegistry) Snapshots() map[string]CircuitSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshots := make(map[string]CircuitSnapshot, len(r.breakers))
	for name, cb := range r.breakers {
		snapshots[name] = cb.Snapshot()
	}
	return snapshots
}

// Providers returns the list of registered provider names.
func (r *CircuitBreakerRegistry) Providers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.breakers))
	for name := range r.breakers {
		names = append(names, name)
	}
	return names
}
