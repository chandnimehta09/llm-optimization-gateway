package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

// =============================================================================
// Config Tests
// =============================================================================

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid OpenAI config",
			config: Config{
				Name:      "test-openai",
				APIFormat: provider.APIFormatOpenAI,
				APIKey:    "sk-test-key",
				Models:    []string{"gpt-4"},
			},
			wantErr: false,
		},
		{
			name: "valid Gemini config",
			config: Config{
				Name:      "test-gemini",
				APIFormat: provider.APIFormatGemini,
				APIKey:    "gemini-test-key",
				Models:    []string{"gemini-pro"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: Config{
				APIFormat: provider.APIFormatOpenAI,
				APIKey:    "sk-test",
				Models:    []string{"gpt-4"},
			},
			wantErr: true,
			errMsg:  "provider name is required",
		},
		{
			name: "invalid API format",
			config: Config{
				Name:      "test",
				APIFormat: "invalid",
				APIKey:    "key",
				Models:    []string{"model"},
			},
			wantErr: true,
			errMsg:  "unsupported API format",
		},
		{
			name: "missing API key",
			config: Config{
				Name:      "test",
				APIFormat: provider.APIFormatOpenAI,
				Models:    []string{"gpt-4"},
			},
			wantErr: true,
			errMsg:  "API key is required",
		},
		{
			name: "no models",
			config: Config{
				Name:      "test",
				APIFormat: provider.APIFormatOpenAI,
				APIKey:    "key",
			},
			wantErr: true,
			errMsg:  "at least one model must be configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{
		Name:      "test",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "sk-test",
		Models:    []string{"gpt-4", "gpt-3.5-turbo"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.BaseURL != DefaultOpenAIBaseURL {
		t.Errorf("expected base URL %q, got %q", DefaultOpenAIBaseURL, cfg.BaseURL)
	}
	if cfg.DefaultModel != "gpt-4" {
		t.Errorf("expected default model %q, got %q", "gpt-4", cfg.DefaultModel)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", cfg.Timeout)
	}
	if cfg.MaxRetries != 2 {
		t.Errorf("expected max retries 2, got %d", cfg.MaxRetries)
	}

	// Gemini defaults.
	cfgG := Config{
		Name:      "test-gemini",
		APIFormat: provider.APIFormatGemini,
		APIKey:    "key",
		Models:    []string{"gemini-pro"},
	}
	if err := cfgG.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfgG.BaseURL != DefaultGeminiBaseURL {
		t.Errorf("expected base URL %q, got %q", DefaultGeminiBaseURL, cfgG.BaseURL)
	}
}

// =============================================================================
// OpenAI Request Building Tests
// =============================================================================

func TestOpenAI_BuildCompleteRequest(t *testing.T) {
	cfg := &Config{
		Name:         "test-openai",
		APIFormat:    provider.APIFormatOpenAI,
		APIKey:       "sk-test-key-123",
		BaseURL:      "https://api.openai.com",
		DefaultModel: "gpt-4",
		Models:       []string{"gpt-4"},
	}

	builder := &openAIRequestBuilder{adapter: provider.NewMessageAdapter()}

	temp := 0.7
	maxTokens := 100
	req := &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello, world!"},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
	}

	httpReq, err := builder.BuildCompleteRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify URL.
	if httpReq.URL.String() != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("unexpected URL: %s", httpReq.URL.String())
	}

	// Verify method.
	if httpReq.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", httpReq.Method)
	}

	// Verify auth header.
	authHeader := httpReq.Header.Get("Authorization")
	if authHeader != "Bearer sk-test-key-123" {
		t.Errorf("expected Bearer auth, got %q", authHeader)
	}

	// Verify body.
	body, _ := io.ReadAll(httpReq.Body)
	var reqBody openAICompletionRequest
	if err := json.Unmarshal(body, &reqBody); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	if reqBody.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", reqBody.Model)
	}
	if len(reqBody.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(reqBody.Messages))
	}
	if *reqBody.Messages[0].Content != "Hello, world!" {
		t.Errorf("unexpected message content: %v", reqBody.Messages[0].Content)
	}
	if reqBody.Stream {
		t.Error("expected stream=false for complete request")
	}
	if *reqBody.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", *reqBody.Temperature)
	}
	if *reqBody.MaxTokens != 100 {
		t.Errorf("expected max_tokens 100, got %d", *reqBody.MaxTokens)
	}
}

func TestOpenAI_BuildStreamRequest(t *testing.T) {
	cfg := &Config{
		Name:         "test-openai",
		APIFormat:    provider.APIFormatOpenAI,
		APIKey:       "sk-test-key",
		BaseURL:      "https://api.openai.com",
		DefaultModel: "gpt-4",
		Models:       []string{"gpt-4"},
	}

	builder := &openAIRequestBuilder{adapter: provider.NewMessageAdapter()}

	req := &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	httpReq, err := builder.BuildStreamRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify stream header.
	if httpReq.Header.Get("Accept") != "text/event-stream" {
		t.Errorf("expected Accept: text/event-stream, got %q", httpReq.Header.Get("Accept"))
	}

	// Verify body has stream=true.
	body, _ := io.ReadAll(httpReq.Body)
	var reqBody openAICompletionRequest
	json.Unmarshal(body, &reqBody)

	if !reqBody.Stream {
		t.Error("expected stream=true for stream request")
	}
}

