package cache

import "context"

// Cache defines the interface for storing and retrieving cached LLM
// completion responses. Implementations may use in-memory maps, Redis,
// DynamoDB, or any other storage backend.
//
// All methods accept a context for cancellation and timeout support.
// The Option variadic parameter on Set allows callers to override
// defaults (TTL, tags, priority) on a per-call basis.
type Cache interface {
	// Get retrieves a cache entry by key. Returns the entry and true if
	// found, or nil and false on a cache miss. Returns an error on backend
	// failure or if the entry is corrupted.
	//
	// Implementations should check entry expiration and return a
	// CacheExpiredError (or a miss) for expired entries.
	Get(ctx context.Context, key string) (*Entry, bool, error)

	// Set stores or updates a cache entry. Options can override the
	// default TTL, add tags, or set priority.
	Set(ctx context.Context, key string, entry *Entry, opts ...Option) error

	// Delete removes a cache entry by key. Returns nil if the key does
	// not exist.
	Delete(ctx context.Context, key string) error

	// Exists checks whether a key is present in the cache without
	// retrieving the full entry. Returns false for expired entries.
	Exists(ctx context.Context, key string) (bool, error)

	// Close releases any resources held by the cache (connections, files, etc.).
	Close() error
}
