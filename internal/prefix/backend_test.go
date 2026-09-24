package prefix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// --- test helpers ---

func newTestBackend(t *testing.T, handler http.HandlerFunc) (*Backend, *httptest.Server, *Telemetry) {
	t.Helper()
	ts := httptest.NewServer(handler)

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tel, err := NewTelemetry(provider)
	if err != nil {
		t.Fatalf("NewTelemetry() error = %v", err)
	}

	cfg := DefaultConfig()
	cfg.BaseURL = ts.URL

	client := NewClientWithHTTP(ts.Client(), ts.URL, "", 1)

	be, err := NewBackend(client, cfg, tel)
	if err != nil {
		t.Fatalf("NewBackend() error = %v", err)
	}

	return be, ts, tel
}

func makeNormalizedRequest(model, systemPrompt string, tools []request.Tool, messages []request.Message) *request.NormalizedRequest {
	return &request.NormalizedRequest{
		ID:           "test-req-1",
		Model:        model,
		SystemPrompt: systemPrompt,
		Tools:        tools,
		Messages:     messages,
		CreatedAt:    time.Now(),
	}
}

func standardCacheHandler(t *testing.T, cachedTokens int64, promptTokens int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t.Helper()
		var req ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []ChatCompletionChoice{
				{
					Index:        0,
					Message:      ChatMessage{Role: "assistant", Content: "Response."},
					FinishReason: "stop",
				},
			},
			Usage: UsageInfo{
				PromptTokens:     promptTokens,
				CompletionTokens: 20,
				TotalTokens:      promptTokens + 20,
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

// --- NewBackend tests ---

func TestNewBackend_Valid(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tel, _ := NewTelemetry(provider)
	cfg := DefaultConfig()
	client := NewClientWithHTTP(&http.Client{}, "http://localhost", "", 1)

	be, err := NewBackend(client, cfg, tel)
	if err != nil {
		t.Fatalf("NewBackend() error = %v", err)
	}
	if be == nil {
		t.Fatal("NewBackend() returned nil")
	}
}

func TestNewBackend_NilClient(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tel, _ := NewTelemetry(provider)
	cfg := DefaultConfig()

	_, err := NewBackend(nil, cfg, tel)
	if err == nil {
		t.Error("NewBackend(nil client) = nil error, want error")
	}
}

func TestNewBackend_NilConfig(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tel, _ := NewTelemetry(provider)
	client := NewClientWithHTTP(&http.Client{}, "http://localhost", "", 1)

	_, err := NewBackend(client, nil, tel)
	if err == nil {
		t.Error("NewBackend(nil config) = nil error, want error")
	}
}

func TestNewBackend_NilTelemetry(t *testing.T) {
	cfg := DefaultConfig()
	client := NewClientWithHTTP(&http.Client{}, "http://localhost", "", 1)

	_, err := NewBackend(client, cfg, nil)
	if err == nil {
		t.Error("NewBackend(nil telemetry) = nil error, want error")
	}
}

func TestNewBackend_InvalidConfig(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tel, _ := NewTelemetry(provider)
	client := NewClientWithHTTP(&http.Client{}, "http://localhost", "", 1)
	cfg := &Config{BaseURL: ""} // invalid

	_, err := NewBackend(client, cfg, tel)
	if err == nil {
		t.Error("NewBackend(invalid config) = nil error, want error")
	}
}

// --- Process tests ---

func TestBackend_Process_NilRequest(t *testing.T) {
	be, ts, _ := newTestBackend(t, successHandler(t, 0))
	defer ts.Close()

	_, err := be.Process(context.Background(), nil, nil)
	if err == nil {
		t.Error("Process(nil request) = nil error, want error")
	}
}

func TestBackend_Process_NoModel(t *testing.T) {
	be, ts, _ := newTestBackend(t, successHandler(t, 0))
	defer ts.Close()
	be.config.Model = "" // no default model

	req := makeNormalizedRequest("", "system prompt", nil, []request.Message{
		{Role: "user", Content: "hello"},
	})

	_, err := be.Process(context.Background(), req, nil)
	if err == nil {
		t.Error("Process(no model) = nil error, want error")
	}
}

func TestBackend_Process_UsesConfigModel(t *testing.T) {
	var gotModel string
	handler := func(w http.ResponseWriter, r *http.Request) {
		var req ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatCompletionResponse{
			ID: "test", Model: req.Model,
			Usage: UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	}

	be, ts, _ := newTestBackend(t, handler)
	defer ts.Close()
	be.config.Model = "default-model"

	req := makeNormalizedRequest("", "system prompt", nil, []request.Message{
		{Role: "user", Content: "hello"},
	})

	_, err := be.Process(context.Background(), req, &policy.PolicyDecision{UsePrefixCache: false})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if gotModel != "default-model" {
		t.Errorf("model sent = %q, want %q", gotModel, "default-model")
	}
}

// --- Acceptance Criteria: Prefix caching enabled ---

func TestAcceptance_PrefixCacheEnabled_CacheHit(t *testing.T) {
	// Given vLLM with prefix caching enabled
	// When I send request with stable prefix
	// Then vLLM uses prefix cache
	// And prefix tokens are reported
	// And cache hit is recorded
	be, ts, tel := newTestBackend(t, standardCacheHandler(t, 80, 100))
	defer ts.Close()

	longPrompt := "You are a highly knowledgeable assistant with expertise in software engineering. " +
		"You always provide detailed, well-structured responses with code examples when appropriate. " +
		"You follow best practices and explain trade-offs clearly."

	req := makeNormalizedRequest("llama-3-8b", longPrompt, []request.Tool{
		{
			Type: "function",
			Function: request.FunctionDefinition{
				Name:        "get_weather",
				Description: "Get weather for a location",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}, []request.Message{
		{Role: "user", Content: "What's the weather in NYC?"},
	})

	decision := &policy.PolicyDecision{UsePrefixCache: true}

	result, err := be.Process(context.Background(), req, decision)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Then vLLM uses prefix cache.
	if !result.PrefixUsed {
		t.Error("PrefixUsed = false, want true")
	}

	// And cache hit is recorded.
	if !result.CacheHit {
		t.Error("CacheHit = false, want true")
	}

	// And prefix tokens are reported.
	if result.PrefixTokens <= 0 {
		t.Errorf("PrefixTokens = %d, want > 0", result.PrefixTokens)
	}
	if result.CachedTokens != 80 {
		t.Errorf("CachedTokens = %d, want 80", result.CachedTokens)
	}
	if result.HitRate != 0.8 {
		t.Errorf("HitRate = %f, want 0.8", result.HitRate)
	}

	// Verify telemetry snapshot.
	snap := tel.Snapshot()
	if snap.TotalRequests != 1 {
		t.Errorf("snap.TotalRequests = %d, want 1", snap.TotalRequests)
	}
	if snap.CacheHitRequests != 1 {
		t.Errorf("snap.CacheHitRequests = %d, want 1", snap.CacheHitRequests)
	}
	if snap.TotalCachedTokens != 80 {
		t.Errorf("snap.TotalCachedTokens = %d, want 80", snap.TotalCachedTokens)
	}
}

// --- Acceptance Criteria: Not prefix-cache-eligible ---

func TestAcceptance_NotEligible_ProceedsNormally(t *testing.T) {
	// Given request is not prefix-cache-eligible
	// When processed
	// Then prefix cache is not used
	// And request proceeds normally
	be, ts, tel := newTestBackend(t, standardCacheHandler(t, 0, 20))
	defer ts.Close()

	// Minimal request with no system prompt, no tools — not eligible.
	req := makeNormalizedRequest("llama-3-8b", "", nil, []request.Message{
		{Role: "user", Content: "hi"},
	})

	decision := &policy.PolicyDecision{UsePrefixCache: true}

	result, err := be.Process(context.Background(), req, decision)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// Then prefix cache is not used.
	if result.PrefixUsed {
		t.Error("PrefixUsed = true, want false for ineligible request")
	}

	// And request proceeds normally.
	if result.Response == nil {
		t.Fatal("Response is nil, want valid response")
	}
	if result.Response.Usage.PromptTokens != 20 {
		t.Errorf("PromptTokens = %d, want 20", result.Response.Usage.PromptTokens)
	}

	snap := tel.Snapshot()
	if snap.TotalRequests != 1 {
		t.Errorf("snap.TotalRequests = %d, want 1", snap.TotalRequests)
	}
}

// --- Policy integration tests ---

func TestBackend_Process_PolicyDisablesPrefixCache(t *testing.T) {
	be, ts, _ := newTestBackend(t, standardCacheHandler(t, 0, 100))
	defer ts.Close()

	longPrompt := "You are an assistant. Provide detailed answers with examples and explanations."
	req := makeNormalizedRequest("llama-3-8b", longPrompt, nil, []request.Message{
		{Role: "user", Content: "Tell me about Go."},
	})

	// Policy says no prefix cache.
	decision := &policy.PolicyDecision{UsePrefixCache: false}

	result, err := be.Process(context.Background(), req, decision)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if result.PrefixUsed {
		t.Error("PrefixUsed = true, want false when policy disables prefix cache")
	}
}

func TestBackend_Process_NilDecision_EnablesPrefix(t *testing.T) {
	longPrompt := "You are an expert coding assistant. You write clean, efficient code with tests."
	be, ts, _ := newTestBackend(t, standardCacheHandler(t, 60, 100))
	defer ts.Close()

	req := makeNormalizedRequest("llama-3-8b", longPrompt, nil, []request.Message{
		{Role: "user", Content: "Write a sorting function."},
	})

	// nil decision — prefix cache should still be used if eligible.
	result, err := be.Process(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if !result.PrefixUsed {
		t.Error("PrefixUsed = false, want true with nil decision and eligible request")
	}
}

func TestBackend_Process_ConfigDisabled(t *testing.T) {
	be, ts, _ := newTestBackend(t, standardCacheHandler(t, 0, 100))
	defer ts.Close()
	be.config.Enabled = false

	longPrompt := "You are an assistant with deep knowledge of systems architecture."
	req := makeNormalizedRequest("llama-3-8b", longPrompt, nil, []request.Message{
		{Role: "user", Content: "Explain CAP theorem."},
	})

	result, err := be.Process(context.Background(), req, &policy.PolicyDecision{UsePrefixCache: true})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if result.PrefixUsed {
		t.Error("PrefixUsed = true, want false when config.Enabled = false")
	}
}

// --- Stable prefix construction tests ---

func TestBackend_BuildPrefixAwareMessages_SystemPrompt(t *testing.T) {
	be, ts, _ := newTestBackend(t, successHandler(t, 0))
	defer ts.Close()

	req := makeNormalizedRequest("model", "System instruction here", nil, []request.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	})

	messages, prefixTokens := be.buildPrefixAwareMessages(req)

	if len(messages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(messages))
	}
	// First message should be the system prompt.
	if messages[0].Role != "system" || messages[0].Content != "System instruction here" {
		t.Errorf("first message = {%s, %q}, want {system, 'System instruction here'}", messages[0].Role, messages[0].Content)
	}
	if prefixTokens <= 0 {
		t.Errorf("prefixTokens = %d, want > 0", prefixTokens)
	}
}

func TestBackend_BuildPrefixAwareMessages_ToolDefinitions(t *testing.T) {
	be, ts, _ := newTestBackend(t, successHandler(t, 0))
	defer ts.Close()

	req := makeNormalizedRequest("model", "System prompt", []request.Tool{
		{
			Type: "function",
			Function: request.FunctionDefinition{
				Name:        "search",
				Description: "Search the web",
			},
		},
	}, []request.Message{
		{Role: "user", Content: "Search for Go tutorials"},
	})

	messages, _ := be.buildPrefixAwareMessages(req)

	// Should have: system prompt, tool defs, user message.
	if len(messages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(messages))
	}
	// Second message should contain tool descriptions.
	if messages[1].Role != "system" {
		t.Errorf("second message role = %q, want 'system'", messages[1].Role)
	}
	if messages[1].Content == "" {
		t.Error("tool definition message is empty")
	}
}

func TestBackend_BuildPrefixAwareMessages_CommonContext(t *testing.T) {
	be, ts, _ := newTestBackend(t, successHandler(t, 0))
	defer ts.Close()

	req := makeNormalizedRequest("model", "Top-level system prompt", nil, []request.Message{
		{Role: "system", Content: "Additional context: you are in a test environment."},
		{Role: "user", Content: "Run the tests"},
	})

	messages, _ := be.buildPrefixAwareMessages(req)

	// Should have: system prompt, common context, user message.
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].Content != "Top-level system prompt" {
		t.Errorf("first = %q, want system prompt", messages[0].Content)
	}
	if messages[1].Content != "Additional context: you are in a test environment." {
		t.Errorf("second = %q, want common context", messages[1].Content)
	}
	if messages[2].Content != "Run the tests" {
		t.Errorf("third = %q, want user message", messages[2].Content)
	}
}

func TestBackend_BuildStandardMessages(t *testing.T) {
	be, ts, _ := newTestBackend(t, successHandler(t, 0))
	defer ts.Close()

	req := makeNormalizedRequest("model", "System", nil, []request.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	})

	messages := be.buildStandardMessages(req)

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].Role != "system" {
		t.Errorf("first role = %q, want 'system'", messages[0].Role)
	}
	if messages[1].Role != "user" {
		t.Errorf("second role = %q, want 'user'", messages[1].Role)
	}
}

