package prefix

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.BaseURL != "http://localhost:8000" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "http://localhost:8000")
	}
	if !cfg.Enabled {
		t.Error("Enabled = false, want true")
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 30*time.Second)
	}
	if cfg.MaxRetries != 2 {
		t.Errorf("MaxRetries = %d, want 2", cfg.MaxRetries)
	}
	if cfg.MinPrefixTokens != 16 {
		t.Errorf("MinPrefixTokens = %d, want 16", cfg.MinPrefixTokens)
	}
	if cfg.IncludeSystemPrompt == nil || !*cfg.IncludeSystemPrompt {
		t.Error("IncludeSystemPrompt should be true")
	}
	if cfg.IncludeToolDefinitions == nil || !*cfg.IncludeToolDefinitions {
		t.Error("IncludeToolDefinitions should be true")
	}
	if cfg.IncludeCommonContext == nil || !*cfg.IncludeCommonContext {
		t.Error("IncludeCommonContext should be true")
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestConfig_Validate_NilConfig(t *testing.T) {
	var cfg *Config
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() = nil, want error for nil config")
	}
}

func TestConfig_Validate_EmptyBaseURL(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BaseURL = ""
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() = nil, want error for empty base_url")
	}
}

func TestConfig_Validate_NegativeTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Timeout = -1 * time.Second
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() = nil, want error for negative timeout")
	}
}

func TestConfig_Validate_NegativeMaxRetries(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxRetries = -1
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() = nil, want error for negative max_retries")
	}
}

func TestConfig_Validate_NegativeMinPrefixTokens(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MinPrefixTokens = -1
	err := cfg.Validate()
	if err == nil {
		t.Error("Validate() = nil, want error for negative min_prefix_tokens")
	}
}

func TestConfig_ResolveTimeout_Default(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:8000"}
	got := cfg.resolveTimeout()
	if got != 30*time.Second {
		t.Errorf("resolveTimeout() = %v, want %v", got, 30*time.Second)
	}
}

func TestConfig_ResolveTimeout_Custom(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:8000", Timeout: 10 * time.Second}
	got := cfg.resolveTimeout()
	if got != 10*time.Second {
		t.Errorf("resolveTimeout() = %v, want %v", got, 10*time.Second)
	}
}

func TestConfig_ResolveMaxRetries_Default(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:8000"}
	got := cfg.resolveMaxRetries()
	if got != 2 {
		t.Errorf("resolveMaxRetries() = %d, want 2", got)
	}
}

func TestConfig_ResolveMinPrefixTokens_Default(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:8000"}
	got := cfg.resolveMinPrefixTokens()
	if got != 16 {
		t.Errorf("resolveMinPrefixTokens() = %d, want 16", got)
	}
}

func TestConfig_IncludeFlags_Defaults(t *testing.T) {
	cfg := &Config{BaseURL: "http://localhost:8000"}

	if !cfg.includeSystemPrompt() {
		t.Error("includeSystemPrompt() = false, want true (default)")
	}
	if !cfg.includeToolDefs() {
		t.Error("includeToolDefs() = false, want true (default)")
	}
	if !cfg.includeCommonCtx() {
		t.Error("includeCommonCtx() = false, want true (default)")
	}
}

func TestConfig_IncludeFlags_Explicit(t *testing.T) {
	f := false
	cfg := &Config{
		BaseURL:                "http://localhost:8000",
		IncludeSystemPrompt:    &f,
		IncludeToolDefinitions: &f,
		IncludeCommonContext:   &f,
	}

	if cfg.includeSystemPrompt() {
		t.Error("includeSystemPrompt() = true, want false")
	}
	if cfg.includeToolDefs() {
		t.Error("includeToolDefs() = true, want false")
	}
	if cfg.includeCommonCtx() {
		t.Error("includeCommonCtx() = true, want false")
	}
}
