// Package memory provides an in-memory cache implementation suitable for
// testing and development. It supports TTL-based expiration, LRU eviction
// when a maximum size is reached, and thread-safe concurrent access.
package memory

import "time"

// Config holds the configuration for the in-memory cache.
type Config struct {
	// MaxSize is the maximum number of entries the cache can hold.
	// When this limit is reached, the least recently used entry is evicted.
	// A value of 0 means unlimited size (no eviction).
	MaxSize int

	// DefaultTTL is the default time-to-live for cache entries.
	// A value of 0 means entries do not expire by default.
	DefaultTTL time.Duration

	// CleanupInterval controls how often the background goroutine scans
	// for and removes expired entries. A value of 0 disables background
	// cleanup (expired entries are still removed lazily on access).
	CleanupInterval time.Duration
}

// DefaultConfig returns a Config with sensible defaults for testing.
func DefaultConfig() Config {
	return Config{
		MaxSize:         1000,
		DefaultTTL:      5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
}

// Validate checks the configuration for invalid values and returns an
// error if any are found.
func (c Config) Validate() error {
	if c.MaxSize < 0 {
		return &ConfigError{Field: "MaxSize", Reason: "must be >= 0"}
	}
	if c.DefaultTTL < 0 {
		return &ConfigError{Field: "DefaultTTL", Reason: "must be >= 0"}
	}
	if c.CleanupInterval < 0 {
		return &ConfigError{Field: "CleanupInterval", Reason: "must be >= 0"}
	}
	return nil
}

// ConfigError indicates an invalid configuration value.
type ConfigError struct {
	Field  string
	Reason string
}

// Error implements the error interface.
func (e *ConfigError) Error() string {
	return "memory cache config: " + e.Field + ": " + e.Reason
}
