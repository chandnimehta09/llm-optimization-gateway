package reliability

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
)

// ErrorClass categorizes an error for retry decisions.
type ErrorClass int

const (
	// ErrorClassNonRetryable means the error should not be retried.
	ErrorClassNonRetryable ErrorClass = iota
	// ErrorClassRetryable means the error is transient and can be retried.
	ErrorClassRetryable
	// ErrorClassThrottled means the caller is being rate limited.
	ErrorClassThrottled
)

// String returns the string representation of an ErrorClass.
func (c ErrorClass) String() string {
	switch c {
	case ErrorClassNonRetryable:
		return "non-retryable"
	case ErrorClassRetryable:
		return "retryable"
	case ErrorClassThrottled:
		return "throttled"
	default:
		return "unknown"
	}
}

// ErrorClassifier determines whether an error is retryable.
type ErrorClassifier struct {
	// Custom classifiers are checked first in order.
	customRules []ClassifierRule
}

// ClassifierRule is a function that inspects an error and returns its class.
// Returning true for the second value means the rule handled the error.
type ClassifierRule func(err error) (ErrorClass, bool)

// NewErrorClassifier creates a new classifier with default rules.
func NewErrorClassifier() *ErrorClassifier {
	return &ErrorClassifier{}
}

// AddRule appends a custom classification rule. Custom rules are evaluated
// before the built-in rules; the first rule that returns handled=true wins.
func (c *ErrorClassifier) AddRule(rule ClassifierRule) {
	c.customRules = append(c.customRules, rule)
}

// Classify returns the ErrorClass for the given error.
func (c *ErrorClassifier) Classify(err error) ErrorClass {
	if err == nil {
		return ErrorClassNonRetryable
	}

	// Check custom rules first.
	for _, rule := range c.customRules {
		if class, handled := rule(err); handled {
			return class
		}
	}

	// Check for explicit retryable/non-retryable markers.
	var retryable *RetryableError
	if errors.As(err, &retryable) {
		if retryable.IsRetryable {
			return ErrorClassRetryable
		}
		return ErrorClassNonRetryable
	}

	var throttled *ThrottledError
	if errors.As(err, &throttled) {
		return ErrorClassThrottled
	}

	// Context errors are not retryable.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrorClassNonRetryable
	}

	// Network errors.
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return ErrorClassRetryable
		}
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return classifyNetOpError(opErr)
	}

	// DNS errors are retryable.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.Temporary() {
			return ErrorClassRetryable
		}
		return ErrorClassNonRetryable
	}

	// Syscall-level connection refused/reset are retryable.
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EPIPE) {
		return ErrorClassRetryable
	}

	// Check for common retryable error strings as a last resort.
	msg := err.Error()
	if containsRetryableMessage(msg) {
		return ErrorClassRetryable
	}

	return ErrorClassNonRetryable
}

// IsRetryable is a convenience function that returns true if the error is retryable or throttled.
func (c *ErrorClassifier) IsRetryable(err error) bool {
	class := c.Classify(err)
	return class == ErrorClassRetryable || class == ErrorClassThrottled
}

func classifyNetOpError(err *net.OpError) ErrorClass {
	if err.Temporary() {
		return ErrorClassRetryable
	}
	if err.Op == "dial" || err.Op == "read" || err.Op == "write" {
		return ErrorClassRetryable
	}
	return ErrorClassNonRetryable
}

func containsRetryableMessage(msg string) bool {
	lower := strings.ToLower(msg)
	patterns := []string{
		"connection reset",
		"connection refused",
		"broken pipe",
		"temporary failure",
		"service unavailable",
		"bad gateway",
		"gateway timeout",
		"too many requests",
		"rate limit",
		"server error",
		"internal server error",
		"503",
		"502",
		"504",
		"429",
	}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// RetryableError wraps an error with an explicit retryable flag.
type RetryableError struct {
	Err         error
	IsRetryable bool
}

func (e *RetryableError) Error() string {
	if e.Err == nil {
		return "retryable error"
	}
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NewRetryableError wraps an error as explicitly retryable.
func NewRetryableError(err error) *RetryableError {
	return &RetryableError{Err: err, IsRetryable: true}
}

// NewNonRetryableError wraps an error as explicitly non-retryable.
func NewNonRetryableError(err error) *RetryableError {
	return &RetryableError{Err: err, IsRetryable: false}
}

// ThrottledError indicates the caller is being rate limited.
type ThrottledError struct {
	Err        error
	RetryAfter interface{} // time.Duration or time.Time
}

func (e *ThrottledError) Error() string {
	if e.Err == nil {
		return "throttled"
	}
	return e.Err.Error()
}

func (e *ThrottledError) Unwrap() error {
	return e.Err
}

// NewThrottledError wraps an error as a throttled error.
func NewThrottledError(err error) *ThrottledError {
	return &ThrottledError{Err: err}
}

// CircuitOpenError is returned when a circuit breaker is open.
type CircuitOpenError struct {
	Provider string
}

func (e *CircuitOpenError) Error() string {
	return "circuit breaker is open for provider: " + e.Provider
}

// TimeoutError indicates a timeout in a specific phase.
type TimeoutError struct {
	Provider string
	Phase    TimeoutPhase
	Timeout  interface{} // time.Duration
	Err      error
}

func (e *TimeoutError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return "timeout in phase " + string(e.Phase) + " for provider " + e.Provider
}

func (e *TimeoutError) Unwrap() error {
	return e.Err
}