func TestBackend_BuildStandardMessages_NoSystemPrompt(t *testing.T) {
	be, ts, _ := newTestBackend(t, successHandler(t, 0))
	defer ts.Close()

	req := makeNormalizedRequest("model", "", nil, []request.Message{
		{Role: "user", Content: "hi"},
	})

	messages := be.buildStandardMessages(req)
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
}

// --- messageContent tests ---

func TestMessageContent_StringContent(t *testing.T) {
	msg := request.Message{Role: "user", Content: "hello world"}
	got := messageContent(msg)
	if got != "hello world" {
		t.Errorf("messageContent() = %q, want %q", got, "hello world")
	}
}

func TestMessageContent_PartsContent(t *testing.T) {
	msg := request.Message{
		Role: "user",
		Parts: []request.ContentPart{
			{Type: "text", Text: "part one"},
			{Type: "text", Text: "part two"},
			{Type: "image_url", ImageURL: "http://example.com/img.png"},
		},
	}
	got := messageContent(msg)
	if got != "part one\npart two" {
		t.Errorf("messageContent() = %q, want %q", got, "part one\npart two")
	}
}

// --- buildToolDescriptionMessage tests ---

func TestBuildToolDescriptionMessage(t *testing.T) {
	tools := []request.Tool{
		{
			Type: "function",
			Function: request.FunctionDefinition{
				Name:        "search",
				Description: "Search the web",
				Parameters:  map[string]interface{}{"type": "object"},
			},
		},
		{
			Type: "function",
			Function: request.FunctionDefinition{
				Name: "calculate",
			},
		},
	}

	got := buildToolDescriptionMessage(tools)
	if got == "" {
		t.Fatal("buildToolDescriptionMessage() returned empty string")
	}
	if !strings.Contains(got, "search") {
		t.Error("expected 'search' in tool description")
	}
	if !strings.Contains(got, "calculate") {
		t.Error("expected 'calculate' in tool description")
	}
	if !strings.Contains(got, "Search the web") {
		t.Error("expected 'Search the web' in tool description")
	}
}

