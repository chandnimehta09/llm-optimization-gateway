package scenarios

import (
	"context"
	"encoding/json"
	"time"
)

// SimpleRequestScenario tests a basic request/response flow through the gateway.
type SimpleRequestScenario struct{}

func (s *SimpleRequestScenario) Name() string { return "simple_request_response" }
func (s *SimpleRequestScenario) Description() string {
	return "Validates basic request/response flow through the gateway"
}

func (s *SimpleRequestScenario) Run(ctx context.Context, env *TestEnvironment) *Result {
	start := time.Now()
	result := &Result{Name: s.Name()}

	provider := env.Providers["openai"]
	if provider == nil {
		result.Error = errMissingProvider("openai")
		result.Duration = time.Since(start)
		return result
	}

	// Send a simple request
	provider.Reset()
	resp, err := provider.NextResponse()
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	// Verify response structure
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

	// Verify required fields
	requiredFields := []string{"id", "choices"}
	for _, field := range requiredFields {
		if _, ok := body[field]; !ok {
			result.Error = errMissingField(field)
			result.Duration = time.Since(start)
			return result
		}
	}

	// Verify choices array
	choices, ok := body["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		result.Error = errEmptyChoices()
		result.Duration = time.Since(start)
		return result
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		result.Error = errInvalidChoiceFormat()
		result.Duration = time.Since(start)
		return result
	}

	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		result.Error = errMissingField("message")
		result.Duration = time.Since(start)
		return result
	}

	content, ok := message["content"].(string)
	if !ok || content == "" {
		result.Error = errEmptyContent()
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Details = append(result.Details, "response has valid structure", "content is non-empty: "+content)
	result.Duration = time.Since(start)
	return result
}

// OpenAIProviderScenario tests sending a request through the OpenAI provider path.
type OpenAIProviderScenario struct{}

func (s *OpenAIProviderScenario) Name() string { return "openai_provider" }
func (s *OpenAIProviderScenario) Description() string {
	return "Validates OpenAI provider request handling"
}

func (s *OpenAIProviderScenario) Run(ctx context.Context, env *TestEnvironment) *Result {
	start := time.Now()
	result := &Result{Name: s.Name()}

	provider := env.Providers["openai"]
	if provider == nil {
		result.Error = errMissingProvider("openai")
		result.Duration = time.Since(start)
		return result
	}

	// Configure OpenAI-specific response
	provider.Reset()
	provider.Responses = []MockResponse{
		{
			StatusCode: 200,
			Body: []byte(`{
				"id": "chatcmpl-openai-123",
				"object": "chat.completion",
				"created": 1700000000,
				"model": "gpt-4",
				"choices": [{
					"index": 0,
					"message": {"role": "assistant", "content": "Paris is the capital of France."},
					"finish_reason": "stop"
				}],
				"usage": {"prompt_tokens": 15, "completion_tokens": 8, "total_tokens": 23}
			}`),
			Headers: map[string]string{"Content-Type": "application/json"},
		},
	}

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

	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		result.Error = errInvalidJSON(err)
		result.Duration = time.Since(start)
		return result
	}

	// Verify model is gpt-4
	model, _ := body["model"].(string)
	if model != "gpt-4" {
		result.Error = errUnexpectedValue("model", "gpt-4", model)
		result.Duration = time.Since(start)
		return result
	}

	// Verify usage
	usage, ok := body["usage"].(map[string]interface{})
	if !ok {
		result.Error = errMissingField("usage")
		result.Duration = time.Since(start)
		return result
	}

	totalTokens, _ := usage["total_tokens"].(float64)
	if totalTokens == 0 {
		result.Error = errMissingField("usage.total_tokens")
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Details = append(result.Details, "OpenAI response format validated", "model: gpt-4", "total_tokens: 23")
	result.Duration = time.Since(start)
	return result
}

// GeminiProviderScenario tests sending a request through the Gemini provider path.
type GeminiProviderScenario struct{}

func (s *GeminiProviderScenario) Name() string { return "gemini_provider" }
func (s *GeminiProviderScenario) Description() string {
	return "Validates Gemini provider request handling"
}

func (s *GeminiProviderScenario) Run(ctx context.Context, env *TestEnvironment) *Result {
	start := time.Now()
	result := &Result{Name: s.Name()}

	provider := env.Providers["gemini"]
	if provider == nil {
		result.Error = errMissingProvider("gemini")
		result.Duration = time.Since(start)
		return result
	}

	// Configure Gemini-specific response
	provider.Reset()
	provider.Responses = []MockResponse{
		{
			StatusCode: 200,
			Body: []byte(`{
				"id": "gemini-resp-456",
				"object": "chat.completion",
				"model": "gemini-pro",
				"choices": [{
					"index": 0,
					"message": {"role": "assistant", "content": "Paris is the capital of France."},
					"finish_reason": "stop"
				}],
				"usage": {"prompt_tokens": 12, "completion_tokens": 8, "total_tokens": 20}
			}`),
			Headers: map[string]string{"Content-Type": "application/json"},
		},
	}

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

	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		result.Error = errInvalidJSON(err)
		result.Duration = time.Since(start)
		return result
	}

	model, _ := body["model"].(string)
	if model != "gemini-pro" {
		result.Error = errUnexpectedValue("model", "gemini-pro", model)
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Details = append(result.Details, "Gemini response format validated", "model: gemini-pro")
	result.Duration = time.Since(start)
	return result
}
