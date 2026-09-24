package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// In-memory cache implementation for testing the interface contract
// ---------------------------------------------------------------------------

// memoryCache is a simple in-memory Cache implementation used exclusively
// for testing the interface contract and acceptance criteria.
type memoryCache struct {
	mu      sync.RWMutex
	entries map[string]storedEntry
	stats   Statistics
	closed  bool
}

type storedEntry struct {
	entry    *Entry
	priority Priority
}

func newMemoryCache() *memoryCache {
	return &memoryCache{
		entries: make(map[string]storedEntry),
	}
}

func (m *memoryCache) Get(ctx context.Context, key string) (*Entry, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	se, ok := m.entries[key]
	if !ok {
		m.stats.Misses++
		return nil, false, nil
	}

	if se.entry.IsExpired() {
		delete(m.entries, key)
		m.stats.Expirations++
		m.stats.Misses++
		return nil, false, &CacheExpiredError{
			Key:       key,
			ExpiredAt: se.entry.ExpiresAt().Format(time.RFC3339),
		}
	}

	se.entry.IncrementHitCount()
	m.stats.Hits++
	return se.entry, true, nil
}

func (m *memoryCache) Set(ctx context.Context, key string, entry *Entry, opts ...Option) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	resolved := ResolveOptions(opts...)

	// Apply option overrides to the entry.
	if resolved.TTL > 0 {
		entry.Metadata.TTL = resolved.TTL
	}
	if len(resolved.Tags) > 0 {
		entry.Tags = append(entry.Tags, resolved.Tags...)
	}

	m.entries[key] = storedEntry{
		entry:    entry,
		priority: resolved.Priority,
	}
	m.stats.Size = int64(len(m.entries))
	return nil
}

func (m *memoryCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
	m.stats.Size = int64(len(m.entries))
	return nil
}

func (m *memoryCache) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	se, ok := m.entries[key]
	if !ok {
		return false, nil
	}
	if se.entry.IsExpired() {
		return false, nil
	}
	return true, nil
}

func (m *memoryCache) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.entries = nil
	return nil
}

func (m *memoryCache) Stats() Statistics {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats
}

func (m *memoryCache) ResetStats() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = Statistics{}
}

// Compile-time interface satisfaction checks.
var _ Cache = (*memoryCache)(nil)
var _ StatsCollector = (*memoryCache)(nil)

// ---------------------------------------------------------------------------
// Helper to build test entries
// ---------------------------------------------------------------------------

func newTestEntry(content string, ttl time.Duration) *Entry {
	return &Entry{
		Response: CompletionResponse{
			ID:           "resp-001",
			Model:        "gpt-4",
			Content:      content,
			FinishReason: "stop",
			ToolCalls: []ToolCallResponse{
				{
					ID:           "tc-1",
					Type:         "function",
					FunctionName: "get_weather",
					FunctionArgs: `{"city":"Seattle"}`,
				},
			},
			Usage: UsageInfo{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		},
		Metadata: EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       ttl,
			HitCount:  0,
			Tokens:    150,
			Cost:      0.0045,
		},
		ContextHash: "sha256:abc123",
		Provider:    "openai",
		Model:       "gpt-4",
		Tags:        []string{"user:42", "session:xyz"},
	}
}

// ---------------------------------------------------------------------------
// Interface contract tests (acceptance criteria)
// ---------------------------------------------------------------------------

