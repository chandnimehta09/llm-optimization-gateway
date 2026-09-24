package api

import (
	"net/http"

	"github.com/llm-optimization-gateway/internal/api/models"
)

// APIError represents an OpenAI-compatible error response.
// This is an alias for models.APIError.
type APIError = models.APIError

// APIErrorDetail contains the error details.
// This is an alias for models.APIErrorDetail.
type APIErrorDetail = models.APIErrorDetail

// WriteErrorResponse writes an OpenAI-compatible error response with
// the appropriate headers and JSON body. It delegates to models.WriteErrorResponse.
func WriteErrorResponse(w http.ResponseWriter, requestID string, statusCode int, message, errType string, param, code *string) {
	models.WriteErrorResponse(w, requestID, statusCode, message, errType, param, code)
}
