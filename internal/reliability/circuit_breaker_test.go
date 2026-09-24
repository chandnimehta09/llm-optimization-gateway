package reliability

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker("openai", DefaultCircuitBreakerConfig())
	if cb.State() != CircuitClosed {
		t.Errorf("expected initial state closed, got %s", cb.State())
	}
	if cb.Provider() != "openai" {
		t.Errorf("expected provider 'openai', got %s", cb.Provider())
	}
}

func TestCircuitBreaker_AllowWhenClosed(t *testing.T) {
	cb := NewCircuitBreaker("openai", DefaultCircuitBreakerConfig())
	if !cb.Allow() {
		t.Error("should allow when closed")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    3,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("openai", config)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("expected open after %d failures, got %s", 3, cb.State())
	}
	if cb.Allow() {
		t.Error("should not allow when open")
	}
}

func TestCircuitBreaker_AcceptanceCriteria_Threshold5(t *testing.T) {
	// Given circuit breaker with threshold = 5
	// And 5 consecutive failures
	// When next request arrives
	// Then circuit is open
	// And request is rejected immediately
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    5,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("test-provider", config)

	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Fatalf("circuit should be open after 5 failures, got %s", cb.State())
	}
	if cb.Allow() {
		t.Fatal("request should be rejected when circuit is open")
	}

	// Verify rejected counter increments.
	_, _, rejected := cb.Counters()
	if rejected < 1 {
		t.Error("rejected counter should be >= 1")
	}
}

func TestCircuitBreaker_SuccessResetsConsecutiveFailures(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    3,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("openai", config)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess() // Resets consecutive failures.
	cb.RecordFailure()
	cb.RecordFailure()

	// Should still be closed because success reset the counter.
	if cb.State() != CircuitClosed {
		t.Errorf("expected closed (success reset counter), got %s", cb.State())
	}
}

func TestCircuitBreaker_BelowThreshold(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    5,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("openai", config)

	for i := 0; i < 4; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed below threshold, got %s", cb.State())
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    2,
		ResetTimeout:        50 * time.Millisecond,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("openai", config)

	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}

	// Wait for reset timeout.
	time.Sleep(60 * time.Millisecond)

	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected half-open after reset timeout, got %s", cb.State())
	}
}

func TestCircuitBreaker_ClosesAfterSuccessfulProbe(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    2,
		ResetTimeout:        50 * time.Millisecond,
		HalfOpenMaxAttempts: 3,
		SuccessThreshold:    2,
	}
	cb := NewCircuitBreaker("openai", config)

	// Open the circuit.
	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)

	// Should be half-open now.
	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected half-open, got %s", cb.State())
	}

	// Success probes.
	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after successful probes, got %s", cb.State())
	}
}

func TestCircuitBreaker_FailureInHalfOpenReOpens(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    2,
		ResetTimeout:        50 * time.Millisecond,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("openai", config)

	// Open the circuit.
	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)

	// Half-open.
	_ = cb.State()

	// Failure reopens.
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("expected open after failure in half-open, got %s", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    2,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("openai", config)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Fatal("expected open")
	}

	cb.Reset()
	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after reset, got %s", cb.State())
	}
	if cb.ConsecutiveFailures() != 0 {
		t.Errorf("expected 0 consecutive failures after reset, got %d", cb.ConsecutiveFailures())
	}
}

func TestCircuitBreaker_ForceOpen(t *testing.T) {
	cb := NewCircuitBreaker("openai", DefaultCircuitBreakerConfig())
	cb.ForceOpen()
	if cb.State() != CircuitOpen {
		t.Errorf("expected open after ForceOpen, got %s", cb.State())
	}
}

func TestCircuitBreaker_ForceClose(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    2,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("openai", config)

	cb.RecordFailure()
	cb.RecordFailure()
	cb.ForceClose()

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after ForceClose, got %s", cb.State())
	}
}

func TestCircuitBreaker_Snapshot(t *testing.T) {
	cb := NewCircuitBreaker("openai", DefaultCircuitBreakerConfig())
	cb.RecordSuccess()
	cb.RecordFailure()

	snap := cb.Snapshot()
	if snap.Provider != "openai" {
		t.Errorf("expected provider 'openai', got %s", snap.Provider)
	}
	if snap.TotalSuccesses != 1 {
		t.Errorf("expected 1 success, got %d", snap.TotalSuccesses)
	}
	if snap.TotalFailures != 1 {
		t.Errorf("expected 1 failure, got %d", snap.TotalFailures)
	}
}

func TestCircuitBreaker_Transitions(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    2,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("openai", config)

	cb.RecordFailure()
	cb.RecordFailure()

	transitions := cb.Transitions()
	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].From != CircuitClosed || transitions[0].To != CircuitOpen {
		t.Errorf("expected closed->open, got %s->%s", transitions[0].From, transitions[0].To)
	}
	if transitions[0].Provider != "openai" {
		t.Errorf("expected provider 'openai' in transition, got %s", transitions[0].Provider)
	}
}

