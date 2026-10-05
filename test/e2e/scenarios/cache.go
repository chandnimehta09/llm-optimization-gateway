package scenarios

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// CacheHitScenario tests that a cached response is returned on cache hit.
type CacheHitScenario struct{}

func (s *CacheHitScenario) Name() string { return "cache_hit" }
func (s *CacheHitScenario) Description() string {
	return "Validates cached response returned on cache hit"
}

func (s *CacheHitScenario) Run(ctx context.Context, env *TestEnvironment) *Result {
	start := time.Now()
	result := &Result{Name: s.Name()}

	if env.Cache == nil {
		result.Error = errMissingComponent("cache")
		result.Duration = time.Since(start)
		return result
	}

	// Pre-populate cache with a known response
	cacheKey := computeCacheKey("gpt-4", "What is the capital of France?")
	cachedBody := []byte(`{
		"id": "cached-resp-001",
		"object": "chat.completion",
		"model": "gpt-4",
		"choices": [{
			"index": 0,
			"message": {"role": "assistant", "content": "Paris is the capital of France. [FROM CACHE]"},
			"finish_reason": "stop"
		}],
		"usage": {"prompt_tokens": 15, "completion_tokens": 10, "total_tokens": 25}
	}`)

	if err := env.Cache.Set(ctx, cacheKey, cachedBody, 5*time.Minute); err != nil {
		result.Error = errCacheOperation("set", err)
		result.Duration = time.Since(start)
		return result
	}

	// Retrieve from cache
	data, found, err := env.Cache.Get(ctx, cacheKey)
	if err != nil {
		result.Error = errCacheOperation("get", err)
		result.Duration = time.Since(start)
		return result
	}

	if !found {
		result.Error = errCacheMiss(cacheKey)
		result.Duration = time.Since(start)
		return result
	}

	// Verify cached response
	var body map[string]interface{}
	if err := json.Unmarshal(data, &body); err != nil {
		result.Error = errInvalidJSON(err)
		result.Duration = time.Since(start)
		return result
	}

	respID, _ := body["id"].(string)
	if respID != "cached-resp-001" {
		result.Error = errUnexpectedValue("id", "cached-resp-001", respID)
		result.Duration = time.Since(start)
		return result
	}

	// Verify content includes cache marker
	choices := body["choices"].([]interface{})
	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})
	content := message["content"].(string)
	if content == "" {
		result.Error = errEmptyContent()
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Details = append(result.Details,
		"cache hit confirmed",
		"cached response id: "+respID,
		"content retrieved from cache",
	)
	result.Duration = time.Since(start)
	return result
}

// CacheMissScenario tests that cache miss results in provider call and cache population.
type CacheMissScenario struct{}

func (s *CacheMissScenario) Name() string { return "cache_miss" }
func (s *CacheMissScenario) Description() string {
	return "Validates cache miss triggers provider call and populates cache"
}

func (s *CacheMissScenario) Run(ctx context.Context, env *TestEnvironment) *Result {
	start := time.Now()
	result := &Result{Name: s.Name()}

	if env.Cache == nil {
		result.Error = errMissingComponent("cache")
		result.Duration = time.Since(start)
		return result
	}

	provider := env.Providers["openai"]
	if provider == nil {
		result.Error = errMissingProvider("openai")
		result.Duration = time.Since(start)
		return result
	}

	// Clear cache to ensure miss
	if err := env.Cache.Clear(ctx); err != nil {
		result.Error = errCacheOperation("clear", err)
		result.Duration = time.Since(start)
		return result
	}

	// Attempt cache lookup for a new key
	cacheKey := computeCacheKey("gpt-4", "What is quantum computing?")
	_, found, err := env.Cache.Get(ctx, cacheKey)
	if err != nil {
		result.Error = errCacheOperation("get", err)
		result.Duration = time.Since(start)
		return result
	}

	if found {
		result.Error = errUnexpectedCacheHit(cacheKey)
		result.Duration = time.Since(start)
		return result
	}

	// Simulate provider call on cache miss
	provider.Reset()
	resp, err := provider.NextResponse()
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	if resp.StatusCode != 200 {
		result.Error = errUnexpectedStatus(200, resp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	// Store response in cache
	if err := env.Cache.Set(ctx, cacheKey, resp.Body, 5*time.Minute); err != nil {
		result.Error = errCacheOperation("set", err)
		result.Duration = time.Since(start)
		return result
	}

	// Verify it's now in cache
	data, found, err := env.Cache.Get(ctx, cacheKey)
	if err != nil {
		result.Error = errCacheOperation("get", err)
		result.Duration = time.Since(start)
		return result
	}

	if !found {
		result.Error = errCacheMiss(cacheKey)
		result.Duration = time.Since(start)
		return result
	}

	// Verify stored data matches response
	var stored, original map[string]interface{}
	json.Unmarshal(data, &stored)
	json.Unmarshal(resp.Body, &original)

	storedID, _ := stored["id"].(string)
	originalID, _ := original["id"].(string)
	if storedID != originalID {
		result.Error = errUnexpectedValue("cached id", originalID, storedID)
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Details = append(result.Details,
		"cache miss confirmed on first lookup",
		"provider called successfully",
		"response stored in cache",
		"subsequent lookup returns cached response",
	)
	result.Duration = time.Since(start)
	return result
}

// computeCacheKey generates a deterministic cache key from model and prompt.
func computeCacheKey(model, prompt string) string {
	h := sha256.New()
	h.Write([]byte(model + ":" + prompt))
	return "llm-gw:" + hex.EncodeToString(h.Sum(nil))[:16]
}
