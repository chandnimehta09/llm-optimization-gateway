package api

import (
	"net/http"

	"github.com/example/llm-optimization-gateway/internal/api/handlers"
)

// NewRouter creates and configures the API router with all routes.
func NewRouter() http.Handler {
	mux := http.NewServeMux()

	chatHandler := handlers.NewChatCompletionsHandler()
	mux.Handle("/v1/chat/completions", chatHandler)

	return mux
}
