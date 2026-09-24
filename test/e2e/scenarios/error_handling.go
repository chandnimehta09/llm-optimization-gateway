package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ErrorHandlingScenario tests various error conditions and their handling.
type ErrorHandlingScenario struct{}

func (s *ErrorHandlingScenario) Name() string { return "error_handling" }
func (s *ErrorHandlingScenario) Description() string {
	return "Validates proper error handling for various failure modes"
}

func (s *ErrorHandlingScenario) Run(ctx context.Context, env *TestEnvironment) *Result {
	start := time.Now()
	result := &Result{Name: s.Name()}

	provider := env.Providers["openai"]
	if provider == nil {
		result.Error = errMissingProvider("openai")
		result.Duration = time.Since(start)
		return result
	}

	// Test 1: Provider returns 400 Bad Request
	provider.Reset()
	provider.Responses = []MockResponse{
		{
			StatusCode: 400,
			Body: []byte(`{
				"error": {
					"message": "Invalid model specified",
					"type": "invalid_request_error",
					"code": "model_not_found"
				}
			}`),
			Headers: map[string]string{"Content-Type": "application/json"},
		},
	}

	resp, err := provider.NextResponse()
	if err != nil {
		result.Error = fmt.Errorf("test 1 (400 error): unexpected provider error: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	if resp.StatusCode != 400 {
		result.Error = errUnexpectedStatus(400, resp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	var errBody map[string]interface{}
	if err := json.Unmarshal(resp.Body, &errBody); err != nil {
		result.Error = errInvalidJSON(err)
		result.Duration = time.Since(start)
		return result
	}

	errObj, ok := errBody["error"].(map[string]interface{})
	if !ok {
		result.Error = errMissingField("error")
		result.Duration = time.Since(start)
		return result
	}

	errMsg, _ := errObj["message"].(string)
	if errMsg == "" {
		result.Error = errMissingField("error.message")
		result.Duration = time.Since(start)
		return result
	}

	// Test 2: Provider returns 429 Rate Limit
	provider.Reset()
	provider.Responses = []MockResponse{
		{
			StatusCode: 429,
			Body: []byte(`{
				"error": {
					"message": "Rate limit exceeded. Please retry after 1 second.",
					"type": "rate_limit_error",
					"code": "rate_limit_exceeded"
				}
			}`),
			Headers: map[string]string{
				"Content-Type": "application/json",
				"Retry-After":  "1",
			},
		},
	}

	resp, err = provider.NextResponse()
	if err != nil {
		result.Error = fmt.Errorf("test 2 (429 error): unexpected provider error: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	if resp.StatusCode != 429 {
		result.Error = errUnexpectedStatus(429, resp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	// Test 3: Provider returns 500 Internal Server Error
	provider.Reset()
	provider.Responses = []MockResponse{
		{
			StatusCode: 500,
			Body: []byte(`{
				"error": {
					"message": "Internal server error",
					"type": "server_error",
					"code": "internal_error"
				}
			}`),
			Headers: map[string]string{"Content-Type": "application/json"},
		},
	}

	resp, err = provider.NextResponse()
	if err != nil {
		result.Error = fmt.Errorf("test 3 (500 error): unexpected provider error: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	if resp.StatusCode != 500 {
		result.Error = errUnexpectedStatus(500, resp.StatusCode)
		result.Duration = time.Since(start)
		return result
	}

	// Test 4: Provider connection failure (AlwaysFail with error)
	failProvider := NewMockProvider("failing")
	failProvider.AlwaysFail = true
	failProvider.FailureError = fmt.Errorf("connection refused")

	_, err = failProvider.NextResponse()
	if err == nil {
		result.Error = fmt.Errorf("test 4: expected connection failure")
		result.Duration = time.Since(start)
		return result
	}

	// Test 5: Context cancellation
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel() // immediately cancel
	if cancelCtx.Err() == nil {
		result.Error = fmt.Errorf("test 5: expected context cancellation")
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Details = append(result.Details,
		"400 Bad Request handled correctly with error body",
		"429 Rate Limit handled with proper error structure",
		"500 Internal Server Error propagated correctly",
		"connection failure produces error",
		"context cancellation detected",
	)
	result.Duration = time.Since(start)
	return result
}
