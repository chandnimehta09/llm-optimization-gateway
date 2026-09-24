package reliability

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// RetryPolicy executes an operation with configurable retries, backoff, and error classification.
type RetryPolicy struct {
	config     RetryConfig
	classifier *ErrorClassifier
	backoff    *BackoffCalculator
	budget     *RetryBudget
	metrics    retryMetricsCollector
}

type retryMetricsCollector struct {
	mu             sync.Mutex
	totalAttempts  int64
	totalRetries   int64
	totalSuccesses int64
	totalFailures  int64
	totalExhausted int64
	totalBudgetHit int64
	totalLatency   int64 // nanoseconds
}

// NewRetryPolicy creates a new retry policy with the given configuration.
func NewRetryPolicy(config RetryConfig, classifier *ErrorClassifier) *RetryPolicy {
	if classifier == nil {
		classifier = NewErrorClassifier()
	}
	return &RetryPolicy{
		config:     config,
		classifier: classifier,
		backoff:    NewBackoffCalculator(config.InitialDelay, config.MaxDelay, config.Multiplier, config.JitterMode),
		budget:     NewRetryBudget(config.RetryBudget),
	}
}

// Execute runs the operation with retry logic. The operation receives the attempt number
// (0-indexed) and returns an error if it fails.
func (p *RetryPolicy) Execute(ctx context.Context, op func(ctx context.Context, attempt int) error) error {
	start := time.Now()
	var lastErr error

	for attempt := 0; attempt < p.config.MaxAttempts; attempt++ {
		atomic.AddInt64(&p.metrics.totalAttempts, 1)
		if attempt > 0 {
			atomic.AddInt64(&p.metrics.totalRetries, 1)
		}

		// Check context before each attempt.
		if err := ctx.Err(); err != nil {
			p.recordLatency(start)
			return fmt.Errorf("context error before attempt %d: %w", attempt, err)
		}

		// Wait for backoff delay (skip for first attempt).
		if attempt > 0 {
			delay := p.backoff.NextDelay(attempt - 1)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				p.recordLatency(start)
				return fmt.Errorf("context canceled during backoff: %w", ctx.Err())
			}
		}

		err := op(ctx, attempt)
		if err == nil {
			atomic.AddInt64(&p.metrics.totalSuccesses, 1)
			p.recordLatency(start)
			return nil
		}

		lastErr = err

		// Check if we should retry.
		if attempt+1 >= p.config.MaxAttempts {
			break
		}

		if !p.classifier.IsRetryable(err) {
			atomic.AddInt64(&p.metrics.totalFailures, 1)
			p.recordLatency(start)
			return err
		}

		// Check retry budget.
		if !p.budget.Allow() {
			atomic.AddInt64(&p.metrics.totalBudgetHit, 1)
			atomic.AddInt64(&p.metrics.totalFailures, 1)
			p.recordLatency(start)
			return fmt.Errorf("retry budget exhausted: %w", lastErr)
		}
	}

	atomic.AddInt64(&p.metrics.totalExhausted, 1)
	atomic.AddInt64(&p.metrics.totalFailures, 1)
	p.recordLatency(start)
	return fmt.Errorf("all %d attempts failed: %w", p.config.MaxAttempts, lastErr)
}

func (p *RetryPolicy) recordLatency(start time.Time) {
	atomic.AddInt64(&p.metrics.totalLatency, int64(time.Since(start)))
}

// Metrics returns the current retry metrics.
func (p *RetryPolicy) Metrics() RetryMetrics {
	attempts := atomic.LoadInt64(&p.metrics.totalAttempts)
	retries := atomic.LoadInt64(&p.metrics.totalRetries)
	successes := atomic.LoadInt64(&p.metrics.totalSuccesses)
	failures := atomic.LoadInt64(&p.metrics.totalFailures)
	exhausted := atomic.LoadInt64(&p.metrics.totalExhausted)
	budgetHit := atomic.LoadInt64(&p.metrics.totalBudgetHit)
	latency := atomic.LoadInt64(&p.metrics.totalLatency)

	var avgLatency time.Duration
	if attempts > 0 {
		avgLatency = time.Duration(latency / attempts)
	}

	return RetryMetrics{
		TotalAttempts:  attempts,
		TotalRetries:   retries,
		TotalSuccesses: successes,
		TotalFailures:  failures,
		TotalExhausted: exhausted,
		TotalBudgetHit: budgetHit,
		AvgLatency:     avgLatency,
	}
}

// Reset resets the retry policy state including backoff and metrics.
func (p *RetryPolicy) Reset() {
	p.backoff.Reset()
	atomic.StoreInt64(&p.metrics.totalAttempts, 0)
	atomic.StoreInt64(&p.metrics.totalRetries, 0)
	atomic.StoreInt64(&p.metrics.totalSuccesses, 0)
	atomic.StoreInt64(&p.metrics.totalFailures, 0)
	atomic.StoreInt64(&p.metrics.totalExhausted, 0)
	atomic.StoreInt64(&p.metrics.totalBudgetHit, 0)
	atomic.StoreInt64(&p.metrics.totalLatency, 0)
}

// RetryBudget enforces a global limit on the number of retries in a rolling time window.
type RetryBudget struct {
	mu      sync.Mutex
	config  RetryBudgetConfig
	entries []time.Time
}

// NewRetryBudget creates a new retry budget.
func NewRetryBudget(config RetryBudgetConfig) *RetryBudget {
	return &RetryBudget{
		config:  config,
		entries: make([]time.Time, 0),
	}
}

// Allow checks whether a retry is allowed under the budget. If allowed, it records
// the retry and returns true. If the budget is disabled, it always returns true.
func (b *RetryBudget) Allow() bool {
	if !b.config.Enabled {
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-b.config.Window)

	// Trim expired entries.
	valid := 0
	for _, t := range b.entries {
		if t.After(cutoff) {
			b.entries[valid] = t
			valid++
		}
	}
	b.entries = b.entries[:valid]

	if len(b.entries) >= b.config.MaxRetries {
		return false
	}

	b.entries = append(b.entries, now)
	return true
}

// Usage returns the current number of retries in the active window and the max.
func (b *RetryBudget) Usage() (current, max int) {
	if !b.config.Enabled {
		return 0, 0
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-b.config.Window)

	count := 0
	for _, t := range b.entries {
		if t.After(cutoff) {
			count++
		}
	}
	return count, b.config.MaxRetries
}

// Reset clears the budget entries.
func (b *RetryBudget) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = b.entries[:0]
}
