package request

import (
	"encoding/json"
	"strings"
	"testing"
)

// =============================================================================
// Message Normalization Tests
// =============================================================================

func TestNormalizeOpenAI_StringContentToArray(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello, world!"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nr.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(nr.Messages))
	}

	msg := nr.Messages[0]
	if len(msg.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(msg.Parts))
	}
	if msg.Parts[0].Type != "text" {
		t.Errorf("expected part type 'text', got %q", msg.Parts[0].Type)
	}
	if msg.Parts[0].Text != "Hello, world!" {
		t.Errorf("expected part text 'Hello, world!', got %q", msg.Parts[0].Text)
	}

	// Verify the string_to_array change was tracked.
	found := false
	for _, c := range nr.Changes {
		if c.Action == "string_to_array" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'string_to_array' normalization change to be tracked")
	}
}

func TestNormalizeOpenAI_ArrayContentPreserved(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type": "text", "text": "What is in this image?"},
					{"type": "image_url", "image_url": {"url": "https://example.com/image.png"}}
				]`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := nr.Messages[0]
	if len(msg.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(msg.Parts))
	}
	if msg.Parts[0].Type != "text" {
		t.Errorf("expected first part type 'text', got %q", msg.Parts[0].Type)
	}
	if msg.Parts[0].Text != "What is in this image?" {
		t.Errorf("expected first part text 'What is in this image?', got %q", msg.Parts[0].Text)
	}
	if msg.Parts[1].Type != "image_url" {
		t.Errorf("expected second part type 'image_url', got %q", msg.Parts[1].Type)
	}
	if msg.Parts[1].ImageURL != "https://example.com/image.png" {
		t.Errorf("expected image URL 'https://example.com/image.png', got %q", msg.Parts[1].ImageURL)
	}
}

func TestNormalizeOpenAI_RoleLowercased(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "User",
				Content: json.RawMessage(`"Hello"`),
			},
			{
				Role:    "ASSISTANT",
				Content: json.RawMessage(`"Hi there"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.Messages[0].Role != "user" {
		t.Errorf("expected role 'user', got %q", nr.Messages[0].Role)
	}
	if nr.Messages[1].Role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", nr.Messages[1].Role)
	}

	// Verify lowercased changes were tracked.
	lowercaseChanges := 0
	for _, c := range nr.Changes {
		if c.Action == "lowercased" {
			lowercaseChanges++
		}
	}
	if lowercaseChanges != 2 {
		t.Errorf("expected 2 lowercased changes, got %d", lowercaseChanges)
	}
}

func TestNormalizeOpenAI_ToolMessages(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Call a function"`),
			},
			{
				Role: "assistant",
				ToolCalls: []OpenAIToolCallInput{
					{
						ID:   "call_123",
						Type: "function",
						Function: OpenAIFunctionCallInput{
							Name:      "get_weather",
							Arguments: `{"location":"NYC"}`,
						},
					},
				},
			},
			{
				Role:       "tool",
				Content:    json.RawMessage(`"Sunny, 72F"`),
				ToolCallID: "call_123",
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nr.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(nr.Messages))
	}

	// Check assistant message with tool calls.
	assistantMsg := nr.Messages[1]
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(assistantMsg.ToolCalls))
	}
	if assistantMsg.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected function name 'get_weather', got %q", assistantMsg.ToolCalls[0].Function.Name)
	}

	// Check tool response message.
	toolMsg := nr.Messages[2]
	if toolMsg.Role != "tool" {
		t.Errorf("expected role 'tool', got %q", toolMsg.Role)
	}
	if toolMsg.ToolCallID != "call_123" {
		t.Errorf("expected tool_call_id 'call_123', got %q", toolMsg.ToolCallID)
	}
}

func TestNormalizeOpenAI_SystemPromptExtracted(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "system",
				Content: json.RawMessage(`"You are a helpful assistant."`),
			},
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("expected system prompt 'You are a helpful assistant.', got %q", nr.SystemPrompt)
	}

	// System message should be removed from messages.
	if len(nr.Messages) != 1 {
		t.Fatalf("expected 1 message after system extraction, got %d", len(nr.Messages))
	}
	if nr.Messages[0].Role != "user" {
		t.Errorf("expected remaining message role 'user', got %q", nr.Messages[0].Role)
	}
}

// =============================================================================
// Gemini Message Conversion Tests
// =============================================================================