// --- isPrefixEligible tests ---

func TestBackend_IsPrefixEligible_LongPrompt(t *testing.T) {
	be, ts, _ := newTestBackend(t, successHandler(t, 0))
	defer ts.Close()

	// 100+ chars system prompt → ~25+ tokens → eligible (min 16).
	req := makeNormalizedRequest("model",
		"You are an expert assistant with deep knowledge of computer science, "+
			"mathematics, and software engineering best practices.",
		nil, nil)

	if !be.isPrefixEligible(req) {
		t.Error("isPrefixEligible() = false, want true for long system prompt")
	}
}

func TestBackend_IsPrefixEligible_ShortPrompt(t *testing.T) {
	be, ts, _ := newTestBackend(t, successHandler(t, 0))
	defer ts.Close()

	// Short prompt: "Hi" → ~0 tokens → not eligible.
	req := makeNormalizedRequest("model", "Hi", nil, nil)

	if be.isPrefixEligible(req) {
		t.Error("isPrefixEligible() = true, want false for short prompt")
	}
}

// --- ProcessWithoutPrefix tests ---

func TestBackend_ProcessWithoutPrefix(t *testing.T) {
	be, ts, _ := newTestBackend(t, standardCacheHandler(t, 0, 50))
	defer ts.Close()

	req := makeNormalizedRequest("llama-3-8b", "System prompt with enough tokens to be eligible for prefix caching", nil, []request.Message{
		{Role: "user", Content: "hello"},
	})

	result, err := be.ProcessWithoutPrefix(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessWithoutPrefix() error = %v", err)
	}
	if result.PrefixUsed {
		t.Error("PrefixUsed = true, want false for ProcessWithoutPrefix")
	}
}

