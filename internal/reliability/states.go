package reliability

import (
	"fmt"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed means the circuit is healthy and requests flow normally.
	CircuitClosed CircuitState = iota
	// CircuitOpen means the circuit has tripped and requests are rejected.
	CircuitOpen
	// CircuitHalfOpen means the circuit is testing recovery with limited traffic.
	CircuitHalfOpen
)

// String returns the string representation of a CircuitState.
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

// StateTransition records a circuit breaker state change.
type StateTransition struct {
	Provider  string       `json:"provider"`
	From      CircuitState `json:"from"`
	To        CircuitState `json:"to"`
	Timestamp time.Time    `json:"timestamp"`
	Reason    string       `json:"reason"`
}

// String returns a human-readable representation of a state transition.
func (t StateTransition) String() string {
	return fmt.Sprintf("[%s] %s: %s -> %s (%s)",
		t.Timestamp.Format(time.RFC3339), t.Provider, t.From, t.To, t.Reason)
}

// CircuitSnapshot captures the current state of a circuit breaker at a point in time.
type CircuitSnapshot struct {
	Provider            string        `json:"provider"`
	State               CircuitState  `json:"state"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	TotalSuccesses      int64         `json:"total_successes"`
	TotalFailures       int64         `json:"total_failures"`
	TotalRejected       int64         `json:"total_rejected"`
	LastFailureTime     time.Time     `json:"last_failure_time,omitempty"`
	LastSuccessTime     time.Time     `json:"last_success_time,omitempty"`
	LastStateChange     time.Time     `json:"last_state_change"`
	ResetTimeout        time.Duration `json:"reset_timeout"`
	Timestamp           time.Time     `json:"timestamp"`
}

// JitterMode defines the type of jitter applied to backoff delays.
type JitterMode string

const (
	// JitterNone applies no jitter.
	JitterNone JitterMode = "none"
	// JitterFull applies full jitter: random value in [0, delay).
	JitterFull JitterMode = "full"
	// JitterDecorrelated applies decorrelated jitter: random value in [base, prev * 3).
	JitterDecorrelated JitterMode = "decorrelated"
)

// RetryMetrics tracks retry-related metrics for a provider.
type RetryMetrics struct {
	Provider       string        `json:"provider"`
	TotalAttempts  int64         `json:"total_attempts"`
	TotalRetries   int64         `json:"total_retries"`
	TotalSuccesses int64         `json:"total_successes"`
	TotalFailures  int64         `json:"total_failures"`
	TotalExhausted int64         `json:"total_exhausted"`
	TotalBudgetHit int64         `json:"total_budget_hit"`
	AvgLatency     time.Duration `json:"avg_latency"`
}

// SuccessRate returns the ratio of successes to total attempts.
func (m RetryMetrics) SuccessRate() float64 {
	if m.TotalAttempts == 0 {
		return 0
	}
	return float64(m.TotalSuccesses) / float64(m.TotalAttempts)
}

// TimeoutEvent records a timeout occurrence.
type TimeoutEvent struct {
	Provider   string        `json:"provider"`
	Phase      TimeoutPhase  `json:"phase"`
	Configured time.Duration `json:"configured"`
	Elapsed    time.Duration `json:"elapsed"`
	Timestamp  time.Time     `json:"timestamp"`
}

// String returns a human-readable representation of a timeout event.
func (e TimeoutEvent) String() string {
	return fmt.Sprintf("[%s] %s timeout on %s: configured=%s elapsed=%s",
		e.Timestamp.Format(time.RFC3339), e.Phase, e.Provider, e.Configured, e.Elapsed)
}
