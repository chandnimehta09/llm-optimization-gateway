package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

type CircuitBreakerConfig struct {
	FailureThreshold    int
	ResetTimeout        time.Duration
	HalfOpenMaxAttempts int
	OnStateChange       func(from, to CircuitState)
}

func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{FailureThreshold: 5, ResetTimeout: 30 * time.Second, HalfOpenMaxAttempts: 1}
}

type StateTransition struct {
	From, To  CircuitState
	Timestamp time.Time
	Reason    string
}

type CircuitBreaker struct {
	mu                  sync.RWMutex
	config              CircuitBreakerConfig
	state               CircuitState
	consecutiveFailures int
	lastFailureTime     time.Time
	lastStateChange     time.Time
	transitions         []StateTransition
	halfOpenSuccesses   int
	totalSuccesses      int64
	totalFailures       int64
	totalBypasses       int64
}

func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.ResetTimeout <= 0 {
		config.ResetTimeout = 30 * time.Second
	}
	if config.HalfOpenMaxAttempts <= 0 {
		config.HalfOpenMaxAttempts = 1
	}
	return &CircuitBreaker{config: config, state: CircuitClosed, lastStateChange: time.Now()}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.stateLocked()
}

func (cb *CircuitBreaker) stateLocked() CircuitState {
	if cb.state == CircuitOpen && time.Since(cb.lastStateChange) >= cb.config.ResetTimeout {
		cb.transitionLocked(CircuitHalfOpen, "reset timeout elapsed")
	}
	return cb.state
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	switch cb.stateLocked() {
	case CircuitOpen:
		cb.totalBypasses++
		return false
	default:
		return true
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.totalSuccesses++
	cb.consecutiveFailures = 0
	if cb.state == CircuitHalfOpen {
		cb.halfOpenSuccesses++
		if cb.halfOpenSuccesses >= cb.config.HalfOpenMaxAttempts {
			cb.transitionLocked(CircuitClosed, "successful probe in half-open state")
			cb.halfOpenSuccesses = 0
		}
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.totalFailures++
	cb.consecutiveFailures++
	cb.lastFailureTime = time.Now()
	switch cb.state {
	case CircuitClosed:
		if cb.consecutiveFailures >= cb.config.FailureThreshold {
			cb.transitionLocked(CircuitOpen, fmt.Sprintf("consecutive failures (%d) reached threshold (%d)", cb.consecutiveFailures, cb.config.FailureThreshold))
		}
	case CircuitHalfOpen:
		cb.transitionLocked(CircuitOpen, "failure during half-open probe")
		cb.halfOpenSuccesses = 0
	}
}

func (cb *CircuitBreaker) transitionLocked(to CircuitState, reason string) {
	from := cb.state
	if from == to {
		return
	}
	cb.transitions = append(cb.transitions, StateTransition{From: from, To: to, Timestamp: time.Now(), Reason: reason})
	cb.state = to
	cb.lastStateChange = time.Now()
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(from, to)
	}
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state != CircuitClosed {
		cb.transitionLocked(CircuitClosed, "manual reset")
	}
	cb.consecutiveFailures = 0
	cb.halfOpenSuccesses = 0
}

func (cb *CircuitBreaker) Transitions() []StateTransition {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	r := make([]StateTransition, len(cb.transitions))
	copy(r, cb.transitions)
	return r
}

func (cb *CircuitBreaker) Counters() (int64, int64, int64) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.totalSuccesses, cb.totalFailures, cb.totalBypasses
}

func (cb *CircuitBreaker) ConsecutiveFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.consecutiveFailures
}

func (cb *CircuitBreaker) Execute(ctx context.Context, op func(context.Context) error, fb func(context.Context) error) error {
	if !cb.Allow() {
		if fb != nil {
			return fb(ctx)
		}
		return nil
	}
	if err := op(ctx); err != nil {
		cb.RecordFailure()
		if fb != nil {
			return fb(ctx)
		}
		return nil
	}
	cb.RecordSuccess()
	return nil
}
