package config

import "time"

// DefaultConfig returns a Config populated with sensible default values.
func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			ShutdownTimeout: 10 * time.Second,
		},
		Cache: CacheConfig{
			Enabled:    false,
			RedisURL:   "localhost:6379",
			DefaultTTL: 5 * time.Minute,
		},
		Providers: nil,
		Telemetry: TelemetryConfig{
			Enabled:       false,
			ServiceName:   "llm-optimization-gateway",
			MetricsPort:   9090,
			TraceEndpoint: "localhost:4317",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Gateway: GatewayConfig{
			RequestIDHeader: "X-Request-ID",
			MaxRequestSize:  1 << 20, // 1 MB
		},
	}
}
