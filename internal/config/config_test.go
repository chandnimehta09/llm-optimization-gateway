package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Default values tests ---

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default server.port=8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected default read_timeout=30s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Errorf("expected default write_timeout=30s, got %v", cfg.Server.WriteTimeout)
	}
	if cfg.Server.ShutdownTimeout != 10*time.Second {
		t.Errorf("expected default shutdown_timeout=10s, got %v", cfg.Server.ShutdownTimeout)
	}
	if cfg.Cache.Enabled != false {
		t.Error("expected default cache.enabled=false")
	}
	if cfg.Cache.RedisURL != "localhost:6379" {
		t.Errorf("expected default redis_url=localhost:6379, got %s", cfg.Cache.RedisURL)
	}
	if cfg.Cache.DefaultTTL != 5*time.Minute {
		t.Errorf("expected default cache.default_ttl=5m, got %v", cfg.Cache.DefaultTTL)
	}
	if cfg.Telemetry.Enabled != false {
		t.Error("expected default telemetry.enabled=false")
	}
	if cfg.Telemetry.ServiceName != "llm-optimization-gateway" {
		t.Errorf("expected default service_name=llm-optimization-gateway, got %s", cfg.Telemetry.ServiceName)
	}
	if cfg.Telemetry.MetricsPort != 9090 {
		t.Errorf("expected default metrics_port=9090, got %d", cfg.Telemetry.MetricsPort)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("expected default logging.level=info, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected default logging.format=json, got %s", cfg.Logging.Format)
	}
	if cfg.Logging.Output != "stdout" {
		t.Errorf("expected default logging.output=stdout, got %s", cfg.Logging.Output)
	}
	if cfg.Gateway.RequestIDHeader != "X-Request-ID" {
		t.Errorf("expected default request_id_header=X-Request-ID, got %s", cfg.Gateway.RequestIDHeader)
	}
	if cfg.Gateway.MaxRequestSize != 1<<20 {
		t.Errorf("expected default max_request_size=1048576, got %d", cfg.Gateway.MaxRequestSize)
	}
	if cfg.Providers != nil {
		t.Errorf("expected default providers=nil, got %v", cfg.Providers)
	}
}

// --- YAML loading tests ---

func TestLoadFromBytes_BasicYAML(t *testing.T) {
	yamlData := []byte(`
server:
  port: 9090
  read_timeout: 15s
  write_timeout: 20s
  shutdown_timeout: 5s
logging:
  level: debug
  format: text
  output: stderr
`)
	cfg, err := LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port=9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Errorf("expected read_timeout=15s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 20*time.Second {
		t.Errorf("expected write_timeout=20s, got %v", cfg.Server.WriteTimeout)
	}
	if cfg.Server.ShutdownTimeout != 5*time.Second {
		t.Errorf("expected shutdown_timeout=5s, got %v", cfg.Server.ShutdownTimeout)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected logging.level=debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("expected logging.format=text, got %s", cfg.Logging.Format)
	}

	// Verify defaults are preserved for unspecified fields
	if cfg.Cache.RedisURL != "localhost:6379" {
		t.Errorf("expected default redis_url preserved, got %s", cfg.Cache.RedisURL)
	}
	if cfg.Gateway.RequestIDHeader != "X-Request-ID" {
		t.Errorf("expected default request_id_header preserved, got %s", cfg.Gateway.RequestIDHeader)
	}
}

func TestLoadFromBytes_WithProviders(t *testing.T) {
	yamlData := []byte(`
providers:
  - name: openai
    api_key: sk-test-key
    base_url: https://api.openai.com/v1
    timeout: 30s
  - name: anthropic
    api_key: ak-test-key
    base_url: https://api.anthropic.com/v1
    timeout: 45s
`)
	cfg, err := LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "openai" {
		t.Errorf("expected first provider name=openai, got %s", cfg.Providers[0].Name)
	}
	if cfg.Providers[0].APIKey != "sk-test-key" {
		t.Errorf("expected first provider api_key=sk-test-key, got %s", cfg.Providers[0].APIKey)
	}
	if cfg.Providers[0].Timeout != 30*time.Second {
		t.Errorf("expected first provider timeout=30s, got %v", cfg.Providers[0].Timeout)
	}
	if cfg.Providers[1].Name != "anthropic" {
		t.Errorf("expected second provider name=anthropic, got %s", cfg.Providers[1].Name)
	}
	if cfg.Providers[1].BaseURL != "https://api.anthropic.com/v1" {
		t.Errorf("expected second provider base_url, got %s", cfg.Providers[1].BaseURL)
	}
}

