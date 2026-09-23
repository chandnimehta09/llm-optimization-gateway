package retry

import (
	"math/rand"
	"testing"
	"time"
)

func TestBackoff_FullJitter_Increases(t *testing.T) {
	cfg := BackoffConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       FullJitter,
	}
	// Use a fixed seed for reproducibility.
	rng := rand.New(rand.NewSource(42))
	b := NewBackoffWithRng(cfg, rng)

	// Collect delays for multiple attempts.
	// The max possible delay should increase exponentially.
	maxPossible := make([]time.Duration, 5)
	for i := 0; i < 5; i++ {
		maxPossible[i] = time.Duration(float64(cfg.InitialDelay) * pow(cfg.Multiplier, float64(i)))
		if maxPossible[i] > cfg.MaxDelay {
			maxPossible[i] = cfg.MaxDelay
		}
	}

	// Verify max possible increases.
	for i := 1; i < len(maxPossible); i++ {
		if maxPossible[i] < maxPossible[i-1] {
			t.Errorf("max possible delay should increase: attempt %d (%v) < attempt %d (%v)",
				i, maxPossible[i], i-1, maxPossible[i-1])
		}
	}

	// Verify actual delays are within bounds.
	for i := 0; i < 5; i++ {
		delay := b.Next(i)
		if delay < time.Millisecond {
			t.Errorf("attempt %d: delay %v below minimum 1ms", i, delay)
		}
		if delay > maxPossible[i] {
			t.Errorf("attempt %d: delay %v exceeds max possible %v", i, delay, maxPossible[i])
		}
	}
}

func TestBackoff_FullJitter_RespectsCap(t *testing.T) {
	cfg := BackoffConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       FullJitter,
	}
	rng := rand.New(rand.NewSource(42))
	b := NewBackoffWithRng(cfg, rng)

	for i := 0; i < 20; i++ {
		delay := b.Next(i)
		if delay > cfg.MaxDelay {
			t.Errorf("attempt %d: delay %v exceeds max delay %v", i, delay, cfg.MaxDelay)
		}
	}
}

func TestBackoff_FullJitter_MinimumDelay(t *testing.T) {
	cfg := BackoffConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       FullJitter,
	}
	// Run many iterations to check minimum bound.
	rng := rand.New(rand.NewSource(42))
	b := NewBackoffWithRng(cfg, rng)

	for i := 0; i < 100; i++ {
		delay := b.Next(i % 5)
		if delay < time.Millisecond {
			t.Errorf("attempt %d: delay %v below minimum 1ms", i, delay)
		}
	}
}

func TestBackoff_DecorrelatedJitter(t *testing.T) {
	cfg := BackoffConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       DecorrelatedJitter,
	}
	rng := rand.New(rand.NewSource(42))
	b := NewBackoffWithRng(cfg, rng)

	var delays []time.Duration
	for i := 0; i < 10; i++ {
		delay := b.Next(i)
		delays = append(delays, delay)

		// Each delay should be >= InitialDelay.
		if delay < cfg.InitialDelay {
			t.Errorf("attempt %d: delay %v below initial delay %v", i, delay, cfg.InitialDelay)
		}

		// Each delay should be <= MaxDelay.
		if delay > cfg.MaxDelay {
			t.Errorf("attempt %d: delay %v exceeds max delay %v", i, delay, cfg.MaxDelay)
		}
	}
}

func TestBackoff_DecorrelatedJitter_RespectsCap(t *testing.T) {
	cfg := BackoffConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     300 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       DecorrelatedJitter,
	}
	rng := rand.New(rand.NewSource(42))
	b := NewBackoffWithRng(cfg, rng)

	for i := 0; i < 20; i++ {
		delay := b.Next(i)
		if delay > cfg.MaxDelay {
			t.Errorf("attempt %d: delay %v exceeds max delay %v", i, delay, cfg.MaxDelay)
		}
	}
}

func TestBackoff_Reset(t *testing.T) {
	cfg := BackoffConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       DecorrelatedJitter,
	}
	rng := rand.New(rand.NewSource(42))
	b := NewBackoffWithRng(cfg, rng)

	// Advance a few attempts.
	for i := 0; i < 5; i++ {
		b.Next(i)
	}

	// Reset and check that lastDelay is back to initial.
	b.Reset()
	if b.lastDelay != cfg.InitialDelay {
		t.Errorf("after Reset, lastDelay = %v, want %v", b.lastDelay, cfg.InitialDelay)
	}
}

func TestBackoff_DefaultConfig(t *testing.T) {
	cfg := DefaultBackoffConfig()
	if cfg.InitialDelay != 500*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 500ms", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", cfg.Multiplier)
	}
	if cfg.Jitter != FullJitter {
		t.Errorf("Jitter = %d, want FullJitter (%d)", cfg.Jitter, FullJitter)
	}
}

// pow computes base^exp.
func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}
