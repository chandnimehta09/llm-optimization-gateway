// Package provider defines the abstraction layer for LLM provider backends.
// It provides a normalized, provider-agnostic interface for chat completions
// that supports both OpenAI and Gemini API formats.
package provider

import "time"

// APIFormat represents the API format a provider uses.
type APIFormat string

const (
	// APIFormatOpenAI represents the OpenAI API message format.
	APIFormatOpenAI APIFormat = "openai"
	// APIFormatGemini represents the Google Gemini API message format.
	APIFormatGemini APIFormat = "gemini"
)

// Role constants for normalized messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// FinishReason constants for completion responses.
const (
	FinishReasonStop      = "stop"
	FinishReasonLength    = "length"
	FinishReasonToolCalls = "tool_calls"
)

// Capability flags for provider feature detection.
type Capability uint32

const (
	// CapabilityStreaming indicates the provider supports streaming responses.
	CapabilityStreaming Capability = 1 << iota
	// CapabilityTools indicates the provider supports tool/function calling.
	CapabilityTools
	// CapabilityVision indicates the provider supports image/vision inputs.
	CapabilityVision
	// CapabilityJSON indicates the provider supports JSON mode output.
	CapabilityJSON
	// CapabilitySystemPrompt indicates the provider supports system prompts.
	CapabilitySystemPrompt
	// CapabilityMultiTurn indicates the provider supports multi-turn conversations.
	CapabilityMultiTurn
)

// ProviderConfig holds the configuration needed to initialize a provider.
type ProviderConfig struct {
	// Name is the unique identifier for this provider instance.
	Name string
	// APIKey is the authentication key for the provider API.
	APIKey string
	// BaseURL is the base URL for the provider's API endpoint.
	BaseURL string
	// Timeout is the maximum duration for API requests.
	Timeout time.Duration
	// MaxRetries is the number of retry attempts for transient failures.
	MaxRetries int
	// Capabilities is the set of supported features for this provider.
	Capabilities Capability
	// DefaultModel is the model to use when none is specified.
	DefaultModel string
	// Models is the list of models available from this provider.
	Models []string
	// Extra holds provider-specific configuration values.
	Extra map[string]string
}

// HasCapability checks whether the provider config includes the given capability.
func (c *ProviderConfig) HasCapability(cap Capability) bool {
	return c.Capabilities&cap != 0
}

// CompletionRequest is the normalized, provider-agnostic request for chat completions.
type CompletionRequest struct {
	// Model is the model identifier to use for the completion.
	Model string
	// Messages is the normalized conversation history.
	Messages []Message
	// Temperature controls randomness (0.0–2.0). nil means use provider default.
	Temperature *float64
	// MaxTokens is the maximum number of tokens to generate. nil means no limit.
	MaxTokens *int
	// TopP is nucleus sampling parameter (0.0–1.0). nil means use provider default.
	TopP *float64
	// SystemPrompt is an optional system-level instruction prepended to the conversation.
	SystemPrompt string
	// Tools is the list of tools available for the model to call.
	Tools []Tool
	// ToolChoice controls how the model selects tools ("auto", "none", or a specific tool name).
	ToolChoice string
	// Metadata holds additional context for caching, tracing, and routing.
	Metadata map[string]string
}

// CompletionResponse is the normalized, provider-agnostic response from a chat completion.
type CompletionResponse struct {
	// ID is the unique identifier for this completion.
	ID string
	// Model is the model that generated the completion.
	Model string
	// Provider is the name of the provider that served this request.
	Provider string
	// Choices contains the completion alternatives.
	Choices []Choice
	// Usage contains token consumption statistics.
	Usage Usage
	// FinishReason indicates why the completion stopped.
	FinishReason string
	// Created is the timestamp when the completion was generated.
	Created time.Time
}

// Choice represents a single completion alternative.
type Choice struct {
	// Index is the position of this choice in the response.
	Index int
	// Message is the generated message content.
	Message Message
	// FinishReason indicates why this particular choice stopped.
	FinishReason string
}

// Usage tracks token consumption for a completion request.
type Usage struct {
	// PromptTokens is the number of tokens in the input.
	PromptTokens int
	// CompletionTokens is the number of tokens generated.
	CompletionTokens int
	// TotalTokens is the sum of prompt and completion tokens.
	TotalTokens int
}

// StreamChunk represents a single piece of a streaming completion response.
type StreamChunk struct {
	// ID is the completion identifier (same across all chunks).
	ID string
	// Model is the model generating the stream.
	Model string
	// Provider is the name of the provider serving the stream.
	Provider string
	// Delta contains the incremental content for this chunk.
	Delta MessageDelta
	// FinishReason is set on the final chunk to indicate why generation stopped.
	FinishReason string
	// Usage is optionally populated on the final chunk with total usage stats.
	Usage *Usage
	// Error is set if an error occurred during streaming.
	Error error
}

// MessageDelta represents the incremental content in a stream chunk.
type MessageDelta struct {
	// Role is set on the first chunk to indicate the message role.
	Role string
	// Content is the incremental text content.
	Content string
	// ToolCalls contains incremental tool call data.
	ToolCalls []ToolCall
}