func TestLoadFromBytes_InvalidYAML(t *testing.T) {
	yamlData := []byte(`{{{invalid yaml`)
	_, err := LoadFromBytes(yamlData)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "test-config.yaml")

	yamlContent := []byte(`
server:
  port: 7070
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
logging:
  level: warn
  format: json
  output: stdout
`)
	if err := os.WriteFile(cfgPath, yamlContent, 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 7070 {
		t.Errorf("expected port=7070, got %d", cfg.Server.Port)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("expected logging.level=warn, got %s", cfg.Logging.Level)
	}
}

func TestLoad_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.yaml")
	overridePath := filepath.Join(dir, "override.yaml")

	baseYAML := []byte(`
server:
  port: 3000
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
logging:
  level: info
  format: json
  output: stdout
`)
	overrideYAML := []byte(`
server:
  port: 4000
logging:
  level: debug
`)

	if err := os.WriteFile(basePath, baseYAML, 0644); err != nil {
		t.Fatalf("failed to write base config: %v", err)
	}
	if err := os.WriteFile(overridePath, overrideYAML, 0644); err != nil {
		t.Fatalf("failed to write override config: %v", err)
	}

	cfg, err := Load(basePath, overridePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 4000 {
		t.Errorf("expected port=4000 from override, got %d", cfg.Server.Port)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected logging.level=debug from override, got %s", cfg.Logging.Level)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// --- Environment variable override tests ---

func TestEnvOverride_ServerPort(t *testing.T) {
	yamlData := []byte(`
server:
  port: 9090
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
`)
	t.Setenv("GATEWAY_SERVER_PORT", "8888")

	cfg, err := LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8888 {
		t.Errorf("expected env override port=8888, got %d", cfg.Server.Port)
	}
}

func TestEnvOverride_CacheSettings(t *testing.T) {
	yamlData := []byte(`
server:
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
`)
	t.Setenv("GATEWAY_CACHE_ENABLED", "true")
	t.Setenv("GATEWAY_CACHE_REDIS_URL", "redis://custom:6380")
	t.Setenv("GATEWAY_CACHE_DEFAULT_TTL", "10m")

	cfg, err := LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Cache.Enabled {
		t.Error("expected cache.enabled=true from env")
	}
	if cfg.Cache.RedisURL != "redis://custom:6380" {
		t.Errorf("expected cache.redis_url=redis://custom:6380, got %s", cfg.Cache.RedisURL)
	}
	if cfg.Cache.DefaultTTL != 10*time.Minute {
		t.Errorf("expected cache.default_ttl=10m, got %v", cfg.Cache.DefaultTTL)
	}
}

func TestEnvOverride_LoggingLevel(t *testing.T) {
	yamlData := []byte(`
server:
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
logging:
  level: info
  format: json
  output: stdout
`)
	t.Setenv("GATEWAY_LOGGING_LEVEL", "debug")

	cfg, err := LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected logging.level=debug from env, got %s", cfg.Logging.Level)
	}
}

func TestEnvOverride_TelemetrySettings(t *testing.T) {
	yamlData := []byte(`
server:
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
`)
	t.Setenv("GATEWAY_TELEMETRY_ENABLED", "true")
	t.Setenv("GATEWAY_TELEMETRY_SERVICE_NAME", "my-gateway")
	t.Setenv("GATEWAY_TELEMETRY_METRICS_PORT", "9191")
	t.Setenv("GATEWAY_TELEMETRY_TRACE_ENDPOINT", "otel:4317")

	cfg, err := LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Telemetry.Enabled {
		t.Error("expected telemetry.enabled=true from env")
	}
	if cfg.Telemetry.ServiceName != "my-gateway" {
		t.Errorf("expected service_name=my-gateway, got %s", cfg.Telemetry.ServiceName)
	}
	if cfg.Telemetry.MetricsPort != 9191 {
		t.Errorf("expected metrics_port=9191, got %d", cfg.Telemetry.MetricsPort)
	}
	if cfg.Telemetry.TraceEndpoint != "otel:4317" {
		t.Errorf("expected trace_endpoint=otel:4317, got %s", cfg.Telemetry.TraceEndpoint)
	}
}

func TestEnvOverride_GatewaySettings(t *testing.T) {
	yamlData := []byte(`
server:
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
`)
	t.Setenv("GATEWAY_GATEWAY_REQUEST_ID_HEADER", "X-Trace-ID")
	t.Setenv("GATEWAY_GATEWAY_MAX_REQUEST_SIZE", "2097152")

	cfg, err := LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Gateway.RequestIDHeader != "X-Trace-ID" {
		t.Errorf("expected request_id_header=X-Trace-ID, got %s", cfg.Gateway.RequestIDHeader)
	}
	if cfg.Gateway.MaxRequestSize != 2097152 {
		t.Errorf("expected max_request_size=2097152, got %d", cfg.Gateway.MaxRequestSize)
	}
}

func TestEnvOverride_ServerTimeouts(t *testing.T) {
	yamlData := []byte(`
server:
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
`)
	t.Setenv("GATEWAY_SERVER_READ_TIMEOUT", "60s")
	t.Setenv("GATEWAY_SERVER_WRITE_TIMEOUT", "45s")
	t.Setenv("GATEWAY_SERVER_SHUTDOWN_TIMEOUT", "20s")

	cfg, err := LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.ReadTimeout != 60*time.Second {
		t.Errorf("expected read_timeout=60s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 45*time.Second {
		t.Errorf("expected write_timeout=45s, got %v", cfg.Server.WriteTimeout)
	}
	if cfg.Server.ShutdownTimeout != 20*time.Second {
		t.Errorf("expected shutdown_timeout=20s, got %v", cfg.Server.ShutdownTimeout)
	}
}

func TestEnvOverride_InvalidPortIgnored(t *testing.T) {
	yamlData := []byte(`
server:
  port: 9090
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
`)
	t.Setenv("GATEWAY_SERVER_PORT", "not-a-number")

	cfg, err := LoadFromBytes(yamlData)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Invalid env value should be silently ignored; YAML value preserved
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port=9090 (invalid env ignored), got %d", cfg.Server.Port)
	}
}

// --- Validation tests ---

func TestValidation_InvalidPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = 0
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for port=0")
	}
	valErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if len(valErr.Errors) == 0 {
		t.Fatal("expected at least one validation error")
	}
}

