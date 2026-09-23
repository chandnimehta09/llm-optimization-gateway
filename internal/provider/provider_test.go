package provider

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// Compile-time interface satisfaction checks.
var _ Provider = (*mockProvider)(nil)
var _ MessageAdapter = (*DefaultMessageAdapter)(nil)

// mockProvider is a test implementation of the Provider interface.
type mockProvider struct {
	name         string
	models       []string
	apiFormat    APIFormat
	healthy      bool
	completeResp *CompletionResponse
	completeErr  error
	streamCh     chan StreamChunk
	streamErr    error
}

func (m *mockProvider) Name() string { return m.name }

func (m *mockProvider) Complete(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
	return m.completeResp, m.completeErr
}

func (m *mockProvider) CompleteStream(_ context.Context, _ *CompletionRequest) (<-chan StreamChunk, error) {
	if m.streamErr != nil {
		return nil, m.streamErr
	}
	return m.streamCh, nil
}

func (m *mockProvider) Models() []string          { return m.models }
func (m *mockProvider) IsHealthy(_ context.Context) bool { return m.healthy }
func (m *mockProvider) APIFormat() APIFormat       { return m.apiFormat }

// =============================================================================
// Provider Interface Tests
// =============================================================================

func TestProviderInterface_Complete(t *testing.T) {
	expected := &CompletionResponse{
		ID:       "test-123",
		Model:    "gpt-4",
		Provider: "openai",
		Choices: []Choice{
			{
				Index:        0,
				Message:      Message{Role: RoleAssistant, Content: "Hello!"},
				FinishReason: FinishReasonStop,
			},
		},
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		FinishReason: FinishReasonStop,
		Created:      time.Now(),
	}

	p := &mockProvider{
		name:         "test-openai",
		models:       []string{"gpt-4", "gpt-3.5-turbo"},
		apiFormat:    APIFormatOpenAI,
		healthy:      true,
		completeResp: expected,
	}

	var provider Provider = p

	req := &CompletionRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: RoleUser, Content: "Hello"},
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != expected.ID {
		t.Errorf("expected ID %q, got %q", expected.ID, resp.ID)
	}
	if resp.Model != expected.Model {
		t.Errorf("expected model %q, got %q", expected.Model, resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello!" {
		t.Errorf("expected content %q, got %q", "Hello!", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected total tokens 15, got %d", resp.Usage.TotalTokens)
	}
}

func TestProviderInterface_CompleteError(t *testing.T) {
	expectedErr := &ProviderError{
		Provider: "test",
		Code:     500,
		Message:  "internal error",
	}
	p := &mockProvider{
		name:        "test",
		completeErr: expectedErr,
	}

	_, err := p.Complete(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var pe *ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if pe.Code != 500 {
		t.Errorf("expected code 500, got %d", pe.Code)
	}
}

func TestProviderInterface_CompleteStream(t *testing.T) {
	ch := make(chan StreamChunk, 3)
	ch <- StreamChunk{ID: "s1", Delta: MessageDelta{Content: "Hel"}}
	ch <- StreamChunk{ID: "s1", Delta: MessageDelta{Content: "lo!"}}
	ch <- StreamChunk{ID: "s1", FinishReason: FinishReasonStop, Usage: &Usage{TotalTokens: 10}}
	close(ch)

	p := &mockProvider{
		name:      "test-stream",
		apiFormat: APIFormatOpenAI,
		streamCh:  ch,
	}

	stream, err := p.CompleteStream(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	var finalChunk StreamChunk
	for chunk := range stream {
		content += chunk.Delta.Content
		finalChunk = chunk
	}

	if content != "Hello!" {
		t.Errorf("expected streamed content %q, got %q", "Hello!", content)
	}
	if finalChunk.FinishReason != FinishReasonStop {
		t.Errorf("expected finish reason %q, got %q", FinishReasonStop, finalChunk.FinishReason)
	}
	if finalChunk.Usage == nil || finalChunk.Usage.TotalTokens != 10 {
		t.Error("expected usage in final chunk")
	}
}

func TestProviderInterface_StreamError(t *testing.T) {
	p := &mockProvider{
		name:      "test-stream-err",
		streamErr: &AuthenticationError{ProviderError: ProviderError{Provider: "test", Message: "bad key"}},
	}

	_, err := p.CompleteStream(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !IsAuthenticationError(err) {
		t.Errorf("expected AuthenticationError, got %T", err)
	}
}

func TestProviderInterface_Metadata(t *testing.T) {
	p := &mockProvider{
		name:      "openai-prod",
		models:    []string{"gpt-4", "gpt-4-turbo"},
		apiFormat: APIFormatOpenAI,
		healthy:   true,
	}

	if p.Name() != "openai-prod" {
		t.Errorf("expected name %q, got %q", "openai-prod", p.Name())
	}
	if p.APIFormat() != APIFormatOpenAI {
		t.Errorf("expected format %q, got %q", APIFormatOpenAI, p.APIFormat())
	}
	models := p.Models()
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if !p.IsHealthy(context.Background()) {
		t.Error("expected provider to be healthy")
	}
}

// =============================================================================
// Error Type Tests
// =============================================================================

func TestProviderError(t *testing.T) {
	err := &ProviderError{
		Provider: "openai",
		Code:     500,
		Message:  "server error",
	}
	if err.Error() != "provider openai (code 500): server error" {
		t.Errorf("unexpected error string: %s", err.Error())
	}
	if !IsProviderError(err) {
		t.Error("expected IsProviderError to be true")
	}
}

func TestProviderError_WithWrappedError(t *testing.T) {
	inner := errors.New("connection refused")
	err := &ProviderError{
		Provider: "gemini",
		Code:     503,
		Message:  "unavailable",
		Err:      inner,
	}
	expected := "provider gemini (code 503): unavailable: connection refused"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
	if !errors.Is(err, inner) {
		t.Error("expected Unwrap to return inner error")
	}
}

func TestRateLimitError(t *testing.T) {
	err := &RateLimitError{
		ProviderError: ProviderError{
			Provider: "openai",
			Code:     429,
			Message:  "too many requests",
		},
		RetryAfter: 30 * time.Second,
	}

	if !IsRateLimitError(err) {
		t.Error("expected IsRateLimitError to be true")
	}
	if !IsProviderError(err) {
		t.Error("expected IsProviderError to be true for RateLimitError")
	}
	if err.RetryAfter != 30*time.Second {
		t.Errorf("expected retry after 30s, got %s", err.RetryAfter)
	}
}

func TestContextLengthError(t *testing.T) {
	err := &ContextLengthError{
		ProviderError: ProviderError{
			Provider: "openai",
			Code:     400,
			Message:  "context too long",
		},
		MaxTokens:       4096,
		RequestedTokens: 8000,
	}

	if !IsContextLengthError(err) {
		t.Error("expected IsContextLengthError to be true")
	}
	if !IsProviderError(err) {
		t.Error("expected IsProviderError to be true for ContextLengthError")
	}
	if err.MaxTokens != 4096 {
		t.Errorf("expected max tokens 4096, got %d", err.MaxTokens)
	}
	if err.RequestedTokens != 8000 {
		t.Errorf("expected requested tokens 8000, got %d", err.RequestedTokens)
	}
}

func TestAuthenticationError(t *testing.T) {
	err := &AuthenticationError{
		ProviderError: ProviderError{
			Provider: "gemini",
			Code:     401,
			Message:  "invalid api key",
		},
	}

	if !IsAuthenticationError(err) {
		t.Error("expected IsAuthenticationError to be true")
	}
	if !IsProviderError(err) {
		t.Error("expected IsProviderError to be true for AuthenticationError")
	}
}

func TestErrorTypeDiscrimination(t *testing.T) {
	// Verify that error type checks are mutually exclusive for different types.
	rateLimitErr := &RateLimitError{
		ProviderError: ProviderError{Provider: "test", Code: 429, Message: "rate limited"},
		RetryAfter:    time.Second,
	}
	contextErr := &ContextLengthError{
		ProviderError:   ProviderError{Provider: "test", Code: 400, Message: "too long"},
		MaxTokens:       4096,
		RequestedTokens: 8000,
	}
	authErr := &AuthenticationError{
		ProviderError: ProviderError{Provider: "test", Code: 401, Message: "unauthorized"},
	}

	// RateLimitError should not match other specific types.
	if IsContextLengthError(rateLimitErr) {
		t.Error("RateLimitError should not match IsContextLengthError")
	}
	if IsAuthenticationError(rateLimitErr) {
		t.Error("RateLimitError should not match IsAuthenticationError")
	}

	// ContextLengthError should not match other specific types.
	if IsRateLimitError(contextErr) {
		t.Error("ContextLengthError should not match IsRateLimitError")
	}
	if IsAuthenticationError(contextErr) {
		t.Error("ContextLengthError should not match IsAuthenticationError")
	}

	// AuthenticationError should not match other specific types.
	if IsRateLimitError(authErr) {
		t.Error("AuthenticationError should not match IsRateLimitError")
	}
	if IsContextLengthError(authErr) {
		t.Error("AuthenticationError should not match IsContextLengthError")
	}

	// All should match IsProviderError.
	if !IsProviderError(rateLimitErr) {
		t.Error("RateLimitError should match IsProviderError")
	}
	if !IsProviderError(contextErr) {
		t.Error("ContextLengthError should match IsProviderError")
	}
	if !IsProviderError(authErr) {
		t.Error("AuthenticationError should match IsProviderError")
	}
}

// =============================================================================
// Types Tests
// =============================================================================

func TestAPIFormatConstants(t *testing.T) {
	if APIFormatOpenAI != "openai" {
		t.Errorf("expected %q, got %q", "openai", APIFormatOpenAI)
	}
	if APIFormatGemini != "gemini" {
		t.Errorf("expected %q, got %q", "gemini", APIFormatGemini)
	}
}

func TestProviderConfig_HasCapability(t *testing.T) {
	cfg := &ProviderConfig{
		Capabilities: CapabilityStreaming | CapabilityTools | CapabilitySystemPrompt,
	}

	if !cfg.HasCapability(CapabilityStreaming) {
		t.Error("expected streaming capability")
	}
	if !cfg.HasCapability(CapabilityTools) {
		t.Error("expected tools capability")
	}
	if !cfg.HasCapability(CapabilitySystemPrompt) {
		t.Error("expected system prompt capability")
	}
	if cfg.HasCapability(CapabilityVision) {
		t.Error("should not have vision capability")
	}
	if cfg.HasCapability(CapabilityJSON) {
		t.Error("should not have JSON capability")
	}
}

func TestCompletionRequest_Fields(t *testing.T) {
	temp := 0.7
	maxTokens := 100
	topP := 0.9

	req := &CompletionRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: RoleSystem, Content: "You are helpful"},
			{Role: RoleUser, Content: "Hello"},
		},
		Temperature:  &temp,
		MaxTokens:    &maxTokens,
		TopP:         &topP,
		SystemPrompt: "You are helpful",
		ToolChoice:   "auto",
		Tools: []Tool{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "get_weather",
					Description: "Get the weather",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
		},
		Metadata: map[string]string{
			"request_id": "req-123",
			"user_id":    "user-456",
		},
	}

	if req.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", req.Model)
	}
	if len(req.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(req.Messages))
	}
	if *req.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", *req.Temperature)
	}
	if *req.MaxTokens != 100 {
		t.Errorf("expected max tokens 100, got %d", *req.MaxTokens)
	}
	if len(req.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(req.Tools))
	}
	if req.Metadata["request_id"] != "req-123" {
		t.Errorf("expected metadata request_id %q, got %q", "req-123", req.Metadata["request_id"])
	}
}

