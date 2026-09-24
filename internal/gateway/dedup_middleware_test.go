package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/llm-optimization-gateway/internal/dedup"
	"github.com/llm-optimization-gateway/internal/request"
)

// mockDedupHandler records whether it was called and captures the request context.
type mockDedupHandler struct {
	called  bool
	request *http.Request
}

func (m *mockDedupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.called = true
	m.request = r
	w.WriteHeader(http.StatusOK)
}

func TestDedupMiddleware_RemovesDuplicates(t *testing.T) {
	config := dedup.DefaultConfig()
	inner := &mockDedupHandler{}
	mw := NewDedupMiddleware(inner, config)

	nr := &request.NormalizedRequest{
		ID:    "test-1",
		Model: "gpt-4",
		Messages: []request.Message{
			{Role: "user", Content: "What is Go?"},
			{Role: "user", Content: "What is Go?"},
			{Role: "user", Content: "What is Rust?"},
		},
	}

	ctx := contextWithNormalizedRequest(context.Background(), nr)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	if !inner.called {
		t.Fatal("inner handler was not called")
	}

	// Check headers.
	if w.Header().Get("X-Dedup-Items-Removed") != "1" {
		t.Errorf("expected X-Dedup-Items-Removed=1, got %q", w.Header().Get("X-Dedup-Items-Removed"))
	}
	if w.Header().Get("X-Dedup-Strategy") != "exact" {
		t.Errorf("expected X-Dedup-Strategy=exact, got %q", w.Header().Get("X-Dedup-Strategy"))
	}

	// Verify the normalized request was updated.
	updatedNR := NormalizedRequestFromContext(inner.request.Context())
	if updatedNR == nil {
		t.Fatal("expected normalized request in context")
	}
	if len(updatedNR.Messages) != 2 {
		t.Fatalf("expected 2 messages after dedup, got %d", len(updatedNR.Messages))
	}

	// Verify dedup result in context.
	dr := DedupResultFromContext(inner.request.Context())
	if dr == nil {
		t.Fatal("expected dedup result in context")
	}
	if dr.ItemsRemoved != 1 {
		t.Errorf("expected 1 item removed, got %d", dr.ItemsRemoved)
	}
}

func TestDedupMiddleware_NoDuplicates(t *testing.T) {
	config := dedup.DefaultConfig()
	inner := &mockDedupHandler{}
	mw := NewDedupMiddleware(inner, config)

	nr := &request.NormalizedRequest{
		ID:    "test-2",
		Model: "gpt-4",
		Messages: []request.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
	}

	ctx := contextWithNormalizedRequest(context.Background(), nr)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	if !inner.called {
		t.Fatal("inner handler was not called")
	}

	// No duplicates → no dedup headers.
	if w.Header().Get("X-Dedup-Items-Removed") != "" {
		t.Errorf("expected no X-Dedup-Items-Removed header, got %q", w.Header().Get("X-Dedup-Items-Removed"))
	}
}

func TestDedupMiddleware_Disabled(t *testing.T) {
	config := dedup.Config{Enabled: false}
	inner := &mockDedupHandler{}
	mw := NewDedupMiddleware(inner, config)

	nr := &request.NormalizedRequest{
		ID:    "test-3",
		Model: "gpt-4",
		Messages: []request.Message{
			{Role: "user", Content: "dup"},
			{Role: "user", Content: "dup"},
		},
	}

	ctx := contextWithNormalizedRequest(context.Background(), nr)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	if !inner.called {
		t.Fatal("inner handler was not called")
	}

	// Disabled → no dedup headers.
	if w.Header().Get("X-Dedup-Items-Removed") != "" {
		t.Errorf("expected no X-Dedup-Items-Removed header when disabled")
	}
}

func TestDedupMiddleware_NoNormalizedRequest(t *testing.T) {
	config := dedup.DefaultConfig()
	inner := &mockDedupHandler{}
	mw := NewDedupMiddleware(inner, config)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	if !inner.called {
		t.Fatal("inner handler was not called when no normalized request in context")
	}
}

