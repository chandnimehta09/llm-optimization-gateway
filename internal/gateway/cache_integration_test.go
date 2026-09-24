package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/cache"
)

// --- Test doubles ---

// mockCache implements cache.Cache for unit testing.
type mockCache struct {
	mu       sync.RWMutex
	store    map[string]*cache.Entry
	getErr   error
	setErr   error
	getCalls int
	setCalls int
	closed   bool
}

func newMockCache() *mockCache {
	return &mockCache{
		store: make(map[string]*cache.Entry),
	}
}

func (m *mockCache) Get(_ context.Context, key string) (*cache.Entry, bool, error) {
	m.mu.Lock()
	m.getCalls++
	m.mu.Unlock()

	if m.getErr != nil {
		return nil, false, m.getErr
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.store[key]
	if !ok {
		return nil, false, nil
	}
	return entry, true, nil
}

func (m *mockCache) Set(_ context.Context, key string, entry *cache.Entry, _ ...cache.Option) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCalls++

	if m.setErr != nil {
		return m.setErr
	}

	m.store[key] = entry
	return nil
}

func (m *mockCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	return nil
}

func (m *mockCache) Exists(_ context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.store[key]
	return ok, nil
}

func (m *mockCache) Close() error {
	m.closed = true
	return nil
}

func (m *mockCache) GetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getCalls
}

func (m *mockCache) SetCallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.setCalls
}

// mockHandler simulates a chat completion provider.
type mockHandler struct {
	calls    int
	mu       sync.Mutex
	response string
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		response: `{
			"id": "chatcmpl-test123",
			"object": "chat.completion",
			"created": 1234567890,
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {"role": "assistant", "content": "Hello from the provider!"},
				"finish_reason": "stop"
			}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`,
	}
}

func (h *mockHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.mu.Lock()
	h.calls++
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(h.response))
}

func (h *mockHandler) CallCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.calls
}

// --- Helper functions ---

func validChatRequest() string {
	return `{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "Hello, how are you?"}
		]
	}`
}

func chatRequestWithMetadata(metadata map[string]interface{}) string {
	req := map[string]interface{}{
		"model": "gpt-4",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello, how are you?"},
		},
		"metadata": metadata,
	}
	b, _ := json.Marshal(req)
	return string(b)
}

func streamingRequest() string {
	return `{
		"model": "gpt-4",
		"messages": [
			{"role": "user", "content": "Hello"}
		],
		"stream": true
	}`
}

func testConfig() CacheConfig {
	return CacheConfig{
		Enabled:         true,
		DefaultTTL:      5 * time.Minute,
		KeyPrefix:       "test",
		BypassStreaming: true,
	}
}

// --- CacheKeyBuilder tests ---

