package gateway

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/llm-optimization-gateway/internal/cache"
)

// CacheWriteInput contains all the information needed to write a response to the cache.
type CacheWriteInput struct {
	// Key is the cache key to store under.
	Key string
	// Response is the completion response to cache.
	Response cache.CompletionResponse
	// Provider is the provider that generated the response.
	Provider string
	// Model is the model that generated the response.
	Model string
	// ContextHash is the identity hash of the request context.
	ContextHash string
	// TTL is the time-to-live for the cache entry. Zero uses the cache's default.
	TTL time.Duration
	// Tags are optional labels for group invalidation.
	Tags []string
	// Cost is the estimated cost of the original request in USD.
	Cost float64
}

// CacheWriteResult contains the result of a cache write operation.
type CacheWriteResult struct {
	// Written indicates whether the entry was successfully cached.
	Written bool
	// Key is the cache key that was written.
	Key string
	// Duration is how long the write took.
	Duration time.Duration
	// Error is any error that occurred during the write.
	Error error
}

// CacheWrite stores a completion response in the cache. Errors are captured
// in the result but do not propagate — cache write failures are non-fatal
// and should not affect the response to the client.
func CacheWrite(ctx context.Context, c cache.Cache, input CacheWriteInput) CacheWriteResult {
	start := time.Now()
	result := CacheWriteResult{Key: input.Key}

	if c == nil {
		result.Duration = time.Since(start)
		result.Error = fmt.Errorf("cache is nil")
		return result
	}

	entry := &cache.Entry{
		Response: input.Response,
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       input.TTL,
			Tokens:    input.Response.Usage.TotalTokens,
			Cost:      input.Cost,
		},
		ContextHash: input.ContextHash,
		Provider:    input.Provider,
		Model:       input.Model,
		Tags:        input.Tags,
	}

	var opts []cache.Option
	if input.TTL > 0 {
		opts = append(opts, cache.WithTTL(input.TTL))
	}
	if len(input.Tags) > 0 {
		opts = append(opts, cache.WithTags(input.Tags...))
	}

	err := c.Set(ctx, input.Key, entry, opts...)
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err
		log.Printf("cache write error for key %s: %v", TruncatedKey(input.Key), err)
		return result
	}

	result.Written = true
	log.Printf("cache write success for key %s (ttl=%s, tokens=%d)",
		TruncatedKey(input.Key), input.TTL, input.Response.Usage.TotalTokens)

	return result
}