func TestNormalizeGemini_BasicConversation(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Model: "gemini-1.5-pro",
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello, Gemini!"}},
			},
			{
				Role:  "model",
				Parts: []GeminiPartInput{{Text: "Hello! How can I help you?"}},
			},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nr.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(nr.Messages))
	}

	if nr.Messages[0].Role != "user" {
		t.Errorf("expected first message role 'user', got %q", nr.Messages[0].Role)
	}
	if nr.Messages[1].Role != "assistant" {
		t.Errorf("expected second message role 'assistant', got %q", nr.Messages[1].Role)
	}

	// Check that model → assistant mapping was tracked.
	found := false
	for _, c := range nr.Changes {
		if c.Action == "gemini_role_mapped" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'gemini_role_mapped' change to be tracked")
	}
}

func TestNormalizeGemini_SystemInstruction(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Model: "gemini-1.5-pro",
		SystemInstruction: &GeminiSystemInstructionInput{
			Parts: []GeminiPartInput{{Text: "You are a code assistant."}},
		},
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Write hello world"}},
			},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.SystemPrompt != "You are a code assistant." {
		t.Errorf("expected system prompt 'You are a code assistant.', got %q", nr.SystemPrompt)
	}
}

func TestNormalizeGemini_FunctionCallAndResponse(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Model: "gemini-1.5-pro",
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "What's the weather?"}},
			},
			{
				Role: "model",
				Parts: []GeminiPartInput{
					{
						FunctionCall: &GeminiFunctionCallInput{
							Name: "get_weather",
							Args: map[string]interface{}{"location": "NYC"},
						},
					},
				},
			},
			{
				Role: "user",
				Parts: []GeminiPartInput{
					{
						FunctionResponse: &GeminiFunctionResponseInput{
							Name:     "get_weather",
							Response: map[string]interface{}{"temp": "72F"},
						},
					},
				},
			},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 messages: user, assistant (with tool call), tool (response).
	if len(nr.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(nr.Messages))
	}

	// Check assistant's tool call.
	if len(nr.Messages[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(nr.Messages[1].ToolCalls))
	}
	if nr.Messages[1].ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected function name 'get_weather', got %q", nr.Messages[1].ToolCalls[0].Function.Name)
	}

	// Check tool response.
	if nr.Messages[2].Role != "tool" {
		t.Errorf("expected role 'tool', got %q", nr.Messages[2].Role)
	}
	if nr.Messages[2].ToolCallID != "get_weather" {
		t.Errorf("expected tool_call_id 'get_weather', got %q", nr.Messages[2].ToolCallID)
	}
}

func TestNormalizeGemini_InlineImageData(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Model: "gemini-1.5-pro",
		Contents: []GeminiContentInput{
			{
				Role: "user",
				Parts: []GeminiPartInput{
					{Text: "What is in this image?"},
					{
						InlineData: &GeminiInlineDataInput{
							MIMEType: "image/png",
							Data:     "base64data",
						},
					},
				},
			},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := nr.Messages[0]
	if len(msg.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(msg.Parts))
	}
	if msg.Parts[0].Type != "text" {
		t.Errorf("expected first part type 'text', got %q", msg.Parts[0].Type)
	}
	if msg.Parts[1].Type != "image" {
		t.Errorf("expected second part type 'image', got %q", msg.Parts[1].Type)
	}
	if msg.Parts[1].MIMEType != "image/png" {
		t.Errorf("expected MIME type 'image/png', got %q", msg.Parts[1].MIMEType)
	}
}

func TestNormalizeGemini_GenerationConfig(t *testing.T) {
	n := NewNormalizer(nil)
	temp := 0.7
	topP := 0.9
	maxTokens := 500
	req := &GeminiRequest{
		Model: "gemini-1.5-pro",
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     &temp,
			TopP:            &topP,
			MaxOutputTokens: &maxTokens,
			StopSequences:   []string{"END"},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.Temperature == nil || *nr.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", nr.Temperature)
	}
	if nr.TopP == nil || *nr.TopP != 0.9 {
		t.Errorf("expected top_p 0.9, got %v", nr.TopP)
	}
	if nr.MaxTokens == nil || *nr.MaxTokens != 500 {
		t.Errorf("expected max_tokens 500, got %v", nr.MaxTokens)
	}
	if len(nr.Stop) != 1 || nr.Stop[0] != "END" {
		t.Errorf("expected stop sequence ['END'], got %v", nr.Stop)
	}
}

