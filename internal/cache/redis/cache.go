package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/llm-optimization-gateway/internal/cache"
)

// Cmdable is the interface satisfied by all go-redis client types
// (redis.Client, redis.ClusterClient, redis.Ring). This allows the
// Cache to work with any deployment mode.
type Cmdable interface {
	redis.Cmdable
	Close() error
	Ping(ctx context.Context) *redis.StatusCmd
}

// Cache is a Redis-backed implementation of the cache.Cache interface.
type Cache struct {
	client     Cmdable
	serializer *serializer
	config     Config
	health     *healthChecker
}

// compile-time interface check.
var _ cache.Cache = (*Cache)(nil)

// New creates a new Redis cache from the provided Config.
// It validates the config, creates the appropriate Redis client, and
// starts the health checker.
func New(cfg Config) (*Cache, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("redis cache: %w", err)
	}

	client, err := newClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("redis cache: create client: %w", err)
	}

	c := &Cache{
		client:     client,
		serializer: newSerializer(cfg.EnableCompression, cfg.CompressionThreshold),
		config:     cfg,
	}
	c.health = newHealthChecker(c, cfg.HealthCheckInterval)
	c.health.Start()

	return c, nil
}

// NewWithClient creates a new Redis cache using a pre-configured redis client.
// Useful for testing or when the caller manages the client lifecycle.
func NewWithClient(client Cmdable, cfg Config) *Cache {
	c := &Cache{
		client:     client,
		serializer: newSerializer(cfg.EnableCompression, cfg.CompressionThreshold),
		config:     cfg,
	}
	c.health = newHealthChecker(c, 0) // no periodic checks for injected clients
	return c
}

// Get retrieves a cache entry by key. Returns the entry and true if found,
// nil and false on a miss, or an error on backend failure.
func (c *Cache) Get(ctx context.Context, key string) (*cache.Entry, bool, error) {
	fullKey := c.prefixedKey(key)

	data, err := c.client.Get(ctx, fullKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("redis get %q: %w", key, err)
	}

	entry, err := c.serializer.Unmarshal(data)
	if err != nil {
		return nil, false, &cache.CacheCorruptedError{
			Key:    key,
			Reason: err.Error(),
		}
	}

	// Check expiration at the application level as well, in case the entry
	// was stored without a Redis TTL (e.g., TTL=0 means no expiration in Redis
	// but the entry may still have an application-level TTL set).
	if entry.IsExpired() {
		// Clean up the expired entry.
		_ = c.client.Del(ctx, fullKey)
		return nil, false, &cache.CacheExpiredError{
			Key:       key,
			ExpiredAt: entry.ExpiresAt().Format(time.RFC3339),
		}
	}

	entry.IncrementHitCount()

	// Write back the updated hit count. Best-effort; don't fail the Get.
	if updated, err := c.serializer.Marshal(entry); err == nil {
		ttl, ttlErr := c.client.TTL(ctx, fullKey).Result()
		if ttlErr == nil && ttl > 0 {
			_ = c.client.Set(ctx, fullKey, updated, ttl)
		} else if ttlErr == nil {
			// No TTL set or key has no expiry — set without expiry.
			_ = c.client.Set(ctx, fullKey, updated, 0)
		}
	}

	return entry, true, nil
}

// Set stores a cache entry with optional TTL, tags, and priority overrides.
func (c *Cache) Set(ctx context.Context, key string, entry *cache.Entry, opts ...cache.Option) error {
	if entry == nil {
		return fmt.Errorf("redis set: entry must not be nil")
	}

	resolved := cache.ResolveOptions(opts...)

	// Apply option overrides to the entry.
	if resolved.TTL > 0 {
		entry.Metadata.TTL = resolved.TTL
	}
	if len(resolved.Tags) > 0 {
		entry.Tags = append(entry.Tags, resolved.Tags...)
	}

	data, err := c.serializer.Marshal(entry)
	if err != nil {
		return fmt.Errorf("redis set %q: marshal: %w", key, err)
	}

	fullKey := c.prefixedKey(key)

	// Determine the Redis TTL. Prefer the resolved option TTL, then
	// the entry metadata TTL, then the default config TTL.
	ttl := resolved.TTL
	if ttl == 0 {
		ttl = entry.Metadata.TTL
	}
	if ttl == 0 {
		ttl = c.config.DefaultTTL
	}

	if err := c.client.Set(ctx, fullKey, data, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %q: %w", key, err)
	}

	return nil
}