func TestValidation_PortTooHigh(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = 70000
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for port=70000")
	}
}

func TestValidation_InvalidLogLevel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logging.Level = "verbose"
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid log level")
	}
}

func TestValidation_InvalidLogFormat(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Logging.Format = "xml"
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for invalid log format")
	}
}

func TestValidation_ProviderMissingAPIKey(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers = []ProviderConfig{
		{Name: "openai", BaseURL: "https://api.openai.com/v1", Timeout: 30 * time.Second},
	}
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for missing provider api_key")
	}
	valErr := err.(*ValidationError)
	found := false
	for _, e := range valErr.Errors {
		if e == "providers[0].api_key is required" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected error about providers[0].api_key, got: %v", valErr.Errors)
	}
}

func TestValidation_ProviderMissingName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers = []ProviderConfig{
		{APIKey: "sk-key", BaseURL: "https://api.openai.com/v1", Timeout: 30 * time.Second},
	}
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for missing provider name")
	}
}

func TestValidation_ProviderMissingBaseURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers = []ProviderConfig{
		{Name: "openai", APIKey: "sk-key", Timeout: 30 * time.Second},
	}
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for missing provider base_url")
	}
}

func TestValidation_ProviderInvalidTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Providers = []ProviderConfig{
		{Name: "openai", APIKey: "sk-key", BaseURL: "https://api.openai.com/v1", Timeout: 0},
	}
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for provider timeout=0")
	}
}

func TestValidation_CacheEnabledNoRedisURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Cache.Enabled = true
	cfg.Cache.RedisURL = ""
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for cache enabled without redis_url")
	}
}

func TestValidation_CacheEnabledInvalidTTL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Cache.Enabled = true
	cfg.Cache.DefaultTTL = 0
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for cache enabled with ttl=0")
	}
}

func TestValidation_TelemetryEnabledInvalidPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.MetricsPort = 0
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for telemetry enabled with port=0")
	}
}

func TestValidation_TelemetryEnabledNoServiceName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.ServiceName = ""
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for telemetry enabled without service_name")
	}
}

func TestValidation_NegativeMaxRequestSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Gateway.MaxRequestSize = -1
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for negative max_request_size")
	}
}

func TestValidation_NegativeReadTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.ReadTimeout = -1
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation error for negative read_timeout")
	}
}

func TestValidation_DefaultConfigIsValid(t *testing.T) {
	cfg := DefaultConfig()
	err := Validate(&cfg)
	if err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}
}

func TestValidation_MultipleErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = 0
	cfg.Logging.Level = "verbose"
	cfg.Logging.Format = "xml"
	err := Validate(&cfg)
	if err == nil {
		t.Fatal("expected validation errors")
	}
	valErr := err.(*ValidationError)
	if len(valErr.Errors) < 3 {
		t.Errorf("expected at least 3 validation errors, got %d: %v", len(valErr.Errors), valErr.Errors)
	}
}

