package rules

import (
	"strings"
	"testing"

	"github.com/llm-optimization-gateway/internal/request"
)

func TestEstimateTextTokens_Empty(t *testing.T) {
	got := EstimateTextTokens("")
	if got != 0 {
		t.Errorf("EstimateTextTokens(\"\") = %d, want 0", got)
	}
}

func TestEstimateTextTokens_Short(t *testing.T) {
	// "Hi" = 2 chars → ceil(2/4) = 1 token
	got := EstimateTextTokens("Hi")
	if got != 1 {
		t.Errorf("EstimateTextTokens(\"Hi\") = %d, want 1", got)
	}
}

func TestEstimateTextTokens_ExactMultiple(t *testing.T) {
	// 8 chars → 8/4 = 2 tokens exactly
	got := EstimateTextTokens("abcdefgh")
	if got != 2 {
		t.Errorf("EstimateTextTokens(\"abcdefgh\") = %d, want 2", got)
	}
}

func TestEstimateTextTokens_RoundsUp(t *testing.T) {
	// 9 chars → ceil(9/4) = 3 tokens
	got := EstimateTextTokens("abcdefghi")
	if got != 3 {
		t.Errorf("EstimateTextTokens(\"abcdefghi\") = %d, want 3", got)
	}
}

func TestEstimateTextTokens_LargeText(t *testing.T) {
	text := strings.Repeat("a", 4000)
	got := EstimateTextTokens(text)
	if got != 1000 {
		t.Errorf("EstimateTextTokens(4000 chars) = %d, want 1000", got)
	}
}

func TestEstimateTokens_NilRequest(t *testing.T) {
	est := EstimateTokens(nil)
	if est.TotalTokens != BaseRequestOverheadTokens {
		t.Errorf("EstimateTokens(nil).TotalTokens = %d, want %d", est.TotalTokens, BaseRequestOverheadTokens)
	}
}

func TestEstimateTokens_EmptyRequest(t *testing.T) {
	req := &request.NormalizedRequest{}
	est := EstimateTokens(req)
	if est.TotalTokens != BaseRequestOverheadTokens {
		t.Errorf("EstimateTokens(empty).TotalTokens = %d, want %d", est.TotalTokens, BaseRequestOverheadTokens)
	}
	if est.MessageTokens != 0 {
		t.Errorf("MessageTokens = %d, want 0", est.MessageTokens)
	}
}

func TestEstimateTokens_SingleMessage(t *testing.T) {
	req := &request.NormalizedRequest{
		Messages: []request.Message{
			{Role: "user", Content: "Hello, world!"},
		},
	}
	est := EstimateTokens(req)

	// "Hello, world!" = 13 chars → ceil(13/4) = 4 tokens
	expectedMsgTokens := 4
	if est.MessageTokens != expectedMsgTokens {
		t.Errorf("MessageTokens = %d, want %d", est.MessageTokens, expectedMsgTokens)
	}
	if est.MessageOverhead != MessageOverheadTokens {
		t.Errorf("MessageOverhead = %d, want %d", est.MessageOverhead, MessageOverheadTokens)
	}
	expectedTotal := expectedMsgTokens + MessageOverheadTokens + BaseRequestOverheadTokens
	if est.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens = %d, want %d", est.TotalTokens, expectedTotal)
	}
}

func TestEstimateTokens_MultipleMessages(t *testing.T) {
	req := &request.NormalizedRequest{
		Messages: []request.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hi"},
			{Role: "assistant", Content: "Hello! How can I help you?"},
		},
	}
	est := EstimateTokens(req)

	if est.MessageOverhead != 3*MessageOverheadTokens {
		t.Errorf("MessageOverhead = %d, want %d", est.MessageOverhead, 3*MessageOverheadTokens)
	}
	if est.TotalTokens <= BaseRequestOverheadTokens {
		t.Error("TotalTokens should be greater than base overhead for messages")
	}
}

func TestEstimateTokens_MultiPartMessage(t *testing.T) {
	req := &request.NormalizedRequest{
		Messages: []request.Message{
			{
				Role: "user",
				Parts: []request.ContentPart{
					{Type: "text", Text: "Describe this image:"},
					{Type: "image_url", ImageURL: "https://example.com/image.png"},
				},
			},
		},
	}
	est := EstimateTokens(req)

	if est.MessageTokens <= 0 {
		t.Error("MessageTokens should be > 0 for multi-part message")
	}
}

func TestEstimateTokens_ToolCalls(t *testing.T) {
	req := &request.NormalizedRequest{
		Messages: []request.Message{
			{
				Role: "assistant",
				ToolCalls: []request.ToolCall{
					{
						ID:   "call_123",
						Type: "function",
						Function: request.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location": "San Francisco"}`,
						},
					},
				},
			},
		},
	}
	est := EstimateTokens(req)

	if est.MessageTokens <= 0 {
		t.Error("MessageTokens should be > 0 for tool call content")
	}
}

func TestEstimateTokens_ToolResponseMessage(t *testing.T) {
	req := &request.NormalizedRequest{
		Messages: []request.Message{
			{
				Role:       "tool",
				Content:    `{"temp": 72}`,
				ToolCallID: "call_123",
			},
		},
	}
	est := EstimateTokens(req)

	if est.MessageTokens <= 0 {
		t.Error("MessageTokens should be > 0 for tool response with call ID")
	}
}

