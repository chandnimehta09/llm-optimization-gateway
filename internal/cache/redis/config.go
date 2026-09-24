// Package redis provides a Redis-backed implementation of the cache.Cache interface.
package redis

import (
	"crypto/tls"
	"fmt"
	"time"
)

// Mode specifies the Redis deployment mode.
type Mode int

const (
	// ModeStandalone is a single Redis instance.
	ModeStandalone Mode = iota
	// ModeCluster connects to a Redis Cluster deployment.
	ModeCluster
	// ModeSentinel connects via Redis Sentinel for high availability.
	ModeSentinel
)

// String returns a human-readable representation of the Mode.
func (m Mode) String() string {
	switch m {
	case ModeStandalone:
		return "standalone"
	case ModeCluster:
		return "cluster"
	case ModeSentinel:
		return "sentinel"
	default:
		return "unknown"
	}
}

// Config holds Redis-specific configuration options.
type Config struct {
	// Mode selects the Redis deployment type (standalone, cluster, sentinel).
	Mode Mode

	// Addr is the Redis server address in "host:port" format.
	// For cluster mode, this can be any node; additional nodes are discovered automatically.
	// For sentinel mode, this is the sentinel address.
	Addr string

	// Addrs is used for cluster and sentinel modes to provide multiple addresses.
	Addrs []string

	// Password is the Redis AUTH password. Empty string means no authentication.
	Password string

	// DB is the Redis database number (0-15). Only used in standalone and sentinel modes.
	DB int

	// SentinelMasterName is the name of the Redis master for sentinel mode.
	SentinelMasterName string

	// PoolSize is the maximum number of connections in the pool.
	// Default: 10.
	PoolSize int

	// MinIdleConns is the minimum number of idle connections maintained in the pool.
	// Default: 2.
	MinIdleConns int

	// DialTimeout is the timeout for establishing new connections.
	// Default: 5s.
	DialTimeout time.Duration

	// ReadTimeout is the timeout for socket reads.
	// Default: 3s.
	ReadTimeout time.Duration

	// WriteTimeout is the timeout for socket writes.
	// Default: 3s.
	WriteTimeout time.Duration

	// MaxRetries is the maximum number of retries before giving up on a command.
	// Default: 3.
	MaxRetries int

	// MinRetryBackoff is the minimum backoff between retries.
	// Default: 8ms.
	MinRetryBackoff time.Duration

	// MaxRetryBackoff is the maximum backoff between retries.
	// Default: 512ms.
	MaxRetryBackoff time.Duration

	// DefaultTTL is the default TTL for cache entries when none is specified.
	// Zero means no expiration.
	DefaultTTL time.Duration

	// KeyPrefix is prepended to all cache keys. Useful for namespacing.
	KeyPrefix string

	// EnableCompression enables gzip compression for entries larger than
	// CompressionThreshold bytes.
	EnableCompression bool

	// CompressionThreshold is the minimum entry size in bytes before compression
	// is applied. Default: 1024 (1KB).
	CompressionThreshold int

	// TLSConfig enables TLS for the Redis connection.
	TLSConfig *tls.Config

	// HealthCheckInterval is the interval between automatic health checks.
	// Default: 30s. Zero disables periodic health checks.
	HealthCheckInterval time.Duration
}

// DefaultConfig returns a Config with sensible defaults for a standalone Redis instance.
func DefaultConfig() Config {
	return Config{
		Mode:                 ModeStandalone,
		Addr:                 "localhost:6379",
		DB:                   0,
		PoolSize:             10,
		MinIdleConns:         2,
		DialTimeout:          5 * time.Second,
		ReadTimeout:          3 * time.Second,
		WriteTimeout:         3 * time.Second,
		MaxRetries:           3,
		MinRetryBackoff:      8 * time.Millisecond,
		MaxRetryBackoff:      512 * time.Millisecond,
		DefaultTTL:           0,
		KeyPrefix:            "llmcache:",
		EnableCompression:    false,
		CompressionThreshold: 1024,
		HealthCheckInterval:  30 * time.Second,
	}
}

// Validate checks the configuration for obvious errors.
func (c *Config) Validate() error {
	switch c.Mode {
	case ModeStandalone:
		if c.Addr == "" {
			return fmt.Errorf("redis config: addr is required for standalone mode")
		}
	case ModeCluster:
		if len(c.Addrs) == 0 && c.Addr == "" {
			return fmt.Errorf("redis config: at least one address is required for cluster mode")
		}
	case ModeSentinel:
		if len(c.Addrs) == 0 && c.Addr == "" {
			return fmt.Errorf("redis config: at least one sentinel address is required")
		}
		if c.SentinelMasterName == "" {
			return fmt.Errorf("redis config: sentinel master name is required for sentinel mode")
		}
	default:
		return fmt.Errorf("redis config: unknown mode %d", c.Mode)
	}

	if c.PoolSize < 0 {
		return fmt.Errorf("redis config: pool size must be non-negative")
	}
	if c.DB < 0 || c.DB > 15 {
		return fmt.Errorf("redis config: db must be between 0 and 15")
	}
	if c.CompressionThreshold < 0 {
		return fmt.Errorf("redis config: compression threshold must be non-negative")
	}

	return nil
}

// addrs returns the list of addresses for cluster/sentinel modes.
// Falls back to Addr if Addrs is empty.
func (c *Config) addrs() []string {
	if len(c.Addrs) > 0 {
		return c.Addrs
	}
	if c.Addr != "" {
		return []string{c.Addr}
	}
	return nil
}
