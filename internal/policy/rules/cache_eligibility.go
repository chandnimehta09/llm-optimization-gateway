// Package rules implements the policy rules for the LLM optimization gateway.
// It provides concrete rule implementations for cache eligibility, bypass
// detection, and staleness checking used by the policy engine.
package rules

import (
	"fmt"
	"strings"
	"time"

	ctxidentity "github.com/llm-optimization-gateway/internal/context"
	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
)

// nonDeterministicTools is the set of tool names considered non-deterministic.
// Requests containing calls to these tools should bypass the cache.
var nonDeterministicTools = map[string]bool{
	"get_current_time":    true,
	"get_current_date":    true,
	"random_number":       true,
	"generate_uuid":       true,
	"fetch_url":           true,
	"web_search":          true,
	"get_weather":         true,
	"get_stock_price":     true,
	"list_directory":      true,
	"read_file":           true,
	"execute_command":     true,
	"database_query":      true,
	"send_email":          true,
	"create_notification": true,
}

// CacheEligibilityResult holds the result of a cache eligibility evaluation.
type CacheEligibilityResult struct {
	// Eligible indicates whether the request is eligible for caching.
	Eligible bool
	// Reason explains why the request is or is not eligible.
	Reason string
	// BypassRule is the name of the bypass rule that triggered, if any.
	BypassRule string
	// CheckedRules lists all rules that were evaluated.
	CheckedRules []string
}

// CacheBypassReason is a typed constant for bypass reasons.
type CacheBypassReason string

const (
	// BypassReasonNone indicates no bypass is needed.
	BypassReasonNone CacheBypassReason = ""
	// BypassReasonStreaming indicates a streaming request.
	BypassReasonStreaming CacheBypassReason = "streaming request"
	// BypassReasonSensitiveData indicates sensitive data flags are present.
	BypassReasonSensitiveData CacheBypassReason = "request contains sensitive data"
	// BypassReasonProbeRequest indicates a probe request (max_tokens: 0).
	BypassReasonProbeRequest CacheBypassReason = "probe request with max_tokens=0"
	// BypassReasonNonDeterministicTools indicates non-deterministic tool calls.
	BypassReasonNonDeterministicTools CacheBypassReason = "request uses non-deterministic tools"
	// BypassReasonCacheDisabled indicates cache was explicitly disabled.
	BypassReasonCacheDisabled CacheBypassReason = "cache disabled by request"
)

// StalenessResult holds the result of a cache staleness check.
type StalenessResult struct {
	// Stale indicates whether the cache entry is stale.
	Stale bool
	// Reason explains why the entry is or is not stale.
	Reason string
	// Age is the age of the cache entry.
	Age time.Duration
	// TTL is the configured time-to-live.
	TTL time.Duration
}

// CostSavings holds estimated cost savings from a cache hit.
type CostSavings struct {
	// EstimatedInputTokens is the number of input tokens saved.
	EstimatedInputTokens int
	// EstimatedOutputTokens is the number of output tokens saved.
	EstimatedOutputTokens int
	// EstimatedCostUSD is the estimated dollar cost saved.
	EstimatedCostUSD float64
	// LatencySavedMs is the estimated latency saved in milliseconds.
	LatencySavedMs float64
}

