package routing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/llm-optimization-gateway/internal/request"
)

// FallbackEvent records a fallback occurrence for observability.
type FallbackEvent struct {
	// RequestID is the request that triggered the fallback.
	RequestID string `json:"request_id"`
	// FromProvider is the provider that failed.
	FromProvider string `json:"from_provider"`
	// ToProvider is the provider being tried next.
	ToProvider string `json:"to_provider"`
	// Reason is a human-readable reason for the fallback.
	Reason string `json:"reason"`
	// AttemptNumber is the attempt number that triggered the fallback.
	AttemptNumber int `json:"attempt_number"`
	// Timestamp is when the fallback occurred.
	Timestamp time.Time `json:"timestamp"`
}

// FallbackResult is the outcome of executing a fallback chain.
type FallbackResult struct {
	// Response is the successful response, if any.
	Response *ProviderResponse
	// FinalProvider is the provider that ultimately handled the request.
	FinalProvider string
	// Attempts is the total number of attempts made.
	Attempts int
	// Events is the list of fallback events that occurred.
	Events []FallbackEvent
	// Errors is the list of errors from all failed attempts.
	Errors []ProviderError
}

// ProviderError wraps an error with the provider that produced it.
type ProviderError struct {
	// Provider is the name of the provider.
	Provider string `json:"provider"`
	// Err is the error.
	Err error `json:"-"`
	// Message is the error message.
	Message string `json:"message"`
	// StatusCode is the HTTP status code, if applicable.
	StatusCode int `json:"status_code,omitempty"`
}

// Error implements the error interface.
func (pe ProviderError) Error() string {
	if pe.StatusCode > 0 {
		return fmt.Sprintf("provider %s: status=%d: %s", pe.Provider, pe.StatusCode, pe.Message)
	}
	return fmt.Sprintf("provider %s: %s", pe.Provider, pe.Message)
}

// Unwrap returns the underlying error.
func (pe ProviderError) Unwrap() error {
	return pe.Err
}

// AllProvidersFailedError is returned when all providers in the fallback chain fail.
type AllProvidersFailedError struct {
	// Errors is the list of provider errors.
	Errors []ProviderError
}

// Error implements the error interface.
func (e *AllProvidersFailedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "all %d providers failed:", len(e.Errors))
	for _, pe := range e.Errors {
		fmt.Fprintf(&b, " [%s]", pe.Error())
	}
	return b.String()
}

// FallbackExecutor executes a request through a fallback chain, trying
// providers in order until one succeeds or the chain is exhausted.
type FallbackExecutor struct {
	registry *Registry
	chain    *FallbackChain
	onEvent  func(FallbackEvent)
}

// NewFallbackExecutor creates a new FallbackExecutor.
func NewFallbackExecutor(registry *Registry, chain *FallbackChain, onEvent func(FallbackEvent)) *FallbackExecutor {
	return &FallbackExecutor{
		registry: registry,
		chain:    chain,
		onEvent:  onEvent,
	}
}

// Execute runs the request through the fallback chain, starting with the
// given primary provider. It tracks all attempts in the routing context.
func (fe *FallbackExecutor) Execute(ctx context.Context, rc *RoutingContext, primaryProvider string) (*FallbackResult, error) {
	result := &FallbackResult{}
	providers := fe.chain.Providers()
	maxAttempts := fe.chain.MaxAttempts()

	// If primaryProvider is in the chain, reorder to start with it.
	providers = reorderProviders(providers, primaryProvider)

	for i, providerName := range providers {
		if i >= maxAttempts {
			break
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		attempt := ProviderAttempt{
			Provider:      providerName,
			AttemptNumber: i + 1,
			StartTime:     time.Now(),
			IsFallback:    i > 0,
		}

		resp, err := fe.executeOnProvider(ctx, providerName, rc.Request)

		attempt.EndTime = time.Now()
		attempt.Duration = attempt.EndTime.Sub(attempt.StartTime)

		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
			attempt.StatusCode = statusCode
		}
		if err != nil {
			attempt.Error = err
			attempt.ErrorMessage = err.Error()
		}

		rc.RecordAttempt(attempt)
		result.Attempts = i + 1

		// Success path: status code in 2xx range and no error.
		if err == nil && statusCode >= 200 && statusCode < 300 {
			result.Response = resp
			result.FinalProvider = providerName
			rc.SelectedProvider = providerName
			return result, nil
		}

		// Record the error.
		provErr := ProviderError{
			Provider:   providerName,
			Err:        err,
			Message:    errorMessage(err, statusCode),
			StatusCode: statusCode,
		}
		result.Errors = append(result.Errors, provErr)

		// Check if we should fallback.
		if !fe.chain.ShouldFallback(err, statusCode) {
			// Don't fallback for this type of error; return it directly.
			return result, &provErr
		}

		// Log fallback event if there's a next provider.
		if i+1 < len(providers) && i+1 < maxAttempts {
			event := FallbackEvent{
				RequestID:     rc.RequestID,
				FromProvider:  providerName,
				ToProvider:    providers[i+1],
				Reason:        provErr.Message,
				AttemptNumber: i + 1,
				Timestamp:     time.Now(),
			}
			result.Events = append(result.Events, event)
			if fe.onEvent != nil {
				fe.onEvent(event)
			}
		}
	}

	// All providers failed.
	allErr := &AllProvidersFailedError{Errors: result.Errors}
	return result, allErr
}

// executeOnProvider sends the request to a specific provider.
func (fe *FallbackExecutor) executeOnProvider(ctx context.Context, name string, req *request.NormalizedRequest) (*ProviderResponse, error) {
	provider, err := fe.registry.Get(name)
	if err != nil {
		return nil, fmt.Errorf("provider lookup failed: %w", err)
	}
	return provider.Send(ctx, req)
}

// reorderProviders moves the primary provider to the front of the list
// while preserving the relative order of remaining providers.
func reorderProviders(providers []string, primary string) []string {
	if primary == "" || len(providers) == 0 {
		return providers
	}

	result := make([]string, 0, len(providers))
	found := false
	for _, p := range providers {
		if p == primary {
			found = true
			continue
		}
		result = append(result, p)
	}

	if found {
		result = append([]string{primary}, result...)
	} else {
		// Primary provider wasn't in the chain; prepend it.
		result = append([]string{primary}, providers...)
	}

	return result
}

// errorMessage creates a human-readable error message.
func errorMessage(err error, statusCode int) string {
	if err != nil {
		return err.Error()
	}
	if statusCode > 0 {
		return fmt.Sprintf("HTTP %d", statusCode)
	}
	return "unknown error"
}
