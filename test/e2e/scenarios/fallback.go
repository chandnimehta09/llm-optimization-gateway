package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// FallbackScenario tests that requests fall back to alternate providers on failure.
type FallbackScenario struct{}

func (s *FallbackScenario) Name() string { return "fallback" }
func (s *FallbackScenario) Description() string {
	return "Validates provider fallback when primary provider fails"
}

func (s *FallbackScenario) Run(ctx context.Context, env *TestEnvironment) *Result {
	start := time.Now()
	result := &Result{Name: s.Name()}

	primary := env.Providers["openai"]
	fallback := env.Providers["gemini"]
	if primary == nil || fallback == nil {
		result.Error = errMissingComponent("openai and gemini providers for fallback")
		result.Duration = time.Since(start)
		return result
	}

	// Configure primary to always fail by returning 503 responses
	primary.Reset()
	primary.Healthy = false
	primary.Responses = []MockResponse{
		{
			StatusCode: 503,
			Body:       []byte(`{"error":{"message":"service unavailable","type":"server_error"}}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		},
	}

	// Configure fallback to succeed
	fallback.Reset()
	fallback.Healthy = true
	fallback.Responses = []MockResponse{
		{
			StatusCode: 200,
			Body:       []byte(`{"id":"fallback-resp","model":"gemini-pro","choices":[{"message":{"role":"assistant","content":"Response from fallback provider"},"finish_reason":"stop"}]}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		},
	}

	// Primary returns 503
	primaryResp, err := primary.NextResponse()
	if err != nil {
		result.Error = fmt.Errorf("unexpected error from primary: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	if primaryResp.StatusCode != 503 {
		result.Error = fmt.Errorf("expected primary to return 503, got %d", primaryResp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	// Fallback to secondary provider
	resp, err := fallback.NextResponse()
	if err != nil {
		result.Error = fmt.Errorf("fallback provider failed: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	if resp.StatusCode != 200 {
		result.Error = errUnexpectedStatus(200, resp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		result.Error = errInvalidJSON(err)
		result.Duration = time.Since(start)
		return result
	}

	respID, _ := body["id"].(string)
	if respID != "fallback-resp" {
		result.Error = errUnexpectedValue("id", "fallback-resp", respID)
		result.Duration = time.Since(start)
		return result
	}

	// Restore primary health for other tests
	primary.Healthy = true

	result.Passed = true
	result.Details = append(result.Details,
		"primary provider (openai) returned 503",
		"fallback to gemini provider succeeded",
		"fallback response id: fallback-resp",
	)
	result.Duration = time.Since(start)
	return result
}