// --- Integration test with mock vLLM ---

func TestIntegration_MockVLLM_PrefixCacheFlow(t *testing.T) {
	var requestCount atomic.Int32
	handler := func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		var req ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)

		cachedTokens := int64(0)
		if count > 1 {
			// Second+ requests hit the prefix cache.
			cachedTokens = 80
		}

		resp := ChatCompletionResponse{
			ID:      fmt.Sprintf("chatcmpl-%d", count),
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []ChatCompletionChoice{{
				Index:        0,
				Message:      ChatMessage{Role: "assistant", Content: "Response"},
				FinishReason: "stop",
			}},
			Usage: UsageInfo{
				PromptTokens:     100,
				CompletionTokens: 15,
				TotalTokens:      115,
			},
		}
		if cachedTokens > 0 {
			resp.Usage.PromptTokensDetails = &PromptTokensDetails{CachedTokens: cachedTokens}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}

	be, ts, tel := newTestBackend(t, handler)
	defer ts.Close()

	systemPrompt := "You are a highly capable coding assistant. You write clean, well-tested code. " +
		"Follow SOLID principles and prefer composition over inheritance."

	// First request: no cache hit.
	req1 := makeNormalizedRequest("llama-3-8b", systemPrompt, nil, []request.Message{
		{Role: "user", Content: "Write a binary search in Go."},
	})

	result1, err := be.Process(context.Background(), req1, &policy.PolicyDecision{UsePrefixCache: true})
	if err != nil {
		t.Fatalf("first Process() error = %v", err)
	}
	if result1.CacheHit {
		t.Error("first request: CacheHit = true, want false")
	}

	// Second request: same prefix, cache hit.
	req2 := makeNormalizedRequest("llama-3-8b", systemPrompt, nil, []request.Message{
		{Role: "user", Content: "Now write a merge sort."},
	})

	result2, err := be.Process(context.Background(), req2, &policy.PolicyDecision{UsePrefixCache: true})
	if err != nil {
		t.Fatalf("second Process() error = %v", err)
	}
	if !result2.CacheHit {
		t.Error("second request: CacheHit = false, want true")
	}
	if result2.CachedTokens != 80 {
		t.Errorf("second request: CachedTokens = %d, want 80", result2.CachedTokens)
	}

	// Verify aggregate telemetry.
	snap := tel.Snapshot()
	if snap.TotalRequests != 2 {
		t.Errorf("snap.TotalRequests = %d, want 2", snap.TotalRequests)
	}
	if snap.CacheHitRequests != 1 {
		t.Errorf("snap.CacheHitRequests = %d, want 1", snap.CacheHitRequests)
	}
	if snap.HitRate() != 0.5 {
		t.Errorf("snap.HitRate() = %f, want 0.5", snap.HitRate())
	}
}

