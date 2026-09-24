// Package e2e provides end-to-end tests for the LLM optimization gateway.
// These tests exercise the full request pipeline including request normalization,
// caching, deduplication, routing, fallback, and error handling.
package e2e

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/test/e2e/scenarios"
)

// inMemoryCache is a simple in-memory cache for e2e testing.
type inMemoryCache struct {
	mu    sync.RWMutex
	store map[string]cacheEntry
}

type cacheEntry struct {
	data      []byte
	expiresAt time.Time
}

func newInMemoryCache() *inMemoryCache {
	return &inMemoryCache{
		store: make(map[string]cacheEntry),
	}
}

func (c *inMemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.store[key]
	if !ok {
		return nil, false, nil
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return nil, false, nil
	}
	return entry.data, true, nil
}

func (c *inMemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := cacheEntry{data: make([]byte, len(value))}
	copy(entry.data, value)
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	c.store[key] = entry
	return nil
}

func (c *inMemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, key)
	return nil
}

func (c *inMemoryCache) Clear(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = make(map[string]cacheEntry)
	return nil
}

// setupTestEnvironment creates a test environment with mock providers and cache.
func setupTestEnvironment() *scenarios.TestEnvironment {
	return &scenarios.TestEnvironment{
		GatewayURL: "http://localhost:8080",
		Cache:      newInMemoryCache(),
		Providers: map[string]*scenarios.MockProvider{
			"openai": scenarios.NewMockProvider("openai"),
			"gemini": scenarios.NewMockProvider("gemini"),
		},
	}
}

// TestSimpleRequestResponse validates the basic request/response flow.
func TestSimpleRequestResponse(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	scenario := &scenarios.SimpleRequestScenario{}
	result := scenario.Run(ctx, env)

	if !result.Passed {
		t.Fatalf("scenario %q failed: %v", result.Name, result.Error)
	}
	t.Logf("scenario %q passed in %v", result.Name, result.Duration)
	for _, d := range result.Details {
		t.Logf("  - %s", d)
	}
}

// TestCacheHit validates cached response retrieval.
func TestCacheHit(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	scenario := &scenarios.CacheHitScenario{}
	result := scenario.Run(ctx, env)

	if !result.Passed {
		t.Fatalf("scenario %q failed: %v", result.Name, result.Error)
	}
	t.Logf("scenario %q passed in %v", result.Name, result.Duration)
	for _, d := range result.Details {
		t.Logf("  - %s", d)
	}
}

// TestCacheMiss validates cache miss triggers provider call and populates cache.
func TestCacheMiss(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	scenario := &scenarios.CacheMissScenario{}
	result := scenario.Run(ctx, env)

	if !result.Passed {
		t.Fatalf("scenario %q failed: %v", result.Name, result.Error)
	}
	t.Logf("scenario %q passed in %v", result.Name, result.Duration)
	for _, d := range result.Details {
		t.Logf("  - %s", d)
	}
}

// TestDeduplication validates concurrent identical requests are deduplicated.
func TestDeduplication(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	scenario := &scenarios.DeduplicationScenario{}
	result := scenario.Run(ctx, env)

	if !result.Passed {
		t.Fatalf("scenario %q failed: %v", result.Name, result.Error)
	}
	t.Logf("scenario %q passed in %v", result.Name, result.Duration)
	for _, d := range result.Details {
		t.Logf("  - %s", d)
	}
}

// TestRouting validates request routing based on model and provider.
func TestRouting(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	scenario := &scenarios.RoutingScenario{}
	result := scenario.Run(ctx, env)

	if !result.Passed {
		t.Fatalf("scenario %q failed: %v", result.Name, result.Error)
	}
	t.Logf("scenario %q passed in %v", result.Name, result.Duration)
	for _, d := range result.Details {
		t.Logf("  - %s", d)
	}
}

