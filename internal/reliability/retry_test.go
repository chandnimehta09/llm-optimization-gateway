package reliability

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryPolicy_SuccessOnFirstAttempt(t *testing.T) {
	config := DefaultRetryConfig()
	config.JitterMode = JitterNone
	config.InitialDelay = time.Millisecond
	policy := NewRetryPolicy(config, nil)

	var attempts int
	err := policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", attempts)
	}

	m := policy.Metrics()
	if m.TotalAttempts != 1 {
		t.Errorf("expected 1 total attempt, got %d", m.TotalAttempts)
	}
	if m.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", m.TotalSuccesses)
	}
	if m.TotalRetries != 0 {
		t.Errorf("expected 0 retries, got %d", m.TotalRetries)
	}
}

func TestRetryPolicy_SuccessAfterRetries(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 3
	config.InitialDelay = time.Millisecond
	config.JitterMode = JitterNone
	policy := NewRetryPolicy(config, nil)

	var attempts int
	err := policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		if attempt < 2 {
			return NewRetryableError(errors.New("transient"))
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	m := policy.Metrics()
	if m.TotalRetries != 2 {
		t.Errorf("expected 2 retries, got %d", m.TotalRetries)
	}
}

func TestRetryPolicy_AllAttemptsExhausted(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 3
	config.InitialDelay = time.Millisecond
	config.JitterMode = JitterNone
	policy := NewRetryPolicy(config, nil)

	var attempts int
	err := policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return NewRetryableError(errors.New("always fails"))
	})

	if err == nil {
		t.Fatal("expected error after all attempts exhausted")
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	m := policy.Metrics()
	if m.TotalExhausted != 1 {
		t.Errorf("expected 1 exhausted, got %d", m.TotalExhausted)
	}
}

func TestRetryPolicy_NonRetryableStopsImmediately(t *testing.T) {
	config := DefaultRetryConfig()
	config.MaxAttempts = 5
	config.InitialDelay = time.Millisecond
	policy := NewRetryPolicy(config, nil)

	var attempts int
	err := policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return NewNonRetryableError(errors.New("permanent"))
	})

	if err == nil {
		t.Fatal("expected error for non-retryable")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (no retries for non-retryable), got %d", attempts)
	}
}

func TestRetryPolicy_BackoffIncreases(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     time.Second,
		Multiplier:   2.0,
		JitterMode:   JitterNone,
		RetryBudget:  RetryBudgetConfig{Enabled: false},
	}
	policy := NewRetryPolicy(config, nil)

	var timestamps []time.Time
	err := policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		timestamps = append(timestamps, time.Now())
		return NewRetryableError(errors.New("fail"))
	})

	if err == nil {
		t.Fatal("expected error")
	}

	// Verify delays increase between attempts.
	for i := 2; i < len(timestamps); i++ {
		prevDelay := timestamps[i-1].Sub(timestamps[i-2])
		currDelay := timestamps[i].Sub(timestamps[i-1])
		// Allow some tolerance for timing.
		if currDelay < prevDelay/2 {
			t.Errorf("delay %d (%v) should be >= delay %d (%v)", i, currDelay, i-1, prevDelay)
		}
	}
}

func TestRetryPolicy_ContextCanceled(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialDelay = time.Second
	policy := NewRetryPolicy(config, nil)

	ctx, cancel := context.WithCancel(context.Background())

	var attempts int32
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := policy.Execute(ctx, func(ctx context.Context, attempt int) error {
		atomic.AddInt32(&attempts, 1)
		return NewRetryableError(errors.New("fail"))
	})

	if err == nil {
		t.Fatal("expected error after context cancellation")
	}
}

func TestRetryPolicy_ContextDeadlineExceeded(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialDelay = time.Second
	policy := NewRetryPolicy(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := policy.Execute(ctx, func(ctx context.Context, attempt int) error {
		return NewRetryableError(errors.New("fail"))
	})

	if err == nil {
		t.Fatal("expected error after deadline exceeded")
	}
}

func TestRetryPolicy_AcceptanceCriteria_MaxAttempts3(t *testing.T) {
	// Given error is retryable
	// And max_attempts = 3
	// When retry policy executes
	// Then request is retried up to 3 times
	// And backoff increases between attempts
	config := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 5 * time.Millisecond,
		MaxDelay:     time.Second,
		Multiplier:   2.0,
		JitterMode:   JitterNone,
		RetryBudget:  RetryBudgetConfig{Enabled: false},
	}
	policy := NewRetryPolicy(config, nil)

	var attempts int
	var delays []time.Duration
	var lastTime time.Time

	err := policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		now := time.Now()
		if attempts > 0 {
			delays = append(delays, now.Sub(lastTime))
		}
		lastTime = now
		attempts++
		return NewRetryableError(errors.New("retryable error"))
	})

	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 3 {
		t.Fatalf("expected exactly 3 attempts, got %d", attempts)
	}

	// Verify backoff increases between attempts.
	if len(delays) >= 2 && delays[1] < delays[0] {
		t.Errorf("expected backoff to increase: delay[0]=%v, delay[1]=%v", delays[0], delays[1])
	}
}

