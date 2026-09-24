package rules

import (
	"context"
	"testing"
	"time"

	ctxidentity "github.com/llm-optimization-gateway/internal/context"
	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
)

// --------------------------------------------------------------------------
// ExactCachePolicy construction tests
// --------------------------------------------------------------------------

func TestNewExactCachePolicy_Defaults(t *testing.T) {
	p := NewExactCachePolicy()
	if p.version != "1.0.0" {
		t.Errorf("version = %q, want %q", p.version, "1.0.0")
	}
	if p.defaultTTL != 0 {
		t.Errorf("defaultTTL = %v, want 0", p.defaultTTL)
	}
}

func TestNewExactCachePolicy_WithOptions(t *testing.T) {
	p := NewExactCachePolicy(
		WithVersion("2.0.0"),
		WithDefaultTTL(10*time.Minute),
	)
	if p.version != "2.0.0" {
		t.Errorf("version = %q, want %q", p.version, "2.0.0")
	}
	if p.defaultTTL != 10*time.Minute {
		t.Errorf("defaultTTL = %v, want 10m", p.defaultTTL)
	}
}

func TestExactCachePolicy_Version(t *testing.T) {
	p := NewExactCachePolicy(WithVersion("3.0.0"))
	if p.Version() != "3.0.0" {
		t.Errorf("Version() = %q, want %q", p.Version(), "3.0.0")
	}
}

// --------------------------------------------------------------------------
// Acceptance criteria tests
// --------------------------------------------------------------------------

func TestExactCachePolicy_CacheHitWithMatchingContext(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-hit",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "Hello, world!"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123def456",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    10 * time.Second,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Acceptance: decision.use_exact_cache = true
	if !decision.UseExactCache {
		t.Error("expected UseExactCache=true for cache hit with matching context")
	}

	// Acceptance: decision.reasoning explains why
	if decision.Reasoning == "" {
		t.Error("expected non-empty reasoning")
	}
	if decision.Reasoning == "" {
		t.Error("reasoning should explain the decision")
	}

	// Verify decision is valid.
	if err := decision.Validate(); err != nil {
		t.Errorf("decision.Validate() = %v", err)
	}
}

func TestExactCachePolicy_CacheDisabledByRequest(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:       "req-disabled",
			Model:    "gpt-4",
			Metadata: map[string]string{"cache": "false"},
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Acceptance: decision.use_exact_cache = false
	if decision.UseExactCache {
		t.Error("expected UseExactCache=false when cache disabled")
	}

	// Acceptance: decision.reasoning = "cache disabled by request"
	if decision.Reasoning != "cache disabled by request" {
		t.Errorf("reasoning = %q, want %q", decision.Reasoning, "cache disabled by request")
	}
}

// --------------------------------------------------------------------------
// Evaluate tests
// --------------------------------------------------------------------------

func TestExactCachePolicy_Evaluate_NilInput(t *testing.T) {
	p := NewExactCachePolicy()

	_, err := p.Evaluate(context.Background(), nil)
	if err == nil {
		t.Error("expected error for nil input")
	}
}

func TestExactCachePolicy_Evaluate_CacheMiss(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-miss",
			Model: "gpt-4",
		},
		CacheStatus: policy.CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected UseExactCache=false for cache miss")
	}
}

func TestExactCachePolicy_Evaluate_StreamingRequest(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:     "req-stream",
			Model:  "gpt-4",
			Stream: true,
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected UseExactCache=false for streaming request")
	}
	if decision.Reasoning != string(BypassReasonStreaming) {
		t.Errorf("reasoning = %q, want %q", decision.Reasoning, BypassReasonStreaming)
	}
}

func TestExactCachePolicy_Evaluate_ProbeRequest(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	zero := 0

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:        "req-probe",
			Model:     "gpt-4",
			MaxTokens: &zero,
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected UseExactCache=false for probe request")
	}
}

func TestExactCachePolicy_Evaluate_SensitiveData(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:       "req-sens",
			Model:    "gpt-4",
			Metadata: map[string]string{"sensitive": "true"},
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected UseExactCache=false for sensitive data")
	}
}

func TestExactCachePolicy_Evaluate_NonDeterministicTools(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-nd-tools",
			Model: "gpt-4",
			Tools: []request.Tool{
				{
					Type: "function",
					Function: request.FunctionDefinition{
						Name: "get_current_time",
					},
				},
			},
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected UseExactCache=false for non-deterministic tools")
	}
}

func TestExactCachePolicy_Evaluate_ExpiredByDefaultTTL(t *testing.T) {
	p := NewExactCachePolicy(WithDefaultTTL(5 * time.Minute))
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-expired-ttl",
			Model: "gpt-4",
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    10 * time.Minute, // Exceeds 5m default TTL.
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected UseExactCache=false for expired by default TTL")
	}
	if decision.Confidence != 0.9 {
		t.Errorf("confidence = %f, want 0.9", decision.Confidence)
	}
}

