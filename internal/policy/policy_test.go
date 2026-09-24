package policy

import (
	"context"
	"testing"
	"time"

	ctxidentity "github.com/llm-optimization-gateway/internal/context"
	"github.com/llm-optimization-gateway/internal/dedup"
	"github.com/llm-optimization-gateway/internal/request"
)

// --------------------------------------------------------------------------
// Compile-time interface satisfaction checks
// --------------------------------------------------------------------------

// staticEngine is a minimal PolicyEngine used for compile-time checks.
type staticEngine struct{}

func (s *staticEngine) Evaluate(_ context.Context, _ *PolicyInput) (*PolicyDecision, error) {
	return NewDecision("gpt-4", "openai", "1.0.0"), nil
}

// Compile-time assertion that *staticEngine satisfies PolicyEngine.
var _ PolicyEngine = (*staticEngine)(nil)

// staticRuleEngine extends staticEngine with RuleEngine methods.
type staticRuleEngine struct {
	staticEngine
}

func (s *staticRuleEngine) Rules() []Rule     { return nil }
func (s *staticRuleEngine) Version() string   { return "1.0.0" }
func (s *staticRuleEngine) Validate() []error { return nil }

// Compile-time assertion that *staticRuleEngine satisfies RuleEngine.
var _ RuleEngine = (*staticRuleEngine)(nil)

// --------------------------------------------------------------------------
// deterministic test engine — used to verify determinism acceptance criteria
// --------------------------------------------------------------------------

// deterministicEngine always returns the same decision for the same input.
type deterministicEngine struct {
	config *PolicyConfig
}

func newDeterministicEngine(config *PolicyConfig) *deterministicEngine {
	return &deterministicEngine{config: config}
}

func (e *deterministicEngine) Evaluate(_ context.Context, input *PolicyInput) (*PolicyDecision, error) {
	decision := NewDecision(
		e.config.Defaults.DefaultModel,
		e.config.Defaults.DefaultProvider,
		e.config.Version,
	)
	decision.UseExactCache = input.CacheStatus == CacheStatusHit
	decision.ApplyDeduplication = e.config.Defaults.EnableDedup
	decision.UsePrefixCache = e.config.Defaults.EnablePrefixCache
	decision.Reasoning = "deterministic evaluation based on defaults"
	decision.Confidence = 1.0

	// Evaluate enabled rules in priority order.
	for _, rule := range e.config.EnabledRules() {
		matched := evaluateConditions(rule.Conditions, input)
		decision.AddAuditEntry(rule.ID, rule.Name, matched, string(rule.Action), "")
		if matched {
			decision.AppliedRules = append(decision.AppliedRules, rule.ID)
			applyRule(decision, rule)
		}
	}

	return decision, nil
}

// evaluateConditions checks if all conditions match the input.
func evaluateConditions(conditions []Condition, input *PolicyInput) bool {
	for _, c := range conditions {
		if !evaluateCondition(c, input) {
			return false
		}
	}
	return true
}

// evaluateCondition checks a single condition against the input.
func evaluateCondition(c Condition, input *PolicyInput) bool {
	switch c.Field {
	case "cache_status":
		return matchString(string(input.CacheStatus), c)
	case "request.model":
		if input.Request != nil {
			return matchString(input.Request.Model, c)
		}
		return false
	case "request.stream":
		if input.Request != nil {
			val := "false"
			if input.Request.Stream {
				val = "true"
			}
			return matchString(val, c)
		}
		return false
	default:
		return false
	}
}