// --- Integration test ---

func TestIntegration_FullLoadWithEnvOverride(t *testing.T) {
	// Create a temp YAML file with server.port=9090
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "integration.yaml")
	yamlContent := []byte(`
server:
  port: 9090
  read_timeout: 15s
  write_timeout: 20s
  shutdown_timeout: 8s
cache:
  enabled: true
  redis_url: "redis://myhost:6379"
  default_ttl: 10m
providers:
  - name: openai
    api_key: sk-integration-test
    base_url: https://api.openai.com/v1
    timeout: 30s
telemetry:
  enabled: true
  service_name: integration-test
  metrics_port: 9191
  trace_endpoint: "otel-collector:4317"
logging:
  level: debug
  format: text
  output: stderr
gateway:
  request_id_header: "X-Correlation-ID"
  max_request_size: 2097152
`)
	if err := os.WriteFile(cfgPath, yamlContent, 0644); err != nil {
		t.Fatalf("failed to write integration config: %v", err)
	}

	// Set env override for server port
	t.Setenv("GATEWAY_SERVER_PORT", "8888")

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Acceptance criteria:
	// server.port equals 8888 (env override wins over YAML 9090)
	if cfg.Server.Port != 8888 {
		t.Errorf("expected server.port=8888 (env override), got %d", cfg.Server.Port)
	}

	// YAML values preserved for other fields
	if cfg.Server.ReadTimeout != 15*time.Second {
		t.Errorf("expected read_timeout=15s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 20*time.Second {
		t.Errorf("expected write_timeout=20s, got %v", cfg.Server.WriteTimeout)
	}
	if cfg.Server.ShutdownTimeout != 8*time.Second {
		t.Errorf("expected shutdown_timeout=8s, got %v", cfg.Server.ShutdownTimeout)
	}

	// Cache
	if !cfg.Cache.Enabled {
		t.Error("expected cache.enabled=true")
	}
	if cfg.Cache.RedisURL != "redis://myhost:6379" {
		t.Errorf("expected redis_url=redis://myhost:6379, got %s", cfg.Cache.RedisURL)
	}
	if cfg.Cache.DefaultTTL != 10*time.Minute {
		t.Errorf("expected default_ttl=10m, got %v", cfg.Cache.DefaultTTL)
	}

	// Providers
	if len(cfg.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "openai" {
		t.Errorf("expected provider name=openai, got %s", cfg.Providers[0].Name)
	}
	if cfg.Providers[0].APIKey != "sk-integration-test" {
		t.Errorf("expected provider api_key=sk-integration-test, got %s", cfg.Providers[0].APIKey)
	}

	// Telemetry
	if !cfg.Telemetry.Enabled {
		t.Error("expected telemetry.enabled=true")
	}
	if cfg.Telemetry.ServiceName != "integration-test" {
		t.Errorf("expected service_name=integration-test, got %s", cfg.Telemetry.ServiceName)
	}
	if cfg.Telemetry.MetricsPort != 9191 {
		t.Errorf("expected metrics_port=9191, got %d", cfg.Telemetry.MetricsPort)
	}

	// Logging
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected logging.level=debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("expected logging.format=text, got %s", cfg.Logging.Format)
	}

	// Gateway
	if cfg.Gateway.RequestIDHeader != "X-Correlation-ID" {
		t.Errorf("expected request_id_header=X-Correlation-ID, got %s", cfg.Gateway.RequestIDHeader)
	}
	if cfg.Gateway.MaxRequestSize != 2097152 {
		t.Errorf("expected max_request_size=2097152, got %d", cfg.Gateway.MaxRequestSize)
	}
}

func TestIntegration_ValidationFailsOnMissingProviderConfig(t *testing.T) {
	yamlData := []byte(`
server:
  port: 8080
  read_timeout: 10s
  write_timeout: 10s
  shutdown_timeout: 5s
providers:
  - name: openai
    base_url: https://api.openai.com/v1
    timeout: 30s
`)
	// Missing api_key should cause validation failure
	_, err := LoadFromBytes(yamlData)
	if err == nil {
		t.Fatal("expected validation error for provider with missing api_key")
	}
}

// --- parseBool tests ---

func TestParseBool(t *testing.T) {
	cases := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"yes", true},
		{"Yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false},
		{"  true  ", true},
	}
	for _, tc := range cases {
		got := parseBool(tc.input)
		if got != tc.expected {
			t.Errorf("parseBool(%q) = %v, want %v", tc.input, got, tc.expected)
		}
	}
}
