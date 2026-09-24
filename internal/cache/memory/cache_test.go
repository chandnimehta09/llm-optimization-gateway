package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/cache"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestCache(t *testing.T, cfg Config) *Cache {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func testEntry(content string, ttl time.Duration) *cache.Entry {
	return &cache.Entry{
		Response: cache.CompletionResponse{
			ID:           "resp-001",
			Model:        "gpt-4",
			Content:      content,
			FinishReason: "stop",
			Usage: cache.UsageInfo{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       ttl,
			Tokens:    150,
			Cost:      0.0045,
		},
		ContextHash: "sha256:abc123",
		Provider:    "openai",
		Model:       "gpt-4",
	}
}

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxSize != 1000 {
		t.Errorf("MaxSize = %d, want 1000", cfg.MaxSize)
	}
	if cfg.DefaultTTL != 5*time.Minute {
		t.Errorf("DefaultTTL = %v, want 5m", cfg.DefaultTTL)
	}
	if cfg.CleanupInterval != 1*time.Minute {
		t.Errorf("CleanupInterval = %v, want 1m", cfg.CleanupInterval)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{"valid defaults", DefaultConfig(), false},
		{"zero values", Config{}, false},
		{"negative MaxSize", Config{MaxSize: -1}, true},
		{"negative DefaultTTL", Config{DefaultTTL: -1}, true},
		{"negative CleanupInterval", Config{CleanupInterval: -1}, true},
		{"unlimited size", Config{MaxSize: 0, DefaultTTL: time.Minute}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				var cfgErr *ConfigError
				if !errors.As(err, &cfgErr) {
					t.Errorf("error type = %T, want *ConfigError", err)
				}
			}
		})
	}
}