// matchString applies the condition operator to a string field value.
func matchString(fieldVal string, c Condition) bool {
	switch c.Operator {
	case ConditionOpEquals:
		return fieldVal == c.Value
	case ConditionOpNotEquals:
		return fieldVal != c.Value
	case ConditionOpContains:
		return len(fieldVal) > 0 && len(c.Value) > 0 && contains(fieldVal, c.Value)
	case ConditionOpExists:
		return fieldVal != ""
	default:
		return false
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// applyRule applies a matched rule's effect to the decision.
func applyRule(d *PolicyDecision, r Rule) {
	switch r.Target {
	case RuleTargetCache:
		d.UseExactCache = r.Action == RuleActionAllow
	case RuleTargetPrefixCache:
		d.UsePrefixCache = r.Action == RuleActionAllow
	case RuleTargetDedup:
		d.ApplyDeduplication = r.Action == RuleActionAllow
	case RuleTargetRouting:
		if r.Value != "" {
			d.SelectedProvider = r.Value
		}
	}
}

// --------------------------------------------------------------------------
// Tests: Interface contract
// --------------------------------------------------------------------------

func TestPolicyEngine_Evaluate(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	engine := newDeterministicEngine(cfg)
	var pe PolicyEngine = engine

	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-1",
			Model: "gpt-4",
		},
		CacheStatus: CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	decision, err := pe.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}
	if decision == nil {
		t.Fatal("Evaluate returned nil decision")
	}
	if decision.SelectedModel != "gpt-4" {
		t.Errorf("SelectedModel = %q, want %q", decision.SelectedModel, "gpt-4")
	}
	if decision.SelectedProvider != "openai" {
		t.Errorf("SelectedProvider = %q, want %q", decision.SelectedProvider, "openai")
	}
	if decision.PolicyVersion != "1.0.0" {
		t.Errorf("PolicyVersion = %q, want %q", decision.PolicyVersion, "1.0.0")
	}
}

func TestPolicyEngine_DeterministicDecision(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{
			ID:       "r1",
			Name:     "cache-hit-allow",
			Priority: 1,
			Conditions: []Condition{
				{Field: "cache_status", Operator: ConditionOpEquals, Value: "hit"},
			},
			Target:  RuleTargetCache,
			Action:  RuleActionAllow,
			Enabled: true,
		},
	}
	engine := newDeterministicEngine(cfg)

	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-det",
			Model: "gpt-4",
		},
		CacheStatus: CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	// Evaluate twice — decisions must be identical (determinism).
	d1, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("first Evaluate: %v", err)
	}
	d2, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("second Evaluate: %v", err)
	}

	if d1.UseExactCache != d2.UseExactCache {
		t.Error("UseExactCache is not deterministic")
	}
	if d1.UsePrefixCache != d2.UsePrefixCache {
		t.Error("UsePrefixCache is not deterministic")
	}
	if d1.ApplyDeduplication != d2.ApplyDeduplication {
		t.Error("ApplyDeduplication is not deterministic")
	}
	if d1.SelectedModel != d2.SelectedModel {
		t.Error("SelectedModel is not deterministic")
	}
	if d1.SelectedProvider != d2.SelectedProvider {
		t.Error("SelectedProvider is not deterministic")
	}
	if d1.Confidence != d2.Confidence {
		t.Error("Confidence is not deterministic")
	}
	if d1.PolicyVersion != d2.PolicyVersion {
		t.Error("PolicyVersion is not deterministic")
	}
	if len(d1.AppliedRules) != len(d2.AppliedRules) {
		t.Error("AppliedRules length is not deterministic")
	}
}

func TestPolicyEngine_DecisionIncludesAllOptimizationChoices(t *testing.T) {
	cfg := NewDefaultConfig("claude-3-opus", "anthropic")
	engine := newDeterministicEngine(cfg)

	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-all",
			Model: "claude-3-opus",
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "identity-hash-123",
			Version: "1.0.0",
		},
		CacheStatus: CacheStatusMiss,
		DedupStats: &dedup.DedupStats{
			OriginalCount:     10,
			DeduplicatedCount: 8,
			DuplicatesRemoved: 2,
			BytesSaved:        512,
		},
		Providers: []ProviderInfo{
			{
				Name:    "anthropic",
				Models:  []string{"claude-3-opus"},
				Healthy: true,
			},
		},
		Config:    cfg,
		Timestamp: time.Now(),
	}

	decision, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate returned error: %v", err)
	}

	// Decision must include all optimization choices.
	if decision.SelectedModel == "" {
		t.Error("SelectedModel must not be empty")
	}
	if decision.SelectedProvider == "" {
		t.Error("SelectedProvider must not be empty")
	}
	if decision.PolicyVersion == "" {
		t.Error("PolicyVersion must not be empty")
	}
	if decision.Reasoning == "" {
		t.Error("Reasoning must not be empty")
	}
	if decision.Confidence < 0 || decision.Confidence > 1 {
		t.Errorf("Confidence %f out of range [0,1]", decision.Confidence)
	}
	if decision.EvaluatedAt.IsZero() {
		t.Error("EvaluatedAt must not be zero")
	}

	// Validate the decision.
	if err := decision.Validate(); err != nil {
		t.Errorf("decision.Validate() = %v", err)
	}
}

