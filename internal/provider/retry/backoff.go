package retry

import (
	"math"
	"math/rand"
	"time"
)

// JitterStrategy determines how randomness is applied to backoff delays.
type JitterStrategy int

const (
	// FullJitter applies uniform random jitter in [0, calculated_delay].
	FullJitter JitterStrategy = iota
	// DecorrelatedJitter uses the "decorrelated jitter" algorithm where each
	// delay is chosen from [baseDelay, previousDelay * 3].
	DecorrelatedJitter
)

// BackoffConfig holds parameters for exponential backoff calculation.
type BackoffConfig struct {
	// InitialDelay is the base delay for the first retry.
	InitialDelay time.Duration
	// MaxDelay is the upper bound on any single delay.
	MaxDelay time.Duration
	// Multiplier is the factor by which the delay grows on each attempt.
	Multiplier float64
	// Jitter is the jitter strategy to apply.
	Jitter JitterStrategy
}

// DefaultBackoffConfig returns a reasonable default backoff configuration.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       FullJitter,
	}
}

// Backoff calculates retry delays using exponential backoff with jitter.
type Backoff struct {
	config    BackoffConfig
	rng       *rand.Rand
	lastDelay time.Duration
}

// NewBackoff creates a new Backoff calculator with the given configuration.
func NewBackoff(config BackoffConfig) *Backoff {
	return &Backoff{
		config:    config,
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		lastDelay: config.InitialDelay,
	}
}

// NewBackoffWithRng creates a Backoff calculator with a specific RNG (for testing).
func NewBackoffWithRng(config BackoffConfig, rng *rand.Rand) *Backoff {
	return &Backoff{
		config:    config,
		rng:       rng,
		lastDelay: config.InitialDelay,
	}
}

// Next returns the delay to wait before the given attempt number (0-indexed retry count).
// attempt 0 = first retry, attempt 1 = second retry, etc.
func (b *Backoff) Next(attempt int) time.Duration {
	switch b.config.Jitter {
	case DecorrelatedJitter:
		return b.decorrelatedNext()
	default:
		return b.fullJitterNext(attempt)
	}
}

// fullJitterNext calculates delay with full jitter:
// delay = random_between(0, min(maxDelay, initialDelay * multiplier^attempt))
func (b *Backoff) fullJitterNext(attempt int) time.Duration {
	// Calculate exponential delay.
	delay := float64(b.config.InitialDelay) * math.Pow(b.config.Multiplier, float64(attempt))

	// Cap at max delay.
	if delay > float64(b.config.MaxDelay) {
		delay = float64(b.config.MaxDelay)
	}

	// Apply full jitter: uniform random in [0, delay].
	jittered := time.Duration(b.rng.Float64() * delay)

	// Ensure at least 1ms to avoid zero delay.
	if jittered < time.Millisecond {
		jittered = time.Millisecond
	}

	b.lastDelay = time.Duration(delay)
	return jittered
}

// decorrelatedNext calculates delay using decorrelated jitter:
// delay = min(maxDelay, random_between(initialDelay, lastDelay * 3))
func (b *Backoff) decorrelatedNext() time.Duration {
	low := float64(b.config.InitialDelay)
	high := float64(b.lastDelay) * 3.0

	if high < low {
		high = low
	}

	// Random between low and high.
	delay := low + b.rng.Float64()*(high-low)

	// Cap at max delay.
	if delay > float64(b.config.MaxDelay) {
		delay = float64(b.config.MaxDelay)
	}

	b.lastDelay = time.Duration(delay)
	return time.Duration(delay)
}

// Reset resets the backoff state for a new retry sequence.
func (b *Backoff) Reset() {
	b.lastDelay = b.config.InitialDelay
}
