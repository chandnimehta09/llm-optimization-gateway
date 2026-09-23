// Package config provides configuration loading and validation for the
// LLM optimization gateway. It supports YAML file loading, environment
// variable overrides with a GATEWAY_ prefix, and sensible defaults.
package config

import "time"

// Config is the root configuration for the gateway.
type Config struct {
	Server    ServerConfig     `yaml:"server"`
	Cache     CacheConfig      `yaml:"cache"`
	Providers []ProviderConfig `yaml:"providers"`
	Telemetry TelemetryConfig  `yaml:"telemetry"`
	Logging   LoggingConfig    `yaml:"logging"`
	Gateway   GatewayConfig    `yaml:"gateway"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port            int           `yaml:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

// CacheConfig holds caching layer settings.
type CacheConfig struct {
	Enabled    bool          `yaml:"enabled"`
	RedisURL   string        `yaml:"redis_url"`
	DefaultTTL time.Duration `yaml:"default_ttl"`
}

// ProviderConfig holds settings for a single LLM provider.
type ProviderConfig struct {
	Name    string        `yaml:"name"`
	APIKey  string        `yaml:"api_key"`
	BaseURL string        `yaml:"base_url"`
	Timeout time.Duration `yaml:"timeout"`
}

// TelemetryConfig holds observability settings.
type TelemetryConfig struct {
	Enabled       bool   `yaml:"enabled"`
	ServiceName   string `yaml:"service_name"`
	MetricsPort   int    `yaml:"metrics_port"`
	TraceEndpoint string `yaml:"trace_endpoint"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// GatewayConfig holds gateway-level settings.
type GatewayConfig struct {
	RequestIDHeader string `yaml:"request_id_header"`
	MaxRequestSize  int64  `yaml:"max_request_size"`
}
