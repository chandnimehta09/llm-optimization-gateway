package prefix

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
)

// Backend is the prefix-cache-aware backend for the LLM optimization gateway.
// It constructs stable prefixes from system prompts, tool definitions, and
// common context, sends requests to a vLLM server, and records prefix cache
// telemetry.
type Backend struct {
	client    *Client
	config    *Config
	telemetry *Telemetry
}

// NewBackend creates a new prefix cache Backend.
func NewBackend(client *Client, config *Config, telemetry *Telemetry) (*Backend, error) {
	if client == nil {
		return nil, fmt.Errorf("prefix backend: client must not be nil")
	}
	if config == nil {
		return nil, fmt.Errorf("prefix backend: config must not be nil")
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("prefix backend: %w", err)
	}
	if telemetry == nil {
		return nil, fmt.Errorf("prefix backend: telemetry must not be nil")
	}
	return &Backend{
		client:    client,
		config:    config,
		telemetry: telemetry,
	}, nil
}

// ProcessResult holds the outcome of a prefix-cache-aware request.
type ProcessResult struct {
	// Response is the vLLM response.
	Response *ChatCompletionResponse
	// PrefixTokens is the estimated number of tokens in the stable prefix.
	PrefixTokens int64
	// CachedTokens is the number of tokens served from the prefix cache.
	CachedTokens int64
	// CacheHit indicates whether the prefix cache was used.
	CacheHit bool
	// HitRate is the fraction of prompt tokens served from cache.
	HitRate float64
	// Duration is the total request duration.
	Duration time.Duration
	// PrefixUsed indicates whether prefix optimization was applied.
	PrefixUsed bool
}

// Process handles a normalized request with prefix cache awareness.
// It checks the policy decision, constructs a stable prefix if eligible,
// sends the request to vLLM, and records telemetry.
func (b *Backend) Process(ctx context.Context, req *request.NormalizedRequest, decision *policy.PolicyDecision) (*ProcessResult, error) {
	if req == nil {
		return nil, fmt.Errorf("prefix backend: request must not be nil")
	}

	model := req.Model
	if model == "" {
		model = b.config.Model
	}
	if model == "" {
		return nil, fmt.Errorf("prefix backend: model must be specified in request or config")
	}

	// Check if prefix caching should be used for this request.
	usePrefixCache := b.shouldUsePrefixCache(req, decision)

	var messages []ChatMessage
	var prefixTokenEstimate int64

	if usePrefixCache {
		messages, prefixTokenEstimate = b.buildPrefixAwareMessages(req)
		b.telemetry.RecordPrefixLength(ctx, model, float64(prefixTokenEstimate))
	} else {
		messages = b.buildStandardMessages(req)
	}

	chatReq := &ChatCompletionRequest{
		Model:       model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stop:        req.Stop,
		Stream:      req.Stream,
	}

	start := time.Now()
	resp, err := b.client.ChatCompletion(ctx, chatReq)
	duration := time.Since(start)

	result := &ProcessResult{
		Response:   resp,
		PrefixUsed: usePrefixCache,
		Duration:   duration,
	}

	if err != nil {
		b.telemetry.RecordRequest(ctx, &RequestResult{
			Model:    model,
			Duration: duration,
			Error:    err,
		})
		return nil, fmt.Errorf("prefix backend: vllm request failed: %w", err)
	}

	// Extract prefix cache metrics from the response.
	result.CachedTokens = resp.CachedTokens()
	result.CacheHit = result.CachedTokens > 0
	result.HitRate = resp.PrefixCacheHitRate()
	result.PrefixTokens = prefixTokenEstimate

	// Record telemetry.
	b.telemetry.RecordRequest(ctx, &RequestResult{
		Model:             model,
		PrefixTokens:      prefixTokenEstimate,
		CachedTokens:      result.CachedTokens,
		TotalPromptTokens: resp.Usage.PromptTokens,
		Duration:          duration,
		CacheHit:          result.CacheHit,
	})

	return result, nil
}

// shouldUsePrefixCache determines whether prefix caching should be used for
// the given request based on configuration, policy, and request content.
func (b *Backend) shouldUsePrefixCache(req *request.NormalizedRequest, decision *policy.PolicyDecision) bool {
	// Disabled globally.
	if !b.config.Enabled {
		return false
	}

	// Policy says no prefix caching.
	if decision != nil && !decision.UsePrefixCache {
		return false
	}

	// Check if the request has enough prefix material.
	if !b.isPrefixEligible(req) {
		return false
	}

	return true
}

// isPrefixEligible checks whether the request has enough content to form a
// meaningful stable prefix.
func (b *Backend) isPrefixEligible(req *request.NormalizedRequest) bool {
	estimatedTokens := b.estimatePrefixTokens(req)
	return estimatedTokens >= int64(b.config.resolveMinPrefixTokens())
}

