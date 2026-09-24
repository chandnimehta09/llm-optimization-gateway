// Package prefix provides vLLM prefix-cache integration for the LLM
// optimization gateway. It constructs stable prefixes from system prompts,
// tool definitions, and common context so that vLLM's automatic prefix
// caching (APC) can reuse KV-cache blocks across requests.
package prefix

import (
	"errors"
	"fmt"
	"time"
)

// Config holds configuration for the vLLM prefix-cache backend.
type Config struct {
	// BaseURL is the vLLM server base URL (e.g. "http://localhost:8000").
	BaseURL string `json:"base_url"`

	// APIKey is the optional authentication token for the vLLM server.
	// Sent as a Bearer token in the Authorization header.
	APIKey string `json:"api_key,omitempty"`

	// Model is the default model to request when not specified on the request.
	Model string `json:"model,omitempty"`

	// Enabled controls whether prefix caching integration is active.
	// When false, requests are passed through without prefix construction.
	Enabled bool `json:"enabled"`

	// Timeout is the HTTP request timeout for calls to the vLLM server.
	// Defaults to 30 seconds if zero.
	Timeout time.Duration `json:"timeout,omitempty"`

	// MaxRetries is the number of times to retry a failed vLLM request.
	// Defaults to 2 if zero.
	MaxRetries int `json:"max_retries,omitempty"`

	// MinPrefixTokens is the minimum number of estimated prefix tokens
	// required for prefix caching to be attempted. Requests with shorter
	// prefixes skip prefix optimisation. Defaults to 16 if zero.
	MinPrefixTokens int `json:"min_prefix_tokens,omitempty"`

	// IncludeSystemPrompt controls whether system prompts are included
	// in the stable prefix. Defaults to true.
	IncludeSystemPrompt *bool `json:"include_system_prompt,omitempty"`

	// IncludeToolDefinitions controls whether tool definitions are included
	// in the stable prefix. Defaults to true.
	IncludeToolDefinitions *bool `json:"include_tool_definitions,omitempty"`

	// IncludeCommonContext controls whether common context messages are
	// included in the stable prefix. Defaults to true.
	IncludeCommonContext *bool `json:"include_common_context,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	t := true
	return &Config{
		BaseURL:                "http://localhost:8000",
		Enabled:                true,
		Timeout:                30 * time.Second,
		MaxRetries:             2,
		MinPrefixTokens:        16,
		IncludeSystemPrompt:    &t,
		IncludeToolDefinitions: &t,
		IncludeCommonContext:   &t,
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("prefix config: config must not be nil")
	}
	if c.BaseURL == "" {
		return errors.New("prefix config: base_url must not be empty")
	}
	if c.Timeout < 0 {
		return fmt.Errorf("prefix config: timeout must not be negative, got %v", c.Timeout)
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("prefix config: max_retries must not be negative, got %d", c.MaxRetries)
	}
	if c.MinPrefixTokens < 0 {
		return fmt.Errorf("prefix config: min_prefix_tokens must not be negative, got %d", c.MinPrefixTokens)
	}
	return nil
}

// resolveTimeout returns the effective timeout.
func (c *Config) resolveTimeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 30 * time.Second
}

// resolveMaxRetries returns the effective max retries.
func (c *Config) resolveMaxRetries() int {
	if c.MaxRetries > 0 {
		return c.MaxRetries
	}
	return 2
}

// resolveMinPrefixTokens returns the effective min prefix tokens.
func (c *Config) resolveMinPrefixTokens() int {
	if c.MinPrefixTokens > 0 {
		return c.MinPrefixTokens
	}
	return 16
}

// includeSystemPrompt returns whether system prompts should be in the prefix.
func (c *Config) includeSystemPrompt() bool {
	if c.IncludeSystemPrompt == nil {
		return true
	}
	return *c.IncludeSystemPrompt
}

// includeToolDefs returns whether tool definitions should be in the prefix.
func (c *Config) includeToolDefs() bool {
	if c.IncludeToolDefinitions == nil {
		return true
	}
	return *c.IncludeToolDefinitions
}

// includeCommonContext returns whether common context should be in the prefix.
func (c *Config) includeCommonCtx() bool {
	if c.IncludeCommonContext == nil {
		return true
	}
	return *c.IncludeCommonContext
}
