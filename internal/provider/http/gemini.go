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
// Gemini Request Builder
// =============================================================================

// geminiRequestBuilder builds HTTP requests for the Google Gemini API format.
type geminiRequestBuilder struct {
	adapter *provider.DefaultMessageAdapter
}

// geminiCompletionRequest is the request body for Gemini's generateContent endpoint.
type geminiCompletionRequest struct {
	Contents          []provider.GeminiContentEntry     `json:"contents"`
	SystemInstruction *provider.GeminiSystemInstruction `json:"system_instruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig           `json:"generationConfig,omitempty"`
	SafetySettings    []geminiSafetySetting             `json:"safetySettings,omitempty"`
	Tools             []geminiToolDef                   `json:"tools,omitempty"`
}

// geminiGenerationConfig holds generation parameters for the Gemini API.
type geminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
}

// geminiSafetySetting represents a safety setting for the Gemini API.
type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// geminiToolDef represents a tool definition in Gemini's format.
type geminiToolDef struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

// geminiFunctionDeclaration describes a function for the Gemini API.
type geminiFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// BuildCompleteRequest builds a non-streaming completion request for Gemini.
func (b *geminiRequestBuilder) BuildCompleteRequest(cfg *Config, req *provider.CompletionRequest) (*http.Request, error) {
	body, err := b.buildBody(cfg, req)
	if err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = cfg.DefaultModel
	}

	url := fmt.Sprintf("%s/v1/models/%s:generateContent",
		strings.TrimRight(cfg.BaseURL, "/"), model)

	httpReq, err := marshalAndCreateRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}

	b.setAuth(httpReq, cfg)
	return httpReq, nil
}

// BuildStreamRequest builds a streaming completion request for Gemini.
func (b *geminiRequestBuilder) BuildStreamRequest(cfg *Config, req *provider.CompletionRequest) (*http.Request, error) {
	body, err := b.buildBody(cfg, req)
	if err != nil {
		return nil, err
	}

	model := req.Model
	if model == "" {
		model = cfg.DefaultModel
	}

	url := fmt.Sprintf("%s/v1/models/%s:streamGenerateContent?alt=sse",
		strings.TrimRight(cfg.BaseURL, "/"), model)

	httpReq, err := marshalAndCreateRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}

	b.setAuth(httpReq, cfg)
	httpReq.Header.Set("Accept", "text/event-stream")
	return httpReq, nil
}

// BuildHealthRequest builds a health check request for Gemini (list models).
func (b *geminiRequestBuilder) BuildHealthRequest(cfg *Config) (*http.Request, error) {
	url := fmt.Sprintf("%s/v1/models?key=%s",
		strings.TrimRight(cfg.BaseURL, "/"), cfg.APIKey)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create health check request: %w", err)
	}
	return req, nil
}

// setAuth sets the authentication for Gemini requests.
// Gemini supports both API key as URL parameter and as header.
func (b *geminiRequestBuilder) setAuth(req *http.Request, cfg *Config) {
	// Use header-based auth with x-goog-api-key.
	req.Header.Set("x-goog-api-key", cfg.APIKey)
}

// buildBody constructs the request body for a Gemini completion.
func (b *geminiRequestBuilder) buildBody(cfg *Config, req *provider.CompletionRequest) (*geminiCompletionRequest, error) {
	// Build messages list, injecting system prompt if present.
	messages := req.Messages
	if req.SystemPrompt != "" {
		sysMsg := provider.Message{Role: provider.RoleSystem, Content: req.SystemPrompt}
		messages = append([]provider.Message{sysMsg}, messages...)
	}

	// Convert to Gemini format using the adapter.
	geminiContent, err := b.adapter.ToGemini(messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages to Gemini format: %w", err)
	}

	body := &geminiCompletionRequest{
		Contents:          geminiContent.Contents,
		SystemInstruction: geminiContent.SystemInstruction,
	}

	// Set generation config if any parameters are specified.
	if req.Temperature != nil || req.MaxTokens != nil || req.TopP != nil {
		body.GenerationConfig = &geminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
			TopP:            req.TopP,
		}
	}

	// Default safety settings: block none.
	body.SafetySettings = []geminiSafetySetting{
		{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "BLOCK_NONE"},
		{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "BLOCK_NONE"},
		{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "BLOCK_NONE"},
		{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "BLOCK_NONE"},
	}

	// Convert tools if present.
	if len(req.Tools) > 0 {
		decls := make([]geminiFunctionDeclaration, len(req.Tools))
		for i, t := range req.Tools {
			decls[i] = geminiFunctionDeclaration{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			}
		}
		body.Tools = []geminiToolDef{{FunctionDeclarations: decls}}
	}

	return body, nil
}

// =============================================================================
// Gemini Response Parser
// =============================================================================

// geminiResponseParser parses Gemini API responses.
type geminiResponseParser struct{}

// geminiCompletionResponse is the response body from Gemini's generateContent.
type geminiCompletionResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata,omitempty"`
	ModelVersion  string            `json:"modelVersion,omitempty"`
}

type geminiCandidate struct {
	Content       provider.GeminiContentEntry `json:"content"`
	FinishReason  string                      `json:"finishReason"`
	Index         int                         `json:"index"`
	SafetyRatings []geminiSafetyRating        `json:"safetyRatings,omitempty"`
}

type geminiSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// geminiErrorResponse is the error response from the Gemini API.
type geminiErrorResponse struct {
	Error geminiErrorDetail `json:"error"`
}

type geminiErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ParseCompletion parses a non-streaming Gemini completion response.
func (p *geminiResponseParser) ParseCompletion(resp *http.Response, providerName string) (*provider.CompletionResponse, error) {
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

	var geminiResp geminiCompletionResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, &provider.ProviderError{
			Provider: providerName,
			Code:     resp.StatusCode,
			Message:  "failed to parse response body",
			Err:      err,
		}
	}

	return convertGeminiResponse(&geminiResp, providerName), nil
}

// convertGeminiResponse converts a Gemini API response to the normalized format.
func convertGeminiResponse(geminiResp *geminiCompletionResponse, providerName string) *provider.CompletionResponse {
	result := &provider.CompletionResponse{
		ID:       fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
		Provider: providerName,
		Created:  time.Now(),
	}

	if geminiResp.UsageMetadata != nil {
		result.Usage = provider.Usage{
			PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		}
	}

	result.Choices = make([]provider.Choice, len(geminiResp.Candidates))
	for i, candidate := range geminiResp.Candidates {
		msg := provider.Message{
			Role: provider.RoleAssistant,
		}

		// Extract text content from parts.
		var textParts []string
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				textParts = append(textParts, part.Text)
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				msg.ToolCalls = append(msg.ToolCalls, provider.ToolCall{
					ID:   fmt.Sprintf("call_%s", part.FunctionCall.Name),
					Type: "function",
					Function: provider.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}
		if len(textParts) > 0 {
			msg.Content = strings.Join(textParts, "")
		}

		// Set model from candidate's content role if available.
		if candidate.Content.Role == "model" {
			msg.Role = provider.RoleAssistant
		}

		finishReason := mapGeminiFinishReason(candidate.FinishReason)

		result.Choices[i] = provider.Choice{
			Index:        candidate.Index,
			Message:      msg,
			FinishReason: finishReason,
		}
	}

	// Set top-level finish reason and model.
	if len(result.Choices) > 0 {
		result.FinishReason = result.Choices[0].FinishReason
	}

	return result
}

// mapGeminiFinishReason converts Gemini finish reasons to the normalized format.
func mapGeminiFinishReason(geminiReason string) string {
	switch geminiReason {
	case "STOP":
		return provider.FinishReasonStop
	case "MAX_TOKENS":
		return provider.FinishReasonLength
	case "SAFETY":
		return "content_filter"
	case "RECITATION":
		return "content_filter"
	case "OTHER":
		return provider.FinishReasonStop
	default:
		return geminiReason
	}
}

// ParseStreamChunk parses a single SSE line from a Gemini streaming response.
func (p *geminiResponseParser) ParseStreamChunk(line string, providerName string) (*provider.StreamChunk, error) {
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

	var geminiResp geminiCompletionResponse
	if err := json.Unmarshal([]byte(data), &geminiResp); err != nil {
		return nil, &provider.ProviderError{
			Provider: providerName,
			Code:     0,
			Message:  "failed to parse Gemini stream chunk",
			Err:      err,
		}
	}

	chunk := &provider.StreamChunk{
		ID:       fmt.Sprintf("gemini-stream-%d", time.Now().UnixNano()),
		Provider: providerName,
	}

	if len(geminiResp.Candidates) > 0 {
		candidate := geminiResp.Candidates[0]

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				chunk.Delta.Content += part.Text
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				chunk.Delta.ToolCalls = append(chunk.Delta.ToolCalls, provider.ToolCall{
					ID:   fmt.Sprintf("call_%s", part.FunctionCall.Name),
					Type: "function",
					Function: provider.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}

		// Map role if present.
		if candidate.Content.Role == "model" {
			chunk.Delta.Role = provider.RoleAssistant
		}

		if candidate.FinishReason != "" {
			chunk.FinishReason = mapGeminiFinishReason(candidate.FinishReason)
		}
	}

	if geminiResp.UsageMetadata != nil {
		chunk.Usage = &provider.Usage{
			PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		}
	}

	return chunk, nil
}

// ParseError parses a Gemini error response into the appropriate error type.
func (p *geminiResponseParser) ParseError(resp *http.Response, providerName string) error {
	body, err := readBody(resp)
	if err != nil {
		return &provider.ProviderError{
			Provider: providerName,
			Code:     resp.StatusCode,
			Message:  "failed to read error response",
			Err:      err,
		}
	}

	var geminiErr geminiErrorResponse
	if err := json.Unmarshal(body, &geminiErr); err != nil {
		return &provider.ProviderError{
			Provider: providerName,
			Code:     resp.StatusCode,
			Message:  string(body),
		}
	}

	errMsg := geminiErr.Error.Message

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

	case http.StatusUnauthorized, http.StatusForbidden:
		return &provider.AuthenticationError{
			ProviderError: provider.ProviderError{
				Provider: providerName,
				Code:     resp.StatusCode,
				Message:  errMsg,
			},
		}

	case http.StatusBadRequest:
		if strings.Contains(errMsg, "context") ||
			strings.Contains(errMsg, "token") ||
			strings.Contains(strings.ToLower(errMsg), "too long") {
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

// geminiStreamArrayResponse handles the streaming array format from Gemini.
type geminiStreamArrayResponse struct {
	Items []geminiCompletionResponse
}

// parseGeminiStreamArray attempts to parse a JSON array of Gemini responses
// used in some streaming modes.
func parseGeminiStreamArray(data string) ([]geminiCompletionResponse, error) {
	var items []geminiCompletionResponse
	if err := json.Unmarshal([]byte(data), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// parseGeminiRetryAfter parses Gemini-specific retry-after information.
func parseGeminiRetryAfter(value string) time.Duration {
	if value == "" {
		return 1 * time.Second
	}

	// Try parsing as seconds.
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		return time.Duration(seconds * float64(time.Second))
	}

	return 1 * time.Second
}