func TestRetryPolicy_Metrics(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialDelay = time.Millisecond
	config.JitterMode = JitterNone
	policy := NewRetryPolicy(config, nil)

	_ = policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		return nil
	})

	m := policy.Metrics()
	if m.SuccessRate() != 1.0 {
		t.Errorf("expected 100%% success rate, got %f", m.SuccessRate())
	}
	if m.AvgLatency <= 0 {
		t.Error("expected positive avg latency")
	}
}

func TestRetryPolicy_MetricsZeroAttempts(t *testing.T) {
	m := RetryMetrics{}
	if m.SuccessRate() != 0 {
		t.Errorf("zero attempts should have 0 success rate, got %f", m.SuccessRate())
	}
}

func TestRetryPolicy_Reset(t *testing.T) {
	config := DefaultRetryConfig()
	config.InitialDelay = time.Millisecond
	config.JitterMode = JitterNone
	policy := NewRetryPolicy(config, nil)

	_ = policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		return nil
	})

	policy.Reset()
	m := policy.Metrics()
	if m.TotalAttempts != 0 {
		t.Errorf("after reset, expected 0 attempts, got %d", m.TotalAttempts)
	}
}

func TestRetryBudget_Allow(t *testing.T) {
	budget := NewRetryBudget(RetryBudgetConfig{
		Enabled:    true,
		MaxRetries: 3,
		Window:     time.Second,
	})

	if !budget.Allow() {
		t.Error("first allow should succeed")
	}
	if !budget.Allow() {
		t.Error("second allow should succeed")
	}
	if !budget.Allow() {
		t.Error("third allow should succeed")
	}
	if budget.Allow() {
		t.Error("fourth allow should be rejected (budget exhausted)")
	}
}

func TestRetryBudget_WindowExpiry(t *testing.T) {
	budget := NewRetryBudget(RetryBudgetConfig{
		Enabled:    true,
		MaxRetries: 2,
		Window:     50 * time.Millisecond,
	})

	budget.Allow()
	budget.Allow()
	if budget.Allow() {
		t.Error("should be at limit")
	}

	time.Sleep(60 * time.Millisecond)
	if !budget.Allow() {
		t.Error("should allow after window expiry")
	}
}

func TestRetryBudget_Disabled(t *testing.T) {
	budget := NewRetryBudget(RetryBudgetConfig{Enabled: false})
	for i := 0; i < 1000; i++ {
		if !budget.Allow() {
			t.Fatalf("disabled budget should always allow, failed at iteration %d", i)
		}
	}
}

func TestRetryBudget_Usage(t *testing.T) {
	budget := NewRetryBudget(RetryBudgetConfig{
		Enabled:    true,
		MaxRetries: 10,
		Window:     time.Second,
	})

	budget.Allow()
	budget.Allow()
	budget.Allow()

	current, max := budget.Usage()
	if current != 3 {
		t.Errorf("expected current=3, got %d", current)
	}
	if max != 10 {
		t.Errorf("expected max=10, got %d", max)
	}
}

func TestRetryBudget_UsageDisabled(t *testing.T) {
	budget := NewRetryBudget(RetryBudgetConfig{Enabled: false})
	current, max := budget.Usage()
	if current != 0 || max != 0 {
		t.Errorf("disabled budget should return 0,0; got %d,%d", current, max)
	}
}

func TestRetryBudget_Reset(t *testing.T) {
	budget := NewRetryBudget(RetryBudgetConfig{
		Enabled:    true,
		MaxRetries: 2,
		Window:     time.Second,
	})

	budget.Allow()
	budget.Allow()
	budget.Reset()

	current, _ := budget.Usage()
	if current != 0 {
		t.Errorf("expected 0 after reset, got %d", current)
	}
}

func TestRetryPolicy_BudgetExhausted(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:  10,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Second,
		Multiplier:   2.0,
		JitterMode:   JitterNone,
		RetryBudget: RetryBudgetConfig{
			Enabled:    true,
			MaxRetries: 1,
			Window:     time.Second,
		},
	}
	policy := NewRetryPolicy(config, nil)

	var attempts int
	err := policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return NewRetryableError(errors.New("fail"))
	})

	if err == nil {
		t.Fatal("expected error when budget exhausted")
	}
	// Should stop after budget is exhausted (first attempt + 1 retry allowed by budget).
	if attempts > 3 {
		t.Errorf("expected at most 3 attempts with budget=1, got %d", attempts)
	}

	m := policy.Metrics()
	if m.TotalBudgetHit != 1 {
		t.Errorf("expected 1 budget hit, got %d", m.TotalBudgetHit)
	}
}