func TestConfigError_Error(t *testing.T) {
	err := &ConfigError{Field: "MaxSize", Reason: "must be >= 0"}
	want := "memory cache config: MaxSize: must be >= 0"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func TestNew_InvalidConfig(t *testing.T) {
	_, err := New(Config{MaxSize: -1})
	if err == nil {
		t.Fatal("New() expected error for invalid config")
	}
}

func TestNew_NoCleanupGoroutine(t *testing.T) {
	c, err := New(Config{CleanupInterval: 0})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer c.Close()
	// cleanupDone should already be closed when CleanupInterval is 0.
	select {
	case <-c.cleanupDone:
		// ok
	default:
		t.Error("cleanupDone not closed when CleanupInterval=0")
	}
}

// ---------------------------------------------------------------------------
// Get / Set / Delete unit tests
// ---------------------------------------------------------------------------

func TestCache_SetAndGet(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry := testEntry("Hello, world!", 5*time.Minute)
	if err := c.Set(ctx, "key1", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, err := c.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !found {
		t.Fatal("Get() found=false, want true")
	}
	if got.Response.Content != "Hello, world!" {
		t.Errorf("Content = %q, want %q", got.Response.Content, "Hello, world!")
	}
	if got.Metadata.HitCount != 1 {
		t.Errorf("HitCount = %d, want 1", got.Metadata.HitCount)
	}
}

func TestCache_GetMiss(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	got, found, err := c.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if found {
		t.Error("Get() found=true for missing key")
	}
	if got != nil {
		t.Error("Get() returned non-nil entry for missing key")
	}
}

func TestCache_Delete(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry := testEntry("to delete", 5*time.Minute)
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
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	if err := c.Delete(ctx, "no-such-key"); err != nil {
		t.Fatalf("Delete() error for missing key: %v", err)
	}
}

func TestCache_Exists(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry := testEntry("exists test", 5*time.Minute)
	_ = c.Set(ctx, "exists-key", entry)

	exists, err := c.Exists(ctx, "exists-key")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !exists {
		t.Error("Exists() returned false for existing key")
	}

	exists, err = c.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if exists {
		t.Error("Exists() returned true for missing key")
	}
}

func TestCache_ExistsExpired(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry := &cache.Entry{
		Response: cache.CompletionResponse{Content: "expired"},
		Metadata: cache.EntryMetadata{
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

func TestCache_SetOverwrite(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry1 := testEntry("first", 5*time.Minute)
	_ = c.Set(ctx, "key", entry1)

	entry2 := testEntry("second", 5*time.Minute)
	_ = c.Set(ctx, "key", entry2)

	got, found, err := c.Get(ctx, "key")
	if err != nil || !found {
		t.Fatalf("Get() found=%v, error=%v", found, err)
	}
	if got.Response.Content != "second" {
		t.Errorf("Content = %q, want %q", got.Response.Content, "second")
	}

	// Overwriting should not increase the entry count.
	stats := c.Stats()
	if stats.Size != 1 {
		t.Errorf("Size = %d, want 1", stats.Size)
	}
}

func TestCache_SetWithOptions(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry := &cache.Entry{
		Response: cache.CompletionResponse{Content: "opts"},
		Metadata: cache.EntryMetadata{CreatedAt: time.Now()},
		Provider: "openai",
		Model:    "gpt-4",
	}

	err := c.Set(ctx, "opt-key", entry,
		cache.WithTTL(30*time.Second),
		cache.WithTags("env:test"),
		cache.WithPriority(cache.PriorityHigh),
	)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, _ := c.Get(ctx, "opt-key")
	if !found {
		t.Fatal("entry not found")
	}
	if got.Metadata.TTL != 30*time.Second {
		t.Errorf("TTL = %v, want 30s", got.Metadata.TTL)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "env:test" {
		t.Errorf("Tags = %v, want [env:test]", got.Tags)
	}
}

func TestCache_DefaultTTLApplied(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, DefaultTTL: 10 * time.Minute, CleanupInterval: 0})
	ctx := context.Background()

	entry := &cache.Entry{
		Response: cache.CompletionResponse{Content: "default ttl"},
		Metadata: cache.EntryMetadata{CreatedAt: time.Now()},
		Provider: "openai",
		Model:    "gpt-4",
	}

	_ = c.Set(ctx, "def-ttl", entry)

	got, found, _ := c.Get(ctx, "def-ttl")
	if !found {
		t.Fatal("entry not found")
	}
	if got.Metadata.TTL != 10*time.Minute {
		t.Errorf("TTL = %v, want 10m", got.Metadata.TTL)
	}
}

// ---------------------------------------------------------------------------
// TTL expiration tests
// ---------------------------------------------------------------------------

func TestCache_TTLExpiration(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	// Create an entry that is already expired by setting CreatedAt in the past.
	entry := &cache.Entry{
		Response: cache.CompletionResponse{Content: "will expire"},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now().Add(-2 * time.Second),
			TTL:       1 * time.Second,
		},
		Provider: "openai",
		Model:    "gpt-4",
	}
	_ = c.Set(ctx, "ttl-key", entry)

	_, found, err := c.Get(ctx, "ttl-key")
	if found {
		t.Error("Get() found=true for expired entry")
	}

	var expiredErr *cache.CacheExpiredError
	if !errors.As(err, &expiredErr) {
		t.Fatalf("error type = %T, want *CacheExpiredError", err)
	}
	if expiredErr.Key != "ttl-key" {
		t.Errorf("Key = %q, want %q", expiredErr.Key, "ttl-key")
	}

	// The entry should have been removed.
	exists, _ := c.Exists(ctx, "ttl-key")
	if exists {
		t.Error("expired entry still exists after Get()")
	}
}

func TestCache_TTLExpiration_RealTime(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping time-based test in short mode")
	}

	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry := &cache.Entry{
		Response: cache.CompletionResponse{Content: "short lived"},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       100 * time.Millisecond,
		},
		Provider: "openai",
		Model:    "gpt-4",
	}
	_ = c.Set(ctx, "short-ttl", entry)

	// Should be present immediately.
	_, found, _ := c.Get(ctx, "short-ttl")
	if !found {
		t.Fatal("entry not found immediately after Set()")
	}

	// Wait for expiration.
	time.Sleep(150 * time.Millisecond)

	_, found, err := c.Get(ctx, "short-ttl")
	if found {
		t.Error("entry still found after TTL expired")
	}
	if !cache.IsCacheExpired(err) {
		t.Errorf("expected CacheExpiredError, got %v", err)
	}
}

func TestCache_BackgroundCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping time-based test in short mode")
	}

	c := newTestCache(t, Config{
		MaxSize:         10,
		CleanupInterval: 50 * time.Millisecond,
	})
	ctx := context.Background()

	entry := &cache.Entry{
		Response: cache.CompletionResponse{Content: "cleanup me"},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       50 * time.Millisecond,
		},
		Provider: "openai",
		Model:    "gpt-4",
	}
	_ = c.Set(ctx, "cleanup-key", entry)

	// Wait for expiration + cleanup cycle.
	time.Sleep(200 * time.Millisecond)

	// The entry should have been cleaned up in the background.
	c.mu.RLock()
	_, exists := c.entries["cleanup-key"]
	c.mu.RUnlock()
	if exists {
		t.Error("expired entry was not cleaned up by background goroutine")
	}
}

