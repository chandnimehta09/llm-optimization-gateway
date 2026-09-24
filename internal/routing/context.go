package routing

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
)

// contextKey is an unexported type for context keys in this package.
type contextKey int

const (
	routingContextKey contextKey = iota
)

// RoutingContext tracks the state of a single request as it moves through the
// routing engine, including provider selection, fallback attempts, and timing.
type RoutingContext struct {
	mu sync.RWMutex

	// RequestID is the unique identifier for this request.
	RequestID string
	// Request is the normalized request being routed.
	Request *request.NormalizedRequest
	// Decision is the policy decision guiding routing.
	Decision *policy.PolicyDecision
	// SelectedProvider is the currently selected provider name.
	SelectedProvider string
	// Attempts tracks all provider attempts made for this request.
	Attempts []ProviderAttempt
	// StartTime is when routing began.
	StartTime time.Time
	// EndTime is when routing completed (zero if still in progress).
	EndTime time.Time
	// FinalError is the error returned if all providers fail.
	FinalError error
	// Metadata holds additional routing metadata.
	Metadata map[string]string
}

// ProviderAttempt records a single attempt to route a request to a provider.
type ProviderAttempt struct {
	// Provider is the name of the provider attempted.
	Provider string `json:"provider"`
	// AttemptNumber is the 1-based attempt number.
	AttemptNumber int `json:"attempt_number"`
	// StartTime is when the attempt began.
	StartTime time.Time `json:"start_time"`
	// EndTime is when the attempt completed.
	EndTime time.Time `json:"end_time"`
	// Duration is the elapsed time for this attempt.
	Duration time.Duration `json:"duration"`
	// Error is the error encountered, if any.
	Error error `json:"-"`
	// ErrorMessage is the string representation of the error.
	ErrorMessage string `json:"error_message,omitempty"`
	// StatusCode is the HTTP status code returned by the provider (0 if N/A).
	StatusCode int `json:"status_code,omitempty"`
	// IsFallback indicates whether this was a fallback attempt.
	IsFallback bool `json:"is_fallback"`
}

// NewRoutingContext creates a new RoutingContext for the given request and decision.
func NewRoutingContext(req *request.NormalizedRequest, decision *policy.PolicyDecision) *RoutingContext {
	requestID := ""
	if req != nil {
		requestID = req.ID
	}
	return &RoutingContext{
		RequestID:        requestID,
		Request:          req,
		Decision:         decision,
		SelectedProvider: decision.SelectedProvider,
		Attempts:         make([]ProviderAttempt, 0, 4),
		StartTime:        time.Now(),
		Metadata:         make(map[string]string),
	}
}

// RecordAttempt records a provider attempt.
func (rc *RoutingContext) RecordAttempt(attempt ProviderAttempt) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.Attempts = append(rc.Attempts, attempt)
}

// AttemptCount returns the number of attempts made.
func (rc *RoutingContext) AttemptCount() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.Attempts)
}

// LastAttempt returns the most recent attempt, or nil if none.
func (rc *RoutingContext) LastAttempt() *ProviderAttempt {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if len(rc.Attempts) == 0 {
		return nil
	}
	a := rc.Attempts[len(rc.Attempts)-1]
	return &a
}

// AllErrors returns all errors from all attempts.
func (rc *RoutingContext) AllErrors() []error {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	var errs []error
	for _, a := range rc.Attempts {
		if a.Error != nil {
			errs = append(errs, a.Error)
		}
	}
	return errs
}

// Finish marks the routing context as complete.
func (rc *RoutingContext) Finish(err error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.EndTime = time.Now()
	rc.FinalError = err
}

// TotalDuration returns the total elapsed time for routing.
func (rc *RoutingContext) TotalDuration() time.Duration {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	end := rc.EndTime
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(rc.StartTime)
}

// SetMetadata sets a metadata key-value pair on the routing context.
func (rc *RoutingContext) SetMetadata(key, value string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.Metadata[key] = value
}

// GetMetadata retrieves a metadata value by key.
func (rc *RoutingContext) GetMetadata(key string) (string, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	v, ok := rc.Metadata[key]
	return v, ok
}

// Summary returns a human-readable summary of the routing context.
func (rc *RoutingContext) Summary() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	var b strings.Builder
	fmt.Fprintf(&b, "request=%s provider=%s attempts=%d duration=%s",
		rc.RequestID, rc.SelectedProvider, len(rc.Attempts), rc.TotalDuration().Round(time.Millisecond))
	if rc.FinalError != nil {
		fmt.Fprintf(&b, " error=%v", rc.FinalError)
	}
	return b.String()
}

// WithRoutingContext attaches a RoutingContext to the given context.
func WithRoutingContext(ctx context.Context, rc *RoutingContext) context.Context {
	return context.WithValue(ctx, routingContextKey, rc)
}

// RoutingContextFromContext extracts the RoutingContext from a context, or nil.
func RoutingContextFromContext(ctx context.Context) *RoutingContext {
	rc, _ := ctx.Value(routingContextKey).(*RoutingContext)
	return rc
}
