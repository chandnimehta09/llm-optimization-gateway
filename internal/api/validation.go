package api

import (
	"fmt"

	"github.com/example/llm-optimization-gateway/internal/api/models"
)

// ValidationError represents a request validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// validRoles defines the set of allowed message roles.
var validRoles = map[string]bool{
	"system":    true,
	"user":      true,
	"assistant": true,
	"function":  true,
}

// ValidateChatCompletionRequest validates a ChatCompletionRequest and returns
// any validation errors found.
func ValidateChatCompletionRequest(req *models.ChatCompletionRequest) []ValidationError {
	var errs []ValidationError

	// Model is required
	if req.Model == "" {
		errs = append(errs, ValidationError{
			Field:   "model",
			Message: "model is required",
		})
	}

	// Messages are required and must not be empty
	if len(req.Messages) == 0 {
		errs = append(errs, ValidationError{
			Field:   "messages",
			Message: "messages is required and must be a non-empty array",
		})
	}

	// Validate each message
	for i, msg := range req.Messages {
		if msg.Role == "" {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("messages[%d].role", i),
				Message: "role is required",
			})
		} else if !validRoles[msg.Role] {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("messages[%d].role", i),
				Message: fmt.Sprintf("invalid role '%s': must be one of system, user, assistant, function", msg.Role),
			})
		}
	}

	// Validate temperature range if provided
	if req.Temperature != nil {
		if *req.Temperature < 0 || *req.Temperature > 2 {
			errs = append(errs, ValidationError{
				Field:   "temperature",
				Message: "temperature must be between 0 and 2",
			})
		}
	}

	// Validate top_p range if provided
	if req.TopP != nil {
		if *req.TopP < 0 || *req.TopP > 1 {
			errs = append(errs, ValidationError{
				Field:   "top_p",
				Message: "top_p must be between 0 and 1",
			})
		}
	}

	// Validate max_tokens if provided
	if req.MaxTokens != nil {
		if *req.MaxTokens < 1 {
			errs = append(errs, ValidationError{
				Field:   "max_tokens",
				Message: "max_tokens must be at least 1",
			})
		}
	}

	// Validate n if provided
	if req.N != nil {
		if *req.N < 1 {
			errs = append(errs, ValidationError{
				Field:   "n",
				Message: "n must be at least 1",
			})
		}
	}

	// Validate presence_penalty range if provided
	if req.PresencePenalty != nil {
		if *req.PresencePenalty < -2 || *req.PresencePenalty > 2 {
			errs = append(errs, ValidationError{
				Field:   "presence_penalty",
				Message: "presence_penalty must be between -2 and 2",
			})
		}
	}

	// Validate frequency_penalty range if provided
	if req.FrequencyPenalty != nil {
		if *req.FrequencyPenalty < -2 || *req.FrequencyPenalty > 2 {
			errs = append(errs, ValidationError{
				Field:   "frequency_penalty",
				Message: "frequency_penalty must be between -2 and 2",
			})
		}
	}

	return errs
}
