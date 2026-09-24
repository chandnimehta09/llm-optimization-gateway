package rules

import (
	"testing"
	"time"

	ctxidentity "github.com/llm-optimization-gateway/internal/context"
	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
)

// --------------------------------------------------------------------------
// Eligibility rule tests
// --------------------------------------------------------------------------

func TestCheckCacheEligibility_AllChecksPassed(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-1",
			Model: "gpt-4",
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    30 * time.Second,
		Config:      policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if !result.Eligible {
		t.Errorf("expected eligible, got ineligible: %s", result.Reason)
	}
	if len(result.CheckedRules) == 0 {
		t.Error("expected checked rules to be populated")
	}
}

func TestCheckCacheEligibility_CacheDisabledByRequest(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:       "req-disabled",
			Model:    "gpt-4",
			Metadata: map[string]string{"cache": "false"},
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if result.Eligible {
		t.Error("expected ineligible when cache disabled")
	}
	if result.Reason != string(BypassReasonCacheDisabled) {
		t.Errorf("reason = %q, want %q", result.Reason, BypassReasonCacheDisabled)
	}
	if result.BypassRule != "request_allows_caching" {
		t.Errorf("bypass rule = %q, want %q", result.BypassRule, "request_allows_caching")
	}
}

func TestCheckCacheEligibility_CacheDisabledVariants(t *testing.T) {
	variants := []string{"false", "disabled", "off", "no"}
	for _, val := range variants {
		t.Run("cache="+val, func(t *testing.T) {
			input := &policy.PolicyInput{
				Request: &request.NormalizedRequest{
					ID:       "req-var",
					Model:    "gpt-4",
					Metadata: map[string]string{"cache": val},
				},
				CacheStatus: policy.CacheStatusHit,
				Config:      policy.NewDefaultConfig("gpt-4", "openai"),
				Timestamp:   time.Now(),
			}

			result := CheckCacheEligibility(input)
			if result.Eligible {
				t.Errorf("expected ineligible for cache=%s", val)
			}
		})
	}
}

func TestCheckCacheEligibility_CacheDisabledViaRequestMetadata(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-meta",
			Model: "gpt-4",
		},
		RequestMetadata: map[string]string{"cache": "false"},
		CacheStatus:     policy.CacheStatusHit,
		Config:          policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:       time.Now(),
	}

	result := CheckCacheEligibility(input)
	if result.Eligible {
		t.Error("expected ineligible when cache disabled via request metadata")
	}
}

func TestCheckCacheEligibility_NoCacheEntry(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-miss",
			Model: "gpt-4",
		},
		CacheStatus: policy.CacheStatusMiss,
		Config:      policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if result.Eligible {
		t.Error("expected ineligible for cache miss")
	}
	if result.BypassRule != "cache_entry_exists" {
		t.Errorf("bypass rule = %q, want %q", result.BypassRule, "cache_entry_exists")
	}
}

func TestCheckCacheEligibility_CacheBypass(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-bypass",
			Model: "gpt-4",
		},
		CacheStatus: policy.CacheStatusBypass,
		Config:      policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if result.Eligible {
		t.Error("expected ineligible for cache bypass status")
	}
}

func TestCheckCacheEligibility_ExpiredEntry(t *testing.T) {
	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []policy.Rule{
		{
			ID:       "ttl-rule",
			Name:     "cache-ttl",
			Priority: 1,
			Target:   policy.RuleTargetCache,
			Action:   policy.RuleActionAllow,
			Value:    "1m", // 1 minute TTL.
			Enabled:  true,
		},
	}

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-expired",
			Model: "gpt-4",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    5 * time.Minute, // Exceeds 1m TTL.
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if result.Eligible {
		t.Error("expected ineligible for expired cache entry")
	}
	if result.BypassRule != "cache_not_expired" {
		t.Errorf("bypass rule = %q, want %q", result.BypassRule, "cache_not_expired")
	}
}

func TestCheckCacheEligibility_FreshEntry(t *testing.T) {
	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []policy.Rule{
		{
			ID:       "ttl-rule",
			Name:     "cache-ttl",
			Priority: 1,
			Target:   policy.RuleTargetCache,
			Action:   policy.RuleActionAllow,
			Value:    "10m",
			Enabled:  true,
		},
	}

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-fresh",
			Model: "gpt-4",
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    30 * time.Second,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if !result.Eligible {
		t.Errorf("expected eligible for fresh cache entry, got: %s", result.Reason)
	}
}

