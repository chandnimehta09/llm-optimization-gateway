package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/llm-optimization-gateway/internal/api/models"
	"github.com/llm-optimization-gateway/internal/cache"
	"github.com/llm-optimization-gateway/internal/cache/memory"
	"github.com/llm-optimization-gateway/internal/provider"
	http_provider "github.com/llm-optimization-gateway/internal/provider/http"
)

// ChatCompletionsHandler handles POST /v1/chat/completions requests.
type ChatCompletionsHandler struct {
	provider provider.Provider
	cache    *memory.Cache
}

// NewChatCompletionsHandler creates a new ChatCompletionsHandler with real provider.
func NewChatCompletionsHandler() *ChatCompletionsHandler {
	// Get provider config from environment
	apiKey := os.Getenv("GATEWAY_PROVIDERS_0_API_KEY")
	providerName := os.Getenv("GATEWAY_PROVIDERS_0_NAME")
	baseURL := os.Getenv("GATEWAY_PROVIDERS_0_BASE_URL")

	// Create HTTP provider if configured
	var prov provider.Provider
	var err error
	
	if apiKey != "" && providerName != "" {
		config := &http_provider.Config{
			Name:    providerName,
			APIKey:  apiKey,
			BaseURL: baseURL,
			Timeout: 30 * time.Second,
			Models:  []string{"gemini-1.5-flash", "gemini-1.5-pro", "gpt-4", "gpt-3.5-turbo"},
		}
		
		// Set API format based on provider
		if providerName == "google" || baseURL == "https://generativelanguage.googleapis.com" {
			config.APIFormat = provider.APIFormatGemini
		} else {
			config.APIFormat = provider.APIFormatOpenAI
		}
		
		prov, err = http_provider.New(config)
		if err != nil {
			log.Printf("Warning: Failed to create HTTP provider: %v, using mock", err)
		} else {
			log.Printf("Configured HTTP provider: %s (%s)", providerName, config.APIFormat)
		}
	}

	// Get cache TTL from environment (default: 1 hour)
	defaultTTL := 1 * time.Hour
	if ttlStr := os.Getenv("GATEWAY_CACHE_DEFAULT_TTL"); ttlStr != "" {
		if parsed, err := time.ParseDuration(ttlStr); err == nil {
			defaultTTL = parsed
			log.Printf("Cache TTL set to %v from environment", defaultTTL)
		} else {
			log.Printf("Warning: Invalid GATEWAY_CACHE_DEFAULT_TTL value '%s', using default 1h: %v", ttlStr, err)
		}
	}

	// Create in-memory cache
	memCache, err := memory.New(memory.Config{
		MaxSize:         1000,
		DefaultTTL:      defaultTTL,
		CleanupInterval: 5 * time.Minute,
	})
	if err != nil {
		log.Printf("Warning: Failed to create cache: %v", err)
	}

	return &ChatCompletionsHandler{
		provider: prov,
		cache:    memCache,
	}
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

	var response *models.ChatCompletionResponse
	var cacheStatus string = "MISS"

	// Build cache key
	cacheKey := h.buildCacheKey(&req)

	// Try cache first
	if h.cache != nil {
		entry, hit, err := h.cache.Get(r.Context(), cacheKey)
		if err != nil {
			log.Printf("[%s] Cache error: %v", requestID, err)
		} else if hit && entry != nil {
			// Cache hit
			cacheStatus = "HIT"
			// Convert cache entry to response
			response = h.cacheEntryToResponse(entry, req.Model)
			log.Printf("[%s] Cache HIT for key=%s", requestID, cacheKey[:12])
		}
	}

	// If cache miss, call provider
	if response == nil {
		if h.provider != nil {
			// Convert to provider request
			provReq := h.convertToProviderRequest(&req)

			// Call provider
			resp, err := h.provider.Complete(r.Context(), provReq)
			if err != nil {
				code := "provider_error"
				models.WriteErrorResponse(w, requestID, http.StatusBadGateway,
					fmt.Sprintf("provider error: %v", err),
					"api_error", nil, &code)
				return
			}

			// Convert response
			response = h.convertFromProviderResponse(resp, req.Model)

			// Cache the response
			if h.cache != nil {
				entry := h.responseToCacheEntry(response)
				if err := h.cache.Set(r.Context(), cacheKey, entry); err != nil {
					log.Printf("[%s] Cache write error: %v", requestID, err)
				}
			}
		} else {
			// Use mock if no provider
			response = h.generateMockResponse(&req)
		}
	}

	log.Printf("[%s] Completed chat completion request: tokens=%d cache=%s",
		requestID, response.Usage.TotalTokens, cacheStatus)

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("X-Cache-Status", cacheStatus)
	w.Header().Set("X-Cache-Key", cacheKey[:min(12, len(cacheKey))])
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

// buildCacheKey creates a deterministic cache key from the request.
func (h *ChatCompletionsHandler) buildCacheKey(req *models.ChatCompletionRequest) string {
	// Build key from model + messages
	key := req.Model + ":"
	for _, msg := range req.Messages {
		key += msg.Role + ":" + msg.Content + ":"
	}
	// Add temperature if set
	if req.Temperature != nil {
		key += fmt.Sprintf("temp:%.2f:", *req.Temperature)
	}
	// Hash it
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash)[:32]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// convertToProviderRequest converts API request to provider request.
func (h *ChatCompletionsHandler) convertToProviderRequest(req *models.ChatCompletionRequest) *provider.CompletionRequest {
	messages := make([]provider.Message, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = provider.Message{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	provReq := &provider.CompletionRequest{
		Model:    req.Model,
		Messages: messages,
	}

	if req.Temperature != nil {
		provReq.Temperature = req.Temperature
	}
	if req.MaxTokens != nil {
		provReq.MaxTokens = req.MaxTokens
	}

	return provReq
}

// convertFromProviderResponse converts provider response to API response.
func (h *ChatCompletionsHandler) convertFromProviderResponse(resp *provider.CompletionResponse, model string) *models.ChatCompletionResponse {
	choices := make([]models.ChatCompletionChoice, len(resp.Choices))
	for i, choice := range resp.Choices {
		choices[i] = models.ChatCompletionChoice{
			Index: choice.Index,
			Message: models.ChatMessage{
				Role:    choice.Message.Role,
				Content: choice.Message.Content,
			},
			FinishReason: choice.FinishReason,
		}
	}

	return &models.ChatCompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: resp.Created.Unix(),
		Model:   model,
		Choices: choices,
		Usage: models.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

// responseToCacheEntry converts API response to cache entry.
func (h *ChatCompletionsHandler) responseToCacheEntry(resp *models.ChatCompletionResponse) *cache.Entry {
	// Extract content from first choice
	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}

	return &cache.Entry{
		Response: cache.CompletionResponse{
			ID:           resp.ID,
			Model:        resp.Model,
			Content:      content,
			FinishReason: resp.Choices[0].FinishReason,
			Usage: cache.UsageInfo{
				PromptTokens:     resp.Usage.PromptTokens,
				CompletionTokens: resp.Usage.CompletionTokens,
				TotalTokens:      resp.Usage.TotalTokens,
			},
		},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       1 * time.Hour,
			Tokens:    resp.Usage.TotalTokens,
		},
		Model: resp.Model,
	}
}

// cacheEntryToResponse converts cache entry to API response.
func (h *ChatCompletionsHandler) cacheEntryToResponse(entry *cache.Entry, model string) *models.ChatCompletionResponse {
	return &models.ChatCompletionResponse{
		ID:      entry.Response.ID,
		Object:  "chat.completion",
		Created: entry.Metadata.CreatedAt.Unix(),
		Model:   model,
		Choices: []models.ChatCompletionChoice{
			{
				Index: 0,
				Message: models.ChatMessage{
					Role:    "assistant",
					Content: entry.Response.Content,
				},
				FinishReason: entry.Response.FinishReason,
			},
		},
		Usage: models.Usage{
			PromptTokens:     entry.Response.Usage.PromptTokens,
			CompletionTokens: entry.Response.Usage.CompletionTokens,
			TotalTokens:      entry.Response.Usage.TotalTokens,
		},
	}
}
