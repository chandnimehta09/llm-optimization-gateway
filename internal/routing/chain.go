package routing

import (
	"fmt"
	"sync"
)

// FallbackCondition defines when a fallback should be triggered.
type FallbackCondition string

const (
	// FallbackOnError triggers fallback on any provider error.
	FallbackOnError FallbackCondition = "on_error"
	// FallbackOnTimeout triggers fallback when the provider times out.
	FallbackOnTimeout FallbackCondition = "on_timeout"
	// FallbackOnRateLimit triggers fallback when the provider returns a rate limit error.
	FallbackOnRateLimit FallbackCondition = "on_rate_limit"
	// FallbackOnServerError triggers fallback on 5xx errors.
	FallbackOnServerError FallbackCondition = "on_server_error"
	// FallbackOnUnavailable triggers fallback when the provider is unavailable.
	FallbackOnUnavailable FallbackCondition = "on_unavailable"
)

// FallbackChainConfig configures a fallback chain.
type FallbackChainConfig struct {
	// MaxAttempts is the maximum number of provider attempts (including
	// the primary). Zero means unlimited (up to chain length).
	MaxAttempts int `json:"max_attempts"`
	// Conditions are the conditions under which fallback is triggered.
	// If empty, defaults to [FallbackOnError].
	Conditions []FallbackCondition `json:"conditions"`
}

// DefaultFallbackChainConfig returns a FallbackChainConfig with sensible defaults.
func DefaultFallbackChainConfig() FallbackChainConfig {
	return FallbackChainConfig{
		MaxAttempts: 3,
		Conditions: []FallbackCondition{
			FallbackOnError,
			FallbackOnTimeout,
			FallbackOnRateLimit,
			FallbackOnServerError,
			FallbackOnUnavailable,
		},
	}
}

// FallbackChain defines a provider priority order with fallback conditions.
// It tracks the current position in the chain for a given request.
type FallbackChain struct {
	mu sync.RWMutex

	// providers is the ordered list of providers to try.
	providers *PriorityList
	// config is the fallback configuration.
	config FallbackChainConfig
	// conditionSet is a lookup set for conditions.
	conditionSet map[FallbackCondition]bool
}

// NewFallbackChain creates a new FallbackChain with the given providers and config.
func NewFallbackChain(providers *PriorityList, config FallbackChainConfig) *FallbackChain {
	if len(config.Conditions) == 0 {
		config.Conditions = []FallbackCondition{FallbackOnError}
	}

	condSet := make(map[FallbackCondition]bool, len(config.Conditions))
	for _, c := range config.Conditions {
		condSet[c] = true
	}

	return &FallbackChain{
		providers:    providers,
		config:       config,
		conditionSet: condSet,
	}
}

// Providers returns the ordered list of providers.
func (fc *FallbackChain) Providers() []string {
	return fc.providers.Ordered()
}

// Config returns the fallback chain configuration.
func (fc *FallbackChain) Config() FallbackChainConfig {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.config
}

// MaxAttempts returns the maximum number of attempts allowed.
func (fc *FallbackChain) MaxAttempts() int {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	max := fc.config.MaxAttempts
	chainLen := fc.providers.Size()
	if max <= 0 || max > chainLen {
		return chainLen
	}
	return max
}

// ShouldFallback determines whether a fallback should be triggered for the
// given error and status code.
func (fc *FallbackChain) ShouldFallback(err error, statusCode int) bool {
	if err == nil && (statusCode == 0 || (statusCode >= 200 && statusCode < 300)) {
		return false
	}

	fc.mu.RLock()
	defer fc.mu.RUnlock()

	// FallbackOnError matches any error.
	if fc.conditionSet[FallbackOnError] && err != nil {
		return true
	}

	// Check specific conditions based on status code.
	if statusCode == 429 && fc.conditionSet[FallbackOnRateLimit] {
		return true
	}
	if statusCode >= 500 && fc.conditionSet[FallbackOnServerError] {
		return true
	}
	if statusCode == 503 && fc.conditionSet[FallbackOnUnavailable] {
		return true
	}

	// Check for context deadline (timeout).
	if err != nil && fc.conditionSet[FallbackOnTimeout] {
		if isTimeoutError(err) {
			return true
		}
	}

	return false
}

// isTimeoutError checks if the error is a timeout-related error.
func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	// Check for context.DeadlineExceeded.
	if err.Error() == "context deadline exceeded" {
		return true
	}
	// Check for net timeout interface.
	type timeoutErr interface {
		Timeout() bool
	}
	if te, ok := err.(timeoutErr); ok {
		return te.Timeout()
	}
	return false
}

// NextProvider returns the next provider to try after the given provider.
func (fc *FallbackChain) NextProvider(current string) (string, bool) {
	return fc.providers.NextAfter(current)
}

// String returns a human-readable representation of the fallback chain.
func (fc *FallbackChain) String() string {
	providers := fc.providers.Ordered()
	return fmt.Sprintf("fallback_chain: providers=%v max_attempts=%d conditions=%v",
		providers, fc.MaxAttempts(), fc.config.Conditions)
}