// --------------------------------------------------------------------------
// Tests: Decision structure
// --------------------------------------------------------------------------

func TestNewDecision(t *testing.T) {
	d := NewDecision("gpt-4", "openai", "2.0.0")
	if d.SelectedModel != "gpt-4" {
		t.Errorf("SelectedModel = %q, want %q", d.SelectedModel, "gpt-4")
	}
	if d.SelectedProvider != "openai" {
		t.Errorf("SelectedProvider = %q, want %q", d.SelectedProvider, "openai")
	}
	if d.PolicyVersion != "2.0.0" {
		t.Errorf("PolicyVersion = %q, want %q", d.PolicyVersion, "2.0.0")
	}
	if d.Confidence != 1.0 {
		t.Errorf("Confidence = %f, want 1.0", d.Confidence)
	}
	if d.EvaluatedAt.IsZero() {
		t.Error("EvaluatedAt must not be zero")
	}
	if err := d.Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
	}
}

func TestDecision_Validate_Valid(t *testing.T) {
	d := NewDecision("gpt-4", "openai", "1.0.0")
	if err := d.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDecision_Validate_EmptyModel(t *testing.T) {
	d := NewDecision("", "openai", "1.0.0")
	if err := d.Validate(); err == nil {
		t.Error("expected error for empty model")
	}
}

func TestDecision_Validate_EmptyProvider(t *testing.T) {
	d := NewDecision("gpt-4", "", "1.0.0")
	if err := d.Validate(); err == nil {
		t.Error("expected error for empty provider")
	}
}

func TestDecision_Validate_InvalidConfidence(t *testing.T) {
	d := NewDecision("gpt-4", "openai", "1.0.0")
	d.Confidence = 1.5
	if err := d.Validate(); err == nil {
		t.Error("expected error for confidence > 1.0")
	}

	d.Confidence = -0.1
	if err := d.Validate(); err == nil {
		t.Error("expected error for negative confidence")
	}
}

func TestDecision_Validate_EmptyVersion(t *testing.T) {
	d := NewDecision("gpt-4", "openai", "")
	if err := d.Validate(); err == nil {
		t.Error("expected error for empty policy version")
	}
}

func TestDecision_Validate_ZeroEvaluatedAt(t *testing.T) {
	d := NewDecision("gpt-4", "openai", "1.0.0")
	d.EvaluatedAt = time.Time{}
	if err := d.Validate(); err == nil {
		t.Error("expected error for zero EvaluatedAt")
	}
}

func TestDecision_Validate_MutuallyExclusiveCache(t *testing.T) {
	d := NewDecision("gpt-4", "openai", "1.0.0")
	d.UseExactCache = true
	d.UsePrefixCache = true
	if err := d.Validate(); err == nil {
		t.Error("expected error for mutually exclusive cache flags")
	}
}

func TestDecision_Warnings(t *testing.T) {
	d := NewDecision("gpt-4", "openai", "1.0.0")
	if d.HasWarnings() {
		t.Error("new decision should have no warnings")
	}

	d.AddWarning("provider latency high")
	d.AddWarning("model deprecated")

	if !d.HasWarnings() {
		t.Error("HasWarnings should be true after adding warnings")
	}
	if len(d.Warnings) != 2 {
		t.Errorf("len(Warnings) = %d, want 2", len(d.Warnings))
	}
	if d.Warnings[0] != "provider latency high" {
		t.Errorf("Warnings[0] = %q, want %q", d.Warnings[0], "provider latency high")
	}
}

func TestDecision_AuditTrail(t *testing.T) {
	d := NewDecision("gpt-4", "openai", "1.0.0")

	d.AddAuditEntry("r1", "cache-hit", true, "allow", "cache is fresh")
	d.AddAuditEntry("r2", "dedup-check", false, "deny", "no duplicates")

	if len(d.AuditTrail) != 2 {
		t.Fatalf("len(AuditTrail) = %d, want 2", len(d.AuditTrail))
	}
	if d.AuditTrail[0].RuleID != "r1" {
		t.Errorf("AuditTrail[0].RuleID = %q, want %q", d.AuditTrail[0].RuleID, "r1")
	}
	if !d.AuditTrail[0].Matched {
		t.Error("AuditTrail[0].Matched should be true")
	}
	if d.AuditTrail[1].Matched {
		t.Error("AuditTrail[1].Matched should be false")
	}

	matched := d.MatchedRuleIDs()
	if len(matched) != 1 || matched[0] != "r1" {
		t.Errorf("MatchedRuleIDs() = %v, want [r1]", matched)
	}
}

// --------------------------------------------------------------------------
// Tests: PolicyConfig
// --------------------------------------------------------------------------

func TestNewDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	if cfg.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", cfg.Version, "1.0.0")
	}
	if cfg.Name != "default" {
		t.Errorf("Name = %q, want %q", cfg.Name, "default")
	}
	if !cfg.Defaults.EnableCache {
		t.Error("EnableCache should default to true")
	}
	if cfg.Defaults.EnablePrefixCache {
		t.Error("EnablePrefixCache should default to false")
	}
	if !cfg.Defaults.EnableDedup {
		t.Error("EnableDedup should default to true")
	}
	if cfg.Defaults.DefaultModel != "gpt-4" {
		t.Errorf("DefaultModel = %q, want %q", cfg.Defaults.DefaultModel, "gpt-4")
	}
	if cfg.Defaults.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want %q", cfg.Defaults.DefaultProvider, "openai")
	}
}