// ---------------------------------------------------------------------------
// LRU eviction tests
// ---------------------------------------------------------------------------

func TestCache_LRUEviction(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 3, CleanupInterval: 0})
	ctx := context.Background()

	// Set 3 entries.
	for i := 1; i <= 3; i++ {
		entry := testEntry(fmt.Sprintf("entry-%d", i), 5*time.Minute)
		_ = c.Set(ctx, fmt.Sprintf("key%d", i), entry)
	}

	// All 3 should be present.
	for i := 1; i <= 3; i++ {
		exists, _ := c.Exists(ctx, fmt.Sprintf("key%d", i))
		if !exists {
			t.Errorf("key%d not found, want present", i)
		}
	}

	// Set a 4th entry - should evict key1 (LRU).
	entry4 := testEntry("entry-4", 5*time.Minute)
	_ = c.Set(ctx, "key4", entry4)

	// key1 should be evicted (it was least recently used).
	exists, _ := c.Exists(ctx, "key1")
	if exists {
		t.Error("key1 should have been evicted")
	}

	// key2, key3, key4 should still be present.
	for _, k := range []string{"key2", "key3", "key4"} {
		exists, _ := c.Exists(ctx, k)
		if !exists {
			t.Errorf("%s not found, want present", k)
		}
	}

	stats := c.Stats()
	if stats.Evictions != 1 {
		t.Errorf("Evictions = %d, want 1", stats.Evictions)
	}
}

func TestCache_LRUEviction_AccessReorder(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 3, CleanupInterval: 0})
	ctx := context.Background()

	// Set 3 entries: key1, key2, key3.
	for i := 1; i <= 3; i++ {
		entry := testEntry(fmt.Sprintf("entry-%d", i), 5*time.Minute)
		_ = c.Set(ctx, fmt.Sprintf("key%d", i), entry)
	}

	// Access key1 to make it most recently used.
	_, _, _ = c.Get(ctx, "key1")

	// Set key4 - should evict key2 (now the LRU since key1 was touched).
	entry4 := testEntry("entry-4", 5*time.Minute)
	_ = c.Set(ctx, "key4", entry4)

	// key2 should be evicted.
	exists, _ := c.Exists(ctx, "key2")
	if exists {
		t.Error("key2 should have been evicted (LRU after key1 was accessed)")
	}

	// key1 should still be present.
	exists, _ = c.Exists(ctx, "key1")
	if !exists {
		t.Error("key1 should still be present (was recently accessed)")
	}
}

func TestCache_LRUEviction_MultipleEvictions(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 2, CleanupInterval: 0})
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		entry := testEntry(fmt.Sprintf("entry-%d", i), 5*time.Minute)
		_ = c.Set(ctx, fmt.Sprintf("key%d", i), entry)
	}

	// Only key4 and key5 should remain.
	stats := c.Stats()
	if stats.Size != 2 {
		t.Errorf("Size = %d, want 2", stats.Size)
	}
	if stats.Evictions != 3 {
		t.Errorf("Evictions = %d, want 3", stats.Evictions)
	}

	for _, k := range []string{"key4", "key5"} {
		exists, _ := c.Exists(ctx, k)
		if !exists {
			t.Errorf("%s not found, want present", k)
		}
	}
}

