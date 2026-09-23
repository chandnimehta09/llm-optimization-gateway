package mock

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

// =============================================================================
// Echo Mode Tests
// =============================================================================

func TestEchoMode_SimpleMessage(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
	})

	req := &provider.CompletionRequest{
		Model: "test-model",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello" {
		t.Errorf("expected echo of %q, got %q", "Hello", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].Message.Role != provider.RoleAssistant {
		t.Errorf("expected role %q, got %q", provider.RoleAssistant, resp.Choices[0].Message.Role)
	}
	if resp.Choices[0].FinishReason != provider.FinishReasonStop {
		t.Errorf("expected finish reason %q, got %q", provider.FinishReasonStop, resp.Choices[0].FinishReason)
	}
}

func TestEchoMode_MultipleMessages(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are a helpful assistant"},
			{Role: provider.RoleUser, Content: "First message"},
			{Role: provider.RoleAssistant, Content: "First reply"},
			{Role: provider.RoleUser, Content: "Second message"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should echo the last user message.
	if resp.Choices[0].Message.Content != "Second message" {
		t.Errorf("expected echo of last user message %q, got %q", "Second message", resp.Choices[0].Message.Content)
	}
}

func TestEchoMode_NoUserMessage(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are a helpful assistant"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Choices[0].Message.Content != "no user message found" {
		t.Errorf("expected fallback message, got %q", resp.Choices[0].Message.Content)
	}
}

func TestEchoMode_DefaultMode(t *testing.T) {
	// Empty ResponseMode should default to echo.
	p := New(MockProviderConfig{})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "default echo"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Choices[0].Message.Content != "default echo" {
		t.Errorf("expected echo of %q, got %q", "default echo", resp.Choices[0].Message.Content)
	}
}

// =============================================================================
// Fixed Response Mode Tests
// =============================================================================

func TestFixedResponseMode(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:  ResponseModeFixed,
		FixedResponse: "I am a fixed response",
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Anything"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Choices[0].Message.Content != "I am a fixed response" {
		t.Errorf("expected fixed response %q, got %q", "I am a fixed response", resp.Choices[0].Message.Content)
	}
}

func TestFixedResponseMode_IgnoresInput(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:  ResponseModeFixed,
		FixedResponse: "always the same",
	})

	messages := []string{"Hello", "World", "Different input"}
	for _, msg := range messages {
		req := &provider.CompletionRequest{
			Messages: []provider.Message{
				{Role: provider.RoleUser, Content: msg},
			},
		}
		resp, err := p.Complete(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", msg, err)
		}
		if resp.Choices[0].Message.Content != "always the same" {
			t.Errorf("expected fixed response for input %q, got %q", msg, resp.Choices[0].Message.Content)
		}
	}
}

func TestFixedResponseMode_EmptyResponse(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:  ResponseModeFixed,
		FixedResponse: "",
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Choices[0].Message.Content != "" {
		t.Errorf("expected empty fixed response, got %q", resp.Choices[0].Message.Content)
	}
}

// =============================================================================
// Token Count Mode Tests
// =============================================================================

func TestTokenCountMode(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:     ResponseModeTokenCount,
		TargetTokenCount: 20,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Generate tokens"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	words := strings.Fields(resp.Choices[0].Message.Content)
	if len(words) != 20 {
		t.Errorf("expected 20 words (tokens), got %d", len(words))
	}
}

func TestTokenCountMode_ZeroTokens(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:     ResponseModeTokenCount,
		TargetTokenCount: 0,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Generate tokens"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should default to 10 tokens.
	words := strings.Fields(resp.Choices[0].Message.Content)
	if len(words) != 10 {
		t.Errorf("expected 10 default words (tokens), got %d", len(words))
	}
}

// =============================================================================
// Simulated Latency Tests
// =============================================================================

