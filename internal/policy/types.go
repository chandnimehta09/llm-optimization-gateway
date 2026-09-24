// Package policy provides the policy engine interface for making deterministic
// optimization decisions in the LLM optimization gateway. The policy engine
// evaluates request context, cache status, deduplication statistics, and
// provider capabilities to produce routing and optimization decisions.
package policy

import (
	"time"

	ctxidentity "github.com/llm-optimization-gateway/internal/context"
	"github.com/llm-optimization-gateway/internal/dedup"
	"github.com/llm-optimization-gateway/internal/request"
)

// CacheStatus represents the current cache state for a request.
type CacheStatus string

const (
	// CacheStatusHit indicates the request matched an existing cache entry.
	CacheStatusHit CacheStatus = "hit"
	// CacheStatusMiss indicates no cache entry was found.
	CacheStatusMiss CacheStatus = "miss"
	// CacheStatusBypass indicates caching was explicitly skipped.
	CacheStatusBypass CacheStatus = "bypass"
)

// ProviderCapability describes a specific capability of an LLM provider.
type ProviderCapability struct {
	// Name is the capability identifier (e.g., "streaming", "function_calling", "vision").
	Name string `json:"name"`
	// Supported indicates whether the provider supports this capability.
	Supported bool `json:"supported"`
	// Version is the version of the capability supported (if applicable).
	Version string `json:"version,omitempty"`
}

// ProviderInfo describes an available LLM provider and its capabilities.
type ProviderInfo struct {
	// Name is the provider identifier (e.g., "openai", "anthropic", "google").
	Name string `json:"name"`
	// Models lists the models available from this provider.
	Models []string `json:"models"`
	// Capabilities lists the provider's supported capabilities.
	Capabilities []ProviderCapability `json:"capabilities"`
	// Priority is the default routing priority (lower is higher priority).
	Priority int `json:"priority"`
	// Healthy indicates whether the provider is currently healthy.
	Healthy bool `json:"healthy"`
	// AverageLatencyMs is the recent average latency in milliseconds.
	AverageLatencyMs float64 `json:"average_latency_ms,omitempty"`
	// ErrorRate is the recent error rate (0.0–1.0).
	ErrorRate float64 `json:"error_rate,omitempty"`
}

// PolicyInput contains all information needed for the policy engine to
// make an optimization decision. It aggregates request data, context
// identity, cache status, deduplication statistics, and provider info.
type PolicyInput struct {
	// Request is the normalized, provider-agnostic request.
	Request *request.NormalizedRequest `json:"request"`
	// Identity is the computed context identity for the request.
	Identity *ctxidentity.ContextIdentity `json:"identity,omitempty"`
	// CacheStatus indicates the current cache state for this request.
	CacheStatus CacheStatus `json:"cache_status"`
	// CacheAge is the age of the cache entry (if cache hit). Zero if miss or bypass.
	CacheAge time.Duration `json:"cache_age,omitempty"`
	// DedupStats contains statistics from deduplication processing.
	DedupStats *dedup.DedupStats `json:"dedup_stats,omitempty"`
	// Providers lists available providers with their capabilities and health.
	Providers []ProviderInfo `json:"providers"`
	// Config is the active policy configuration.
	Config *PolicyConfig `json:"config"`
	// Timestamp is when the evaluation was requested.
	Timestamp time.Time `json:"timestamp"`
	// RequestMetadata holds additional metadata from the request.
	RequestMetadata map[string]string `json:"request_metadata,omitempty"`
}

// PolicyDecision is the output of a policy evaluation. It contains all
// optimization choices and provides explainability through reasoning and
// an audit trail.
type PolicyDecision struct {
	// UseExactCache indicates whether to serve the response from an exact cache match.
	UseExactCache bool `json:"use_exact_cache"`
	// UsePrefixCache indicates whether to use prefix-based cache matching.
	UsePrefixCache bool `json:"use_prefix_cache"`
	// ApplyDeduplication indicates whether to apply content deduplication.
	ApplyDeduplication bool `json:"apply_deduplication"`
	// SelectedModel is the model to use for this request.
	SelectedModel string `json:"selected_model"`
	// SelectedProvider is the provider to route this request to.
	SelectedProvider string `json:"selected_provider"`
	// Reasoning explains why this decision was made (for explainability).
	Reasoning string `json:"reasoning"`
	// Confidence is a score (0.0–1.0) indicating how confident the engine
	// is in this decision.
	Confidence float64 `json:"confidence"`
	// Warnings contains non-fatal issues encountered during evaluation.
	Warnings []string `json:"warnings,omitempty"`
	// AppliedRules lists the rule IDs that contributed to this decision.
	AppliedRules []string `json:"applied_rules,omitempty"`
	// AuditTrail records the sequence of evaluations that produced this decision.
	AuditTrail []AuditEntry `json:"audit_trail,omitempty"`
	// PolicyVersion is the version of the policy that produced this decision.
	PolicyVersion string `json:"policy_version"`
	// EvaluatedAt is the timestamp of when the decision was made.
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// AuditEntry records a single step in the decision evaluation process.
type AuditEntry struct {
	// RuleID identifies the rule that was evaluated.
	RuleID string `json:"rule_id"`
	// RuleName is the human-readable name of the rule.
	RuleName string `json:"rule_name"`
	// Matched indicates whether the rule's conditions were satisfied.
	Matched bool `json:"matched"`
	// Effect describes what the rule would do (or did) if matched.
	Effect string `json:"effect"`
	// Reason explains why the rule matched or did not match.
	Reason string `json:"reason"`
	// Timestamp is when this rule was evaluated.
	Timestamp time.Time `json:"timestamp"`
}