func TestPolicyConfig_Validate_Valid(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{
			ID:       "r1",
			Name:     "test-rule",
			Priority: 1,
			Target:   RuleTargetCache,
			Action:   RuleActionAllow,
			Enabled:  true,
		},
	}
	errs := cfg.Validate()
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestPolicyConfig_Validate_MissingFields(t *testing.T) {
	cfg := &PolicyConfig{}
	errs := cfg.Validate()
	// Should report missing version, name, default_model, default_provider.
	if len(errs) < 4 {
		t.Errorf("expected at least 4 errors, got %d: %v", len(errs), errs)
	}
}

func TestPolicyConfig_Validate_DuplicateRuleIDs(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{ID: "r1", Name: "a", Priority: 1, Target: RuleTargetCache, Action: RuleActionAllow, Enabled: true},
		{ID: "r1", Name: "b", Priority: 2, Target: RuleTargetDedup, Action: RuleActionDeny, Enabled: true},
	}
	errs := cfg.Validate()
	found := false
	for _, err := range errs {
		if err.Error() == `policy config: duplicate rule id "r1"` {
			found = true
		}
	}
	if !found {
		t.Errorf("expected duplicate rule id error, got %v", errs)
	}
}

func TestPolicyConfig_Validate_RuleMissingFields(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{Priority: 1, Enabled: true},
	}
	errs := cfg.Validate()
	// Missing id, name, target, action.
	if len(errs) < 4 {
		t.Errorf("expected at least 4 errors for rule missing fields, got %d: %v", len(errs), errs)
	}
}

func TestPolicyConfig_Validate_ConditionMissingFields(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{
			ID:         "r1",
			Name:       "test",
			Priority:   1,
			Target:     RuleTargetCache,
			Action:     RuleActionAllow,
			Enabled:    true,
			Conditions: []Condition{{Field: "", Operator: ""}},
		},
	}
	errs := cfg.Validate()
	condErrs := 0
	for _, err := range errs {
		if searchSubstring(err.Error(), "condition") {
			condErrs++
		}
	}
	if condErrs < 2 {
		t.Errorf("expected at least 2 condition errors, got %d: %v", condErrs, errs)
	}
}

func TestPolicyConfig_SortedRules(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{ID: "r3", Name: "low", Priority: 10, Target: RuleTargetCache, Action: RuleActionAllow, Enabled: true},
		{ID: "r1", Name: "high", Priority: 1, Target: RuleTargetCache, Action: RuleActionAllow, Enabled: true},
		{ID: "r2", Name: "medium", Priority: 5, Target: RuleTargetDedup, Action: RuleActionDeny, Enabled: true},
	}

	sorted := cfg.SortedRules()
	if len(sorted) != 3 {
		t.Fatalf("len(SortedRules) = %d, want 3", len(sorted))
	}
	if sorted[0].ID != "r1" {
		t.Errorf("sorted[0].ID = %q, want %q", sorted[0].ID, "r1")
	}
	if sorted[1].ID != "r2" {
		t.Errorf("sorted[1].ID = %q, want %q", sorted[1].ID, "r2")
	}
	if sorted[2].ID != "r3" {
		t.Errorf("sorted[2].ID = %q, want %q", sorted[2].ID, "r3")
	}
}

