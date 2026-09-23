package retry

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

// Compile-time interface check.
var _ provider.Provider = (*Wrapper)(nil)

// MetricsKey is used for request-level retry override in metadata.
const MetricsKey = "retry_count"

// RequestRetryPolicyKey is the metadata key for per-request retry overrides.
const RequestRetryPolicyKey = "retry_max_attempts"

// Metrics tracks retry statistics.
type Metrics struct {
	// TotalRetries is the total number of retry attempts across all requests.
	TotalRetries atomic.Int64
	// TotalExhausted is the number of requests that exhausted all retries.
	TotalExhausted atomic.Int64
	// TotalSucceededAfterRetry is the number of requests that succeeded after at least one retry.
	TotalSucceededAfterRetry atomic.Int64

	mu             sync.Mutex
	retryHistogram map[int]int64 // retryCount -> occurrences
}

// NewMetrics creates a new Metrics instance.
func NewMetrics() *Metrics {
	return &Metrics{
		retryHistogram: make(map[int]int64),
	}
}

// RecordRetries records the number of retries for a single request.
func (m *Metrics) RecordRetries(count int) {
	m.mu.Lock()
	m.retryHistogram[count]++
	m.mu.Unlock()
}

// Histogram returns a copy of the retry count histogram.
func (m *Metrics) Histogram() map[int]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[int]int64, len(m.retryHistogram))
	for k, v := range m.retryHistogram {
		result[k] = v
	}
	return result
}

// Wrapper decorates a provider.Provider with retry logic.
type Wrapper struct {
	inner      provider.Provider
	policy     Policy
	classifier *Classifier
	metrics    *Metrics
	logger     *log.Logger
}

// Option is a functional option for configuring the Wrapper.
type Option func(*Wrapper)

// WithPolicy sets the retry policy.
func WithPolicy(p Policy) Option {
	return func(w *Wrapper) {
		w.policy = p
	}
}

// WithLogger sets a custom logger.
func WithLogger(l *log.Logger) Option {
	return func(w *Wrapper) {
		w.logger = l
	}
}

// WithMetrics sets a custom metrics collector.
func WithMetrics(m *Metrics) Option {
	return func(w *Wrapper) {
		w.metrics = m
	}
}

// New creates a new retry Wrapper around the given provider.
func New(inner provider.Provider, opts ...Option) *Wrapper {
	w := &Wrapper{
		inner:      inner,
		policy:     DefaultPolicy(),
		classifier: NewClassifier(),
		metrics:    NewMetrics(),
		logger:     log.New(log.Writer(), fmt.Sprintf("[retry:%s] ", inner.Name()), log.LstdFlags),
	}

	for _, opt := range opts {
		opt(w)
	}

	w.policy.Validate()
	return w
}

// GetMetrics returns the wrapper's metrics collector.
func (w *Wrapper) GetMetrics() *Metrics {
	return w.metrics
}

// Name delegates to the inner provider.
func (w *Wrapper) Name() string {
	return w.inner.Name()
}

// Models delegates to the inner provider.
func (w *Wrapper) Models() []string {
	return w.inner.Models()
}

// IsHealthy delegates to the inner provider.
func (w *Wrapper) IsHealthy(ctx context.Context) bool {
	return w.inner.IsHealthy(ctx)
}

// APIFormat delegates to the inner provider.
func (w *Wrapper) APIFormat() provider.APIFormat {
	return w.inner.APIFormat()
}

// Complete sends a completion request with retry logic.
func (w *Wrapper) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	policy := w.effectivePolicy(req)
	backoff := NewBackoff(policy.Backoff)
	var lastErr error

	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		// Check context before each attempt.
		if ctx.Err() != nil {
			w.recordRetryMetrics(attempt, false)
			return nil, ctx.Err()
		}

		// Execute the request.
		resp, err := w.inner.Complete(ctx, req)
		if err == nil {
			// Success.
			if attempt > 0 {
				w.metrics.TotalSucceededAfterRetry.Add(1)
				w.logger.Printf("[INFO] Request succeeded after %d retries", attempt)
			}
			w.recordRetryMetrics(attempt, true)
			// Annotate response metadata with retry count.
			w.annotateResponse(resp, attempt)
			return resp, nil
		}

		lastErr = err

		// Evaluate whether to retry.
		decision := Evaluate(policy, w.classifier, backoff, attempt, err)
		if !decision.ShouldRetry {
			w.logger.Printf("[DEBUG] Not retrying: %s (attempt %d/%d)", decision.Reason, attempt+1, policy.MaxAttempts)
			if decision.Reason == "max attempts exhausted" {
				w.metrics.TotalExhausted.Add(1)
			}
			w.recordRetryMetrics(attempt+1, false)
			return nil, err
		}

		w.logger.Printf("[DEBUG] Retrying in %s: %s (attempt %d/%d)", decision.Delay, decision.Reason, attempt+1, policy.MaxAttempts)
		w.metrics.TotalRetries.Add(1)

		// Wait for backoff delay or context cancellation.
		if err := w.wait(ctx, decision.Delay); err != nil {
			w.recordRetryMetrics(attempt+1, false)
			return nil, err
		}
	}

	// All attempts exhausted (fallback path).
	w.metrics.TotalExhausted.Add(1)
	w.recordRetryMetrics(policy.MaxAttempts, false)
	return nil, fmt.Errorf("retry exhausted after %d attempts: %w", policy.MaxAttempts, lastErr)
}