func TestCircuitBreaker_Counters(t *testing.T) {
	cb := NewCircuitBreaker("openai", DefaultCircuitBreakerConfig())
	cb.RecordSuccess()
	cb.RecordSuccess()
	cb.RecordFailure()

	successes, failures, rejected := cb.Counters()
	if successes != 2 {
		t.Errorf("expected 2 successes, got %d", successes)
	}
	if failures != 1 {
		t.Errorf("expected 1 failure, got %d", failures)
	}
	if rejected != 0 {
		t.Errorf("expected 0 rejected, got %d", rejected)
	}
}

func TestCircuitBreaker_RejectedCountIncreases(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    1,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	}
	cb := NewCircuitBreaker("openai", config)

	cb.RecordFailure() // Opens circuit.

	// Try to allow 5 requests.
	for i := 0; i < 5; i++ {
		cb.Allow()
	}

	_, _, rejected := cb.Counters()
	if rejected != 5 {
		t.Errorf("expected 5 rejected, got %d", rejected)
	}
}

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	var mu sync.Mutex
	var changes []struct{ from, to CircuitState }

	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    2,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
		OnStateChange: func(provider string, from, to CircuitState) {
			mu.Lock()
			defer mu.Unlock()
			changes = append(changes, struct{ from, to CircuitState }{from, to})
		},
	}
	cb := NewCircuitBreaker("openai", config)

	cb.RecordFailure()
	cb.RecordFailure()

	// Give the goroutine time to execute the callback.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(changes) != 1 {
		t.Fatalf("expected 1 state change callback, got %d", len(changes))
	}
	if changes[0].from != CircuitClosed || changes[0].to != CircuitOpen {
		t.Errorf("expected closed->open, got %s->%s", changes[0].from, changes[0].to)
	}
}

func TestCircuitBreaker_DefaultValues(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{})
	if cb.State() != CircuitClosed {
		t.Error("expected initial state closed with default values")
	}
	// Should use defaults.
	for i := 0; i < DefaultFailureThreshold; i++ {
		cb.RecordFailure()
	}
	if cb.State() != CircuitOpen {
		t.Error("expected open after default threshold failures")
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker("openai", DefaultCircuitBreakerConfig())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			cb.RecordSuccess()
		}()
		go func() {
			defer wg.Done()
			cb.RecordFailure()
		}()
		go func() {
			defer wg.Done()
			cb.Allow()
		}()
	}
	wg.Wait()

	// Just verify no panics and state is valid.
	state := cb.State()
	if state.String() == "unknown" {
		t.Error("unexpected unknown state")
	}
}

func TestCircuitBreaker_HalfOpenLimitsTraffic(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    1,
		ResetTimeout:        50 * time.Millisecond,
		HalfOpenMaxAttempts: 2,
		SuccessThreshold:    2,
	}
	cb := NewCircuitBreaker("openai", config)

	// Open circuit.
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)

	// Half-open: should allow HalfOpenMaxAttempts requests.
	allowed := 0
	for i := 0; i < 5; i++ {
		if cb.Allow() {
			allowed++
		}
	}

	if allowed != 2 {
		t.Errorf("expected 2 allowed in half-open, got %d", allowed)
	}
}

// --- CircuitBreakerRegistry Tests ---

func TestCircuitBreakerRegistry_Get(t *testing.T) {
	reg := NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig())
	cb := reg.Get("openai")
	if cb == nil {
		t.Fatal("expected non-nil circuit breaker")
	}
	if cb.Provider() != "openai" {
		t.Errorf("expected provider 'openai', got %s", cb.Provider())
	}

	// Same provider should return same instance.
	cb2 := reg.Get("openai")
	if cb != cb2 {
		t.Error("expected same instance for same provider")
	}
}

func TestCircuitBreakerRegistry_Allow(t *testing.T) {
	reg := NewCircuitBreakerRegistry(CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    2,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	})

	if !reg.Allow("openai") {
		t.Error("should allow initially")
	}

	reg.RecordFailure("openai")
	reg.RecordFailure("openai")

	if reg.Allow("openai") {
		t.Error("should reject after threshold failures")
	}
}

func TestCircuitBreakerRegistry_RecordSuccessFailure(t *testing.T) {
	reg := NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig())
	reg.RecordSuccess("openai")
	reg.RecordFailure("anthropic")

	cb1 := reg.Get("openai")
	s, _, _ := cb1.Counters()
	if s != 1 {
		t.Errorf("expected 1 success for openai, got %d", s)
	}

	cb2 := reg.Get("anthropic")
	_, f, _ := cb2.Counters()
	if f != 1 {
		t.Errorf("expected 1 failure for anthropic, got %d", f)
	}
}

func TestCircuitBreakerRegistry_Reset(t *testing.T) {
	reg := NewCircuitBreakerRegistry(CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    1,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	})

	reg.RecordFailure("openai")
	reg.Reset("openai")

	if !reg.Allow("openai") {
		t.Error("should allow after reset")
	}
}

func TestCircuitBreakerRegistry_ResetAll(t *testing.T) {
	reg := NewCircuitBreakerRegistry(CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    1,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	})

	reg.RecordFailure("openai")
	reg.RecordFailure("anthropic")
	reg.ResetAll()

	if !reg.Allow("openai") {
		t.Error("openai should allow after reset all")
	}
	if !reg.Allow("anthropic") {
		t.Error("anthropic should allow after reset all")
	}
}

func TestCircuitBreakerRegistry_ForceOpenClose(t *testing.T) {
	reg := NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig())

	reg.ForceOpen("openai")
	if reg.Allow("openai") {
		t.Error("should reject after force open")
	}

	reg.ForceClose("openai")
	if !reg.Allow("openai") {
		t.Error("should allow after force close")
	}
}

func TestCircuitBreakerRegistry_Snapshots(t *testing.T) {
	reg := NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig())
	reg.RecordSuccess("openai")
	reg.RecordFailure("anthropic")

	snapshots := reg.Snapshots()
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}
	if snapshots["openai"].TotalSuccesses != 1 {
		t.Error("expected 1 success for openai")
	}
	if snapshots["anthropic"].TotalFailures != 1 {
		t.Error("expected 1 failure for anthropic")
	}
}

func TestCircuitBreakerRegistry_Providers(t *testing.T) {
	reg := NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig())
	reg.Get("openai")
	reg.Get("anthropic")

	providers := reg.Providers()
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}

func TestCircuitBreakerRegistry_ResetNonexistent(t *testing.T) {
	reg := NewCircuitBreakerRegistry(DefaultCircuitBreakerConfig())
	// Should not panic.
	reg.Reset("nonexistent")
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("%d.String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}

func TestStateTransition_String(t *testing.T) {
	st := StateTransition{
		Provider:  "openai",
		From:      CircuitClosed,
		To:        CircuitOpen,
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		Reason:    "threshold exceeded",
	}
	s := st.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

// --- Integration Tests ---

func TestIntegration_FailingProvider_Recovery(t *testing.T) {
	config := CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    3,
		ResetTimeout:        50 * time.Millisecond,
		HalfOpenMaxAttempts: 2,
		SuccessThreshold:    2,
	}
	cb := NewCircuitBreaker("flaky-provider", config)

	// Phase 1: Provider fails, circuit opens.
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	if cb.State() != CircuitOpen {
		t.Fatal("expected open")
	}

	// Phase 2: Wait for reset timeout, circuit goes half-open.
	time.Sleep(60 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatal("expected half-open")
	}

	// Phase 3: Provider recovers, probes succeed, circuit closes.
	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Fatalf("expected closed after recovery, got %s", cb.State())
	}
}

func TestIntegration_RetryWithCircuitBreaker(t *testing.T) {
	reg := NewCircuitBreakerRegistry(CircuitBreakerConfig{
		Enabled:             true,
		FailureThreshold:    3,
		ResetTimeout:        30 * time.Second,
		HalfOpenMaxAttempts: 1,
		SuccessThreshold:    1,
	})

	retryConfig := RetryConfig{
		MaxAttempts:  5,
		InitialDelay: time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		JitterMode:   JitterNone,
		RetryBudget:  RetryBudgetConfig{Enabled: false},
	}
	policy := NewRetryPolicy(retryConfig, nil)

	provider := "openai"
	var attempts int

	err := policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		if !reg.Allow(provider) {
			return &CircuitOpenError{Provider: provider}
		}
		reg.RecordFailure(provider)
		return NewRetryableError(errors.New("provider error"))
	})

	if err == nil {
		t.Fatal("expected error")
	}

	// After 3 failures, circuit opens and subsequent requests get CircuitOpenError.
	snap := reg.Get(provider).Snapshot()
	if snap.State != CircuitOpen {
		t.Errorf("expected circuit open, got %s", snap.State)
	}
}

func TestIntegration_TimeoutWithRetry(t *testing.T) {
	timeoutConfig := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: 20 * time.Millisecond,
		},
	}
	mgr := NewTimeoutManager(timeoutConfig)

	retryConfig := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		JitterMode:   JitterNone,
		RetryBudget:  RetryBudgetConfig{Enabled: false},
	}
	policy := NewRetryPolicy(retryConfig, nil)

	var attempts int
	_ = policy.Execute(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return mgr.Execute(ctx, PhaseTotal, "slow-provider", "", nil, func(ctx context.Context) error {
			select {
			case <-time.After(100 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	})

	// TimeoutError wraps context.DeadlineExceeded, which is non-retryable by default,
	// so it should stop after 1 attempt.
	if attempts != 1 {
		t.Errorf("expected 1 attempt (timeout is non-retryable), got %d", attempts)
	}
}
