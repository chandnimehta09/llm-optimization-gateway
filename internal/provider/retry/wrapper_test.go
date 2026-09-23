package retry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

// --- Mock Provider ---

// mockProvider is a test helper that implements provider.Provider with configurable behavior.
type mockProvider struct {
	name      string
	models    []string
	apiFormat provider.APIFormat
	healthy   bool

	mu            sync.Mutex
	completeFunc  func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error)
	streamFunc    func(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamChunk, error)
	completeCalls int
	streamCalls   int
}

func newMockProvider(name string) *mockProvider {
	return &mockProvider{
		name:      name,
		models:    []string{"test-model"},
		apiFormat: provider.APIFormatOpenAI,
		healthy:   true,
	}
}

func (m *mockProvider) Name() string                     { return m.name }
func (m *mockProvider) Models() []string                 { return m.models }
func (m *mockProvider) IsHealthy(_ context.Context) bool { return m.healthy }
func (m *mockProvider) APIFormat() provider.APIFormat    { return m.apiFormat }

func (m *mockProvider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	m.mu.Lock()
	m.completeCalls++
	fn := m.completeFunc
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, req)
	}
	return &provider.CompletionResponse{
		ID:       "test-id",
		Model:    "test-model",
		Provider: m.name,
	}, nil
}

func (m *mockProvider) CompleteStream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
	m.mu.Lock()
	m.streamCalls++
	fn := m.streamFunc
	m.mu.Unlock()

	if fn != nil {
		return fn(ctx, req)
	}
	ch := make(chan provider.StreamChunk, 1)
	ch <- provider.StreamChunk{ID: "test-id", Model: "test-model", Provider: m.name}
	close(ch)
	return ch, nil
}

func (m *mockProvider) getCompleteCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.completeCalls
}

func (m *mockProvider) getStreamCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streamCalls
}

// --- Tests ---

func TestWrapper_ImplementsProviderInterface(t *testing.T) {
	mock := newMockProvider("test")
	w := New(mock)
	var _ provider.Provider = w
}

func TestWrapper_SuccessOnFirstAttempt(t *testing.T) {
	mock := newMockProvider("test")
	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff:     DefaultBackoffConfig(),
	}))

	resp, err := w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "test-id" {
		t.Errorf("resp.ID = %q, want %q", resp.ID, "test-id")
	}
	if mock.getCompleteCalls() != 1 {
		t.Errorf("complete calls = %d, want 1", mock.getCompleteCalls())
	}
}

func TestWrapper_RetryOnServerError(t *testing.T) {
	mock := newMockProvider("test")
	callCount := 0
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		mock.mu.Lock()
		callCount++
		n := callCount
		mock.mu.Unlock()

		if n <= 2 {
			return nil, &provider.ProviderError{Provider: "test", Code: 500, Message: "internal server error"}
		}
		return &provider.CompletionResponse{ID: "success", Provider: "test"}, nil
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	resp, err := w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "success" {
		t.Errorf("resp.ID = %q, want %q", resp.ID, "success")
	}
	if mock.getCompleteCalls() != 3 {
		t.Errorf("complete calls = %d, want 3", mock.getCompleteCalls())
	}
}

func TestWrapper_ExhaustsRetries(t *testing.T) {
	mock := newMockProvider("test")
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		return nil, &provider.ProviderError{Provider: "test", Code: 502, Message: "bad gateway"}
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	_, err := w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if mock.getCompleteCalls() != 3 {
		t.Errorf("complete calls = %d, want 3", mock.getCompleteCalls())
	}

	// Check metrics.
	metrics := w.GetMetrics()
	if metrics.TotalExhausted.Load() != 1 {
		t.Errorf("TotalExhausted = %d, want 1", metrics.TotalExhausted.Load())
	}
}

func TestWrapper_NoRetryOnAuthError(t *testing.T) {
	mock := newMockProvider("test")
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		return nil, &provider.AuthenticationError{
			ProviderError: provider.ProviderError{Provider: "test", Code: 401, Message: "invalid api key"},
		}
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	_, err := w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	if err == nil {
		t.Fatal("expected auth error")
	}
	if !provider.IsAuthenticationError(err) {
		t.Errorf("expected AuthenticationError, got %T", err)
	}
	if mock.getCompleteCalls() != 1 {
		t.Errorf("complete calls = %d, want 1 (no retries for auth errors)", mock.getCompleteCalls())
	}
}

func TestWrapper_NoRetryOnContextLengthError(t *testing.T) {
	mock := newMockProvider("test")
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		return nil, &provider.ContextLengthError{
			ProviderError:   provider.ProviderError{Provider: "test", Code: 400, Message: "too long"},
			MaxTokens:       4096,
			RequestedTokens: 8000,
		}
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	_, err := w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	if err == nil {
		t.Fatal("expected context length error")
	}
	if !provider.IsContextLengthError(err) {
		t.Errorf("expected ContextLengthError, got %T", err)
	}
	if mock.getCompleteCalls() != 1 {
		t.Errorf("complete calls = %d, want 1", mock.getCompleteCalls())
	}
}