func TestNormalizeGemini_Tools(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Model: "gemini-1.5-pro",
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
		Tools: []GeminiToolInput{
			{
				FunctionDeclarations: []GeminiFunctionDeclarationInput{
					{
						Name:        "get_weather",
						Description: "Get weather for a location",
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
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nr.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(nr.Tools))
	}
	if nr.Tools[0].Type != "function" {
		t.Errorf("expected tool type 'function', got %q", nr.Tools[0].Type)
	}
	if nr.Tools[0].Function.Name != "get_weather" {
		t.Errorf("expected function name 'get_weather', got %q", nr.Tools[0].Function.Name)
	}
}

// =============================================================================
// Parameter Normalization Tests
// =============================================================================

func TestNormalizeOpenAI_DefaultsApplied(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.Temperature == nil {
		t.Fatal("expected temperature to have default value")
	}
	if *nr.Temperature != DefaultTemperature {
		t.Errorf("expected default temperature %v, got %v", DefaultTemperature, *nr.Temperature)
	}

	if nr.TopP == nil {
		t.Fatal("expected top_p to have default value")
	}
	if *nr.TopP != DefaultTopP {
		t.Errorf("expected default top_p %v, got %v", DefaultTopP, *nr.TopP)
	}

	// Verify defaults were tracked.
	defaultChanges := 0
	for _, c := range nr.Changes {
		if c.Action == "default_applied" {
			defaultChanges++
		}
	}
	if defaultChanges < 2 {
		t.Errorf("expected at least 2 default_applied changes, got %d", defaultChanges)
	}
}

func TestNormalizeOpenAI_ExplicitParametersPreserved(t *testing.T) {
	n := NewNormalizer(nil)
	temp := 0.5
	topP := 0.8
	maxTokens := 100
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTokens,
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if *nr.Temperature != 0.5 {
		t.Errorf("expected temperature 0.5, got %v", *nr.Temperature)
	}
	if *nr.TopP != 0.8 {
		t.Errorf("expected top_p 0.8, got %v", *nr.TopP)
	}
	if *nr.MaxTokens != 100 {
		t.Errorf("expected max_tokens 100, got %v", *nr.MaxTokens)
	}
}

func TestNormalizeOpenAI_CustomConfig(t *testing.T) {
	config := &NormalizerConfig{
		ModelAliases:            map[string]string{"my-model": "gpt-4o"},
		ApplyDefaultTemperature: false,
		ApplyDefaultTopP:        false,
	}
	n := NewNormalizer(config)
	req := &OpenAIRequest{
		Model: "my-model",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.Temperature != nil {
		t.Errorf("expected no default temperature, got %v", *nr.Temperature)
	}
	if nr.TopP != nil {
		t.Errorf("expected no default top_p, got %v", *nr.TopP)
	}
	if nr.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", nr.Model)
	}
}

// =============================================================================
// Model Name Normalization Tests
// =============================================================================

func TestNormalizeOpenAI_ModelWithProviderPrefix(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		wantModel    string
		wantProvider string
	}{
		{
			name:         "openai prefix",
			model:        "openai/gpt-4",
			wantModel:    "gpt-4",
			wantProvider: "openai",
		},
		{
			name:         "google prefix",
			model:        "google/gemini-1.5-pro",
			wantModel:    "gemini-1.5-pro",
			wantProvider: "google",
		},
		{
			name:         "anthropic prefix",
			model:        "anthropic/claude-3",
			wantModel:    "claude-3",
			wantProvider: "anthropic",
		},
		{
			name:         "no prefix with openai model",
			model:        "gpt-4",
			wantModel:    "gpt-4",
			wantProvider: "openai",
		},
		{
			name:         "no prefix with gemini model",
			model:        "gemini-1.5-pro",
			wantModel:    "gemini-1.5-pro",
			wantProvider: "gemini",
		},
		{
			name:         "alias resolution",
			model:        "gpt-3.5",
			wantModel:    "gpt-3.5-turbo",
			wantProvider: "openai",
		},
		{
			name:         "unknown model no hint",
			model:        "custom-model",
			wantModel:    "custom-model",
			wantProvider: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNormalizer(nil)
			req := &OpenAIRequest{
				Model: tt.model,
				Messages: []OpenAIMessageInput{
					{
						Role:    "user",
						Content: json.RawMessage(`"Hello"`),
					},
				},
			}

			nr, err := n.NormalizeOpenAI(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if nr.Model != tt.wantModel {
				t.Errorf("expected model %q, got %q", tt.wantModel, nr.Model)
			}
			if nr.ProviderHint != tt.wantProvider {
				t.Errorf("expected provider hint %q, got %q", tt.wantProvider, nr.ProviderHint)
			}
		})
	}
}

func TestStripProviderPrefix(t *testing.T) {
	tests := []struct {
		input    string
		model    string
		provider string
	}{
		{"openai/gpt-4", "gpt-4", "openai"},
		{"google/gemini-pro", "gemini-pro", "google"},
		{"gemini/gemini-1.5-pro", "gemini-1.5-pro", "gemini"},
		{"anthropic/claude-3", "claude-3", "anthropic"},
		{"azure/gpt-4", "gpt-4", "azure"},
		{"gpt-4", "gpt-4", ""},
		{"custom-model", "custom-model", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			model, prov := StripProviderPrefix(tt.input)
			if model != tt.model {
				t.Errorf("expected model %q, got %q", tt.model, model)
			}
			if prov != tt.provider {
				t.Errorf("expected provider %q, got %q", tt.provider, prov)
			}
		})
	}
}

