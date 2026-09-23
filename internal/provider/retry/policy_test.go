package retry

import (
	"math/rand"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

func TestPolicy_DefaultPolicy(t *testing.T) {
	p := DefaultPolicy()
	if p.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", p.MaxAttempts)
	}
	if p.Backoff.InitialDelay != 500*time.Millisecond {
		t.Errorf("Backoff.InitialDelay = %v, want 500ms", p.Backoff.InitialDelay)
	}
}

func TestPolicy_Validate_Defaults(t *testing.T) {
	p := Policy{}
	p.Validate()

	if p.MaxAttempts != 1 {
		t.Errorf("MaxAttempts = %d, want 1 (minimum)", p.MaxAttempts)
	}
	if p.Backoff.InitialDelay != 500*time.Millisecond {
		t.Errorf("Backoff.InitialDelay = %v, want 500ms", p.Backoff.InitialDelay)
	}
	if p.Backoff.MaxDelay != 30*time.Second {
		t.Errorf("Backoff.MaxDelay = %v, want 30s", p.Backoff.MaxDelay)
	}
	if p.Backoff.Multiplier != 2.0 {
		t.Errorf("Backoff.Multiplier = %f, want 2.0", p.Backoff.Multiplier)
	}
}

func TestPolicy_Validate_NegativeValues(t *testing.T) {
	p := Policy{
		MaxAttempts: -1,
		Backoff: BackoffConfig{
			InitialDelay: -1,
			MaxDelay:     -1,
			Multiplier:   -1,
		},
	}
	p.Validate()

	if p.MaxAttempts != 1 {
		t.Errorf("MaxAttempts = %d, want 1", p.MaxAttempts)
	}
	if p.Backoff.InitialDelay != 500*time.Millisecond {
		t.Errorf("Backoff.InitialDelay = %v, want 500ms", p.Backoff.InitialDelay)
	}
	if p.Backoff.MaxDelay != 30*time.Second {
		t.Errorf("Backoff.MaxDelay = %v, want 30s", p.Backoff.MaxDelay)
	}
	if p.Backoff.Multiplier != 2.0 {
		t.Errorf("Backoff.Multiplier = %f, want 2.0", p.Backoff.Multiplier)
	}
}

func TestEvaluate_MaxAttemptsExhausted(t *testing.T) {
	policy := Policy{MaxAttempts: 3, Backoff: DefaultBackoffConfig()}
	classifier := NewClassifier()
	backoff := NewBackoff(policy.Backoff)

	err := &provider.ProviderError{Provider: "test", Code: 500, Message: "server error"}
	decision := Evaluate(policy, classifier, backoff, 2, err) // attempt 2, max 3 → last attempt
	if decision.ShouldRetry {
		t.Error("expected ShouldRetry=false when attempts exhausted")
	}
	if decision.Reason != "max attempts exhausted" {
		t.Errorf("Reason = %q, want %q", decision.Reason, "max attempts exhausted")
	}
}

func TestEvaluate_NonRetryableError(t *testing.T) {
	policy := Policy{MaxAttempts: 3, Backoff: DefaultBackoffConfig()}
	classifier := NewClassifier()
	backoff := NewBackoff(policy.Backoff)

	err := &provider.AuthenticationError{
		ProviderError: provider.ProviderError{Provider: "test", Code: 401, Message: "bad key"},
	}
	decision := Evaluate(policy, classifier, backoff, 0, err)
	if decision.ShouldRetry {
		t.Error("expected ShouldRetry=false for auth error")
	}
	if decision.Reason != "error is not retryable" {
		t.Errorf("Reason = %q, want %q", decision.Reason, "error is not retryable")
	}
}

func TestEvaluate_RetryableError(t *testing.T) {
	policy := Policy{MaxAttempts: 3, Backoff: DefaultBackoffConfig()}
	classifier := NewClassifier()
	rng := rand.New(rand.NewSource(42))
	backoff := NewBackoffWithRng(policy.Backoff, rng)

	err := &provider.ProviderError{Provider: "test", Code: 500, Message: "server error"}
	decision := Evaluate(policy, classifier, backoff, 0, err)
	if !decision.ShouldRetry {
		t.Error("expected ShouldRetry=true for server error")
	}
	if decision.Delay <= 0 {
		t.Errorf("expected positive delay, got %v", decision.Delay)
	}
	if decision.Reason != "retryable error" {
		t.Errorf("Reason = %q, want %q", decision.Reason, "retryable error")
	}
}

func TestEvaluate_RateLimitWithRetryAfter(t *testing.T) {
	policy := Policy{MaxAttempts: 3, Backoff: DefaultBackoffConfig()}
	classifier := NewClassifier()
	backoff := NewBackoff(policy.Backoff)

	err := &provider.RateLimitError{
		ProviderError: provider.ProviderError{Provider: "test", Code: 429, Message: "rate limited"},
		RetryAfter:    5 * time.Second,
	}
	decision := Evaluate(policy, classifier, backoff, 0, err)
	if !decision.ShouldRetry {
		t.Error("expected ShouldRetry=true for rate limit")
	}
	if decision.Delay != 5*time.Second {
		t.Errorf("Delay = %v, want 5s (from Retry-After)", decision.Delay)
	}
}

func TestEvaluate_RateLimitWithoutRetryAfter(t *testing.T) {
	policy := Policy{MaxAttempts: 3, Backoff: DefaultBackoffConfig()}
	classifier := NewClassifier()
	rng := rand.New(rand.NewSource(42))
	backoff := NewBackoffWithRng(policy.Backoff, rng)

	err := &provider.RateLimitError{
		ProviderError: provider.ProviderError{Provider: "test", Code: 429, Message: "rate limited"},
		RetryAfter:    0, // No Retry-After header.
	}
	decision := Evaluate(policy, classifier, backoff, 0, err)
	if !decision.ShouldRetry {
		t.Error("expected ShouldRetry=true for rate limit")
	}
	if decision.Delay <= 0 {
		t.Errorf("expected positive delay from backoff, got %v", decision.Delay)
	}
}

func TestEvaluate_SingleAttemptPolicy(t *testing.T) {
	policy := Policy{MaxAttempts: 1, Backoff: DefaultBackoffConfig()}
	classifier := NewClassifier()
	backoff := NewBackoff(policy.Backoff)

	err := &provider.ProviderError{Provider: "test", Code: 500, Message: "server error"}
	decision := Evaluate(policy, classifier, backoff, 0, err)
	if decision.ShouldRetry {
		t.Error("expected ShouldRetry=false when MaxAttempts=1")
	}
}