// =============================================================================
// MessageAdapter Tests
// =============================================================================

func TestMessageAdapter_ToOpenAI_SimpleMessages(t *testing.T) {
	adapter := NewMessageAdapter()

	messages := []Message{
		{Role: RoleSystem, Content: "You are a helpful assistant"},
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there!"},
	}

	oaiMsgs, err := adapter.ToOpenAI(messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(oaiMsgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(oaiMsgs))
	}

	if oaiMsgs[0].Role != "system" || *oaiMsgs[0].Content != "You are a helpful assistant" {
		t.Errorf("system message mismatch: role=%q content=%v", oaiMsgs[0].Role, oaiMsgs[0].Content)
	}
	if oaiMsgs[1].Role != "user" || *oaiMsgs[1].Content != "Hello" {
		t.Errorf("user message mismatch")
	}
	if oaiMsgs[2].Role != "assistant" || *oaiMsgs[2].Content != "Hi there!" {
		t.Errorf("assistant message mismatch")
	}
}

func TestMessageAdapter_ToOpenAI_WithToolCalls(t *testing.T) {
	adapter := NewMessageAdapter()

	messages := []Message{
		{
			Role:    RoleAssistant,
			Content: "",
			ToolCalls: []ToolCall{
				{
					ID:   "call_123",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location":"NYC"}`,
					},
				},
			},
		},
		{
			Role:       RoleTool,
			Content:    `{"temp": 72}`,
			ToolCallID: "call_123",
		},
	}

	oaiMsgs, err := adapter.ToOpenAI(messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(oaiMsgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(oaiMsgs))
	}

	if len(oaiMsgs[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(oaiMsgs[0].ToolCalls))
	}
	if oaiMsgs[0].ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected function name %q, got %q", "get_weather", oaiMsgs[0].ToolCalls[0].Function.Name)
	}
	if oaiMsgs[1].ToolCallID != "call_123" {
		t.Errorf("expected tool_call_id %q, got %q", "call_123", oaiMsgs[1].ToolCallID)
	}
}