func TestOpenAI_BuildRequest_WithSystemPrompt(t *testing.T) {
	cfg := &Config{
		Name:         "test-openai",
		APIFormat:    provider.APIFormatOpenAI,
		APIKey:       "sk-test",
		BaseURL:      "https://api.openai.com",
		DefaultModel: "gpt-4",
		Models:       []string{"gpt-4"},
	}

	builder := &openAIRequestBuilder{adapter: provider.NewMessageAdapter()}

	req := &provider.CompletionRequest{
		Model:        "gpt-4",
		SystemPrompt: "You are a helpful assistant.",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	}

	httpReq, err := builder.BuildCompleteRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var reqBody openAICompletionRequest
	json.Unmarshal(body, &reqBody)

	if len(reqBody.Messages) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %d", len(reqBody.Messages))
	}
	if reqBody.Messages[0].Role != provider.RoleSystem {
		t.Errorf("expected first message role 'system', got %q", reqBody.Messages[0].Role)
	}
	if *reqBody.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("unexpected system prompt: %v", *reqBody.Messages[0].Content)
	}
}

func TestOpenAI_BuildRequest_WithTools(t *testing.T) {
	cfg := &Config{
		Name:         "test-openai",
		APIFormat:    provider.APIFormatOpenAI,
		APIKey:       "sk-test",
		BaseURL:      "https://api.openai.com",
		DefaultModel: "gpt-4",
		Models:       []string{"gpt-4"},
	}

	builder := &openAIRequestBuilder{adapter: provider.NewMessageAdapter()}

	req := &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "What's the weather?"},
		},
		Tools: []provider.Tool{
			{
				Type: "function",
				Function: provider.FunctionDefinition{
					Name:        "get_weather",
					Description: "Get the current weather",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "City name",
							},
						},
					},
				},
			},
		},
		ToolChoice: "auto",
	}

	httpReq, err := builder.BuildCompleteRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var reqBody map[string]interface{}
	json.Unmarshal(body, &reqBody)

	tools, ok := reqBody["tools"].([]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %v", reqBody["tools"])
	}
	if reqBody["tool_choice"] != "auto" {
		t.Errorf("expected tool_choice 'auto', got %v", reqBody["tool_choice"])
	}
}

func TestOpenAI_BuildRequest_DefaultModel(t *testing.T) {
	cfg := &Config{
		Name:         "test-openai",
		APIFormat:    provider.APIFormatOpenAI,
		APIKey:       "sk-test",
		BaseURL:      "https://api.openai.com",
		DefaultModel: "gpt-3.5-turbo",
		Models:       []string{"gpt-3.5-turbo"},
	}

	builder := &openAIRequestBuilder{adapter: provider.NewMessageAdapter()}

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	}

	httpReq, err := builder.BuildCompleteRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var reqBody openAICompletionRequest
	json.Unmarshal(body, &reqBody)

	if reqBody.Model != "gpt-3.5-turbo" {
		t.Errorf("expected default model gpt-3.5-turbo, got %s", reqBody.Model)
	}
}

// =============================================================================
// OpenAI Response Parsing Tests
// =============================================================================

func TestOpenAI_ParseCompletion(t *testing.T) {
	respBody := `{
		"id": "chatcmpl-123",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gpt-4",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "Hello! How can I help you?"
			},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 9,
			"completion_tokens": 12,
			"total_tokens": 21
		}
	}`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &openAIResponseParser{}
	result, err := parser.ParseCompletion(resp, "test-openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != "chatcmpl-123" {
		t.Errorf("expected ID chatcmpl-123, got %s", result.ID)
	}
	if result.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", result.Model)
	}
	if result.Provider != "test-openai" {
		t.Errorf("expected provider test-openai, got %s", result.Provider)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(result.Choices))
	}
	if result.Choices[0].Message.Content != "Hello! How can I help you?" {
		t.Errorf("unexpected content: %s", result.Choices[0].Message.Content)
	}
	if result.Choices[0].FinishReason != provider.FinishReasonStop {
		t.Errorf("expected finish reason 'stop', got %s", result.Choices[0].FinishReason)
	}
	if result.Usage.PromptTokens != 9 {
		t.Errorf("expected 9 prompt tokens, got %d", result.Usage.PromptTokens)
	}
	if result.Usage.CompletionTokens != 12 {
		t.Errorf("expected 12 completion tokens, got %d", result.Usage.CompletionTokens)
	}
	if result.Usage.TotalTokens != 21 {
		t.Errorf("expected 21 total tokens, got %d", result.Usage.TotalTokens)
	}
}

func TestOpenAI_ParseCompletion_WithToolCalls(t *testing.T) {
	respBody := `{
		"id": "chatcmpl-456",
		"object": "chat.completion",
		"created": 1677652288,
		"model": "gpt-4",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call_abc123",
					"type": "function",
					"function": {
						"name": "get_weather",
						"arguments": "{\"location\": \"San Francisco\"}"
					}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {
			"prompt_tokens": 20,
			"completion_tokens": 15,
			"total_tokens": 35
		}
	}`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &openAIResponseParser{}
	result, err := parser.ParseCompletion(resp, "test-openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.Choices[0].Message.ToolCalls))
	}

	tc := result.Choices[0].Message.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Errorf("expected tool call ID call_abc123, got %s", tc.ID)
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected function name get_weather, got %s", tc.Function.Name)
	}
	if result.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("expected finish reason tool_calls, got %s", result.Choices[0].FinishReason)
	}
}

