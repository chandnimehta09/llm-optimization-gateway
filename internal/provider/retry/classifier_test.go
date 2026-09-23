package retry

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

func TestClassifier_NilError(t *testing.T) {
	c := NewClassifier()
	if got := c.Classify(nil); got != NonRetryable {
		t.Errorf("Classify(nil) = %d, want NonRetryable (%d)", got, NonRetryable)
	}
}

func TestClassifier_AuthenticationError(t *testing.T) {
	c := NewClassifier()
	err := &provider.AuthenticationError{
		ProviderError: provider.ProviderError{
			Provider: "test",
			Code:     401,
			Message:  "invalid api key",
		},
	}
	if got := c.Classify(err); got != NonRetryable {
		t.Errorf("Classify(AuthenticationError) = %d, want NonRetryable (%d)", got, NonRetryable)
	}
}

func TestClassifier_ContextLengthError(t *testing.T) {
	c := NewClassifier()
	err := &provider.ContextLengthError{
		ProviderError: provider.ProviderError{
			Provider: "test",
			Code:     400,
			Message:  "context length exceeded",
		},
		MaxTokens:       4096,
		RequestedTokens: 8000,
	}
	if got := c.Classify(err); got != NonRetryable {
		t.Errorf("Classify(ContextLengthError) = %d, want NonRetryable (%d)", got, NonRetryable)
	}
}

func TestClassifier_RateLimitError(t *testing.T) {
	c := NewClassifier()
	err := &provider.RateLimitError{
		ProviderError: provider.ProviderError{
			Provider: "test",
			Code:     429,
			Message:  "rate limited",
		},
		RetryAfter: 5 * time.Second,
	}
	if got := c.Classify(err); got != RetryableWithBackoff {
		t.Errorf("Classify(RateLimitError) = %d, want RetryableWithBackoff (%d)", got, RetryableWithBackoff)
	}
}

func TestClassifier_ServerError(t *testing.T) {
	c := NewClassifier()
	codes := []int{500, 502, 503, 504}
	for _, code := range codes {
		err := &provider.ProviderError{
			Provider: "test",
			Code:     code,
			Message:  "server error",
		}
		if got := c.Classify(err); got != Retryable {
			t.Errorf("Classify(ProviderError code=%d) = %d, want Retryable (%d)", code, got, Retryable)
		}
	}
}

func TestClassifier_ClientError_NonRetryable(t *testing.T) {
	c := NewClassifier()
	codes := []int{400, 403, 404, 422}
	for _, code := range codes {
		err := &provider.ProviderError{
			Provider: "test",
			Code:     code,
			Message:  "client error",
		}
		if got := c.Classify(err); got != NonRetryable {
			t.Errorf("Classify(ProviderError code=%d) = %d, want NonRetryable (%d)", code, got, NonRetryable)
		}
	}
}

func TestClassifier_RequestTimeout(t *testing.T) {
	c := NewClassifier()
	err := &provider.ProviderError{
		Provider: "test",
		Code:     408,
		Message:  "request timeout",
	}
	if got := c.Classify(err); got != Retryable {
		t.Errorf("Classify(ProviderError code=408) = %d, want Retryable (%d)", got, Retryable)
	}
}

func TestClassifier_RateLimitByStatusCode(t *testing.T) {
	c := NewClassifier()
	err := &provider.ProviderError{
		Provider: "test",
		Code:     429,
		Message:  "too many requests",
	}
	if got := c.Classify(err); got != RetryableWithBackoff {
		t.Errorf("Classify(ProviderError code=429) = %d, want RetryableWithBackoff (%d)", got, RetryableWithBackoff)
	}
}

func TestClassifier_ZeroCodeProviderError(t *testing.T) {
	c := NewClassifier()
	err := &provider.ProviderError{
		Provider: "test",
		Code:     0,
		Message:  "network error",
		Err:      errors.New("connection refused"),
	}
	if got := c.Classify(err); got != Retryable {
		t.Errorf("Classify(ProviderError code=0) = %d, want Retryable (%d)", got, Retryable)
	}
}

func TestClassifier_ContextCanceled(t *testing.T) {
	c := NewClassifier()
	if got := c.Classify(context.Canceled); got != NonRetryable {
		t.Errorf("Classify(context.Canceled) = %d, want NonRetryable (%d)", got, NonRetryable)
	}
}

func TestClassifier_ContextDeadlineExceeded(t *testing.T) {
	c := NewClassifier()
	if got := c.Classify(context.DeadlineExceeded); got != NonRetryable {
		t.Errorf("Classify(context.DeadlineExceeded) = %d, want NonRetryable (%d)", got, NonRetryable)
	}
}

func TestClassifier_NetworkError_OpError(t *testing.T) {
	c := NewClassifier()
	err := &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: errors.New("connection refused"),
	}
	if got := c.Classify(err); got != Retryable {
		t.Errorf("Classify(net.OpError) = %d, want Retryable (%d)", got, Retryable)
	}
}

func TestClassifier_NetworkError_DNSError(t *testing.T) {
	c := NewClassifier()
	err := &net.DNSError{
		Err:  "no such host",
		Name: "api.example.com",
	}
	if got := c.Classify(err); got != Retryable {
		t.Errorf("Classify(net.DNSError) = %d, want Retryable (%d)", got, Retryable)
	}
}

func TestClassifier_NetworkError_ConnectionRefused(t *testing.T) {
	c := NewClassifier()
	err := syscall.ECONNREFUSED
	if got := c.Classify(err); got != Retryable {
		t.Errorf("Classify(ECONNREFUSED) = %d, want Retryable (%d)", got, Retryable)
	}
}

func TestClassifier_NetworkError_ConnectionReset(t *testing.T) {
	c := NewClassifier()
	err := syscall.ECONNRESET
	if got := c.Classify(err); got != Retryable {
		t.Errorf("Classify(ECONNRESET) = %d, want Retryable (%d)", got, Retryable)
	}
}

func TestClassifier_TimeoutError(t *testing.T) {
	c := NewClassifier()
	err := &timeoutErr{msg: "i/o timeout"}
	if got := c.Classify(err); got != Retryable {
		t.Errorf("Classify(timeout) = %d, want Retryable (%d)", got, Retryable)
	}
}

func TestClassifier_UnknownError(t *testing.T) {
	c := NewClassifier()
	err := errors.New("something completely unexpected")
	if got := c.Classify(err); got != NonRetryable {
		t.Errorf("Classify(unknown) = %d, want NonRetryable (%d)", got, NonRetryable)
	}
}

func TestClassifier_WrappedRateLimitError(t *testing.T) {
	c := NewClassifier()
	inner := &provider.RateLimitError{
		ProviderError: provider.ProviderError{
			Provider: "test",
			Code:     429,
			Message:  "rate limited",
		},
		RetryAfter: 3 * time.Second,
	}
	err := errors.Join(errors.New("wrapper"), inner)
	if got := c.Classify(err); got != RetryableWithBackoff {
		t.Errorf("Classify(wrapped RateLimitError) = %d, want RetryableWithBackoff (%d)", got, RetryableWithBackoff)
	}
}

// timeoutErr implements net.Error with Timeout() returning true.
type timeoutErr struct {
	msg string
}

func (e *timeoutErr) Error() string   { return e.msg }
func (e *timeoutErr) Timeout() bool   { return true }
func (e *timeoutErr) Temporary() bool { return true }
