package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCB_Initial(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	if cb.State() != CircuitClosed {
		t.Error("want closed")
	}
	if cb.ConsecutiveFailures() != 0 {
		t.Error("want 0")
	}
}
func TestCB_AllowClosed(t *testing.T) {
	if !NewCircuitBreaker(DefaultCircuitBreakerConfig()).Allow() {
		t.Error("want true")
	}
}
func TestCB_Opens(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, ResetTimeout: time.Minute})
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	if cb.State() != CircuitOpen {
		t.Error("want open")
	}
	if cb.Allow() {
		t.Error("want false")
	}
}
func TestCB_BelowThreshold(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 5, ResetTimeout: time.Minute})
	for i := 0; i < 4; i++ {
		cb.RecordFailure()
	}
	if cb.State() != CircuitClosed {
		t.Error("want closed")
	}
}
func TestCB_SuccessResets(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 5, ResetTimeout: time.Minute})
	for i := 0; i < 4; i++ {
		cb.RecordFailure()
	}
	cb.RecordSuccess()
	if cb.ConsecutiveFailures() != 0 {
		t.Error("want 0")
	}
	for i := 0; i < 4; i++ {
		cb.RecordFailure()
	}
	if cb.State() != CircuitClosed {
		t.Error("want closed")
	}
}
func TestCB_HalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: 50 * time.Millisecond})
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Error("want half-open")
	}
	if !cb.Allow() {
		t.Error("want true")
	}
}
func TestCB_Closes(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: 50 * time.Millisecond, HalfOpenMaxAttempts: 1})
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	_ = cb.State()
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Error("want closed")
	}
}
func TestCB_FailHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: 50 * time.Millisecond})
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	_ = cb.State()
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Error("want open")
	}
}
func TestCB_Reset(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: time.Minute})
	cb.RecordFailure()
	cb.RecordFailure()
	cb.Reset()
	if cb.State() != CircuitClosed {
		t.Error("want closed")
	}
}
func TestCB_Transitions(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: 50 * time.Millisecond})
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	_ = cb.State()
	cb.RecordSuccess()
	tr := cb.Transitions()
	if len(tr) != 3 {
		t.Fatalf("got %d", len(tr))
	}
	if tr[0].From != CircuitClosed || tr[2].To != CircuitClosed {
		t.Error("bad transitions")
	}
}
func TestCB_Counters(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 10, ResetTimeout: time.Minute})
	for i := 0; i < 5; i++ {
		cb.RecordSuccess()
	}
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	s, f, b := cb.Counters()
	if s != 5 || f != 3 || b != 0 {
		t.Errorf("(%d,%d,%d)", s, f, b)
	}
}
func TestCB_Bypass(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: time.Minute})
	cb.RecordFailure()
	cb.RecordFailure()
	cb.Allow()
	cb.Allow()
	cb.Allow()
	_, _, b := cb.Counters()
	if b != 3 {
		t.Errorf("bypasses=%d", b)
	}
}
func TestCB_String(t *testing.T) {
	if CircuitClosed.String() != "closed" || CircuitOpen.String() != "open" || CircuitHalfOpen.String() != "half-open" || CircuitState(99).String() != "unknown" {
		t.Error("strings")
	}
}
func TestCB_Defaults(t *testing.T) {
	c := DefaultCircuitBreakerConfig()
	if c.FailureThreshold != 5 || c.ResetTimeout != 30*time.Second || c.HalfOpenMaxAttempts != 1 {
		t.Error("defaults")
	}
}
func TestCB_Callback(t *testing.T) {
	var mu sync.Mutex
	var got []CircuitState
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: time.Minute, OnStateChange: func(_, to CircuitState) { mu.Lock(); got = append(got, to); mu.Unlock() }})
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != CircuitOpen {
		t.Error("callback")
	}
}
func TestCB_Concurrent(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 100, ResetTimeout: time.Minute})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); cb.Allow() }()
		go func() { defer wg.Done(); cb.RecordSuccess() }()
		go func() { defer wg.Done(); cb.RecordFailure() }()
	}
	wg.Wait()
}
func TestCB_ExecOK(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	called := false
	cb.Execute(context.Background(), func(ctx context.Context) error { called = true; return nil }, nil)
	if !called {
		t.Error("not called")
	}
}
func TestCB_ExecFail(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
	fb := false
	cb.Execute(context.Background(), func(ctx context.Context) error { return errors.New("e") }, func(ctx context.Context) error { fb = true; return nil })
	if !fb {
		t.Error("fb")
	}
}
func TestCB_ExecOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: time.Minute})
	cb.RecordFailure()
	cb.RecordFailure()
	op, fb := false, false
	cb.Execute(context.Background(), func(ctx context.Context) error { op = true; return nil }, func(ctx context.Context) error { fb = true; return nil })
	if op {
		t.Error("op")
	}
	if !fb {
		t.Error("fb")
	}
}
func TestCB_HalfOpenMax(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 2, ResetTimeout: 50 * time.Millisecond, HalfOpenMaxAttempts: 3})
	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)
	_ = cb.State()
	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Error("1")
	}
	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Error("2")
	}
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Error("3")
	}
}
func TestAccept_Opens(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 5, ResetTimeout: time.Minute})
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}
	if cb.State() != CircuitOpen {
		t.Fatal("open")
	}
	if cb.Allow() {
		t.Error("allow")
	}
	fb := false
	cb.Execute(context.Background(), func(ctx context.Context) error { t.Error("no"); return nil }, func(ctx context.Context) error { fb = true; return nil })
	if !fb {
		t.Error("fb")
	}
}
func TestAccept_Recovery(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3, ResetTimeout: 50 * time.Millisecond})
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	time.Sleep(60 * time.Millisecond)
	if cb.State() != CircuitHalfOpen {
		t.Fatal("ho")
	}
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Error("closed")
	}
}
