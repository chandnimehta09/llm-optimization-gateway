package reliability

import "time"

// Default timeout values for each phase.
const (
	DefaultConnectionTimeout = 5 * time.Second
	DefaultDNSTimeout        = 2 * time.Second
	DefaultTLSTimeout        = 5 * time.Second
	DefaultReadTimeout       = 30 * time.Second
	DefaultWriteTimeout      = 10 * time.Second
	DefaultTotalTimeout      = 60 * time.Second
)

// Default retry values.
const (
	DefaultMaxAttempts  = 3
	DefaultInitialDelay = 100 * time.Millisecond
	DefaultMaxDelay     = 10 * time.Second
	DefaultMultiplier   = 2.0
)

// Default circuit breaker values.
const (
	DefaultFailureThreshold    = 5
	DefaultResetTimeout        = 30 * time.Second
	DefaultHalfOpenMaxAttempts = 3
	DefaultSuccessThreshold    = 2
)

// Default retry budget values.
const (
	DefaultBudgetMaxRetries = 100
	DefaultBudgetWindow     = 10 * time.Second
)

// DefaultPhaseTimeouts returns the default timeout for each phase.
func DefaultPhaseTimeouts() map[TimeoutPhase]time.Duration {
	return map[TimeoutPhase]time.Duration{
		PhaseConnection: DefaultConnectionTimeout,
		PhaseDNS:        DefaultDNSTimeout,
		PhaseTLS:        DefaultTLSTimeout,
		PhaseRead:       DefaultReadTimeout,
		PhaseWrite:      DefaultWriteTimeout,
		PhaseTotal:      DefaultTotalTimeout,
	}
}