func TestPolicyConfig_EnabledRules(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{ID: "r1", Name: "on", Priority: 2, Target: RuleTargetCache, Action: RuleActionAllow, Enabled: true},
		{ID: "r2", Name: "off", Priority: 1, Target: RuleTargetDedup, Action: RuleActionDeny, Enabled: false},
		{ID: "r3", Name: "on2", Priority: 3, Target: RuleTargetRouting, Action: RuleActionAllow, Enabled: true},
	}

	enabled := cfg.EnabledRules()
	if len(enabled) != 2 {
		t.Fatalf("len(EnabledRules) = %d, want 2", len(enabled))
	}
	if enabled[0].ID != "r1" {
		t.Errorf("enabled[0].ID = %q, want %q", enabled[0].ID, "r1")
	}
	if enabled[1].ID != "r3" {
		t.Errorf("enabled[1].ID = %q, want %q", enabled[1].ID, "r3")
	}
}

// --------------------------------------------------------------------------
// Tests: Rule evaluation with deterministic engine
// --------------------------------------------------------------------------

func TestDeterministicEngine_CacheHit(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	engine := newDeterministicEngine(cfg)

	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-hit",
			Model: "gpt-4",
		},
		CacheStatus: CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	d, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !d.UseExactCache {
		t.Error("UseExactCache should be true for cache hit")
	}
}

func TestDeterministicEngine_CacheMiss(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	engine := newDeterministicEngine(cfg)

	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-miss",
			Model: "gpt-4",
		},
		CacheStatus: CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	d, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.UseExactCache {
		t.Error("UseExactCache should be false for cache miss")
	}
}

func TestDeterministicEngine_CacheBypass(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	engine := newDeterministicEngine(cfg)

	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-bypass",
			Model: "gpt-4",
		},
		CacheStatus: CacheStatusBypass,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	d, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if d.UseExactCache {
		t.Error("UseExactCache should be false for cache bypass")
	}
}

func TestDeterministicEngine_RulePriority(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{
			ID:       "r-low",
			Name:     "deny-cache",
			Priority: 10,
			Conditions: []Condition{
				{Field: "cache_status", Operator: ConditionOpEquals, Value: "hit"},
			},
			Target:  RuleTargetCache,
			Action:  RuleActionDeny,
			Enabled: true,
		},
		{
			ID:       "r-high",
			Name:     "allow-cache",
			Priority: 1,
			Conditions: []Condition{
				{Field: "cache_status", Operator: ConditionOpEquals, Value: "hit"},
			},
			Target:  RuleTargetCache,
			Action:  RuleActionAllow,
			Enabled: true,
		},
	}

	engine := newDeterministicEngine(cfg)
	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-priority",
			Model: "gpt-4",
		},
		CacheStatus: CacheStatusHit,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	d, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Lower-priority (higher number) rule runs last and "wins" because it overrides.
	// r-high (priority 1) runs first → sets UseExactCache=true
	// r-low (priority 10) runs last → sets UseExactCache=false
	if d.UseExactCache {
		t.Error("last matching rule (deny at priority 10) should override: UseExactCache should be false")
	}

	// Both rules should be in applied rules.
	if len(d.AppliedRules) != 2 {
		t.Errorf("len(AppliedRules) = %d, want 2", len(d.AppliedRules))
	}
}

func TestDeterministicEngine_DisabledRulesSkipped(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{
			ID:       "r-disabled",
			Name:     "disabled-rule",
			Priority: 1,
			Conditions: []Condition{
				{Field: "cache_status", Operator: ConditionOpEquals, Value: "miss"},
			},
			Target:  RuleTargetCache,
			Action:  RuleActionAllow,
			Enabled: false,
		},
	}

	engine := newDeterministicEngine(cfg)
	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-disabled",
			Model: "gpt-4",
		},
		CacheStatus: CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	d, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if len(d.AppliedRules) != 0 {
		t.Errorf("disabled rules should not be applied, got %v", d.AppliedRules)
	}
}

func TestDeterministicEngine_RoutingRule(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{
			ID:       "route-anthropic",
			Name:     "route-to-anthropic",
			Priority: 1,
			Conditions: []Condition{
				{Field: "request.model", Operator: ConditionOpContains, Value: "claude"},
			},
			Target:  RuleTargetRouting,
			Action:  RuleActionAllow,
			Value:   "anthropic",
			Enabled: true,
		},
	}

	engine := newDeterministicEngine(cfg)
	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-route",
			Model: "claude-3-opus",
		},
		CacheStatus: CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	d, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if d.SelectedProvider != "anthropic" {
		t.Errorf("SelectedProvider = %q, want %q", d.SelectedProvider, "anthropic")
	}
}

