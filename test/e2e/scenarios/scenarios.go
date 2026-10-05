// Package scenarios defines end-to-end test scenarios for the LLM optimization gateway.
// Each scenario exercises a specific gateway capability and can be run independently
// or as part of the full e2e suite.
package scenarios

import (
	"context"
	"fmt"
	"time"
)

// Result captures the outcome of a scenario execution.
type Result struct {
	// Name is the scenario name.
	Name string
	// Passed indicates whether the scenario passed.
	Passed bool
	// Duration is how long the scenario took.
	Duration time.Duration
	// Error is the error if the scenario failed.
	Error error
	// Details holds human-readable detail messages for diagnostics.
	Details []string
}

// Scenario defines a single e2e test scenario.
type Scenario interface {
	// Name returns the scenario name.
	Name() string
	// Description returns a human-readable description.
	Description() string
	// Run executes the scenario and returns the result.
	Run(ctx context.Context, env *TestEnvironment) *Result
}

// TestEnvironment holds shared test infrastructure for scenarios.
type TestEnvironment struct {
	// GatewayURL is the base URL of the running gateway.
	GatewayURL string
	// Cache is a test cache instance for verifying cache behavior.
	Cache TestCache
	// Providers holds mock provider instances.
	Providers map[string]*MockProvider
}

// TestCache is a simplified cache interface for e2e testing.
type TestCache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}

// MockProvider simulates an LLM provider for e2e testing.
type MockProvider struct {
	Name          string
	Healthy       bool
	Latency       time.Duration
	Responses     []MockResponse
	RequestCount  int
	AlwaysFail    bool // if true, always return FailureError
	FailAfter     int  // fail after N successful requests; 0 means never fail
	FailureError  error
	FailureStatus int
	responseIndex int
}

// MockResponse defines a canned response from a mock provider.
type MockResponse struct {
	StatusCode int
	Body       []byte
	Headers    map[string]string
}

// NewMockProvider creates a new MockProvider with default settings.
func NewMockProvider(name string) *MockProvider {
	return &MockProvider{
		Name:    name,
		Healthy: true,
		Latency: 10 * time.Millisecond,
		Responses: []MockResponse{
			{
				StatusCode: 200,
				Body:       []byte(fmt.Sprintf(`{"id":"resp-1","object":"chat.completion","model":"%s-model","choices":[{"index":0,"message":{"role":"assistant","content":"Mock response from %s"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`, name, name)),
				Headers:    map[string]string{"Content-Type": "application/json"},
			},
		},
	}
}

// NextResponse returns the next response in the rotation, handling failures.
func (mp *MockProvider) NextResponse() (*MockResponse, error) {
	mp.RequestCount++
	if mp.AlwaysFail {
		if mp.FailureError != nil {
			return nil, mp.FailureError
		}
		return &MockResponse{
			StatusCode: mp.FailureStatus,
			Body:       []byte(`{"error":"provider failure"}`),
		}, nil
	}
	if mp.FailAfter > 0 && mp.RequestCount > mp.FailAfter {
		if mp.FailureError != nil {
			return nil, mp.FailureError
		}
		return &MockResponse{
			StatusCode: mp.FailureStatus,
			Body:       []byte(`{"error":"provider failure"}`),
		}, nil
	}

	if len(mp.Responses) == 0 {
		return &MockResponse{
			StatusCode: 200,
			Body:       []byte(`{"id":"resp-default","choices":[{"message":{"role":"assistant","content":"default"}}]}`),
		}, nil
	}

	resp := &mp.Responses[mp.responseIndex%len(mp.Responses)]
	mp.responseIndex++
	return resp, nil
}

// Reset resets the mock provider state.
func (mp *MockProvider) Reset() {
	mp.RequestCount = 0
	mp.responseIndex = 0
	mp.AlwaysFail = false
	mp.FailAfter = 0
	mp.FailureError = nil
	mp.FailureStatus = 0
}

// RunAll executes all provided scenarios and returns their results.
func RunAll(ctx context.Context, env *TestEnvironment, scenarios []Scenario) []*Result {
	results := make([]*Result, 0, len(scenarios))
	for _, s := range scenarios {
		result := s.Run(ctx, env)
		results = append(results, result)
	}
	return results
}

// AllPassed returns true if all results indicate success.
func AllPassed(results []*Result) bool {
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}
	return true
}

// Summary returns a human-readable summary of scenario results.
func Summary(results []*Result) string {
	passed, failed := 0, 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}
	return fmt.Sprintf("Scenarios: %d passed, %d failed, %d total", passed, failed, passed+failed)
}