func TestCache_SetAndGet(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("Hello, world!", 5*time.Minute)
	if err := c.Set(ctx, "abc", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, err := c.Get(ctx, "abc")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !found {
		t.Fatal("Get() returned found=false, want true")
	}

	// Verify response data preserved.
	if got.Response.Content != "Hello, world!" {
		t.Errorf("Response.Content = %q, want %q", got.Response.Content, "Hello, world!")
	}
	if got.Response.Model != "gpt-4" {
		t.Errorf("Response.Model = %q, want %q", got.Response.Model, "gpt-4")
	}
	if got.Response.FinishReason != "stop" {
		t.Errorf("Response.FinishReason = %q, want %q", got.Response.FinishReason, "stop")
	}
	if got.Response.Usage.TotalTokens != 150 {
		t.Errorf("Response.Usage.TotalTokens = %d, want 150", got.Response.Usage.TotalTokens)
	}

	// Verify metadata preserved.
	if got.Metadata.Tokens != 150 {
		t.Errorf("Metadata.Tokens = %d, want 150", got.Metadata.Tokens)
	}
	if got.Metadata.Cost != 0.0045 {
		t.Errorf("Metadata.Cost = %f, want 0.0045", got.Metadata.Cost)
	}

	// Verify provider/model info.
	if got.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", got.Provider, "openai")
	}
	if got.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", got.Model, "gpt-4")
	}

	// Verify context hash.
	if got.ContextHash != "sha256:abc123" {
		t.Errorf("ContextHash = %q, want %q", got.ContextHash, "sha256:abc123")
	}

	// Verify tags.
	if len(got.Tags) != 2 {
		t.Errorf("Tags length = %d, want 2", len(got.Tags))
	}

	// Verify hit count incremented on Get.
	if got.Metadata.HitCount != 1 {
		t.Errorf("HitCount = %d, want 1 after one Get()", got.Metadata.HitCount)
	}

	// Verify tool calls preserved.
	if len(got.Response.ToolCalls) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(got.Response.ToolCalls))
	}
	if got.Response.ToolCalls[0].FunctionName != "get_weather" {
		t.Errorf("ToolCalls[0].FunctionName = %q, want %q", got.Response.ToolCalls[0].FunctionName, "get_weather")
	}
}

func TestCache_GetMiss(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	got, found, err := c.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if found {
		t.Error("Get() returned found=true for missing key")
	}
	if got != nil {
		t.Error("Get() returned non-nil entry for missing key")
	}
}

func TestCache_GetExpiredEntry(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	entry := &Entry{
		Response: CompletionResponse{Content: "expired content"},
		Metadata: EntryMetadata{
			CreatedAt: time.Now().Add(-2 * time.Hour),
			TTL:       1 * time.Hour,
		},
		ContextHash: "hash",
		Provider:    "openai",
		Model:       "gpt-4",
	}

	if err := c.Set(ctx, "expired-key", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	_, found, err := c.Get(ctx, "expired-key")
	if found {
		t.Error("Get() returned found=true for expired entry")
	}
	if err == nil {
		t.Fatal("Get() returned nil error for expired entry")
	}

	var expiredErr *CacheExpiredError
	if !errors.As(err, &expiredErr) {
		t.Fatalf("Get() error type = %T, want *CacheExpiredError", err)
	}
	if expiredErr.Key != "expired-key" {
		t.Errorf("CacheExpiredError.Key = %q, want %q", expiredErr.Key, "expired-key")
	}
}

func TestCache_SetWithTTLOption(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	entry := &Entry{
		Response: CompletionResponse{Content: "ttl test"},
		Metadata: EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       0, // no default TTL
		},
		ContextHash: "hash",
		Provider:    "openai",
		Model:       "gpt-4",
	}

	if err := c.Set(ctx, "ttl-key", entry, WithTTL(60*time.Second)); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, err := c.Get(ctx, "ttl-key")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !found {
		t.Fatal("Get() returned found=false")
	}
	if got.Metadata.TTL != 60*time.Second {
		t.Errorf("Metadata.TTL = %v, want 60s", got.Metadata.TTL)
	}
}

func TestCache_Delete(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("to delete", 5*time.Minute)
	_ = c.Set(ctx, "del-key", entry)

	if err := c.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, found, _ := c.Get(ctx, "del-key")
	if found {
		t.Error("entry still found after Delete()")
	}
}

func TestCache_DeleteMissing(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	// Deleting a non-existent key should not error.
	if err := c.Delete(ctx, "no-such-key"); err != nil {
		t.Fatalf("Delete() error for missing key: %v", err)
	}
}