// --- Benchmark ---

func BenchmarkBackend_Process(b *testing.B) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatCompletionResponse{
			ID:    "bench",
			Model: "model",
			Usage: UsageInfo{
				PromptTokens:     100,
				CompletionTokens: 20,
				TotalTokens:      120,
				PromptTokensDetails: &PromptTokensDetails{
					CachedTokens: 80,
				},
			},
			Choices: []ChatCompletionChoice{{
				Index:        0,
				Message:      ChatMessage{Role: "assistant", Content: "ok"},
				FinishReason: "stop",
			}},
		})
	}

	ts := httptest.NewServer(http.HandlerFunc(handler))
	defer ts.Close()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	tel, _ := NewTelemetry(provider)
	cfg := DefaultConfig()
	cfg.BaseURL = ts.URL
	client := NewClientWithHTTP(ts.Client(), ts.URL, "", 0)
	be, _ := NewBackend(client, cfg, tel)

	systemPrompt := "You are a helpful assistant that provides detailed technical explanations."

	req := makeNormalizedRequest("test-model", systemPrompt, []request.Tool{
		{Type: "function", Function: request.FunctionDefinition{Name: "search", Description: "Search"}},
	}, []request.Message{
		{Role: "user", Content: "Explain prefix caching."},
	})
	decision := &policy.PolicyDecision{UsePrefixCache: true}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := be.Process(context.Background(), req, decision)
		if err != nil {
			b.Fatalf("Process() error = %v", err)
		}
	}
}

