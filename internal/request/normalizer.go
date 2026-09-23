package request

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Normalizer transforms incoming requests from various formats into the
// canonical NormalizedRequest. It tracks all changes for observability
// and preserves the original request for debugging.
type Normalizer struct {
	config *NormalizerConfig
}

// NewNormalizer creates a new Normalizer with the given configuration.
// If config is nil, DefaultNormalizerConfig() is used.
func NewNormalizer(config *NormalizerConfig) *Normalizer {
	if config == nil {
		config = DefaultNormalizerConfig()
	}
	return &Normalizer{config: config}
}

// NormalizeOpenAI normalizes an OpenAI-format request into the canonical format.
func (n *Normalizer) NormalizeOpenAI(req *OpenAIRequest) (*NormalizedRequest, error) {
	// Validate first.
	vr := ValidateOpenAIRequest(req)
	if !vr.Valid {
		return nil, fmt.Errorf("validation failed: %s", formatValidationErrors(vr.Errors))
	}

	// Preserve original request.
	originalJSON, _ := json.Marshal(req)

	nr := &NormalizedRequest{
		ID:              generateRequestID(),
		InputFormat:     InputFormatOpenAI,
		OriginalRequest: originalJSON,
		Stream:          req.Stream,
		Stop:            req.Stop,
		Metadata:        make(map[string]string),
		CreatedAt:       time.Now().UTC(),
	}

	if req.User != "" {
		nr.Metadata["user"] = req.User
	}

	// Normalize model.
	nr.Model, nr.ProviderHint = n.normalizeModel(req.Model, nr)

	// Normalize messages.
	messages, err := n.normalizeOpenAIMessages(req.Messages, nr)
	if err != nil {
		return nil, fmt.Errorf("message normalization failed: %w", err)
	}
	nr.Messages = messages

	// Normalize parameters.
	n.normalizeParameters(req.Temperature, req.TopP, req.MaxTokens, nr)

	// Normalize tools.
	if len(req.Tools) > 0 {
		nr.Tools = n.normalizeOpenAITools(req.Tools, nr)
	}

	// Normalize tool choice.
	if req.ToolChoice != nil {
		nr.ToolChoice = normalizeToolChoice(req.ToolChoice)
	}

	// Extract system prompt from messages.
	n.extractSystemPrompt(nr)

	return nr, nil
}

// NormalizeGemini normalizes a Gemini-format request into the canonical format.
func (n *Normalizer) NormalizeGemini(req *GeminiRequest) (*NormalizedRequest, error) {
	// Validate first.
	vr := ValidateGeminiRequest(req)
	if !vr.Valid {
		return nil, fmt.Errorf("validation failed: %s", formatValidationErrors(vr.Errors))
	}

	// Preserve original request.
	originalJSON, _ := json.Marshal(req)

	nr := &NormalizedRequest{
		ID:              generateRequestID(),
		InputFormat:     InputFormatGemini,
		OriginalRequest: originalJSON,
		Metadata:        make(map[string]string),
		CreatedAt:       time.Now().UTC(),
	}

	// Normalize model.
	if req.Model != "" {
		nr.Model, nr.ProviderHint = n.normalizeModel(req.Model, nr)
	}
	if nr.ProviderHint == "" {
		nr.ProviderHint = "gemini"
	}

	// Convert system instruction.
	if req.SystemInstruction != nil {
		var parts []string
		for _, p := range req.SystemInstruction.Parts {
			if p.Text != "" {
				parts = append(parts, p.Text)
			}
		}
		nr.SystemPrompt = strings.Join(parts, "\n")
	}

	// Convert Gemini contents to normalized messages.
	messages, err := n.normalizeGeminiContents(req.Contents, nr)
	if err != nil {
		return nil, fmt.Errorf("message normalization failed: %w", err)
	}
	nr.Messages = messages

	// Normalize generation config parameters.
	if req.GenerationConfig != nil {
		gc := req.GenerationConfig
		n.normalizeParameters(gc.Temperature, gc.TopP, gc.MaxOutputTokens, nr)
		if len(gc.StopSequences) > 0 {
			nr.Stop = gc.StopSequences
		}
	} else {
		n.normalizeParameters(nil, nil, nil, nr)
	}

	// Normalize Gemini tools.
	if len(req.Tools) > 0 {
		nr.Tools = n.normalizeGeminiTools(req.Tools, nr)
	}

	return nr, nil
}