func TestMessageAdapter_ToOpenAI_MultiPart(t *testing.T) {
	adapter := NewMessageAdapter()

	messages := []Message{
		{
			Role: RoleUser,
			Parts: []ContentPart{
				{Type: "text", Text: "What is in this image?"},
				{Type: "image", ImageURL: "https://example.com/image.png"},
			},
		},
	}

	oaiMsgs, err := adapter.ToOpenAI(messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(oaiMsgs[0].MultiContent) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(oaiMsgs[0].MultiContent))
	}
	if oaiMsgs[0].MultiContent[0].Type != "text" {
		t.Errorf("expected first part type %q, got %q", "text", oaiMsgs[0].MultiContent[0].Type)
	}
	if oaiMsgs[0].MultiContent[1].Type != "image_url" {
		t.Errorf("expected second part type %q, got %q", "image_url", oaiMsgs[0].MultiContent[1].Type)
	}
	if oaiMsgs[0].MultiContent[1].ImageURL.URL != "https://example.com/image.png" {
		t.Errorf("expected image URL %q", "https://example.com/image.png")
	}
}

func TestMessageAdapter_ToGemini_SimpleMessages(t *testing.T) {
	adapter := NewMessageAdapter()

	messages := []Message{
		{Role: RoleSystem, Content: "You are a helpful assistant"},
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there!"},
	}

	gc, err := adapter.ToGemini(messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// System message should be in SystemInstruction.
	if gc.SystemInstruction == nil {
		t.Fatal("expected SystemInstruction to be set")
	}
	if gc.SystemInstruction.Parts[0].Text != "You are a helpful assistant" {
		t.Errorf("expected system instruction text %q", "You are a helpful assistant")
	}

	// Should have 2 content entries (user + model).
	if len(gc.Contents) != 2 {
		t.Fatalf("expected 2 content entries, got %d", len(gc.Contents))
	}
	if gc.Contents[0].Role != "user" {
		t.Errorf("expected role %q, got %q", "user", gc.Contents[0].Role)
	}
	if gc.Contents[1].Role != "model" {
		t.Errorf("expected role %q, got %q", "model", gc.Contents[1].Role)
	}
	if gc.Contents[1].Parts[0].Text != "Hi there!" {
		t.Errorf("expected model text %q", "Hi there!")
	}
}

func TestMessageAdapter_ToGemini_WithToolCalls(t *testing.T) {
	adapter := NewMessageAdapter()

	messages := []Message{
		{Role: RoleUser, Content: "What's the weather?"},
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location":"NYC"}`,
					},
				},
			},
		},
		{
			Role:       RoleTool,
			Content:    `{"temp": 72}`,
			ToolCallID: "get_weather",
		},
	}

	gc, err := adapter.ToGemini(messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(gc.Contents) != 3 {
		t.Fatalf("expected 3 content entries, got %d", len(gc.Contents))
	}

	// Second entry should be model with function call.
	modelEntry := gc.Contents[1]
	if modelEntry.Role != "model" {
		t.Errorf("expected role %q, got %q", "model", modelEntry.Role)
	}
	if len(modelEntry.Parts) != 1 || modelEntry.Parts[0].FunctionCall == nil {
		t.Fatal("expected function call in model entry")
	}
	if modelEntry.Parts[0].FunctionCall.Name != "get_weather" {
		t.Errorf("expected function name %q", "get_weather")
	}

	// Third entry should be user with function response.
	toolEntry := gc.Contents[2]
	if toolEntry.Role != "user" {
		t.Errorf("expected role %q, got %q", "user", toolEntry.Role)
	}
	if len(toolEntry.Parts) != 1 || toolEntry.Parts[0].FunctionResponse == nil {
		t.Fatal("expected function response in tool entry")
	}
	if toolEntry.Parts[0].FunctionResponse.Name != "get_weather" {
		t.Errorf("expected function response name %q", "get_weather")
	}
}

