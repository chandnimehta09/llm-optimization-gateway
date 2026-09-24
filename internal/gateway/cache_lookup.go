// Package gateway provides HTTP middleware and request lifecycle integration
// for the LLM optimization gateway, including cache lookup and write operations.
package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/llm-optimization-gateway/internal/cache"
)

// CacheKeyBuilder builds deterministic cache keys from request parameters.
type CacheKeyBuilder struct {
	// Prefix is prepended to all generated keys.
	Prefix string
}

// NewCacheKeyBuilder creates a CacheKeyBuilder with the given prefix.
func NewCacheKeyBuilder(prefix string) *CacheKeyBuilder {
	return &CacheKeyBuilder{Prefix: prefix}
}

// CacheKeyInput contains the normalized fields used to compute a cache key.
type CacheKeyInput struct {
	Model        string            `json:"model"`
	Messages     []CacheKeyMessage `json:"messages"`
	Temperature  *float64          `json:"temperature,omitempty"`
	MaxTokens    *int              `json:"max_tokens,omitempty"`
	TopP         *float64          `json:"top_p,omitempty"`
	Stop         []string          `json:"stop,omitempty"`
	Tools        []CacheKeyTool    `json:"tools,omitempty"`
	ToolChoice   string            `json:"tool_choice,omitempty"`
	SystemPrompt string            `json:"system_prompt,omitempty"`
}

// CacheKeyMessage is a minimal message representation for cache key computation.
type CacheKeyMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

// CacheKeyTool is a minimal tool representation for cache key computation.
type CacheKeyTool struct {
	Type         string `json:"type"`
	FunctionName string `json:"function_name"`
}

// BuildKey generates a deterministic cache key from the input parameters.
// The key is a SHA-256 hash of a canonical representation of all cache-relevant fields.
func (b *CacheKeyBuilder) BuildKey(input CacheKeyInput) string {
	h := sha256.New()

	// Write model
	h.Write([]byte("model=" + input.Model + "\n"))

	// Write system prompt
	if input.SystemPrompt != "" {
		h.Write([]byte("system_prompt=" + input.SystemPrompt + "\n"))
	}

	// Write messages in order (order matters for conversations)
	for i, msg := range input.Messages {
		h.Write([]byte(fmt.Sprintf("msg[%d].role=%s\n", i, msg.Role)))
		h.Write([]byte(fmt.Sprintf("msg[%d].content=%s\n", i, msg.Content)))
		if msg.ToolCallID != "" {
			h.Write([]byte(fmt.Sprintf("msg[%d].tool_call_id=%s\n", i, msg.ToolCallID)))
		}
	}

	// Write parameters (only when set)
	if input.Temperature != nil {
		h.Write([]byte(fmt.Sprintf("temperature=%g\n", *input.Temperature)))
	}
	if input.MaxTokens != nil {
		h.Write([]byte(fmt.Sprintf("max_tokens=%d\n", *input.MaxTokens)))
	}
	if input.TopP != nil {
		h.Write([]byte(fmt.Sprintf("top_p=%g\n", *input.TopP)))
	}

	// Write stop sequences sorted for determinism
	if len(input.Stop) > 0 {
		sorted := make([]string, len(input.Stop))
		copy(sorted, input.Stop)
		sort.Strings(sorted)
		h.Write([]byte("stop=" + strings.Join(sorted, ",") + "\n"))
	}

	// Write tools sorted by function name for determinism
	if len(input.Tools) > 0 {
		sorted := make([]CacheKeyTool, len(input.Tools))
		copy(sorted, input.Tools)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].FunctionName < sorted[j].FunctionName
		})
		for _, t := range sorted {
			h.Write([]byte(fmt.Sprintf("tool.%s=%s\n", t.FunctionName, t.Type)))
		}
	}

	if input.ToolChoice != "" {
		h.Write([]byte("tool_choice=" + input.ToolChoice + "\n"))
	}

	hash := hex.EncodeToString(h.Sum(nil))
	if b.Prefix != "" {
		return b.Prefix + ":" + hash
	}
	return hash
}

// TruncatedKey returns the first 12 characters of a cache key hash for display
// in response headers. Strips any prefix before truncating.
func TruncatedKey(key string) string {
	parts := strings.SplitN(key, ":", 2)
	hash := parts[len(parts)-1]
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// CacheLookupResult contains the result of a cache lookup operation.
type CacheLookupResult struct {
	// Hit indicates whether a cache entry was found and is valid.
	Hit bool
	// Entry is the cached entry (nil on miss or error).
	Entry *cache.Entry
	// Key is the cache key that was looked up.
	Key string
	// Duration is how long the lookup took.
	Duration time.Duration
	// Error is any error that occurred during lookup.
	Error error
	// Age is the age of the cached entry (zero if miss).
	Age time.Duration
}

// CacheLookup performs a cache lookup using the provided cache and key.
// Errors are captured in the result but do not propagate — the caller should
// treat an error result the same as a cache miss and proceed to the provider.
func CacheLookup(ctx context.Context, c cache.Cache, key string) CacheLookupResult {
	start := time.Now()
	result := CacheLookupResult{Key: key}

	if c == nil {
		result.Duration = time.Since(start)
		result.Error = fmt.Errorf("cache is nil")
		return result
	}

	entry, found, err := c.Get(ctx, key)
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err
		log.Printf("cache lookup error for key %s: %v", TruncatedKey(key), err)
		return result
	}

	if !found || entry == nil {
		return result
	}

	// Check expiration
	if entry.IsExpired() {
		log.Printf("cache entry expired for key %s", TruncatedKey(key))
		return result
	}

	result.Hit = true
	result.Entry = entry
	result.Age = time.Since(entry.Metadata.CreatedAt)

	return result
}

// CacheLookupResultToJSON converts a cache entry's response to an OpenAI-compatible
// JSON response body for returning cached responses directly to the client.
func CacheLookupResultToJSON(entry *cache.Entry, requestModel string) ([]byte, error) {
	if entry == nil {
		return nil, fmt.Errorf("nil cache entry")
	}

	resp := entry.Response

	// Build message map
	message := map[string]interface{}{
		"role":    "assistant",
		"content": resp.Content,
	}

	// Add tool calls if present
	if len(resp.ToolCalls) > 0 {
		toolCalls := make([]map[string]interface{}, len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			toolCalls[i] = map[string]interface{}{
				"id":   tc.ID,
				"type": tc.Type,
				"function": map[string]interface{}{
					"name":      tc.FunctionName,
					"arguments": tc.FunctionArgs,
				},
			}
		}
		message["tool_calls"] = toolCalls
	}

	// Build OpenAI-compatible response structure
	response := map[string]interface{}{
		"id":      resp.ID,
		"object":  "chat.completion",
		"created": entry.Metadata.CreatedAt.Unix(),
		"model":   requestModel,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       message,
				"finish_reason": resp.FinishReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     resp.Usage.PromptTokens,
			"completion_tokens": resp.Usage.CompletionTokens,
			"total_tokens":      resp.Usage.TotalTokens,
		},
	}

	return json.Marshal(response)
}