func TestCheckCacheEligibility_ContextIdentityMismatch(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-identity",
			Model: "gpt-4",
		},
		Identity: &ctxidentity.ContextIdentity{
			ID: "", // Empty ID indicates invalid identity.
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if result.Eligible {
		t.Error("expected ineligible for identity mismatch")
	}
	if result.BypassRule != "context_identity_matches" {
		t.Errorf("bypass rule = %q, want %q", result.BypassRule, "context_identity_matches")
	}
}

func TestCheckCacheEligibility_HighTemperatureNotCompatible(t *testing.T) {
	temp := 1.8
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:          "req-temp",
			Model:       "gpt-4",
			Temperature: &temp,
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if result.Eligible {
		t.Error("expected ineligible for high temperature")
	}
	if result.BypassRule != "model_params_compatible" {
		t.Errorf("bypass rule = %q, want %q", result.BypassRule, "model_params_compatible")
	}
}

func TestCheckCacheEligibility_ModerateTemperatureCompatible(t *testing.T) {
	temp := 0.7
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:          "req-temp-ok",
			Model:       "gpt-4",
			Temperature: &temp,
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if !result.Eligible {
		t.Errorf("expected eligible for moderate temperature, got: %s", result.Reason)
	}
}

func TestCheckCacheEligibility_NilRequest(t *testing.T) {
	input := &policy.PolicyInput{
		CacheStatus: policy.CacheStatusHit,
		Config:      policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if !result.Eligible {
		t.Errorf("expected eligible with nil request, got: %s", result.Reason)
	}
}

func TestCheckCacheEligibility_NilIdentityAllowed(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-no-id",
			Model: "gpt-4",
		},
		Identity:    nil,
		CacheStatus: policy.CacheStatusHit,
		Config:      policy.NewDefaultConfig("gpt-4", "openai"),
		Timestamp:   time.Now(),
	}

	result := CheckCacheEligibility(input)
	if !result.Eligible {
		t.Errorf("expected eligible with nil identity, got: %s", result.Reason)
	}
}

// --------------------------------------------------------------------------
// Bypass rule tests
// --------------------------------------------------------------------------

func TestCheckCacheBypass_NoBypass(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-normal",
			Model: "gpt-4",
		},
	}

	result := CheckCacheBypass(input)
	if result.Bypass {
		t.Errorf("expected no bypass, got: %s", result.Reason)
	}
	if result.Reason != BypassReasonNone {
		t.Errorf("reason = %q, want empty", result.Reason)
	}
}

func TestCheckCacheBypass_StreamingRequest(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:     "req-stream",
			Model:  "gpt-4",
			Stream: true,
		},
	}

	result := CheckCacheBypass(input)
	if !result.Bypass {
		t.Error("expected bypass for streaming request")
	}
	if result.Reason != BypassReasonStreaming {
		t.Errorf("reason = %q, want %q", result.Reason, BypassReasonStreaming)
	}
	if result.RuleName != "streaming_bypass" {
		t.Errorf("rule name = %q, want %q", result.RuleName, "streaming_bypass")
	}
}

func TestCheckCacheBypass_SensitiveData_RequestMetadata(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:       "req-sensitive",
			Model:    "gpt-4",
			Metadata: map[string]string{"sensitive": "true"},
		},
	}

	result := CheckCacheBypass(input)
	if !result.Bypass {
		t.Error("expected bypass for sensitive data")
	}
	if result.Reason != BypassReasonSensitiveData {
		t.Errorf("reason = %q, want %q", result.Reason, BypassReasonSensitiveData)
	}
}

func TestCheckCacheBypass_SensitiveData_PII(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:       "req-pii",
			Model:    "gpt-4",
			Metadata: map[string]string{"pii": "true"},
		},
	}

	result := CheckCacheBypass(input)
	if !result.Bypass {
		t.Error("expected bypass for PII data")
	}
	if result.Reason != BypassReasonSensitiveData {
		t.Errorf("reason = %q, want %q", result.Reason, BypassReasonSensitiveData)
	}
}