func TestSimulatedLatency(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
		Latency:      100 * time.Millisecond,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	start := time.Now()
	resp, err := p.Complete(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if elapsed < 100*time.Millisecond {
		t.Errorf("expected at least 100ms latency, got %s", elapsed)
	}

	if resp.Choices[0].Message.Content != "Hello" {
		t.Errorf("expected echo despite latency, got %q", resp.Choices[0].Message.Content)
	}
}

func TestSimulatedLatency_ContextCancellation(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
		Latency:      5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	_, err := p.Complete(ctx, req)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestSimulatedLatency_ZeroLatency(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
		Latency:      0,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	start := time.Now()
	_, err := p.Complete(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if elapsed > 50*time.Millisecond {
		t.Errorf("zero latency should be fast, took %s", elapsed)
	}
}

// =============================================================================
// Simulated Error Tests
// =============================================================================

func TestSimulatedErrors_AlwaysError(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
		ErrorRate:    1.0,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	_, err := p.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error with error rate 1.0")
	}

	if !provider.IsProviderError(err) {
		t.Errorf("expected ProviderError, got %T", err)
	}
}

func TestSimulatedErrors_NeverError(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
		ErrorRate:    0.0,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	// Run multiple times to verify no errors.
	for i := 0; i < 10; i++ {
		_, err := p.Complete(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
	}
}

func TestSimulatedErrors_StreamAlwaysError(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
		ErrorRate:    1.0,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	_, err := p.CompleteStream(context.Background(), req)
	if err == nil {
		t.Fatal("expected error with error rate 1.0 in streaming")
	}

	if !provider.IsProviderError(err) {
		t.Errorf("expected ProviderError, got %T", err)
	}
}

// =============================================================================
// Streaming Tests
// =============================================================================

func TestStreaming_BasicFlow(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:     ResponseModeFixed,
		FixedResponse:    "Hello World",
		StreamChunkSize:  5,
		StreamChunkDelay: time.Millisecond,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Stream please"},
		},
	}

	ch, err := p.CompleteStream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	var chunks []provider.StreamChunk
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Error)
		}
		chunks = append(chunks, chunk)
		content += chunk.Delta.Content
	}

	if content != "Hello World" {
		t.Errorf("expected streamed content %q, got %q", "Hello World", content)
	}

	// Should have content chunks + 1 final chunk.
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks, got %d", len(chunks))
	}

	// First chunk should have role set.
	if chunks[0].Delta.Role != provider.RoleAssistant {
		t.Errorf("expected first chunk role %q, got %q", provider.RoleAssistant, chunks[0].Delta.Role)
	}

	// Final chunk should have finish reason.
	finalChunk := chunks[len(chunks)-1]
	if finalChunk.FinishReason != provider.FinishReasonStop {
		t.Errorf("expected final chunk finish reason %q, got %q", provider.FinishReasonStop, finalChunk.FinishReason)
	}

	// Final chunk should have usage.
	if finalChunk.Usage == nil {
		t.Fatal("expected usage in final chunk")
	}
	if finalChunk.Usage.TotalTokens <= 0 {
		t.Errorf("expected positive total tokens, got %d", finalChunk.Usage.TotalTokens)
	}
}

func TestStreaming_ChunkSizes(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:     ResponseModeFixed,
		FixedResponse:    "ABCDEFGHIJ",
		StreamChunkSize:  3,
		StreamChunkDelay: time.Millisecond,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	}

	ch, err := p.CompleteStream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var contentChunks []string
	for chunk := range ch {
		if chunk.Delta.Content != "" {
			contentChunks = append(contentChunks, chunk.Delta.Content)
		}
	}

	// "ABCDEFGHIJ" with chunk size 3 => "ABC", "DEF", "GHI", "J"
	expected := []string{"ABC", "DEF", "GHI", "J"}
	if len(contentChunks) != len(expected) {
		t.Fatalf("expected %d content chunks, got %d: %v", len(expected), len(contentChunks), contentChunks)
	}
	for i, exp := range expected {
		if contentChunks[i] != exp {
			t.Errorf("chunk %d: expected %q, got %q", i, exp, contentChunks[i])
		}
	}
}

func TestStreaming_EchoMode(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:     ResponseModeEcho,
		StreamChunkSize:  5,
		StreamChunkDelay: time.Millisecond,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Echo this back"},
		},
	}

	ch, err := p.CompleteStream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	for chunk := range ch {
		content += chunk.Delta.Content
	}

	if content != "Echo this back" {
		t.Errorf("expected streamed echo %q, got %q", "Echo this back", content)
	}
}

func TestStreaming_ContextCancellation(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:     ResponseModeFixed,
		FixedResponse:    strings.Repeat("a", 1000),
		StreamChunkSize:  1,
		StreamChunkDelay: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	}

	ch, err := p.CompleteStream(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var gotError bool
	chunkCount := 0
	for chunk := range ch {
		chunkCount++
		if chunk.Error != nil {
			gotError = true
		}
	}

	// Should have received some chunks before cancellation.
	if chunkCount == 0 {
		t.Error("expected at least some chunks before cancellation")
	}

	// Should eventually get a context error.
	if !gotError {
		t.Error("expected context cancellation error in stream")
	}
}