func TestCache_Exists(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("exists test", 5*time.Minute)
	_ = c.Set(ctx, "exists-key", entry)

	exists, err := c.Exists(ctx, "exists-key")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !exists {
		t.Error("Exists() returned false for existing key")
	}

	exists, err = c.Exists(ctx, "missing-key")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if exists {
		t.Error("Exists() returned true for missing key")
	}
}

func TestCache_ExistsExpired(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	entry := &Entry{
		Response: CompletionResponse{Content: "expired"},
		Metadata: EntryMetadata{
			CreatedAt: time.Now().Add(-2 * time.Hour),
			TTL:       1 * time.Hour,
		},
		Provider: "openai",
		Model:    "gpt-4",
	}
	_ = c.Set(ctx, "expired-exists", entry)

	exists, err := c.Exists(ctx, "expired-exists")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if exists {
		t.Error("Exists() returned true for expired entry")
	}
}

func TestCache_Close(t *testing.T) {
	c := newMemoryCache()

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if !c.closed {
		t.Error("Close() did not set closed flag")
	}
}

// ---------------------------------------------------------------------------
// Entry serialization tests
// ---------------------------------------------------------------------------

func TestEntry_JSONRoundTrip(t *testing.T) {
	original := newTestEntry("round trip content", 30*time.Minute)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var restored Entry
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	// Verify all fields round-trip correctly.
	if restored.Response.Content != original.Response.Content {
		t.Errorf("Response.Content = %q, want %q", restored.Response.Content, original.Response.Content)
	}
	if restored.Response.ID != original.Response.ID {
		t.Errorf("Response.ID = %q, want %q", restored.Response.ID, original.Response.ID)
	}
	if restored.Response.Model != original.Response.Model {
		t.Errorf("Response.Model = %q, want %q", restored.Response.Model, original.Response.Model)
	}
	if restored.Response.FinishReason != original.Response.FinishReason {
		t.Errorf("Response.FinishReason = %q, want %q", restored.Response.FinishReason, original.Response.FinishReason)
	}
	if restored.Response.Usage.TotalTokens != original.Response.Usage.TotalTokens {
		t.Errorf("Usage.TotalTokens = %d, want %d", restored.Response.Usage.TotalTokens, original.Response.Usage.TotalTokens)
	}
	if restored.Metadata.TTL != original.Metadata.TTL {
		t.Errorf("Metadata.TTL = %v, want %v", restored.Metadata.TTL, original.Metadata.TTL)
	}
	if restored.Metadata.HitCount != original.Metadata.HitCount {
		t.Errorf("Metadata.HitCount = %d, want %d", restored.Metadata.HitCount, original.Metadata.HitCount)
	}
	if restored.Metadata.Tokens != original.Metadata.Tokens {
		t.Errorf("Metadata.Tokens = %d, want %d", restored.Metadata.Tokens, original.Metadata.Tokens)
	}
	if restored.Metadata.Cost != original.Metadata.Cost {
		t.Errorf("Metadata.Cost = %f, want %f", restored.Metadata.Cost, original.Metadata.Cost)
	}
	if restored.ContextHash != original.ContextHash {
		t.Errorf("ContextHash = %q, want %q", restored.ContextHash, original.ContextHash)
	}
	if restored.Provider != original.Provider {
		t.Errorf("Provider = %q, want %q", restored.Provider, original.Provider)
	}
	if restored.Model != original.Model {
		t.Errorf("Model = %q, want %q", restored.Model, original.Model)
	}
	if len(restored.Tags) != len(original.Tags) {
		t.Errorf("Tags length = %d, want %d", len(restored.Tags), len(original.Tags))
	}
	for i, tag := range original.Tags {
		if i < len(restored.Tags) && restored.Tags[i] != tag {
			t.Errorf("Tags[%d] = %q, want %q", i, restored.Tags[i], tag)
		}
	}

	// Tool calls.
	if len(restored.Response.ToolCalls) != len(original.Response.ToolCalls) {
		t.Fatalf("ToolCalls length = %d, want %d", len(restored.Response.ToolCalls), len(original.Response.ToolCalls))
	}
	if restored.Response.ToolCalls[0].FunctionName != original.Response.ToolCalls[0].FunctionName {
		t.Errorf("ToolCalls[0].FunctionName = %q, want %q",
			restored.Response.ToolCalls[0].FunctionName, original.Response.ToolCalls[0].FunctionName)
	}
}