func TestCheckCacheBypass_SensitiveData_PolicyMetadata(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-sens-policy",
			Model: "gpt-4",
		},
		RequestMetadata: map[string]string{"sensitive": "true"},
	}

	result := CheckCacheBypass(input)
	if !result.Bypass {
		t.Error("expected bypass for sensitive data in policy metadata")
	}
}

func TestCheckCacheBypass_ProbeRequest(t *testing.T) {
	zero := 0
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:        "req-probe",
			Model:     "gpt-4",
			MaxTokens: &zero,
		},
	}

	result := CheckCacheBypass(input)
	if !result.Bypass {
		t.Error("expected bypass for probe request")
	}
	if result.Reason != BypassReasonProbeRequest {
		t.Errorf("reason = %q, want %q", result.Reason, BypassReasonProbeRequest)
	}
	if result.RuleName != "probe_request_bypass" {
		t.Errorf("rule name = %q, want %q", result.RuleName, "probe_request_bypass")
	}
}

func TestCheckCacheBypass_NonDeterministicTools(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-tools",
			Model: "gpt-4",
			Tools: []request.Tool{
				{
					Type: "function",
					Function: request.FunctionDefinition{
						Name:        "get_current_time",
						Description: "Gets the current time",
					},
				},
			},
		},
	}

	result := CheckCacheBypass(input)
	if !result.Bypass {
		t.Error("expected bypass for non-deterministic tools")
	}
	if result.Reason != BypassReasonNonDeterministicTools {
		t.Errorf("reason = %q, want %q", result.Reason, BypassReasonNonDeterministicTools)
	}
}

func TestCheckCacheBypass_NonDeterministicToolInMessages(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-tool-msg",
			Model: "gpt-4",
			Messages: []request.Message{
				{
					Role:    "assistant",
					Content: "Calling tool...",
					ToolCalls: []request.ToolCall{
						{
							ID:   "tc-1",
							Type: "function",
							Function: request.FunctionCall{
								Name:      "web_search",
								Arguments: `{"query": "latest news"}`,
							},
						},
					},
				},
			},
		},
	}

	result := CheckCacheBypass(input)
	if !result.Bypass {
		t.Error("expected bypass for non-deterministic tool in messages")
	}
}

func TestCheckCacheBypass_DeterministicToolsAllowed(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-det-tools",
			Model: "gpt-4",
			Tools: []request.Tool{
				{
					Type: "function",
					Function: request.FunctionDefinition{
						Name:        "calculate_sum",
						Description: "Calculates the sum of two numbers",
					},
				},
			},
		},
	}

	result := CheckCacheBypass(input)
	if result.Bypass {
		t.Errorf("expected no bypass for deterministic tools, got: %s", result.Reason)
	}
}

func TestCheckCacheBypass_NilRequest(t *testing.T) {
	input := &policy.PolicyInput{}

	result := CheckCacheBypass(input)
	if result.Bypass {
		t.Error("expected no bypass for nil request")
	}
}

func TestCheckCacheBypass_NonZeroMaxTokensNotProbe(t *testing.T) {
	maxTokens := 100
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:        "req-tokens",
			Model:     "gpt-4",
			MaxTokens: &maxTokens,
		},
	}

	result := CheckCacheBypass(input)
	if result.Bypass {
		t.Errorf("expected no bypass for non-zero max_tokens, got: %s", result.Reason)
	}
}

// --------------------------------------------------------------------------
// Staleness detection tests
// --------------------------------------------------------------------------

func TestCheckCacheStaleness_Fresh(t *testing.T) {
	result := CheckCacheStaleness(30*time.Second, 5*time.Minute)
	if result.Stale {
		t.Error("expected fresh, got stale")
	}
	if result.Age != 30*time.Second {
		t.Errorf("age = %v, want 30s", result.Age)
	}
	if result.TTL != 5*time.Minute {
		t.Errorf("ttl = %v, want 5m", result.TTL)
	}
}

func TestCheckCacheStaleness_Expired(t *testing.T) {
	result := CheckCacheStaleness(10*time.Minute, 5*time.Minute)
	if !result.Stale {
		t.Error("expected stale, got fresh")
	}
}

func TestCheckCacheStaleness_ExactlyAtTTL(t *testing.T) {
	result := CheckCacheStaleness(5*time.Minute, 5*time.Minute)
	if result.Stale {
		t.Error("expected fresh at exact TTL boundary")
	}
}