func TestStreaming_ConsistentIDs(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:     ResponseModeFixed,
		FixedResponse:    "Test content",
		StreamChunkSize:  3,
		StreamChunkDelay: time.Millisecond,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	}

	ch, err := p.CompleteStream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var firstID string
	for chunk := range ch {
		if firstID == "" {
			firstID = chunk.ID
		} else if chunk.ID != firstID {
			t.Errorf("expected consistent ID %q, got %q", firstID, chunk.ID)
		}
	}
}

// =============================================================================
// Response Metadata Tests
// =============================================================================

func TestResponseMetadata(t *testing.T) {
	p := New(MockProviderConfig{
		Name:         "test-mock",
		Model:        "test-model-v1",
		ResponseMode: ResponseModeEcho,
	})

	req := &provider.CompletionRequest{
		Model: "custom-model",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Provider != "test-mock" {
		t.Errorf("expected provider %q, got %q", "test-mock", resp.Provider)
	}
	if resp.Model != "custom-model" {
		t.Errorf("expected model %q, got %q", "custom-model", resp.Model)
	}
	if resp.ID == "" {
		t.Error("expected non-empty response ID")
	}
	if resp.Created.IsZero() {
		t.Error("expected non-zero created time")
	}
	if resp.FinishReason != provider.FinishReasonStop {
		t.Errorf("expected finish reason %q, got %q", provider.FinishReasonStop, resp.FinishReason)
	}
}

func TestResponseMetadata_DefaultModel(t *testing.T) {
	p := New(MockProviderConfig{
		Model:        "configured-model",
		ResponseMode: ResponseModeEcho,
	})

	req := &provider.CompletionRequest{
		// Model is empty, should fall back to config.
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Hello"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Model != "configured-model" {
		t.Errorf("expected default model %q, got %q", "configured-model", resp.Model)
	}
}

func TestUsageStats(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:  ResponseModeFixed,
		FixedResponse: "one two three four five",
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hello world"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "hello world" = 2 prompt tokens.
	if resp.Usage.PromptTokens != 2 {
		t.Errorf("expected 2 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
	// "one two three four five" = 5 completion tokens.
	if resp.Usage.CompletionTokens != 5 {
		t.Errorf("expected 5 completion tokens, got %d", resp.Usage.CompletionTokens)
	}
	// Total = prompt + completion.
	if resp.Usage.TotalTokens != resp.Usage.PromptTokens+resp.Usage.CompletionTokens {
		t.Errorf("expected total tokens %d, got %d",
			resp.Usage.PromptTokens+resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
	}
}

// =============================================================================
// Call Tracking Tests
// =============================================================================

func TestCallTracking(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
	})

	if p.CallCount() != 0 {
		t.Errorf("expected initial call count 0, got %d", p.CallCount())
	}

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "first"},
		},
	}

	_, _ = p.Complete(context.Background(), req)
	if p.CallCount() != 1 {
		t.Errorf("expected call count 1 after Complete, got %d", p.CallCount())
	}

	req2 := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "second"},
		},
	}
	_, _ = p.Complete(context.Background(), req2)
	if p.CallCount() != 2 {
		t.Errorf("expected call count 2, got %d", p.CallCount())
	}

	// Verify last request.
	last := p.LastRequest()
	if last == nil {
		t.Fatal("expected non-nil last request")
	}
	if last.Messages[0].Content != "second" {
		t.Errorf("expected last request content %q, got %q", "second", last.Messages[0].Content)
	}
}

func TestCallTracking_StreamCounts(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode:     ResponseModeFixed,
		FixedResponse:    "Hello",
		StreamChunkDelay: time.Millisecond,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	}

	ch, _ := p.CompleteStream(context.Background(), req)
	// Drain the channel.
	for range ch {
	}

	if p.CallCount() != 1 {
		t.Errorf("expected call count 1 after CompleteStream, got %d", p.CallCount())
	}
}

func TestCallTracking_Reset(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	}

	_, _ = p.Complete(context.Background(), req)
	if p.CallCount() != 1 {
		t.Fatalf("expected call count 1, got %d", p.CallCount())
	}

	p.Reset()

	if p.CallCount() != 0 {
		t.Errorf("expected call count 0 after reset, got %d", p.CallCount())
	}
	if p.LastRequest() != nil {
		t.Error("expected nil last request after reset")
	}
}

