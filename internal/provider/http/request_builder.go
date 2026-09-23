package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/llm-optimization-gateway/internal/provider"
)

// RequestBuilder constructs HTTP requests for a specific API format.
type RequestBuilder interface {
	// BuildCompleteRequest builds an HTTP request for a non-streaming completion.
	BuildCompleteRequest(cfg *Config, req *provider.CompletionRequest) (*http.Request, error)

	// BuildStreamRequest builds an HTTP request for a streaming completion.
	BuildStreamRequest(cfg *Config, req *provider.CompletionRequest) (*http.Request, error)

	// BuildHealthRequest builds an HTTP request for a health check.
	BuildHealthRequest(cfg *Config) (*http.Request, error)
}

// newRequestBuilder creates the appropriate RequestBuilder for the given API format.
func newRequestBuilder(format provider.APIFormat) (RequestBuilder, error) {
	switch format {
	case provider.APIFormatOpenAI:
		return &openAIRequestBuilder{adapter: provider.NewMessageAdapter()}, nil
	case provider.APIFormatGemini:
		return &geminiRequestBuilder{adapter: provider.NewMessageAdapter()}, nil
	default:
		return nil, fmt.Errorf("unsupported API format: %q", format)
	}
}

// marshalAndCreateRequest is a helper that marshals the body and creates an HTTP request.
func marshalAndCreateRequest(method, url string, body interface{}) (*http.Request, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
