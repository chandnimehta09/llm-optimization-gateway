package rules

import (
	"context"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
)

// --------------------------------------------------------------------------
// Helper: create a PolicyInput with common defaults
// --------------------------------------------------------------------------

func newTestInput(model string, providers []policy.ProviderInfo) *policy.PolicyInput {
	cfg := policy.NewDefaultConfig("gpt-3.5-turbo", "openai")
	return &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "test-req",
			Model: model,
		},
		CacheStatus: policy.CacheStatusMiss,
		Providers:   providers,
		Config:      cfg,
		Timestamp:   time.Now(),
	}
}

// --------------------------------------------------------------------------
// Tests: Acceptance criteria
// --------------------------------------------------------------------------

func TestRoutingPolicy_AcceptanceCriteria_GPT4(t *testing.T) {
	// Given request for "gpt-4"
	// And routing config maps gpt-4 to provider "openai"
	// When routing policy is evaluated
	// Then decision.model = "gpt-4" and decision.provider = "openai"

	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	input := newTestInput("gpt-4", nil)

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedModel != "gpt-4" {
		t.Errorf("SelectedModel = %q, want %q", decision.SelectedModel, "gpt-4")
	}
	if decision.SelectedProvider != "openai" {
		t.Errorf("SelectedProvider = %q, want %q", decision.SelectedProvider, "openai")
	}
}

func TestRoutingPolicy_AcceptanceCriteria_FastModel(t *testing.T) {
	// Given request for "fast-model"
	// And routing config maps fast-model to "gpt-3.5-turbo"
	// When routing policy is evaluated
	// Then decision.model = "gpt-3.5-turbo"

	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	input := newTestInput("fast-model", nil)

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedModel != "gpt-3.5-turbo" {
		t.Errorf("SelectedModel = %q, want %q", decision.SelectedModel, "gpt-3.5-turbo")
	}
}

func TestRoutingPolicy_AcceptanceCriteria_GeminiPro(t *testing.T) {
	// Given request for "gemini-pro"
	// And routing config maps gemini-pro to provider "google"
	// When routing policy is evaluated
	// Then decision.model = "gemini-pro" and decision.provider = "google"

	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	input := newTestInput("gemini-pro", nil)

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedModel != "gemini-pro" {
		t.Errorf("SelectedModel = %q, want %q", decision.SelectedModel, "gemini-pro")
	}
	if decision.SelectedProvider != "google" {
		t.Errorf("SelectedProvider = %q, want %q", decision.SelectedProvider, "google")
	}
}

// --------------------------------------------------------------------------
// Tests: Basic routing
// --------------------------------------------------------------------------

func TestRoutingPolicy_NilInput(t *testing.T) {
	rp := NewRoutingPolicy(nil, "1.0.0")
	_, err := rp.Evaluate(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil input")
	}
}

func TestRoutingPolicy_NilConfig(t *testing.T) {
	rp := NewRoutingPolicy(nil, "1.0.0")
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{Model: "gpt-4"},
	}
	_, err := rp.Evaluate(context.Background(), input)
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestRoutingPolicy_EmptyModel(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	input := newTestInput("", nil)

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Should use defaults.
	if decision.SelectedModel != "gpt-3.5-turbo" {
		t.Errorf("SelectedModel = %q, want default gpt-3.5-turbo", decision.SelectedModel)
	}
	if decision.SelectedProvider != "openai" {
		t.Errorf("SelectedProvider = %q, want default openai", decision.SelectedProvider)
	}
}

