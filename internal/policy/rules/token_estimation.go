// Package rules implements policy rules for the LLM optimization gateway.
// It provides token estimation and context size evaluation to determine
// when optimization strategies should be applied.
package rules

import (
	"encoding/json"

	"github.com/llm-optimization-gateway/internal/request"
)

const (
	// CharsPerToken is the approximate number of characters per token for English text.
	// This is a widely used heuristic: ~4 characters ≈ 1 token for most English text.
	CharsPerToken = 4

	// MessageOverheadTokens is the estimated token overhead per message for role markers,
	// delimiters, and formatting that LLM APIs add around each message.
	MessageOverheadTokens = 4

	// ToolDefinitionOverheadTokens is the estimated token overhead per tool definition
	// for the JSON schema wrapper, type annotations, and delimiters.
	ToolDefinitionOverheadTokens = 10

	// SystemPromptOverheadTokens is the token overhead for system prompt framing
	// (e.g., system role marker and delimiters).
	SystemPromptOverheadTokens = 4

	// BaseRequestOverheadTokens accounts for the baseline token overhead of any
	// request: model identifier encoding, API framing, and parameter serialization.
	BaseRequestOverheadTokens = 10
)

// TokenEstimate holds a detailed breakdown of estimated token usage.
type TokenEstimate struct {
	// MessageTokens is the estimated tokens from message content.
	MessageTokens int `json:"message_tokens"`
	// MessageOverhead is the estimated tokens from message framing.
	MessageOverhead int `json:"message_overhead"`
	// ToolTokens is the estimated tokens from tool definitions.
	ToolTokens int `json:"tool_tokens"`
	// ToolOverhead is the estimated tokens from tool definition framing.
	ToolOverhead int `json:"tool_overhead"`
	// SystemPromptTokens is the estimated tokens from the system prompt.
	SystemPromptTokens int `json:"system_prompt_tokens"`
	// SystemPromptOverhead is the estimated tokens from system prompt framing.
	SystemPromptOverhead int `json:"system_prompt_overhead"`
	// BaseOverhead is the baseline request overhead in tokens.
	BaseOverhead int `json:"base_overhead"`
	// TotalTokens is the sum of all estimated tokens.
	TotalTokens int `json:"total_tokens"`
}

// EstimateTokens produces a detailed token estimate for the given normalized request.
// It accounts for message content, tool definitions, system prompt, and all
// associated protocol overhead.
func EstimateTokens(req *request.NormalizedRequest) TokenEstimate {
	if req == nil {
		return TokenEstimate{BaseOverhead: BaseRequestOverheadTokens, TotalTokens: BaseRequestOverheadTokens}
	}

	est := TokenEstimate{
		BaseOverhead: BaseRequestOverheadTokens,
	}

	// Estimate message tokens.
	for _, msg := range req.Messages {
		est.MessageTokens += estimateMessageContentTokens(&msg)
		est.MessageOverhead += MessageOverheadTokens
	}

	// Estimate tool definition tokens.
	for _, tool := range req.Tools {
		est.ToolTokens += estimateToolTokens(&tool)
		est.ToolOverhead += ToolDefinitionOverheadTokens
	}

	// Estimate system prompt tokens.
	if req.SystemPrompt != "" {
		est.SystemPromptTokens = estimateTextTokens(req.SystemPrompt)
		est.SystemPromptOverhead = SystemPromptOverheadTokens
	}

	est.TotalTokens = est.MessageTokens + est.MessageOverhead +
		est.ToolTokens + est.ToolOverhead +
		est.SystemPromptTokens + est.SystemPromptOverhead +
		est.BaseOverhead

	return est
}

// EstimateTextTokens returns the estimated token count for a plain text string.
// It uses the 4 chars ≈ 1 token heuristic and rounds up.
func EstimateTextTokens(text string) int {
	return estimateTextTokens(text)
}

// estimateTextTokens calculates approximate tokens for a text string.
func estimateTextTokens(text string) int {
	n := len(text)
	if n == 0 {
		return 0
	}
	return (n + CharsPerToken - 1) / CharsPerToken // round up
}

// estimateMessageContentTokens estimates tokens for a single message's content.
func estimateMessageContentTokens(msg *request.Message) int {
	tokens := 0

	// Multi-part content takes precedence.
	if len(msg.Parts) > 0 {
		for _, part := range msg.Parts {
			switch part.Type {
			case "text":
				tokens += estimateTextTokens(part.Text)
			case "image_url", "image":
				// Images are typically encoded as a fixed token cost by providers.
				// Use a conservative estimate for the URL/reference.
				tokens += estimateTextTokens(part.ImageURL)
			}
		}
	} else if msg.Content != "" {
		tokens += estimateTextTokens(msg.Content)
	}

	// Tool calls in assistant messages.
	for _, tc := range msg.ToolCalls {
		tokens += estimateTextTokens(tc.Function.Name)
		tokens += estimateTextTokens(tc.Function.Arguments)
		tokens += MessageOverheadTokens // overhead per tool call
	}

	// Tool call ID for tool response messages.
	if msg.ToolCallID != "" {
		tokens += estimateTextTokens(msg.ToolCallID)
	}

	return tokens
}

// estimateToolTokens estimates tokens for a single tool definition.
func estimateToolTokens(tool *request.Tool) int {
	tokens := 0
	tokens += estimateTextTokens(tool.Function.Name)
	tokens += estimateTextTokens(tool.Function.Description)

	// Serialize parameters to estimate their token cost.
	if len(tool.Function.Parameters) > 0 {
		paramJSON, err := json.Marshal(tool.Function.Parameters)
		if err == nil {
			tokens += estimateTextTokens(string(paramJSON))
		}
	}

	return tokens
}
