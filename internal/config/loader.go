package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultConfigPath is the default location for the configuration file.
	DefaultConfigPath = "config/gateway.yaml"
	// EnvPrefix is the prefix for environment variable overrides.
	EnvPrefix = "GATEWAY_"
)

// Load loads configuration from the given file paths, applies environment
// variable overrides, and validates the result. If no paths are provided,
// it attempts to load from DefaultConfigPath. Files are merged in order;
// later files override earlier ones.
func Load(paths ...string) (*Config, error) {
	cfg := DefaultConfig()

	if len(paths) == 0 {
		paths = []string{DefaultConfigPath}
	}

	for _, path := range paths {
		if err := loadYAMLFile(path, &cfg); err != nil {
			return nil, fmt.Errorf("loading config from %s: %w", path, err)
		}
	}

	applyEnvOverrides(&cfg)

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &cfg, nil
}

// LoadFromBytes loads configuration from raw YAML bytes, applies environment
// variable overrides, and validates the result. Useful for testing.
func LoadFromBytes(data []byte) (*Config, error) {
	cfg := DefaultConfig()

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	applyEnvOverrides(&cfg)

	if err := Validate(&cfg); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return &cfg, nil
}

// loadYAMLFile reads and unmarshals a YAML file into the given Config.
// If the file does not exist, it returns an error.
func loadYAMLFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("parsing YAML: %w", err)
	}
	return nil
}

// applyEnvOverrides reads environment variables with the GATEWAY_ prefix
// and applies them to the configuration. Nested fields use underscores
// as separators (e.g., GATEWAY_SERVER_PORT).
func applyEnvOverrides(cfg *Config) {
	// Server overrides
	if v := os.Getenv(EnvPrefix + "SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv(EnvPrefix + "SERVER_READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ReadTimeout = d
		}
	}
	if v := os.Getenv(EnvPrefix + "SERVER_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.WriteTimeout = d
		}
	}
	if v := os.Getenv(EnvPrefix + "SERVER_SHUTDOWN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Server.ShutdownTimeout = d
		}
	}

	// Cache overrides
	if v := os.Getenv(EnvPrefix + "CACHE_ENABLED"); v != "" {
		cfg.Cache.Enabled = parseBool(v)
	}
	if v := os.Getenv(EnvPrefix + "CACHE_REDIS_URL"); v != "" {
		cfg.Cache.RedisURL = v
	}
	if v := os.Getenv(EnvPrefix + "CACHE_DEFAULT_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Cache.DefaultTTL = d
		}
	}

	// Telemetry overrides
	if v := os.Getenv(EnvPrefix + "TELEMETRY_ENABLED"); v != "" {
		cfg.Telemetry.Enabled = parseBool(v)
	}
	if v := os.Getenv(EnvPrefix + "TELEMETRY_SERVICE_NAME"); v != "" {
		cfg.Telemetry.ServiceName = v
	}
	if v := os.Getenv(EnvPrefix + "TELEMETRY_METRICS_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Telemetry.MetricsPort = port
		}
	}
	if v := os.Getenv(EnvPrefix + "TELEMETRY_TRACE_ENDPOINT"); v != "" {
		cfg.Telemetry.TraceEndpoint = v
	}

	// Logging overrides
	if v := os.Getenv(EnvPrefix + "LOGGING_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv(EnvPrefix + "LOGGING_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}
	if v := os.Getenv(EnvPrefix + "LOGGING_OUTPUT"); v != "" {
		cfg.Logging.Output = v
	}

	// Gateway overrides
	if v := os.Getenv(EnvPrefix + "GATEWAY_REQUEST_ID_HEADER"); v != "" {
		cfg.Gateway.RequestIDHeader = v
	}
	if v := os.Getenv(EnvPrefix + "GATEWAY_MAX_REQUEST_SIZE"); v != "" {
		if size, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.Gateway.MaxRequestSize = size
		}
	}
}

// parseBool parses common boolean string representations.
func parseBool(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	return lower == "true" || lower == "1" || lower == "yes"
}