// CheckCacheEligibility evaluates all cache eligibility rules against the
// given policy input. It returns a result indicating whether caching is
// allowed and the reasoning behind the decision.
func CheckCacheEligibility(input *policy.PolicyInput) CacheEligibilityResult {
	result := CacheEligibilityResult{
		Eligible: true,
		Reason:   "all eligibility checks passed",
	}

	// Rule 1: Request allows caching (no cache: false).
	result.CheckedRules = append(result.CheckedRules, "request_allows_caching")
	if !requestAllowsCaching(input) {
		result.Eligible = false
		result.Reason = string(BypassReasonCacheDisabled)
		result.BypassRule = "request_allows_caching"
		return result
	}

	// Rule 2: Check bypass rules (streaming, sensitive data, probes, non-deterministic tools).
	bypassResult := CheckCacheBypass(input)
	if bypassResult.Reason != BypassReasonNone {
		result.Eligible = false
		result.Reason = string(bypassResult.Reason)
		result.BypassRule = bypassResult.RuleName
		result.CheckedRules = append(result.CheckedRules, bypassResult.RuleName)
		return result
	}
	result.CheckedRules = append(result.CheckedRules, "bypass_rules")

	// Rule 3: Cache entry must exist.
	result.CheckedRules = append(result.CheckedRules, "cache_entry_exists")
	if input.CacheStatus != policy.CacheStatusHit {
		result.Eligible = false
		result.Reason = fmt.Sprintf("no cache entry (status: %s)", input.CacheStatus)
		result.BypassRule = "cache_entry_exists"
		return result
	}

	// Rule 4: Cache entry must not be expired.
	result.CheckedRules = append(result.CheckedRules, "cache_not_expired")
	if input.Config != nil {
		stalenessResult := CheckCacheStaleness(input.CacheAge, getCacheTTL(input.Config))
		if stalenessResult.Stale {
			result.Eligible = false
			result.Reason = stalenessResult.Reason
			result.BypassRule = "cache_not_expired"
			return result
		}
	}

	// Rule 5: Context identity matches.
	result.CheckedRules = append(result.CheckedRules, "context_identity_matches")
	if !contextIdentityMatches(input) {
		result.Eligible = false
		result.Reason = "context identity mismatch"
		result.BypassRule = "context_identity_matches"
		return result
	}

	// Rule 6: Model parameters are cache-compatible.
	result.CheckedRules = append(result.CheckedRules, "model_params_compatible")
	if !modelParametersCacheCompatible(input) {
		result.Eligible = false
		result.Reason = "model parameters not cache-compatible (high temperature)"
		result.BypassRule = "model_params_compatible"
		return result
	}

	return result
}

// CacheBypassResult holds the result of a cache bypass check.
type CacheBypassResult struct {
	// Bypass indicates whether the cache should be bypassed.
	Bypass bool
	// Reason is the typed bypass reason.
	Reason CacheBypassReason
	// RuleName is the name of the rule that triggered the bypass.
	RuleName string
}

// CheckCacheBypass evaluates bypass rules to determine if caching should
// be skipped entirely for this request.
func CheckCacheBypass(input *policy.PolicyInput) CacheBypassResult {
	if input.Request == nil {
		return CacheBypassResult{}
	}

	// Bypass rule 1: Streaming requests.
	if input.Request.Stream {
		return CacheBypassResult{
			Bypass:   true,
			Reason:   BypassReasonStreaming,
			RuleName: "streaming_bypass",
		}
	}

	// Bypass rule 2: Requests with sensitive data flags.
	if hasSensitiveDataFlag(input) {
		return CacheBypassResult{
			Bypass:   true,
			Reason:   BypassReasonSensitiveData,
			RuleName: "sensitive_data_bypass",
		}
	}

	// Bypass rule 3: Probe requests (max_tokens: 0).
	if input.Request.MaxTokens != nil && *input.Request.MaxTokens == 0 {
		return CacheBypassResult{
			Bypass:   true,
			Reason:   BypassReasonProbeRequest,
			RuleName: "probe_request_bypass",
		}
	}

	// Bypass rule 4: Non-deterministic tool calls.
	if hasNonDeterministicTools(input.Request) {
		return CacheBypassResult{
			Bypass:   true,
			Reason:   BypassReasonNonDeterministicTools,
			RuleName: "non_deterministic_tools_bypass",
		}
	}

	return CacheBypassResult{}
}

