package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

// =============================================================================
// OpenAI Request Builder
// =============================================================================

// openAIRequestBuilder builds HTTP requests for the OpenAI API format.
type openAIRequestBuilder struct {
	adapter *provider.DefaultMessageAdapter
}

// openAICompletionRequest is the request body for POST /v1/chat/completions.
type openAICompletionRequest struct {
	Model       string                   `json:"model"`
	Messages    []provider.OpenAIMessage `json:"messages"`
	Temperature *float64                 `json:"temperature,omitempty"`
	MaxTokens   *int                     `json:"max_tokens,omitempty"`
	TopP        *float64                 `json:"top_p,omitempty"`
	Stream      bool                     `json:"stream,omitempty"`
	Tools       []openAITool             `json:"tools,omitempty"`
	ToolChoice  interface{}              `json:"tool_choice,omitempty"`
}

// openAITool represents a tool definition in OpenAI's format.
type openAITool struct {
	Type     string            `json:"type"`
	Function openAIFunctionDef `json:"function"`
}

// openAIFunctionDef represents a function definition in OpenAI's format.
type openAIFunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// BuildCompleteRequest builds a non-streaming completion request for OpenAI.
func (b *openAIRequestBuilder) BuildCompleteRequest(cfg *Config, req *provider.CompletionRequest) (*http.Request, error) {
	body, err := b.buildBody(cfg, req, false)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(cfg.BaseURL, "/") + "/v1/chat/completions"
	httpReq, err := marshalAndCreateRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	return httpReq, nil
}

// BuildStreamRequest builds a streaming completion request for OpenAI.
func (b *openAIRequestBuilder) BuildStreamRequest(cfg *Config, req *provider.CompletionRequest) (*http.Request, error) {
	body, err := b.buildBody(cfg, req, true)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(cfg.BaseURL, "/") + "/v1/chat/completions"
	httpReq, err := marshalAndCreateRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	return httpReq, nil
}