func TestCheckCacheStaleness_JustOverTTL(t *testing.T) {
	result := CheckCacheStaleness(5*time.Minute+1*time.Millisecond, 5*time.Minute)
	if !result.Stale {
		t.Error("expected stale just over TTL")
	}
}

func TestCheckCacheStaleness_ZeroTTL(t *testing.T) {
	result := CheckCacheStaleness(1*time.Hour, 0)
	if result.Stale {
		t.Error("expected fresh with zero TTL (no expiration)")
	}
}

func TestCheckCacheStaleness_NegativeTTL(t *testing.T) {
	result := CheckCacheStaleness(1*time.Hour, -1*time.Minute)
	if result.Stale {
		t.Error("expected fresh with negative TTL (no expiration)")
	}
}

func TestCheckCacheStaleness_ZeroAge(t *testing.T) {
	result := CheckCacheStaleness(0, 5*time.Minute)
	if result.Stale {
		t.Error("expected fresh with zero age")
	}
}

func TestCheckContextChangeStaleness_Same(t *testing.T) {
	result := CheckContextChangeStaleness("abc123", "abc123")
	if result.Stale {
		t.Error("expected not stale for same identity")
	}
}

func TestCheckContextChangeStaleness_Different(t *testing.T) {
	result := CheckContextChangeStaleness("abc123", "def456")
	if !result.Stale {
		t.Error("expected stale for different identities")
	}
}

func TestCheckContextChangeStaleness_EmptyCached(t *testing.T) {
	result := CheckContextChangeStaleness("", "def456")
	if result.Stale {
		t.Error("expected not stale when cached identity is empty")
	}
}

func TestCheckContextChangeStaleness_EmptyCurrent(t *testing.T) {
	result := CheckContextChangeStaleness("abc123", "")
	if result.Stale {
		t.Error("expected not stale when current identity is empty")
	}
}

func TestCheckContextChangeStaleness_BothEmpty(t *testing.T) {
	result := CheckContextChangeStaleness("", "")
	if result.Stale {
		t.Error("expected not stale when both identities are empty")
	}
}

// --------------------------------------------------------------------------
// Cost savings tests
// --------------------------------------------------------------------------

func TestCalculateCostSavings_BasicRequest(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-cost",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "Hello, how are you today? This is a test message with some content."},
			},
		},
		Providers: []policy.ProviderInfo{
			{Name: "openai", AverageLatencyMs: 300.0},
		},
	}

	savings := CalculateCostSavings(input)
	if savings.EstimatedInputTokens <= 0 {
		t.Error("expected positive input tokens")
	}
	if savings.EstimatedOutputTokens <= 0 {
		t.Error("expected positive output tokens")
	}
	if savings.EstimatedCostUSD <= 0 {
		t.Error("expected positive cost savings")
	}
	if savings.LatencySavedMs != 300.0 {
		t.Errorf("latency saved = %f, want 300.0", savings.LatencySavedMs)
	}
}

func TestCalculateCostSavings_WithMaxTokens(t *testing.T) {
	maxTokens := 500
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:        "req-cost-mt",
			Model:     "gpt-4",
			MaxTokens: &maxTokens,
			Messages: []request.Message{
				{Role: "user", Content: "Test message"},
			},
		},
	}

	savings := CalculateCostSavings(input)
	if savings.EstimatedOutputTokens != 250 {
		t.Errorf("estimated output tokens = %d, want 250 (half of max_tokens)", savings.EstimatedOutputTokens)
	}
}

func TestCalculateCostSavings_NilRequest(t *testing.T) {
	input := &policy.PolicyInput{}

	savings := CalculateCostSavings(input)
	if savings.EstimatedInputTokens != 0 {
		t.Errorf("expected 0 input tokens for nil request, got %d", savings.EstimatedInputTokens)
	}
}

func TestCalculateCostSavings_DefaultLatency(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-lat",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "test"},
			},
		},
	}

	savings := CalculateCostSavings(input)
	if savings.LatencySavedMs != 500.0 {
		t.Errorf("default latency saved = %f, want 500.0", savings.LatencySavedMs)
	}
}