func TestResolveModelAlias(t *testing.T) {
	aliases := map[string]string{
		"gpt-3.5": "gpt-3.5-turbo",
		"gemini":  "gemini-pro",
	}

	tests := []struct {
		input string
		want  string
	}{
		{"gpt-3.5", "gpt-3.5-turbo"},
		{"gemini", "gemini-pro"},
		{"gpt-4", "gpt-4"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveModelAlias(tt.input, aliases)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestDetectProviderFromModel(t *testing.T) {
	tests := []struct {
		model    string
		provider string
	}{
		{"gpt-4", "openai"},
		{"gpt-3.5-turbo", "openai"},
		{"o1-preview", "openai"},
		{"o3-mini", "openai"},
		{"gemini-pro", "gemini"},
		{"gemini-1.5-pro", "gemini"},
		{"gemini-2.0-flash", "gemini"},
		{"models/gemini-pro", "gemini"},
		{"claude-3", ""},
		{"custom-model", ""},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := DetectProviderFromModel(tt.model)
			if got != tt.provider {
				t.Errorf("expected %q, got %q", tt.provider, got)
			}
		})
	}
}

// =============================================================================
// Validation Tests
// =============================================================================

func TestValidateOpenAIRequest_MissingModel(t *testing.T) {
	req := &OpenAIRequest{
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	vr := ValidateOpenAIRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for missing model")
	}

	found := false
	for _, e := range vr.Errors {
		if e.Field == "model" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected model validation error")
	}
}

func TestValidateOpenAIRequest_MissingMessages(t *testing.T) {
	req := &OpenAIRequest{
		Model: "gpt-4",
	}

	vr := ValidateOpenAIRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for missing messages")
	}
}

func TestValidateOpenAIRequest_TemperatureTooHigh(t *testing.T) {
	temp := 2.5
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
		Temperature: &temp,
	}

	vr := ValidateOpenAIRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for temperature > 2")
	}

	found := false
	for _, e := range vr.Errors {
		if e.Field == "temperature" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected temperature validation error")
	}
}

func TestValidateOpenAIRequest_TemperatureNegative(t *testing.T) {
	temp := -0.5
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
		Temperature: &temp,
	}

	vr := ValidateOpenAIRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for negative temperature")
	}
}

func TestValidateOpenAIRequest_TopPOutOfRange(t *testing.T) {
	topP := 1.5
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
		TopP: &topP,
	}

	vr := ValidateOpenAIRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for top_p > 1")
	}
}

func TestValidateOpenAIRequest_MaxTokensZero(t *testing.T) {
	maxTokens := 0
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
		MaxTokens: &maxTokens,
	}

	vr := ValidateOpenAIRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for max_tokens < 1")
	}
}

func TestValidateOpenAIRequest_Valid(t *testing.T) {
	temp := 0.7
	topP := 0.9
	maxTokens := 100
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   &maxTokens,
	}

	vr := ValidateOpenAIRequest(req)
	if !vr.Valid {
		t.Errorf("expected validation to pass, errors: %v", vr.Errors)
	}
}

func TestValidateGeminiRequest_MissingContents(t *testing.T) {
	req := &GeminiRequest{}

	vr := ValidateGeminiRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for missing contents")
	}
}

func TestValidateGeminiRequest_TemperatureOutOfRange(t *testing.T) {
	temp := 3.0
	req := &GeminiRequest{
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
		GenerationConfig: &GeminiGenerationConfig{
			Temperature: &temp,
		},
	}

	vr := ValidateGeminiRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for temperature > 2")
	}
}

func TestValidateNormalizedRequest_MissingModel(t *testing.T) {
	nr := &NormalizedRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for missing model")
	}
}

func TestValidateNormalizedRequest_InvalidRole(t *testing.T) {
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "invalid_role", Content: "Hello"},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for invalid role")
	}
}

func TestValidateNormalizedRequest_ToolWithoutType(t *testing.T) {
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Tools: []Tool{
			{Function: FunctionDefinition{Name: "test"}},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for tool without type")
	}
}

func TestValidateNormalizedRequest_ToolWithoutFunctionName(t *testing.T) {
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Tools: []Tool{
			{Type: "function", Function: FunctionDefinition{}},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for tool without function name")
	}
}

func TestValidateNormalizedRequest_ToolMessageWithoutCallID(t *testing.T) {
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "tool", Content: "result"},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for tool message without tool_call_id")
	}
}