func TestCache_UnlimitedSize(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 0, CleanupInterval: 0})
	ctx := context.Background()

	// Should not evict when MaxSize is 0 (unlimited).
	for i := 0; i < 100; i++ {
		entry := testEntry(fmt.Sprintf("entry-%d", i), 5*time.Minute)
		_ = c.Set(ctx, fmt.Sprintf("key%d", i), entry)
	}

	stats := c.Stats()
	if stats.Size != 100 {
		t.Errorf("Size = %d, want 100", stats.Size)
	}
	if stats.Evictions != 0 {
		t.Errorf("Evictions = %d, want 0", stats.Evictions)
	}
}

// ---------------------------------------------------------------------------
// Concurrent access tests
// ---------------------------------------------------------------------------

func TestCache_ConcurrentAccess(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 100, CleanupInterval: 0})
	ctx := context.Background()

	const goroutines = 50
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := fmt.Sprintf("key-%d-%d", id, i)
				entry := testEntry(fmt.Sprintf("value-%d-%d", id, i), 5*time.Minute)

				// Set
				if err := c.Set(ctx, key, entry); err != nil {
					t.Errorf("goroutine %d: Set() error: %v", id, err)
					return
				}

				// Get
				_, _, err := c.Get(ctx, key)
				if err != nil && !cache.IsCacheExpired(err) {
					t.Errorf("goroutine %d: Get() error: %v", id, err)
					return
				}

				// Exists
				_, err = c.Exists(ctx, key)
				if err != nil {
					t.Errorf("goroutine %d: Exists() error: %v", id, err)
					return
				}

				// Delete every other entry
				if i%2 == 0 {
					if err := c.Delete(ctx, key); err != nil {
						t.Errorf("goroutine %d: Delete() error: %v", id, err)
						return
					}
				}
			}
		}(g)
	}

	wg.Wait()
}

func TestCache_ConcurrentReadWrite(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 50, CleanupInterval: 0})
	ctx := context.Background()

	// Pre-populate.
	for i := 0; i < 10; i++ {
		entry := testEntry(fmt.Sprintf("init-%d", i), 5*time.Minute)
		_ = c.Set(ctx, fmt.Sprintf("shared-%d", i), entry)
	}

	var wg sync.WaitGroup
	const readers = 20
	const writers = 10

	// Readers.
	wg.Add(readers)
	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("shared-%d", i%10)
				c.Get(ctx, key)
				c.Exists(ctx, key)
			}
		}()
	}

	// Writers.
	wg.Add(writers)
	for w := 0; w < writers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				key := fmt.Sprintf("shared-%d", i%10)
				entry := testEntry(fmt.Sprintf("update-%d-%d", id, i), 5*time.Minute)
				c.Set(ctx, key, entry)
			}
		}(w)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Statistics tests
// ---------------------------------------------------------------------------

func TestCache_Stats(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry := testEntry("stats test", 5*time.Minute)
	_ = c.Set(ctx, "s1", entry)

	// Hit.
	_, _, _ = c.Get(ctx, "s1")
	// Miss.
	_, _, _ = c.Get(ctx, "missing")

	stats := c.Stats()
	if stats.Hits != 1 {
		t.Errorf("Hits = %d, want 1", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Misses = %d, want 1", stats.Misses)
	}
	if stats.Size != 1 {
		t.Errorf("Size = %d, want 1", stats.Size)
	}
}

func TestCache_StatsEvictions(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 2, CleanupInterval: 0})
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		entry := testEntry(fmt.Sprintf("e%d", i), 5*time.Minute)
		_ = c.Set(ctx, fmt.Sprintf("k%d", i), entry)
	}

	stats := c.Stats()
	if stats.Evictions != 2 {
		t.Errorf("Evictions = %d, want 2", stats.Evictions)
	}
	if stats.Size != 2 {
		t.Errorf("Size = %d, want 2", stats.Size)
	}
}

func TestCache_StatsExpirations(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry := &cache.Entry{
		Response: cache.CompletionResponse{Content: "expired"},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now().Add(-2 * time.Hour),
			TTL:       1 * time.Hour,
		},
		Provider: "openai",
		Model:    "gpt-4",
	}
	_ = c.Set(ctx, "exp-key", entry)

	// Trigger expiration via Get.
	_, _, _ = c.Get(ctx, "exp-key")

	stats := c.Stats()
	if stats.Expirations != 1 {
		t.Errorf("Expirations = %d, want 1", stats.Expirations)
	}
}

