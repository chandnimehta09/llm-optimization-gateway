package provider

import (
	"encoding/json"
	"fmt"
)

// MessageAdapter converts messages between the normalized format and
// provider-specific API formats. This enables the gateway to accept
// requests in one format and route them to any backend.
type MessageAdapter interface {
	// ToOpenAI converts normalized messages to OpenAI's API format.
	ToOpenAI(messages []Message) ([]OpenAIMessage, error)
	// ToGemini converts normalized messages to Gemini's API format.
	ToGemini(messages []Message) (*GeminiContent, error)
	// FromOpenAI converts OpenAI-format messages to the normalized format.
	FromOpenAI(msgs []OpenAIMessage) ([]Message, error)
	// FromGemini converts Gemini-format content to the normalized format.
	FromGemini(content *GeminiContent) ([]Message, error)
}

// DefaultMessageAdapter is the standard implementation of MessageAdapter.
type DefaultMessageAdapter struct{}

// NewMessageAdapter creates a new DefaultMessageAdapter.
func NewMessageAdapter() *DefaultMessageAdapter {
	return &DefaultMessageAdapter{}
}

// ToOpenAI converts normalized messages to OpenAI's API format.
func (a *DefaultMessageAdapter) ToOpenAI(messages []Message) ([]OpenAIMessage, error) {
	result := make([]OpenAIMessage, 0, len(messages))

	for i, msg := range messages {
		oaiMsg := OpenAIMessage{
			Role:       msg.Role,
			ToolCallID: msg.ToolCallID,
		}

		// Handle multi-part content.
		if len(msg.Parts) > 0 {
			parts := make([]OpenAIContentPart, 0, len(msg.Parts))
			for _, p := range msg.Parts {
				switch p.Type {
				case "text":
					parts = append(parts, OpenAIContentPart{
						Type: "text",
						Text: p.Text,
					})
				case "image":
					parts = append(parts, OpenAIContentPart{
						Type:     "image_url",
						ImageURL: &OpenAIImageURL{URL: p.ImageURL},
					})
				default:
					return nil, fmt.Errorf("message %d: unsupported content part type %q", i, p.Type)
				}
			}
			oaiMsg.MultiContent = parts
		} else {
			content := msg.Content
			oaiMsg.Content = &content
		}

		// Convert tool calls.
		if len(msg.ToolCalls) > 0 {
			oaiMsg.ToolCalls = make([]OpenAIToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				oaiMsg.ToolCalls[j] = OpenAIToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: OpenAIFunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		result = append(result, oaiMsg)
	}

	return result, nil
}

// ToGemini converts normalized messages to Gemini's API format.
// Gemini uses a different structure: system prompts are separate,
// roles are "user" and "model" (not "assistant"), and tool
// calls/responses are inline parts.
func (a *DefaultMessageAdapter) ToGemini(messages []Message) (*GeminiContent, error) {
	gc := &GeminiContent{
		Contents: make([]GeminiContentEntry, 0),
	}

	for i, msg := range messages {
		switch msg.Role {
		case RoleSystem:
			// Gemini handles system prompts as a separate instruction.
			gc.SystemInstruction = &GeminiSystemInstruction{
				Parts: []GeminiPart{{Text: msg.Content}},
			}

		case RoleUser:
			entry := GeminiContentEntry{
				Role:  "user",
				Parts: a.messageToGeminiParts(msg),
			}
			gc.Contents = append(gc.Contents, entry)

		case RoleAssistant:
			entry := GeminiContentEntry{
				Role:  "model",
				Parts: make([]GeminiPart, 0),
			}
			// Add text content if present.
			if msg.Content != "" {
				entry.Parts = append(entry.Parts, GeminiPart{Text: msg.Content})
			}
			// Convert tool calls to Gemini function calls.
			for _, tc := range msg.ToolCalls {
				args := make(map[string]interface{})
				if tc.Function.Arguments != "" {
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
						return nil, fmt.Errorf("message %d: failed to parse tool call arguments: %w", i, err)
					}
				}
				entry.Parts = append(entry.Parts, GeminiPart{
					FunctionCall: &GeminiFunctionCall{
						Name: tc.Function.Name,
						Args: args,
					},
				})
			}
			gc.Contents = append(gc.Contents, entry)

		case RoleTool:
			// Gemini represents tool responses as user-role entries with functionResponse parts.
			response := make(map[string]interface{})
			if msg.Content != "" {
				if err := json.Unmarshal([]byte(msg.Content), &response); err != nil {
					// If the content is not valid JSON, wrap it.
					response = map[string]interface{}{"result": msg.Content}
				}
			}
			// Extract function name from ToolCallID or use a default.
			funcName := msg.ToolCallID
			if funcName == "" {
				funcName = "unknown"
			}
			entry := GeminiContentEntry{
				Role: "user",
				Parts: []GeminiPart{
					{
						FunctionResponse: &GeminiFunctionResponse{
							Name:     funcName,
							Response: response,
						},
					},
				},
			}
			gc.Contents = append(gc.Contents, entry)

		default:
			return nil, fmt.Errorf("message %d: unsupported role %q", i, msg.Role)
		}
	}

	return gc, nil
}

// FromOpenAI converts OpenAI-format messages to the normalized format.
func (a *DefaultMessageAdapter) FromOpenAI(msgs []OpenAIMessage) ([]Message, error) {
	result := make([]Message, 0, len(msgs))

	for _, oaiMsg := range msgs {
		msg := Message{
			Role:       oaiMsg.Role,
			ToolCallID: oaiMsg.ToolCallID,
		}

		// Handle multi-part content.
		if len(oaiMsg.MultiContent) > 0 {
			msg.Parts = make([]ContentPart, 0, len(oaiMsg.MultiContent))
			for _, p := range oaiMsg.MultiContent {
				switch p.Type {
				case "text":
					msg.Parts = append(msg.Parts, ContentPart{
						Type: "text",
						Text: p.Text,
					})
				case "image_url":
					cp := ContentPart{Type: "image"}
					if p.ImageURL != nil {
						cp.ImageURL = p.ImageURL.URL
					}
					msg.Parts = append(msg.Parts, cp)
				}
			}
		} else if oaiMsg.Content != nil {
			msg.Content = *oaiMsg.Content
		}

		// Convert tool calls.
		if len(oaiMsg.ToolCalls) > 0 {
			msg.ToolCalls = make([]ToolCall, len(oaiMsg.ToolCalls))
			for j, tc := range oaiMsg.ToolCalls {
				msg.ToolCalls[j] = ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		result = append(result, msg)
	}

	return result, nil
}

// FromGemini converts Gemini-format content to the normalized format.
func (a *DefaultMessageAdapter) FromGemini(content *GeminiContent) ([]Message, error) {
	if content == nil {
		return nil, fmt.Errorf("gemini content is nil")
	}

	result := make([]Message, 0)

	// Convert system instruction if present.
	if content.SystemInstruction != nil {
		text := ""
		for _, p := range content.SystemInstruction.Parts {
			if p.Text != "" {
				if text != "" {
					text += "\n"
				}
				text += p.Text
			}
		}
		if text != "" {
			result = append(result, Message{
				Role:    RoleSystem,
				Content: text,
			})
		}
	}

	// Convert content entries.
	for _, entry := range content.Contents {
		role := entry.Role
		if role == "model" {
			role = RoleAssistant
		}

		// Check if this entry contains function responses (tool results).
		hasFunctionResponse := false
		for _, p := range entry.Parts {
			if p.FunctionResponse != nil {
				hasFunctionResponse = true
				break
			}
		}

		if hasFunctionResponse {
			// Each function response becomes a separate tool message.
			for _, p := range entry.Parts {
				if p.FunctionResponse != nil {
					respJSON, err := json.Marshal(p.FunctionResponse.Response)
					if err != nil {
						return nil, fmt.Errorf("failed to marshal function response: %w", err)
					}
					result = append(result, Message{
						Role:       RoleTool,
						Content:    string(respJSON),
						ToolCallID: p.FunctionResponse.Name,
					})
				}
			}
			continue
		}

		msg := Message{
			Role: role,
		}

		// Process parts.
		var textParts []string
		for _, p := range entry.Parts {
			if p.Text != "" {
				textParts = append(textParts, p.Text)
			}
			if p.FunctionCall != nil {
				argsJSON, err := json.Marshal(p.FunctionCall.Args)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal function call args: %w", err)
				}
				msg.ToolCalls = append(msg.ToolCalls, ToolCall{
					ID:   fmt.Sprintf("call_%s", p.FunctionCall.Name),
					Type: "function",
					Function: FunctionCall{
						Name:      p.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
			if p.InlineData != nil {
				msg.Parts = append(msg.Parts, ContentPart{
					Type:     "image",
					MIMEType: p.InlineData.MIMEType,
					ImageURL: "data:" + p.InlineData.MIMEType + ";base64," + p.InlineData.Data,
				})
			}
		}

		// Combine text parts.
		if len(msg.Parts) > 0 {
			// Multi-part message: add text parts to Parts slice.
			for _, t := range textParts {
				msg.Parts = append([]ContentPart{{Type: "text", Text: t}}, msg.Parts...)
			}
		} else if len(textParts) == 1 {
			msg.Content = textParts[0]
		} else if len(textParts) > 1 {
			// Multiple text parts without other media: join them.
			combined := ""
			for i, t := range textParts {
				if i > 0 {
					combined += "\n"
				}
				combined += t
			}
			msg.Content = combined
		}

		result = append(result, msg)
	}

	return result, nil
}

// messageToGeminiParts converts a normalized message's content to Gemini parts.
func (a *DefaultMessageAdapter) messageToGeminiParts(msg Message) []GeminiPart {
	if len(msg.Parts) > 0 {
		parts := make([]GeminiPart, 0, len(msg.Parts))
		for _, p := range msg.Parts {
			switch p.Type {
			case "text":
				parts = append(parts, GeminiPart{Text: p.Text})
			case "image":
				parts = append(parts, GeminiPart{
					InlineData: &GeminiInlineData{
						MIMEType: p.MIMEType,
						Data:     p.ImageURL, // Simplified: in practice would need base64 extraction.
					},
				})
			}
		}
		return parts
	}
	return []GeminiPart{{Text: msg.Content}}
}