func TestEntry_JSONRoundTrip_NoTTL(t *testing.T) {
	entry := &Entry{
		Response: CompletionResponse{Content: "no ttl"},
		Metadata: EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       0,
		},
		Provider: "openai",
		Model:    "gpt-4",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var restored Entry
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if restored.Metadata.TTL != 0 {
		t.Errorf("Metadata.TTL = %v, want 0", restored.Metadata.TTL)
	}
}

func TestEntry_JSONRoundTrip_EmptyToolCalls(t *testing.T) {
	entry := &Entry{
		Response: CompletionResponse{
			Content: "no tools",
			Usage:   UsageInfo{TotalTokens: 10},
		},
		Metadata: EntryMetadata{CreatedAt: time.Now()},
		Provider: "openai",
		Model:    "gpt-4",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var restored Entry
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if len(restored.Response.ToolCalls) != 0 {
		t.Errorf("ToolCalls length = %d, want 0", len(restored.Response.ToolCalls))
	}
}

func TestEntryMetadata_JSONRoundTrip_Duration(t *testing.T) {
	original := EntryMetadata{
		CreatedAt: time.Now().Truncate(time.Second),
		TTL:       90 * time.Second,
		HitCount:  42,
		Tokens:    200,
		Cost:      0.01,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	var restored EntryMetadata
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if restored.TTL != original.TTL {
		t.Errorf("TTL = %v, want %v", restored.TTL, original.TTL)
	}
	if restored.HitCount != original.HitCount {
		t.Errorf("HitCount = %d, want %d", restored.HitCount, original.HitCount)
	}
}

// ---------------------------------------------------------------------------
// Entry method tests
// ---------------------------------------------------------------------------

func TestEntry_IsExpired(t *testing.T) {
	tests := []struct {
		name    string
		entry   *Entry
		expired bool
	}{
		{
			name: "not expired",
			entry: &Entry{
				Metadata: EntryMetadata{
					CreatedAt: time.Now(),
					TTL:       1 * time.Hour,
				},
			},
			expired: false,
		},
		{
			name: "expired",
			entry: &Entry{
				Metadata: EntryMetadata{
					CreatedAt: time.Now().Add(-2 * time.Hour),
					TTL:       1 * time.Hour,
				},
			},
			expired: true,
		},
		{
			name: "no TTL never expires",
			entry: &Entry{
				Metadata: EntryMetadata{
					CreatedAt: time.Now().Add(-100 * time.Hour),
					TTL:       0,
				},
			},
			expired: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entry.IsExpired(); got != tt.expired {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expired)
			}
		})
	}
}

func TestEntry_ExpiresAt(t *testing.T) {
	now := time.Now()
	entry := &Entry{
		Metadata: EntryMetadata{
			CreatedAt: now,
			TTL:       30 * time.Minute,
		},
	}

	expected := now.Add(30 * time.Minute)
	if got := entry.ExpiresAt(); !got.Equal(expected) {
		t.Errorf("ExpiresAt() = %v, want %v", got, expected)
	}
}

func TestEntry_ExpiresAt_NoTTL(t *testing.T) {
	entry := &Entry{
		Metadata: EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       0,
		},
	}

	if got := entry.ExpiresAt(); !got.IsZero() {
		t.Errorf("ExpiresAt() = %v, want zero time", got)
	}
}

func TestEntry_IncrementHitCount(t *testing.T) {
	entry := &Entry{}
	if entry.Metadata.HitCount != 0 {
		t.Fatalf("initial HitCount = %d, want 0", entry.Metadata.HitCount)
	}
	entry.IncrementHitCount()
	if entry.Metadata.HitCount != 1 {
		t.Errorf("HitCount = %d, want 1", entry.Metadata.HitCount)
	}
	entry.IncrementHitCount()
	if entry.Metadata.HitCount != 2 {
		t.Errorf("HitCount = %d, want 2", entry.Metadata.HitCount)
	}
}

