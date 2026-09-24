package request

import (
	"fmt"
	"strings"
)

// ValidationError represents a request validation error with field-level detail.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult holds the outcome of request validation.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// validRoles defines the set of allowed message roles after normalization.
var validRoles = map[string]bool{
	"system":    true,
	"user":      true,
	"assistant": true,
	"tool":      true,
	"function":  true,
}

// validContentTypes defines valid content part types.
var validContentTypes = map[string]bool{
	"text":      true,
	"image_url": true,
	"image":     true,
}

// ValidateNormalizedRequest performs comprehensive validation on a NormalizedRequest
// and returns all validation errors found.
func ValidateNormalizedRequest(req *NormalizedRequest) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Model is required.
	if req.Model == "" {
		result.addError("model", "model is required")
	}

	// Messages must be non-empty.
	if len(req.Messages) == 0 {
		result.addError("messages", "messages is required and must be a non-empty array")
	}

	// Validate each message.
	for i, msg := range req.Messages {
		validateMessage(result, msg, i)
	}

	// Validate temperature range.
	if req.Temperature != nil {
		if *req.Temperature < 0 || *req.Temperature > 2 {
			result.addError("temperature", "temperature must be between 0 and 2")
		}
	}

	// Validate top_p range.
	if req.TopP != nil {
		if *req.TopP < 0 || *req.TopP > 1 {
			result.addError("top_p", "top_p must be between 0 and 1")
		}
	}

	// Validate max_tokens.
	if req.MaxTokens != nil {
		if *req.MaxTokens < 1 {
			result.addError("max_tokens", "max_tokens must be at least 1")
		}
	}

	// Validate tools.
	for i, tool := range req.Tools {
		validateTool(result, tool, i)
	}

	return result
}

// ValidateOpenAIRequest validates an incoming OpenAI-format request before normalization.
func ValidateOpenAIRequest(req *OpenAIRequest) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if req.Model == "" {
		result.addError("model", "model is required")
	}

	if len(req.Messages) == 0 {
		result.addError("messages", "messages is required and must be a non-empty array")
	}

	for i, msg := range req.Messages {
		if msg.Role == "" {
			result.addError(fmt.Sprintf("messages[%d].role", i), "role is required")
		}
	}

	if req.Temperature != nil {
		if *req.Temperature < 0 || *req.Temperature > 2 {
			result.addError("temperature", "temperature must be between 0 and 2")
		}
	}

	if req.TopP != nil {
		if *req.TopP < 0 || *req.TopP > 1 {
			result.addError("top_p", "top_p must be between 0 and 1")
		}
	}

	if req.MaxTokens != nil {
		if *req.MaxTokens < 1 {
			result.addError("max_tokens", "max_tokens must be at least 1")
		}
	}

	return result
}

// ValidateGeminiRequest validates an incoming Gemini-format request before normalization.
func ValidateGeminiRequest(req *GeminiRequest) *ValidationResult {
	result := &ValidationResult{Valid: true}

	if len(req.Contents) == 0 {
		result.addError("contents", "contents is required and must be a non-empty array")
	}

	for i, content := range req.Contents {
		if content.Role == "" {
			result.addError(fmt.Sprintf("contents[%d].role", i), "role is required")
		}
		if len(content.Parts) == 0 {
			result.addError(fmt.Sprintf("contents[%d].parts", i), "parts must not be empty")
		}
	}

	if req.GenerationConfig != nil {
		gc := req.GenerationConfig
		if gc.Temperature != nil {
			if *gc.Temperature < 0 || *gc.Temperature > 2 {
				result.addError("generation_config.temperature", "temperature must be between 0 and 2")
			}
		}
		if gc.TopP != nil {
			if *gc.TopP < 0 || *gc.TopP > 1 {
				result.addError("generation_config.top_p", "top_p must be between 0 and 1")
			}
		}
		if gc.MaxOutputTokens != nil {
			if *gc.MaxOutputTokens < 1 {
				result.addError("generation_config.max_output_tokens", "max_output_tokens must be at least 1")
			}
		}
	}

	return result
}

// validateMessage validates a single normalized message.
func validateMessage(result *ValidationResult, msg Message, index int) {
	prefix := fmt.Sprintf("messages[%d]", index)

	// Role must be valid.
	if msg.Role == "" {
		result.addError(prefix+".role", "role is required")
	} else if !validRoles[strings.ToLower(msg.Role)] {
		result.addError(prefix+".role", fmt.Sprintf("invalid role '%s': must be one of system, user, assistant, tool, function", msg.Role))
	}

	// Validate content parts if present.
	for j, part := range msg.Parts {
		partPrefix := fmt.Sprintf("%s.parts[%d]", prefix, j)
		if !validContentTypes[part.Type] {
			result.addError(partPrefix+".type", fmt.Sprintf("invalid content type '%s': must be one of text, image_url, image", part.Type))
		}
		if part.Type == "text" && part.Text == "" {
			result.addError(partPrefix+".text", "text content must not be empty")
		}
		if (part.Type == "image_url" || part.Type == "image") && part.ImageURL == "" {
			result.addError(partPrefix+".image_url", "image_url must not be empty")
		}
	}

	// Tool messages must have a tool_call_id.
	if msg.Role == "tool" && msg.ToolCallID == "" {
		result.addError(prefix+".tool_call_id", "tool_call_id is required for tool messages")
	}
}

// validateTool validates a single tool definition.
func validateTool(result *ValidationResult, tool Tool, index int) {
	prefix := fmt.Sprintf("tools[%d]", index)

	if tool.Type == "" {
		result.addError(prefix+".type", "tool type is required")
	} else if tool.Type != "function" {
		result.addError(prefix+".type", fmt.Sprintf("unsupported tool type '%s': must be 'function'", tool.Type))
	}

	if tool.Function.Name == "" {
		result.addError(prefix+".function.name", "function name is required")
	}
}

// addError adds a validation error to the result.
func (r *ValidationResult) addError(field, message string) {
	r.Valid = false
	r.Errors = append(r.Errors, ValidationError{
		Field:   field,
		Message: message,
	})
}