func TestDeterministicEngine_StreamingDenyDedup(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{
			ID:       "no-dedup-streaming",
			Name:     "disable-dedup-for-streaming",
			Priority: 1,
			Conditions: []Condition{
				{Field: "request.stream", Operator: ConditionOpEquals, Value: "true"},
			},
			Target:  RuleTargetDedup,
			Action:  RuleActionDeny,
			Enabled: true,
		},
	}

	engine := newDeterministicEngine(cfg)
	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:     "req-stream",
			Model:  "gpt-4",
			Stream: true,
		},
		CacheStatus: CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	d, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if d.ApplyDeduplication {
		t.Error("ApplyDeduplication should be false for streaming requests with deny rule")
	}
}

func TestDeterministicEngine_AuditTrailRecorded(t *testing.T) {
	cfg := NewDefaultConfig("gpt-4", "openai")
	cfg.Rules = []Rule{
		{
			ID:       "r1",
			Name:     "always-match",
			Priority: 1,
			Target:   RuleTargetCache,
			Action:   RuleActionAllow,
			Enabled:  true,
		},
		{
			ID:       "r2",
			Name:     "never-match",
			Priority: 2,
			Conditions: []Condition{
				{Field: "cache_status", Operator: ConditionOpEquals, Value: "nonexistent"},
			},
			Target:  RuleTargetDedup,
			Action:  RuleActionDeny,
			Enabled: true,
		},
	}

	engine := newDeterministicEngine(cfg)
	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:    "req-audit",
			Model: "gpt-4",
		},
		CacheStatus: CacheStatusMiss,
		Config:      cfg,
		Timestamp:   time.Now(),
	}

	d, err := engine.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if len(d.AuditTrail) != 2 {
		t.Fatalf("len(AuditTrail) = %d, want 2", len(d.AuditTrail))
	}

	// First rule should match (no conditions = always match).
	if !d.AuditTrail[0].Matched {
		t.Error("AuditTrail[0] should be matched (no conditions = always match)")
	}
	if d.AuditTrail[0].RuleID != "r1" {
		t.Errorf("AuditTrail[0].RuleID = %q, want %q", d.AuditTrail[0].RuleID, "r1")
	}

	// Second rule should not match.
	if d.AuditTrail[1].Matched {
		t.Error("AuditTrail[1] should not be matched")
	}
}

// --------------------------------------------------------------------------
// Tests: CacheStatus constants
// --------------------------------------------------------------------------

func TestCacheStatus_Values(t *testing.T) {
	if CacheStatusHit != "hit" {
		t.Errorf("CacheStatusHit = %q, want %q", CacheStatusHit, "hit")
	}
	if CacheStatusMiss != "miss" {
		t.Errorf("CacheStatusMiss = %q, want %q", CacheStatusMiss, "miss")
	}
	if CacheStatusBypass != "bypass" {
		t.Errorf("CacheStatusBypass = %q, want %q", CacheStatusBypass, "bypass")
	}
}

// --------------------------------------------------------------------------
// Tests: PolicyInput construction
// --------------------------------------------------------------------------