// estimatePrefixTokens estimates the number of tokens in the stable prefix
// components of a request. Uses a rough 4-characters-per-token heuristic.
func (b *Backend) estimatePrefixTokens(req *request.NormalizedRequest) int64 {
	var charCount int64

	if b.config.includeSystemPrompt() && req.SystemPrompt != "" {
		charCount += int64(len(req.SystemPrompt))
	}

	if b.config.includeToolDefs() && len(req.Tools) > 0 {
		toolJSON, err := json.Marshal(req.Tools)
		if err == nil {
			charCount += int64(len(toolJSON))
		}
	}

	if b.config.includeCommonCtx() {
		for _, msg := range req.Messages {
			if msg.Role == request.RoleSystem {
				charCount += int64(len(msg.Content))
			}
		}
	}

	// Rough estimate: 1 token ≈ 4 characters.
	return charCount / 4
}

// buildPrefixAwareMessages constructs messages with a stable prefix ordering:
// 1. System prompt (as first message)
// 2. Tool definitions (serialised into a system message)
// 3. Common context (additional system messages)
// 4. Conversation messages (user/assistant/tool turns)
//
// This ordering ensures that the first N tokens are identical across requests
// that share the same system prompt and tools, maximising prefix cache hits.
func (b *Backend) buildPrefixAwareMessages(req *request.NormalizedRequest) ([]ChatMessage, int64) {
	var messages []ChatMessage
	var prefixChars int64

	// 1. System prompt always first.
	if b.config.includeSystemPrompt() && req.SystemPrompt != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
		prefixChars += int64(len(req.SystemPrompt))
	}

	// 2. Tool definitions as a system message.
	if b.config.includeToolDefs() && len(req.Tools) > 0 {
		toolDesc := buildToolDescriptionMessage(req.Tools)
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: toolDesc,
		})
		prefixChars += int64(len(toolDesc))
	}

	// 3. Common context (other system messages from the conversation).
	if b.config.includeCommonCtx() {
		for _, msg := range req.Messages {
			if msg.Role == request.RoleSystem {
				messages = append(messages, ChatMessage{
					Role:    "system",
					Content: messageContent(msg),
				})
				prefixChars += int64(len(messageContent(msg)))
			}
		}
	}

	// 4. Non-system messages in original order.
	for _, msg := range req.Messages {
		if msg.Role == request.RoleSystem {
			continue // already included above
		}
		messages = append(messages, ChatMessage{
			Role:    msg.Role,
			Content: messageContent(msg),
		})
	}

	// Estimate prefix tokens (1 token ≈ 4 chars).
	prefixTokens := prefixChars / 4

	return messages, prefixTokens
}

// buildStandardMessages converts normalized messages to vLLM chat messages
// without prefix reordering. Used when prefix caching is not applicable.
func (b *Backend) buildStandardMessages(req *request.NormalizedRequest) []ChatMessage {
	var messages []ChatMessage

	// Include system prompt if present.
	if req.SystemPrompt != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	for _, msg := range req.Messages {
		messages = append(messages, ChatMessage{
			Role:    msg.Role,
			Content: messageContent(msg),
		})
	}

	return messages
}

// messageContent extracts the text content from a normalized message.
func messageContent(msg request.Message) string {
	if msg.Content != "" {
		return msg.Content
	}
	// Fall back to concatenating text parts.
	var parts []string
	for _, p := range msg.Parts {
		if p.Type == "text" && p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// buildToolDescriptionMessage creates a deterministic text description of tool
// definitions suitable for inclusion as a system message prefix.
func buildToolDescriptionMessage(tools []request.Tool) string {
	var sb strings.Builder
	sb.WriteString("Available tools:\n")
	for i, tool := range tools {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("- %s", tool.Function.Name))
		if tool.Function.Description != "" {
			sb.WriteString(fmt.Sprintf(": %s", tool.Function.Description))
		}
		if len(tool.Function.Parameters) > 0 {
			paramJSON, err := json.Marshal(tool.Function.Parameters)
			if err == nil {
				sb.WriteString(fmt.Sprintf("\n  Parameters: %s", string(paramJSON)))
			}
		}
	}
	return sb.String()
}

// ProcessWithoutPrefix sends a request to vLLM without any prefix optimisation.
// This is used when the request is not eligible for prefix caching or when
// prefix caching is disabled.
func (b *Backend) ProcessWithoutPrefix(ctx context.Context, req *request.NormalizedRequest) (*ProcessResult, error) {
	return b.Process(ctx, req, &policy.PolicyDecision{UsePrefixCache: false})
}
