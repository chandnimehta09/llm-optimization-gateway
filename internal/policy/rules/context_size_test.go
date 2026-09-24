package rules

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/dedup"
	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
)

// --- Helper to build a request with approximately N tokens ---

func buildRequestWithTokens(approxTokens int) *request.NormalizedRequest {
	// Each message has MessageOverheadTokens overhead + content tokens.
	// We also have BaseRequestOverheadTokens base overhead.
	// Target: produce a request whose EstimateTokens().TotalTokens ≈ approxTokens.
	//
	// Strategy: use a single large message. Content chars needed:
	//   approxTokens - BaseRequestOverheadTokens - MessageOverheadTokens = content tokens
	//   content tokens * CharsPerToken = chars needed
	overhead := BaseRequestOverheadTokens + MessageOverheadTokens
	contentTokens := approxTokens - overhead
	if contentTokens < 0 {
		contentTokens = 0
	}
	charCount := contentTokens * CharsPerToken

	return &request.NormalizedRequest{
		Model: "gpt-3.5-turbo",
		Messages: []request.Message{
			{Role: "user", Content: strings.Repeat("a", charCount)},
		},
	}
}

func defaultPolicyConfig() *policy.PolicyConfig {
	return policy.NewDefaultConfig("gpt-3.5-turbo", "openai")
}

// --- Acceptance Criteria Tests ---

func TestContextSizePolicy_OverLimit_OptimizationNeeded(t *testing.T) {
	// Given context with 5000 tokens and model limit is 4096
	// When context size policy is evaluated
	// Then decision indicates optimization needed
	// And decision.reasoning explains token overflow

	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	})

	req := buildRequestWithTokens(5000)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.OptimizationNeeded {
		t.Error("expected OptimizationNeeded to be true for 5000 tokens with 4096 limit")
	}
	if decision.TokenOverflow <= 0 {
		t.Errorf("expected positive TokenOverflow, got %d", decision.TokenOverflow)
	}
	if !strings.Contains(decision.Reasoning, "exceeds model limit") {
		t.Errorf("reasoning should mention exceeding model limit, got: %s", decision.Reasoning)
	}
	if !strings.Contains(decision.Reasoning, "overflow") {
		t.Errorf("reasoning should mention overflow, got: %s", decision.Reasoning)
	}
}

func TestContextSizePolicy_UnderLimit_NoOptimization(t *testing.T) {
	// Given context with 1000 tokens
	// When context size policy is evaluated
	// Then decision indicates no optimization needed

	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	})

	req := buildRequestWithTokens(1000)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.OptimizationNeeded {
		t.Error("expected OptimizationNeeded to be false for 1000 tokens with 4096 limit")
	}
	if decision.TokenOverflow != 0 {
		t.Errorf("expected zero TokenOverflow, got %d", decision.TokenOverflow)
	}
	if !strings.Contains(decision.Reasoning, "no optimization needed") {
		t.Errorf("reasoning should indicate no optimization, got: %s", decision.Reasoning)
	}
}

// --- Threshold Tests ---

func TestContextSizePolicy_AtWarningThreshold(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: 0.85,
		EnableDeduplication:   true,
	})

	// 85% of 4096 = 3481.6 → warning threshold is 3481
	// Build a request slightly above warning threshold but below limit.
	req := buildRequestWithTokens(3600)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.OptimizationNeeded {
		t.Error("expected OptimizationNeeded to be false at warning threshold")
	}
	if len(decision.Warnings) == 0 {
		t.Error("expected warnings when above warning threshold")
	}
	if !decision.RecommendDeduplication {
		t.Error("expected deduplication recommendation above warning threshold")
	}
}

func TestContextSizePolicy_BelowWarningThreshold(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: 0.85,
		EnableDeduplication:   true,
	})

	// Well below warning threshold.
	req := buildRequestWithTokens(500)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.OptimizationNeeded {
		t.Error("expected OptimizationNeeded to be false below warning threshold")
	}
	if len(decision.Warnings) != 0 {
		t.Errorf("expected no warnings below warning threshold, got %v", decision.Warnings)
	}
	if decision.RecommendDeduplication {
		t.Error("expected no dedup recommendation below warning threshold")
	}
}

func TestContextSizePolicy_ExactlyAtLimit(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: 0.85,
		EnableDeduplication:   true,
	})

	req := buildRequestWithTokens(4096)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Exactly at limit: should be in warning zone (above warning threshold)
	// but not optimization-needed.
	if decision.OptimizationNeeded {
		t.Error("expected OptimizationNeeded to be false at exact limit")
	}
}