func BenchmarkBackend_BuildPrefixAwareMessages(b *testing.B) {
	be := &Backend{config: DefaultConfig()}

	req := makeNormalizedRequest("model",
		"You are a detailed coding assistant. Follow best practices.",
		[]request.Tool{
			{Type: "function", Function: request.FunctionDefinition{
				Name:        "search",
				Description: "Search for code",
				Parameters:  map[string]interface{}{"type": "object"},
			}},
		},
		[]request.Message{
			{Role: "user", Content: "Write a function"},
			{Role: "assistant", Content: "Here it is"},
			{Role: "user", Content: "Add tests"},
		},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		be.buildPrefixAwareMessages(req)
	}
}

// --- Error handling test ---

func TestBackend_Process_VLLMError(t *testing.T) {
	be, ts, tel := newTestBackend(t, errorHandler(503, `{"error":"service unavailable"}`))
	defer ts.Close()

	longPrompt := "You are an assistant with expertise in distributed systems and cloud architecture."
	req := makeNormalizedRequest("llama-3-8b", longPrompt, nil, []request.Message{
		{Role: "user", Content: "Explain consensus algorithms."},
	})

	_, err := be.Process(context.Background(), req, &policy.PolicyDecision{UsePrefixCache: true})
	if err == nil {
		t.Fatal("Process() = nil error, want error for 503")
	}

	// Telemetry should still record the attempt.
	snap := tel.Snapshot()
	if snap.TotalRequests != 1 {
		t.Errorf("snap.TotalRequests = %d, want 1 (error counted)", snap.TotalRequests)
	}
}

// --- Prefix config flags tests ---

func TestBackend_Process_DisableSystemPromptInPrefix(t *testing.T) {
	var gotMessages []ChatMessage
	handler := func(w http.ResponseWriter, r *http.Request) {
		var req ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotMessages = req.Messages
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ChatCompletionResponse{
			ID: "test", Model: req.Model,
			Usage:   UsageInfo{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
			Choices: []ChatCompletionChoice{{Message: ChatMessage{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
		})
	}

	be, ts, _ := newTestBackend(t, handler)
	defer ts.Close()

	f := false
	be.config.IncludeSystemPrompt = &f
	be.config.MinPrefixTokens = 1 // low threshold so tools alone qualify

	req := makeNormalizedRequest("model", "System prompt", []request.Tool{
		{Type: "function", Function: request.FunctionDefinition{Name: "tool1", Description: "A tool with a description that is long enough to generate tokens for prefix caching eligibility"}},
	}, []request.Message{
		{Role: "user", Content: "hello"},
	})

	_, err := be.Process(context.Background(), req, &policy.PolicyDecision{UsePrefixCache: true})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// The system prompt from req.SystemPrompt should NOT appear in the prefix messages,
	// but the tool definitions should.
	for _, msg := range gotMessages {
		if msg.Content == "System prompt" {
			t.Error("system prompt should not appear when IncludeSystemPrompt=false")
		}
	}
}