func TestCalculateCostSavings_DifferentModels(t *testing.T) {
	models := []struct {
		name        string
		expectInput float64 // approx per-1K
	}{
		{"gpt-4", 0.03},
		{"gpt-3.5-turbo", 0.0005},
		{"claude-3-opus-20240229", 0.015},
		{"claude-3-sonnet-20240229", 0.003},
		{"claude-3-haiku-20240307", 0.00025},
		{"gemini-pro", 0.0005},
		{"unknown-model", 0.01},
	}

	for _, m := range models {
		t.Run(m.name, func(t *testing.T) {
			input := &policy.PolicyInput{
				Request: &request.NormalizedRequest{
					ID:    "req-model",
					Model: m.name,
					Messages: []request.Message{
						{Role: "user", Content: "test message content here for cost estimation"},
					},
				},
			}

			savings := CalculateCostSavings(input)
			if savings.EstimatedCostUSD <= 0 {
				t.Error("expected positive cost savings")
			}
		})
	}
}

func TestCalculateCostSavings_MultiPartContent(t *testing.T) {
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-multi",
			Model: "gpt-4",
			Messages: []request.Message{
				{
					Role: "user",
					Parts: []request.ContentPart{
						{Type: "text", Text: "Hello, this is part one."},
						{Type: "text", Text: "And this is part two."},
					},
				},
			},
		},
	}

	savings := CalculateCostSavings(input)
	if savings.EstimatedInputTokens <= 0 {
		t.Error("expected positive input tokens from multi-part content")
	}
}

// --------------------------------------------------------------------------
// Helper function tests
// --------------------------------------------------------------------------

func TestIsNonDeterministicTool(t *testing.T) {
	deterministic := []string{"calculate_sum", "format_text", "validate_json"}
	nonDeterministic := []string{"get_current_time", "web_search", "random_number", "execute_command"}

	for _, name := range deterministic {
		if IsNonDeterministicTool(name) {
			t.Errorf("%q should be deterministic", name)
		}
	}

	for _, name := range nonDeterministic {
		if !IsNonDeterministicTool(name) {
			t.Errorf("%q should be non-deterministic", name)
		}
	}
}

func TestRegisterNonDeterministicTool(t *testing.T) {
	name := "my_custom_random_tool"
	if IsNonDeterministicTool(name) {
		t.Fatalf("%q should not be non-deterministic initially", name)
	}

	RegisterNonDeterministicTool(name)
	if !IsNonDeterministicTool(name) {
		t.Errorf("%q should be non-deterministic after registration", name)
	}

	// Cleanup: remove the registered tool.
	delete(nonDeterministicTools, name)
}

func TestGetCacheTTL_NoConfig(t *testing.T) {
	ttl := getCacheTTL(nil)
	if ttl != 0 {
		t.Errorf("expected 0 TTL for nil config, got %v", ttl)
	}
}

func TestGetCacheTTL_NoRules(t *testing.T) {
	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	ttl := getCacheTTL(cfg)
	if ttl != 0 {
		t.Errorf("expected 0 TTL for no rules, got %v", ttl)
	}
}

func TestGetCacheTTL_WithRule(t *testing.T) {
	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []policy.Rule{
		{
			ID:      "ttl",
			Name:    "ttl",
			Target:  policy.RuleTargetCache,
			Action:  policy.RuleActionAllow,
			Value:   "5m",
			Enabled: true,
		},
	}

	ttl := getCacheTTL(cfg)
	if ttl != 5*time.Minute {
		t.Errorf("expected 5m TTL, got %v", ttl)
	}
}

func TestGetCacheTTL_InvalidDuration(t *testing.T) {
	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []policy.Rule{
		{
			ID:      "ttl",
			Name:    "ttl",
			Target:  policy.RuleTargetCache,
			Action:  policy.RuleActionAllow,
			Value:   "not-a-duration",
			Enabled: true,
		},
	}

	ttl := getCacheTTL(cfg)
	if ttl != 0 {
		t.Errorf("expected 0 TTL for invalid duration, got %v", ttl)
	}
}

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "short"},
		{"exactly12ch", "exactly12ch"},
		{"a-very-long-identity-hash-here", "a-very-long-..."},
		{"", ""},
	}

	for _, tc := range tests {
		got := truncateID(tc.input)
		if got != tc.want {
			t.Errorf("truncateID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