// Delete removes a cache entry by key. Returns nil if the key does not exist.
func (c *Cache) Delete(ctx context.Context, key string) error {
	fullKey := c.prefixedKey(key)

	if err := c.client.Del(ctx, fullKey).Err(); err != nil {
		return fmt.Errorf("redis delete %q: %w", key, err)
	}

	return nil
}

// Exists checks whether a key is present in the cache. Returns false for
// expired entries.
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := c.prefixedKey(key)

	n, err := c.client.Exists(ctx, fullKey).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists %q: %w", key, err)
	}

	return n > 0, nil
}

// Close releases resources held by the cache. It stops the health checker
// and closes the Redis client connection pool.
func (c *Cache) Close() error {
	c.health.Stop()
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("redis close: %w", err)
	}
	return nil
}

// Health returns the most recent health check report.
func (c *Cache) Health() HealthReport {
	return c.health.LastReport()
}

// CheckHealth performs an immediate health check and returns the result.
func (c *Cache) CheckHealth(ctx context.Context) HealthReport {
	return c.health.Check(ctx)
}

// prefixedKey prepends the configured key prefix to the given key.
func (c *Cache) prefixedKey(key string) string {
	return c.config.KeyPrefix + key
}

// newClient creates the appropriate go-redis client based on the config mode.
func newClient(cfg Config) (Cmdable, error) {
	switch cfg.Mode {
	case ModeStandalone:
		return redis.NewClient(&redis.Options{
			Addr:            cfg.Addr,
			Password:        cfg.Password,
			DB:              cfg.DB,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			MaxRetries:      cfg.MaxRetries,
			MinRetryBackoff: cfg.MinRetryBackoff,
			MaxRetryBackoff: cfg.MaxRetryBackoff,
			TLSConfig:       cfg.TLSConfig,
		}), nil

	case ModeCluster:
		return redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:           cfg.addrs(),
			Password:        cfg.Password,
			PoolSize:        cfg.PoolSize,
			MinIdleConns:    cfg.MinIdleConns,
			DialTimeout:     cfg.DialTimeout,
			ReadTimeout:     cfg.ReadTimeout,
			WriteTimeout:    cfg.WriteTimeout,
			MaxRetries:      cfg.MaxRetries,
			MinRetryBackoff: cfg.MinRetryBackoff,
			MaxRetryBackoff: cfg.MaxRetryBackoff,
			TLSConfig:       cfg.TLSConfig,
		}), nil

	case ModeSentinel:
		return redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:       cfg.SentinelMasterName,
			SentinelAddrs:    cfg.addrs(),
			SentinelPassword: "",
			Password:         cfg.Password,
			DB:               cfg.DB,
			PoolSize:         cfg.PoolSize,
			MinIdleConns:     cfg.MinIdleConns,
			DialTimeout:      cfg.DialTimeout,
			ReadTimeout:      cfg.ReadTimeout,
			WriteTimeout:     cfg.WriteTimeout,
			MaxRetries:       cfg.MaxRetries,
			MinRetryBackoff:  cfg.MinRetryBackoff,
			MaxRetryBackoff:  cfg.MaxRetryBackoff,
			TLSConfig:        cfg.TLSConfig,
		}), nil

	default:
		return nil, fmt.Errorf("unsupported mode: %d", cfg.Mode)
	}
}