func TestContextSizePolicy_OneOverLimit(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: 0.85,
		EnableDeduplication:   true,
	})

	req := buildRequestWithTokens(4097)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.OptimizationNeeded {
		t.Error("expected OptimizationNeeded to be true just over limit")
	}
	if decision.TokenOverflow != 1 {
		t.Errorf("expected TokenOverflow = 1, got %d", decision.TokenOverflow)
	}
}

// --- Config Tests ---

func TestContextSizePolicy_ModelLimitFromKnownModel(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	})

	req := &request.NormalizedRequest{
		Model: "gpt-4",
		Messages: []request.Message{
			{Role: "user", Content: "Hello"},
		},
	}
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.ModelLimit != 8192 {
		t.Errorf("expected model limit 8192 for gpt-4, got %d", decision.ModelLimit)
	}
}

func TestContextSizePolicy_ModelLimitDefault(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	})

	req := &request.NormalizedRequest{
		Model: "unknown-model",
		Messages: []request.Message{
			{Role: "user", Content: "Hello"},
		},
	}
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.ModelLimit != DefaultModelTokenLimit {
		t.Errorf("expected default model limit %d, got %d", DefaultModelTokenLimit, decision.ModelLimit)
	}
}

func TestContextSizePolicy_ModelLimitOverride(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       10000,
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	})

	req := &request.NormalizedRequest{
		Model: "gpt-4", // known to be 8192, but override is 10000
		Messages: []request.Message{
			{Role: "user", Content: "Hello"},
		},
	}
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.ModelLimit != 10000 {
		t.Errorf("expected overridden model limit 10000, got %d", decision.ModelLimit)
	}
}

func TestContextSizePolicy_DeduplicationDisabled(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       100,
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   false,
	})

	req := buildRequestWithTokens(200)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !decision.OptimizationNeeded {
		t.Error("expected optimization needed")
	}
	if decision.RecommendDeduplication {
		t.Error("expected no dedup recommendation when disabled")
	}
}

func TestContextSizePolicy_DefaultConfig(t *testing.T) {
	cfg := DefaultContextSizeConfig()
	if cfg.ModelTokenLimit != 0 {
		t.Errorf("default ModelTokenLimit should be 0 (auto-detect), got %d", cfg.ModelTokenLimit)
	}
	if cfg.WarningThresholdRatio != DefaultWarningThresholdRatio {
		t.Errorf("default WarningThresholdRatio = %f, want %f", cfg.WarningThresholdRatio, DefaultWarningThresholdRatio)
	}
	if !cfg.EnableDeduplication {
		t.Error("default EnableDeduplication should be true")
	}
}

// --- Error Handling Tests ---

func TestContextSizePolicy_NilInput(t *testing.T) {
	p := NewContextSizePolicy(DefaultContextSizeConfig())
	_, err := p.Evaluate(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil input")
	}
}

func TestContextSizePolicy_NilRequest(t *testing.T) {
	p := NewContextSizePolicy(DefaultContextSizeConfig())
	input := &policy.PolicyInput{
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}
	_, err := p.Evaluate(context.Background(), input)
	if err == nil {
		t.Error("expected error for nil request")
	}
}

// --- DedupStats Token Savings ---

func TestContextSizePolicy_WithDedupStats(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	})

	req := buildRequestWithTokens(5000)
	input := &policy.PolicyInput{
		Request: req,
		Config:  defaultPolicyConfig(),
		DedupStats: &dedup.DedupStats{
			OriginalCount:        10,
			DeduplicatedCount:    7,
			DuplicatesRemoved:    3,
			BytesSaved:           2000,
			EstimatedTokensSaved: 500,
			Strategy:             "exact",
		},
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.EstimatedTokenSavings != 500 {
		t.Errorf("EstimatedTokenSavings = %d, want 500", decision.EstimatedTokenSavings)
	}
}

func TestContextSizePolicy_NilDedupStats(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	})

	req := buildRequestWithTokens(1000)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.EstimatedTokenSavings != 0 {
		t.Errorf("EstimatedTokenSavings = %d, want 0 without stats", decision.EstimatedTokenSavings)
	}
}

// --- ApplyToDecision Tests ---