func TestRoutingPolicy_NilRequest(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	cfg := policy.NewDefaultConfig("gpt-3.5-turbo", "openai")
	input := &policy.PolicyInput{
		Request:     nil,
		CacheStatus: policy.CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedModel != "gpt-3.5-turbo" {
		t.Errorf("SelectedModel = %q, want default", decision.SelectedModel)
	}
}

// --------------------------------------------------------------------------
// Tests: Alias routing
// --------------------------------------------------------------------------

func TestRoutingPolicy_AliasRouting(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	tests := []struct {
		alias        string
		wantModel    string
		wantProvider string
	}{
		{"fast-model", "gpt-3.5-turbo", "openai"},
		{"smart-model", "gpt-4", "openai"},
		{"cheap-model", "gpt-3.5-turbo", "openai"},
		{"best-model", "gpt-4", "openai"},
		{"gemini", "gemini-pro", "google"},
		{"gemini-latest", "gemini-1.5-pro", "google"},
	}

	for _, tt := range tests {
		t.Run(tt.alias, func(t *testing.T) {
			input := newTestInput(tt.alias, nil)

			decision, err := rp.Evaluate(context.Background(), input)
			if err != nil {
				t.Fatalf("Evaluate returned error: %v", err)
			}

			if decision.SelectedModel != tt.wantModel {
				t.Errorf("SelectedModel = %q, want %q", decision.SelectedModel, tt.wantModel)
			}
			if decision.SelectedProvider != tt.wantProvider {
				t.Errorf("SelectedProvider = %q, want %q", decision.SelectedProvider, tt.wantProvider)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Tests: Direct model routing for OpenAI models
// --------------------------------------------------------------------------

func TestRoutingPolicy_OpenAIModels(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	models := []string{"gpt-4", "gpt-4-turbo", "gpt-3.5-turbo"}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			input := newTestInput(model, nil)

			decision, err := rp.Evaluate(context.Background(), input)
			if err != nil {
				t.Fatalf("Evaluate returned error: %v", err)
			}

			if decision.SelectedModel != model {
				t.Errorf("SelectedModel = %q, want %q", decision.SelectedModel, model)
			}
			if decision.SelectedProvider != "openai" {
				t.Errorf("SelectedProvider = %q, want openai", decision.SelectedProvider)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Tests: Direct model routing for Gemini models
// --------------------------------------------------------------------------

func TestRoutingPolicy_GeminiModels(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	models := []string{"gemini-pro", "gemini-1.5-pro", "gemini-1.5-flash"}
	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			input := newTestInput(model, nil)

			decision, err := rp.Evaluate(context.Background(), input)
			if err != nil {
				t.Fatalf("Evaluate returned error: %v", err)
			}

			if decision.SelectedModel != model {
				t.Errorf("SelectedModel = %q, want %q", decision.SelectedModel, model)
			}
			if decision.SelectedProvider != "google" {
				t.Errorf("SelectedProvider = %q, want google", decision.SelectedProvider)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Tests: Deprecated model routing
// --------------------------------------------------------------------------

func TestRoutingPolicy_DeprecatedModel(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	routingConfig.Models["old-gpt"] = ModelInfo{
		CanonicalName:    "old-gpt",
		Provider:         "openai",
		Deprecated:       true,
		ReplacementModel: "gpt-4",
		CostPer1KTokens:  0.06,
	}
	routingConfig.Providers["openai"] = ProviderConfig{
		Name:               "openai",
		Priority:           1,
		Healthy:            true,
		RateLimitRemaining: 1.0,
		CostMultiplier:     1.0,
		Models:             append(routingConfig.Providers["openai"].Models, "old-gpt"),
	}

	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	input := newTestInput("old-gpt", nil)

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedModel != "gpt-4" {
		t.Errorf("SelectedModel = %q, want gpt-4 (replacement)", decision.SelectedModel)
	}
	if !decision.HasWarnings() {
		t.Error("expected deprecation warning")
	}
}

// --------------------------------------------------------------------------
// Tests: Provider health
// --------------------------------------------------------------------------

func TestRoutingPolicy_UnhealthyProviderFallback(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	// Make openai unhealthy.
	openai := routingConfig.Providers["openai"]
	openai.Healthy = false
	routingConfig.Providers["openai"] = openai

	// Add gpt-4 to google's models.
	google := routingConfig.Providers["google"]
	google.Models = append(google.Models, "gpt-4")
	routingConfig.Providers["google"] = google

	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	input := newTestInput("gpt-4", nil)

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedProvider == "openai" {
		t.Error("should not route to unhealthy provider openai")
	}
}

// --------------------------------------------------------------------------
// Tests: Rate limit awareness
// --------------------------------------------------------------------------

func TestRoutingPolicy_RateLimitedProviderFallback(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	// Make openai rate limited.
	openai := routingConfig.Providers["openai"]
	openai.RateLimitRemaining = 0
	routingConfig.Providers["openai"] = openai

	// Add gpt-4 to google's models.
	google := routingConfig.Providers["google"]
	google.Models = append(google.Models, "gpt-4")
	routingConfig.Providers["google"] = google

	rp := NewRoutingPolicy(routingConfig, "1.0.0")
	input := newTestInput("gpt-4", nil)

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedProvider == "openai" {
		t.Error("should not route to rate-limited provider openai")
	}
}

// --------------------------------------------------------------------------
// Tests: Cost-optimized routing strategy
// --------------------------------------------------------------------------

func TestRoutingPolicy_CostOptimizedStrategy(t *testing.T) {
	routingConfig := &RoutingConfig{
		Models: map[string]ModelInfo{
			"test-model": {
				CanonicalName:   "test-model",
				CostPer1KTokens: 0.01,
				Capabilities:    []string{"chat"},
			},
		},
		Providers: map[string]ProviderConfig{
			"expensive": {
				Name:               "expensive",
				Priority:           1,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     2.0,
				Models:             []string{"test-model"},
			},
			"cheap": {
				Name:               "cheap",
				Priority:           2,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     0.5,
				Models:             []string{"test-model"},
			},
		},
		DefaultStrategy: RoutingStrategyCostOptimized,
		DefaultProvider:  "expensive",
		DefaultModel:     "test-model",
	}

	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	cfg := policy.NewDefaultConfig("test-model", "expensive")
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-cost",
			Model: "test-model",
		},
		CacheStatus: policy.CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedProvider != "cheap" {
		t.Errorf("SelectedProvider = %q, want cheap (cost-optimized)", decision.SelectedProvider)
	}
}

// --------------------------------------------------------------------------
// Tests: Latency-optimized routing strategy
// --------------------------------------------------------------------------

func TestRoutingPolicy_LatencyOptimizedStrategy(t *testing.T) {
	routingConfig := &RoutingConfig{
		Models: map[string]ModelInfo{
			"test-model": {
				CanonicalName:    "test-model",
				AverageLatencyMs: 100,
				Capabilities:     []string{"chat"},
			},
		},
		Providers: map[string]ProviderConfig{
			"prov-a": {
				Name:               "prov-a",
				Priority:           1,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				Models:             []string{"test-model"},
			},
			"prov-b": {
				Name:               "prov-b",
				Priority:           2,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				Models:             []string{"test-model"},
			},
		},
		DefaultStrategy: RoutingStrategyLatencyOptimized,
		DefaultProvider:  "prov-a",
		DefaultModel:     "test-model",
	}

	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	cfg := policy.NewDefaultConfig("test-model", "prov-a")
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-latency",
			Model: "test-model",
		},
		CacheStatus: policy.CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Both have same model-level latency, so either is valid.
	if decision.SelectedProvider != "prov-a" && decision.SelectedProvider != "prov-b" {
		t.Errorf("SelectedProvider = %q, want prov-a or prov-b", decision.SelectedProvider)
	}
}

// --------------------------------------------------------------------------
// Tests: Capability-based routing strategy
// --------------------------------------------------------------------------

func TestRoutingPolicy_CapabilityBasedStrategy(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	routingConfig.DefaultStrategy = RoutingStrategyCapabilityBased

	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-cap",
			Model: "gpt-4",
			Tools: []request.Tool{
				{Type: "function", Function: request.FunctionDefinition{Name: "get_weather"}},
			},
		},
		CacheStatus: policy.CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedProvider != "openai" {
		t.Errorf("SelectedProvider = %q, want openai (supports function_calling)", decision.SelectedProvider)
	}
}

// --------------------------------------------------------------------------
// Tests: Strategy from metadata
// --------------------------------------------------------------------------

func TestRoutingPolicy_StrategyFromMetadata(t *testing.T) {
	routingConfig := &RoutingConfig{
		Models: map[string]ModelInfo{
			"test-model": {
				CanonicalName:   "test-model",
				CostPer1KTokens: 0.01,
				Capabilities:    []string{"chat"},
			},
		},
		Providers: map[string]ProviderConfig{
			"expensive": {
				Name:               "expensive",
				Priority:           1,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     3.0,
				Models:             []string{"test-model"},
			},
			"cheap": {
				Name:               "cheap",
				Priority:           2,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     0.5,
				Models:             []string{"test-model"},
			},
		},
		DefaultStrategy: RoutingStrategyDirect,
		DefaultProvider:  "expensive",
		DefaultModel:     "test-model",
	}

	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	cfg := policy.NewDefaultConfig("test-model", "expensive")
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-meta",
			Model: "test-model",
			Metadata: map[string]string{
				"routing_strategy": "cost_optimized",
			},
		},
		CacheStatus: policy.CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedProvider != "cheap" {
		t.Errorf("SelectedProvider = %q, want cheap (cost_optimized via metadata)", decision.SelectedProvider)
	}
}

// --------------------------------------------------------------------------
// Tests: Fallback providers in decision
// --------------------------------------------------------------------------

func TestRoutingPolicy_FallbackProviders(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	input := newTestInput("gpt-4", nil)

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// gpt-4 on openai should have google as fallback (via equivalent gemini-1.5-pro).
	if decision.SelectedProvider != "openai" {
		t.Errorf("primary provider = %q, want openai", decision.SelectedProvider)
	}
	// Verify audit trail records routing steps.
	if len(decision.AuditTrail) == 0 {
		t.Error("expected audit trail entries")
	}
}

// --------------------------------------------------------------------------
// Tests: Audit trail
// --------------------------------------------------------------------------

func TestRoutingPolicy_AuditTrail(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	input := newTestInput("fast-model", nil)

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Should have alias resolution and provider selection audit entries.
	if len(decision.AuditTrail) < 2 {
		t.Errorf("expected at least 2 audit entries, got %d", len(decision.AuditTrail))
	}

	// Check that alias resolution is recorded.
	foundAlias := false
	for _, entry := range decision.AuditTrail {
		if entry.RuleID == "routing-alias" && entry.Matched {
			foundAlias = true
		}
	}
	if !foundAlias {
		t.Error("expected alias resolution audit entry")
	}

	// Applied rules should include alias and provider.
	if len(decision.AppliedRules) < 2 {
		t.Errorf("expected at least 2 applied rules, got %d: %v", len(decision.AppliedRules), decision.AppliedRules)
	}
}

// --------------------------------------------------------------------------
// Tests: Policy rules evaluation
// --------------------------------------------------------------------------

func TestRoutingPolicy_PolicyRuleOverridesProvider(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	cfg := policy.NewDefaultConfig("gpt-3.5-turbo", "openai")
	cfg.Rules = []policy.Rule{
		{
			ID:       "force-google",
			Name:     "force google for gpt-4",
			Priority: 1,
			Conditions: []policy.Condition{
				{Field: "request.model", Operator: policy.ConditionOpEquals, Value: "gpt-4"},
			},
			Target:  policy.RuleTargetRouting,
			Action:  policy.RuleActionAllow,
			Value:   "google",
			Enabled: true,
		},
	}

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-override",
			Model: "gpt-4",
		},
		CacheStatus: policy.CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedProvider != "google" {
		t.Errorf("SelectedProvider = %q, want google (overridden by policy rule)", decision.SelectedProvider)
	}
}

// --------------------------------------------------------------------------
// Tests: Decision validation
// --------------------------------------------------------------------------

func TestRoutingPolicy_DecisionIsValid(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	models := []string{"gpt-4", "gpt-3.5-turbo", "gemini-pro", "gemini-1.5-pro", "fast-model", "gemini"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			input := newTestInput(model, nil)

			decision, err := rp.Evaluate(context.Background(), input)
			if err != nil {
				t.Fatalf("Evaluate returned error: %v", err)
			}

			if err := decision.Validate(); err != nil {
				t.Errorf("decision.Validate() = %v", err)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Tests: Deterministic routing
// --------------------------------------------------------------------------

func TestRoutingPolicy_Deterministic(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	input := newTestInput("gpt-4", nil)

	d1, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("first Evaluate: %v", err)
	}

	d2, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("second Evaluate: %v", err)
	}

	if d1.SelectedModel != d2.SelectedModel {
		t.Errorf("SelectedModel not deterministic: %q vs %q", d1.SelectedModel, d2.SelectedModel)
	}
	if d1.SelectedProvider != d2.SelectedProvider {
		t.Errorf("SelectedProvider not deterministic: %q vs %q", d1.SelectedProvider, d2.SelectedProvider)
	}
}

// --------------------------------------------------------------------------
// Tests: Constructor defaults
// --------------------------------------------------------------------------

func TestNewRoutingPolicy_DefaultConfig(t *testing.T) {
	rp := NewRoutingPolicy(nil, "")
	if rp.routingConfig == nil {
		t.Error("expected default routing config")
	}
	if rp.policyVersion != "1.0.0" {
		t.Errorf("policyVersion = %q, want 1.0.0", rp.policyVersion)
	}
}

func TestRoutingPolicy_Version(t *testing.T) {
	rp := NewRoutingPolicy(nil, "2.0.0")
	if rp.Version() != "2.0.0" {
		t.Errorf("Version() = %q, want 2.0.0", rp.Version())
	}
}

func TestRoutingPolicy_RoutingConfigAccessor(t *testing.T) {
	config := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(config, "1.0.0")
	if rp.RoutingConfigAccessor() != config {
		t.Error("RoutingConfigAccessor() returned different instance")
	}
}

// --------------------------------------------------------------------------
// Tests: Integration - multiple providers
// --------------------------------------------------------------------------

func TestRoutingPolicy_Integration_MultipleProviders(t *testing.T) {
	routingConfig := &RoutingConfig{
		ModelAliases: map[string]string{
			"fast":   "gpt-3.5-turbo",
			"smart":  "gpt-4",
			"gemini": "gemini-pro",
		},
		Models: map[string]ModelInfo{
			"gpt-4": {
				CanonicalName:    "gpt-4",
				Provider:         "openai",
				CostPer1KTokens:  0.03,
				AverageLatencyMs: 500,
				Capabilities:     []string{"chat", "function_calling", "vision"},
				MaxContextTokens: 8192,
			},
			"gpt-3.5-turbo": {
				CanonicalName:    "gpt-3.5-turbo",
				Provider:         "openai",
				CostPer1KTokens:  0.0005,
				AverageLatencyMs: 200,
				Capabilities:     []string{"chat", "function_calling"},
				MaxContextTokens: 16385,
			},
			"gemini-pro": {
				CanonicalName:    "gemini-pro",
				Provider:         "google",
				CostPer1KTokens:  0.00025,
				AverageLatencyMs: 300,
				Capabilities:     []string{"chat", "function_calling"},
				MaxContextTokens: 32768,
			},
			"gemini-1.5-pro": {
				CanonicalName:    "gemini-1.5-pro",
				Provider:         "google",
				CostPer1KTokens:  0.00125,
				AverageLatencyMs: 350,
				Capabilities:     []string{"chat", "function_calling", "vision"},
				MaxContextTokens: 1048576,
			},
		},
		Providers: map[string]ProviderConfig{
			"openai": {
				Name:               "openai",
				Priority:           1,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     1.0,
				Models:             []string{"gpt-4", "gpt-3.5-turbo"},
			},
			"google": {
				Name:               "google",
				Priority:           2,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     1.0,
				Models:             []string{"gemini-pro", "gemini-1.5-pro"},
			},
			"azure": {
				Name:               "azure",
				Priority:           3,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     0.9,
				Models:             []string{"gpt-4", "gpt-3.5-turbo"},
			},
		},
		EquivalentModels: map[string][]ModelEquivalent{
			"gpt-4": {
				{Model: "gemini-1.5-pro", Provider: "google", SimilarityScore: 0.85},
			},
			"gpt-3.5-turbo": {
				{Model: "gemini-pro", Provider: "google", SimilarityScore: 0.80},
			},
		},
		DefaultStrategy: RoutingStrategyDirect,
		DefaultProvider:  "openai",
		DefaultModel:     "gpt-3.5-turbo",
	}

	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	t.Run("direct_openai", func(t *testing.T) {
		input := newTestInput("gpt-4", nil)
		decision, err := rp.Evaluate(context.Background(), input)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if decision.SelectedModel != "gpt-4" {
			t.Errorf("model = %q, want gpt-4", decision.SelectedModel)
		}
		if decision.SelectedProvider != "openai" {
			t.Errorf("provider = %q, want openai", decision.SelectedProvider)
		}
	})

	t.Run("direct_google", func(t *testing.T) {
		input := newTestInput("gemini-pro", nil)
		decision, err := rp.Evaluate(context.Background(), input)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if decision.SelectedModel != "gemini-pro" {
			t.Errorf("model = %q, want gemini-pro", decision.SelectedModel)
		}
		if decision.SelectedProvider != "google" {
			t.Errorf("provider = %q, want google", decision.SelectedProvider)
		}
	})

	t.Run("alias_fast", func(t *testing.T) {
		input := newTestInput("fast", nil)
		decision, err := rp.Evaluate(context.Background(), input)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if decision.SelectedModel != "gpt-3.5-turbo" {
			t.Errorf("model = %q, want gpt-3.5-turbo", decision.SelectedModel)
		}
	})

	t.Run("alias_gemini", func(t *testing.T) {
		input := newTestInput("gemini", nil)
		decision, err := rp.Evaluate(context.Background(), input)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if decision.SelectedModel != "gemini-pro" {
			t.Errorf("model = %q, want gemini-pro", decision.SelectedModel)
		}
		if decision.SelectedProvider != "google" {
			t.Errorf("provider = %q, want google", decision.SelectedProvider)
		}
	})

	t.Run("cost_optimized_via_metadata", func(t *testing.T) {
		cfg := policy.NewDefaultConfig("gpt-3.5-turbo", "openai")
		input := &policy.PolicyInput{
			Request: &request.NormalizedRequest{
				ID:    "req-cost-int",
				Model: "gpt-4",
				Metadata: map[string]string{
					"routing_strategy": "cost_optimized",
				},
			},
			CacheStatus: policy.CacheStatusMiss,
			Config:      cfg,
			Timestamp:   time.Now(),
		}

		decision, err := rp.Evaluate(context.Background(), input)
		if err != nil {
			t.Fatalf("error: %v", err)
		}

		// Azure has 0.9 cost multiplier, so it should be cheaper for gpt-4.
		if decision.SelectedProvider != "azure" {
			t.Errorf("provider = %q, want azure (cheapest for gpt-4)", decision.SelectedProvider)
		}
	})

	t.Run("unhealthy_openai_fallback", func(t *testing.T) {
		// Create a modified config with unhealthy openai.
		unhealthyConfig := &RoutingConfig{
			Models:           routingConfig.Models,
			Providers:        make(map[string]ProviderConfig),
			EquivalentModels: routingConfig.EquivalentModels,
			DefaultStrategy:  RoutingStrategyDirect,
			DefaultProvider:  "openai",
			DefaultModel:     "gpt-3.5-turbo",
		}
		for k, v := range routingConfig.Providers {
			unhealthyConfig.Providers[k] = v
		}
		openai := unhealthyConfig.Providers["openai"]
		openai.Healthy = false
		unhealthyConfig.Providers["openai"] = openai

		rpUnhealthy := NewRoutingPolicy(unhealthyConfig, "1.0.0")
		input := newTestInput("gpt-4", nil)

		decision, err := rpUnhealthy.Evaluate(context.Background(), input)
		if err != nil {
			t.Fatalf("error: %v", err)
		}

		if decision.SelectedProvider == "openai" {
			t.Error("should not route to unhealthy openai")
		}
		// Should fallback to azure (also has gpt-4) or google (equivalent).
		if decision.SelectedProvider != "azure" && decision.SelectedProvider != "google" {
			t.Errorf("provider = %q, want azure or google", decision.SelectedProvider)
		}
	})

	t.Run("all_decisions_valid", func(t *testing.T) {
		models := []string{"gpt-4", "gpt-3.5-turbo", "gemini-pro", "gemini-1.5-pro", "fast", "smart", "gemini"}
		for _, model := range models {
			input := newTestInput(model, nil)
			decision, err := rp.Evaluate(context.Background(), input)
			if err != nil {
				t.Fatalf("model %q: error: %v", model, err)
			}
			if err := decision.Validate(); err != nil {
				t.Errorf("model %q: invalid decision: %v", model, err)
			}
		}
	})
}

// --------------------------------------------------------------------------
// Tests: Cache status interaction
// --------------------------------------------------------------------------

func TestRoutingPolicy_CacheHitInteraction(t *testing.T) {
	routingConfig := NewDefaultRoutingConfig()
	rp := NewRoutingPolicy(routingConfig, "1.0.0")

	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-cache",
			Model: "gpt-4",
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := rp.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Cache should be enabled on hit.
	if !decision.UseExactCache {
		t.Error("UseExactCache should be true on cache hit")
	}

	// Model and provider should still be resolved.
	if decision.SelectedModel != "gpt-4" {
		t.Errorf("SelectedModel = %q, want gpt-4", decision.SelectedModel)
	}
	if decision.SelectedProvider != "openai" {
		t.Errorf("SelectedProvider = %q, want openai", decision.SelectedProvider)
	}
}