func TestExactCachePolicy_Evaluate_FreshWithDefaultTTL(t *testing.T) {
	p := NewExactCachePolicy(WithDefaultTTL(10 * time.Minute))
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-fresh-ttl",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "Hello"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    2 * time.Minute,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.UseExactCache {
		t.Errorf("expected UseExactCache=true for fresh entry, reasoning: %s", decision.Reasoning)
	}
}

func TestExactCachePolicy_Evaluate_ContextChangeInvalidation(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-ctx-change",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "test"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "new-identity-hash",
			Version: "1.0.0",
		},
		CacheStatus:     policy.CacheStatusHit,
		CacheAge:        30 * time.Second,
		Config:          cfg,
		RequestMetadata: map[string]string{"cached_identity_id": "old-identity-hash"},
		Timestamp:       time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected UseExactCache=false for context change")
	}
	if decision.Confidence != 0.95 {
		t.Errorf("confidence = %f, want 0.95", decision.Confidence)
	}
}

func TestExactCachePolicy_Evaluate_SameContextIdentity(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-same-ctx",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "Hello"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "same-identity-hash",
			Version: "1.0.0",
		},
		CacheStatus:     policy.CacheStatusHit,
		CacheAge:        30 * time.Second,
		Config:          cfg,
		RequestMetadata: map[string]string{"cached_identity_id": "same-identity-hash"},
		Timestamp:       time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.UseExactCache {
		t.Errorf("expected UseExactCache=true for matching context, reasoning: %s", decision.Reasoning)
	}
}

// --------------------------------------------------------------------------
// Decision structure tests
// --------------------------------------------------------------------------

func TestExactCachePolicy_DecisionFields(t *testing.T) {
	p := NewExactCachePolicy(WithVersion("2.0.0"))
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-fields",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "test"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    10 * time.Second,
		Config:      cfg,
		Providers: []policy.ProviderInfo{
			{Name: "openai", AverageLatencyMs: 200.0},
		},
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if decision.SelectedModel != "gpt-4" {
		t.Errorf("SelectedModel = %q, want %q", decision.SelectedModel, "gpt-4")
	}
	if decision.SelectedProvider != "openai" {
		t.Errorf("SelectedProvider = %q, want %q", decision.SelectedProvider, "openai")
	}
	if decision.PolicyVersion != "2.0.0" {
		t.Errorf("PolicyVersion = %q, want %q", decision.PolicyVersion, "2.0.0")
	}
	if decision.EvaluatedAt.IsZero() {
		t.Error("EvaluatedAt should not be zero")
	}
	if len(decision.AuditTrail) == 0 {
		t.Error("expected audit trail entries")
	}
	if len(decision.AppliedRules) == 0 {
		t.Error("expected applied rules for cache hit")
	}
}

func TestExactCachePolicy_DecisionValidation(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-valid",
			Model: "gpt-4",
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	if vErr := decision.Validate(); vErr != nil {
		t.Errorf("decision.Validate() = %v", vErr)
	}
}