func TestMessageAdapter_FromOpenAI(t *testing.T) {
	adapter := NewMessageAdapter()

	systemContent := "You are helpful"
	userContent := "Hello"
	assistantContent := "Hi!"

	oaiMsgs := []OpenAIMessage{
		{Role: "system", Content: &systemContent},
		{Role: "user", Content: &userContent},
		{Role: "assistant", Content: &assistantContent},
	}

	messages, err := adapter.FromOpenAI(oaiMsgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].Role != RoleSystem || messages[0].Content != "You are helpful" {
		t.Errorf("system message mismatch")
	}
	if messages[1].Role != RoleUser || messages[1].Content != "Hello" {
		t.Errorf("user message mismatch")
	}
	if messages[2].Role != RoleAssistant || messages[2].Content != "Hi!" {
		t.Errorf("assistant message mismatch")
	}
}

func TestMessageAdapter_FromOpenAI_WithToolCalls(t *testing.T) {
	adapter := NewMessageAdapter()

	emptyContent := ""
	toolContent := `{"temp": 72}`

	oaiMsgs := []OpenAIMessage{
		{
			Role:    "assistant",
			Content: &emptyContent,
			ToolCalls: []OpenAIToolCall{
				{
					ID:   "call_abc",
					Type: "function",
					Function: OpenAIFunctionCall{
						Name:      "get_weather",
						Arguments: `{"location":"NYC"}`,
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    &toolContent,
			ToolCallID: "call_abc",
		},
	}

	messages, err := adapter.FromOpenAI(oaiMsgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if len(messages[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(messages[0].ToolCalls))
	}
	if messages[0].ToolCalls[0].ID != "call_abc" {
		t.Errorf("expected tool call ID %q", "call_abc")
	}
	if messages[1].ToolCallID != "call_abc" {
		t.Errorf("expected tool call ID %q in response", "call_abc")
	}
}

func TestMessageAdapter_FromGemini(t *testing.T) {
	adapter := NewMessageAdapter()

	gc := &GeminiContent{
		SystemInstruction: &GeminiSystemInstruction{
			Parts: []GeminiPart{{Text: "You are helpful"}},
		},
		Contents: []GeminiContentEntry{
			{
				Role:  "user",
				Parts: []GeminiPart{{Text: "Hello"}},
			},
			{
				Role:  "model",
				Parts: []GeminiPart{{Text: "Hi!"}},
			},
		},
	}

	messages, err := adapter.FromGemini(gc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].Role != RoleSystem || messages[0].Content != "You are helpful" {
		t.Errorf("system message mismatch: role=%q content=%q", messages[0].Role, messages[0].Content)
	}
	if messages[1].Role != RoleUser || messages[1].Content != "Hello" {
		t.Errorf("user message mismatch")
	}
	if messages[2].Role != RoleAssistant || messages[2].Content != "Hi!" {
		t.Errorf("assistant message mismatch")
	}
}

func TestMessageAdapter_FromGemini_NilContent(t *testing.T) {
	adapter := NewMessageAdapter()

	_, err := adapter.FromGemini(nil)
	if err == nil {
		t.Fatal("expected error for nil content")
	}
}

func TestMessageAdapter_FromGemini_WithFunctionCall(t *testing.T) {
	adapter := NewMessageAdapter()

	gc := &GeminiContent{
		Contents: []GeminiContentEntry{
			{
				Role:  "user",
				Parts: []GeminiPart{{Text: "What's the weather?"}},
			},
			{
				Role: "model",
				Parts: []GeminiPart{
					{
						FunctionCall: &GeminiFunctionCall{
							Name: "get_weather",
							Args: map[string]interface{}{"location": "NYC"},
						},
					},
				},
			},
			{
				Role: "user",
				Parts: []GeminiPart{
					{
						FunctionResponse: &GeminiFunctionResponse{
							Name:     "get_weather",
							Response: map[string]interface{}{"temp": float64(72)},
						},
					},
				},
			},
		},
	}

	messages, err := adapter.FromGemini(gc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// User message.
	if messages[0].Role != RoleUser || messages[0].Content != "What's the weather?" {
		t.Errorf("user message mismatch")
	}

	// Assistant with tool call.
	if messages[1].Role != RoleAssistant {
		t.Errorf("expected assistant role, got %q", messages[1].Role)
	}
	if len(messages[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(messages[1].ToolCalls))
	}
	if messages[1].ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected function name %q", "get_weather")
	}

	// Tool response.
	if messages[2].Role != RoleTool {
		t.Errorf("expected tool role, got %q", messages[2].Role)
	}
	if messages[2].ToolCallID != "get_weather" {
		t.Errorf("expected tool call ID %q, got %q", "get_weather", messages[2].ToolCallID)
	}
}

// =============================================================================
// Roundtrip Tests: OpenAI → Normalized → Gemini → Normalized
// =============================================================================

func TestMessageAdapter_OpenAIToGeminiRoundtrip(t *testing.T) {
	adapter := NewMessageAdapter()

	// Start with OpenAI messages.
	systemContent := "You are a helpful assistant"
	userContent := "What's the weather in NYC?"
	oaiMsgs := []OpenAIMessage{
		{Role: "system", Content: &systemContent},
		{Role: "user", Content: &userContent},
	}

	// OpenAI → Normalized.
	normalized, err := adapter.FromOpenAI(oaiMsgs)
	if err != nil {
		t.Fatalf("FromOpenAI failed: %v", err)
	}

	// Normalized → Gemini.
	gemini, err := adapter.ToGemini(normalized)
	if err != nil {
		t.Fatalf("ToGemini failed: %v", err)
	}

	// Verify Gemini structure.
	if gemini.SystemInstruction == nil {
		t.Fatal("expected system instruction")
	}
	if gemini.SystemInstruction.Parts[0].Text != "You are a helpful assistant" {
		t.Errorf("system instruction mismatch")
	}
	if len(gemini.Contents) != 1 {
		t.Fatalf("expected 1 content entry, got %d", len(gemini.Contents))
	}
	if gemini.Contents[0].Parts[0].Text != "What's the weather in NYC?" {
		t.Errorf("user content mismatch")
	}

	// Gemini → Normalized.
	roundtripped, err := adapter.FromGemini(gemini)
	if err != nil {
		t.Fatalf("FromGemini failed: %v", err)
	}

	// Verify semantic preservation.
	if len(roundtripped) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(roundtripped))
	}
	if roundtripped[0].Role != RoleSystem || roundtripped[0].Content != "You are a helpful assistant" {
		t.Errorf("system message not preserved: role=%q content=%q", roundtripped[0].Role, roundtripped[0].Content)
	}
	if roundtripped[1].Role != RoleUser || roundtripped[1].Content != "What's the weather in NYC?" {
		t.Errorf("user message not preserved: role=%q content=%q", roundtripped[1].Role, roundtripped[1].Content)
	}
}

func TestMessageAdapter_ToolCallRoundtrip(t *testing.T) {
	adapter := NewMessageAdapter()

	// Start with normalized messages containing tool calls.
	original := []Message{
		{Role: RoleUser, Content: "What's the weather?"},
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{
					ID:   "call_weather",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location":"NYC"}`,
					},
				},
			},
		},
		{
			Role:       RoleTool,
			Content:    `{"temp":72,"unit":"F"}`,
			ToolCallID: "get_weather",
		},
	}

	// Normalized → OpenAI.
	oaiMsgs, err := adapter.ToOpenAI(original)
	if err != nil {
		t.Fatalf("ToOpenAI failed: %v", err)
	}

	// OpenAI → Normalized.
	fromOAI, err := adapter.FromOpenAI(oaiMsgs)
	if err != nil {
		t.Fatalf("FromOpenAI failed: %v", err)
	}

	// Verify round-trip through OpenAI.
	if len(fromOAI) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(fromOAI))
	}
	if fromOAI[0].Content != "What's the weather?" {
		t.Errorf("user content not preserved")
	}
	if len(fromOAI[1].ToolCalls) != 1 || fromOAI[1].ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("tool call not preserved through OpenAI roundtrip")
	}
	if fromOAI[2].ToolCallID != "get_weather" {
		t.Errorf("tool call ID not preserved")
	}

	// Normalized → Gemini.
	gemini, err := adapter.ToGemini(original)
	if err != nil {
		t.Fatalf("ToGemini failed: %v", err)
	}

	// Gemini → Normalized.
	fromGemini, err := adapter.FromGemini(gemini)
	if err != nil {
		t.Fatalf("FromGemini failed: %v", err)
	}

	// Verify semantic preservation through Gemini.
	if len(fromGemini) != 3 {
		t.Fatalf("expected 3 messages from Gemini roundtrip, got %d", len(fromGemini))
	}
	if fromGemini[0].Role != RoleUser || fromGemini[0].Content != "What's the weather?" {
		t.Errorf("user message not preserved through Gemini")
	}
	if fromGemini[1].Role != RoleAssistant {
		t.Errorf("expected assistant role, got %q", fromGemini[1].Role)
	}
	if len(fromGemini[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call from Gemini, got %d", len(fromGemini[1].ToolCalls))
	}
	if fromGemini[1].ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("tool call name not preserved through Gemini")
	}

	// Verify tool call arguments are semantically equivalent.
	var origArgs, roundArgs map[string]interface{}
	if err := json.Unmarshal([]byte(original[1].ToolCalls[0].Function.Arguments), &origArgs); err != nil {
		t.Fatalf("failed to parse original args: %v", err)
	}
	if err := json.Unmarshal([]byte(fromGemini[1].ToolCalls[0].Function.Arguments), &roundArgs); err != nil {
		t.Fatalf("failed to parse roundtripped args: %v", err)
	}
	if origArgs["location"] != roundArgs["location"] {
		t.Errorf("tool call args not preserved: original=%v roundtripped=%v", origArgs, roundArgs)
	}

	// Tool response.
	if fromGemini[2].Role != RoleTool {
		t.Errorf("expected tool role, got %q", fromGemini[2].Role)
	}
	// Verify the tool response content is semantically equivalent.
	var origResp, roundResp map[string]interface{}
	if err := json.Unmarshal([]byte(original[2].Content), &origResp); err != nil {
		t.Fatalf("failed to parse original tool response: %v", err)
	}
	if err := json.Unmarshal([]byte(fromGemini[2].Content), &roundResp); err != nil {
		t.Fatalf("failed to parse roundtripped tool response: %v", err)
	}
	if origResp["temp"] != roundResp["temp"] {
		t.Errorf("tool response not preserved: original=%v roundtripped=%v", origResp, roundResp)
	}
}

