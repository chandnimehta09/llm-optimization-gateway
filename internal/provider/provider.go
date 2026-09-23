package provider

import "context"

// Provider is the core abstraction for LLM backends. Each provider implementation
// (OpenAI, Gemini, etc.) must satisfy this interface, enabling the gateway to
// route requests to any supported backend through a unified API.
type Provider interface {
	// Name returns the unique identifier for this provider instance.
	Name() string

	// Complete sends a synchronous completion request and returns the full response.
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

	// CompleteStream sends a streaming completion request and returns a channel
	// that emits incremental response chunks. The channel is closed when the
	// stream ends or an error occurs. Errors during streaming are delivered
	// via the StreamChunk.Error field.
	CompleteStream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error)

	// Models returns the list of model identifiers available from this provider.
	Models() []string

	// IsHealthy checks whether the provider is currently reachable and operational.
	IsHealthy(ctx context.Context) bool

	// APIFormat returns the native API format this provider uses.
	APIFormat() APIFormat
}