func TestApplyToDecision_OptimizationNeeded(t *testing.T) {
	p := NewContextSizePolicy(DefaultContextSizeConfig())

	csDecision := &ContextSizeDecision{
		OptimizationNeeded:     true,
		RecommendDeduplication: true,
		Reasoning:              "token overflow detected",
		Warnings:               []string{"context too large"},
	}

	pDecision := policy.NewDecision("gpt-4", "openai", "1.0.0")
	p.ApplyToDecision(csDecision, pDecision)

	if !pDecision.ApplyDeduplication {
		t.Error("expected ApplyDeduplication to be set")
	}
	if !pDecision.HasWarnings() {
		t.Error("expected warnings to be propagated")
	}
	if len(pDecision.AuditTrail) == 0 {
		t.Error("expected audit trail entry")
	}
	if !pDecision.AuditTrail[0].Matched {
		t.Error("expected audit entry to be matched")
	}
	if pDecision.AuditTrail[0].Effect != "optimize_context" {
		t.Errorf("expected effect 'optimize_context', got %q", pDecision.AuditTrail[0].Effect)
	}
}

func TestApplyToDecision_NoOptimization(t *testing.T) {
	p := NewContextSizePolicy(DefaultContextSizeConfig())

	csDecision := &ContextSizeDecision{
		OptimizationNeeded:     false,
		RecommendDeduplication: false,
		Reasoning:              "within limits",
	}

	pDecision := policy.NewDecision("gpt-4", "openai", "1.0.0")
	p.ApplyToDecision(csDecision, pDecision)

	if pDecision.ApplyDeduplication {
		t.Error("expected ApplyDeduplication to be false")
	}
	if pDecision.HasWarnings() {
		t.Error("expected no warnings")
	}
	if len(pDecision.AuditTrail) == 0 {
		t.Error("expected audit trail entry")
	}
	if pDecision.AuditTrail[0].Matched {
		t.Error("expected audit entry to not be matched")
	}
	if pDecision.AuditTrail[0].Effect != "no_action" {
		t.Errorf("expected effect 'no_action', got %q", pDecision.AuditTrail[0].Effect)
	}
}

func TestApplyToDecision_WarningZone(t *testing.T) {
	p := NewContextSizePolicy(DefaultContextSizeConfig())

	csDecision := &ContextSizeDecision{
		OptimizationNeeded:     false,
		RecommendDeduplication: true,
		Reasoning:              "near limit",
		Warnings:               []string{"approaching limit"},
	}

	pDecision := policy.NewDecision("gpt-4", "openai", "1.0.0")
	p.ApplyToDecision(csDecision, pDecision)

	if !pDecision.ApplyDeduplication {
		t.Error("expected ApplyDeduplication when recommended")
	}
	if !pDecision.HasWarnings() {
		t.Error("expected warnings to be propagated")
	}
	if pDecision.AuditTrail[0].Effect != "warn" {
		t.Errorf("expected effect 'warn', got %q", pDecision.AuditTrail[0].Effect)
	}
}

func TestApplyToDecision_NilInputs(t *testing.T) {
	p := NewContextSizePolicy(DefaultContextSizeConfig())

	// Should not panic.
	p.ApplyToDecision(nil, policy.NewDecision("m", "p", "1.0"))
	p.ApplyToDecision(&ContextSizeDecision{}, nil)
	p.ApplyToDecision(nil, nil)
}

// --- Integration Test with Large Context ---

func TestContextSizePolicy_Integration_LargeContext(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: 0.85,
		EnableDeduplication:   true,
	})

	// Build a realistic large context: system prompt + many messages + tools.
	messages := make([]request.Message, 0)
	for i := 0; i < 20; i++ {
		messages = append(messages, request.Message{
			Role:    "user",
			Content: strings.Repeat("This is a detailed user message with substantial content. ", 10),
		})
		messages = append(messages, request.Message{
			Role:    "assistant",
			Content: strings.Repeat("This is a comprehensive assistant response with code examples and explanations. ", 10),
		})
	}

	tools := make([]request.Tool, 5)
	for i := range tools {
		tools[i] = request.Tool{
			Type: "function",
			Function: request.FunctionDefinition{
				Name:        "complex_tool",
				Description: strings.Repeat("A detailed tool description. ", 5),
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"arg1": map[string]interface{}{"type": "string", "description": "First argument"},
						"arg2": map[string]interface{}{"type": "integer", "description": "Second argument"},
						"arg3": map[string]interface{}{"type": "boolean", "description": "Third argument"},
					},
				},
			},
		}
	}

	req := &request.NormalizedRequest{
		Model:        "gpt-3.5-turbo",
		SystemPrompt: strings.Repeat("You are a highly specialized assistant. ", 5),
		Messages:     messages,
		Tools:        tools,
	}

	input := &policy.PolicyInput{
		Request: req,
		Config:  defaultPolicyConfig(),
		DedupStats: &dedup.DedupStats{
			OriginalCount:        40,
			DeduplicatedCount:    35,
			DuplicatesRemoved:    5,
			BytesSaved:           5000,
			EstimatedTokensSaved: 1250,
			Strategy:             "exact",
		},
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With 40 long messages and 5 tools, this should exceed 4096 tokens.
	if !decision.OptimizationNeeded {
		t.Errorf("expected optimization needed for large context (estimated %d tokens, limit %d)",
			decision.EstimatedTokens.TotalTokens, decision.ModelLimit)
	}
	if decision.RecommendDeduplication != true {
		t.Error("expected dedup recommendation for large context")
	}
	if decision.EstimatedTokenSavings != 1250 {
		t.Errorf("EstimatedTokenSavings = %d, want 1250", decision.EstimatedTokenSavings)
	}

	// Apply to a policy decision and verify integration.
	pDecision := policy.NewDecision("gpt-3.5-turbo", "openai", "1.0.0")
	p.ApplyToDecision(decision, pDecision)

	if !pDecision.ApplyDeduplication {
		t.Error("expected ApplyDeduplication on policy decision")
	}
	if len(pDecision.AuditTrail) == 0 {
		t.Error("expected audit trail entries")
	}
}