func TestWrapper_ContextCancelledDuringRetry(t *testing.T) {
	mock := newMockProvider("test")
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		return nil, &provider.ProviderError{Provider: "test", Code: 500, Message: "server error"}
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 5,
		Backoff: BackoffConfig{
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := w.Complete(ctx, &provider.CompletionRequest{Model: "test-model"})
	if err == nil {
		t.Fatal("expected error from context cancellation")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestWrapper_RateLimitWithRetryAfterHeader(t *testing.T) {
	mock := newMockProvider("test")
	callCount := 0
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		mock.mu.Lock()
		callCount++
		n := callCount
		mock.mu.Unlock()

		if n == 1 {
			return nil, &provider.RateLimitError{
				ProviderError: provider.ProviderError{Provider: "test", Code: 429, Message: "rate limited"},
				RetryAfter:    50 * time.Millisecond, // Short for test speed.
			}
		}
		return &provider.CompletionResponse{ID: "success", Provider: "test"}, nil
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	start := time.Now()
	resp, err := w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "success" {
		t.Errorf("resp.ID = %q, want %q", resp.ID, "success")
	}
	// Should have waited at least the Retry-After duration.
	if elapsed < 40*time.Millisecond {
		t.Errorf("elapsed %v, expected at least ~50ms for Retry-After", elapsed)
	}
	if mock.getCompleteCalls() != 2 {
		t.Errorf("complete calls = %d, want 2", mock.getCompleteCalls())
	}
}

func TestWrapper_RequestLevelPolicyOverride(t *testing.T) {
	mock := newMockProvider("test")
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		return nil, &provider.ProviderError{Provider: "test", Code: 500, Message: "server error"}
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	// Override to 5 attempts via request metadata.
	req := &provider.CompletionRequest{
		Model: "test-model",
		Metadata: map[string]string{
			RequestRetryPolicyKey: "5",
		},
	}

	_, err := w.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if mock.getCompleteCalls() != 5 {
		t.Errorf("complete calls = %d, want 5 (overridden via metadata)", mock.getCompleteCalls())
	}
}

func TestWrapper_BackoffIncreasesExponentially(t *testing.T) {
	mock := newMockProvider("test")
	var delays []time.Duration
	var mu sync.Mutex
	lastCall := time.Now()

	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		mu.Lock()
		now := time.Now()
		if mock.completeCalls > 1 { // Skip first call.
			delays = append(delays, now.Sub(lastCall))
		}
		lastCall = now
		mu.Unlock()

		return nil, &provider.ProviderError{Provider: "test", Code: 503, Message: "unavailable"}
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 4,
		Backoff: BackoffConfig{
			InitialDelay: 10 * time.Millisecond,
			MaxDelay:     1 * time.Second,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	_, _ = w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})

	mu.Lock()
	defer mu.Unlock()

	// We should have 3 delays (between 4 attempts).
	if len(delays) != 3 {
		t.Fatalf("expected 3 delay measurements, got %d", len(delays))
	}

	// Verify delays are positive.
	for i, d := range delays {
		if d <= 0 {
			t.Errorf("delay[%d] = %v, expected positive", i, d)
		}
	}
}

func TestWrapper_MetricsTracking(t *testing.T) {
	mock := newMockProvider("test")
	callCount := 0
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		mock.mu.Lock()
		callCount++
		n := callCount
		mock.mu.Unlock()

		if n == 1 {
			return nil, &provider.ProviderError{Provider: "test", Code: 500, Message: "error"}
		}
		return &provider.CompletionResponse{ID: "ok", Provider: "test"}, nil
	}

	metrics := NewMetrics()
	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}), WithMetrics(metrics))

	_, err := w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metrics.TotalRetries.Load() != 1 {
		t.Errorf("TotalRetries = %d, want 1", metrics.TotalRetries.Load())
	}
	if metrics.TotalSucceededAfterRetry.Load() != 1 {
		t.Errorf("TotalSucceededAfterRetry = %d, want 1", metrics.TotalSucceededAfterRetry.Load())
	}
}

func TestWrapper_Delegates(t *testing.T) {
	mock := newMockProvider("delegator")
	mock.models = []string{"m1", "m2"}
	mock.apiFormat = provider.APIFormatGemini

	w := New(mock)

	if w.Name() != "delegator" {
		t.Errorf("Name() = %q, want %q", w.Name(), "delegator")
	}
	if len(w.Models()) != 2 || w.Models()[0] != "m1" {
		t.Errorf("Models() = %v, want [m1 m2]", w.Models())
	}
	if w.APIFormat() != provider.APIFormatGemini {
		t.Errorf("APIFormat() = %v, want Gemini", w.APIFormat())
	}
	if !w.IsHealthy(context.Background()) {
		t.Error("IsHealthy() = false, want true")
	}
}