// normalizeModel strips provider prefixes, resolves aliases, and detects provider hints.
func (n *Normalizer) normalizeModel(model string, nr *NormalizedRequest) (string, string) {
	original := model

	// Strip provider prefix.
	cleanModel, providerHint := StripProviderPrefix(model)
	if providerHint != "" {
		nr.Changes = append(nr.Changes, NormalizationChange{
			Field:    "model",
			Action:   "provider_prefix_stripped",
			OldValue: original,
			NewValue: cleanModel,
		})
	}

	// Resolve alias.
	canonical := ResolveModelAlias(cleanModel, n.config.ModelAliases)
	if canonical != cleanModel {
		nr.Changes = append(nr.Changes, NormalizationChange{
			Field:    "model",
			Action:   "alias_resolved",
			OldValue: cleanModel,
			NewValue: canonical,
		})
		cleanModel = canonical
	}

	// Detect provider from model name if no explicit prefix.
	if providerHint == "" {
		providerHint = DetectProviderFromModel(cleanModel)
		if providerHint != "" {
			nr.Changes = append(nr.Changes, NormalizationChange{
				Field:    "provider_hint",
				Action:   "detected_from_model",
				NewValue: providerHint,
			})
		}
	}

	return cleanModel, providerHint
}

// normalizeOpenAIMessages converts OpenAI-format messages to normalized format.
func (n *Normalizer) normalizeOpenAIMessages(msgs []OpenAIMessageInput, nr *NormalizedRequest) ([]Message, error) {
	result := make([]Message, 0, len(msgs))

	for i, msg := range msgs {
		normalized := Message{
			ToolCallID: msg.ToolCallID,
		}

		// Normalize role to lowercase.
		role := strings.ToLower(msg.Role)
		if role != msg.Role {
			nr.Changes = append(nr.Changes, NormalizationChange{
				Field:    fmt.Sprintf("messages[%d].role", i),
				Action:   "lowercased",
				OldValue: msg.Role,
				NewValue: role,
			})
		}
		normalized.Role = role

		// Normalize content - handle string or array format.
		if msg.Content != nil && len(msg.Content) > 0 {
			parts, contentStr, err := parseOpenAIContent(msg.Content)
			if err != nil {
				return nil, fmt.Errorf("messages[%d].content: %w", i, err)
			}

			if contentStr != "" {
				// Convert string content to parts format.
				normalized.Parts = []ContentPart{
					{Type: "text", Text: contentStr},
				}
				nr.Changes = append(nr.Changes, NormalizationChange{
					Field:  fmt.Sprintf("messages[%d].content", i),
					Action: "string_to_array",
				})
			} else if len(parts) > 0 {
				normalized.Parts = parts
			}
		}

		// Convert tool calls.
		if len(msg.ToolCalls) > 0 {
			normalized.ToolCalls = make([]ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				normalized.ToolCalls[j] = ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		result = append(result, normalized)
	}

	return result, nil
}

// parseOpenAIContent handles the dual format of OpenAI content: string or array.
// Returns either parsed parts or a content string.
func parseOpenAIContent(raw json.RawMessage) ([]ContentPart, string, error) {
	// Try string first.
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return nil, str, nil
	}

	// Try array of content parts.
	var parts []OpenAIContentPartInput
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, "", fmt.Errorf("content must be a string or array of content parts: %w", err)
	}

	normalized := make([]ContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Type {
		case "text":
			normalized = append(normalized, ContentPart{
				Type: "text",
				Text: p.Text,
			})
		case "image_url":
			cp := ContentPart{
				Type: "image_url",
			}
			if p.ImageURL != nil {
				cp.ImageURL = p.ImageURL.URL
			}
			normalized = append(normalized, cp)
		default:
			return nil, "", fmt.Errorf("unsupported content part type %q", p.Type)
		}
	}

	return normalized, "", nil
}