func TestPolicyInput_WithAllFields(t *testing.T) {
	temp := 0.7
	maxTokens := 2048
	cfg := NewDefaultConfig("gpt-4", "openai")

	input := &PolicyInput{
		Request: &request.NormalizedRequest{
			ID:          "req-full",
			Model:       "gpt-4",
			Temperature: &temp,
			MaxTokens:   &maxTokens,
			Stream:      false,
		},
		Identity: &ctxidentity.ContextIdentity{
			ID:      "ctx-identity-abc",
			Version: "1.0.0",
			Model: ctxidentity.ModelIdentity{
				Name:     "gpt-4",
				Provider: "openai",
			},
		},
		CacheStatus: CacheStatusMiss,
		CacheAge:    0,
		DedupStats: &dedup.DedupStats{
			OriginalCount:        15,
			DeduplicatedCount:    12,
			DuplicatesRemoved:    3,
			BytesSaved:           1024,
			EstimatedTokensSaved: 256,
			Strategy:             "exact",
		},
		Providers: []ProviderInfo{
			{
				Name:    "openai",
				Models:  []string{"gpt-4", "gpt-3.5-turbo"},
				Healthy: true,
				Capabilities: []ProviderCapability{
					{Name: "streaming", Supported: true},
					{Name: "function_calling", Supported: true},
				},
				Priority:         1,
				AverageLatencyMs: 250.5,
				ErrorRate:        0.01,
			},
		},
		Config:    cfg,
		Timestamp: time.Now(),
		RequestMetadata: map[string]string{
			"user_id": "user-123",
		},
	}

	// Verify all fields are set.
	if input.Request == nil {
		t.Error("Request must not be nil")
	}
	if input.Identity == nil {
		t.Error("Identity must not be nil")
	}
	if input.DedupStats == nil {
		t.Error("DedupStats must not be nil")
	}
	if len(input.Providers) != 1 {
		t.Errorf("len(Providers) = %d, want 1", len(input.Providers))
	}
	if input.Config == nil {
		t.Error("Config must not be nil")
	}
	if input.RequestMetadata["user_id"] != "user-123" {
		t.Errorf("RequestMetadata[user_id] = %q, want %q", input.RequestMetadata["user_id"], "user-123")
	}
}

// --------------------------------------------------------------------------
// Tests: ProviderInfo / ProviderCapability
// --------------------------------------------------------------------------

func TestProviderInfo(t *testing.T) {
	p := ProviderInfo{
		Name:             "openai",
		Models:           []string{"gpt-4", "gpt-4-turbo"},
		Healthy:          true,
		Priority:         1,
		AverageLatencyMs: 200.0,
		ErrorRate:        0.02,
		Capabilities: []ProviderCapability{
			{Name: "streaming", Supported: true, Version: "1.0"},
			{Name: "vision", Supported: false},
		},
	}

	if p.Name != "openai" {
		t.Errorf("Name = %q, want %q", p.Name, "openai")
	}
	if len(p.Models) != 2 {
		t.Errorf("len(Models) = %d, want 2", len(p.Models))
	}
	if len(p.Capabilities) != 2 {
		t.Errorf("len(Capabilities) = %d, want 2", len(p.Capabilities))
	}
	if !p.Capabilities[0].Supported {
		t.Error("streaming should be supported")
	}
	if p.Capabilities[1].Supported {
		t.Error("vision should not be supported")
	}
}

// --------------------------------------------------------------------------
// Tests: RuleAction and RuleTarget constants
// --------------------------------------------------------------------------

func TestRuleActionConstants(t *testing.T) {
	if RuleActionAllow != "allow" {
		t.Errorf("RuleActionAllow = %q", RuleActionAllow)
	}
	if RuleActionDeny != "deny" {
		t.Errorf("RuleActionDeny = %q", RuleActionDeny)
	}
	if RuleActionOverride != "override" {
		t.Errorf("RuleActionOverride = %q", RuleActionOverride)
	}
}

func TestRuleTargetConstants(t *testing.T) {
	if RuleTargetCache != "cache" {
		t.Errorf("RuleTargetCache = %q", RuleTargetCache)
	}
	if RuleTargetPrefixCache != "prefix_cache" {
		t.Errorf("RuleTargetPrefixCache = %q", RuleTargetPrefixCache)
	}
	if RuleTargetDedup != "dedup" {
		t.Errorf("RuleTargetDedup = %q", RuleTargetDedup)
	}
	if RuleTargetRouting != "routing" {
		t.Errorf("RuleTargetRouting = %q", RuleTargetRouting)
	}
}

func TestConditionOperatorConstants(t *testing.T) {
	if ConditionOpEquals != "equals" {
		t.Errorf("ConditionOpEquals = %q", ConditionOpEquals)
	}
	if ConditionOpNotEquals != "not_equals" {
		t.Errorf("ConditionOpNotEquals = %q", ConditionOpNotEquals)
	}
	if ConditionOpContains != "contains" {
		t.Errorf("ConditionOpContains = %q", ConditionOpContains)
	}
	if ConditionOpGreaterThan != "greater_than" {
		t.Errorf("ConditionOpGreaterThan = %q", ConditionOpGreaterThan)
	}
	if ConditionOpLessThan != "less_than" {
		t.Errorf("ConditionOpLessThan = %q", ConditionOpLessThan)
	}
	if ConditionOpExists != "exists" {
		t.Errorf("ConditionOpExists = %q", ConditionOpExists)
	}
}
