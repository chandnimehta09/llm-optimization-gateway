package http

import (
	"fmt"
	"net/http"

	"github.com/llm-optimization-gateway/internal/provider"
)

// ResponseParser parses HTTP responses into normalized provider types.
type ResponseParser interface {
	// ParseCompletion parses a non-streaming completion response.
	ParseCompletion(resp *http.Response, providerName string) (*provider.CompletionResponse, error)

	// ParseStreamChunk parses a single SSE line into a StreamChunk.
	// Returns nil, nil if the line should be skipped (comments, empty lines).
	// Returns nil, io.EOF if the stream is complete.
	ParseStreamChunk(line string, providerName string) (*provider.StreamChunk, error)

	// ParseError parses an HTTP error response into the appropriate provider error type.
	ParseError(resp *http.Response, providerName string) error
}

// newResponseParser creates the appropriate ResponseParser for the given API format.
func newResponseParser(format provider.APIFormat) (ResponseParser, error) {
	switch format {
	case provider.APIFormatOpenAI:
		return &openAIResponseParser{}, nil
	case provider.APIFormatGemini:
		return &geminiResponseParser{}, nil
	default:
		return nil, fmt.Errorf("unsupported API format: %q", format)
	}
}
