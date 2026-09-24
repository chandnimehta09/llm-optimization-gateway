// Package mock provides a mock LLM provider for testing and development.
// It returns deterministic responses based on configuration, avoiding
// actual LLM API calls and their associated costs.
package mock

import "time"

// ResponseMode controls how the mock provider generates responses.
type ResponseMode string

const (
	// ResponseModeEcho returns the last user message as the response.
	ResponseModeEcho ResponseMode = "echo"
	// ResponseModeFixed returns a preconfigured fixed response.
	ResponseModeFixed ResponseMode = "fixed"
	// ResponseModeTokenCount returns a response with a specific token count.
	ResponseModeTokenCount ResponseMode = "token_count"
)

// MockProviderConfig configures the mock provider's behavior.
type MockProviderConfig struct {
	// Name is the unique identifier for this provider instance.
	// Defaults to "mock" if empty.
	Name string

	// Model is the model name reported in responses.
	// Defaults to "mock-model" if empty.
	Model string

	// ResponseMode controls the response generation strategy.
	// Defaults to ResponseModeEcho if empty.
	ResponseMode ResponseMode

	// FixedResponse is the response returned when ResponseMode is ResponseModeFixed.
	FixedResponse string

	// TargetTokenCount is the desired token count when ResponseMode is ResponseModeTokenCount.
	// The provider generates a response approximately this many tokens long.
	TargetTokenCount int

	// Latency is the simulated delay before returning a response.
	// Zero means no delay.
	Latency time.Duration

	// ErrorRate is the probability (0.0–1.0) of returning an error.
	// 0.0 means never error, 1.0 means always error.
	ErrorRate float64

	// StreamChunkSize is the number of characters per chunk in streaming mode.
	// Defaults to 5 if zero.
	StreamChunkSize int

	// StreamChunkDelay is the delay between streaming chunks.
	// Defaults to 10ms if zero.
	StreamChunkDelay time.Duration
}

// withDefaults returns a copy of the config with default values applied.
func (c MockProviderConfig) withDefaults() MockProviderConfig {
	if c.Name == "" {
		c.Name = "mock"
	}
	if c.Model == "" {
		c.Model = "mock-model"
	}
	if c.ResponseMode == "" {
		c.ResponseMode = ResponseModeEcho
	}
	if c.StreamChunkSize <= 0 {
		c.StreamChunkSize = 5
	}
	if c.StreamChunkDelay <= 0 {
		c.StreamChunkDelay = 10 * time.Millisecond
	}
	return c
}
