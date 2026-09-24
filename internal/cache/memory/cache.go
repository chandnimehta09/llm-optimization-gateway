package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/llm-optimization-gateway/internal/cache"
)

// Cache is an in-memory implementation of cache.Cache. It is intended for
// testing and development and should not be used in production for
// large-scale workloads. It supports:
//   - TTL-based expiration with background cleanup
//   - LRU eviction when a maximum size is reached
//   - Thread-safe concurrent access via sync.RWMutex
//   - Cache statistics collection
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*cacheItem
	lru     *lruTracker
	config  Config

	// Statistics tracked with atomics for lock-free reads.
	hits        atomic.Int64
	misses      atomic.Int64
	evictions   atomic.Int64
	expirations atomic.Int64

	// stopCleanup signals the background cleanup goroutine to stop.
	stopCleanup chan struct{}
	// cleanupDone is closed when the cleanup goroutine has exited.
	cleanupDone chan struct{}
}

// cacheItem wraps a cache.Entry with the resolved priority.
type cacheItem struct {
	entry    *cache.Entry
	priority cache.Priority
}

// New creates a new in-memory cache with the given configuration. If the
// config is invalid, New returns an error.
func New(cfg Config) (*Cache, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	c := &Cache{
		entries:     make(map[string]*cacheItem),
		lru:         newLRUTracker(),
		config:      cfg,
		stopCleanup: make(chan struct{}),
		cleanupDone: make(chan struct{}),
	}

	if cfg.CleanupInterval > 0 {
		go c.cleanupLoop()
	} else {
		close(c.cleanupDone)
	}

	return c, nil
}

// Get retrieves a cache entry by key. Returns the entry and true if found
// and not expired. Returns nil, false, CacheExpiredError for expired entries.
// Returns nil, false, nil for cache misses.
func (c *Cache) Get(_ context.Context, key string) (*cache.Entry, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.entries[key]
	if !ok {
		c.misses.Add(1)
		return nil, false, nil
	}

	if item.entry.IsExpired() {
		c.removeEntryLocked(key)
		c.expirations.Add(1)
		c.misses.Add(1)
		return nil, false, &cache.CacheExpiredError{
			Key:       key,
			ExpiredAt: item.entry.ExpiresAt().Format(time.RFC3339),
		}
	}

	item.entry.IncrementHitCount()
	c.lru.Touch(key)
	c.hits.Add(1)
	return item.entry, true, nil
}

// Set stores a cache entry. Options can override the default TTL, add tags,
// or set priority. If the cache is at capacity, the least recently used
// entry is evicted before inserting the new one.
func (c *Cache) Set(_ context.Context, key string, entry *cache.Entry, opts ...cache.Option) error {
	resolved := cache.ResolveOptions(opts...)

	// Apply TTL: per-call option > entry metadata > config default.
	if resolved.TTL > 0 {
		entry.Metadata.TTL = resolved.TTL
	} else if entry.Metadata.TTL == 0 && c.config.DefaultTTL > 0 {
		entry.Metadata.TTL = c.config.DefaultTTL
	}

	if len(resolved.Tags) > 0 {
		entry.Tags = append(entry.Tags, resolved.Tags...)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If the key already exists, update in place (no eviction needed).
	if _, exists := c.entries[key]; exists {
		c.entries[key] = &cacheItem{entry: entry, priority: resolved.Priority}
		c.lru.Touch(key)
		return nil
	}

	// Evict if at capacity.
	if c.config.MaxSize > 0 && len(c.entries) >= c.config.MaxSize {
		c.evictLocked()
	}

	c.entries[key] = &cacheItem{entry: entry, priority: resolved.Priority}
	c.lru.Touch(key)
	return nil
}

// Delete removes a cache entry by key. Returns nil if the key does not exist.
func (c *Cache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeEntryLocked(key)
	return nil
}

// Exists checks whether a key is present and not expired.
func (c *Cache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, ok := c.entries[key]
	if !ok {
		return false, nil
	}
	if item.entry.IsExpired() {
		return false, nil
	}
	return true, nil
}

// Close stops the background cleanup goroutine and releases resources.
func (c *Cache) Close() error {
	select {
	case <-c.stopCleanup:
		// Already closed.
	default:
		close(c.stopCleanup)
	}
	<-c.cleanupDone

	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
	return nil
}

// Stats returns a snapshot of the current cache statistics.
func (c *Cache) Stats() cache.Statistics {
	c.mu.RLock()
	size := int64(len(c.entries))
	var oldest, newest time.Time
	for _, item := range c.entries {
		created := item.entry.Metadata.CreatedAt
		if oldest.IsZero() || created.Before(oldest) {
			oldest = created
		}
		if newest.IsZero() || created.After(newest) {
			newest = created
		}
	}
	c.mu.RUnlock()

	return cache.Statistics{
		Hits:        c.hits.Load(),
		Misses:      c.misses.Load(),
		Evictions:   c.evictions.Load(),
		Expirations: c.expirations.Load(),
		Size:        size,
		OldestEntry: oldest,
		NewestEntry: newest,
	}
}

// ResetStats resets all statistics counters to zero.
func (c *Cache) ResetStats() {
	c.hits.Store(0)
	c.misses.Store(0)
	c.evictions.Store(0)
	c.expirations.Store(0)
}

// removeEntryLocked removes a key from both the map and the LRU tracker.
// Caller must hold c.mu (write lock).
func (c *Cache) removeEntryLocked(key string) {
	delete(c.entries, key)
	c.lru.Remove(key)
}

// evictLocked removes the least recently used entry from the cache.
// Caller must hold c.mu (write lock).
func (c *Cache) evictLocked() {
	victim := c.lru.Oldest()
	if victim == "" {
		return
	}
	c.removeEntryLocked(victim)
	c.evictions.Add(1)
}

// cleanupLoop periodically scans for and removes expired entries.
func (c *Cache) cleanupLoop() {
	defer close(c.cleanupDone)
	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCleanup:
			return
		case <-ticker.C:
			c.removeExpired()
		}
	}
}

// removeExpired scans all entries and removes those that have expired.
func (c *Cache) removeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, item := range c.entries {
		if item.entry.IsExpired() {
			c.removeEntryLocked(key)
			c.expirations.Add(1)
		}
	}
}

// Compile-time interface satisfaction checks.
var _ cache.Cache = (*Cache)(nil)
var _ cache.StatsCollector = (*Cache)(nil)
