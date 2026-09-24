// Package cache provides the cache abstraction for storing and retrieving
// cached LLM completion responses. The interface is generic enough for
// different cache backends (in-memory, Redis, DynamoDB, etc.).
package cache

import (
	"encoding/json"
	"time"
)

// CompletionResponse is the cached representation of an LLM completion response.
// It stores the essential response data in a provider-agnostic format.
type CompletionResponse struct {
	// ID is the unique response identifier from the provider.
	ID string `json:"id"`
	// Model is the model that generated this response.
	Model string `json:"model"`
	// Content is the primary text content of the response.
	Content string `json:"content"`
	// FinishReason indicates why the model stopped generating (e.g., "stop", "length").
	FinishReason string `json:"finish_reason"`
	// ToolCalls holds any tool invocations returned by the model.
	ToolCalls []ToolCallResponse `json:"tool_calls,omitempty"`
	// Usage tracks token consumption for the cached response.
	Usage UsageInfo `json:"usage"`
}

// ToolCallResponse represents a tool call in a cached response.
type ToolCallResponse struct {
	// ID is the unique identifier for this tool call.
	ID string `json:"id"`
	// Type is the tool call type (e.g., "function").
	Type string `json:"type"`
	// FunctionName is the name of the function to invoke.
	FunctionName string `json:"function_name"`
	// FunctionArgs is the JSON-encoded arguments string.
	FunctionArgs string `json:"function_args"`
}

// UsageInfo tracks token usage for a cached response.
type UsageInfo struct {
	// PromptTokens is the number of tokens in the prompt.
	PromptTokens int `json:"prompt_tokens"`
	// CompletionTokens is the number of tokens in the completion.
	CompletionTokens int `json:"completion_tokens"`
	// TotalTokens is the total token count.
	TotalTokens int `json:"total_tokens"`
}

// EntryMetadata holds metadata about a cache entry.
type EntryMetadata struct {
	// CreatedAt is the time the entry was first cached.
	CreatedAt time.Time `json:"created_at"`
	// TTL is the time-to-live for this entry.
	TTL time.Duration `json:"ttl"`
	// HitCount tracks how many times this entry has been retrieved.
	HitCount int64 `json:"hit_count"`
	// Tokens is the total token count for the cached response.
	Tokens int `json:"tokens"`
	// Cost is the estimated cost of the original request in USD.
	Cost float64 `json:"cost"`
}

// entryMetadataJSON is the JSON-friendly representation of EntryMetadata.
// time.Duration is serialized as nanoseconds for lossless round-tripping.
type entryMetadataJSON struct {
	CreatedAt time.Time `json:"created_at"`
	TTLNanos  int64     `json:"ttl_nanos"`
	HitCount  int64     `json:"hit_count"`
	Tokens    int       `json:"tokens"`
	Cost      float64   `json:"cost"`
}

// MarshalJSON implements custom JSON marshaling for EntryMetadata.
func (m EntryMetadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(entryMetadataJSON{
		CreatedAt: m.CreatedAt,
		TTLNanos:  int64(m.TTL),
		HitCount:  m.HitCount,
		Tokens:    m.Tokens,
		Cost:      m.Cost,
	})
}

// UnmarshalJSON implements custom JSON unmarshaling for EntryMetadata.
func (m *EntryMetadata) UnmarshalJSON(data []byte) error {
	var j entryMetadataJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	m.CreatedAt = j.CreatedAt
	m.TTL = time.Duration(j.TTLNanos)
	m.HitCount = j.HitCount
	m.Tokens = j.Tokens
	m.Cost = j.Cost
	return nil
}

// Entry is a single cache entry containing the cached response and
// associated metadata for cache management and invalidation.
type Entry struct {
	// Response is the cached completion response data.
	Response CompletionResponse `json:"response"`
	// Metadata holds cache management metadata.
	Metadata EntryMetadata `json:"metadata"`
	// ContextHash is the identity hash of the request context that produced
	// this response. Used to validate cache key correctness.
	ContextHash string `json:"context_hash"`
	// Provider is the provider that generated this response (e.g., "openai", "anthropic").
	Provider string `json:"provider"`
	// Model is the specific model used (e.g., "gpt-4", "claude-3-opus").
	Model string `json:"model"`
	// Tags are labels for group invalidation (e.g., ["user:123", "session:abc"]).
	Tags []string `json:"tags,omitempty"`
}

// IsExpired returns true if the entry has exceeded its TTL.
func (e *Entry) IsExpired() bool {
	if e.Metadata.TTL <= 0 {
		return false
	}
	return time.Since(e.Metadata.CreatedAt) > e.Metadata.TTL
}

// ExpiresAt returns the time at which the entry expires.
// Returns zero time if the entry has no TTL.
func (e *Entry) ExpiresAt() time.Time {
	if e.Metadata.TTL <= 0 {
		return time.Time{}
	}
	return e.Metadata.CreatedAt.Add(e.Metadata.TTL)
}

// IncrementHitCount increments the entry's hit count by one.
func (e *Entry) IncrementHitCount() {
	e.Metadata.HitCount++
}

// Statistics provides cache performance metrics.
type Statistics struct {
	// Hits is the total number of cache hits.
	Hits int64 `json:"hits"`
	// Misses is the total number of cache misses.
	Misses int64 `json:"misses"`
	// Evictions is the total number of entries evicted.
	Evictions int64 `json:"evictions"`
	// Expirations is the total number of entries that expired.
	Expirations int64 `json:"expirations"`
	// Size is the current number of entries in the cache.
	Size int64 `json:"size"`
	// SizeBytes is the approximate memory usage in bytes.
	SizeBytes int64 `json:"size_bytes"`
	// OldestEntry is the creation time of the oldest entry.
	OldestEntry time.Time `json:"oldest_entry,omitempty"`
	// NewestEntry is the creation time of the newest entry.
	NewestEntry time.Time `json:"newest_entry,omitempty"`
}

// HitRate returns the cache hit rate as a float between 0 and 1.
// Returns 0 if there have been no requests.
func (s *Statistics) HitRate() float64 {
	total := s.Hits + s.Misses
	if total == 0 {
		return 0
	}
	return float64(s.Hits) / float64(total)
}

// StatsCollector provides an interface for collecting cache statistics.
type StatsCollector interface {
	// Stats returns a snapshot of the current cache statistics.
	Stats() Statistics
	// ResetStats resets all statistics counters to zero.
	ResetStats()
}