func TestValidateNormalizedRequest_EmptyTextPart(t *testing.T) {
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{
				Role: "user",
				Parts: []ContentPart{
					{Type: "text", Text: ""},
				},
			},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for empty text part")
	}
}

func TestValidateNormalizedRequest_InvalidContentType(t *testing.T) {
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{
				Role: "user",
				Parts: []ContentPart{
					{Type: "video", Text: "test"},
				},
			},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for invalid content type")
	}
}

// =============================================================================
// Normalization Idempotency Tests (Property-based)
// =============================================================================

func TestNormalization_Idempotency_OpenAI(t *testing.T) {
	// A normalized request, when serialized back to OpenAI format and re-normalized,
	// should produce equivalent output.
	n := NewNormalizer(nil)

	original := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello, world!"`),
			},
		},
	}

	nr1, err := n.NormalizeOpenAI(original)
	if err != nil {
		t.Fatalf("first normalization failed: %v", err)
	}

	// Re-create a request from the normalized result and normalize again.
	msgs2 := make([]OpenAIMessageInput, len(nr1.Messages))
	for i, msg := range nr1.Messages {
		// Serialize parts back to content.
		if len(msg.Parts) > 0 {
			parts := make([]OpenAIContentPartInput, len(msg.Parts))
			for j, p := range msg.Parts {
				parts[j] = OpenAIContentPartInput{
					Type: p.Type,
					Text: p.Text,
				}
				if p.ImageURL != "" {
					parts[j].ImageURL = &OpenAIImageURLInput{URL: p.ImageURL}
				}
			}
			contentJSON, _ := json.Marshal(parts)
			msgs2[i] = OpenAIMessageInput{
				Role:    msg.Role,
				Content: contentJSON,
			}
		} else {
			contentJSON, _ := json.Marshal(msg.Content)
			msgs2[i] = OpenAIMessageInput{
				Role:    msg.Role,
				Content: contentJSON,
			}
		}
	}

	// Add system message back if it was extracted.
	if nr1.SystemPrompt != "" {
		sysContent, _ := json.Marshal(nr1.SystemPrompt)
		sysMsg := OpenAIMessageInput{
			Role:    "system",
			Content: sysContent,
		}
		msgs2 = append([]OpenAIMessageInput{sysMsg}, msgs2...)
	}

	reconstructed := &OpenAIRequest{
		Model:       nr1.Model,
		Messages:    msgs2,
		Temperature: nr1.Temperature,
		TopP:        nr1.TopP,
		MaxTokens:   nr1.MaxTokens,
	}

	nr2, err := n.NormalizeOpenAI(reconstructed)
	if err != nil {
		t.Fatalf("second normalization failed: %v", err)
	}

	// Compare key fields.
	if nr1.Model != nr2.Model {
		t.Errorf("model mismatch: %q vs %q", nr1.Model, nr2.Model)
	}
	if nr1.SystemPrompt != nr2.SystemPrompt {
		t.Errorf("system prompt mismatch: %q vs %q", nr1.SystemPrompt, nr2.SystemPrompt)
	}
	if *nr1.Temperature != *nr2.Temperature {
		t.Errorf("temperature mismatch: %v vs %v", *nr1.Temperature, *nr2.Temperature)
	}
	if *nr1.TopP != *nr2.TopP {
		t.Errorf("top_p mismatch: %v vs %v", *nr1.TopP, *nr2.TopP)
	}
}

func TestNormalization_Idempotency_Gemini(t *testing.T) {
	n := NewNormalizer(nil)

	original := &GeminiRequest{
		Model: "gemini-1.5-pro",
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello, Gemini!"}},
			},
		},
		SystemInstruction: &GeminiSystemInstructionInput{
			Parts: []GeminiPartInput{{Text: "You are helpful."}},
		},
	}

	nr1, err := n.NormalizeGemini(original)
	if err != nil {
		t.Fatalf("first normalization failed: %v", err)
	}

	// Reconstruct Gemini request from normalized.
	contents2 := make([]GeminiContentInput, len(nr1.Messages))
	for i, msg := range nr1.Messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		parts := make([]GeminiPartInput, 0)
		if len(msg.Parts) > 0 {
			for _, p := range msg.Parts {
				if p.Type == "text" {
					parts = append(parts, GeminiPartInput{Text: p.Text})
				}
			}
		} else if msg.Content != "" {
			parts = append(parts, GeminiPartInput{Text: msg.Content})
		}
		contents2[i] = GeminiContentInput{
			Role:  role,
			Parts: parts,
		}
	}

	reconstructed := &GeminiRequest{
		Model:    nr1.Model,
		Contents: contents2,
	}
	if nr1.SystemPrompt != "" {
		reconstructed.SystemInstruction = &GeminiSystemInstructionInput{
			Parts: []GeminiPartInput{{Text: nr1.SystemPrompt}},
		}
	}
	if nr1.Temperature != nil || nr1.TopP != nil || nr1.MaxTokens != nil {
		reconstructed.GenerationConfig = &GeminiGenerationConfig{
			Temperature:     nr1.Temperature,
			TopP:            nr1.TopP,
			MaxOutputTokens: nr1.MaxTokens,
		}
	}

	nr2, err := n.NormalizeGemini(reconstructed)
	if err != nil {
		t.Fatalf("second normalization failed: %v", err)
	}

	if nr1.Model != nr2.Model {
		t.Errorf("model mismatch: %q vs %q", nr1.Model, nr2.Model)
	}
	if nr1.SystemPrompt != nr2.SystemPrompt {
		t.Errorf("system prompt mismatch: %q vs %q", nr1.SystemPrompt, nr2.SystemPrompt)
	}
}