// CheckCacheStaleness determines if a cache entry is stale based on its age
// and the configured TTL. It also handles context-change-based invalidation.
func CheckCacheStaleness(cacheAge time.Duration, ttl time.Duration) StalenessResult {
	result := StalenessResult{
		Age: cacheAge,
		TTL: ttl,
	}

	if ttl <= 0 {
		// TTL of 0 or negative means cache never expires (no TTL configured).
		result.Stale = false
		result.Reason = "no TTL configured, cache entry is valid"
		return result
	}

	if cacheAge > ttl {
		result.Stale = true
		result.Reason = fmt.Sprintf("cache entry expired: age %s exceeds TTL %s", cacheAge.Round(time.Second), ttl.Round(time.Second))
		return result
	}

	result.Stale = false
	result.Reason = fmt.Sprintf("cache entry fresh: age %s within TTL %s", cacheAge.Round(time.Second), ttl.Round(time.Second))
	return result
}

// CheckContextChangeStaleness determines if a cache entry is stale based on
// context identity changes. If the cached identity differs from the current
// request identity, the cache is stale.
func CheckContextChangeStaleness(cachedIdentityID, currentIdentityID string) StalenessResult {
	if cachedIdentityID == "" || currentIdentityID == "" {
		return StalenessResult{
			Stale:  false,
			Reason: "identity comparison skipped (missing identity)",
		}
	}

	if cachedIdentityID != currentIdentityID {
		return StalenessResult{
			Stale:  true,
			Reason: fmt.Sprintf("context changed: cached identity %s differs from current %s", truncateID(cachedIdentityID), truncateID(currentIdentityID)),
		}
	}

	return StalenessResult{
		Stale:  false,
		Reason: "context identity unchanged",
	}
}

// CalculateCostSavings estimates the cost savings from serving a cache hit
// instead of making an LLM API call.
func CalculateCostSavings(input *policy.PolicyInput) CostSavings {
	savings := CostSavings{}

	if input.Request == nil {
		return savings
	}

	// Estimate input tokens from message content (approx 4 chars per token).
	var totalChars int
	for _, msg := range input.Request.Messages {
		totalChars += len(msg.Content)
		for _, part := range msg.Parts {
			totalChars += len(part.Text)
		}
	}
	savings.EstimatedInputTokens = totalChars / 4
	if savings.EstimatedInputTokens < 1 && totalChars > 0 {
		savings.EstimatedInputTokens = 1
	}

	// Estimate output tokens from max_tokens or a default.
	if input.Request.MaxTokens != nil && *input.Request.MaxTokens > 0 {
		savings.EstimatedOutputTokens = *input.Request.MaxTokens / 2 // Assume half of max is used on average.
	} else {
		savings.EstimatedOutputTokens = 256 // Default estimate.
	}

	// Estimate cost based on model pricing (simplified).
	inputCostPer1K, outputCostPer1K := getModelPricing(input.Request.Model)
	savings.EstimatedCostUSD = (float64(savings.EstimatedInputTokens)/1000)*inputCostPer1K +
		(float64(savings.EstimatedOutputTokens)/1000)*outputCostPer1K

	// Estimate latency savings from average provider latency.
	if len(input.Providers) > 0 {
		for _, p := range input.Providers {
			if p.AverageLatencyMs > 0 {
				savings.LatencySavedMs = p.AverageLatencyMs
				break
			}
		}
	}
	if savings.LatencySavedMs == 0 {
		savings.LatencySavedMs = 500.0 // Default estimate.
	}

	return savings
}

// requestAllowsCaching checks if the request explicitly disables caching.
func requestAllowsCaching(input *policy.PolicyInput) bool {
	if input.Request == nil {
		return true
	}
	// Check metadata for cache: false directive.
	if input.Request.Metadata != nil {
		if val, ok := input.Request.Metadata["cache"]; ok {
			return val != "false" && val != "disabled" && val != "off" && val != "no"
		}
	}
	// Check request metadata on the policy input.
	if input.RequestMetadata != nil {
		if val, ok := input.RequestMetadata["cache"]; ok {
			return val != "false" && val != "disabled" && val != "off" && val != "no"
		}
	}
	return true
}