// CompleteStream sends a streaming request with retry logic.
// Retries are applied to the initial stream establishment. If the stream is
// established successfully but fails mid-stream, the wrapper will attempt to
// reconnect by re-issuing the request.
func (w *Wrapper) CompleteStream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
	policy := w.effectivePolicy(req)
	backoff := NewBackoff(policy.Backoff)
	var lastErr error

	for attempt := 0; attempt < policy.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		ch, err := w.inner.CompleteStream(ctx, req)
		if err == nil {
			// Wrap the stream to handle mid-stream errors with reconnection.
			wrappedCh := w.wrapStream(ctx, req, ch, policy, attempt)
			return wrappedCh, nil
		}

		lastErr = err

		decision := Evaluate(policy, w.classifier, backoff, attempt, err)
		if !decision.ShouldRetry {
			return nil, err
		}

		w.logger.Printf("[DEBUG] Stream retry in %s: %s (attempt %d/%d)", decision.Delay, decision.Reason, attempt+1, policy.MaxAttempts)
		w.metrics.TotalRetries.Add(1)

		if err := w.wait(ctx, decision.Delay); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("stream retry exhausted after %d attempts: %w", policy.MaxAttempts, lastErr)
}

// wrapStream wraps a stream channel to detect mid-stream errors and attempt reconnection.
func (w *Wrapper) wrapStream(ctx context.Context, req *provider.CompletionRequest, ch <-chan provider.StreamChunk, policy Policy, startAttempt int) <-chan provider.StreamChunk {
	out := make(chan provider.StreamChunk, 64)

	go func() {
		defer close(out)

		currentCh := ch
		attempt := startAttempt
		backoff := NewBackoff(policy.Backoff)

	outer:
		for {
			for chunk := range currentCh {
				if chunk.Error != nil {
					// Mid-stream error — try to reconnect.
					decision := Evaluate(policy, w.classifier, backoff, attempt, chunk.Error)
					if !decision.ShouldRetry || attempt >= policy.MaxAttempts-1 {
						// Cannot retry, forward the error.
						select {
						case out <- chunk:
						case <-ctx.Done():
						}
						return
					}

					w.logger.Printf("[DEBUG] Stream reconnect in %s (attempt %d/%d)", decision.Delay, attempt+1, policy.MaxAttempts)
					w.metrics.TotalRetries.Add(1)

					if err := w.wait(ctx, decision.Delay); err != nil {
						select {
						case out <- provider.StreamChunk{Error: err}:
						case <-ctx.Done():
						}
						return
					}

					attempt++
					newCh, err := w.inner.CompleteStream(ctx, req)
					if err != nil {
						select {
						case out <- provider.StreamChunk{Error: err}:
						case <-ctx.Done():
						}
						return
					}
					currentCh = newCh
					continue outer // Restart reading from the new channel.
				}

				// Forward successful chunks.
				select {
				case out <- chunk:
				case <-ctx.Done():
					return
				}
			}

			// Channel closed normally (no error chunk) — stream complete.
			return
		}
	}()

	return out
}

// wait sleeps for the given duration, returning early if the context is cancelled.
func (w *Wrapper) wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// effectivePolicy returns the retry policy, potentially overridden by request metadata.
func (w *Wrapper) effectivePolicy(req *provider.CompletionRequest) Policy {
	policy := w.policy

	if req != nil && req.Metadata != nil {
		if maxStr, ok := req.Metadata[RequestRetryPolicyKey]; ok {
			var max int
			if _, err := fmt.Sscanf(maxStr, "%d", &max); err == nil && max > 0 {
				policy.MaxAttempts = max
			}
		}
	}

	return policy
}

// annotateResponse adds retry count metadata to the response.
func (w *Wrapper) annotateResponse(resp *provider.CompletionResponse, retryCount int) {
	// CompletionResponse doesn't have a Metadata field, so we use the ID prefix
	// as a workaround. In a production system, we'd add a Metadata map to the response.
	// For now, we track this in the wrapper's metrics.
	_ = retryCount
}

// recordRetryMetrics records retry statistics for a request.
func (w *Wrapper) recordRetryMetrics(retries int, success bool) {
	w.metrics.RecordRetries(retries)
}
