package reliability

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// BackoffCalculator computes the delay before the next retry attempt.
type BackoffCalculator struct {
	mu           sync.Mutex
	initialDelay time.Duration
	maxDelay     time.Duration
	multiplier   float64
	jitterMode   JitterMode
	rng          *rand.Rand
	prevDelay    time.Duration
}

// NewBackoffCalculator creates a new backoff calculator.
func NewBackoffCalculator(initialDelay, maxDelay time.Duration, multiplier float64, jitter JitterMode) *BackoffCalculator {
	if initialDelay <= 0 {
		initialDelay = DefaultInitialDelay
	}
	if maxDelay <= 0 {
		maxDelay = DefaultMaxDelay
	}
	if multiplier < 1.0 {
		multiplier = DefaultMultiplier
	}
	return &BackoffCalculator{
		initialDelay: initialDelay,
		maxDelay:     maxDelay,
		multiplier:   multiplier,
		jitterMode:   jitter,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
		prevDelay:    initialDelay,
	}
}

// NextDelay returns the delay for the given attempt number (0-indexed).
// Attempt 0 is the delay before the first retry.
func (b *BackoffCalculator) NextDelay(attempt int) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	if attempt < 0 {
		attempt = 0
	}

	var delay time.Duration

	switch b.jitterMode {
	case JitterDecorrelated:
		delay = b.decorrelatedDelay(attempt)
	default:
		delay = b.exponentialDelay(attempt)
	}

	// Cap at max delay.
	if delay > b.maxDelay {
		delay = b.maxDelay
	}

	return delay
}

// exponentialDelay computes base * multiplier^attempt, then applies jitter.
func (b *BackoffCalculator) exponentialDelay(attempt int) time.Duration {
	base := float64(b.initialDelay) * math.Pow(b.multiplier, float64(attempt))
	if base > float64(b.maxDelay) {
		base = float64(b.maxDelay)
	}

	delay := time.Duration(base)
	if b.jitterMode == JitterFull {
		// Full jitter: uniform random in [0, delay).
		if delay > 0 {
			delay = time.Duration(b.rng.Int63n(int64(delay)))
		}
	}
	return delay
}

// decorrelatedDelay computes delay = random(initialDelay, prevDelay * 3).
func (b *BackoffCalculator) decorrelatedDelay(attempt int) time.Duration {
	if attempt == 0 {
		b.prevDelay = b.initialDelay
		return b.initialDelay
	}

	upper := int64(float64(b.prevDelay) * 3)
	lower := int64(b.initialDelay)
	if upper <= lower {
		upper = lower + 1
	}

	delay := time.Duration(lower + b.rng.Int63n(upper-lower))
	if delay > b.maxDelay {
		delay = b.maxDelay
	}
	b.prevDelay = delay
	return delay
}

// Reset resets the backoff state for a new sequence.
func (b *BackoffCalculator) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.prevDelay = b.initialDelay
}

// ComputeDelay is a standalone function that computes a single backoff delay.
func ComputeDelay(attempt int, initialDelay, maxDelay time.Duration, multiplier float64, jitter JitterMode) time.Duration {
	calc := NewBackoffCalculator(initialDelay, maxDelay, multiplier, jitter)
	return calc.NextDelay(attempt)
}
