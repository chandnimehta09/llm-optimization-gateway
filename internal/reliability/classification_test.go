package reliability

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestErrorClassifier_NilError(t *testing.T) {
	c := NewErrorClassifier()
	if got := c.Classify(nil); got != ErrorClassNonRetryable {
		t.Errorf("nil error should be non-retryable, got %s", got)
	}
}

func TestErrorClassifier_RetryableError(t *testing.T) {
	c := NewErrorClassifier()
	err := NewRetryableError(errors.New("transient"))
	if got := c.Classify(err); got != ErrorClassRetryable {
		t.Errorf("retryable error should be retryable, got %s", got)
	}
}

func TestErrorClassifier_NonRetryableError(t *testing.T) {
	c := NewErrorClassifier()
	err := NewNonRetryableError(errors.New("permanent"))
	if got := c.Classify(err); got != ErrorClassNonRetryable {
		t.Errorf("non-retryable error should be non-retryable, got %s", got)
	}
}

func TestErrorClassifier_ThrottledError(t *testing.T) {
	c := NewErrorClassifier()
	err := NewThrottledError(errors.New("rate limited"))
	if got := c.Classify(err); got != ErrorClassThrottled {
		t.Errorf("throttled error should be throttled, got %s", got)
	}
}

func TestErrorClassifier_ContextCanceled(t *testing.T) {
	c := NewErrorClassifier()
	if got := c.Classify(context.Canceled); got != ErrorClassNonRetryable {
		t.Errorf("context.Canceled should be non-retryable, got %s", got)
	}
}

func TestErrorClassifier_ContextDeadlineExceeded(t *testing.T) {
	c := NewErrorClassifier()
	if got := c.Classify(context.DeadlineExceeded); got != ErrorClassNonRetryable {
		t.Errorf("context.DeadlineExceeded should be non-retryable, got %s", got)
	}
}

func TestErrorClassifier_NetworkTimeout(t *testing.T) {
	c := NewErrorClassifier()
	err := &net.OpError{Op: "read", Err: &timeoutError{}}
	if got := c.Classify(err); got != ErrorClassRetryable {
		t.Errorf("network timeout should be retryable, got %s", got)
	}
}

func TestErrorClassifier_ConnectionRefused(t *testing.T) {
	c := NewErrorClassifier()
	err := errors.New("connection refused")
	if got := c.Classify(err); got != ErrorClassRetryable {
		t.Errorf("connection refused should be retryable, got %s", got)
	}
}

func TestErrorClassifier_ServerError(t *testing.T) {
	c := NewErrorClassifier()
	err := errors.New("502 bad gateway")
	if got := c.Classify(err); got != ErrorClassRetryable {
		t.Errorf("502 error should be retryable, got %s", got)
	}
}

func TestErrorClassifier_RateLimitMessage(t *testing.T) {
	c := NewErrorClassifier()
	err := errors.New("rate limit exceeded")
	if got := c.Classify(err); got != ErrorClassRetryable {
		t.Errorf("rate limit message should be retryable, got %s", got)
	}
}

func TestErrorClassifier_UnknownError(t *testing.T) {
	c := NewErrorClassifier()
	err := errors.New("something weird happened")
	if got := c.Classify(err); got != ErrorClassNonRetryable {
		t.Errorf("unknown error should be non-retryable, got %s", got)
	}
}

func TestErrorClassifier_CustomRule(t *testing.T) {
	c := NewErrorClassifier()
	customErr := errors.New("custom-retryable")

	c.AddRule(func(err error) (ErrorClass, bool) {
		if err.Error() == "custom-retryable" {
			return ErrorClassRetryable, true
		}
		return 0, false
	})

	if got := c.Classify(customErr); got != ErrorClassRetryable {
		t.Errorf("custom rule should classify as retryable, got %s", got)
	}
}

func TestErrorClassifier_CustomRulePriority(t *testing.T) {
	c := NewErrorClassifier()
	// This would normally be non-retryable.
	err := NewNonRetryableError(errors.New("overridden"))

	c.AddRule(func(err error) (ErrorClass, bool) {
		return ErrorClassRetryable, true
	})

	// Custom rule should take priority.
	if got := c.Classify(err); got != ErrorClassRetryable {
		t.Errorf("custom rule should override default, got %s", got)
	}
}

func TestErrorClassifier_IsRetryable(t *testing.T) {
	c := NewErrorClassifier()

	if c.IsRetryable(nil) {
		t.Error("nil should not be retryable")
	}
	if !c.IsRetryable(NewRetryableError(errors.New("retry"))) {
		t.Error("retryable error should be retryable")
	}
	if !c.IsRetryable(NewThrottledError(errors.New("throttle"))) {
		t.Error("throttled error should be retryable")
	}
	if c.IsRetryable(NewNonRetryableError(errors.New("no"))) {
		t.Error("non-retryable error should not be retryable")
	}
}

func TestErrorClass_String(t *testing.T) {
	tests := []struct {
		class    ErrorClass
		expected string
	}{
		{ErrorClassNonRetryable, "non-retryable"},
		{ErrorClassRetryable, "retryable"},
		{ErrorClassThrottled, "throttled"},
		{ErrorClass(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.class.String(); got != tt.expected {
			t.Errorf("%d.String() = %s, want %s", tt.class, got, tt.expected)
		}
	}
}

func TestRetryableError_Unwrap(t *testing.T) {
	inner := errors.New("inner")
	err := NewRetryableError(inner)
	if !errors.Is(err, inner) {
		t.Error("should unwrap to inner error")
	}
}

func TestRetryableError_NilError(t *testing.T) {
	err := &RetryableError{Err: nil, IsRetryable: true}
	if err.Error() != "retryable error" {
		t.Errorf("expected 'retryable error', got %s", err.Error())
	}
}

func TestThrottledError_Unwrap(t *testing.T) {
	inner := errors.New("inner")
	err := NewThrottledError(inner)
	if !errors.Is(err, inner) {
		t.Error("should unwrap to inner error")
	}
}

func TestThrottledError_NilError(t *testing.T) {
	err := &ThrottledError{Err: nil}
	if err.Error() != "throttled" {
		t.Errorf("expected 'throttled', got %s", err.Error())
	}
}

func TestCircuitOpenError(t *testing.T) {
	err := &CircuitOpenError{Provider: "openai"}
	if err.Error() != "circuit breaker is open for provider: openai" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestTimeoutError(t *testing.T) {
	err := &TimeoutError{Provider: "openai", Phase: PhaseTotal}
	if err.Error() != "timeout in phase total for provider openai" {
		t.Errorf("unexpected error message: %s", err.Error())
	}

	inner := errors.New("deadline exceeded")
	err2 := &TimeoutError{Provider: "anthropic", Phase: PhaseRead, Err: inner}
	if err2.Error() != "deadline exceeded" {
		t.Errorf("should use inner error message, got %s", err2.Error())
	}
	if !errors.Is(err2, inner) {
		t.Error("should unwrap to inner error")
	}
}

// timeoutError implements net.Error for testing.
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "i/o timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }
