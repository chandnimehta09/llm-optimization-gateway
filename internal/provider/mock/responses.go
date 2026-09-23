package mock

import (
	"fmt"
	"strings"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

// generateResponse produces a CompletionResponse based on the provider config and request.
func generateResponse(cfg MockProviderConfig, req *provider.CompletionRequest) *provider.CompletionResponse {
	content := buildResponseContent(cfg, req)
	promptTokens := countTokens(req)
	completionTokens := estimateTokens(content)

	return &provider.CompletionResponse{
		ID:       fmt.Sprintf("mock-%d", time.Now().UnixNano()),
		Model:    resolveModel(cfg, req),
		Provider: cfg.Name,
		Choices: []provider.Choice{
			{
				Index: 0,
				Message: provider.Message{
					Role:    provider.RoleAssistant,
					Content: content,
				},
				FinishReason: provider.FinishReasonStop,
			},
		},
		Usage: provider.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
		FinishReason: provider.FinishReasonStop,
		Created:      time.Now(),
	}
}

// buildResponseContent generates the response text based on the configured mode.
func buildResponseContent(cfg MockProviderConfig, req *provider.CompletionRequest) string {
	switch cfg.ResponseMode {
	case ResponseModeEcho:
		return echoLastUserMessage(req)
	case ResponseModeFixed:
		return cfg.FixedResponse
	case ResponseModeTokenCount:
		return generateTokenCountResponse(cfg.TargetTokenCount)
	default:
		return echoLastUserMessage(req)
	}
}

// echoLastUserMessage finds and returns the content of the last user message.
func echoLastUserMessage(req *provider.CompletionRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == provider.RoleUser {
			return req.Messages[i].Content
		}
	}
	return "no user message found"
}

// generateTokenCountResponse creates a response string with approximately the
// target number of tokens. Each word is roughly one token.
func generateTokenCountResponse(targetTokens int) string {
	if targetTokens <= 0 {
		targetTokens = 10
	}
	words := make([]string, targetTokens)
	vocabulary := []string{"the", "quick", "brown", "fox", "jumps", "over", "lazy", "dog", "and", "cat"}
	for i := 0; i < targetTokens; i++ {
		words[i] = vocabulary[i%len(vocabulary)]
	}
	return strings.Join(words, " ")
}

// resolveModel returns the model name from the request, falling back to the config default.
func resolveModel(cfg MockProviderConfig, req *provider.CompletionRequest) string {
	if req.Model != "" {
		return req.Model
	}
	return cfg.Model
}

// countTokens estimates the number of tokens in the request messages.
// Uses a simple word-count heuristic (1 word ≈ 1 token).
func countTokens(req *provider.CompletionRequest) int {
	total := 0
	for _, msg := range req.Messages {
		// Count content tokens.
		if msg.Content != "" {
			total += len(strings.Fields(msg.Content))
		}
		// Count multi-part text tokens.
		for _, part := range msg.Parts {
			if part.Type == "text" && part.Text != "" {
				total += len(strings.Fields(part.Text))
			}
		}
	}
	if total == 0 {
		total = 1
	}
	return total
}

// estimateTokens estimates the token count for a string using word count.
func estimateTokens(s string) int {
	count := len(strings.Fields(s))
	if count == 0 {
		count = 1
	}
	return count
}