// =============================================================================
// Provider Interface Compliance Tests
// =============================================================================

func TestProviderInterface_Compliance(t *testing.T) {
	// Verify that *Provider satisfies provider.Provider.
	var _ provider.Provider = (*Provider)(nil)

	p := New(MockProviderConfig{
		Name:  "interface-test",
		Model: "test-model",
	})

	// Test Name().
	if p.Name() != "interface-test" {
		t.Errorf("expected name %q, got %q", "interface-test", p.Name())
	}

	// Test APIFormat().
	if p.APIFormat() != provider.APIFormatOpenAI {
		t.Errorf("expected API format %q, got %q", provider.APIFormatOpenAI, p.APIFormat())
	}

	// Test Models().
	models := p.Models()
	if len(models) != 1 || models[0] != "test-model" {
		t.Errorf("expected models [test-model], got %v", models)
	}

	// Test IsHealthy().
	if !p.IsHealthy(context.Background()) {
		t.Error("expected mock provider to be healthy")
	}
}

func TestProviderInterface_UsedAsInterface(t *testing.T) {
	// Create a mock and use it through the provider.Provider interface.
	var p provider.Provider = New(MockProviderConfig{
		ResponseMode:  ResponseModeFixed,
		FixedResponse: "interface response",
	})

	resp, err := p.Complete(context.Background(), &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Choices[0].Message.Content != "interface response" {
		t.Errorf("expected %q, got %q", "interface response", resp.Choices[0].Message.Content)
	}
}

// =============================================================================
// Message Role Tests
// =============================================================================

func TestAllMessageRoles(t *testing.T) {
	p := New(MockProviderConfig{
		ResponseMode: ResponseModeEcho,
	})

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are a helpful assistant"},
			{Role: provider.RoleUser, Content: "What's the weather?"},
			{
				Role: provider.RoleAssistant,
				ToolCalls: []provider.ToolCall{
					{
						ID:   "call_1",
						Type: "function",
						Function: provider.FunctionCall{
							Name:      "get_weather",
							Arguments: `{"location":"NYC"}`,
						},
					},
				},
			},
			{Role: provider.RoleTool, Content: `{"temp": 72}`, ToolCallID: "call_1"},
			{Role: provider.RoleUser, Content: "Thanks!"},
		},
	}

	resp, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should echo the last user message.
	if resp.Choices[0].Message.Content != "Thanks!" {
		t.Errorf("expected echo of %q, got %q", "Thanks!", resp.Choices[0].Message.Content)
	}

	// Verify all messages were counted for token usage.
	if resp.Usage.PromptTokens <= 0 {
		t.Error("expected positive prompt tokens")
	}
}

// =============================================================================
// Configuration Tests
// =============================================================================

func TestConfig_Defaults(t *testing.T) {
	p := New(MockProviderConfig{})

	if p.Name() != "mock" {
		t.Errorf("expected default name %q, got %q", "mock", p.Name())
	}
	if p.cfg.Model != "mock-model" {
		t.Errorf("expected default model %q, got %q", "mock-model", p.cfg.Model)
	}
	if p.cfg.ResponseMode != ResponseModeEcho {
		t.Errorf("expected default mode %q, got %q", ResponseModeEcho, p.cfg.ResponseMode)
	}
	if p.cfg.StreamChunkSize != 5 {
		t.Errorf("expected default chunk size 5, got %d", p.cfg.StreamChunkSize)
	}
	if p.cfg.StreamChunkDelay != 10*time.Millisecond {
		t.Errorf("expected default chunk delay 10ms, got %s", p.cfg.StreamChunkDelay)
	}
}

func TestConfig_SetMethods(t *testing.T) {
	p := New(MockProviderConfig{})

	p.SetErrorRate(0.5)
	if p.cfg.ErrorRate != 0.5 {
		t.Errorf("expected error rate 0.5, got %f", p.cfg.ErrorRate)
	}

	p.SetLatency(200 * time.Millisecond)
	if p.cfg.Latency != 200*time.Millisecond {
		t.Errorf("expected latency 200ms, got %s", p.cfg.Latency)
	}

	p.SetResponseMode(ResponseModeFixed)
	if p.cfg.ResponseMode != ResponseModeFixed {
		t.Errorf("expected mode %q, got %q", ResponseModeFixed, p.cfg.ResponseMode)
	}

	p.SetFixedResponse("new response")
	if p.cfg.FixedResponse != "new response" {
		t.Errorf("expected fixed response %q, got %q", "new response", p.cfg.FixedResponse)
	}
}