// Message is the normalized, provider-agnostic representation of a chat message.
type Message struct {
	// Role is the message author's role (system, user, assistant, tool).
	Role string
	// Content is the text content of the message.
	Content string
	// Parts holds multi-part content (text + images). If non-empty, it takes precedence over Content.
	Parts []ContentPart
	// ToolCalls contains any tool invocations requested by the assistant.
	ToolCalls []ToolCall
	// ToolCallID identifies which tool call this message is responding to (role=tool).
	ToolCallID string
}

// ContentPart represents a single part of a multi-part message.
type ContentPart struct {
	// Type is the content part type ("text" or "image").
	Type string
	// Text is the text content (when Type is "text").
	Text string
	// ImageURL is the image URL (when Type is "image").
	ImageURL string
	// MIMEType is the MIME type of the image (when Type is "image").
	MIMEType string
}

// ToolCall represents a request from the model to invoke a tool.
type ToolCall struct {
	// ID is the unique identifier for this tool call.
	ID string
	// Type is the tool call type (always "function" currently).
	Type string
	// Function contains the function name and arguments.
	Function FunctionCall
}

// FunctionCall contains the details of a function invocation.
type FunctionCall struct {
	// Name is the function name to invoke.
	Name string
	// Arguments is the JSON-encoded arguments string.
	Arguments string
}

// Tool describes a tool available for the model to call.
type Tool struct {
	// Type is the tool type (always "function" currently).
	Type string
	// Function describes the function signature and parameters.
	Function FunctionDefinition
}

// FunctionDefinition describes a function that can be called by the model.
type FunctionDefinition struct {
	// Name is the function name.
	Name string
	// Description explains what the function does.
	Description string
	// Parameters is the JSON schema describing the function parameters.
	Parameters map[string]interface{}
}

// OpenAI-specific message types for the MessageAdapter.

// OpenAIMessage represents a message in OpenAI's API format.
type OpenAIMessage struct {
	// Role is the message author's role.
	Role string `json:"role"`
	// Content is the text content (may be nil for tool calls).
	Content *string `json:"content,omitempty"`
	// MultiContent holds multi-part content for vision messages.
	MultiContent []OpenAIContentPart `json:"multi_content,omitempty"`
	// ToolCalls contains function calls requested by the assistant.
	ToolCalls []OpenAIToolCall `json:"tool_calls,omitempty"`
	// ToolCallID links a tool response to the originating call.
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// OpenAIContentPart represents a content part in OpenAI's multi-modal format.
type OpenAIContentPart struct {
	// Type is "text" or "image_url".
	Type string `json:"type"`
	// Text is the text content.
	Text string `json:"text,omitempty"`
	// ImageURL holds image URL details.
	ImageURL *OpenAIImageURL `json:"image_url,omitempty"`
}

// OpenAIImageURL represents an image URL in OpenAI's format.
type OpenAIImageURL struct {
	// URL is the image URL.
	URL string `json:"url"`
}

// OpenAIToolCall represents a tool call in OpenAI's format.
type OpenAIToolCall struct {
	// ID is the tool call identifier.
	ID string `json:"id"`
	// Type is always "function".
	Type string `json:"type"`
	// Function contains the call details.
	Function OpenAIFunctionCall `json:"function"`
}

// OpenAIFunctionCall represents a function call in OpenAI's format.
type OpenAIFunctionCall struct {
	// Name is the function name.
	Name string `json:"name"`
	// Arguments is the JSON-encoded arguments.
	Arguments string `json:"arguments"`
}

// Gemini-specific message types for the MessageAdapter.

// GeminiContent represents the content structure in Gemini's API format.
type GeminiContent struct {
	// Contents is the list of content entries in the conversation.
	Contents []GeminiContentEntry `json:"contents"`
	// SystemInstruction holds the system prompt if present.
	SystemInstruction *GeminiSystemInstruction `json:"system_instruction,omitempty"`
}

// GeminiSystemInstruction represents a system instruction in Gemini's format.
type GeminiSystemInstruction struct {
	// Parts contains the system instruction text.
	Parts []GeminiPart `json:"parts"`
}

// GeminiContentEntry represents a single turn in a Gemini conversation.
type GeminiContentEntry struct {
	// Role is "user" or "model" in Gemini's format.
	Role string `json:"role"`
	// Parts contains the content parts for this turn.
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a content part in Gemini's format.
type GeminiPart struct {
	// Text is the text content.
	Text string `json:"text,omitempty"`
	// InlineData holds inline binary data (e.g., images).
	InlineData *GeminiInlineData `json:"inline_data,omitempty"`
	// FunctionCall holds a function call from the model.
	FunctionCall *GeminiFunctionCall `json:"functionCall,omitempty"`
	// FunctionResponse holds a function response from the user.
	FunctionResponse *GeminiFunctionResponse `json:"functionResponse,omitempty"`
}

// GeminiInlineData holds binary data inline in Gemini's format.
type GeminiInlineData struct {
	// MIMEType is the MIME type of the data.
	MIMEType string `json:"mime_type"`
	// Data is the base64-encoded data.
	Data string `json:"data"`
}

// GeminiFunctionCall represents a function call in Gemini's format.
type GeminiFunctionCall struct {
	// Name is the function name.
	Name string `json:"name"`
	// Args is the function arguments as a map.
	Args map[string]interface{} `json:"args"`
}

// GeminiFunctionResponse represents a function response in Gemini's format.
type GeminiFunctionResponse struct {
	// Name is the function name being responded to.
	Name string `json:"name"`
	// Response holds the function result.
	Response map[string]interface{} `json:"response"`
}