// hasSensitiveDataFlag checks if the request has sensitive data flags.
func hasSensitiveDataFlag(input *policy.PolicyInput) bool {
	if input.Request != nil && input.Request.Metadata != nil {
		if val, ok := input.Request.Metadata["sensitive"]; ok {
			return val == "true" || val == "yes" || val == "1"
		}
		if val, ok := input.Request.Metadata["pii"]; ok {
			return val == "true" || val == "yes" || val == "1"
		}
	}
	if input.RequestMetadata != nil {
		if val, ok := input.RequestMetadata["sensitive"]; ok {
			return val == "true" || val == "yes" || val == "1"
		}
		if val, ok := input.RequestMetadata["pii"]; ok {
			return val == "true" || val == "yes" || val == "1"
		}
	}
	return false
}

// hasNonDeterministicTools checks if the request uses non-deterministic tools.
func hasNonDeterministicTools(req *request.NormalizedRequest) bool {
	if req == nil {
		return false
	}
	for _, tool := range req.Tools {
		if nonDeterministicTools[tool.Function.Name] {
			return true
		}
	}
	// Also check tool calls in messages.
	for _, msg := range req.Messages {
		for _, tc := range msg.ToolCalls {
			if nonDeterministicTools[tc.Function.Name] {
				return true
			}
		}
	}
	return false
}

// contextIdentityMatches checks if the context identity is present and valid.
// When an identity is provided, it must have a non-empty ID, indicating a
// valid computed fingerprint.
func contextIdentityMatches(input *policy.PolicyInput) bool {
	// If no identity is provided, we skip this check (allow by default).
	if input.Identity == nil {
		return true
	}
	// Identity must have a computed ID.
	return input.Identity.ID != ""
}

// modelParametersCacheCompatible checks if the model parameters allow caching.
// Very high temperature values (>1.5) produce non-deterministic output and
// should not be cached.
func modelParametersCacheCompatible(input *policy.PolicyInput) bool {
	if input.Request == nil {
		return true
	}
	// High temperature reduces determinism significantly.
	if input.Request.Temperature != nil && *input.Request.Temperature > 1.5 {
		return false
	}
	return true
}

// getCacheTTL extracts the cache TTL from the policy config.
// It looks for a TTL value in the config rules or returns a default.
func getCacheTTL(config *policy.PolicyConfig) time.Duration {
	if config == nil {
		return 0
	}
	// Look for a cache TTL rule.
	for _, rule := range config.Rules {
		if rule.Target == policy.RuleTargetCache && rule.Value != "" {
			if d, err := time.ParseDuration(rule.Value); err == nil {
				return d
			}
		}
	}
	return 0 // No TTL configured = no expiration.
}

// getModelPricing returns simplified per-1K-token pricing for a model.
// Returns (inputCostPer1K, outputCostPer1K) in USD.
func getModelPricing(model string) (float64, float64) {
	model = strings.ToLower(model)
	switch {
	case strings.Contains(model, "gpt-4"):
		return 0.03, 0.06
	case strings.Contains(model, "gpt-3.5"):
		return 0.0005, 0.0015
	case strings.Contains(model, "claude-3-opus"):
		return 0.015, 0.075
	case strings.Contains(model, "claude-3-sonnet"):
		return 0.003, 0.015
	case strings.Contains(model, "claude-3-haiku"):
		return 0.00025, 0.00125
	case strings.Contains(model, "gemini"):
		return 0.0005, 0.0015
	default:
		return 0.01, 0.03 // Conservative default.
	}
}

// truncateID truncates a long identity hash for display purposes.
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12] + "..."
	}
	return id
}

// IsNonDeterministicTool checks whether a tool name is in the non-deterministic set.
func IsNonDeterministicTool(name string) bool {
	return nonDeterministicTools[name]
}

// RegisterNonDeterministicTool adds a tool name to the non-deterministic set.
func RegisterNonDeterministicTool(name string) {
	nonDeterministicTools[name] = true
}

// Ensure ContextIdentity is used (avoid import cycle concerns).
var _ = (*ctxidentity.ContextIdentity)(nil)