func TestCacheKeyBuilder_DeterministicKeys(t *testing.T) {
	builder := NewCacheKeyBuilder("test")

	input := CacheKeyInput{
		Model: "gpt-4",
		Messages: []CacheKeyMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	key1 := builder.BuildKey(input)
	key2 := builder.BuildKey(input)

	if key1 != key2 {
		t.Errorf("expected deterministic keys, got %s and %s", key1, key2)
	}
}

func TestCacheKeyBuilder_DifferentInputsDifferentKeys(t *testing.T) {
	builder := NewCacheKeyBuilder("test")

	input1 := CacheKeyInput{
		Model: "gpt-4",
		Messages: []CacheKeyMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	input2 := CacheKeyInput{
		Model: "gpt-4",
		Messages: []CacheKeyMessage{
			{Role: "user", Content: "Goodbye"},
		},
	}

	key1 := builder.BuildKey(input1)
	key2 := builder.BuildKey(input2)

	if key1 == key2 {
		t.Error("expected different keys for different inputs")
	}
}

func TestCacheKeyBuilder_IncludesPrefix(t *testing.T) {
	builder := NewCacheKeyBuilder("myprefix")

	input := CacheKeyInput{
		Model: "gpt-4",
		Messages: []CacheKeyMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	key := builder.BuildKey(input)
	if !strings.HasPrefix(key, "myprefix:") {
		t.Errorf("expected key to start with 'myprefix:', got %s", key)
	}
}

func TestCacheKeyBuilder_EmptyPrefix(t *testing.T) {
	builder := NewCacheKeyBuilder("")

	input := CacheKeyInput{
		Model: "gpt-4",
		Messages: []CacheKeyMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	key := builder.BuildKey(input)
	if strings.Contains(key, ":") {
		t.Errorf("expected no prefix separator, got %s", key)
	}
}

func TestCacheKeyBuilder_ParametersSensitive(t *testing.T) {
	builder := NewCacheKeyBuilder("test")
	temp1 := 0.7
	temp2 := 0.9

	base := CacheKeyInput{
		Model: "gpt-4",
		Messages: []CacheKeyMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	input1 := base
	input1.Temperature = &temp1

	input2 := base
	input2.Temperature = &temp2

	key1 := builder.BuildKey(input1)
	key2 := builder.BuildKey(input2)

	if key1 == key2 {
		t.Error("expected different keys for different temperatures")
	}
}

func TestCacheKeyBuilder_ToolsIncluded(t *testing.T) {
	builder := NewCacheKeyBuilder("test")

	withTools := CacheKeyInput{
		Model: "gpt-4",
		Messages: []CacheKeyMessage{
			{Role: "user", Content: "Hello"},
		},
		Tools: []CacheKeyTool{
			{Type: "function", FunctionName: "get_weather"},
		},
	}

	withoutTools := CacheKeyInput{
		Model: "gpt-4",
		Messages: []CacheKeyMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	key1 := builder.BuildKey(withTools)
	key2 := builder.BuildKey(withoutTools)

	if key1 == key2 {
		t.Error("expected different keys for requests with and without tools")
	}
}

func TestTruncatedKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{
			name:     "long hash",
			key:      "abcdef1234567890abcdef1234567890",
			expected: "abcdef123456",
		},
		{
			name:     "with prefix",
			key:      "prefix:abcdef1234567890abcdef1234567890",
			expected: "abcdef123456",
		},
		{
			name:     "short hash",
			key:      "abc",
			expected: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncatedKey(tt.key)
			if got != tt.expected {
				t.Errorf("TruncatedKey(%q) = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

// --- CacheLookup tests ---

func TestCacheLookup_Hit(t *testing.T) {
	mc := newMockCache()
	entry := &cache.Entry{
		Response: cache.CompletionResponse{
			ID:      "test-id",
			Content: "cached response",
			Usage:   cache.UsageInfo{TotalTokens: 10},
		},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now().Add(-30 * time.Second),
			TTL:       5 * time.Minute,
		},
	}
	mc.store["test-key"] = entry

	result := CacheLookup(context.Background(), mc, "test-key")

	if !result.Hit {
		t.Error("expected cache hit")
	}
	if result.Entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if result.Entry.Response.Content != "cached response" {
		t.Errorf("unexpected content: %s", result.Entry.Response.Content)
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
	if result.Age < 29*time.Second {
		t.Errorf("expected age >= 29s, got %s", result.Age)
	}
}

func TestCacheLookup_Miss(t *testing.T) {
	mc := newMockCache()

	result := CacheLookup(context.Background(), mc, "missing-key")

	if result.Hit {
		t.Error("expected cache miss")
	}
	if result.Entry != nil {
		t.Error("expected nil entry on miss")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
}

func TestCacheLookup_Error(t *testing.T) {
	mc := newMockCache()
	mc.getErr = fmt.Errorf("redis connection refused")

	result := CacheLookup(context.Background(), mc, "test-key")

	if result.Hit {
		t.Error("expected no hit on error")
	}
	if result.Error == nil {
		t.Error("expected error in result")
	}
}

func TestCacheLookup_NilCache(t *testing.T) {
	result := CacheLookup(context.Background(), nil, "test-key")

	if result.Hit {
		t.Error("expected no hit with nil cache")
	}
	if result.Error == nil {
		t.Error("expected error with nil cache")
	}
}

func TestCacheLookup_ExpiredEntry(t *testing.T) {
	mc := newMockCache()
	entry := &cache.Entry{
		Response: cache.CompletionResponse{
			ID:      "test-id",
			Content: "old response",
		},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now().Add(-10 * time.Minute),
			TTL:       1 * time.Minute, // Expired 9 minutes ago
		},
	}
	mc.store["test-key"] = entry

	result := CacheLookup(context.Background(), mc, "test-key")

	if result.Hit {
		t.Error("expected miss for expired entry")
	}
}

// --- CacheWrite tests ---

func TestCacheWrite_Success(t *testing.T) {
	mc := newMockCache()

	input := CacheWriteInput{
		Key: "test-key",
		Response: cache.CompletionResponse{
			ID:      "resp-123",
			Content: "test response",
			Usage:   cache.UsageInfo{TotalTokens: 15},
		},
		Provider: "openai",
		Model:    "gpt-4",
		TTL:      5 * time.Minute,
	}

	result := CacheWrite(context.Background(), mc, input)

	if !result.Written {
		t.Error("expected successful write")
	}
	if result.Error != nil {
		t.Errorf("unexpected error: %v", result.Error)
	}
	if mc.SetCallCount() != 1 {
		t.Errorf("expected 1 set call, got %d", mc.SetCallCount())
	}
}

func TestCacheWrite_Error(t *testing.T) {
	mc := newMockCache()
	mc.setErr = fmt.Errorf("write failed")

	input := CacheWriteInput{
		Key: "test-key",
		Response: cache.CompletionResponse{
			ID:      "resp-123",
			Content: "test response",
		},
		Provider: "openai",
		Model:    "gpt-4",
		TTL:      5 * time.Minute,
	}

	result := CacheWrite(context.Background(), mc, input)

	if result.Written {
		t.Error("expected write to fail")
	}
	if result.Error == nil {
		t.Error("expected error in result")
	}
}

func TestCacheWrite_NilCache(t *testing.T) {
	input := CacheWriteInput{
		Key: "test-key",
		Response: cache.CompletionResponse{
			ID: "resp-123",
		},
	}

	result := CacheWrite(context.Background(), nil, input)

	if result.Written {
		t.Error("expected write to fail with nil cache")
	}
	if result.Error == nil {
		t.Error("expected error with nil cache")
	}
}

// --- CacheLookupResultToJSON tests ---

func TestCacheLookupResultToJSON_ValidEntry(t *testing.T) {
	entry := &cache.Entry{
		Response: cache.CompletionResponse{
			ID:           "chatcmpl-abc",
			Model:        "gpt-4",
			Content:      "Hello!",
			FinishReason: "stop",
			Usage:        cache.UsageInfo{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
		},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now(),
		},
	}

	data, err := CacheLookupResultToJSON(entry, "gpt-4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp["id"] != "chatcmpl-abc" {
		t.Errorf("unexpected id: %v", resp["id"])
	}
	if resp["object"] != "chat.completion" {
		t.Errorf("unexpected object: %v", resp["object"])
	}
}

func TestCacheLookupResultToJSON_NilEntry(t *testing.T) {
	_, err := CacheLookupResultToJSON(nil, "gpt-4")
	if err == nil {
		t.Error("expected error for nil entry")
	}
}

// --- shouldBypass tests ---

func TestShouldBypass_CacheDisabled(t *testing.T) {
	config := CacheConfig{Enabled: false}
	req := &chatCompletionRequestWithCache{Model: "gpt-4"}

	bypass, reason := shouldBypass(config, req)
	if !bypass {
		t.Error("expected bypass when cache disabled")
	}
	if reason != "caching disabled" {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestShouldBypass_Streaming(t *testing.T) {
	config := CacheConfig{Enabled: true, BypassStreaming: true}
	req := &chatCompletionRequestWithCache{Model: "gpt-4", Stream: true}

	bypass, _ := shouldBypass(config, req)
	if !bypass {
		t.Error("expected bypass for streaming request")
	}
}

func TestShouldBypass_MetadataCacheFalse(t *testing.T) {
	config := CacheConfig{Enabled: true}
	req := &chatCompletionRequestWithCache{
		Model:    "gpt-4",
		Metadata: map[string]interface{}{"cache": false},
	}

	bypass, reason := shouldBypass(config, req)
	if !bypass {
		t.Error("expected bypass when metadata cache=false")
	}
	if reason != "cache disabled by metadata" {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestShouldBypass_MetadataCacheFalseString(t *testing.T) {
	config := CacheConfig{Enabled: true}
	req := &chatCompletionRequestWithCache{
		Model:    "gpt-4",
		Metadata: map[string]interface{}{"cache": "false"},
	}

	bypass, _ := shouldBypass(config, req)
	if !bypass {
		t.Error("expected bypass when metadata cache='false'")
	}
}

func TestShouldBypass_NormalRequest(t *testing.T) {
	config := CacheConfig{Enabled: true, BypassStreaming: true}
	req := &chatCompletionRequestWithCache{Model: "gpt-4"}

	bypass, _ := shouldBypass(config, req)
	if bypass {
		t.Error("expected no bypass for normal request")
	}
}

// --- resolveTTL tests ---

func TestResolveTTL_Default(t *testing.T) {
	config := CacheConfig{DefaultTTL: 10 * time.Minute}
	ttl := resolveTTL(config, nil)
	if ttl != 10*time.Minute {
		t.Errorf("expected 10m, got %s", ttl)
	}
}

func TestResolveTTL_Override(t *testing.T) {
	config := CacheConfig{DefaultTTL: 10 * time.Minute}
	overrideTTL := 30 * time.Second
	override := &RequestCacheOverride{TTL: &overrideTTL}

	ttl := resolveTTL(config, override)
	if ttl != 30*time.Second {
		t.Errorf("expected 30s, got %s", ttl)
	}
}

// --- extractOverride tests ---

func TestExtractOverride_Nil(t *testing.T) {
	override := extractOverride(nil)
	if override != nil {
		t.Error("expected nil override for nil metadata")
	}
}

func TestExtractOverride_WithTTL(t *testing.T) {
	metadata := map[string]interface{}{
		"cache_ttl": float64(120),
	}
	override := extractOverride(metadata)
	if override == nil {
		t.Fatal("expected non-nil override")
	}
	if override.TTL == nil {
		t.Fatal("expected non-nil TTL")
	}
	if *override.TTL != 120*time.Second {
		t.Errorf("expected 120s TTL, got %s", *override.TTL)
	}
}

func TestExtractOverride_WithTags(t *testing.T) {
	metadata := map[string]interface{}{
		"cache_tags": []interface{}{"tag1", "tag2"},
	}
	override := extractOverride(metadata)
	if override == nil {
		t.Fatal("expected non-nil override")
	}
	if len(override.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(override.Tags))
	}
}

func TestExtractOverride_NoOverrides(t *testing.T) {
	metadata := map[string]interface{}{
		"some_other_key": "value",
	}
	override := extractOverride(metadata)
	if override != nil {
		t.Error("expected nil override when no cache keys")
	}
}

// --- Integration tests: CacheMiddleware ---

func TestIntegration_CacheHit(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	// First request — should be a MISS
	resp1 := doPost(t, server.URL, validChatRequest())
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request failed: %d", resp1.StatusCode)
	}
	if resp1.Header.Get(HeaderCacheStatus) != string(CacheStatusMISS) {
		t.Errorf("expected MISS, got %s", resp1.Header.Get(HeaderCacheStatus))
	}
	if handler.CallCount() != 1 {
		t.Errorf("expected 1 provider call, got %d", handler.CallCount())
	}

	// Wait for async cache write
	time.Sleep(100 * time.Millisecond)

	// Second request — should be a HIT
	resp2 := doPost(t, server.URL, validChatRequest())
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("second request failed: %d", resp2.StatusCode)
	}
	if resp2.Header.Get(HeaderCacheStatus) != string(CacheStatusHIT) {
		t.Errorf("expected HIT, got %s", resp2.Header.Get(HeaderCacheStatus))
	}
	// Provider should NOT be called for the second request
	if handler.CallCount() != 1 {
		t.Errorf("expected provider to be called only once, got %d calls", handler.CallCount())
	}

	// Verify cache key header is present
	if resp2.Header.Get(HeaderCacheKey) == "" {
		t.Error("expected X-Cache-Key header")
	}

	// Verify cache age header is present
	if resp2.Header.Get(HeaderCacheAge) == "" {
		t.Error("expected X-Cache-Age header")
	}

	// Verify response body is valid JSON
	var body map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode cached response: %v", err)
	}
	if body["id"] == nil {
		t.Error("expected 'id' in cached response")
	}
}

func TestIntegration_CacheMiss(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	resp := doPost(t, server.URL, validChatRequest())
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("request failed: %d", resp.StatusCode)
	}
	if resp.Header.Get(HeaderCacheStatus) != string(CacheStatusMISS) {
		t.Errorf("expected MISS, got %s", resp.Header.Get(HeaderCacheStatus))
	}
	if resp.Header.Get(HeaderCacheKey) == "" {
		t.Error("expected X-Cache-Key header on miss")
	}
	if handler.CallCount() != 1 {
		t.Errorf("expected 1 provider call, got %d", handler.CallCount())
	}
}

func TestIntegration_CacheBypass_Streaming(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	resp := doPost(t, server.URL, streamingRequest())
	defer resp.Body.Close()

	if resp.Header.Get(HeaderCacheStatus) != string(CacheStatusBYPASS) {
		t.Errorf("expected BYPASS for streaming, got %s", resp.Header.Get(HeaderCacheStatus))
	}
}

func TestIntegration_CacheBypass_MetadataFalse(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	req := chatRequestWithMetadata(map[string]interface{}{
		"cache": false,
	})
	resp := doPost(t, server.URL, req)
	defer resp.Body.Close()

	if resp.Header.Get(HeaderCacheStatus) != string(CacheStatusBYPASS) {
		t.Errorf("expected BYPASS for cache:false, got %s", resp.Header.Get(HeaderCacheStatus))
	}
}

func TestIntegration_CacheBypass_Disabled(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()
	config.Enabled = false

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	resp := doPost(t, server.URL, validChatRequest())
	defer resp.Body.Close()

	if resp.Header.Get(HeaderCacheStatus) != string(CacheStatusBYPASS) {
		t.Errorf("expected BYPASS when disabled, got %s", resp.Header.Get(HeaderCacheStatus))
	}
}

func TestIntegration_CacheWriteFailureIsolation(t *testing.T) {
	mc := newMockCache()
	mc.setErr = fmt.Errorf("disk full")
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	// Request should succeed despite cache write failure
	resp := doPost(t, server.URL, validChatRequest())
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 despite cache write error, got %d", resp.StatusCode)
	}
	if resp.Header.Get(HeaderCacheStatus) != string(CacheStatusMISS) {
		t.Errorf("expected MISS, got %s", resp.Header.Get(HeaderCacheStatus))
	}

	// Verify the response body is still valid
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("response body invalid despite cache error: %v", err)
	}
}

func TestIntegration_CacheLookupFailureIsolation(t *testing.T) {
	mc := newMockCache()
	mc.getErr = fmt.Errorf("redis connection refused")
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	// Request should succeed despite cache lookup failure
	resp := doPost(t, server.URL, validChatRequest())
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 despite cache error, got %d", resp.StatusCode)
	}

	// X-Cache-Status should be BYPASS when cache is unavailable
	status := resp.Header.Get(HeaderCacheStatus)
	if status != string(CacheStatusBYPASS) {
		t.Errorf("expected BYPASS when cache unavailable, got %s", status)
	}

	// Provider should still be called
	if handler.CallCount() != 1 {
		t.Errorf("expected 1 provider call, got %d", handler.CallCount())
	}
}

func TestIntegration_NonPostMethodPassesThrough(t *testing.T) {
	mc := newMockCache()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// GET should pass through without cache headers
	if resp.Header.Get(HeaderCacheStatus) != "" {
		t.Errorf("expected no cache status for GET, got %s", resp.Header.Get(HeaderCacheStatus))
	}
}

func TestIntegration_InvalidJSON(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	resp := doPost(t, server.URL, "not json")
	defer resp.Body.Close()

	// Should bypass cache and let handler deal with invalid JSON
	if resp.Header.Get(HeaderCacheStatus) != string(CacheStatusBYPASS) {
		t.Errorf("expected BYPASS for invalid JSON, got %s", resp.Header.Get(HeaderCacheStatus))
	}
}

func TestIntegration_RequestLevelTTLOverride(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	req := chatRequestWithMetadata(map[string]interface{}{
		"cache_ttl": float64(3600),
	})
	resp := doPost(t, server.URL, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("request failed: %d", resp.StatusCode)
	}

	// Wait for async cache write
	time.Sleep(100 * time.Millisecond)

	// Verify TTL was applied (check the entry in mock cache)
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	for _, entry := range mc.store {
		if entry.Metadata.TTL != 3600*time.Second {
			t.Errorf("expected TTL of 3600s, got %s", entry.Metadata.TTL)
		}
	}
}

func TestIntegration_RequestLevelTagsOverride(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	req := chatRequestWithMetadata(map[string]interface{}{
		"cache_tags": []interface{}{"user:123", "session:abc"},
	})
	resp := doPost(t, server.URL, req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("request failed: %d", resp.StatusCode)
	}

	// Wait for async cache write
	time.Sleep(100 * time.Millisecond)

	// Verify tags were applied
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	for _, entry := range mc.store {
		if len(entry.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(entry.Tags))
		}
	}
}

func TestIntegration_CacheHitResponseBody(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	// First request to populate cache
	resp1 := doPost(t, server.URL, validChatRequest())
	resp1.Body.Close()
	time.Sleep(100 * time.Millisecond)

	// Second request for cache hit
	resp2 := doPost(t, server.URL, validChatRequest())
	defer resp2.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode cached response: %v", err)
	}

	// Verify the cached response has the expected structure
	if body["object"] != "chat.completion" {
		t.Errorf("expected object=chat.completion, got %v", body["object"])
	}
	choices, ok := body["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		t.Fatal("expected choices in response")
	}
	choice := choices[0].(map[string]interface{})
	msg := choice["message"].(map[string]interface{})
	if msg["content"] != "Hello from the provider!" {
		t.Errorf("unexpected content: %v", msg["content"])
	}
}

func TestIntegration_DifferentRequestsDifferentCacheKeys(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := testConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	req1 := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
	req2 := `{"model":"gpt-4","messages":[{"role":"user","content":"Goodbye"}]}`

	resp1 := doPost(t, server.URL, req1)
	resp1.Body.Close()
	time.Sleep(100 * time.Millisecond)

	resp2 := doPost(t, server.URL, req2)
	resp2.Body.Close()

	// Both should be misses (different content)
	if resp2.Header.Get(HeaderCacheStatus) != string(CacheStatusMISS) {
		t.Errorf("expected MISS for different request, got %s", resp2.Header.Get(HeaderCacheStatus))
	}

	// Provider should be called twice
	if handler.CallCount() != 2 {
		t.Errorf("expected 2 provider calls, got %d", handler.CallCount())
	}
}

// --- End-to-end test with mock Redis (using in-memory mock as stand-in) ---

func TestE2E_CacheLifecycle(t *testing.T) {
	mc := newMockCache()
	handler := newMockHandler()
	config := DefaultCacheConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	// Step 1: First request — MISS, provider called
	resp1 := doPost(t, server.URL, validChatRequest())
	defer resp1.Body.Close()

	if resp1.Header.Get(HeaderCacheStatus) != string(CacheStatusMISS) {
		t.Fatalf("step 1: expected MISS, got %s", resp1.Header.Get(HeaderCacheStatus))
	}
	if handler.CallCount() != 1 {
		t.Fatalf("step 1: expected 1 provider call, got %d", handler.CallCount())
	}

	// Wait for async cache write
	time.Sleep(100 * time.Millisecond)

	// Step 2: Same request — HIT, provider NOT called
	resp2 := doPost(t, server.URL, validChatRequest())
	defer resp2.Body.Close()

	if resp2.Header.Get(HeaderCacheStatus) != string(CacheStatusHIT) {
		t.Fatalf("step 2: expected HIT, got %s", resp2.Header.Get(HeaderCacheStatus))
	}
	if handler.CallCount() != 1 {
		t.Fatalf("step 2: expected 1 provider call (no new call), got %d", handler.CallCount())
	}

	// Step 3: Different request — MISS, provider called
	differentReq := `{"model":"gpt-3.5-turbo","messages":[{"role":"user","content":"Different"}]}`
	resp3 := doPost(t, server.URL, differentReq)
	defer resp3.Body.Close()

	if resp3.Header.Get(HeaderCacheStatus) != string(CacheStatusMISS) {
		t.Fatalf("step 3: expected MISS, got %s", resp3.Header.Get(HeaderCacheStatus))
	}
	if handler.CallCount() != 2 {
		t.Fatalf("step 3: expected 2 provider calls, got %d", handler.CallCount())
	}

	// Step 4: Bypass request — BYPASS
	bypassReq := chatRequestWithMetadata(map[string]interface{}{
		"cache": false,
	})
	resp4 := doPost(t, server.URL, bypassReq)
	defer resp4.Body.Close()

	if resp4.Header.Get(HeaderCacheStatus) != string(CacheStatusBYPASS) {
		t.Fatalf("step 4: expected BYPASS, got %s", resp4.Header.Get(HeaderCacheStatus))
	}
}

func TestE2E_CacheUnavailable(t *testing.T) {
	mc := newMockCache()
	mc.getErr = fmt.Errorf("connection refused")
	handler := newMockHandler()
	config := DefaultCacheConfig()

	middleware := CacheMiddleware(mc, config)
	server := httptest.NewServer(middleware(handler))
	defer server.Close()

	// Request should succeed despite cache being unavailable
	resp := doPost(t, server.URL, validChatRequest())
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Cache status should be BYPASS when unavailable
	if resp.Header.Get(HeaderCacheStatus) != string(CacheStatusBYPASS) {
		t.Errorf("expected BYPASS when cache unavailable, got %s", resp.Header.Get(HeaderCacheStatus))
	}

	// Response should still be valid
	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("response body invalid: %v", err)
	}
	if body["id"] == nil {
		t.Error("expected 'id' in response")
	}
}

// --- buildCacheKeyInput tests ---

func TestBuildCacheKeyInput(t *testing.T) {
	req := &chatCompletionRequestWithCache{
		Model: "gpt-4",
		Messages: []chatCompletionMsg{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		},
		Tools: []chatCompletionTool{
			{Type: "function", Function: chatCompletionToolFunction{Name: "get_weather"}},
		},
		ToolChoice: "auto",
	}

	input := buildCacheKeyInput(req)

	if input.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", input.Model)
	}
	if len(input.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(input.Messages))
	}
	if len(input.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(input.Tools))
	}
	if input.ToolChoice != "auto" {
		t.Errorf("expected tool_choice=auto, got %s", input.ToolChoice)
	}
}

// --- DefaultCacheConfig tests ---

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	if !config.Enabled {
		t.Error("expected Enabled=true")
	}
	if config.DefaultTTL != 10*time.Minute {
		t.Errorf("expected 10m TTL, got %s", config.DefaultTTL)
	}
	if config.KeyPrefix != "llm-gw" {
		t.Errorf("expected prefix 'llm-gw', got %s", config.KeyPrefix)
	}
	if !config.BypassStreaming {
		t.Error("expected BypassStreaming=true")
	}
}

// --- responseCapture tests ---

func TestResponseCapture_CapturesBody(t *testing.T) {
	recorder := httptest.NewRecorder()
	rc := newResponseCapture(recorder)

	rc.WriteHeader(http.StatusOK)
	rc.Write([]byte("hello"))
	rc.Write([]byte(" world"))

	if string(rc.body) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(rc.body))
	}
	if rc.statusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", rc.statusCode)
	}
}

func TestResponseCapture_DefaultsTo200(t *testing.T) {
	recorder := httptest.NewRecorder()
	rc := newResponseCapture(recorder)

	if rc.statusCode != http.StatusOK {
		t.Errorf("expected default 200, got %d", rc.statusCode)
	}
}

// --- Helper ---

func doPost(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	return resp
}