// ---------------------------------------------------------------------------
// Error type tests
// ---------------------------------------------------------------------------

func TestCacheMissError(t *testing.T) {
	err := &CacheMissError{Key: "some-key"}
	expected := `cache miss: key "some-key" not found`
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestCacheExpiredError(t *testing.T) {
	err := &CacheExpiredError{Key: "expired-key", ExpiredAt: "2026-01-01T00:00:00Z"}
	expected := `cache expired: key "expired-key" expired at 2026-01-01T00:00:00Z`
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestCacheCorruptedError(t *testing.T) {
	err := &CacheCorruptedError{Key: "bad-key", Reason: "invalid JSON"}
	expected := `cache corrupted: key "bad-key": invalid JSON`
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestIsCacheMiss(t *testing.T) {
	if !IsCacheMiss(&CacheMissError{Key: "k"}) {
		t.Error("IsCacheMiss() returned false for CacheMissError")
	}
	if IsCacheMiss(fmt.Errorf("other error")) {
		t.Error("IsCacheMiss() returned true for non-CacheMissError")
	}
}

func TestIsCacheExpired(t *testing.T) {
	if !IsCacheExpired(&CacheExpiredError{Key: "k"}) {
		t.Error("IsCacheExpired() returned false for CacheExpiredError")
	}
	if IsCacheExpired(fmt.Errorf("other error")) {
		t.Error("IsCacheExpired() returned true for non-CacheExpiredError")
	}
}

func TestIsCacheCorrupted(t *testing.T) {
	if !IsCacheCorrupted(&CacheCorruptedError{Key: "k", Reason: "bad"}) {
		t.Error("IsCacheCorrupted() returned false for CacheCorruptedError")
	}
	if IsCacheCorrupted(fmt.Errorf("other error")) {
		t.Error("IsCacheCorrupted() returned true for non-CacheCorruptedError")
	}
}

func TestErrorInterfaces(t *testing.T) {
	// Verify all error types implement the error interface.
	var _ error = (*CacheMissError)(nil)
	var _ error = (*CacheExpiredError)(nil)
	var _ error = (*CacheCorruptedError)(nil)
}

// ---------------------------------------------------------------------------
// Option pattern tests
// ---------------------------------------------------------------------------

func TestDefaultSetOptions(t *testing.T) {
	opts := DefaultSetOptions()
	if opts.TTL != 0 {
		t.Errorf("default TTL = %v, want 0", opts.TTL)
	}
	if opts.Priority != PriorityNormal {
		t.Errorf("default Priority = %v, want PriorityNormal", opts.Priority)
	}
	if len(opts.Tags) != 0 {
		t.Errorf("default Tags length = %d, want 0", len(opts.Tags))
	}
}

func TestWithTTL(t *testing.T) {
	opts := ResolveOptions(WithTTL(5 * time.Minute))
	if opts.TTL != 5*time.Minute {
		t.Errorf("TTL = %v, want 5m", opts.TTL)
	}
}

func TestWithTags(t *testing.T) {
	opts := ResolveOptions(WithTags("tag1", "tag2"))
	if len(opts.Tags) != 2 {
		t.Fatalf("Tags length = %d, want 2", len(opts.Tags))
	}
	if opts.Tags[0] != "tag1" || opts.Tags[1] != "tag2" {
		t.Errorf("Tags = %v, want [tag1 tag2]", opts.Tags)
	}
}

func TestWithTags_Multiple(t *testing.T) {
	opts := ResolveOptions(WithTags("a"), WithTags("b", "c"))
	if len(opts.Tags) != 3 {
		t.Fatalf("Tags length = %d, want 3", len(opts.Tags))
	}
}

func TestWithPriority(t *testing.T) {
	opts := ResolveOptions(WithPriority(PriorityHigh))
	if opts.Priority != PriorityHigh {
		t.Errorf("Priority = %v, want PriorityHigh", opts.Priority)
	}
}

func TestResolveOptions_Combined(t *testing.T) {
	opts := ResolveOptions(
		WithTTL(10*time.Second),
		WithTags("user:1"),
		WithPriority(PriorityLow),
	)
	if opts.TTL != 10*time.Second {
		t.Errorf("TTL = %v, want 10s", opts.TTL)
	}
	if len(opts.Tags) != 1 || opts.Tags[0] != "user:1" {
		t.Errorf("Tags = %v, want [user:1]", opts.Tags)
	}
	if opts.Priority != PriorityLow {
		t.Errorf("Priority = %v, want PriorityLow", opts.Priority)
	}
}

func TestResolveOptions_NoOptions(t *testing.T) {
	opts := ResolveOptions()
	defaults := DefaultSetOptions()
	if opts.TTL != defaults.TTL || opts.Priority != defaults.Priority {
		t.Error("ResolveOptions() with no options should return defaults")
	}
}

// ---------------------------------------------------------------------------
// Priority tests
// ---------------------------------------------------------------------------

func TestPriority_String(t *testing.T) {
	tests := []struct {
		p    Priority
		want string
	}{
		{PriorityLow, "low"},
		{PriorityNormal, "normal"},
		{PriorityHigh, "high"},
		{Priority(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.p.String(); got != tt.want {
				t.Errorf("Priority(%d).String() = %q, want %q", tt.p, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Statistics tests
// ---------------------------------------------------------------------------

func TestStatistics_HitRate(t *testing.T) {
	tests := []struct {
		name string
		hits int64
		miss int64
		want float64
	}{
		{"50%", 5, 5, 0.5},
		{"100%", 10, 0, 1.0},
		{"0%", 0, 10, 0.0},
		{"no requests", 0, 0, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Statistics{Hits: tt.hits, Misses: tt.miss}
			if got := s.HitRate(); got != tt.want {
				t.Errorf("HitRate() = %f, want %f", got, tt.want)
			}
		})
	}
}

func TestStatsCollector_Integration(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("stats test", 5*time.Minute)
	_ = c.Set(ctx, "s1", entry)

	// Cache hit.
	_, _, _ = c.Get(ctx, "s1")
	// Cache miss.
	_, _, _ = c.Get(ctx, "missing")

	stats := c.Stats()
	if stats.Hits != 1 {
		t.Errorf("stats.Hits = %d, want 1", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("stats.Misses = %d, want 1", stats.Misses)
	}
	if stats.Size != 1 {
		t.Errorf("stats.Size = %d, want 1", stats.Size)
	}

	c.ResetStats()
	stats = c.Stats()
	if stats.Hits != 0 || stats.Misses != 0 {
		t.Error("ResetStats() did not zero out counters")
	}
}

// ---------------------------------------------------------------------------
// Acceptance Criteria: Set with tags via options
// ---------------------------------------------------------------------------

func TestCache_SetWithTagsOption(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	entry := &Entry{
		Response: CompletionResponse{Content: "tagged"},
		Metadata: EntryMetadata{CreatedAt: time.Now(), TTL: 5 * time.Minute},
		Provider: "openai",
		Model:    "gpt-4",
	}

	err := c.Set(ctx, "tagged-key", entry, WithTags("env:prod", "team:ml"))
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, err := c.Get(ctx, "tagged-key")
	if err != nil || !found {
		t.Fatalf("Get() found=%v, error=%v", found, err)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags length = %d, want 2", len(got.Tags))
	}
}

// ---------------------------------------------------------------------------
// Acceptance Criteria: Set with priority via options
// ---------------------------------------------------------------------------

func TestCache_SetWithPriorityOption(t *testing.T) {
	c := newMemoryCache()
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("priority test", 5*time.Minute)

	err := c.Set(ctx, "pri-key", entry, WithPriority(PriorityHigh))
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Verify the stored priority (internal to memoryCache).
	c.mu.RLock()
	se, ok := c.entries["pri-key"]
	c.mu.RUnlock()
	if !ok {
		t.Fatal("entry not found in internal map")
	}
	if se.priority != PriorityHigh {
		t.Errorf("stored priority = %v, want PriorityHigh", se.priority)
	}
}
