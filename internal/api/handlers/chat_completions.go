package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/llm-optimization-gateway/internal/api/models"
)

// ChatCompletionsHandler handles POST /v1/chat/completions requests.
type ChatCompletionsHandler struct{}

// NewChatCompletionsHandler creates a new ChatCompletionsHandler.
func NewChatCompletionsHandler() *ChatCompletionsHandler {
	return &ChatCompletionsHandler{}
}

// ServeHTTP handles incoming HTTP requests for chat completions.
func (h *ChatCompletionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New().String()

	log.Printf("[%s] %s %s", requestID, r.Method, r.URL.Path)

	// Only allow POST method
	if r.Method != http.MethodPost {
		code := "method_not_allowed"
		models.WriteErrorResponse(w, requestID, http.StatusMethodNotAllowed,
			fmt.Sprintf("method %s is not allowed, use POST", r.Method),
			"invalid_request_error", nil, &code)
		return
	}

	// Parse request body
	var req models.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		code := "invalid_json"
		models.WriteErrorResponse(w, requestID, http.StatusBadRequest,
			"invalid JSON in request body: "+err.Error(),
			"invalid_request_error", nil, &code)
		return
	}

	// Validate request
	if validationErrs := models.ValidateChatCompletionRequest(&req); len(validationErrs) > 0 {
		firstErr := validationErrs[0]
		param := firstErr.Field
		code := "invalid_request_error"
		models.WriteErrorResponse(w, requestID, http.StatusBadRequest,
			firstErr.Message,
			"invalid_request_error", &param, &code)
		return
	}

	// Handle streaming placeholder
	if req.Stream {
		code := "streaming_not_supported"
		models.WriteErrorResponse(w, requestID, http.StatusNotImplemented,
			"streaming is not yet supported",
			"invalid_request_error", nil, &code)
		return
	}

	log.Printf("[%s] Processing chat completion request: model=%s messages=%d",
		requestID, req.Model, len(req.Messages))

	// Generate mock response
	response := h.generateMockResponse(&req)

	log.Printf("[%s] Completed chat completion request: tokens=%d",
		requestID, response.Usage.TotalTokens)

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(response)
}

// generateMockResponse creates a mock ChatCompletionResponse for the given request.
func (h *ChatCompletionsHandler) generateMockResponse(req *models.ChatCompletionRequest) *models.ChatCompletionResponse {
	responseID := fmt.Sprintf("chatcmpl-%s", uuid.New().String())

	// Estimate prompt tokens (rough approximation: ~4 chars per token)
	promptTokens := 0
	for _, msg := range req.Messages {
		promptTokens += len(msg.Content)/4 + 4 // +4 for role overhead
	}
	if promptTokens == 0 {
		promptTokens = 1
	}

	completionContent := "This is a mock response from the LLM optimization gateway."
	completionTokens := len(completionContent)/4 + 1

	return &models.ChatCompletionResponse{
		ID:      responseID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []models.ChatCompletionChoice{
			{
				Index: 0,
				Message: models.ChatMessage{
					Role:    "assistant",
					Content: completionContent,
				},
				FinishReason: "stop",
			},
		},
		Usage: models.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}
}