func TestCache_StatsOldestNewest(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	now := time.Now()
	entry1 := &cache.Entry{
		Response: cache.CompletionResponse{Content: "old"},
		Metadata: cache.EntryMetadata{CreatedAt: now.Add(-1 * time.Hour)},
		Provider: "openai",
		Model:    "gpt-4",
	}
	entry2 := &cache.Entry{
		Response: cache.CompletionResponse{Content: "new"},
		Metadata: cache.EntryMetadata{CreatedAt: now},
		Provider: "openai",
		Model:    "gpt-4",
	}
	_ = c.Set(ctx, "old", entry1)
	_ = c.Set(ctx, "new", entry2)

	stats := c.Stats()
	if stats.OldestEntry.After(stats.NewestEntry) {
		t.Error("OldestEntry is after NewestEntry")
	}
}

func TestCache_ResetStats(t *testing.T) {
	c := newTestCache(t, Config{MaxSize: 10, CleanupInterval: 0})
	ctx := context.Background()

	entry := testEntry("reset", 5*time.Minute)
	_ = c.Set(ctx, "r1", entry)
	_, _, _ = c.Get(ctx, "r1")
	_, _, _ = c.Get(ctx, "missing")

	c.ResetStats()
	stats := c.Stats()

	if stats.Hits != 0 {
		t.Errorf("Hits = %d after reset, want 0", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("Misses = %d after reset, want 0", stats.Misses)
	}
	if stats.Evictions != 0 {
		t.Errorf("Evictions = %d after reset, want 0", stats.Evictions)
	}
	if stats.Expirations != 0 {
		t.Errorf("Expirations = %d after reset, want 0", stats.Expirations)
	}
}

// ---------------------------------------------------------------------------
// Close / lifecycle tests
// ---------------------------------------------------------------------------

func TestCache_Close(t *testing.T) {
	c, err := New(Config{MaxSize: 10, CleanupInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	// Entries should be nil after close.
	c.mu.RLock()
	isNil := c.entries == nil
	c.mu.RUnlock()
	if !isNil {
		t.Error("entries not nil after Close()")
	}
}

func TestCache_CloseIdempotent(t *testing.T) {
	c, err := New(Config{MaxSize: 10, CleanupInterval: 50 * time.Millisecond})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("first Close() error: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close() error: %v", err)
	}
}

func TestCache_Lifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping lifecycle test in short mode")
	}

	// Full lifecycle: create -> use -> close.
	c, err := New(Config{
		MaxSize:         5,
		DefaultTTL:      200 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	ctx := context.Background()

	// Phase 1: populate.
	for i := 0; i < 5; i++ {
		entry := testEntry(fmt.Sprintf("lifecycle-%d", i), 200*time.Millisecond)
		_ = c.Set(ctx, fmt.Sprintf("lc%d", i), entry)
	}

	stats := c.Stats()
	if stats.Size != 5 {
		t.Errorf("Phase 1: Size = %d, want 5", stats.Size)
	}

	// Phase 2: cause eviction.
	entry := testEntry("overflow", 200*time.Millisecond)
	_ = c.Set(ctx, "lc-overflow", entry)
	stats = c.Stats()
	if stats.Evictions < 1 {
		t.Errorf("Phase 2: Evictions = %d, want >= 1", stats.Evictions)
	}

	// Phase 3: wait for TTL expiration.
	time.Sleep(300 * time.Millisecond)

	stats = c.Stats()
	if stats.Size != 0 {
		// Background cleanup should have removed expired entries.
		t.Errorf("Phase 3: Size = %d after TTL, want 0", stats.Size)
	}

	// Phase 4: shutdown.
	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// LRU tracker unit tests
// ---------------------------------------------------------------------------

func TestLRUTracker_TouchAndOldest(t *testing.T) {
	l := newLRUTracker()

	l.Touch("a")
	l.Touch("b")
	l.Touch("c")

	if l.Oldest() != "a" {
		t.Errorf("Oldest() = %q, want %q", l.Oldest(), "a")
	}

	// Touch "a" to move it to front.
	l.Touch("a")
	if l.Oldest() != "b" {
		t.Errorf("Oldest() = %q, want %q after Touch(a)", l.Oldest(), "b")
	}
}

func TestLRUTracker_Remove(t *testing.T) {
	l := newLRUTracker()

	l.Touch("a")
	l.Touch("b")
	l.Touch("c")

	l.Remove("a")
	if l.Len() != 2 {
		t.Errorf("Len() = %d, want 2", l.Len())
	}
	if l.Oldest() != "b" {
		t.Errorf("Oldest() = %q, want %q", l.Oldest(), "b")
	}
}

func TestLRUTracker_RemoveNonexistent(t *testing.T) {
	l := newLRUTracker()
	l.Touch("a")
	l.Remove("nonexistent") // Should not panic.
	if l.Len() != 1 {
		t.Errorf("Len() = %d, want 1", l.Len())
	}
}

func TestLRUTracker_OldestEmpty(t *testing.T) {
	l := newLRUTracker()
	if l.Oldest() != "" {
		t.Errorf("Oldest() = %q, want empty string", l.Oldest())
	}
}

func TestLRUTracker_Len(t *testing.T) {
	l := newLRUTracker()
	if l.Len() != 0 {
		t.Errorf("Len() = %d, want 0", l.Len())
	}
	l.Touch("a")
	l.Touch("b")
	if l.Len() != 2 {
		t.Errorf("Len() = %d, want 2", l.Len())
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkCache_Set(b *testing.B) {
	c, _ := New(Config{MaxSize: 10000, CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := &cache.Entry{
			Response: cache.CompletionResponse{Content: "bench"},
			Metadata: cache.EntryMetadata{CreatedAt: time.Now(), TTL: 5 * time.Minute},
			Provider: "openai",
			Model:    "gpt-4",
		}
		c.Set(ctx, fmt.Sprintf("bench-%d", i), entry)
	}
}

func BenchmarkCache_Get_Hit(b *testing.B) {
	c, _ := New(Config{MaxSize: 10000, CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	entry := &cache.Entry{
		Response: cache.CompletionResponse{Content: "bench"},
		Metadata: cache.EntryMetadata{CreatedAt: time.Now(), TTL: 5 * time.Minute},
		Provider: "openai",
		Model:    "gpt-4",
	}
	c.Set(ctx, "bench-key", entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(ctx, "bench-key")
	}
}

func BenchmarkCache_Get_Miss(b *testing.B) {
	c, _ := New(Config{MaxSize: 10000, CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(ctx, "nonexistent")
	}
}

func BenchmarkCache_SetWithEviction(b *testing.B) {
	c, _ := New(Config{MaxSize: 100, CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := &cache.Entry{
			Response: cache.CompletionResponse{Content: "bench"},
			Metadata: cache.EntryMetadata{CreatedAt: time.Now(), TTL: 5 * time.Minute},
			Provider: "openai",
			Model:    "gpt-4",
		}
		c.Set(ctx, fmt.Sprintf("bench-%d", i), entry)
	}
}

func BenchmarkCache_ConcurrentMixed(b *testing.B) {
	c, _ := New(Config{MaxSize: 1000, CleanupInterval: 0})
	defer c.Close()
	ctx := context.Background()

	// Pre-populate.
	for i := 0; i < 100; i++ {
		entry := &cache.Entry{
			Response: cache.CompletionResponse{Content: fmt.Sprintf("init-%d", i)},
			Metadata: cache.EntryMetadata{CreatedAt: time.Now(), TTL: 5 * time.Minute},
			Provider: "openai",
			Model:    "gpt-4",
		}
		c.Set(ctx, fmt.Sprintf("key-%d", i), entry)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%100)
			if i%3 == 0 {
				entry := &cache.Entry{
					Response: cache.CompletionResponse{Content: "update"},
					Metadata: cache.EntryMetadata{CreatedAt: time.Now(), TTL: 5 * time.Minute},
					Provider: "openai",
					Model:    "gpt-4",
				}
				c.Set(ctx, key, entry)
			} else {
				c.Get(ctx, key)
			}
			i++
		}
	})
}
