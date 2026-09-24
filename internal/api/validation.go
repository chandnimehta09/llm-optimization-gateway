package api

import (
	"github.com/llm-optimization-gateway/internal/api/models"
)

// ValidationError represents a request validation error.
// This is an alias for models.ValidationError.
type ValidationError = models.ValidationError

// ValidateChatCompletionRequest validates a ChatCompletionRequest and returns
// any validation errors found. It delegates to models.ValidateChatCompletionRequest.
func ValidateChatCompletionRequest(req *models.ChatCompletionRequest) []models.ValidationError {
	return models.ValidateChatCompletionRequest(req)
}