// TestFallback validates provider fallback when primary provider fails.
func TestFallback(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	scenario := &scenarios.FallbackScenario{}
	result := scenario.Run(ctx, env)

	if !result.Passed {
		t.Fatalf("scenario %q failed: %v", result.Name, result.Error)
	}
	t.Logf("scenario %q passed in %v", result.Name, result.Duration)
	for _, d := range result.Details {
		t.Logf("  - %s", d)
	}
}

// TestErrorHandling validates proper error handling for various failure modes.
func TestErrorHandling(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	scenario := &scenarios.ErrorHandlingScenario{}
	result := scenario.Run(ctx, env)

	if !result.Passed {
		t.Fatalf("scenario %q failed: %v", result.Name, result.Error)
	}
	t.Logf("scenario %q passed in %v", result.Name, result.Duration)
	for _, d := range result.Details {
		t.Logf("  - %s", d)
	}
}

// TestOpenAIProvider validates OpenAI provider request handling.
func TestOpenAIProvider(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	scenario := &scenarios.OpenAIProviderScenario{}
	result := scenario.Run(ctx, env)

	if !result.Passed {
		t.Fatalf("scenario %q failed: %v", result.Name, result.Error)
	}
	t.Logf("scenario %q passed in %v", result.Name, result.Duration)
	for _, d := range result.Details {
		t.Logf("  - %s", d)
	}
}

// TestGeminiProvider validates Gemini provider request handling.
func TestGeminiProvider(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	scenario := &scenarios.GeminiProviderScenario{}
	result := scenario.Run(ctx, env)

	if !result.Passed {
		t.Fatalf("scenario %q failed: %v", result.Name, result.Error)
	}
	t.Logf("scenario %q passed in %v", result.Name, result.Duration)
	for _, d := range result.Details {
		t.Logf("  - %s", d)
	}
}

// TestAllScenariosSequential runs all scenarios sequentially to verify repeatability.
func TestAllScenariosSequential(t *testing.T) {
	env := setupTestEnvironment()
	ctx := context.Background()

	allScenarios := []scenarios.Scenario{
		&scenarios.SimpleRequestScenario{},
		&scenarios.OpenAIProviderScenario{},
		&scenarios.GeminiProviderScenario{},
		&scenarios.CacheHitScenario{},
		&scenarios.CacheMissScenario{},
		&scenarios.DeduplicationScenario{},
		&scenarios.RoutingScenario{},
		&scenarios.FallbackScenario{},
		&scenarios.ErrorHandlingScenario{},
	}

	results := scenarios.RunAll(ctx, env, allScenarios)
	summary := scenarios.Summary(results)
	t.Logf("Sequential run: %s", summary)

	for _, r := range results {
		if !r.Passed {
			t.Errorf("scenario %q failed: %v", r.Name, r.Error)
		} else {
			t.Logf("  PASS: %s (%v)", r.Name, r.Duration)
		}
	}

	if !scenarios.AllPassed(results) {
		t.Fatal("not all scenarios passed")
	}
}

// TestAllScenariosRepeatable runs all scenarios twice to verify repeatability.
func TestAllScenariosRepeatable(t *testing.T) {
	allScenarios := []scenarios.Scenario{
		&scenarios.SimpleRequestScenario{},
		&scenarios.OpenAIProviderScenario{},
		&scenarios.GeminiProviderScenario{},
		&scenarios.CacheHitScenario{},
		&scenarios.CacheMissScenario{},
		&scenarios.DeduplicationScenario{},
		&scenarios.RoutingScenario{},
		&scenarios.FallbackScenario{},
		&scenarios.ErrorHandlingScenario{},
	}

	for run := 1; run <= 2; run++ {
		t.Logf("=== Run %d ===", run)
		env := setupTestEnvironment()
		ctx := context.Background()

		results := scenarios.RunAll(ctx, env, allScenarios)
		summary := scenarios.Summary(results)
		t.Logf("Run %d: %s", run, summary)

		for _, r := range results {
			if !r.Passed {
				t.Errorf("run %d, scenario %q failed: %v", run, r.Name, r.Error)
			}
		}

		if !scenarios.AllPassed(results) {
			t.Fatalf("run %d: not all scenarios passed", run)
		}
	}
}
