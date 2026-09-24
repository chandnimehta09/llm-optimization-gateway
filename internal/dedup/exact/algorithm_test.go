package exact

import (
	"testing"

	"github.com/llm-optimization-gateway/internal/dedup"
	"github.com/llm-optimization-gateway/internal/request"
)

func TestDeduplicateMessages_DuplicateUserMessages(t *testing.T) {
	config := dedup.DefaultConfig()
	messages := []request.Message{
		{Role: "user", Content: "What is Go?"},
		{Role: "user", Content: "What is Go?"},
		{Role: "user", Content: "Tell me about Rust"},
	}

	result, stats := DeduplicateMessages(messages, config)

	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Content != "What is Go?" {
		t.Errorf("expected first message 'What is Go?', got %q", result[0].Content)
	}
	if result[1].Content != "Tell me about Rust" {
		t.Errorf("expected second message 'Tell me about Rust', got %q", result[1].Content)
	}
	if stats.DuplicatesRemoved != 1 {
		t.Errorf("expected 1 duplicate removed, got %d", stats.DuplicatesRemoved)
	}
}

func TestDeduplicateMessages_NoDuplicates(t *testing.T) {
	config := dedup.DefaultConfig()
	messages := []request.Message{
		{Role: "system", Content: "You are a helper."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	result, stats := DeduplicateMessages(messages, config)

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if stats.DuplicatesRemoved != 0 {
		t.Errorf("expected 0 duplicates removed, got %d", stats.DuplicatesRemoved)
	}
}

func TestDeduplicateMessages_EmptyInput(t *testing.T) {
	config := dedup.DefaultConfig()

	result, stats := DeduplicateMessages(nil, config)

	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
	if stats.DuplicatesRemoved != 0 {
		t.Errorf("expected 0 duplicates removed, got %d", stats.DuplicatesRemoved)
	}
}

func TestDeduplicateMessages_PreservesToolCalls(t *testing.T) {
	config := dedup.DefaultConfig()
	messages := []request.Message{
		{Role: "user", Content: "Search for Go docs"},
		{Role: "assistant", Content: "Search for Go docs", ToolCalls: []request.ToolCall{
			{ID: "tc1", Type: "function", Function: request.FunctionCall{Name: "search", Arguments: "{}"}},
		}},
		{Role: "tool", Content: "Results here", ToolCallID: "tc1"},
	}

	result, _ := DeduplicateMessages(messages, config)

	// All messages should be preserved: tool calls and tool responses are always kept.
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
}

func TestDeduplicateMessages_PreservesToolResponses(t *testing.T) {
	config := dedup.DefaultConfig()
	messages := []request.Message{
		{Role: "tool", Content: "same result", ToolCallID: "tc1"},
		{Role: "tool", Content: "same result", ToolCallID: "tc2"},
	}

	result, _ := DeduplicateMessages(messages, config)

	// Tool responses with ToolCallIDs should always be preserved.
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (tool responses always kept), got %d", len(result))
	}
}

func TestDeduplicateMessages_CaseInsensitive(t *testing.T) {
	config := dedup.Config{
		Enabled:       true,
		Strategy:      "exact",
		CaseSensitive: false,
	}
	messages := []request.Message{
		{Role: "user", Content: "Hello World"},
		{Role: "user", Content: "hello world"},
	}

	result, stats := DeduplicateMessages(messages, config)

	if len(result) != 1 {
		t.Fatalf("expected 1 message (case-insensitive), got %d", len(result))
	}
	if result[0].Content != "Hello World" {
		t.Errorf("expected first occurrence preserved, got %q", result[0].Content)
	}
	if stats.DuplicatesRemoved != 1 {
		t.Errorf("expected 1 duplicate removed, got %d", stats.DuplicatesRemoved)
	}
}

func TestDeduplicateMessages_MultiPartContent(t *testing.T) {
	config := dedup.DefaultConfig()
	messages := []request.Message{
		{Role: "user", Parts: []request.ContentPart{
			{Type: "text", Text: "Explain this code"},
		}},
		{Role: "user", Parts: []request.ContentPart{
			{Type: "text", Text: "Explain this code"},
		}},
	}

	result, stats := DeduplicateMessages(messages, config)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if stats.DuplicatesRemoved != 1 {
		t.Errorf("expected 1 duplicate removed, got %d", stats.DuplicatesRemoved)
	}
}

func TestDeduplicateMessages_MixedConversation(t *testing.T) {
	config := dedup.DefaultConfig()
	messages := []request.Message{
		{Role: "system", Content: "You are a coding assistant."},
		{Role: "user", Content: "Explain Go interfaces"},
		{Role: "assistant", Content: "Go interfaces are..."},
		{Role: "user", Content: "Explain Go interfaces"}, // duplicate
		{Role: "user", Content: "Now explain Go channels"},
	}

	result, stats := DeduplicateMessages(messages, config)

	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	if stats.DuplicatesRemoved != 1 {
		t.Errorf("expected 1 duplicate removed, got %d", stats.DuplicatesRemoved)
	}
}
