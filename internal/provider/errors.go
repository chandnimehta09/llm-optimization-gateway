package provider

import (
	"errors"
	"fmt"
	"time"
)

// ProviderError is the base error type for all provider-related errors.
// It wraps an underlying error with provider context.
type ProviderError struct {
	// Provider is the name of the provider that produced the error.
	Provider string
	// Code is a machine-readable error code (e.g., HTTP status code).
	Code int
	// Message is a human-readable error description.
	Message string
	// Err is the underlying error, if any.
	Err error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("provider %s (code %d): %s: %v", e.Provider, e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("provider %s (code %d): %s", e.Provider, e.Code, e.Message)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// RateLimitError indicates the provider rejected the request due to rate limiting.
type RateLimitError struct {
	ProviderError
	// RetryAfter is the duration the caller should wait before retrying.
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("provider %s: rate limited (retry after %s): %s", e.Provider, e.RetryAfter, e.Message)
}

// ContextLengthError indicates the request exceeded the model's context window.
type ContextLengthError struct {
	ProviderError
	// MaxTokens is the model's maximum context length.
	MaxTokens int
	// RequestedTokens is the number of tokens in the request.
	RequestedTokens int
}

func (e *ContextLengthError) Error() string {
	return fmt.Sprintf("provider %s: context length exceeded (max %d, requested %d): %s",
		e.Provider, e.MaxTokens, e.RequestedTokens, e.Message)
}

// AuthenticationError indicates the provider rejected the request due to invalid credentials.
type AuthenticationError struct {
	ProviderError
}

func (e *AuthenticationError) Error() string {
	return fmt.Sprintf("provider %s: authentication failed: %s", e.Provider, e.Message)
}

// IsRateLimitError checks whether an error is a RateLimitError.
func IsRateLimitError(err error) bool {
	var rle *RateLimitError
	return errors.As(err, &rle)
}

// IsContextLengthError checks whether an error is a ContextLengthError.
func IsContextLengthError(err error) bool {
	var cle *ContextLengthError
	return errors.As(err, &cle)
}

// IsAuthenticationError checks whether an error is an AuthenticationError.
func IsAuthenticationError(err error) bool {
	var ae *AuthenticationError
	return errors.As(err, &ae)
}

// IsProviderError checks whether an error is a ProviderError (or any subtype).
func IsProviderError(err error) bool {
	var pe *ProviderError
	return errors.As(err, &pe)
}