func TestOpenAI_ParseStreamChunk(t *testing.T) {
	parser := &openAIResponseParser{}

	tests := []struct {
		name        string
		line        string
		wantNil     bool
		wantEOF     bool
		wantContent string
		wantRole    string
		wantFinish  string
	}{
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
		{
			name:    "comment line",
			line:    ": keep-alive",
			wantNil: true,
		},
		{
			name:    "non-data line",
			line:    "event: message",
			wantNil: true,
		},
		{
			name:    "done signal",
			line:    "data: [DONE]",
			wantEOF: true,
		},
		{
			name:        "content chunk",
			line:        `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			wantContent: "Hello",
		},
		{
			name:     "role chunk",
			line:     `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			wantRole: "assistant",
		},
		{
			name:       "finish chunk",
			line:       `data: {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			wantFinish: "stop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk, err := parser.ParseStreamChunk(tt.line, "test-openai")

			if tt.wantEOF {
				if err != io.EOF {
					t.Errorf("expected io.EOF, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if chunk != nil {
					t.Error("expected nil chunk")
				}
				return
			}

			if chunk == nil {
				t.Fatal("expected non-nil chunk")
			}

			if tt.wantContent != "" && chunk.Delta.Content != tt.wantContent {
				t.Errorf("expected content %q, got %q", tt.wantContent, chunk.Delta.Content)
			}
			if tt.wantRole != "" && chunk.Delta.Role != tt.wantRole {
				t.Errorf("expected role %q, got %q", tt.wantRole, chunk.Delta.Role)
			}
			if tt.wantFinish != "" && chunk.FinishReason != tt.wantFinish {
				t.Errorf("expected finish reason %q, got %q", tt.wantFinish, chunk.FinishReason)
			}
		})
	}
}

// =============================================================================
// OpenAI Error Mapping Tests
// =============================================================================

func TestOpenAI_ParseError_RateLimit(t *testing.T) {
	respBody := `{"error": {"message": "Rate limit exceeded", "type": "tokens", "param": null, "code": "rate_limit_exceeded"}}`

	header := make(http.Header)
	header.Set("Retry-After", "5")

	resp := &http.Response{
		StatusCode: 429,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     header,
	}

	parser := &openAIResponseParser{}
	err := parser.ParseError(resp, "test-openai")

	if !provider.IsRateLimitError(err) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}

	var rle *provider.RateLimitError
	if ok := errorAs(err, &rle); !ok {
		t.Fatal("failed to extract RateLimitError")
	}
	if rle.RetryAfter != 5*time.Second {
		t.Errorf("expected retry after 5s, got %v", rle.RetryAfter)
	}
}

func TestOpenAI_ParseError_Authentication(t *testing.T) {
	respBody := `{"error": {"message": "Invalid API key", "type": "invalid_request_error", "param": null, "code": "invalid_api_key"}}`

	resp := &http.Response{
		StatusCode: 401,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &openAIResponseParser{}
	err := parser.ParseError(resp, "test-openai")

	if !provider.IsAuthenticationError(err) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

func TestOpenAI_ParseError_ContextLength(t *testing.T) {
	respBody := `{"error": {"message": "This model's maximum context length is 8192 tokens", "type": "invalid_request_error", "param": null, "code": null}}`

	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &openAIResponseParser{}
	err := parser.ParseError(resp, "test-openai")

	if !provider.IsContextLengthError(err) {
		t.Fatalf("expected ContextLengthError, got %T: %v", err, err)
	}
}

func TestOpenAI_ParseError_ServerError(t *testing.T) {
	respBody := `{"error": {"message": "Internal server error", "type": "server_error", "param": null, "code": null}}`

	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &openAIResponseParser{}
	err := parser.ParseError(resp, "test-openai")

	if !provider.IsProviderError(err) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}

	var pe *provider.ProviderError
	if ok := errorAs(err, &pe); !ok {
		t.Fatal("failed to extract ProviderError")
	}
	if pe.Code != 500 {
		t.Errorf("expected code 500, got %d", pe.Code)
	}
}

// =============================================================================
// Gemini Request Building Tests
// =============================================================================

func TestGemini_BuildCompleteRequest(t *testing.T) {
	cfg := &Config{
		Name:         "test-gemini",
		APIFormat:    provider.APIFormatGemini,
		APIKey:       "gemini-test-key-123",
		BaseURL:      "https://generativelanguage.googleapis.com",
		DefaultModel: "gemini-pro",
		Models:       []string{"gemini-pro"},
	}

	builder := &geminiRequestBuilder{adapter: provider.NewMessageAdapter()}

	req := &provider.CompletionRequest{
		Model: "gemini-pro",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello, Gemini!"},
		},
	}

	httpReq, err := builder.BuildCompleteRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify URL.
	expectedURL := "https://generativelanguage.googleapis.com/v1/models/gemini-pro:generateContent"
	if httpReq.URL.String() != expectedURL {
		t.Errorf("expected URL %q, got %q", expectedURL, httpReq.URL.String())
	}

	// Verify method.
	if httpReq.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", httpReq.Method)
	}

	// Verify auth header.
	apiKey := httpReq.Header.Get("x-goog-api-key")
	if apiKey != "gemini-test-key-123" {
		t.Errorf("expected x-goog-api-key header, got %q", apiKey)
	}

	// Verify body.
	body, _ := io.ReadAll(httpReq.Body)
	var reqBody geminiCompletionRequest
	if err := json.Unmarshal(body, &reqBody); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	if len(reqBody.Contents) != 1 {
		t.Fatalf("expected 1 content entry, got %d", len(reqBody.Contents))
	}
	if reqBody.Contents[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", reqBody.Contents[0].Role)
	}
	if len(reqBody.Contents[0].Parts) != 1 || reqBody.Contents[0].Parts[0].Text != "Hello, Gemini!" {
		t.Errorf("unexpected content parts: %+v", reqBody.Contents[0].Parts)
	}

	// Verify safety settings are present.
	if len(reqBody.SafetySettings) != 4 {
		t.Errorf("expected 4 safety settings, got %d", len(reqBody.SafetySettings))
	}
}

func TestGemini_BuildCompleteRequest_WithSystemPrompt(t *testing.T) {
	cfg := &Config{
		Name:         "test-gemini",
		APIFormat:    provider.APIFormatGemini,
		APIKey:       "key",
		BaseURL:      "https://generativelanguage.googleapis.com",
		DefaultModel: "gemini-pro",
		Models:       []string{"gemini-pro"},
	}

	builder := &geminiRequestBuilder{adapter: provider.NewMessageAdapter()}

	req := &provider.CompletionRequest{
		Model:        "gemini-pro",
		SystemPrompt: "You are a helpful assistant.",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	}

	httpReq, err := builder.BuildCompleteRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var reqBody geminiCompletionRequest
	json.Unmarshal(body, &reqBody)

	if reqBody.SystemInstruction == nil {
		t.Fatal("expected system instruction")
	}
	if len(reqBody.SystemInstruction.Parts) != 1 || reqBody.SystemInstruction.Parts[0].Text != "You are a helpful assistant." {
		t.Errorf("unexpected system instruction: %+v", reqBody.SystemInstruction)
	}
}

func TestGemini_BuildCompleteRequest_WithGenerationConfig(t *testing.T) {
	cfg := &Config{
		Name:         "test-gemini",
		APIFormat:    provider.APIFormatGemini,
		APIKey:       "key",
		BaseURL:      "https://generativelanguage.googleapis.com",
		DefaultModel: "gemini-pro",
		Models:       []string{"gemini-pro"},
	}

	builder := &geminiRequestBuilder{adapter: provider.NewMessageAdapter()}

	temp := 0.5
	maxTokens := 200
	topP := 0.9

	req := &provider.CompletionRequest{
		Model: "gemini-pro",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		TopP:        &topP,
	}

	httpReq, err := builder.BuildCompleteRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var reqBody geminiCompletionRequest
	json.Unmarshal(body, &reqBody)

	if reqBody.GenerationConfig == nil {
		t.Fatal("expected generation config")
	}
	if *reqBody.GenerationConfig.Temperature != 0.5 {
		t.Errorf("expected temperature 0.5, got %f", *reqBody.GenerationConfig.Temperature)
	}
	if *reqBody.GenerationConfig.MaxOutputTokens != 200 {
		t.Errorf("expected maxOutputTokens 200, got %d", *reqBody.GenerationConfig.MaxOutputTokens)
	}
	if *reqBody.GenerationConfig.TopP != 0.9 {
		t.Errorf("expected topP 0.9, got %f", *reqBody.GenerationConfig.TopP)
	}
}

func TestGemini_BuildStreamRequest(t *testing.T) {
	cfg := &Config{
		Name:         "test-gemini",
		APIFormat:    provider.APIFormatGemini,
		APIKey:       "key",
		BaseURL:      "https://generativelanguage.googleapis.com",
		DefaultModel: "gemini-pro",
		Models:       []string{"gemini-pro"},
	}

	builder := &geminiRequestBuilder{adapter: provider.NewMessageAdapter()}

	req := &provider.CompletionRequest{
		Model: "gemini-pro",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	httpReq, err := builder.BuildStreamRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify URL includes streamGenerateContent and alt=sse.
	urlStr := httpReq.URL.String()
	if !strings.Contains(urlStr, "streamGenerateContent") {
		t.Errorf("expected URL to contain 'streamGenerateContent', got %q", urlStr)
	}
	if !strings.Contains(urlStr, "alt=sse") {
		t.Errorf("expected URL to contain 'alt=sse', got %q", urlStr)
	}

	// Verify Accept header.
	if httpReq.Header.Get("Accept") != "text/event-stream" {
		t.Errorf("expected Accept: text/event-stream, got %q", httpReq.Header.Get("Accept"))
	}
}

func TestGemini_BuildRequest_WithTools(t *testing.T) {
	cfg := &Config{
		Name:         "test-gemini",
		APIFormat:    provider.APIFormatGemini,
		APIKey:       "key",
		BaseURL:      "https://generativelanguage.googleapis.com",
		DefaultModel: "gemini-pro",
		Models:       []string{"gemini-pro"},
	}

	builder := &geminiRequestBuilder{adapter: provider.NewMessageAdapter()}

	req := &provider.CompletionRequest{
		Model: "gemini-pro",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "What's the weather?"},
		},
		Tools: []provider.Tool{
			{
				Type: "function",
				Function: provider.FunctionDefinition{
					Name:        "get_weather",
					Description: "Get weather for a location",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{"type": "string"},
						},
					},
				},
			},
		},
	}

	httpReq, err := builder.BuildCompleteRequest(cfg, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(httpReq.Body)
	var reqBody geminiCompletionRequest
	json.Unmarshal(body, &reqBody)

	if len(reqBody.Tools) != 1 {
		t.Fatalf("expected 1 tool group, got %d", len(reqBody.Tools))
	}
	if len(reqBody.Tools[0].FunctionDeclarations) != 1 {
		t.Fatalf("expected 1 function declaration, got %d", len(reqBody.Tools[0].FunctionDeclarations))
	}
	if reqBody.Tools[0].FunctionDeclarations[0].Name != "get_weather" {
		t.Errorf("expected function name 'get_weather', got %q", reqBody.Tools[0].FunctionDeclarations[0].Name)
	}
}

// =============================================================================
// Gemini Response Parsing Tests
// =============================================================================

func TestGemini_ParseCompletion(t *testing.T) {
	respBody := `{
		"candidates": [{
			"content": {
				"parts": [{"text": "Hello! How can I help?"}],
				"role": "model"
			},
			"finishReason": "STOP",
			"index": 0
		}],
		"usageMetadata": {
			"promptTokenCount": 5,
			"candidatesTokenCount": 8,
			"totalTokenCount": 13
		}
	}`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &geminiResponseParser{}
	result, err := parser.ParseCompletion(resp, "test-gemini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Provider != "test-gemini" {
		t.Errorf("expected provider test-gemini, got %s", result.Provider)
	}
	if len(result.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(result.Choices))
	}
	if result.Choices[0].Message.Content != "Hello! How can I help?" {
		t.Errorf("unexpected content: %s", result.Choices[0].Message.Content)
	}
	if result.Choices[0].Message.Role != provider.RoleAssistant {
		t.Errorf("expected role assistant, got %s", result.Choices[0].Message.Role)
	}
	if result.Choices[0].FinishReason != provider.FinishReasonStop {
		t.Errorf("expected finish reason 'stop', got %s", result.Choices[0].FinishReason)
	}
	if result.Usage.PromptTokens != 5 {
		t.Errorf("expected 5 prompt tokens, got %d", result.Usage.PromptTokens)
	}
	if result.Usage.CompletionTokens != 8 {
		t.Errorf("expected 8 completion tokens, got %d", result.Usage.CompletionTokens)
	}
	if result.Usage.TotalTokens != 13 {
		t.Errorf("expected 13 total tokens, got %d", result.Usage.TotalTokens)
	}
}

func TestGemini_ParseCompletion_WithFunctionCall(t *testing.T) {
	respBody := `{
		"candidates": [{
			"content": {
				"parts": [{
					"functionCall": {
						"name": "get_weather",
						"args": {"location": "Tokyo"}
					}
				}],
				"role": "model"
			},
			"finishReason": "STOP",
			"index": 0
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 5,
			"totalTokenCount": 15
		}
	}`

	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &geminiResponseParser{}
	result, err := parser.ParseCompletion(resp, "test-gemini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.Choices[0].Message.ToolCalls))
	}

	tc := result.Choices[0].Message.ToolCalls[0]
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected function name get_weather, got %s", tc.Function.Name)
	}
	if tc.Type != "function" {
		t.Errorf("expected type function, got %s", tc.Type)
	}
}

func TestGemini_MapFinishReason(t *testing.T) {
	tests := []struct {
		gemini   string
		expected string
	}{
		{"STOP", provider.FinishReasonStop},
		{"MAX_TOKENS", provider.FinishReasonLength},
		{"SAFETY", "content_filter"},
		{"RECITATION", "content_filter"},
		{"OTHER", provider.FinishReasonStop},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.gemini, func(t *testing.T) {
			result := mapGeminiFinishReason(tt.gemini)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGemini_ParseStreamChunk(t *testing.T) {
	parser := &geminiResponseParser{}

	tests := []struct {
		name        string
		line        string
		wantNil     bool
		wantEOF     bool
		wantContent string
		wantFinish  string
	}{
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
		{
			name:    "done signal",
			line:    "data: [DONE]",
			wantEOF: true,
		},
		{
			name:        "content chunk",
			line:        `data: {"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":"","index":0}]}`,
			wantContent: "Hello",
		},
		{
			name:       "finish chunk",
			line:       `data: {"candidates":[{"content":{"parts":[{"text":""}],"role":"model"},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":10,"totalTokenCount":15}}`,
			wantFinish: provider.FinishReasonStop,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk, err := parser.ParseStreamChunk(tt.line, "test-gemini")

			if tt.wantEOF {
				if err != io.EOF {
					t.Errorf("expected io.EOF, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if chunk != nil {
					t.Error("expected nil chunk")
				}
				return
			}

			if chunk == nil {
				t.Fatal("expected non-nil chunk")
			}

			if tt.wantContent != "" && chunk.Delta.Content != tt.wantContent {
				t.Errorf("expected content %q, got %q", tt.wantContent, chunk.Delta.Content)
			}
			if tt.wantFinish != "" && chunk.FinishReason != tt.wantFinish {
				t.Errorf("expected finish reason %q, got %q", tt.wantFinish, chunk.FinishReason)
			}
		})
	}
}

// =============================================================================
// Gemini Error Mapping Tests
// =============================================================================

func TestGemini_ParseError_RateLimit(t *testing.T) {
	respBody := `{"error": {"code": 429, "message": "Resource exhausted", "status": "RESOURCE_EXHAUSTED"}}`

	header := make(http.Header)
	header.Set("Retry-After", "10")

	resp := &http.Response{
		StatusCode: 429,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     header,
	}

	parser := &geminiResponseParser{}
	err := parser.ParseError(resp, "test-gemini")

	if !provider.IsRateLimitError(err) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}

	var rle *provider.RateLimitError
	if ok := errorAs(err, &rle); !ok {
		t.Fatal("failed to extract RateLimitError")
	}
	if rle.RetryAfter != 10*time.Second {
		t.Errorf("expected retry after 10s, got %v", rle.RetryAfter)
	}
}

func TestGemini_ParseError_Authentication(t *testing.T) {
	respBody := `{"error": {"code": 401, "message": "Invalid API key", "status": "UNAUTHENTICATED"}}`

	resp := &http.Response{
		StatusCode: 401,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &geminiResponseParser{}
	err := parser.ParseError(resp, "test-gemini")

	if !provider.IsAuthenticationError(err) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

func TestGemini_ParseError_Forbidden(t *testing.T) {
	respBody := `{"error": {"code": 403, "message": "Permission denied", "status": "PERMISSION_DENIED"}}`

	resp := &http.Response{
		StatusCode: 403,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &geminiResponseParser{}
	err := parser.ParseError(resp, "test-gemini")

	if !provider.IsAuthenticationError(err) {
		t.Fatalf("expected AuthenticationError for 403, got %T: %v", err, err)
	}
}

func TestGemini_ParseError_ServerError(t *testing.T) {
	respBody := `{"error": {"code": 500, "message": "Internal error", "status": "INTERNAL"}}`

	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	parser := &geminiResponseParser{}
	err := parser.ParseError(resp, "test-gemini")

	if !provider.IsProviderError(err) {
		t.Fatalf("expected ProviderError, got %T: %v", err, err)
	}
}

// =============================================================================
// Integration Tests with httptest.Server
// =============================================================================

func TestIntegration_OpenAI_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request.
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing Bearer auth")
		}

		// Parse request body.
		var reqBody openAICompletionRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody.Model != "gpt-4" {
			t.Errorf("expected model gpt-4, got %s", reqBody.Model)
		}

		// Send response.
		resp := map[string]interface{}{
			"id":      "chatcmpl-test-123",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Integration test response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     15,
				"completion_tokens": 5,
				"total_tokens":      20,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:         "test-openai",
		APIFormat:    provider.APIFormatOpenAI,
		APIKey:       "sk-test-integration",
		BaseURL:      server.URL,
		Models:       []string{"gpt-4"},
		DefaultModel: "gpt-4",
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	resp, err := p.Complete(context.Background(), &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello, test!"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "chatcmpl-test-123" {
		t.Errorf("expected ID chatcmpl-test-123, got %s", resp.ID)
	}
	if resp.Provider != "test-openai" {
		t.Errorf("expected provider test-openai, got %s", resp.Provider)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Integration test response" {
		t.Errorf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 20 {
		t.Errorf("expected 20 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestIntegration_OpenAI_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"error": {"message": "Rate limit exceeded", "type": "tokens", "param": null, "code": "rate_limit_exceeded"}}`)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:      "test-openai",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "sk-test",
		BaseURL:   server.URL,
		Models:    []string{"gpt-4"},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	_, err = p.Complete(context.Background(), &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	})

	if !provider.IsRateLimitError(err) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}
}

func TestIntegration_OpenAI_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		// Send role chunk.
		fmt.Fprintf(w, "data: %s\n\n", `{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`)
		flusher.Flush()

		// Send content chunks.
		for _, word := range []string{"Hello", " ", "world", "!"} {
			chunk := fmt.Sprintf(`{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"%s"},"finish_reason":null}]}`, word)
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}

		// Send finish chunk.
		fmt.Fprintf(w, "data: %s\n\n", `{"id":"chatcmpl-stream","object":"chat.completion.chunk","created":1677652288,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9}}`)
		flusher.Flush()

		// Send done signal.
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:      "test-openai",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "sk-test",
		BaseURL:   server.URL,
		Models:    []string{"gpt-4"},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	ch, err := p.CompleteStream(context.Background(), &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var fullContent string
	var gotRole bool
	var gotFinish bool
	var gotUsage bool

	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		if chunk.Delta.Role == "assistant" {
			gotRole = true
		}
		fullContent += chunk.Delta.Content
		if chunk.FinishReason == "stop" {
			gotFinish = true
		}
		if chunk.Usage != nil {
			gotUsage = true
			if chunk.Usage.TotalTokens != 9 {
				t.Errorf("expected 9 total tokens, got %d", chunk.Usage.TotalTokens)
			}
		}
	}

	if !gotRole {
		t.Error("expected to receive role chunk")
	}
	if fullContent != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", fullContent)
	}
	if !gotFinish {
		t.Error("expected finish reason")
	}
	if !gotUsage {
		t.Error("expected usage data in final chunk")
	}
}

func TestIntegration_Gemini_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path.
		expectedPath := "/v1/models/gemini-pro:generateContent"
		if r.URL.Path != expectedPath {
			t.Errorf("expected path %q, got %q", expectedPath, r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		// Verify auth.
		if r.Header.Get("x-goog-api-key") == "" {
			t.Error("missing x-goog-api-key header")
		}

		// Parse request body.
		var reqBody geminiCompletionRequest
		json.NewDecoder(r.Body).Decode(&reqBody)
		if len(reqBody.Contents) == 0 {
			t.Error("expected contents in request")
		}

		// Send response.
		resp := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": "Gemini integration response"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
					"index":        0,
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     10,
				"candidatesTokenCount": 6,
				"totalTokenCount":      16,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:         "test-gemini",
		APIFormat:    provider.APIFormatGemini,
		APIKey:       "gemini-test-key",
		BaseURL:      server.URL,
		Models:       []string{"gemini-pro"},
		DefaultModel: "gemini-pro",
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	resp, err := p.Complete(context.Background(), &provider.CompletionRequest{
		Model: "gemini-pro",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello, Gemini!"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Provider != "test-gemini" {
		t.Errorf("expected provider test-gemini, got %s", resp.Provider)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Gemini integration response" {
		t.Errorf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != provider.FinishReasonStop {
		t.Errorf("expected finish reason 'stop', got %s", resp.Choices[0].FinishReason)
	}
	if resp.Usage.TotalTokens != 16 {
		t.Errorf("expected 16 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestIntegration_Gemini_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "15")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprintf(w, `{"error": {"code": 429, "message": "Resource exhausted", "status": "RESOURCE_EXHAUSTED"}}`)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:      "test-gemini",
		APIFormat: provider.APIFormatGemini,
		APIKey:    "key",
		BaseURL:   server.URL,
		Models:    []string{"gemini-pro"},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	_, err = p.Complete(context.Background(), &provider.CompletionRequest{
		Model: "gemini-pro",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	})

	if !provider.IsRateLimitError(err) {
		t.Fatalf("expected RateLimitError, got %T: %v", err, err)
	}

	var rle *provider.RateLimitError
	if ok := errorAs(err, &rle); !ok {
		t.Fatal("failed to extract RateLimitError")
	}
	if rle.RetryAfter != 15*time.Second {
		t.Errorf("expected retry after 15s, got %v", rle.RetryAfter)
	}
}

func TestIntegration_Gemini_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify URL contains streamGenerateContent.
		if !strings.Contains(r.URL.Path, "streamGenerateContent") {
			t.Errorf("expected streamGenerateContent in path, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(200)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		// Send chunks.
		chunks := []string{
			`{"candidates":[{"content":{"parts":[{"text":"Hello"}],"role":"model"},"finishReason":"","index":0}]}`,
			`{"candidates":[{"content":{"parts":[{"text":" from"}],"role":"model"},"finishReason":"","index":0}]}`,
			`{"candidates":[{"content":{"parts":[{"text":" Gemini!"}],"role":"model"},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":6,"totalTokenCount":11}}`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:      "test-gemini",
		APIFormat: provider.APIFormatGemini,
		APIKey:    "key",
		BaseURL:   server.URL,
		Models:    []string{"gemini-pro"},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	ch, err := p.CompleteStream(context.Background(), &provider.CompletionRequest{
		Model: "gemini-pro",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var fullContent string
	var gotFinish bool
	var gotUsage bool

	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream error: %v", chunk.Error)
		}
		fullContent += chunk.Delta.Content
		if chunk.FinishReason == provider.FinishReasonStop {
			gotFinish = true
		}
		if chunk.Usage != nil {
			gotUsage = true
			if chunk.Usage.TotalTokens != 11 {
				t.Errorf("expected 11 total tokens, got %d", chunk.Usage.TotalTokens)
			}
		}
	}

	if fullContent != "Hello from Gemini!" {
		t.Errorf("expected 'Hello from Gemini!', got %q", fullContent)
	}
	if !gotFinish {
		t.Error("expected finish reason")
	}
	if !gotUsage {
		t.Error("expected usage data")
	}
}

// =============================================================================
// Provider Lifecycle Tests
// =============================================================================

func TestProvider_New_InvalidConfig(t *testing.T) {
	_, err := New(&Config{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestProvider_Name(t *testing.T) {
	p, err := New(&Config{
		Name:      "my-provider",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "key",
		Models:    []string{"gpt-4"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "my-provider" {
		t.Errorf("expected name 'my-provider', got %q", p.Name())
	}
}

func TestProvider_Models(t *testing.T) {
	p, err := New(&Config{
		Name:      "test",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "key",
		Models:    []string{"gpt-4", "gpt-3.5-turbo"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	models := p.Models()
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
}

func TestProvider_APIFormat(t *testing.T) {
	p, err := New(&Config{
		Name:      "test",
		APIFormat: provider.APIFormatGemini,
		APIKey:    "key",
		Models:    []string{"gemini-pro"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.APIFormat() != provider.APIFormatGemini {
		t.Errorf("expected API format %q, got %q", provider.APIFormatGemini, p.APIFormat())
	}
}

func TestProvider_IsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprintf(w, `{"data": []}`)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:      "test",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "key",
		BaseURL:   server.URL,
		Models:    []string{"gpt-4"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer p.Close()

	if !p.IsHealthy(context.Background()) {
		t.Error("expected healthy provider")
	}
}

func TestProvider_IsHealthy_Unhealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:      "test",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "key",
		BaseURL:   server.URL,
		Models:    []string{"gpt-4"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer p.Close()

	if p.IsHealthy(context.Background()) {
		t.Error("expected unhealthy provider")
	}
}

func TestIntegration_OpenAI_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error": {"message": "Invalid API key", "type": "invalid_request_error", "param": null, "code": "invalid_api_key"}}`)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:      "test-openai",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "bad-key",
		BaseURL:   server.URL,
		Models:    []string{"gpt-4"},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	_, err = p.Complete(context.Background(), &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	})

	// Auth errors should not be retried.
	if !provider.IsAuthenticationError(err) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

func TestIntegration_OpenAI_Retry_TransientError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(w, `{"error": {"message": "Internal server error", "type": "server_error", "param": null, "code": null}}`)
			return
		}

		// Success on third attempt.
		resp := map[string]interface{}{
			"id":      "chatcmpl-retry",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": "Success after retry"},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:       "test-openai",
		APIFormat:  provider.APIFormatOpenAI,
		APIKey:     "sk-test",
		BaseURL:    server.URL,
		Models:     []string{"gpt-4"},
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	resp, err := p.Complete(context.Background(), &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error after retries: %v", err)
	}

	if resp.Choices[0].Message.Content != "Success after retry" {
		t.Errorf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestIntegration_OpenAI_Stream_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"error": {"message": "Invalid API key", "type": "invalid_request_error"}}`)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:      "test-openai",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "bad-key",
		BaseURL:   server.URL,
		Models:    []string{"gpt-4"},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	_, err = p.CompleteStream(context.Background(), &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	})

	if !provider.IsAuthenticationError(err) {
		t.Fatalf("expected AuthenticationError, got %T: %v", err, err)
	}
}

func TestProvider_CustomHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "custom-value" {
			t.Errorf("expected custom header, got %q", r.Header.Get("X-Custom-Header"))
		}

		resp := map[string]interface{}{
			"id":      "chatcmpl-custom",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]interface{}{"role": "assistant", "content": "OK"},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p, err := New(&Config{
		Name:      "test-openai",
		APIFormat: provider.APIFormatOpenAI,
		APIKey:    "key",
		BaseURL:   server.URL,
		Models:    []string{"gpt-4"},
		CustomHeaders: map[string]string{
			"X-Custom-Header": "custom-value",
		},
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	_, err = p.Complete(context.Background(), &provider.CompletionRequest{
		Model: "gpt-4",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected time.Duration
	}{
		{"empty", "", 1 * time.Second},
		{"seconds", "5", 5 * time.Second},
		{"float seconds", "2.5", 2500 * time.Millisecond},
		{"invalid", "invalid", 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRetryAfter(tt.value)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// =============================================================================
// Interface Satisfaction Tests
// =============================================================================

func TestProvider_ImplementsInterface(t *testing.T) {
	// This is a compile-time check; the var_ line at top of provider.go ensures it.
	var _ provider.Provider = (*Provider)(nil)
}

// =============================================================================
// Helpers
// =============================================================================

// errorAs is a helper wrapping errors.As for use in tests.
func errorAs(err error, target interface{}) bool {
	switch v := target.(type) {
	case **provider.RateLimitError:
		return errAsRateLimit(err, v)
	case **provider.AuthenticationError:
		return errAsAuth(err, v)
	case **provider.ContextLengthError:
		return errAsContextLength(err, v)
	case **provider.ProviderError:
		return errAsProvider(err, v)
	default:
		return false
	}
}

func errAsRateLimit(err error, target **provider.RateLimitError) bool {
	var rle *provider.RateLimitError
	if ok := extractError(err, &rle); ok {
		*target = rle
		return true
	}
	return false
}

func errAsAuth(err error, target **provider.AuthenticationError) bool {
	var ae *provider.AuthenticationError
	if ok := extractError(err, &ae); ok {
		*target = ae
		return true
	}
	return false
}

func errAsContextLength(err error, target **provider.ContextLengthError) bool {
	var cle *provider.ContextLengthError
	if ok := extractError(err, &cle); ok {
		*target = cle
		return true
	}
	return false
}

func errAsProvider(err error, target **provider.ProviderError) bool {
	var pe *provider.ProviderError
	if ok := extractError(err, &pe); ok {
		*target = pe
		return true
	}
	return false
}

// extractError uses type switch to extract errors without importing errors package.
func extractError(err error, target interface{}) bool {
	switch v := target.(type) {
	case **provider.RateLimitError:
		for e := err; e != nil; {
			if rle, ok := e.(*provider.RateLimitError); ok {
				*v = rle
				return true
			}
			if uw, ok := e.(interface{ Unwrap() error }); ok {
				e = uw.Unwrap()
			} else {
				break
			}
		}
	case **provider.AuthenticationError:
		for e := err; e != nil; {
			if ae, ok := e.(*provider.AuthenticationError); ok {
				*v = ae
				return true
			}
			if uw, ok := e.(interface{ Unwrap() error }); ok {
				e = uw.Unwrap()
			} else {
				break
			}
		}
	case **provider.ContextLengthError:
		for e := err; e != nil; {
			if cle, ok := e.(*provider.ContextLengthError); ok {
				*v = cle
				return true
			}
			if uw, ok := e.(interface{ Unwrap() error }); ok {
				e = uw.Unwrap()
			} else {
				break
			}
		}
	case **provider.ProviderError:
		for e := err; e != nil; {
			if pe, ok := e.(*provider.ProviderError); ok {
				*v = pe
				return true
			}
			if uw, ok := e.(interface{ Unwrap() error }); ok {
				e = uw.Unwrap()
			} else {
				break
			}
		}
	}
	return false
}
