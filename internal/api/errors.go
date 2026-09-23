package api

import (
	"encoding/json"
	"net/http"
)

// APIError represents an OpenAI-compatible error response.
type APIError struct {
	Error APIErrorDetail `json:"error"`
}

// APIErrorDetail contains the error details.
type APIErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    *string `json:"code"`
}

// WriteErrorResponse writes an OpenAI-compatible error response with
// the appropriate headers and JSON body.
func WriteErrorResponse(w http.ResponseWriter, requestID string, statusCode int, message, errType string, param, code *string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(statusCode)

	apiErr := APIError{
		Error: APIErrorDetail{
			Message: message,
			Type:    errType,
			Param:   param,
			Code:    code,
		},
	}

	json.NewEncoder(w).Encode(apiErr)
}