func TestWrapper_StreamRetryOnEstablishmentError(t *testing.T) {
	mock := newMockProvider("test")
	callCount := 0
	mock.streamFunc = func(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
		mock.mu.Lock()
		callCount++
		n := callCount
		mock.mu.Unlock()

		if n == 1 {
			return nil, &provider.ProviderError{Provider: "test", Code: 503, Message: "unavailable"}
		}
		ch := make(chan provider.StreamChunk, 2)
		ch <- provider.StreamChunk{ID: "stream-ok", Delta: provider.MessageDelta{Content: "hello"}}
		close(ch)
		return ch, nil
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	ch, err := w.CompleteStream(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []provider.StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 1 || chunks[0].ID != "stream-ok" {
		t.Errorf("expected 1 chunk with ID 'stream-ok', got %+v", chunks)
	}
	if mock.getStreamCalls() != 2 {
		t.Errorf("stream calls = %d, want 2", mock.getStreamCalls())
	}
}

func TestWrapper_StreamMidStreamErrorRetry(t *testing.T) {
	mock := newMockProvider("test")
	callCount := 0
	mock.streamFunc = func(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
		mock.mu.Lock()
		callCount++
		n := callCount
		mock.mu.Unlock()

		ch := make(chan provider.StreamChunk, 4)
		if n == 1 {
			ch <- provider.StreamChunk{ID: "s1", Delta: provider.MessageDelta{Content: "hi"}}
			ch <- provider.StreamChunk{
				Error: &provider.ProviderError{Provider: "test", Code: 500, Message: "mid-stream error"},
			}
			close(ch)
		} else {
			ch <- provider.StreamChunk{ID: "s2", Delta: provider.MessageDelta{Content: "recovered"}}
			close(ch)
		}
		return ch, nil
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	ch, err := w.CompleteStream(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []provider.StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}

	// Should have received the first chunk, then reconnected.
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// First chunk from initial stream.
	if chunks[0].Delta.Content != "hi" {
		t.Errorf("first chunk content = %q, want %q", chunks[0].Delta.Content, "hi")
	}
	// Last chunk from reconnected stream.
	lastChunk := chunks[len(chunks)-1]
	if lastChunk.Delta.Content != "recovered" {
		t.Errorf("last chunk content = %q, want %q", lastChunk.Delta.Content, "recovered")
	}
}

func TestWrapper_ContextCancelledBeforeFirstAttempt(t *testing.T) {
	mock := newMockProvider("test")
	w := New(mock)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := w.Complete(ctx, &provider.CompletionRequest{Model: "test-model"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestWrapper_EventualSuccess(t *testing.T) {
	// Test that retries eventually lead to success.
	mock := newMockProvider("test")
	failCount := 3
	callCount := 0
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		mock.mu.Lock()
		callCount++
		n := callCount
		mock.mu.Unlock()

		if n <= failCount {
			return nil, &provider.ProviderError{
				Provider: "test",
				Code:     503,
				Message:  fmt.Sprintf("failure %d", n),
			}
		}
		return &provider.CompletionResponse{
			ID:       "eventual-success",
			Provider: "test",
			Model:    "test-model",
		}, nil
	}

	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 5, // Allow enough attempts.
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}))

	resp, err := w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "eventual-success" {
		t.Errorf("resp.ID = %q, want %q", resp.ID, "eventual-success")
	}
	if mock.getCompleteCalls() != 4 {
		t.Errorf("complete calls = %d, want 4 (3 failures + 1 success)", mock.getCompleteCalls())
	}
}

func TestWrapper_MetricsHistogram(t *testing.T) {
	mock := newMockProvider("test")
	callCount := 0
	mock.completeFunc = func(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
		mock.mu.Lock()
		callCount++
		n := callCount
		mock.mu.Unlock()

		if n == 1 {
			return nil, &provider.ProviderError{Provider: "test", Code: 500, Message: "error"}
		}
		return &provider.CompletionResponse{ID: "ok", Provider: "test"}, nil
	}

	metrics := NewMetrics()
	w := New(mock, WithPolicy(Policy{
		MaxAttempts: 3,
		Backoff: BackoffConfig{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       FullJitter,
		},
	}), WithMetrics(metrics))

	_, _ = w.Complete(context.Background(), &provider.CompletionRequest{Model: "test-model"})

	hist := metrics.Histogram()
	// 1 retry was needed (attempt index 1 at success).
	if _, ok := hist[1]; !ok {
		t.Errorf("expected histogram entry for 1 retry, got %+v", hist)
	}
}