func TestDedupMiddleware_MetadataAdded(t *testing.T) {
	config := dedup.DefaultConfig()
	inner := &mockDedupHandler{}
	mw := NewDedupMiddleware(inner, config)

	nr := &request.NormalizedRequest{
		ID:    "test-meta",
		Model: "gpt-4",
		Messages: []request.Message{
			{Role: "user", Content: "test"},
			{Role: "user", Content: "test"},
		},
	}

	ctx := contextWithNormalizedRequest(context.Background(), nr)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	updatedNR := NormalizedRequestFromContext(inner.request.Context())
	if updatedNR == nil {
		t.Fatal("expected normalized request in context")
	}
	if updatedNR.Metadata == nil {
		t.Fatal("expected metadata to be set")
	}
	if updatedNR.Metadata["dedup_strategy"] != "exact" {
		t.Errorf("expected dedup_strategy=exact in metadata, got %q", updatedNR.Metadata["dedup_strategy"])
	}
	if updatedNR.Metadata["dedup_items_removed"] != "1" {
		t.Errorf("expected dedup_items_removed=1 in metadata, got %q", updatedNR.Metadata["dedup_items_removed"])
	}
}

func TestDedupProcessor_DisabledPassthrough(t *testing.T) {
	config := dedup.Config{Enabled: false}
	p := NewDedupProcessor(config)

	messages := []request.Message{
		{Role: "user", Content: "dup"},
		{Role: "user", Content: "dup"},
	}

	result := p.ProcessMessages(messages)

	if len(result.Messages) != 2 {
		t.Errorf("expected 2 messages (disabled), got %d", len(result.Messages))
	}
	if result.ItemsRemoved != 0 {
		t.Errorf("expected 0 items removed, got %d", result.ItemsRemoved)
	}
}

func TestDedupProcessor_EmptyMessages(t *testing.T) {
	config := dedup.DefaultConfig()
	p := NewDedupProcessor(config)

	result := p.ProcessMessages(nil)

	if result.Messages != nil {
		t.Errorf("expected nil messages, got %v", result.Messages)
	}
	if result.ItemsRemoved != 0 {
		t.Errorf("expected 0 items removed, got %d", result.ItemsRemoved)
	}
}

func TestDedupMiddleware_DuplicateDocuments_EndToEnd(t *testing.T) {
	// Integration test: Given request with duplicate documents,
	// when processed through gateway, deduplication removes duplicates
	// and X-Dedup-Items-Removed header shows count.
	config := dedup.DefaultConfig()

	var capturedMessages []request.Message
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nr := NormalizedRequestFromContext(r.Context())
		if nr != nil {
			capturedMessages = nr.Messages
		}
		w.WriteHeader(http.StatusOK)
	})

	mw := NewDedupMiddleware(inner, config)

	nr := &request.NormalizedRequest{
		ID:    "integration-test",
		Model: "gpt-4",
		Messages: []request.Message{
			{Role: "system", Content: "You are a coding assistant."},
			{Role: "user", Content: "Here is a document about Go."},
			{Role: "user", Content: "Here is a document about Go."}, // duplicate
			{Role: "user", Content: "Here is a document about Rust."},
			{Role: "user", Content: "Here is a document about Rust."}, // duplicate
		},
	}

	ctx := contextWithNormalizedRequest(context.Background(), nr)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	mw.ServeHTTP(w, req)

	// Provider receives deduplicated content.
	if len(capturedMessages) != 3 {
		t.Fatalf("expected 3 messages (deduplicated), got %d", len(capturedMessages))
	}

	// Verify header counts.
	if w.Header().Get("X-Dedup-Items-Removed") != "2" {
		t.Errorf("expected X-Dedup-Items-Removed=2, got %q", w.Header().Get("X-Dedup-Items-Removed"))
	}
}

func TestContextHelpers_RoundTrip(t *testing.T) {
	nr := &request.NormalizedRequest{
		ID:    "ctx-test",
		Model: "gpt-4",
	}
	dr := &DedupResult{
		ItemsRemoved: 5,
	}

	ctx := context.Background()
	ctx = contextWithNormalizedRequest(ctx, nr)
	ctx = contextWithDedupResult(ctx, dr)

	gotNR := NormalizedRequestFromContext(ctx)
	if gotNR == nil || gotNR.ID != "ctx-test" {
		t.Errorf("expected normalized request with ID 'ctx-test', got %v", gotNR)
	}

	gotDR := DedupResultFromContext(ctx)
	if gotDR == nil || gotDR.ItemsRemoved != 5 {
		t.Errorf("expected dedup result with 5 items removed, got %v", gotDR)
	}
}

func TestContextHelpers_MissingValues(t *testing.T) {
	ctx := context.Background()

	if NormalizedRequestFromContext(ctx) != nil {
		t.Error("expected nil normalized request from empty context")
	}
	if DedupResultFromContext(ctx) != nil {
		t.Error("expected nil dedup result from empty context")
	}
}
