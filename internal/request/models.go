// Package request provides request normalization and validation for the
// LLM optimization gateway. It transforms incoming requests from various
// formats (OpenAI, Gemini) into a canonical internal representation that
// can be routed to any supported provider backend.
package request

import (
	"encoding/json"
	"time"
)

// InputFormat identifies the format of the incoming request.
type InputFormat string

const (
	// InputFormatOpenAI indicates the request uses OpenAI's API format.
	InputFormatOpenAI InputFormat = "openai"
	// InputFormatGemini indicates the request uses Gemini's API format.
	InputFormatGemini InputFormat = "gemini"
)

// Role constants for normalized messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is the normalized, provider-agnostic representation of a chat message.
type Message struct {
	// Role is the message author's role (system, user, assistant, tool).
	Role string `json:"role"`
	// Content is the text content of the message.
	Content string `json:"content,omitempty"`
	// Parts holds multi-part content (text + images). If non-empty, it takes precedence over Content.
	Parts []ContentPart `json:"parts,omitempty"`
	// ToolCalls contains any tool invocations requested by the assistant.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID identifies which tool call this message is responding to (role=tool).
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// ContentPart represents a single part of a multi-part message.
type ContentPart struct {
	// Type is the content part type ("text", "image_url", or "image").
	Type string `json:"type"`
	// Text is the text content (when Type is "text").
	Text string `json:"text,omitempty"`
	// ImageURL is the image URL (when Type is "image_url" or "image").
	ImageURL string `json:"image_url,omitempty"`
	// MIMEType is the MIME type of the image (when Type is "image").
	MIMEType string `json:"mime_type,omitempty"`
}

// ToolCall represents a request from the model to invoke a tool.
type ToolCall struct {
	// ID is the unique identifier for this tool call.
	ID string `json:"id"`
	// Type is the tool call type (always "function" currently).
	Type string `json:"type"`
	// Function contains the function name and arguments.
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the details of a function invocation.
type FunctionCall struct {
	// Name is the function name to invoke.
	Name string `json:"name"`
	// Arguments is the JSON-encoded arguments string.
	Arguments string `json:"arguments"`
}

// Tool describes a tool available for the model to call.
type Tool struct {
	// Type is the tool type (always "function" currently).
	Type string `json:"type"`
	// Function describes the function signature and parameters.
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition describes a function that can be called by the model.
type FunctionDefinition struct {
	// Name is the function name.
	Name string `json:"name"`
	// Description explains what the function does.
	Description string `json:"description,omitempty"`
	// Parameters is the JSON schema describing the function parameters.
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// NormalizedRequest is the canonical internal representation of a chat
// completion request. It is provider-agnostic and carries metadata about
// the normalization process for observability.
type NormalizedRequest struct {
	// ID is a unique identifier for this request (generated if not present).
	ID string `json:"id"`
	// Model is the canonical model name after alias resolution and prefix stripping.
	Model string `json:"model"`
	// Messages is the normalized conversation history.
	Messages []Message `json:"messages"`
	// Temperature controls randomness (0.0–2.0). nil means use provider default.
	Temperature *float64 `json:"temperature,omitempty"`
	// MaxTokens is the maximum number of tokens to generate. nil means no limit.
	MaxTokens *int `json:"max_tokens,omitempty"`
	// TopP is nucleus sampling parameter (0.0–1.0). nil means use provider default.
	TopP *float64 `json:"top_p,omitempty"`
	// Stream indicates whether to stream the response.
	Stream bool `json:"stream,omitempty"`
	// Stop contains stop sequences.
	Stop []string `json:"stop,omitempty"`
	// Tools is the list of tools available for the model to call.
	Tools []Tool `json:"tools,omitempty"`
	// ToolChoice controls how the model selects tools.
	ToolChoice string `json:"tool_choice,omitempty"`
	// SystemPrompt is an optional system-level instruction.
	SystemPrompt string `json:"system_prompt,omitempty"`
	// ProviderHint indicates which provider this request should route to.
	ProviderHint string `json:"provider_hint,omitempty"`
	// OriginalRequest preserves the raw incoming request for logging/debugging.
	OriginalRequest json.RawMessage `json:"original_request,omitempty"`
	// InputFormat records the detected format of the incoming request.
	InputFormat InputFormat `json:"input_format"`
	// Changes tracks all normalization transformations applied.
	Changes []NormalizationChange `json:"changes,omitempty"`
	// Metadata holds additional context for caching, tracing, and routing.
	Metadata map[string]string `json:"metadata,omitempty"`
	// CreatedAt is the timestamp when normalization was performed.
	CreatedAt time.Time `json:"created_at"`
}

// NormalizationChange records a single transformation applied during normalization.
type NormalizationChange struct {
	// Field identifies which field was changed (e.g., "model", "messages[0].role").
	Field string `json:"field"`
	// Action describes what was done (e.g., "default_applied", "format_converted").
	Action string `json:"action"`
	// OldValue is the value before normalization (may be empty).
	OldValue string `json:"old_value,omitempty"`
	// NewValue is the value after normalization.
	NewValue string `json:"new_value,omitempty"`
}

// OpenAIRequest represents an incoming OpenAI-format chat completion request.
// Content can be either a string or an array of content parts, so we use
// json.RawMessage to handle both cases.
type OpenAIRequest struct {
	Model            string               `json:"model"`
	Messages         []OpenAIMessageInput `json:"messages"`
	Temperature      *float64             `json:"temperature,omitempty"`
	TopP             *float64             `json:"top_p,omitempty"`
	N                *int                 `json:"n,omitempty"`
	Stream           bool                 `json:"stream,omitempty"`
	Stop             []string             `json:"stop,omitempty"`
	MaxTokens        *int                 `json:"max_tokens,omitempty"`
	PresencePenalty  *float64             `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64             `json:"frequency_penalty,omitempty"`
	User             string               `json:"user,omitempty"`
	Tools            []OpenAIToolInput    `json:"tools,omitempty"`
	ToolChoice       interface{}          `json:"tool_choice,omitempty"`
}

// OpenAIMessageInput represents an incoming OpenAI message where content
// may be a string or an array of content parts.
type OpenAIMessageInput struct {
	Role       string                `json:"role"`
	Content    json.RawMessage       `json:"content,omitempty"`
	Name       string                `json:"name,omitempty"`
	ToolCalls  []OpenAIToolCallInput `json:"tool_calls,omitempty"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
}

// OpenAIContentPartInput represents a content part in OpenAI's multi-modal format.
type OpenAIContentPartInput struct {
	Type     string               `json:"type"`
	Text     string               `json:"text,omitempty"`
	ImageURL *OpenAIImageURLInput `json:"image_url,omitempty"`
}

// OpenAIImageURLInput represents an image URL in an OpenAI content part.
type OpenAIImageURLInput struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// OpenAIToolCallInput represents a tool call in the incoming request.
type OpenAIToolCallInput struct {
	ID       string                  `json:"id"`
	Type     string                  `json:"type"`
	Function OpenAIFunctionCallInput `json:"function"`
}

// OpenAIFunctionCallInput represents a function call in the incoming request.
type OpenAIFunctionCallInput struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIToolInput represents a tool definition in the incoming request.
type OpenAIToolInput struct {
	Type     string                        `json:"type"`
	Function OpenAIFunctionDefinitionInput `json:"function"`
}

// OpenAIFunctionDefinitionInput represents a function definition in the incoming request.
type OpenAIFunctionDefinitionInput struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// GeminiRequest represents an incoming Gemini-format request.
type GeminiRequest struct {
	Contents          []GeminiContentInput          `json:"contents"`
	SystemInstruction *GeminiSystemInstructionInput `json:"system_instruction,omitempty"`
	Tools             []GeminiToolInput             `json:"tools,omitempty"`
	GenerationConfig  *GeminiGenerationConfig       `json:"generation_config,omitempty"`
	Model             string                        `json:"model,omitempty"`
}

// GeminiContentInput represents a content entry in Gemini's format.
type GeminiContentInput struct {
	Role  string            `json:"role"`
	Parts []GeminiPartInput `json:"parts"`
}

// GeminiSystemInstructionInput represents a system instruction in Gemini's format.
type GeminiSystemInstructionInput struct {
	Parts []GeminiPartInput `json:"parts"`
}

// GeminiPartInput represents a content part in Gemini's format.
type GeminiPartInput struct {
	Text             string                       `json:"text,omitempty"`
	InlineData       *GeminiInlineDataInput       `json:"inline_data,omitempty"`
	FunctionCall     *GeminiFunctionCallInput     `json:"functionCall,omitempty"`
	FunctionResponse *GeminiFunctionResponseInput `json:"functionResponse,omitempty"`
}

// GeminiInlineDataInput represents inline binary data in Gemini's format.
type GeminiInlineDataInput struct {
	MIMEType string `json:"mime_type"`
	Data     string `json:"data"`
}

// GeminiFunctionCallInput represents a function call in Gemini's format.
type GeminiFunctionCallInput struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
}

// GeminiFunctionResponseInput represents a function response in Gemini's format.
type GeminiFunctionResponseInput struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response,omitempty"`
}

// GeminiToolInput represents a tool definition in Gemini's format.
type GeminiToolInput struct {
	FunctionDeclarations []GeminiFunctionDeclarationInput `json:"function_declarations,omitempty"`
}

// GeminiFunctionDeclarationInput represents a function declaration in Gemini's format.
type GeminiFunctionDeclarationInput struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// GeminiGenerationConfig holds generation parameters in Gemini's format.
type GeminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"top_p,omitempty"`
	TopK            *int     `json:"top_k,omitempty"`
	MaxOutputTokens *int     `json:"max_output_tokens,omitempty"`
	StopSequences   []string `json:"stop_sequences,omitempty"`
}