func TestExactCachePolicy_HighTemperatureWarning(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	temp := 1.2

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:          "req-warn",
			Model:       "gpt-4",
			Temperature: &temp,
			Messages: []request.Message{
				{Role: "user", Content: "test"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Temperature 1.2 is compatible (< 1.5) but should trigger a warning.
	if !decision.UseExactCache {
		t.Error("expected UseExactCache=true for temperature 1.2 (compatible)")
	}
	if !decision.HasWarnings() {
		t.Error("expected warnings for high temperature > 1.0")
	}
}

func TestExactCachePolicy_NoWarningLowTemperature(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	temp := 0.5

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:          "req-no-warn",
			Model:       "gpt-4",
			Temperature: &temp,
			Messages: []request.Message{
				{Role: "user", Content: "test"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.HasWarnings() {
		t.Errorf("unexpected warnings for low temperature: %v", decision.Warnings)
	}
}

// --------------------------------------------------------------------------
// AuditTrail tests
// --------------------------------------------------------------------------

func TestExactCachePolicy_AuditTrailOnHit(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-audit-hit",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "test"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    10 * time.Second,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Should have at least eligibility check and cost savings entries.
	if len(decision.AuditTrail) < 2 {
		t.Errorf("expected at least 2 audit entries, got %d", len(decision.AuditTrail))
	}

	// First entry should be the eligibility check.
	if decision.AuditTrail[0].RuleID != "exact-cache-eligibility" {
		t.Errorf("first audit entry rule_id = %q, want %q", decision.AuditTrail[0].RuleID, "exact-cache-eligibility")
	}
}

func TestExactCachePolicy_AuditTrailOnMiss(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-audit-miss",
			Model: "gpt-4",
		},
		CacheStatus: policy.CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Should have at least the eligibility check entry.
	if len(decision.AuditTrail) < 1 {
		t.Error("expected at least 1 audit entry")
	}

	// Eligibility should not match (miss).
	if decision.AuditTrail[0].Matched {
		t.Error("eligibility audit entry should not match for cache miss")
	}
}

// --------------------------------------------------------------------------
// Determinism tests
// --------------------------------------------------------------------------

func TestExactCachePolicy_Deterministic(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("gpt-4", "openai")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-det",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "determinism test"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "abc123",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    10 * time.Second,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	d1, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("first Evaluate: %v", err)
	}
	d2, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("second Evaluate: %v", err)
	}

	if d1.UseExactCache != d2.UseExactCache {
		t.Error("UseExactCache not deterministic")
	}
	if d1.Reasoning != d2.Reasoning {
		t.Errorf("Reasoning not deterministic: %q vs %q", d1.Reasoning, d2.Reasoning)
	}
	if d1.Confidence != d2.Confidence {
		t.Error("Confidence not deterministic")
	}
	if d1.SelectedModel != d2.SelectedModel {
		t.Error("SelectedModel not deterministic")
	}
	if d1.SelectedProvider != d2.SelectedProvider {
		t.Error("SelectedProvider not deterministic")
	}
}

// --------------------------------------------------------------------------
// Integration test with policy engine interface
// --------------------------------------------------------------------------

func TestExactCachePolicy_ImplementsPolicyEngine(t *testing.T) {
	// ExactCachePolicy should satisfy the PolicyEngine interface (Evaluate method).
	p := NewExactCachePolicy()
	var _ policy.PolicyEngine = p
}

func TestExactCachePolicy_IntegrationWithPolicyConfig(t *testing.T) {
	p := NewExactCachePolicy(WithVersion("1.0.0"), WithDefaultTTL(5*time.Minute))

	cfg := policy.NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []policy.Rule{
		{
			ID:       "cache-ttl",
			Name:     "default-cache-ttl",
			Priority: 1,
			Target:   policy.RuleTargetCache,
			Action:   policy.RuleActionAllow,
			Value:    "10m",
			Enabled:  true,
		},
	}

	// Test 1: Cache hit within TTL.
	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-int-1",
			Model: "gpt-4",
			Messages: []request.Message{
				{Role: "user", Content: "integration test"},
			},
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "integration-id",
			Version: "1.0.0",
		},
		CacheStatus: policy.CacheStatusHit,
		CacheAge:    2 * time.Minute,
		Config:      cfg,
		Providers: []policy.ProviderInfo{
			{Name: "openai", Models: []string{"gpt-4"}, Healthy: true, AverageLatencyMs: 250},
		},
		Timestamp: time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if !decision.UseExactCache {
		t.Errorf("expected cache hit to be used, reasoning: %s", decision.Reasoning)
	}
	if err := decision.Validate(); err != nil {
		t.Errorf("decision validation failed: %v", err)
	}

	// Test 2: Cache miss.
	input.CacheStatus = policy.CacheStatusMiss
	decision, err = p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected cache miss to not use cache")
	}

	// Test 3: Streaming bypass.
	input.CacheStatus = policy.CacheStatusHit
	input.Request.Stream = true
	decision, err = p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected streaming to bypass cache")
	}
	input.Request.Stream = false

	// Test 4: Cache disabled by request.
	input.Request.Metadata = map[string]string{"cache": "false"}
	decision, err = p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.UseExactCache {
		t.Error("expected cache disabled by request")
	}
	if decision.Reasoning != "cache disabled by request" {
		t.Errorf("reasoning = %q, want %q", decision.Reasoning, "cache disabled by request")
	}
}

func TestExactCachePolicy_MissingModelUsesConfigDefault(t *testing.T) {
	p := NewExactCachePolicy()
	cfg := policy.NewDefaultConfig("default-model", "default-provider")

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID: "req-no-model",
		},
		CacheStatus: policy.CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.SelectedModel != "default-model" {
		t.Errorf("SelectedModel = %q, want %q", decision.SelectedModel, "default-model")
	}
	if decision.SelectedProvider != "default-provider" {
		t.Errorf("SelectedProvider = %q, want %q", decision.SelectedProvider, "default-provider")
	}
}

func TestExactCachePolicy_NoConfigUsesUnknown(t *testing.T) {
	p := NewExactCachePolicy()

	input := &policy.PolicyInput{
		Request: &request.NormalizedRequest{
			ID: "req-no-cfg",
		},
		CacheStatus: policy.CacheStatusMiss,
		Timestamp:   time.Now(),
	}

	decision, err := p.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision.SelectedProvider != "unknown" {
		t.Errorf("SelectedProvider = %q, want %q", decision.SelectedProvider, "unknown")
	}
}
