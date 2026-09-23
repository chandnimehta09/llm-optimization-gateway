// Package retry provides a retry wrapper that decorates any provider.Provider
// with configurable retry logic, exponential backoff, and error classification.
package retry

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"

	"github.com/llm-optimization-gateway/internal/provider"
)

// ErrorClassification describes whether an error is eligible for retry.
type ErrorClassification int

const (
	// NonRetryable indicates the error should not be retried.
	NonRetryable ErrorClassification = iota
	// Retryable indicates the error can be retried.
	Retryable
	// RetryableWithBackoff indicates the error can be retried and the caller
	// should respect a provider-specified wait time (e.g. Retry-After).
	RetryableWithBackoff
)

// Classifier determines whether errors are retryable.
type Classifier struct{}

// NewClassifier creates a new error classifier.
func NewClassifier() *Classifier {
	return &Classifier{}
}

// Classify inspects err and returns its retry classification.
func (c *Classifier) Classify(err error) ErrorClassification {
	if err == nil {
		return NonRetryable
	}

	// Context errors are not retryable — the caller cancelled.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return NonRetryable
	}

	// Authentication errors are never retryable.
	if provider.IsAuthenticationError(err) {
		return NonRetryable
	}

	// Context length errors are never retryable.
	if provider.IsContextLengthError(err) {
		return NonRetryable
	}

	// Rate limit errors are retryable with provider-specified backoff.
	if provider.IsRateLimitError(err) {
		return RetryableWithBackoff
	}

	// Provider errors: check HTTP status code.
	var pe *provider.ProviderError
	if errors.As(err, &pe) {
		return classifyStatusCode(pe.Code)
	}

	// Network-level errors are retryable.
	if isNetworkError(err) {
		return Retryable
	}

	// Timeout errors are retryable.
	if isTimeoutError(err) {
		return Retryable
	}

	// Unknown errors are not retried by default.
	return NonRetryable
}

// classifyStatusCode maps HTTP status codes to retry eligibility.
func classifyStatusCode(code int) ErrorClassification {
	switch {
	case code == 0:
		// Code 0 typically means a network/transport error occurred.
		return Retryable
	case code == 408: // Request Timeout
		return Retryable
	case code == 429: // Too Many Requests (rate limit)
		return RetryableWithBackoff
	case code >= 500 && code < 600: // Server errors
		return Retryable
	case code >= 400 && code < 500: // Client errors (other than above)
		return NonRetryable
	default:
		return NonRetryable
	}
}

// isNetworkError checks if the error is a network-level error.
func isNetworkError(err error) bool {
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}

	// Connection reset / refused.
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}

	// Check for common network error messages.
	msg := err.Error()
	networkPatterns := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"network is unreachable",
		"i/o timeout",
	}
	for _, pattern := range networkPatterns {
		if strings.Contains(strings.ToLower(msg), pattern) {
			return true
		}
	}

	return false
}

// isTimeoutError checks if the error represents a timeout.
func isTimeoutError(err error) bool {
	// net.Error with Timeout() == true.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Fallback: check error message.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded")
}