func TestConfig_Returns(t *testing.T) {
	cfg := MockProviderConfig{
		Name:         "my-mock",
		Model:        "my-model",
		ResponseMode: ResponseModeFixed,
	}
	p := New(cfg)

	got := p.Config()
	if got.Name != "my-mock" {
		t.Errorf("expected config name %q, got %q", "my-mock", got.Name)
	}
}

func TestNewWithSeed(t *testing.T) {
	p := NewWithSeed(MockProviderConfig{
		ErrorRate: 0.5,
	}, 42)

	if p.rng == nil {
		t.Fatal("expected rng to be initialized")
	}

	// With the same seed, should get deterministic behavior.
	p2 := NewWithSeed(MockProviderConfig{
		ErrorRate: 0.5,
	}, 42)

	req := &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "test"},
		},
	}

	// Both should produce the same error/success sequence.
	var results1, results2 []bool
	for i := 0; i < 10; i++ {
		_, err1 := p.Complete(context.Background(), req)
		_, err2 := p2.Complete(context.Background(), req)
		results1 = append(results1, err1 != nil)
		results2 = append(results2, err2 != nil)
	}

	for i := range results1 {
		if results1[i] != results2[i] {
			t.Errorf("iteration %d: seed-based results diverged (p1=%v, p2=%v)", i, results1[i], results2[i])
		}
	}
}

func TestString(t *testing.T) {
	p := New(MockProviderConfig{
		Name:         "test",
		Model:        "test-model",
		ResponseMode: ResponseModeFixed,
	})

	s := p.String()
	if !strings.Contains(s, "test") {
		t.Errorf("String() should contain provider name, got %q", s)
	}
	if !strings.Contains(s, "test-model") {
		t.Errorf("String() should contain model name, got %q", s)
	}
	if !strings.Contains(s, string(ResponseModeFixed)) {
		t.Errorf("String() should contain response mode, got %q", s)
	}
}

// =============================================================================
// Integration Test: Full Workflow
// =============================================================================

func TestIntegration_FullWorkflow(t *testing.T) {
	// Create a mock provider and exercise it through a realistic workflow.
	p := New(MockProviderConfig{
		Name:         "integration-mock",
		Model:        "gpt-4-mock",
		ResponseMode: ResponseModeEcho,
	})

	// Step 1: Send a simple message.
	resp, err := p.Complete(context.Background(), &provider.CompletionRequest{
		Model: "gpt-4-mock",
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "You are a helpful assistant"},
			{Role: provider.RoleUser, Content: "Hello, world!"},
		},
	})
	if err != nil {
		t.Fatalf("step 1: unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "Hello, world!" {
		t.Errorf("step 1: expected echo, got %q", resp.Choices[0].Message.Content)
	}

	// Step 2: Switch to fixed mode and verify.
	p.SetResponseMode(ResponseModeFixed)
	p.SetFixedResponse("Fixed reply")

	resp, err = p.Complete(context.Background(), &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Anything"},
		},
	})
	if err != nil {
		t.Fatalf("step 2: unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "Fixed reply" {
		t.Errorf("step 2: expected fixed reply, got %q", resp.Choices[0].Message.Content)
	}

	// Step 3: Stream a response.
	ch, err := p.CompleteStream(context.Background(), &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "Stream"},
		},
	})
	if err != nil {
		t.Fatalf("step 3: unexpected error: %v", err)
	}

	var streamContent string
	for chunk := range ch {
		streamContent += chunk.Delta.Content
	}
	if streamContent != "Fixed reply" {
		t.Errorf("step 3: expected streamed %q, got %q", "Fixed reply", streamContent)
	}

	// Step 4: Verify call count.
	if p.CallCount() != 3 {
		t.Errorf("step 4: expected call count 3, got %d", p.CallCount())
	}

	// Step 5: Reset and verify.
	p.Reset()
	if p.CallCount() != 0 {
		t.Errorf("step 5: expected call count 0 after reset, got %d", p.CallCount())
	}

	// Step 6: Enable errors and verify.
	p.SetErrorRate(1.0)
	_, err = p.Complete(context.Background(), &provider.CompletionRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "fail"},
		},
	})
	if err == nil {
		t.Fatal("step 6: expected error with error rate 1.0")
	}
}
