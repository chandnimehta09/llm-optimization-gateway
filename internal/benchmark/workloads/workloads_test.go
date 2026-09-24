package workloads

import (
	"testing"

	"github.com/llm-optimization-gateway/internal/benchmark"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}

	types := r.Types()
	if len(types) == 0 {
		t.Error("expected registered types")
	}

	expectedTypes := map[string]bool{
		"repetitive":         false,
		"context_change":     false,
		"static_prefix":      false,
		"repository_context": false,
		"random":             false,
	}

	for _, typ := range types {
		if _, ok := expectedTypes[typ]; ok {
			expectedTypes[typ] = true
		}
	}

	for k, found := range expectedTypes {
		if !found {
			t.Errorf("expected type %q to be registered", k)
		}
	}
}

func TestRegistry_Create_Unknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Create(benchmark.WorkloadConfig{Type: "nonexistent"})
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestRegistry_Create_AllTypes(t *testing.T) {
	r := NewRegistry()
	types := []string{"repetitive", "context_change", "static_prefix", "repository_context", "random"}

	for _, typ := range types {
		gen, err := r.Create(benchmark.WorkloadConfig{Type: typ, Weight: 1.0})
		if err != nil {
			t.Errorf("failed to create %q: %v", typ, err)
			continue
		}
		if gen.Name() != typ {
			t.Errorf("expected name %q, got %q", typ, gen.Name())
		}
	}
}

func TestRepetitiveGenerator_SameOutput(t *testing.T) {
	gen := NewRepetitiveGenerator(benchmark.WorkloadConfig{Type: "repetitive"})

	req1 := gen.Generate(0)
	req2 := gen.Generate(1)
	req3 := gen.Generate(100)

	if req1.Messages[0].Content != req2.Messages[0].Content {
		t.Error("repetitive generator should produce identical messages")
	}
	if req2.Messages[0].Content != req3.Messages[0].Content {
		t.Error("repetitive generator should produce identical messages")
	}
	if req1.SystemPrompt != req2.SystemPrompt {
		t.Error("repetitive generator should produce identical system prompts")
	}
}

func TestRepetitiveGenerator_CustomParams(t *testing.T) {
	gen := NewRepetitiveGenerator(benchmark.WorkloadConfig{
		Type: "repetitive",
		Params: map[string]interface{}{
			"system_prompt": "custom system",
			"user_message":  "custom message",
		},
	})

	req := gen.Generate(0)
	if req.SystemPrompt != "custom system" {
		t.Errorf("expected custom system prompt, got %q", req.SystemPrompt)
	}
	if req.Messages[0].Content != "custom message" {
		t.Errorf("expected custom message, got %q", req.Messages[0].Content)
	}
}

func TestContextChangeGenerator_DifferentContext(t *testing.T) {
	gen := NewContextChangeGenerator(benchmark.WorkloadConfig{Type: "context_change"})

	req1 := gen.Generate(0)
	req2 := gen.Generate(1)

	// Context should differ.
	if req1.Messages[0].Content == req2.Messages[0].Content {
		t.Error("context change generator should produce different contexts")
	}

	// Final prompt should be the same.
	last1 := req1.Messages[len(req1.Messages)-1].Content
	last2 := req2.Messages[len(req2.Messages)-1].Content
	if last1 != last2 {
		t.Error("context change generator should produce the same final prompt")
	}
}

func TestStaticPrefixGenerator_SamePrefix(t *testing.T) {
	gen := NewStaticPrefixGenerator(benchmark.WorkloadConfig{Type: "static_prefix"})

	req1 := gen.Generate(0)
	req2 := gen.Generate(1)

	// System prompt (prefix) should be the same.
	if req1.SystemPrompt != req2.SystemPrompt {
		t.Error("static prefix generator should have same system prompt")
	}

	// User message should differ.
	if req1.Messages[0].Content == req2.Messages[0].Content {
		t.Error("static prefix generator should have different questions")
	}
}

func TestStaticPrefixGenerator_LargePrefix(t *testing.T) {
	gen := NewStaticPrefixGenerator(benchmark.WorkloadConfig{
		Type: "static_prefix",
		Params: map[string]interface{}{
			"prefix_tokens": 1000,
		},
	})

	req := gen.Generate(0)
	// 1000 tokens ~ 4000 chars.
	if len(req.SystemPrompt) < 3000 {
		t.Errorf("expected large prefix, got %d chars", len(req.SystemPrompt))
	}
}

func TestRepositoryContextGenerator_HasRepoContent(t *testing.T) {
	gen := NewRepositoryContextGenerator(benchmark.WorkloadConfig{Type: "repository_context"})

	req := gen.Generate(0)
	if len(req.Messages) < 2 {
		t.Error("expected multiple messages with repo context")
	}

	// First message should contain file content.
	firstMsg := req.Messages[0].Content
	if len(firstMsg) < 100 {
		t.Error("expected substantial repo context")
	}
}

func TestRepositoryContextGenerator_DifferentInstructions(t *testing.T) {
	gen := NewRepositoryContextGenerator(benchmark.WorkloadConfig{Type: "repository_context"})

	// With different iterations mod len(instructions), should get different prompts.
	req1 := gen.Generate(0)
	req2 := gen.Generate(1)

	last1 := req1.Messages[len(req1.Messages)-1].Content
	last2 := req2.Messages[len(req2.Messages)-1].Content

	if last1 == last2 {
		t.Error("expected different instructions for different iterations")
	}
}

func TestRandomGenerator_VariedOutput(t *testing.T) {
	gen := NewRandomGenerator(benchmark.WorkloadConfig{
		Type:   "random",
		Params: map[string]interface{}{"seed": 42},
	})

	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		req := gen.Generate(i)
		key := req.Messages[0].Content
		seen[key] = true
	}

	if len(seen) < 5 {
		t.Errorf("expected varied output, got only %d unique first messages", len(seen))
	}
}

func TestRandomGenerator_LastMessageIsUser(t *testing.T) {
	gen := NewRandomGenerator(benchmark.WorkloadConfig{
		Type:   "random",
		Params: map[string]interface{}{"seed": 42},
	})

	for i := 0; i < 20; i++ {
		req := gen.Generate(i)
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role != "user" {
			t.Errorf("iteration %d: expected last message role 'user', got %q", i, lastMsg.Role)
		}
	}
}

func TestParamString_Default(t *testing.T) {
	got := paramString(nil, "key", "default")
	if got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
}

func TestParamString_Found(t *testing.T) {
	params := map[string]interface{}{"key": "value"}
	got := paramString(params, "key", "default")
	if got != "value" {
		t.Errorf("expected 'value', got %q", got)
	}
}

func TestParamString_WrongType(t *testing.T) {
	params := map[string]interface{}{"key": 42}
	got := paramString(params, "key", "default")
	if got != "default" {
		t.Errorf("expected 'default', got %q", got)
	}
}

func TestParamInt_Default(t *testing.T) {
	got := paramInt(nil, "key", 10)
	if got != 10 {
		t.Errorf("expected 10, got %d", got)
	}
}

func TestParamInt_Float64(t *testing.T) {
	params := map[string]interface{}{"key": float64(42)}
	got := paramInt(params, "key", 10)
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestParamInt_Int(t *testing.T) {
	params := map[string]interface{}{"key": 42}
	got := paramInt(params, "key", 10)
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestGenerateText(t *testing.T) {
	text := generateText("prefix: ", 100)
	if len(text) < 100 {
		t.Errorf("expected at least 100 chars, got %d", len(text))
	}
	if text[:8] != "prefix: " {
		t.Errorf("expected prefix, got %q", text[:8])
	}
}
