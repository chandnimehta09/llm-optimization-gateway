package reliability

import (
	"testing"
	"time"
)

func TestBackoffCalculator_ExponentialNoJitter(t *testing.T) {
	b := NewBackoffCalculator(100*time.Millisecond, 10*time.Second, 2.0, JitterNone)

	expected := []time.Duration{
		100 * time.Millisecond,  // 100ms * 2^0
		200 * time.Millisecond,  // 100ms * 2^1
		400 * time.Millisecond,  // 100ms * 2^2
		800 * time.Millisecond,  // 100ms * 2^3
		1600 * time.Millisecond, // 100ms * 2^4
	}

	for i, exp := range expected {
		got := b.NextDelay(i)
		if got != exp {
			t.Errorf("attempt %d: expected %v, got %v", i, exp, got)
		}
	}
}

func TestBackoffCalculator_MaxDelayCap(t *testing.T) {
	b := NewBackoffCalculator(100*time.Millisecond, 500*time.Millisecond, 2.0, JitterNone)

	got := b.NextDelay(10) // Would be 100ms * 2^10 = 102.4s
	if got > 500*time.Millisecond {
		t.Errorf("expected delay capped at 500ms, got %v", got)
	}
}

func TestBackoffCalculator_FullJitter(t *testing.T) {
	b := NewBackoffCalculator(100*time.Millisecond, 10*time.Second, 2.0, JitterFull)

	for i := 0; i < 100; i++ {
		delay := b.NextDelay(0)
		if delay < 0 || delay >= 100*time.Millisecond {
			t.Errorf("full jitter attempt 0: delay %v should be in [0, 100ms)", delay)
		}
	}
}

func TestBackoffCalculator_FullJitter_Increases(t *testing.T) {
	b := NewBackoffCalculator(100*time.Millisecond, 10*time.Second, 2.0, JitterFull)

	// With full jitter the max possible delay increases with each attempt.
	// Verify that later attempts can produce larger delays on average.
	var sum0, sum5 time.Duration
	trials := 1000
	for i := 0; i < trials; i++ {
		sum0 += b.NextDelay(0)
		sum5 += b.NextDelay(5)
	}
	avg0 := sum0 / time.Duration(trials)
	avg5 := sum5 / time.Duration(trials)
	if avg5 <= avg0 {
		t.Errorf("expected avg delay at attempt 5 (%v) > attempt 0 (%v)", avg5, avg0)
	}
}

func TestBackoffCalculator_DecorrelatedJitter(t *testing.T) {
	b := NewBackoffCalculator(100*time.Millisecond, 10*time.Second, 2.0, JitterDecorrelated)

	// First attempt should return initial delay.
	d0 := b.NextDelay(0)
	if d0 != 100*time.Millisecond {
		t.Errorf("expected first decorrelated delay to be initial_delay, got %v", d0)
	}

	// Subsequent attempts should vary.
	for i := 1; i < 10; i++ {
		d := b.NextDelay(i)
		if d < 100*time.Millisecond {
			t.Errorf("decorrelated delay %v should be >= initial_delay", d)
		}
		if d > 10*time.Second {
			t.Errorf("decorrelated delay %v should be <= max_delay", d)
		}
	}
}

func TestBackoffCalculator_NegativeAttempt(t *testing.T) {
	b := NewBackoffCalculator(100*time.Millisecond, 10*time.Second, 2.0, JitterNone)
	got := b.NextDelay(-1)
	if got != 100*time.Millisecond {
		t.Errorf("negative attempt should be treated as 0, got %v", got)
	}
}

func TestBackoffCalculator_Reset(t *testing.T) {
	b := NewBackoffCalculator(100*time.Millisecond, 10*time.Second, 2.0, JitterDecorrelated)

	_ = b.NextDelay(0)
	_ = b.NextDelay(1)
	_ = b.NextDelay(2)

	b.Reset()

	d := b.NextDelay(0)
	if d != 100*time.Millisecond {
		t.Errorf("after reset, first delay should be initial_delay, got %v", d)
	}
}

func TestBackoffCalculator_DefaultValues(t *testing.T) {
	b := NewBackoffCalculator(0, 0, 0, JitterNone)
	d := b.NextDelay(0)
	if d != DefaultInitialDelay {
		t.Errorf("expected default initial delay %v, got %v", DefaultInitialDelay, d)
	}
}

func TestComputeDelay(t *testing.T) {
	d := ComputeDelay(0, 100*time.Millisecond, 10*time.Second, 2.0, JitterNone)
	if d != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", d)
	}

	d = ComputeDelay(3, 100*time.Millisecond, 10*time.Second, 2.0, JitterNone)
	if d != 800*time.Millisecond {
		t.Errorf("expected 800ms, got %v", d)
	}
}
