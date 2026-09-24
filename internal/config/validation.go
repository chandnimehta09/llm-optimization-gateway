package config

import (
	"fmt"
	"strings"
)

// ValidationError collects multiple configuration errors.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("configuration errors: %s", strings.Join(e.Errors, "; "))
}

// Validate checks the Config for invalid or missing values and returns a
// ValidationError if any problems are found.
func Validate(cfg *Config) error {
	var errs []string

	// Server validation
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		errs = append(errs, fmt.Sprintf("server.port must be between 1 and 65535, got %d", cfg.Server.Port))
	}
	if cfg.Server.ReadTimeout <= 0 {
		errs = append(errs, "server.read_timeout must be positive")
	}
	if cfg.Server.WriteTimeout <= 0 {
		errs = append(errs, "server.write_timeout must be positive")
	}
	if cfg.Server.ShutdownTimeout <= 0 {
		errs = append(errs, "server.shutdown_timeout must be positive")
	}

	// Cache validation
	if cfg.Cache.Enabled {
		if cfg.Cache.RedisURL == "" {
			errs = append(errs, "cache.redis_url is required when cache is enabled")
		}
		if cfg.Cache.DefaultTTL <= 0 {
			errs = append(errs, "cache.default_ttl must be positive when cache is enabled")
		}
	}

	// Provider validation
	for i, p := range cfg.Providers {
		if p.Name == "" {
			errs = append(errs, fmt.Sprintf("providers[%d].name is required", i))
		}
		if p.APIKey == "" {
			errs = append(errs, fmt.Sprintf("providers[%d].api_key is required", i))
		}
		if p.BaseURL == "" {
			errs = append(errs, fmt.Sprintf("providers[%d].base_url is required", i))
		}
		if p.Timeout <= 0 {
			errs = append(errs, fmt.Sprintf("providers[%d].timeout must be positive", i))
		}
	}

	// Telemetry validation
	if cfg.Telemetry.Enabled {
		if cfg.Telemetry.ServiceName == "" {
			errs = append(errs, "telemetry.service_name is required when telemetry is enabled")
		}
		if cfg.Telemetry.MetricsPort < 1 || cfg.Telemetry.MetricsPort > 65535 {
			errs = append(errs, fmt.Sprintf("telemetry.metrics_port must be between 1 and 65535, got %d", cfg.Telemetry.MetricsPort))
		}
	}

	// Logging validation
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(cfg.Logging.Level)] {
		errs = append(errs, fmt.Sprintf("logging.level must be one of debug, info, warn, error; got %q", cfg.Logging.Level))
	}
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[strings.ToLower(cfg.Logging.Format)] {
		errs = append(errs, fmt.Sprintf("logging.format must be one of json, text; got %q", cfg.Logging.Format))
	}

	// Gateway validation
	if cfg.Gateway.MaxRequestSize <= 0 {
		errs = append(errs, "gateway.max_request_size must be positive")
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}