func TestEstimateTokens_WithTools(t *testing.T) {
	req := &request.NormalizedRequest{
		Tools: []request.Tool{
			{
				Type: "function",
				Function: request.FunctionDefinition{
					Name:        "get_weather",
					Description: "Get the current weather for a location",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "The city and state",
							},
						},
						"required": []interface{}{"location"},
					},
				},
			},
		},
	}
	est := EstimateTokens(req)

	if est.ToolTokens <= 0 {
		t.Error("ToolTokens should be > 0")
	}
	if est.ToolOverhead != ToolDefinitionOverheadTokens {
		t.Errorf("ToolOverhead = %d, want %d", est.ToolOverhead, ToolDefinitionOverheadTokens)
	}
}

func TestEstimateTokens_WithSystemPrompt(t *testing.T) {
	req := &request.NormalizedRequest{
		SystemPrompt: "You are a helpful coding assistant.",
	}
	est := EstimateTokens(req)

	if est.SystemPromptTokens <= 0 {
		t.Error("SystemPromptTokens should be > 0")
	}
	if est.SystemPromptOverhead != SystemPromptOverheadTokens {
		t.Errorf("SystemPromptOverhead = %d, want %d", est.SystemPromptOverhead, SystemPromptOverheadTokens)
	}
}

func TestEstimateTokens_NoSystemPrompt(t *testing.T) {
	req := &request.NormalizedRequest{
		Messages: []request.Message{
			{Role: "user", Content: "Hi"},
		},
	}
	est := EstimateTokens(req)

	if est.SystemPromptTokens != 0 {
		t.Errorf("SystemPromptTokens = %d, want 0", est.SystemPromptTokens)
	}
	if est.SystemPromptOverhead != 0 {
		t.Errorf("SystemPromptOverhead = %d, want 0", est.SystemPromptOverhead)
	}
}

func TestEstimateTokens_FullRequest(t *testing.T) {
	req := &request.NormalizedRequest{
		SystemPrompt: "You are a coding assistant.",
		Messages: []request.Message{
			{Role: "user", Content: "Write a hello world program"},
			{Role: "assistant", Content: "Here is a hello world program in Python:\n```python\nprint('Hello, world!')\n```"},
			{Role: "user", Content: "Now in Go"},
		},
		Tools: []request.Tool{
			{
				Type: "function",
				Function: request.FunctionDefinition{
					Name:        "run_code",
					Description: "Execute code in a sandbox",
				},
			},
		},
	}
	est := EstimateTokens(req)

	// Verify all components contribute.
	if est.MessageTokens <= 0 {
		t.Error("MessageTokens should be > 0")
	}
	if est.MessageOverhead != 3*MessageOverheadTokens {
		t.Errorf("MessageOverhead = %d, want %d", est.MessageOverhead, 3*MessageOverheadTokens)
	}
	if est.ToolTokens <= 0 {
		t.Error("ToolTokens should be > 0")
	}
	if est.SystemPromptTokens <= 0 {
		t.Error("SystemPromptTokens should be > 0")
	}
	if est.BaseOverhead != BaseRequestOverheadTokens {
		t.Errorf("BaseOverhead = %d, want %d", est.BaseOverhead, BaseRequestOverheadTokens)
	}

	// TotalTokens should equal the sum of all components.
	expectedTotal := est.MessageTokens + est.MessageOverhead +
		est.ToolTokens + est.ToolOverhead +
		est.SystemPromptTokens + est.SystemPromptOverhead +
		est.BaseOverhead
	if est.TotalTokens != expectedTotal {
		t.Errorf("TotalTokens = %d, want %d (sum of components)", est.TotalTokens, expectedTotal)
	}
}

func TestEstimateTokens_MultipleTools(t *testing.T) {
	req := &request.NormalizedRequest{
		Tools: []request.Tool{
			{Type: "function", Function: request.FunctionDefinition{Name: "tool_a"}},
			{Type: "function", Function: request.FunctionDefinition{Name: "tool_b"}},
			{Type: "function", Function: request.FunctionDefinition{Name: "tool_c"}},
		},
	}
	est := EstimateTokens(req)

	if est.ToolOverhead != 3*ToolDefinitionOverheadTokens {
		t.Errorf("ToolOverhead = %d, want %d", est.ToolOverhead, 3*ToolDefinitionOverheadTokens)
	}
}

func BenchmarkEstimateTokens_SmallRequest(b *testing.B) {
	req := &request.NormalizedRequest{
		Messages: []request.Message{
			{Role: "user", Content: "Hello"},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateTokens(req)
	}
}

func BenchmarkEstimateTokens_LargeRequest(b *testing.B) {
	msgs := make([]request.Message, 50)
	for i := range msgs {
		msgs[i] = request.Message{
			Role:    "user",
			Content: strings.Repeat("This is a test message with some content. ", 20),
		}
	}
	tools := make([]request.Tool, 10)
	for i := range tools {
		tools[i] = request.Tool{
			Type: "function",
			Function: request.FunctionDefinition{
				Name:        "tool_function",
				Description: "A tool that does something useful",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"param1": map[string]interface{}{"type": "string"},
						"param2": map[string]interface{}{"type": "integer"},
					},
				},
			},
		}
	}

	req := &request.NormalizedRequest{
		SystemPrompt: strings.Repeat("You are a helpful assistant. ", 10),
		Messages:     msgs,
		Tools:        tools,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateTokens(req)
	}
}

func BenchmarkEstimateTextTokens(b *testing.B) {
	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateTextTokens(text)
	}
}