// --- Warning Threshold Ratio Edge Cases ---

func TestContextSizePolicy_InvalidWarningRatio_UsesDefault(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: 0, // invalid
		EnableDeduplication:   true,
	})

	req := buildRequestWithTokens(3600)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default ratio = 0.85, threshold = 3481
	// 3600 > 3481 → should be in warning zone
	limit := 4096
	ratio := DefaultWarningThresholdRatio
	expectedThreshold := int(float64(limit) * ratio)
	if decision.WarningThreshold != expectedThreshold {
		t.Errorf("WarningThreshold = %d, want %d (default ratio)", decision.WarningThreshold, expectedThreshold)
	}
}

func TestContextSizePolicy_RatioAboveOne_UsesDefault(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: 1.5, // invalid
		EnableDeduplication:   true,
	})

	req := buildRequestWithTokens(100)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ratio := DefaultWarningThresholdRatio
	expectedThreshold := int(float64(4096) * ratio)
	if decision.WarningThreshold != expectedThreshold {
		t.Errorf("WarningThreshold = %d, want %d", decision.WarningThreshold, expectedThreshold)
	}
}

// --- Model Limits Table ---

func TestModelLimits_KnownModels(t *testing.T) {
	expected := map[string]int{
		"gpt-3.5-turbo":  4096,
		"gpt-4":          8192,
		"gpt-4-turbo":    128000,
		"claude-3-opus":  200000,
		"gemini-1.5-pro": 1000000,
	}
	for model, limit := range expected {
		got, ok := ModelLimits[model]
		if !ok {
			t.Errorf("ModelLimits missing %q", model)
			continue
		}
		if got != limit {
			t.Errorf("ModelLimits[%q] = %d, want %d", model, got, limit)
		}
	}
}

// --- Decision Fields ---

func TestContextSizeDecision_EvaluatedAt(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       4096,
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	})

	before := time.Now()
	req := buildRequestWithTokens(100)
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := time.Now()

	if decision.EvaluatedAt.Before(before) || decision.EvaluatedAt.After(after) {
		t.Errorf("EvaluatedAt %v not between %v and %v", decision.EvaluatedAt, before, after)
	}
}

func TestContextSizeDecision_EstimateBreakdown(t *testing.T) {
	p := NewContextSizePolicy(ContextSizeConfig{
		ModelTokenLimit:       100000,
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	})

	req := &request.NormalizedRequest{
		Model:        "gpt-4",
		SystemPrompt: "Be helpful.",
		Messages: []request.Message{
			{Role: "user", Content: "Hello"},
		},
		Tools: []request.Tool{
			{Type: "function", Function: request.FunctionDefinition{Name: "test_tool", Description: "A test"}},
		},
	}
	input := &policy.PolicyInput{
		Request:   req,
		Config:    defaultPolicyConfig(),
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	est := decision.EstimatedTokens
	if est.MessageTokens <= 0 {
		t.Error("expected MessageTokens > 0")
	}
	if est.MessageOverhead <= 0 {
		t.Error("expected MessageOverhead > 0")
	}
	if est.ToolTokens <= 0 {
		t.Error("expected ToolTokens > 0")
	}
	if est.SystemPromptTokens <= 0 {
		t.Error("expected SystemPromptTokens > 0")
	}
	if est.BaseOverhead != BaseRequestOverheadTokens {
		t.Errorf("BaseOverhead = %d, want %d", est.BaseOverhead, BaseRequestOverheadTokens)
	}
}
