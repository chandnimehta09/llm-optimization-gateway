// Package handlers provides HTTP handlers for the API endpoints.
package handlers

import "net/http"

// ChatCompletionsHandler handles chat completion requests.
type ChatCompletionsHandler struct{}

// NewChatCompletionsHandler creates a new handler for /v1/chat/completions.
func NewChatCompletionsHandler() *ChatCompletionsHandler {
	return &ChatCompletionsHandler{}
}

// ServeHTTP handles incoming HTTP requests for chat completions.
func (h *ChatCompletionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte(`{"error":{"message":"not implemented","type":"server_error"}}`))
}
