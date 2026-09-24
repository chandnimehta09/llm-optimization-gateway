package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/alicebob/miniredis/v2"

	"github.com/llm-optimization-gateway/internal/cache"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestCache creates a Cache backed by miniredis for testing.
func newTestCache(t *testing.T, opts ...func(*Config)) (*Cache, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)

	cfg := Config{
		Mode:                 ModeStandalone,
		Addr:                 mr.Addr(),
		PoolSize:             5,
		MinIdleConns:         1,
		DialTimeout:          1 * time.Second,
		ReadTimeout:          1 * time.Second,
		WriteTimeout:         1 * time.Second,
		MaxRetries:           1,
		MinRetryBackoff:      8 * time.Millisecond,
		MaxRetryBackoff:      64 * time.Millisecond,
		DefaultTTL:           0,
		KeyPrefix:            "test:",
		EnableCompression:    false,
		CompressionThreshold: 1024,
		HealthCheckInterval:  0, // disable periodic checks in tests
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	client := goredis.NewClient(&goredis.Options{
		Addr:         mr.Addr(),
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		MaxRetries:   cfg.MaxRetries,
	})

	c := NewWithClient(client, cfg)
	return c, mr
}

// ---------------------------------------------------------------------------
// Integration tests: Set and Get
// ---------------------------------------------------------------------------

func TestRedisCache_SetAndGet(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("Hello from Redis!", 60*time.Second)
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

	if got.Response.Content != "Hello from Redis!" {
		t.Errorf("Content = %q, want %q", got.Response.Content, "Hello from Redis!")
	}
	if got.Response.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", got.Response.Model, "gpt-4")
	}
	if got.Response.Usage.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", got.Response.Usage.TotalTokens)
	}
	if got.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", got.Provider, "openai")
	}
	if got.ContextHash != "sha256:abc123" {
		t.Errorf("ContextHash = %q, want %q", got.ContextHash, "sha256:abc123")
	}
	if got.Metadata.HitCount != 1 {
		t.Errorf("HitCount = %d, want 1", got.Metadata.HitCount)
	}
}

