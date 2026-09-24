package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/llm-optimization-gateway/internal/cache"
)

// CacheStatus represents the cache status for a request.
type CacheStatus string

const (
	// CacheStatusHIT means the response was served from cache.
	CacheStatusHIT CacheStatus = "HIT"
	// CacheStatusMISS means the cache did not contain a matching entry.
	CacheStatusMISS CacheStatus = "MISS"
	// CacheStatusBYPASS means the cache was intentionally skipped.
	CacheStatusBYPASS CacheStatus = "BYPASS"
)

const (
	// HeaderCacheStatus is the response header for cache status.
	HeaderCacheStatus = "X-Cache-Status"
	// HeaderCacheKey is the response header for the truncated cache key.
	HeaderCacheKey = "X-Cache-Key"
	// HeaderCacheAge is the response header for cache entry age in seconds.
	HeaderCacheAge = "X-Cache-Age"
)

// CacheConfig holds configuration for the cache middleware.
type CacheConfig struct {
	// Enabled controls whether caching is active.
	Enabled bool
	// DefaultTTL is the default time-to-live for cached entries.
	DefaultTTL time.Duration
	// KeyPrefix is prepended to all cache keys.
	KeyPrefix string
	// BypassStreaming skips caching for streaming requests.
	BypassStreaming bool
}

// DefaultCacheConfig returns a CacheConfig with sensible defaults.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		Enabled:         true,
		DefaultTTL:      10 * time.Minute,
		KeyPrefix:       "llm-gw",
		BypassStreaming: true,
	}
}

// RequestCacheOverride allows per-request cache configuration overrides
// specified in the request metadata.
type RequestCacheOverride struct {
	// CacheEnabled overrides the global cache enabled setting.
	CacheEnabled *bool
	// TTL overrides the default TTL for this request.
	TTL *time.Duration
	// Tags adds custom tags to the cache entry.
	Tags []string
}

// chatCompletionRequestWithCache is the request structure used by the cache
// middleware. It extends the standard chat completion request with metadata
// for cache control.
type chatCompletionRequestWithCache struct {
	Model       string                 `json:"model"`
	Messages    []chatCompletionMsg    `json:"messages"`
	Temperature *float64               `json:"temperature,omitempty"`
	TopP        *float64               `json:"top_p,omitempty"`
	N           *int                   `json:"n,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Stop        []string               `json:"stop,omitempty"`
	MaxTokens   *int                   `json:"max_tokens,omitempty"`
	Tools       []chatCompletionTool   `json:"tools,omitempty"`
	ToolChoice  string                 `json:"tool_choice,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	User        string                 `json:"user,omitempty"`
}