// =============================================================================
// Integration Test with Provider Interface
// =============================================================================

func TestNormalizedRequest_FullOpenAIPipeline(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "openai/gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "system",
				Content: json.RawMessage(`"You are helpful."`),
			},
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
		Tools: []OpenAIToolInput{
			{
				Type: "function",
				Function: OpenAIFunctionDefinitionInput{
					Name:        "get_weather",
					Description: "Get weather",
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

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify normalized fields.
	if nr.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", nr.Model)
	}
	if nr.ProviderHint != "openai" {
		t.Errorf("expected provider hint 'openai', got %q", nr.ProviderHint)
	}
	if nr.SystemPrompt != "You are helpful." {
		t.Errorf("expected system prompt 'You are helpful.', got %q", nr.SystemPrompt)
	}
	if len(nr.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(nr.Messages))
	}
	if nr.Messages[0].Role != "user" {
		t.Errorf("expected message role 'user', got %q", nr.Messages[0].Role)
	}
	if nr.Temperature == nil || *nr.Temperature != DefaultTemperature {
		t.Errorf("expected default temperature, got %v", nr.Temperature)
	}
	if len(nr.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(nr.Tools))
	}
	if nr.Tools[0].Function.Name != "get_weather" {
		t.Errorf("expected tool function name 'get_weather', got %q", nr.Tools[0].Function.Name)
	}
}

func TestNormalizedRequest_FullGeminiPipeline(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Model: "gemini-1.5-pro",
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello from Gemini"}},
			},
		},
		SystemInstruction: &GeminiSystemInstructionInput{
			Parts: []GeminiPartInput{{Text: "Be concise."}},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.Model != "gemini-1.5-pro" {
		t.Errorf("expected model 'gemini-1.5-pro', got %q", nr.Model)
	}
	if nr.SystemPrompt != "Be concise." {
		t.Errorf("expected system prompt 'Be concise.', got %q", nr.SystemPrompt)
	}
	if len(nr.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(nr.Messages))
	}
	if nr.ProviderHint != "gemini" {
		t.Errorf("expected provider hint 'gemini', got %q", nr.ProviderHint)
	}
}

// =============================================================================
// Edge Cases and Additional Coverage
// =============================================================================

func TestNormalizeOpenAI_RequestIDGenerated(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.ID == "" {
		t.Error("expected request ID to be generated")
	}
	if !strings.HasPrefix(nr.ID, "req_") {
		t.Errorf("expected request ID to have 'req_' prefix, got %q", nr.ID)
	}
}

func TestNormalizeOpenAI_OriginalRequestPreserved(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.OriginalRequest == nil {
		t.Fatal("expected original request to be preserved")
	}

	var original OpenAIRequest
	if err := json.Unmarshal(nr.OriginalRequest, &original); err != nil {
		t.Fatalf("failed to unmarshal original request: %v", err)
	}
	if original.Model != "gpt-4" {
		t.Errorf("expected original model 'gpt-4', got %q", original.Model)
	}
}

func TestNormalizeOpenAI_InputFormatSet(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.InputFormat != InputFormatOpenAI {
		t.Errorf("expected input format 'openai', got %q", nr.InputFormat)
	}
}

func TestNormalizeGemini_InputFormatSet(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Model: "gemini-1.5-pro",
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.InputFormat != InputFormatGemini {
		t.Errorf("expected input format 'gemini', got %q", nr.InputFormat)
	}
}

func TestNormalizeOpenAI_ProviderHintFromGeminiModel(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gemini-1.5-pro",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.ProviderHint != "gemini" {
		t.Errorf("expected provider hint 'gemini', got %q", nr.ProviderHint)
	}
}

func TestNormalizeOpenAI_StreamPreserved(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model:  "gpt-4",
		Stream: true,
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !nr.Stream {
		t.Error("expected stream to be true")
	}
}