// BuildHealthRequest builds a simple health check request (list models).
func (b *openAIRequestBuilder) BuildHealthRequest(cfg *Config) (*http.Request, error) {
	url := strings.TrimRight(cfg.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health check request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	return req, nil
}

// buildBody constructs the request body for an OpenAI completion.
func (b *openAIRequestBuilder) buildBody(cfg *Config, req *provider.CompletionRequest, stream bool) (*openAICompletionRequest, error) {
	// Resolve model.
	model := req.Model
	if model == "" {
		model = cfg.DefaultModel
	}

	// Prepend system prompt as a system message if provided.
	messages := req.Messages
	if req.SystemPrompt != "" {
		sysMsg := provider.Message{Role: provider.RoleSystem, Content: req.SystemPrompt}
		messages = append([]provider.Message{sysMsg}, messages...)
	}

	// Convert to OpenAI format.
	oaiMessages, err := b.adapter.ToOpenAI(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages to OpenAI format: %w", err)
	}

	body := &openAICompletionRequest{
		Model:       model,
		Messages:    oaiMessages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		TopP:        req.TopP,
		Stream:      stream,
	}

	// Convert tools if present.
	if len(req.Tools) > 0 {
		body.Tools = make([]openAITool, len(req.Tools))
		for i, t := range req.Tools {
			body.Tools[i] = openAITool{
				Type: t.Type,
				Function: openAIFunctionDef{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				},
			}
		}
	}

	// Convert tool choice.
	if req.ToolChoice != "" {
		if req.ToolChoice == "auto" || req.ToolChoice == "none" {
			body.ToolChoice = req.ToolChoice
		} else {
			body.ToolChoice = map[string]interface{}{
				"type": "function",
				"function": map[string]string{
					"name": req.ToolChoice,
				},
			}
		}
	}

	return body, nil
}

// =============================================================================
// OpenAI Response Parser
// =============================================================================

// openAIResponseParser parses OpenAI API responses.
type openAIResponseParser struct{}

// openAICompletionResponse is the response body from POST /v1/chat/completions.
type openAICompletionResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

type openAIChoice struct {
	Index        int                   `json:"index"`
	Message      openAIMessageResponse `json:"message"`
	Delta        openAIMessageResponse `json:"delta"`
	FinishReason *string               `json:"finish_reason"`
}

type openAIMessageResponse struct {
	Role      string                    `json:"role"`
	Content   *string                   `json:"content"`
	ToolCalls []provider.OpenAIToolCall `json:"tool_calls,omitempty"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// openAIStreamResponse is the response for a single SSE event in streaming mode.
type openAIStreamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   *openAIUsage   `json:"usage,omitempty"`
}

// openAIErrorResponse is the error response from the OpenAI API.
type openAIErrorResponse struct {
	Error openAIErrorDetail `json:"error"`
}

type openAIErrorDetail struct {
	Message string      `json:"message"`
	Type    string      `json:"type"`
	Param   string      `json:"param"`
	Code    interface{} `json:"code"` // Can be string or null.
}

// ParseCompletion parses a non-streaming OpenAI completion response.
func (p *openAIResponseParser) ParseCompletion(resp *http.Response, providerName string) (*provider.CompletionResponse, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, p.ParseError(resp, providerName)
	}

	body, err := readBody(resp)
	if err != nil {
		return nil, &provider.ProviderError{
			Provider: providerName,
			Code:     resp.StatusCode,
			Message:  "failed to read response body",
			Err:      err,
		}
	}

	var oaiResp openAICompletionResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return nil, &provider.ProviderError{
			Provider: providerName,
			Code:     resp.StatusCode,
			Message:  "failed to parse response body",
			Err:      err,
		}
	}

	return convertOpenAIResponse(&oaiResp, providerName), nil
}

// convertOpenAIResponse converts an OpenAI API response to the normalized format.
func convertOpenAIResponse(oaiResp *openAICompletionResponse, providerName string) *provider.CompletionResponse {
	result := &provider.CompletionResponse{
		ID:       oaiResp.ID,
		Model:    oaiResp.Model,
		Provider: providerName,
		Created:  time.Unix(oaiResp.Created, 0),
		Usage: provider.Usage{
			PromptTokens:     oaiResp.Usage.PromptTokens,
			CompletionTokens: oaiResp.Usage.CompletionTokens,
			TotalTokens:      oaiResp.Usage.TotalTokens,
		},
	}

	result.Choices = make([]provider.Choice, len(oaiResp.Choices))
	for i, c := range oaiResp.Choices {
		msg := provider.Message{
			Role: c.Message.Role,
		}
		if c.Message.Content != nil {
			msg.Content = *c.Message.Content
		}
		if len(c.Message.ToolCalls) > 0 {
			msg.ToolCalls = make([]provider.ToolCall, len(c.Message.ToolCalls))
			for j, tc := range c.Message.ToolCalls {
				msg.ToolCalls[j] = provider.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: provider.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		finishReason := ""
		if c.FinishReason != nil {
			finishReason = *c.FinishReason
		}

		result.Choices[i] = provider.Choice{
			Index:        c.Index,
			Message:      msg,
			FinishReason: finishReason,
		}
	}

	// Set top-level finish reason from first choice.
	if len(result.Choices) > 0 {
		result.FinishReason = result.Choices[0].FinishReason
	}

	return result
}

// ParseStreamChunk parses a single SSE line from an OpenAI streaming response.
func (p *openAIResponseParser) ParseStreamChunk(line string, providerName string) (*provider.StreamChunk, error) {
	// Skip empty lines and comments.
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") {
		return nil, nil
	}

	// Strip "data: " prefix.
	if !strings.HasPrefix(line, "data: ") {
		return nil, nil
	}
	data := strings.TrimPrefix(line, "data: ")

	// Check for stream termination.
	if data == "[DONE]" {
		return nil, io.EOF
	}

	var streamResp openAIStreamResponse
	if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
		return nil, &provider.ProviderError{
			Provider: providerName,
			Code:     0,
			Message:  "failed to parse stream chunk",
			Err:      err,
		}
	}

	chunk := &provider.StreamChunk{
		ID:       streamResp.ID,
		Model:    streamResp.Model,
		Provider: providerName,
	}

	if len(streamResp.Choices) > 0 {
		delta := streamResp.Choices[0].Delta
		chunk.Delta = provider.MessageDelta{
			Role: delta.Role,
		}
		if delta.Content != nil {
			chunk.Delta.Content = *delta.Content
		}
		if len(delta.ToolCalls) > 0 {
			chunk.Delta.ToolCalls = make([]provider.ToolCall, len(delta.ToolCalls))
			for i, tc := range delta.ToolCalls {
				chunk.Delta.ToolCalls[i] = provider.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: provider.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}

		if streamResp.Choices[0].FinishReason != nil {
			chunk.FinishReason = *streamResp.Choices[0].FinishReason
		}
	}

	// Usage is sometimes sent in the final chunk.
	if streamResp.Usage != nil {
		chunk.Usage = &provider.Usage{
			PromptTokens:     streamResp.Usage.PromptTokens,
			CompletionTokens: streamResp.Usage.CompletionTokens,
			TotalTokens:      streamResp.Usage.TotalTokens,
		}
	}

	return chunk, nil
}

// ParseError parses an OpenAI error response into the appropriate error type.
func (p *openAIResponseParser) ParseError(resp *http.Response, providerName string) error {
	body, err := readBody(resp)
	if err != nil {
		return &provider.ProviderError{
			Provider: providerName,
			Code:     resp.StatusCode,
			Message:  "failed to read error response",
			Err:      err,
		}
	}

	var oaiErr openAIErrorResponse
	if err := json.Unmarshal(body, &oaiErr); err != nil {
		return &provider.ProviderError{
			Provider: providerName,
			Code:     resp.StatusCode,
			Message:  string(body),
		}
	}

	errMsg := oaiErr.Error.Message

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"))
		return &provider.RateLimitError{
			ProviderError: provider.ProviderError{
				Provider: providerName,
				Code:     resp.StatusCode,
				Message:  errMsg,
			},
			RetryAfter: retryAfter,
		}

	case http.StatusUnauthorized:
		return &provider.AuthenticationError{
			ProviderError: provider.ProviderError{
				Provider: providerName,
				Code:     resp.StatusCode,
				Message:  errMsg,
			},
		}

	case http.StatusBadRequest:
		// Check if it's a context length error.
		if strings.Contains(errMsg, "maximum context length") ||
			strings.Contains(errMsg, "max_tokens") {
			return &provider.ContextLengthError{
				ProviderError: provider.ProviderError{
					Provider: providerName,
					Code:     resp.StatusCode,
					Message:  errMsg,
				},
			}
		}
		return &provider.ProviderError{
			Provider: providerName,
			Code:     resp.StatusCode,
			Message:  errMsg,
		}

	default:
		return &provider.ProviderError{
			Provider: providerName,
			Code:     resp.StatusCode,
			Message:  errMsg,
		}
	}
}

// parseRetryAfter parses the Retry-After header value into a duration.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 1 * time.Second // Default retry-after.
	}

	// Try parsing as seconds.
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		return time.Duration(seconds * float64(time.Second))
	}

	// Try parsing as HTTP date.
	if t, err := time.Parse(time.RFC1123, value); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}

	return 1 * time.Second
}
