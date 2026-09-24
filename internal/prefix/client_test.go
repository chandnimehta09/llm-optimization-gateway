package prefix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

func successHandler(t *testing.T, cachedTokens int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t.Helper()

		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		resp := ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []ChatCompletionChoice{
				{
					Index:        0,
					Message:      ChatMessage{Role: "assistant", Content: "Hello!"},
					FinishReason: "stop",
				},
			},
			Usage: UsageInfo{
				PromptTokens:     100,
				CompletionTokens: 10,
				TotalTokens:      110,
			},
		}

		if cachedTokens > 0 {
			resp.Usage.PromptTokensDetails = &PromptTokensDetails{
				CachedTokens: cachedTokens,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func errorHandler(statusCode int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(body))
	}
}

func TestNewClient_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}
	if client.baseURL != cfg.BaseURL {
		t.Errorf("client.baseURL = %q, want %q", client.baseURL, cfg.BaseURL)
	}
}

func TestNewClient_NilConfig(t *testing.T) {
	_, err := NewClient(nil)
	if err == nil {
		t.Error("NewClient(nil) = nil error, want error")
	}
}

func TestNewClient_InvalidConfig(t *testing.T) {
	cfg := &Config{BaseURL: ""}
	_, err := NewClient(cfg)
	if err == nil {
		t.Error("NewClient() = nil error, want error for empty base_url")
	}
}

func TestNewClient_WithAPIKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.APIKey = "test-key-123"
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client.apiKey != "test-key-123" {
		t.Errorf("client.apiKey = %q, want %q", client.apiKey, "test-key-123")
	}
}

func TestNewClientWithHTTP(t *testing.T) {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	client := NewClientWithHTTP(httpClient, "http://example.com", "key", 3)
	if client.baseURL != "http://example.com" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "http://example.com")
	}
	if client.apiKey != "key" {
		t.Errorf("apiKey = %q, want %q", client.apiKey, "key")
	}
	if client.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", client.maxRetries)
	}
}

func TestNewClientWithHTTP_ZeroRetries(t *testing.T) {
	httpClient := &http.Client{}
	client := NewClientWithHTTP(httpClient, "http://example.com", "", 0)
	if client.maxRetries != 2 {
		t.Errorf("maxRetries = %d, want 2 (default)", client.maxRetries)
	}
}

func TestClient_ChatCompletion_Success(t *testing.T) {
	ts := newTestServer(successHandler(t, 80))
	defer ts.Close()

	client := NewClientWithHTTP(ts.Client(), ts.URL, "", 1)

	req := &ChatCompletionRequest{
		Model: "test-model",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	resp, err := client.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if resp.Model != "test-model" {
		t.Errorf("resp.Model = %q, want %q", resp.Model, "test-model")
	}
	if resp.Usage.PromptTokens != 100 {
		t.Errorf("resp.Usage.PromptTokens = %d, want 100", resp.Usage.PromptTokens)
	}
	if resp.CachedTokens() != 80 {
		t.Errorf("resp.CachedTokens() = %d, want 80", resp.CachedTokens())
	}
	if got := resp.PrefixCacheHitRate(); got != 0.8 {
		t.Errorf("resp.PrefixCacheHitRate() = %f, want 0.8", got)
	}
}

func TestClient_ChatCompletion_NilRequest(t *testing.T) {
	client := NewClientWithHTTP(&http.Client{}, "http://localhost", "", 1)

	_, err := client.ChatCompletion(context.Background(), nil)
	if err == nil {
		t.Error("ChatCompletion(nil) = nil error, want error")
	}
}

func TestClient_ChatCompletion_AuthHeader(t *testing.T) {
	var gotAuth string
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatCompletionResponse{
			ID:    "test",
			Model: "m",
			Usage: UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	})
	defer ts.Close()

	client := NewClientWithHTTP(ts.Client(), ts.URL, "my-secret-key", 0)
	req := &ChatCompletionRequest{
		Model:    "test",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}
	_, err := client.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if gotAuth != "Bearer my-secret-key" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer my-secret-key")
	}
}

func TestClient_ChatCompletion_ServerError(t *testing.T) {
	ts := newTestServer(errorHandler(500, `{"error":"internal server error"}`))
	defer ts.Close()

	client := NewClientWithHTTP(ts.Client(), ts.URL, "", 0)
	req := &ChatCompletionRequest{
		Model:    "test",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}

	_, err := client.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("ChatCompletion() = nil error, want error for 500")
	}

	var apiErr *APIError
	if !isAPIErrorType(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("APIError.StatusCode = %d, want 500", apiErr.StatusCode)
	}
}