func TestMessageAdapter_ToGemini_UnsupportedRole(t *testing.T) {
	adapter := NewMessageAdapter()

	messages := []Message{
		{Role: "unknown_role", Content: "test"},
	}

	_, err := adapter.ToGemini(messages)
	if err == nil {
		t.Fatal("expected error for unsupported role")
	}
}

func TestMessageAdapter_ToOpenAI_UnsupportedPartType(t *testing.T) {
	adapter := NewMessageAdapter()

	messages := []Message{
		{
			Role: RoleUser,
			Parts: []ContentPart{
				{Type: "video", Text: "unsupported"},
			},
		},
	}

	_, err := adapter.ToOpenAI(messages)
	if err == nil {
		t.Fatal("expected error for unsupported part type")
	}
}

func TestStreamChunk_ErrorField(t *testing.T) {
	chunk := StreamChunk{
		ID:    "test",
		Error: errors.New("stream interrupted"),
	}

	if chunk.Error == nil {
		t.Error("expected error in chunk")
	}
	if chunk.Error.Error() != "stream interrupted" {
		t.Errorf("unexpected error message: %s", chunk.Error.Error())
	}
}

func TestMessage_MultiPartPrecedence(t *testing.T) {
	// When Parts is non-empty, it should be used over Content.
	msg := Message{
		Role:    RoleUser,
		Content: "this should be ignored",
		Parts: []ContentPart{
			{Type: "text", Text: "this should be used"},
		},
	}

	adapter := NewMessageAdapter()
	oaiMsgs, err := adapter.ToOpenAI([]Message{msg})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use MultiContent, not Content.
	if len(oaiMsgs[0].MultiContent) != 1 {
		t.Fatalf("expected multi-content to be used")
	}
	if oaiMsgs[0].MultiContent[0].Text != "this should be used" {
		t.Errorf("expected Parts to take precedence over Content")
	}
	if oaiMsgs[0].Content != nil {
		t.Error("expected Content to be nil when Parts are present")
	}
}

func TestCapabilityFlags_Bitmask(t *testing.T) {
	// Verify capability flags are distinct powers of 2.
	caps := []Capability{
		CapabilityStreaming,
		CapabilityTools,
		CapabilityVision,
		CapabilityJSON,
		CapabilitySystemPrompt,
		CapabilityMultiTurn,
	}

	for i := 0; i < len(caps); i++ {
		for j := i + 1; j < len(caps); j++ {
			if caps[i]&caps[j] != 0 {
				t.Errorf("capability flags %d and %d overlap", caps[i], caps[j])
			}
		}
	}

	// Verify all capabilities combined.
	all := CapabilityStreaming | CapabilityTools | CapabilityVision | CapabilityJSON | CapabilitySystemPrompt | CapabilityMultiTurn
	cfg := &ProviderConfig{Capabilities: all}
	for _, cap := range caps {
		if !cfg.HasCapability(cap) {
			t.Errorf("expected all capabilities to be set, missing %d", cap)
		}
	}
}