// normalizeGeminiContents converts Gemini content entries to normalized messages.
func (n *Normalizer) normalizeGeminiContents(contents []GeminiContentInput, nr *NormalizedRequest) ([]Message, error) {
	result := make([]Message, 0, len(contents))

	for i, entry := range contents {
		// Map Gemini roles to normalized roles.
		role := strings.ToLower(entry.Role)
		if role == "model" {
			role = "assistant"
			nr.Changes = append(nr.Changes, NormalizationChange{
				Field:    fmt.Sprintf("messages[%d].role", i),
				Action:   "gemini_role_mapped",
				OldValue: entry.Role,
				NewValue: role,
			})
		}

		// Check for function responses (these become tool messages).
		for _, part := range entry.Parts {
			if part.FunctionResponse != nil {
				respJSON, err := json.Marshal(part.FunctionResponse.Response)
				if err != nil {
					return nil, fmt.Errorf("contents[%d]: failed to marshal function response: %w", i, err)
				}
				result = append(result, Message{
					Role:       "tool",
					Content:    string(respJSON),
					ToolCallID: part.FunctionResponse.Name,
				})
				nr.Changes = append(nr.Changes, NormalizationChange{
					Field:  fmt.Sprintf("messages[%d]", len(result)-1),
					Action: "gemini_function_response_to_tool",
				})
			}
		}

		// Check if all parts are function responses; if so, skip further processing.
		allFunctionResponses := true
		for _, part := range entry.Parts {
			if part.FunctionResponse == nil {
				allFunctionResponses = false
				break
			}
		}
		if allFunctionResponses && len(entry.Parts) > 0 {
			continue
		}

		msg := Message{
			Role: role,
		}

		// Process non-function-response parts.
		var textParts []string
		for _, part := range entry.Parts {
			if part.FunctionResponse != nil {
				continue
			}
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.InlineData != nil {
				msg.Parts = append(msg.Parts, ContentPart{
					Type:     "image",
					MIMEType: part.InlineData.MIMEType,
					ImageURL: "data:" + part.InlineData.MIMEType + ";base64," + part.InlineData.Data,
				})
			}
			if part.FunctionCall != nil {
				argsJSON, err := json.Marshal(part.FunctionCall.Args)
				if err != nil {
					return nil, fmt.Errorf("contents[%d]: failed to marshal function call args: %w", i, err)
				}
				msg.ToolCalls = append(msg.ToolCalls, ToolCall{
					ID:   fmt.Sprintf("call_%s", part.FunctionCall.Name),
					Type: "function",
					Function: FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}

		// Set content from text parts.
		if len(msg.Parts) > 0 {
			// Multi-part: prepend text parts.
			for _, t := range textParts {
				msg.Parts = append([]ContentPart{{Type: "text", Text: t}}, msg.Parts...)
			}
		} else if len(textParts) > 0 {
			// Text-only: convert to parts format for consistency.
			for _, t := range textParts {
				msg.Parts = append(msg.Parts, ContentPart{Type: "text", Text: t})
			}
			nr.Changes = append(nr.Changes, NormalizationChange{
				Field:  fmt.Sprintf("messages[%d].content", len(result)),
				Action: "gemini_text_to_parts",
			})
		}

		result = append(result, msg)
	}

	return result, nil
}

// normalizeParameters applies defaults and validates parameter ranges.
func (n *Normalizer) normalizeParameters(temperature *float64, topP *float64, maxTokens *int, nr *NormalizedRequest) {
	// Temperature.
	if temperature != nil {
		nr.Temperature = temperature
	} else if n.config.ApplyDefaultTemperature {
		t := n.config.DefaultTemperature
		nr.Temperature = &t
		nr.Changes = append(nr.Changes, NormalizationChange{
			Field:    "temperature",
			Action:   "default_applied",
			NewValue: fmt.Sprintf("%v", t),
		})
	}

	// TopP.
	if topP != nil {
		nr.TopP = topP
	} else if n.config.ApplyDefaultTopP {
		p := n.config.DefaultTopP
		nr.TopP = &p
		nr.Changes = append(nr.Changes, NormalizationChange{
			Field:    "top_p",
			Action:   "default_applied",
			NewValue: fmt.Sprintf("%v", p),
		})
	}

	// MaxTokens.
	if maxTokens != nil {
		nr.MaxTokens = maxTokens
	}
}

// normalizeOpenAITools converts OpenAI tool definitions to the normalized format.
func (n *Normalizer) normalizeOpenAITools(tools []OpenAIToolInput, nr *NormalizedRequest) []Tool {
	result := make([]Tool, 0, len(tools))
	for i, t := range tools {
		toolType := t.Type
		if toolType == "" {
			toolType = "function"
			nr.Changes = append(nr.Changes, NormalizationChange{
				Field:    fmt.Sprintf("tools[%d].type", i),
				Action:   "default_applied",
				NewValue: "function",
			})
		}

		// Normalize function name: trim whitespace.
		funcName := strings.TrimSpace(t.Function.Name)
		if funcName != t.Function.Name {
			nr.Changes = append(nr.Changes, NormalizationChange{
				Field:    fmt.Sprintf("tools[%d].function.name", i),
				Action:   "whitespace_trimmed",
				OldValue: t.Function.Name,
				NewValue: funcName,
			})
		}

		result = append(result, Tool{
			Type: toolType,
			Function: FunctionDefinition{
				Name:        funcName,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}
	return result
}

// normalizeGeminiTools converts Gemini tool definitions to the normalized format.
func (n *Normalizer) normalizeGeminiTools(tools []GeminiToolInput, nr *NormalizedRequest) []Tool {
	result := make([]Tool, 0)
	for _, t := range tools {
		for _, fd := range t.FunctionDeclarations {
			funcName := strings.TrimSpace(fd.Name)
			nr.Changes = append(nr.Changes, NormalizationChange{
				Field:  fmt.Sprintf("tools.function_declarations.%s", funcName),
				Action: "gemini_tool_converted",
			})
			result = append(result, Tool{
				Type: "function",
				Function: FunctionDefinition{
					Name:        funcName,
					Description: fd.Description,
					Parameters:  fd.Parameters,
				},
			})
		}
	}
	return result
}

// extractSystemPrompt pulls system messages out of the messages array
// and sets them as the SystemPrompt field.
func (n *Normalizer) extractSystemPrompt(nr *NormalizedRequest) {
	if nr.SystemPrompt != "" {
		return // Already set (e.g., from Gemini system_instruction).
	}

	var systemParts []string
	filtered := make([]Message, 0, len(nr.Messages))
	for _, msg := range nr.Messages {
		if msg.Role == "system" {
			text := ""
			if len(msg.Parts) > 0 {
				for _, p := range msg.Parts {
					if p.Type == "text" {
						text += p.Text
					}
				}
			} else {
				text = msg.Content
			}
			if text != "" {
				systemParts = append(systemParts, text)
			}
		} else {
			filtered = append(filtered, msg)
		}
	}

	if len(systemParts) > 0 {
		nr.SystemPrompt = strings.Join(systemParts, "\n")
		nr.Messages = filtered
		nr.Changes = append(nr.Changes, NormalizationChange{
			Field:  "system_prompt",
			Action: "extracted_from_messages",
		})
	}
}

// normalizeToolChoice converts the flexible tool_choice value to a string.
func normalizeToolChoice(tc interface{}) string {
	switch v := tc.(type) {
	case string:
		return v
	case map[string]interface{}:
		// Handle {"type": "function", "function": {"name": "..."}} format.
		if fn, ok := v["function"]; ok {
			if fnMap, ok := fn.(map[string]interface{}); ok {
				if name, ok := fnMap["name"].(string); ok {
					return name
				}
			}
		}
		return "auto"
	default:
		return "auto"
	}
}

// generateRequestID creates a unique request identifier.
func generateRequestID() string {
	return "req_" + uuid.New().String()
}

// formatValidationErrors formats a list of validation errors into a readable string.
func formatValidationErrors(errs []ValidationError) string {
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}
