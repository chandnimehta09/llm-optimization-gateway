package reliability

import (
	"errors"
	"fmt"
	"time"
)

// Config holds the complete reliability configuration.
type Config struct {
	Retry          RetryConfig          `json:"retry"`
	Timeout        TimeoutConfig        `json:"timeout"`
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Retry:          DefaultRetryConfig(),
		Timeout:        DefaultTimeoutConfig(),
		CircuitBreaker: DefaultCircuitBreakerConfig(),
	}
}

// Validate checks the configuration for errors.
func (c Config) Validate() error {
	var errs []error
	if err := c.Retry.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("retry: %w", err))
	}
	if err := c.Timeout.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("timeout: %w", err))
	}
	if err := c.CircuitBreaker.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("circuit_breaker: %w", err))
	}
	return errors.Join(errs...)
}

// RetryConfig configures the retry policy.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the initial one).
	MaxAttempts int `json:"max_attempts"`
	// InitialDelay is the base delay before the first retry.
	InitialDelay time.Duration `json:"initial_delay"`
	// MaxDelay caps the backoff delay.
	MaxDelay time.Duration `json:"max_delay"`
	// Multiplier is the backoff multiplier applied after each retry.
	Multiplier float64 `json:"multiplier"`
	// JitterMode controls the type of jitter applied to delays.
	JitterMode JitterMode `json:"jitter_mode"`
	// RetryBudget limits total retries across all requests in a time window.
	RetryBudget RetryBudgetConfig `json:"retry_budget"`
}

// DefaultRetryConfig returns a RetryConfig with sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		JitterMode:   JitterFull,
		RetryBudget:  DefaultRetryBudgetConfig(),
	}
}

// Validate checks the retry configuration for errors.
func (c RetryConfig) Validate() error {
	var errs []error
	if c.MaxAttempts < 1 {
		errs = append(errs, errors.New("max_attempts must be >= 1"))
	}
	if c.InitialDelay < 0 {
		errs = append(errs, errors.New("initial_delay must be >= 0"))
	}
	if c.MaxDelay < 0 {
		errs = append(errs, errors.New("max_delay must be >= 0"))
	}
	if c.MaxDelay > 0 && c.InitialDelay > c.MaxDelay {
		errs = append(errs, errors.New("initial_delay must be <= max_delay"))
	}
	if c.Multiplier < 1.0 {
		errs = append(errs, errors.New("multiplier must be >= 1.0"))
	}
	if c.JitterMode != JitterNone && c.JitterMode != JitterFull && c.JitterMode != JitterDecorrelated {
		errs = append(errs, fmt.Errorf("unknown jitter mode: %s", c.JitterMode))
	}
	if err := c.RetryBudget.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("retry_budget: %w", err))
	}
	return errors.Join(errs...)
}

// RetryBudgetConfig limits the total number of retries in a rolling time window.
type RetryBudgetConfig struct {
	// Enabled controls whether the retry budget is enforced.
	Enabled bool `json:"enabled"`
	// MaxRetries is the maximum retries allowed in the window.
	MaxRetries int `json:"max_retries"`
	// Window is the rolling window duration.
	Window time.Duration `json:"window"`
}

// DefaultRetryBudgetConfig returns a budget allowing 100 retries per 10s.
func DefaultRetryBudgetConfig() RetryBudgetConfig {
	return RetryBudgetConfig{
		Enabled:    true,
		MaxRetries: 100,
		Window:     10 * time.Second,
	}
}

// Validate checks the retry budget configuration.
func (c RetryBudgetConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	var errs []error
	if c.MaxRetries < 1 {
		errs = append(errs, errors.New("max_retries must be >= 1"))
	}
	if c.Window <= 0 {
		errs = append(errs, errors.New("window must be > 0"))
	}
	return errors.Join(errs...)
}

// CircuitBreakerConfig configures the circuit breaker.
type CircuitBreakerConfig struct {
	// Enabled controls whether the circuit breaker is active.
	Enabled bool `json:"enabled"`
	// FailureThreshold is the number of consecutive failures to open the circuit.
	FailureThreshold int `json:"failure_threshold"`
	// ResetTimeout is how long to wait before transitioning from open to half-open.
	ResetTimeout time.Duration `json:"reset_timeout"`
	// HalfOpenMaxAttempts is the number of successful probes needed to close the circuit.
	HalfOpenMaxAttempts int `json:"half_open_max_attempts"`
	// SuccessThreshold is the number of consecutive successes in half-open to close.
	SuccessThreshold int `json:"success_threshold"`
	// OnStateChange is called when the circuit breaker transitions states.
	OnStateChange func(provider string, from, to CircuitState) `json:"-"`
}

// DefaultCircuitBreakerConfig returns a CircuitBreakerConfig with sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    5,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 3,
		SuccessThreshold:    2,
	}
}

// Validate checks the circuit breaker configuration.
func (c CircuitBreakerConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	var errs []error
	if c.FailureThreshold < 1 {
		errs = append(errs, errors.New("failure_threshold must be >= 1"))
	}
	if c.ResetTimeout <= 0 {
		errs = append(errs, errors.New("reset_timeout must be > 0"))
	}
	if c.HalfOpenMaxAttempts < 1 {
		errs = append(errs, errors.New("half_open_max_attempts must be >= 1"))
	}
	if c.SuccessThreshold < 1 {
		errs = append(errs, errors.New("success_threshold must be >= 1"))
	}
	if c.SuccessThreshold > c.HalfOpenMaxAttempts {
		errs = append(errs, errors.New("success_threshold must be <= half_open_max_attempts"))
	}
	return errors.Join(errs...)
}