type chatCompletionMsg struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	Name       string `json:"name,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type chatCompletionTool struct {
	Type     string                     `json:"type"`
	Function chatCompletionToolFunction `json:"function"`
}

type chatCompletionToolFunction struct {
	Name string `json:"name"`
}

// shouldBypass determines if the cache should be bypassed for this request.
func shouldBypass(config CacheConfig, req *chatCompletionRequestWithCache) (bool, string) {
	if !config.Enabled {
		return true, "caching disabled"
	}

	// Bypass streaming requests
	if config.BypassStreaming && req.Stream {
		return true, "streaming request"
	}

	// Check metadata for explicit cache:false
	if req.Metadata != nil {
		if val, ok := req.Metadata["cache"]; ok {
			switch v := val.(type) {
			case bool:
				if !v {
					return true, "cache disabled by metadata"
				}
			case string:
				if v == "false" {
					return true, "cache disabled by metadata"
				}
			}
		}
	}

	return false, ""
}

// buildCacheKeyInput creates a CacheKeyInput from the parsed request.
func buildCacheKeyInput(req *chatCompletionRequestWithCache) CacheKeyInput {
	messages := make([]CacheKeyMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = CacheKeyMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
	}

	tools := make([]CacheKeyTool, len(req.Tools))
	for i, t := range req.Tools {
		tools[i] = CacheKeyTool{
			Type:         t.Type,
			FunctionName: t.Function.Name,
		}
	}

	return CacheKeyInput{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stop:        req.Stop,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
	}
}

// extractOverride parses per-request cache overrides from request metadata.
func extractOverride(metadata map[string]interface{}) *RequestCacheOverride {
	if metadata == nil {
		return nil
	}

	override := &RequestCacheOverride{}
	hasOverride := false

	if ttlVal, ok := metadata["cache_ttl"]; ok {
		if ttlFloat, ok := ttlVal.(float64); ok {
			ttl := time.Duration(int64(ttlFloat)) * time.Second
			override.TTL = &ttl
			hasOverride = true
		}
	}

	if tags, ok := metadata["cache_tags"]; ok {
		if tagSlice, ok := tags.([]interface{}); ok {
			for _, t := range tagSlice {
				if s, ok := t.(string); ok {
					override.Tags = append(override.Tags, s)
					hasOverride = true
				}
			}
		}
	}

	if !hasOverride {
		return nil
	}
	return override
}

// resolveTTL resolves the TTL from request override or config default.
func resolveTTL(config CacheConfig, override *RequestCacheOverride) time.Duration {
	if override != nil && override.TTL != nil {
		return *override.TTL
	}
	return config.DefaultTTL
}

// responseCapture wraps http.ResponseWriter to capture the response body and status code
// while still writing to the underlying writer.
type responseCapture struct {
	http.ResponseWriter
	statusCode  int
	body        []byte
	wroteHeader bool
}

func newResponseCapture(w http.ResponseWriter) *responseCapture {
	return &responseCapture{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rc *responseCapture) WriteHeader(code int) {
	if !rc.wroteHeader {
		rc.statusCode = code
		rc.wroteHeader = true
		rc.ResponseWriter.WriteHeader(code)
	}
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.body = append(rc.body, b...)
	if !rc.wroteHeader {
		rc.wroteHeader = true
	}
	return rc.ResponseWriter.Write(b)
}

// CacheMiddleware creates an HTTP middleware that integrates cache lookup
// and write into the request lifecycle for chat completion endpoints.
//
// On cache hit, the cached response is returned directly without calling the
// next handler. On cache miss, the next handler is called and its successful
// response is asynchronously written to the cache.
//
// Cache errors are handled gracefully — they are logged but never prevent
// the request from being served.
func CacheMiddleware(c cache.Cache, config CacheConfig) func(http.Handler) http.Handler {
	keyBuilder := NewCacheKeyBuilder(config.KeyPrefix)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only apply cache logic to POST requests (chat completions)
			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}

			// Read the full request body so we can parse and re-send it
			bodyBytes, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil {
				w.Header().Set(HeaderCacheStatus, string(CacheStatusBYPASS))
				writeJSONError(w, http.StatusBadRequest, "failed to read request body")
				return
			}

			// Parse the request to extract cache-relevant fields
			var req chatCompletionRequestWithCache
			if err := json.Unmarshal(bodyBytes, &req); err != nil {
				// Can't parse — bypass cache and let the handler deal with the error
				w.Header().Set(HeaderCacheStatus, string(CacheStatusBYPASS))
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				next.ServeHTTP(w, r)
				return
			}

			// Check for bypass conditions
			if bypass, reason := shouldBypass(config, &req); bypass {
				log.Printf("cache bypass: %s", reason)
				w.Header().Set(HeaderCacheStatus, string(CacheStatusBYPASS))
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				next.ServeHTTP(w, r)
				return
			}

			// Build cache key
			keyInput := buildCacheKeyInput(&req)
			cacheKey := keyBuilder.BuildKey(keyInput)

			// Perform cache lookup
			lookupResult := CacheLookup(r.Context(), c, cacheKey)

			if lookupResult.Error != nil {
				// Cache error — bypass and continue to provider
				log.Printf("cache lookup error, continuing to provider: %v", lookupResult.Error)
				w.Header().Set(HeaderCacheStatus, string(CacheStatusBYPASS))
				w.Header().Set(HeaderCacheKey, TruncatedKey(cacheKey))
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				next.ServeHTTP(w, r)
				return
			}

			if lookupResult.Hit {
				// Cache HIT — return cached response directly
				log.Printf("cache hit for key %s (age=%s)", TruncatedKey(cacheKey), lookupResult.Age)

				respBytes, err := CacheLookupResultToJSON(lookupResult.Entry, req.Model)
				if err != nil {
					log.Printf("cache hit serialization error: %v", err)
					w.Header().Set(HeaderCacheStatus, string(CacheStatusBYPASS))
					r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
					next.ServeHTTP(w, r)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set(HeaderCacheStatus, string(CacheStatusHIT))
				w.Header().Set(HeaderCacheKey, TruncatedKey(cacheKey))
				w.Header().Set(HeaderCacheAge, fmt.Sprintf("%.1f", lookupResult.Age.Seconds()))
				w.WriteHeader(http.StatusOK)
				w.Write(respBytes)
				return
			}

			// Cache MISS — call the provider via the next handler
			log.Printf("cache miss for key %s", TruncatedKey(cacheKey))

			rc := newResponseCapture(w)
			rc.Header().Set(HeaderCacheStatus, string(CacheStatusMISS))
			rc.Header().Set(HeaderCacheKey, TruncatedKey(cacheKey))

			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			next.ServeHTTP(rc, r)

			// Asynchronously cache successful responses
			if rc.statusCode == http.StatusOK && len(rc.body) > 0 {
				bodyCopy := make([]byte, len(rc.body))
				copy(bodyCopy, rc.body)
				go writeCacheEntry(c, config, cacheKey, bodyCopy, req)
			}
		})
	}
}

// writeCacheEntry parses the response body and writes it to the cache.
func writeCacheEntry(c cache.Cache, config CacheConfig, key string, body []byte, req chatCompletionRequestWithCache) CacheWriteResult {
	// Parse the response to extract fields for the cache entry
	var response struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls,omitempty"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return CacheWriteResult{
			Key:   key,
			Error: fmt.Errorf("failed to parse response for caching: %w", err),
		}
	}

	if len(response.Choices) == 0 {
		return CacheWriteResult{
			Key:   key,
			Error: fmt.Errorf("no choices in response"),
		}
	}

	// Build completion response for cache storage
	completionResponse := cache.CompletionResponse{
		ID:           response.ID,
		Model:        response.Model,
		Content:      response.Choices[0].Message.Content,
		FinishReason: response.Choices[0].FinishReason,
		Usage: cache.UsageInfo{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		},
	}

	// Include tool calls if present
	for _, tc := range response.Choices[0].Message.ToolCalls {
		completionResponse.ToolCalls = append(completionResponse.ToolCalls, cache.ToolCallResponse{
			ID:           tc.ID,
			Type:         tc.Type,
			FunctionName: tc.Function.Name,
			FunctionArgs: tc.Function.Arguments,
		})
	}

	// Resolve TTL and tags from per-request overrides
	override := extractOverride(req.Metadata)
	ttl := resolveTTL(config, override)

	var tags []string
	if override != nil && len(override.Tags) > 0 {
		tags = override.Tags
	}

	writeInput := CacheWriteInput{
		Key:      key,
		Response: completionResponse,
		Provider: "mock",
		Model:    req.Model,
		TTL:      ttl,
		Tags:     tags,
	}

	return CacheWrite(context.Background(), c, writeInput)
}

// writeJSONError writes a JSON error response.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    "invalid_request_error",
		},
	})
}
