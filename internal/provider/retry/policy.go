package retry

import (
	"errors"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

// Policy defines the retry behavior for a provider wrapper.
type Policy struct {
	// MaxAttempts is the total number of attempts (including the initial request).
	// A value of 1 means no retries. A value of 3 means up to 2 retries.
	MaxAttempts int

	// Backoff holds the backoff configuration for calculating delays between retries.
	Backoff BackoffConfig

	// RetryableStatusCodes allows overriding which HTTP status codes are retryable.
	// If nil, the default classifier is used.
	RetryableStatusCodes []int
}

// DefaultPolicy returns a reasonable default retry policy.
func DefaultPolicy() Policy {
	return Policy{
		MaxAttempts: 3,
		Backoff:     DefaultBackoffConfig(),
	}
}

// Validate checks that the policy configuration is valid and applies defaults.
func (p *Policy) Validate() {
	if p.MaxAttempts < 1 {
		p.MaxAttempts = 1
	}
	if p.Backoff.InitialDelay <= 0 {
		p.Backoff.InitialDelay = 500 * time.Millisecond
	}
	if p.Backoff.MaxDelay <= 0 {
		p.Backoff.MaxDelay = 30 * time.Second
	}
	if p.Backoff.Multiplier <= 0 {
		p.Backoff.Multiplier = 2.0
	}
}

// RetryDecision encapsulates the outcome of evaluating whether to retry.
type RetryDecision struct {
	// ShouldRetry indicates whether the request should be retried.
	ShouldRetry bool
	// Delay is the duration to wait before retrying.
	Delay time.Duration
	// Reason is a human-readable explanation for the decision.
	Reason string
}

// Evaluate determines whether a retry should occur for the given attempt and error.
// attempt is 0-indexed (0 = evaluating whether to do the first retry after initial failure).
func Evaluate(policy Policy, classifier *Classifier, backoff *Backoff, attempt int, err error) RetryDecision {
	// Check if we've exhausted attempts.
	// attempt is the retry number (0-based). MaxAttempts includes the initial request.
	// So we can retry if attempt < MaxAttempts - 1.
	if attempt >= policy.MaxAttempts-1 {
		return RetryDecision{
			ShouldRetry: false,
			Reason:      "max attempts exhausted",
		}
	}

	// Classify the error.
	classification := classifier.Classify(err)

	switch classification {
	case NonRetryable:
		return RetryDecision{
			ShouldRetry: false,
			Reason:      "error is not retryable",
		}

	case RetryableWithBackoff:
		delay := backoff.Next(attempt)
		// Check for Retry-After from rate limit errors.
		if retryAfter := extractRetryAfter(err); retryAfter > 0 {
			delay = retryAfter
		}
		return RetryDecision{
			ShouldRetry: true,
			Delay:       delay,
			Reason:      "retryable error with provider-specified backoff",
		}

	case Retryable:
		delay := backoff.Next(attempt)
		return RetryDecision{
			ShouldRetry: true,
			Delay:       delay,
			Reason:      "retryable error",
		}

	default:
		return RetryDecision{
			ShouldRetry: false,
			Reason:      "unknown classification",
		}
	}
}

// extractRetryAfter pulls the RetryAfter duration from a RateLimitError if present.
func extractRetryAfter(err error) time.Duration {
	var rle *provider.RateLimitError
	if errors.As(err, &rle) && rle.RetryAfter > 0 {
		return rle.RetryAfter
	}
	return 0
}