func TestRedisCache_GetMiss(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	got, found, err := c.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if found {
		t.Error("Get() found=true for missing key")
	}
	if got != nil {
		t.Error("Get() returned non-nil for missing key")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: TTL
// ---------------------------------------------------------------------------

func TestRedisCache_TTL(t *testing.T) {
	c, mr := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("ttl test", 0) // no default TTL
	if err := c.Set(ctx, "ttl-key", entry, cache.WithTTL(60*time.Second)); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Verify the entry is retrievable.
	got, found, err := c.Get(ctx, "ttl-key")
	if err != nil || !found {
		t.Fatalf("Get() found=%v, err=%v", found, err)
	}
	if got.Metadata.TTL != 60*time.Second {
		t.Errorf("TTL = %v, want 60s", got.Metadata.TTL)
	}

	// Verify Redis has a TTL set.
	redisTTL := mr.TTL("test:ttl-key")
	if redisTTL <= 0 {
		t.Errorf("Redis TTL = %v, want > 0", redisTTL)
	}
}

func TestRedisCache_TTLExpiration(t *testing.T) {
	c, mr := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("will expire", 0)
	if err := c.Set(ctx, "expire-key", entry, cache.WithTTL(2*time.Second)); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Fast-forward time in miniredis.
	mr.FastForward(3 * time.Second)

	_, found, _ := c.Get(ctx, "expire-key")
	if found {
		t.Error("Get() found=true after TTL expiration")
	}
}

func TestRedisCache_DefaultTTL(t *testing.T) {
	c, mr := newTestCache(t, func(cfg *Config) {
		cfg.DefaultTTL = 120 * time.Second
	})
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("default ttl", 0)
	if err := c.Set(ctx, "dttl-key", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Should have a Redis TTL from default config.
	redisTTL := mr.TTL("test:dttl-key")
	if redisTTL <= 0 {
		t.Errorf("Redis TTL = %v, want > 0", redisTTL)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Delete
// ---------------------------------------------------------------------------

func TestRedisCache_Delete(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("to delete", 5*time.Minute)
	_ = c.Set(ctx, "del-key", entry)

	if err := c.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, found, _ := c.Get(ctx, "del-key")
	if found {
		t.Error("entry found after Delete()")
	}
}

func TestRedisCache_DeleteMissing(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	if err := c.Delete(ctx, "nonexistent"); err != nil {
		t.Fatalf("Delete() for missing key error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Exists
// ---------------------------------------------------------------------------

func TestRedisCache_Exists(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("exists", 5*time.Minute)
	_ = c.Set(ctx, "exists-key", entry)

	found, err := c.Exists(ctx, "exists-key")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if !found {
		t.Error("Exists() = false for existing key")
	}

	found, err = c.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if found {
		t.Error("Exists() = true for missing key")
	}
}

func TestRedisCache_ExistsAfterExpiration(t *testing.T) {
	c, mr := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("will expire", 0)
	_ = c.Set(ctx, "expire-exists", entry, cache.WithTTL(1*time.Second))

	mr.FastForward(2 * time.Second)

	found, err := c.Exists(ctx, "expire-exists")
	if err != nil {
		t.Fatalf("Exists() error: %v", err)
	}
	if found {
		t.Error("Exists() = true after TTL expiration")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Connection failures
// ---------------------------------------------------------------------------

func TestRedisCache_ConnectionFailure_Get(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	// Store an entry first.
	entry := newTestEntry("before failure", 5*time.Minute)
	_ = c.Set(ctx, "key1", entry)

	// Shut down miniredis to simulate connection failure.
	mr.Close()

	_, _, err := c.Get(ctx, "key1")
	if err == nil {
		t.Fatal("Get() should return error when Redis is down")
	}

	// Verify the error indicates a connection problem.
	if !strings.Contains(err.Error(), "redis get") {
		t.Errorf("error should mention redis get, got: %v", err)
	}
}

func TestRedisCache_ConnectionFailure_Set(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	mr.Close()

	entry := newTestEntry("after failure", 5*time.Minute)
	err := c.Set(ctx, "key1", entry)
	if err == nil {
		t.Fatal("Set() should return error when Redis is down")
	}
}

func TestRedisCache_ConnectionFailure_Delete(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	mr.Close()

	err := c.Delete(ctx, "key1")
	if err == nil {
		t.Fatal("Delete() should return error when Redis is down")
	}
}

func TestRedisCache_ConnectionFailure_Exists(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	mr.Close()

	_, err := c.Exists(ctx, "key1")
	if err == nil {
		t.Fatal("Exists() should return error when Redis is down")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Compression
// ---------------------------------------------------------------------------

func TestRedisCache_Compression(t *testing.T) {
	c, _ := newTestCache(t, func(cfg *Config) {
		cfg.EnableCompression = true
		cfg.CompressionThreshold = 1024
	})
	defer c.Close()
	ctx := context.Background()

	// Create a large entry that will trigger compression.
	entry := newTestEntry(largeContent(2048), 5*time.Minute)
	if err := c.Set(ctx, "compressed-key", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, err := c.Get(ctx, "compressed-key")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !found {
		t.Fatal("Get() found=false")
	}

	// Verify content was preserved through compression round trip.
	if got.Response.Content != entry.Response.Content {
		t.Errorf("Content length = %d, want %d", len(got.Response.Content), len(entry.Response.Content))
	}
	if got.Provider != entry.Provider {
		t.Errorf("Provider = %q, want %q", got.Provider, entry.Provider)
	}
	if got.Metadata.TTL != entry.Metadata.TTL {
		t.Errorf("TTL = %v, want %v", got.Metadata.TTL, entry.Metadata.TTL)
	}
}

func TestRedisCache_CompressionSmallEntry(t *testing.T) {
	c, _ := newTestCache(t, func(cfg *Config) {
		cfg.EnableCompression = true
		cfg.CompressionThreshold = 1024
	})
	defer c.Close()
	ctx := context.Background()

	// Small entry should not be compressed.
	entry := newTestEntry("small", 5*time.Minute)
	if err := c.Set(ctx, "small-key", entry); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, err := c.Get(ctx, "small-key")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !found {
		t.Fatal("Get() found=false")
	}
	if got.Response.Content != "small" {
		t.Errorf("Content = %q, want %q", got.Response.Content, "small")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Tags
// ---------------------------------------------------------------------------

func TestRedisCache_SetWithTags(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	entry := &cache.Entry{
		Response: cache.CompletionResponse{Content: "tagged"},
		Metadata: cache.EntryMetadata{CreatedAt: time.Now(), TTL: 5 * time.Minute},
		Provider: "openai",
		Model:    "gpt-4",
	}

	if err := c.Set(ctx, "tagged-key", entry, cache.WithTags("env:prod", "team:ml")); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, err := c.Get(ctx, "tagged-key")
	if err != nil || !found {
		t.Fatalf("Get() found=%v, err=%v", found, err)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags length = %d, want 2", len(got.Tags))
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Health check
// ---------------------------------------------------------------------------

func TestRedisCache_HealthCheck(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	report := c.CheckHealth(ctx)
	if report.Status != HealthStatusUp {
		t.Errorf("Status = %v, want %v", report.Status, HealthStatusUp)
	}
	if report.Message != "ok" {
		t.Errorf("Message = %q, want %q", report.Message, "ok")
	}
	if report.Latency <= 0 {
		t.Errorf("Latency = %v, want > 0", report.Latency)
	}
}

func TestRedisCache_HealthCheckDown(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	mr.Close()

	report := c.CheckHealth(ctx)
	if report.Status != HealthStatusDown {
		t.Errorf("Status = %v, want %v", report.Status, HealthStatusDown)
	}
	if !strings.Contains(report.Message, "ping failed") {
		t.Errorf("Message = %q, want to contain 'ping failed'", report.Message)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Close
// ---------------------------------------------------------------------------

func TestRedisCache_Close(t *testing.T) {
	c, _ := newTestCache(t)

	if err := c.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Key prefix
// ---------------------------------------------------------------------------

func TestRedisCache_KeyPrefix(t *testing.T) {
	c, mr := newTestCache(t, func(cfg *Config) {
		cfg.KeyPrefix = "myapp:"
	})
	defer c.Close()
	ctx := context.Background()

	entry := newTestEntry("prefixed", 5*time.Minute)
	_ = c.Set(ctx, "pk", entry)

	// Verify the key in Redis has the prefix.
	if !mr.Exists("myapp:pk") {
		t.Error("key not found with prefix 'myapp:' in Redis")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Nil entry
// ---------------------------------------------------------------------------

func TestRedisCache_SetNilEntry(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	err := c.Set(ctx, "nil-key", nil)
	if err == nil {
		t.Error("Set(nil) should return error")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Corrupted data
// ---------------------------------------------------------------------------

func TestRedisCache_CorruptedData(t *testing.T) {
	c, mr := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	// Write corrupt data directly to Redis.
	mr.Set("test:corrupt", "not valid json at all")

	_, _, err := c.Get(ctx, "corrupt")
	if err == nil {
		t.Fatal("Get() should return error for corrupted data")
	}

	var corruptErr *cache.CacheCorruptedError
	if !errors.As(err, &corruptErr) {
		t.Fatalf("error type = %T, want *CacheCorruptedError", err)
	}
	if corruptErr.Key != "corrupt" {
		t.Errorf("CacheCorruptedError.Key = %q, want %q", corruptErr.Key, "corrupt")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Overwrite existing entry
// ---------------------------------------------------------------------------

func TestRedisCache_Overwrite(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	entry1 := newTestEntry("first", 5*time.Minute)
	_ = c.Set(ctx, "ow-key", entry1)

	entry2 := newTestEntry("second", 5*time.Minute)
	if err := c.Set(ctx, "ow-key", entry2); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	got, found, err := c.Get(ctx, "ow-key")
	if err != nil || !found {
		t.Fatalf("Get() found=%v, err=%v", found, err)
	}
	if got.Response.Content != "second" {
		t.Errorf("Content = %q, want %q", got.Response.Content, "second")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Context cancellation
// ---------------------------------------------------------------------------

func TestRedisCache_ContextCancellation(t *testing.T) {
	c, _ := newTestCache(t)
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	entry := newTestEntry("cancelled", 5*time.Minute)
	err := c.Set(ctx, "cancel-key", entry)
	if err == nil {
		t.Error("Set() with cancelled context should return error")
	}
}

// ---------------------------------------------------------------------------
// Acceptance Criteria Tests
// ---------------------------------------------------------------------------

func TestAcceptance_SetWithTTLAndGet(t *testing.T) {
	// Given Redis cache is configured
	c, mr := newTestCache(t)
	defer c.Close()
	ctx := context.Background()

	// When I Set an entry with TTL=60s
	entry := newTestEntry("acceptance test", 0)
	err := c.Set(ctx, "accept-key", entry, cache.WithTTL(60*time.Second))
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Then entry exists in Redis with correct TTL
	redisTTL := mr.TTL("test:accept-key")
	if redisTTL <= 0 || redisTTL > 60*time.Second {
		t.Errorf("Redis TTL = %v, want > 0 and <= 60s", redisTTL)
	}

	// And I can Get the entry
	got, found, err := c.Get(ctx, "accept-key")
	if err != nil || !found {
		t.Fatalf("Get() found=%v, err=%v", found, err)
	}
	if got.Response.Content != "acceptance test" {
		t.Errorf("Content = %q, want %q", got.Response.Content, "acceptance test")
	}
}

func TestAcceptance_RedisUnavailable(t *testing.T) {
	// Given Redis is unavailable
	c, mr := newTestCache(t)
	ctx := context.Background()

	mr.Close()

	// When I call Get()
	_, _, err := c.Get(ctx, "any-key")

	// Then error is returned
	if err == nil {
		t.Fatal("Get() should return error when Redis is unavailable")
	}

	// And error indicates connection failure
	errMsg := err.Error()
	if !strings.Contains(errMsg, "redis get") {
		t.Errorf("error should indicate redis connection issue, got: %v", err)
	}
}

func TestAcceptance_Compression(t *testing.T) {
	// Given large entry (>1KB) when compression is enabled
	c, mr := newTestCache(t, func(cfg *Config) {
		cfg.EnableCompression = true
		cfg.CompressionThreshold = 1024
	})
	defer c.Close()
	ctx := context.Background()

	largeData := largeContent(2048)
	entry := newTestEntry(largeData, 5*time.Minute)

	// Then entry is compressed before storage
	err := c.Set(ctx, "large-key", entry)
	if err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Verify data in Redis is compressed (has gzip magic bytes).
	rawData, err := mr.Get("test:large-key")
	if err != nil {
		t.Fatalf("direct Redis Get error: %v", err)
	}
	if len(rawData) < 2 || rawData[0] != 0x1f || rawData[1] != 0x8b {
		t.Error("stored data should be gzip-compressed (missing magic bytes)")
	}

	// And decompressed on retrieval
	got, found, err := c.Get(ctx, "large-key")
	if err != nil || !found {
		t.Fatalf("Get() found=%v, err=%v", found, err)
	}
	if got.Response.Content != largeData {
		t.Errorf("Content length = %d, want %d (data was not properly decompressed)", len(got.Response.Content), len(largeData))
	}
}

// ---------------------------------------------------------------------------
// Health status string tests
// ---------------------------------------------------------------------------

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		status HealthStatus
		want   string
	}{
		{HealthStatusUp, "up"},
		{HealthStatusDown, "down"},
		{HealthStatusDegraded, "degraded"},
		{HealthStatus(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("HealthStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}