func TestNormalizeOpenAI_StopSequencesPreserved(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Stop:  []string{"END", "STOP"},
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nr.Stop) != 2 {
		t.Fatalf("expected 2 stop sequences, got %d", len(nr.Stop))
	}
}

func TestNormalizeOpenAI_UserMetadata(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		User:  "user-123",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.Metadata["user"] != "user-123" {
		t.Errorf("expected user metadata 'user-123', got %q", nr.Metadata["user"])
	}
}

func TestNormalizeOpenAI_ToolChoice(t *testing.T) {
	tests := []struct {
		name       string
		toolChoice interface{}
		want       string
	}{
		{"string auto", "auto", "auto"},
		{"string none", "none", "none"},
		{"string required", "required", "required"},
		{"object with function name", map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name": "get_weather",
			},
		}, "get_weather"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeToolChoice(tt.toolChoice)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNormalizeOpenAI_ToolsNormalized(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
		Tools: []OpenAIToolInput{
			{
				// Missing type - should default to "function".
				Function: OpenAIFunctionDefinitionInput{
					Name:        " get_weather ",
					Description: "Get weather for a location",
				},
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nr.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(nr.Tools))
	}
	if nr.Tools[0].Type != "function" {
		t.Errorf("expected tool type 'function', got %q", nr.Tools[0].Type)
	}
	if nr.Tools[0].Function.Name != "get_weather" {
		t.Errorf("expected function name 'get_weather' (trimmed), got %q", nr.Tools[0].Function.Name)
	}

	// Verify changes were tracked.
	typeDefault := false
	trimmed := false
	for _, c := range nr.Changes {
		if c.Action == "default_applied" && strings.Contains(c.Field, "tools") {
			typeDefault = true
		}
		if c.Action == "whitespace_trimmed" {
			trimmed = true
		}
	}
	if !typeDefault {
		t.Error("expected tool type default_applied change")
	}
	if !trimmed {
		t.Error("expected whitespace_trimmed change")
	}
}

func TestNormalizeOpenAI_ValidationError(t *testing.T) {
	n := NewNormalizer(nil)

	// Missing model and messages.
	req := &OpenAIRequest{}

	_, err := n.NormalizeOpenAI(req)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected 'validation failed' in error, got %q", err.Error())
	}
}

func TestNormalizeGemini_ValidationError(t *testing.T) {
	n := NewNormalizer(nil)

	// Missing contents.
	req := &GeminiRequest{}

	_, err := n.NormalizeGemini(req)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("expected 'validation failed' in error, got %q", err.Error())
	}
}

func TestNormalizeOpenAI_InvalidContent(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`{invalid json`),
			},
		},
	}

	_, err := n.NormalizeOpenAI(req)
	if err == nil {
		t.Fatal("expected error for invalid content JSON")
	}
}

func TestNormalizeGemini_DefaultProviderHint(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.ProviderHint != "gemini" {
		t.Errorf("expected provider hint 'gemini', got %q", nr.ProviderHint)
	}
}

func TestNormalizeOpenAI_MultipleSystemMessages(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "system",
				Content: json.RawMessage(`"Part 1."`),
			},
			{
				Role:    "system",
				Content: json.RawMessage(`"Part 2."`),
			},
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.SystemPrompt != "Part 1.\nPart 2." {
		t.Errorf("expected combined system prompt 'Part 1.\\nPart 2.', got %q", nr.SystemPrompt)
	}
	if len(nr.Messages) != 1 {
		t.Fatalf("expected 1 message after system extraction, got %d", len(nr.Messages))
	}
}

func TestNormalizerConfig_Default(t *testing.T) {
	config := DefaultNormalizerConfig()

	if !config.ApplyDefaultTemperature {
		t.Error("expected ApplyDefaultTemperature to be true")
	}
	if !config.ApplyDefaultTopP {
		t.Error("expected ApplyDefaultTopP to be true")
	}
	if config.DefaultTemperature != DefaultTemperature {
		t.Errorf("expected default temperature %v, got %v", DefaultTemperature, config.DefaultTemperature)
	}
	if config.DefaultTopP != DefaultTopP {
		t.Errorf("expected default top_p %v, got %v", DefaultTopP, config.DefaultTopP)
	}
	if len(config.ModelAliases) == 0 {
		t.Error("expected model aliases to be populated")
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:   "temperature",
		Message: "must be between 0 and 2",
	}
	expected := "temperature: must be between 0 and 2"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestNormalizeGemini_NoModel(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Model should be empty but provider hint should be gemini.
	if nr.Model != "" {
		t.Errorf("expected empty model, got %q", nr.Model)
	}
	if nr.ProviderHint != "gemini" {
		t.Errorf("expected provider hint 'gemini', got %q", nr.ProviderHint)
	}
}

func TestNormalizeOpenAI_NullContent(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role: "assistant",
				ToolCalls: []OpenAIToolCallInput{
					{
						ID:   "call_1",
						Type: "function",
						Function: OpenAIFunctionCallInput{
							Name:      "test",
							Arguments: "{}",
						},
					},
				},
				// Content is nil - valid for assistant with tool calls.
			},
			{
				Role:       "tool",
				Content:    json.RawMessage(`"result"`),
				ToolCallID: "call_1",
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(nr.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(nr.Messages))
	}
}

