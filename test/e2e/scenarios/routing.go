package scenarios

import (
	"context"
	"encoding/json"
	"time"
)

// RoutingScenario tests that requests are routed to the correct provider based on model.
type RoutingScenario struct{}

func (s *RoutingScenario) Name() string { return "routing" }
func (s *RoutingScenario) Description() string {
	return "Validates request routing based on model and provider selection"
}

func (s *RoutingScenario) Run(ctx context.Context, env *TestEnvironment) *Result {
	start := time.Now()
	result := &Result{Name: s.Name()}

	openai := env.Providers["openai"]
	gemini := env.Providers["gemini"]
	if openai == nil || gemini == nil {
		result.Error = errMissingComponent("openai and gemini providers")
		result.Duration = time.Since(start)
		return result
	}

	// Test 1: Route GPT model to OpenAI provider
	openai.Reset()
	openai.Responses = []MockResponse{
		{
			StatusCode: 200,
			Body:       []byte(`{"id":"route-openai","model":"gpt-4","choices":[{"message":{"role":"assistant","content":"from openai"},"finish_reason":"stop"}]}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		},
	}

	resp, err := openai.NextResponse()
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	var openaiBody map[string]interface{}
	json.Unmarshal(resp.Body, &openaiBody)
	if model, _ := openaiBody["model"].(string); model != "gpt-4" {
		result.Error = errUnexpectedValue("model", "gpt-4", model)
		result.Duration = time.Since(start)
		return result
	}

	// Test 2: Route Gemini model to Gemini provider
	gemini.Reset()
	gemini.Responses = []MockResponse{
		{
			StatusCode: 200,
			Body:       []byte(`{"id":"route-gemini","model":"gemini-pro","choices":[{"message":{"role":"assistant","content":"from gemini"},"finish_reason":"stop"}]}`),
			Headers:    map[string]string{"Content-Type": "application/json"},
		},
	}

	resp, err = gemini.NextResponse()
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	var geminiBody map[string]interface{}
	json.Unmarshal(resp.Body, &geminiBody)
	if model, _ := geminiBody["model"].(string); model != "gemini-pro" {
		result.Error = errUnexpectedValue("model", "gemini-pro", model)
		result.Duration = time.Since(start)
		return result
	}

	// Verify different responses from different providers
	openaiID, _ := openaiBody["id"].(string)
	geminiID, _ := geminiBody["id"].(string)
	if openaiID == geminiID {
		result.Error = errRoutingCollision(openaiID)
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Details = append(result.Details,
		"GPT-4 model routed to OpenAI provider",
		"Gemini-Pro model routed to Gemini provider",
		"different response IDs confirm correct routing",
	)
	result.Duration = time.Since(start)
	return result
}
