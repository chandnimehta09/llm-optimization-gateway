// Package http provides a generic HTTP-based provider adapter that supports
// both OpenAI-compatible and Google Gemini API formats.
package http

import (
	"fmt"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

// Config holds the configuration for an HTTP provider instance.
type Config struct {
	// Name is the unique identifier for this provider instance.
	Name string

	// APIFormat selects the API format: "openai" or "gemini".
	APIFormat provider.APIFormat

	// APIKey is the authentication credential.
	APIKey string

	// BaseURL is the base URL for the provider's API.
	// OpenAI default: https://api.openai.com
	// Gemini default: https://generativelanguage.googleapis.com
	BaseURL string

	// Models is the list of model identifiers available from this provider.
	Models []string

	// DefaultModel is the model to use when none is specified in the request.
	DefaultModel string

	// Timeout is the maximum duration for a single HTTP request.
	// Defaults to 30 seconds.
	Timeout time.Duration

	// StreamTimeout is the maximum duration for a streaming HTTP request.
	// Defaults to 5 minutes.
	StreamTimeout time.Duration

	// MaxRetries is the number of retry attempts for transient failures.
	// Defaults to 2.
	MaxRetries int

	// CustomHeaders holds additional headers to include in every request.
	CustomHeaders map[string]string

	// MaxIdleConns controls the maximum number of idle keep-alive connections.
	// Defaults to 100.
	MaxIdleConns int

	// MaxIdleConnsPerHost controls idle connections per host.
	// Defaults to 10.
	MaxIdleConnsPerHost int

	// IdleConnTimeout is how long idle connections stay in the pool.
	// Defaults to 90 seconds.
	IdleConnTimeout time.Duration
}

// DefaultOpenAIBaseURL is the default base URL for the OpenAI API.
const DefaultOpenAIBaseURL = "https://api.openai.com"

// DefaultGeminiBaseURL is the default base URL for the Google Gemini API.
const DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com"

// Validate checks the configuration for required fields and applies defaults.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("provider name is required")
	}
	if c.APIFormat != provider.APIFormatOpenAI && c.APIFormat != provider.APIFormatGemini {
		return fmt.Errorf("unsupported API format: %q (must be %q or %q)", c.APIFormat, provider.APIFormatOpenAI, provider.APIFormatGemini)
	}
	if c.APIKey == "" {
		return fmt.Errorf("API key is required")
	}
	if len(c.Models) == 0 {
		return fmt.Errorf("at least one model must be configured")
	}

	// Apply defaults.
	if c.BaseURL == "" {
		switch c.APIFormat {
		case provider.APIFormatOpenAI:
			c.BaseURL = DefaultOpenAIBaseURL
		case provider.APIFormatGemini:
			c.BaseURL = DefaultGeminiBaseURL
		}
	}
	if c.DefaultModel == "" {
		c.DefaultModel = c.Models[0]
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.StreamTimeout == 0 {
		c.StreamTimeout = 5 * time.Minute
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 2
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 100
	}
	if c.MaxIdleConnsPerHost == 0 {
		c.MaxIdleConnsPerHost = 10
	}
	if c.IdleConnTimeout == 0 {
		c.IdleConnTimeout = 90 * time.Second
	}

	return nil
}