func TestNormalizeGemini_NoGenerationConfig(t *testing.T) {
	n := NewNormalizer(nil)
	req := &GeminiRequest{
		Model: "gemini-1.5-pro",
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
	}

	nr, err := n.NormalizeGemini(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Defaults should still be applied.
	if nr.Temperature == nil {
		t.Fatal("expected default temperature to be applied")
	}
	if *nr.Temperature != DefaultTemperature {
		t.Errorf("expected default temperature %v, got %v", DefaultTemperature, *nr.Temperature)
	}
}

func TestCreatedAtTimestamp(t *testing.T) {
	n := NewNormalizer(nil)
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Role:    "user",
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	nr, err := n.NormalizeOpenAI(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nr.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestValidateNormalizedRequest_Valid(t *testing.T) {
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{
				Role:  "user",
				Parts: []ContentPart{{Type: "text", Text: "Hello"}},
			},
		},
		Tools: []Tool{
			{
				Type: "function",
				Function: FunctionDefinition{
					Name:        "test_func",
					Description: "A test function",
				},
			},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if !vr.Valid {
		t.Errorf("expected validation to pass, errors: %v", vr.Errors)
	}
}

func TestValidateNormalizedRequest_UnsupportedToolType(t *testing.T) {
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Tools: []Tool{
			{Type: "retrieval", Function: FunctionDefinition{Name: "test"}},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for unsupported tool type")
	}
	found := false
	for _, e := range vr.Errors {
		if strings.Contains(e.Message, "unsupported tool type") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected unsupported tool type error message")
	}
}

func TestValidateNormalizedRequest_TemperatureOutOfRange(t *testing.T) {
	temp := 2.5
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &temp,
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for temperature > 2")
	}
}

func TestValidateNormalizedRequest_TopPOutOfRange(t *testing.T) {
	topP := 1.5
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		TopP: &topP,
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for top_p > 1")
	}
}

func TestValidateNormalizedRequest_MaxTokensTooLow(t *testing.T) {
	maxTokens := 0
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: &maxTokens,
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for max_tokens < 1")
	}
}

func TestValidateGeminiRequest_EmptyParts(t *testing.T) {
	req := &GeminiRequest{
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{},
			},
		},
	}

	vr := ValidateGeminiRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for empty parts")
	}
}

func TestValidateGeminiRequest_MissingRole(t *testing.T) {
	req := &GeminiRequest{
		Contents: []GeminiContentInput{
			{
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
	}

	vr := ValidateGeminiRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for missing role")
	}
}

func TestValidateGeminiRequest_TopPOutOfRange(t *testing.T) {
	topP := -0.5
	req := &GeminiRequest{
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
		GenerationConfig: &GeminiGenerationConfig{
			TopP: &topP,
		},
	}

	vr := ValidateGeminiRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for top_p < 0")
	}
}

func TestValidateGeminiRequest_MaxOutputTokensTooLow(t *testing.T) {
	maxTokens := 0
	req := &GeminiRequest{
		Contents: []GeminiContentInput{
			{
				Role:  "user",
				Parts: []GeminiPartInput{{Text: "Hello"}},
			},
		},
		GenerationConfig: &GeminiGenerationConfig{
			MaxOutputTokens: &maxTokens,
		},
	}

	vr := ValidateGeminiRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for max_output_tokens < 1")
	}
}

func TestValidateOpenAIRequest_MissingRole(t *testing.T) {
	req := &OpenAIRequest{
		Model: "gpt-4",
		Messages: []OpenAIMessageInput{
			{
				Content: json.RawMessage(`"Hello"`),
			},
		},
	}

	vr := ValidateOpenAIRequest(req)
	if vr.Valid {
		t.Error("expected validation to fail for missing role")
	}
}

func TestValidateNormalizedRequest_ImageWithoutURL(t *testing.T) {
	nr := &NormalizedRequest{
		Model: "gpt-4",
		Messages: []Message{
			{
				Role: "user",
				Parts: []ContentPart{
					{Type: "image_url"},
				},
			},
		},
	}

	vr := ValidateNormalizedRequest(nr)
	if vr.Valid {
		t.Error("expected validation to fail for image_url without URL")
	}
}
