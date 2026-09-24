package rules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/llm-optimization-gateway/internal/policy"
)

// ExactCachePolicy implements the exact cache eligibility policy.
// It evaluates whether a cached response should be used for a given request
// based on eligibility rules, bypass conditions, and staleness checks.
type ExactCachePolicy struct {
	// version is the policy version string.
	version string
	// defaultTTL is the default cache TTL if none is configured.
	defaultTTL time.Duration
}

// ExactCachePolicyOption configures an ExactCachePolicy.
type ExactCachePolicyOption func(*ExactCachePolicy)

// WithVersion sets the policy version.
func WithVersion(version string) ExactCachePolicyOption {
	return func(p *ExactCachePolicy) {
		p.version = version
	}
}

// WithDefaultTTL sets the default cache TTL.
func WithDefaultTTL(ttl time.Duration) ExactCachePolicyOption {
	return func(p *ExactCachePolicy) {
		p.defaultTTL = ttl
	}
}

// NewExactCachePolicy creates a new ExactCachePolicy with the given options.
func NewExactCachePolicy(opts ...ExactCachePolicyOption) *ExactCachePolicy {
	p := &ExactCachePolicy{
		version:    "1.0.0",
		defaultTTL: 0,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// ExactCacheResult is the detailed result of an exact cache policy evaluation.
type ExactCacheResult struct {
	// UseCache indicates whether the cached response should be used.
	UseCache bool
	// Reasoning explains the decision in human-readable form.
	Reasoning string
	// Confidence is a score from 0.0 to 1.0.
	Confidence float64
	// Eligibility is the detailed eligibility result.
	Eligibility CacheEligibilityResult
	// Staleness is the staleness check result (if evaluated).
	Staleness *StalenessResult
	// CostSavings is the estimated cost savings (if cache hit).
	CostSavings *CostSavings
	// AuditEntries records all rule evaluations.
	AuditEntries []policy.AuditEntry
	// Warnings lists non-fatal issues.
	Warnings []string
}

// Evaluate runs the exact cache policy against the given input and returns
// a PolicyDecision. It implements the full evaluation pipeline:
// 1. Check cache eligibility rules
// 2. Check bypass rules
// 3. Check staleness (TTL and context-change)
// 4. Calculate cost savings
// 5. Build decision with reasoning
func (p *ExactCachePolicy) Evaluate(ctx context.Context, input *policy.PolicyInput) (*policy.PolicyDecision, error) {
	if input == nil {
		return nil, fmt.Errorf("exact cache policy: input must not be nil")
	}

	result := p.evaluate(input)
	return p.buildDecision(input, result), nil
}

// evaluate performs the core evaluation logic and returns a detailed result.
func (p *ExactCachePolicy) evaluate(input *policy.PolicyInput) ExactCacheResult {
	result := ExactCacheResult{
		Confidence: 1.0,
	}

	now := time.Now()

	// Step 1: Check cache eligibility (includes bypass rules).
	eligibility := CheckCacheEligibility(input)
	result.Eligibility = eligibility

	result.AuditEntries = append(result.AuditEntries, policy.AuditEntry{
		RuleID:    "exact-cache-eligibility",
		RuleName:  "Cache Eligibility Check",
		Matched:   eligibility.Eligible,
		Effect:    boolToEffect(eligibility.Eligible, "use_exact_cache"),
		Reason:    eligibility.Reason,
		Timestamp: now,
	})

	if !eligibility.Eligible {
		result.UseCache = false
		result.Reasoning = eligibility.Reason
		return result
	}

	// Step 2: Additional staleness check with default TTL.
	if p.defaultTTL > 0 && input.CacheAge > 0 {
		staleness := CheckCacheStaleness(input.CacheAge, p.defaultTTL)
		result.Staleness = &staleness

		result.AuditEntries = append(result.AuditEntries, policy.AuditEntry{
			RuleID:    "exact-cache-default-ttl",
			RuleName:  "Default TTL Staleness Check",
			Matched:   staleness.Stale,
			Effect:    "invalidate_cache_if_stale",
			Reason:    staleness.Reason,
			Timestamp: now,
		})

		if staleness.Stale {
			result.UseCache = false
			result.Reasoning = staleness.Reason
			result.Confidence = 0.9
			return result
		}
	}

	// Step 3: Context-change-based staleness check.
	if input.Identity != nil && input.RequestMetadata != nil {
		cachedIdentityID := input.RequestMetadata["cached_identity_id"]
		if cachedIdentityID != "" {
			contextStaleness := CheckContextChangeStaleness(cachedIdentityID, input.Identity.ID)
			result.AuditEntries = append(result.AuditEntries, policy.AuditEntry{
				RuleID:    "exact-cache-context-change",
				RuleName:  "Context Change Staleness Check",
				Matched:   contextStaleness.Stale,
				Effect:    "invalidate_cache_if_context_changed",
				Reason:    contextStaleness.Reason,
				Timestamp: now,
			})

			if contextStaleness.Stale {
				result.UseCache = false
				result.Reasoning = contextStaleness.Reason
				result.Confidence = 0.95
				return result
			}
		}
	}

	// Step 4: Calculate cost savings.
	savings := CalculateCostSavings(input)
	result.CostSavings = &savings

	result.AuditEntries = append(result.AuditEntries, policy.AuditEntry{
		RuleID:    "exact-cache-cost-savings",
		RuleName:  "Cost Savings Calculation",
		Matched:   true,
		Effect:    fmt.Sprintf("estimated_savings: $%.6f, latency: %.0fms", savings.EstimatedCostUSD, savings.LatencySavedMs),
		Reason:    fmt.Sprintf("input_tokens=%d, output_tokens=%d", savings.EstimatedInputTokens, savings.EstimatedOutputTokens),
		Timestamp: now,
	})

	// Step 5: All checks passed — use cache.
	result.UseCache = true
	result.Reasoning = buildCacheHitReasoning(input, &savings)

	// Add warnings for edge cases.
	if input.Request != nil && input.Request.Temperature != nil && *input.Request.Temperature > 1.0 {
		result.Warnings = append(result.Warnings, "temperature > 1.0 may produce varied outputs; cached response may not match expected variance")
	}

	return result
}

// buildDecision converts an ExactCacheResult into a PolicyDecision.
func (p *ExactCachePolicy) buildDecision(input *policy.PolicyInput, result ExactCacheResult) *policy.PolicyDecision {
	model := ""
	provider := ""

	if input.Request != nil {
		model = input.Request.Model
	}
	if input.Config != nil {
		if model == "" {
			model = input.Config.Defaults.DefaultModel
		}
		provider = input.Config.Defaults.DefaultProvider
	}
	if model == "" {
		model = "unknown"
	}
	if provider == "" {
		provider = "unknown"
	}

	decision := policy.NewDecision(model, provider, p.version)
	decision.UseExactCache = result.UseCache
	decision.Reasoning = result.Reasoning
	decision.Confidence = result.Confidence
	decision.AuditTrail = result.AuditEntries
	decision.Warnings = result.Warnings

	// Collect applied rule IDs from matched audit entries.
	for _, entry := range result.AuditEntries {
		if entry.Matched {
			decision.AppliedRules = append(decision.AppliedRules, entry.RuleID)
		}
	}

	return decision
}

// buildCacheHitReasoning constructs a human-readable reasoning string for
// why a cache hit was approved.
func buildCacheHitReasoning(input *policy.PolicyInput, savings *CostSavings) string {
	var parts []string
	parts = append(parts, "exact cache hit approved")

	if input.Identity != nil && input.Identity.ID != "" {
		parts = append(parts, fmt.Sprintf("context identity %s verified", truncateID(input.Identity.ID)))
	}

	if input.CacheAge > 0 {
		parts = append(parts, fmt.Sprintf("cache age %s", input.CacheAge.Round(time.Second)))
	}

	if savings != nil && savings.EstimatedCostUSD > 0 {
		parts = append(parts, fmt.Sprintf("estimated savings $%.6f", savings.EstimatedCostUSD))
	}

	return strings.Join(parts, "; ")
}

// boolToEffect converts a boolean to an effect description.
func boolToEffect(matched bool, target string) string {
	if matched {
		return "allow " + target
	}
	return "deny " + target
}

// Version returns the policy version.
func (p *ExactCachePolicy) Version() string {
	return p.version
}