func TestClient_ChatCompletion_RetryOnServerError(t *testing.T) {
	var callCount atomic.Int32
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		count := callCount.Add(1)
		if count <= 2 {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":"temporary"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatCompletionResponse{
			ID:    "test",
			Model: "m",
			Usage: UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	})
	defer ts.Close()

	client := NewClientWithHTTP(ts.Client(), ts.URL, "", 3)
	req := &ChatCompletionRequest{
		Model:    "test",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}

	resp, err := client.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v (after retries)", err)
	}
	if resp == nil {
		t.Fatal("ChatCompletion() returned nil response")
	}
	if callCount.Load() != 3 {
		t.Errorf("server called %d times, want 3", callCount.Load())
	}
}

func TestClient_ChatCompletion_ClientError_NoRetry(t *testing.T) {
	var callCount atomic.Int32
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"bad request"}`))
	})
	defer ts.Close()

	client := NewClientWithHTTP(ts.Client(), ts.URL, "", 3)
	req := &ChatCompletionRequest{
		Model:    "test",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}

	_, err := client.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("ChatCompletion() = nil error, want error for 400")
	}
	if callCount.Load() != 1 {
		t.Errorf("server called %d times, want 1 (no retry for 400)", callCount.Load())
	}
}

func TestClient_ChatCompletion_ContextCanceled(t *testing.T) {
	ts := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	})
	defer ts.Close()

	client := NewClientWithHTTP(ts.Client(), ts.URL, "", 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := &ChatCompletionRequest{
		Model:    "test",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}

	_, err := client.ChatCompletion(ctx, req)
	if err == nil {
		t.Fatal("ChatCompletion() = nil error, want error for cancelled context")
	}
}

func TestClient_ChatCompletion_NoCachedTokens(t *testing.T) {
	ts := newTestServer(successHandler(t, 0))
	defer ts.Close()

	client := NewClientWithHTTP(ts.Client(), ts.URL, "", 1)
	req := &ChatCompletionRequest{
		Model:    "test",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}

	resp, err := client.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}
	if resp.CachedTokens() != 0 {
		t.Errorf("CachedTokens() = %d, want 0", resp.CachedTokens())
	}
	if resp.PrefixCacheHitRate() != 0 {
		t.Errorf("PrefixCacheHitRate() = %f, want 0", resp.PrefixCacheHitRate())
	}
}

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 429, Body: "rate limited"}
	got := err.Error()
	if !strings.Contains(got, "429") {
		t.Errorf("Error() = %q, want to contain '429'", got)
	}
	if !strings.Contains(got, "rate limited") {
		t.Errorf("Error() = %q, want to contain 'rate limited'", got)
	}
}

func TestAPIError_IsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"429 rate limit", 429, true},
		{"500 server error", 500, true},
		{"502 bad gateway", 502, true},
		{"503 unavailable", 503, true},
		{"400 bad request", 400, false},
		{"401 unauthorized", 401, false},
		{"403 forbidden", 403, false},
		{"404 not found", 404, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &APIError{StatusCode: tt.statusCode}
			if got := err.IsRetryable(); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChatCompletionResponse_CachedTokens_NilDetails(t *testing.T) {
	resp := &ChatCompletionResponse{
		Usage: UsageInfo{PromptTokens: 100},
	}
	if got := resp.CachedTokens(); got != 0 {
		t.Errorf("CachedTokens() = %d, want 0 for nil details", got)
	}
}

func TestChatCompletionResponse_PrefixCacheHitRate_ZeroPromptTokens(t *testing.T) {
	resp := &ChatCompletionResponse{
		Usage: UsageInfo{PromptTokens: 0},
	}
	if got := resp.PrefixCacheHitRate(); got != 0 {
		t.Errorf("PrefixCacheHitRate() = %f, want 0 for zero prompt tokens", got)
	}
}

// isAPIErrorType checks if err is an *APIError.
func isAPIErrorType(err error, target **APIError) bool {
	for err != nil {
		if ae, ok := err.(*APIError); ok {
			*target = ae
			return true
		}
		// unwrap
		if u, ok := err.(interface{ Unwrap() error }); ok {
			err = u.Unwrap()
		} else {
			break
		}
	}
	return false
}
