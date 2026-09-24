package rules

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/llm-optimization-gateway/internal/policy"
)

// RoutingPolicy implements the policy.PolicyEngine interface and makes
// model/provider routing decisions based on request characteristics and
// routing configuration.
type RoutingPolicy struct {
	routingConfig *RoutingConfig
	policyVersion string
}

// NewRoutingPolicy creates a RoutingPolicy with the given routing configuration.
func NewRoutingPolicy(routingConfig *RoutingConfig, policyVersion string) *RoutingPolicy {
	if routingConfig == nil {
		routingConfig = NewDefaultRoutingConfig()
	}
	if policyVersion == "" {
		policyVersion = "1.0.0"
	}
	return &RoutingPolicy{
		routingConfig: routingConfig,
		policyVersion: policyVersion,
	}
}

// Evaluate inspects the given PolicyInput and returns a PolicyDecision with
// routing information populated. It resolves model aliases, selects providers,
// identifies fallbacks, and records audit trail entries.
func (rp *RoutingPolicy) Evaluate(_ context.Context, input *policy.PolicyInput) (*policy.PolicyDecision, error) {
	if input == nil {
		return nil, fmt.Errorf("routing policy: input must not be nil")
	}
	if input.Config == nil {
		return nil, fmt.Errorf("routing policy: input config must not be nil")
	}

	requestedModel := ""
	if input.Request != nil {
		requestedModel = input.Request.Model
	}

	decision := policy.NewDecision(
		input.Config.Defaults.DefaultModel,
		input.Config.Defaults.DefaultProvider,
		rp.policyVersion,
	)
	decision.EvaluatedAt = time.Now()

	// Apply cache/dedup defaults from config.
	decision.UseExactCache = input.CacheStatus == policy.CacheStatusHit && input.Config.Defaults.EnableCache
	decision.UsePrefixCache = input.Config.Defaults.EnablePrefixCache && input.CacheStatus != policy.CacheStatusHit
	decision.ApplyDeduplication = input.Config.Defaults.EnableDedup

	if requestedModel == "" {
		decision.Reasoning = "no model requested, using defaults"
		decision.AddAuditEntry("routing-default", "default routing", true,
			"use_defaults", "no model specified in request")
		decision.AppliedRules = append(decision.AppliedRules, "routing-default")
		return decision, nil
	}

	rd := rp.resolveRouting(requestedModel, input)

	decision.SelectedModel = rd.Model
	decision.SelectedProvider = rd.Provider
	decision.Reasoning = rd.Reasoning
	decision.Warnings = append(decision.Warnings, rd.Warnings...)

	// Record audit trail entries for each routing step.
	rp.recordAuditTrail(decision, rd, requestedModel)

	// Evaluate any explicit policy rules from the config.
	rp.evaluatePolicyRules(decision, input)

	return decision, nil
}

// resolveRouting performs the full routing resolution pipeline.
func (rp *RoutingPolicy) resolveRouting(requestedModel string, input *policy.PolicyInput) *RoutingDecision {
	rd := &RoutingDecision{
		OriginalModel: requestedModel,
		Strategy:      rp.routingConfig.DefaultStrategy,
	}

	// Step 1: Resolve alias.
	canonical, wasAlias := ResolveModelAlias(requestedModel, rp.routingConfig)
	rd.Model = canonical
	rd.AliasResolved = wasAlias

	if wasAlias {
		rd.Reasoning = fmt.Sprintf("alias %q resolved to %q", requestedModel, canonical)
	}

	// Step 2: Check deprecation.
	deprecated, replacement, depWarning := CheckDeprecation(canonical, rp.routingConfig)
	if deprecated {
		rd.Warnings = append(rd.Warnings, depWarning)
		rd.DeprecationWarning = depWarning
		if replacement != "" {
			rd.Model = replacement
			rd.Reasoning = appendReasoning(rd.Reasoning,
				fmt.Sprintf("model %q deprecated, using replacement %q", canonical, replacement))
		}
	}

	// Step 3: Determine routing strategy.
	strategy := rp.determineStrategy(input)
	rd.Strategy = strategy

	// Step 4: Select provider based on strategy.
	provider, found := rp.selectProviderByStrategy(rd.Model, strategy, input)
	if found {
		rd.Provider = provider
		rd.Reasoning = appendReasoning(rd.Reasoning,
			fmt.Sprintf("provider %q selected via %s strategy", provider, strategy))
	} else {
		// Fallback: try direct provider selection.
		provider, found = SelectProviderForModel(rd.Model, rp.routingConfig)
		if found {
			rd.Provider = provider
			rd.Reasoning = appendReasoning(rd.Reasoning,
				fmt.Sprintf("provider %q selected as primary for model %q", provider, rd.Model))
		} else {
			// Last resort: use defaults.
			rd.Provider = rp.routingConfig.DefaultProvider
			rd.Model = ensureModel(rd.Model, rp.routingConfig.DefaultModel)
			rd.Warnings = append(rd.Warnings,
				fmt.Sprintf("no provider found for model %q, using default provider %q",
					rd.Model, rp.routingConfig.DefaultProvider))
			rd.Reasoning = appendReasoning(rd.Reasoning, "using default provider (no match found)")
		}
	}

	// Step 5: Respect provider health.
	rd.Provider = rp.ensureHealthyProvider(rd.Model, rd.Provider, rd)

	// Step 6: Respect rate limits.
	rd.Provider = rp.ensureRateLimitCapacity(rd.Model, rd.Provider, rd)

	// Step 7: Select fallback providers.
	rd.FallbackProviders = SelectFallbackProviders(rd.Model, rd.Provider, rp.routingConfig)

	return rd
}

// determineStrategy determines the routing strategy based on input hints.
func (rp *RoutingPolicy) determineStrategy(input *policy.PolicyInput) RoutingStrategy {
	if input.Request != nil && input.Request.Metadata != nil {
		if strategy, ok := input.Request.Metadata["routing_strategy"]; ok {
			switch RoutingStrategy(strategy) {
			case RoutingStrategyCostOptimized, RoutingStrategyLatencyOptimized,
				RoutingStrategyCapabilityBased, RoutingStrategyDirect:
				return RoutingStrategy(strategy)
			}
		}
	}
	return rp.routingConfig.DefaultStrategy
}

// selectProviderByStrategy selects a provider using the specified strategy.
func (rp *RoutingPolicy) selectProviderByStrategy(model string, strategy RoutingStrategy, input *policy.PolicyInput) (string, bool) {
	switch strategy {
	case RoutingStrategyCostOptimized:
		return SelectCostOptimalProvider(model, rp.routingConfig)
	case RoutingStrategyLatencyOptimized:
		return SelectLatencyOptimalProvider(model, rp.routingConfig)
	case RoutingStrategyCapabilityBased:
		caps := rp.extractRequiredCapabilities(input)
		if len(caps) > 0 {
			return SelectProviderByCapabilities(model, caps, rp.routingConfig)
		}
		return SelectProviderForModel(model, rp.routingConfig)
	case RoutingStrategyDirect:
		return SelectProviderForModel(model, rp.routingConfig)
	default:
		return SelectProviderForModel(model, rp.routingConfig)
	}
}

// extractRequiredCapabilities extracts capability requirements from the request.
func (rp *RoutingPolicy) extractRequiredCapabilities(input *policy.PolicyInput) []string {
	var caps []string
	if input.Request == nil {
		return caps
	}
	// Tool usage implies function_calling capability.
	if len(input.Request.Tools) > 0 {
		caps = append(caps, "function_calling")
	}
	// Check metadata for explicit capability requirements.
	if input.Request.Metadata != nil {
		if reqCaps, ok := input.Request.Metadata["required_capabilities"]; ok {
			for _, cap := range strings.Split(reqCaps, ",") {
				cap = strings.TrimSpace(cap)
				if cap != "" {
					caps = append(caps, cap)
				}
			}
		}
	}
	return caps
}

// ensureHealthyProvider checks if the selected provider is healthy and
// switches to a fallback if not.
func (rp *RoutingPolicy) ensureHealthyProvider(model, provider string, rd *RoutingDecision) string {
	pc, ok := rp.routingConfig.Providers[provider]
	if !ok || pc.Healthy {
		return provider
	}

	// Provider is unhealthy — find an alternative.
	rd.Warnings = append(rd.Warnings,
		fmt.Sprintf("provider %q is unhealthy, selecting alternative", provider))

	for name, pc := range rp.routingConfig.Providers {
		if name == provider || !pc.Healthy {
			continue
		}
		for _, m := range pc.Models {
			if m == model {
				rd.Reasoning = appendReasoning(rd.Reasoning,
					fmt.Sprintf("switched from unhealthy %q to %q", provider, name))
				return name
			}
		}
	}

	// Check equivalent models as well.
	equivalents := FindEquivalentModels(model, rp.routingConfig)
	for _, eq := range equivalents {
		if pc, ok := rp.routingConfig.Providers[eq.Provider]; ok && pc.Healthy {
			rd.Model = eq.Model
			rd.Reasoning = appendReasoning(rd.Reasoning,
				fmt.Sprintf("switched to equivalent model %q on %q due to unhealthy provider %q",
					eq.Model, eq.Provider, provider))
			return eq.Provider
		}
	}

	return provider
}

// ensureRateLimitCapacity checks if the selected provider has rate limit
// capacity and switches to a fallback if not.
func (rp *RoutingPolicy) ensureRateLimitCapacity(model, provider string, rd *RoutingDecision) string {
	pc, ok := rp.routingConfig.Providers[provider]
	if !ok || pc.RateLimitRemaining > 0 {
		return provider
	}

	rd.Warnings = append(rd.Warnings,
		fmt.Sprintf("provider %q rate limited, selecting alternative", provider))

	for name, pc := range rp.routingConfig.Providers {
		if name == provider || !pc.Healthy || pc.RateLimitRemaining <= 0 {
			continue
		}
		for _, m := range pc.Models {
			if m == model {
				rd.Reasoning = appendReasoning(rd.Reasoning,
					fmt.Sprintf("switched from rate-limited %q to %q", provider, name))
				return name
			}
		}
	}

	return provider
}

// recordAuditTrail adds audit trail entries for the routing resolution steps.
func (rp *RoutingPolicy) recordAuditTrail(decision *policy.PolicyDecision, rd *RoutingDecision, requestedModel string) {
	if rd.AliasResolved {
		decision.AddAuditEntry("routing-alias", "alias resolution", true,
			"resolve_alias",
			fmt.Sprintf("resolved %q to %q", requestedModel, rd.Model))
		decision.AppliedRules = append(decision.AppliedRules, "routing-alias")
	}

	if rd.DeprecationWarning != "" {
		decision.AddAuditEntry("routing-deprecation", "deprecation check", true,
			"deprecation_warning", rd.DeprecationWarning)
		decision.AppliedRules = append(decision.AppliedRules, "routing-deprecation")
	}

	decision.AddAuditEntry("routing-provider", "provider selection", true,
		"select_provider",
		fmt.Sprintf("selected provider %q for model %q via %s", rd.Provider, rd.Model, rd.Strategy))
	decision.AppliedRules = append(decision.AppliedRules, "routing-provider")

	if len(rd.FallbackProviders) > 0 {
		decision.AddAuditEntry("routing-fallback", "fallback selection", true,
			"select_fallbacks",
			fmt.Sprintf("fallback providers: %v", rd.FallbackProviders))
	}
}

// evaluatePolicyRules evaluates explicit policy rules from the config that
// target routing.
func (rp *RoutingPolicy) evaluatePolicyRules(decision *policy.PolicyDecision, input *policy.PolicyInput) {
	for _, rule := range input.Config.EnabledRules() {
		matched := rp.matchConditions(rule.Conditions, input)

		switch rule.Target {
		case policy.RuleTargetRouting:
			decision.AddAuditEntry(rule.ID, rule.Name, matched,
				string(rule.Action), fmt.Sprintf("routing rule target=%s", rule.Target))
			if matched {
				decision.AppliedRules = append(decision.AppliedRules, rule.ID)
				if rule.Value != "" {
					decision.SelectedProvider = rule.Value
				}
			}
		case policy.RuleTargetCache:
			decision.AddAuditEntry(rule.ID, rule.Name, matched,
				string(rule.Action), fmt.Sprintf("cache rule target=%s", rule.Target))
			if matched {
				decision.AppliedRules = append(decision.AppliedRules, rule.ID)
				decision.UseExactCache = rule.Action == policy.RuleActionAllow
			}
		case policy.RuleTargetPrefixCache:
			decision.AddAuditEntry(rule.ID, rule.Name, matched,
				string(rule.Action), fmt.Sprintf("prefix_cache rule target=%s", rule.Target))
			if matched {
				decision.AppliedRules = append(decision.AppliedRules, rule.ID)
				decision.UsePrefixCache = rule.Action == policy.RuleActionAllow
			}
		case policy.RuleTargetDedup:
			decision.AddAuditEntry(rule.ID, rule.Name, matched,
				string(rule.Action), fmt.Sprintf("dedup rule target=%s", rule.Target))
			if matched {
				decision.AppliedRules = append(decision.AppliedRules, rule.ID)
				decision.ApplyDeduplication = rule.Action == policy.RuleActionAllow
			}
		}
	}
}

// matchConditions checks if all conditions match the input.
func (rp *RoutingPolicy) matchConditions(conditions []policy.Condition, input *policy.PolicyInput) bool {
	if len(conditions) == 0 {
		return true // No conditions = always matches.
	}
	for _, c := range conditions {
		if !rp.matchCondition(c, input) {
			return false
		}
	}
	return true
}

// matchCondition checks a single condition against the input.
func (rp *RoutingPolicy) matchCondition(c policy.Condition, input *policy.PolicyInput) bool {
	fieldVal := rp.extractFieldValue(c.Field, input)
	switch c.Operator {
	case policy.ConditionOpEquals:
		return fieldVal == c.Value
	case policy.ConditionOpNotEquals:
		return fieldVal != c.Value
	case policy.ConditionOpContains:
		return strings.Contains(fieldVal, c.Value)
	case policy.ConditionOpExists:
		return fieldVal != ""
	default:
		return false
	}
}

// extractFieldValue gets the value of a field from the input.
func (rp *RoutingPolicy) extractFieldValue(field string, input *policy.PolicyInput) string {
	switch field {
	case "cache_status":
		return string(input.CacheStatus)
	case "request.model":
		if input.Request != nil {
			return input.Request.Model
		}
	case "request.stream":
		if input.Request != nil {
			if input.Request.Stream {
				return "true"
			}
			return "false"
		}
	case "request.provider_hint":
		if input.Request != nil {
			return input.Request.ProviderHint
		}
	}
	return ""
}

// appendReasoning appends to a reasoning string with "; " separator.
func appendReasoning(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + "; " + addition
}

// ensureModel returns the model if non-empty, or the fallback.
func ensureModel(model, fallback string) string {
	if model != "" {
		return model
	}
	return fallback
}

// RoutingConfigAccessor returns the routing configuration.
func (rp *RoutingPolicy) RoutingConfigAccessor() *RoutingConfig {
	return rp.routingConfig
}

// Version returns the policy version.
func (rp *RoutingPolicy) Version() string {
	return rp.policyVersion
}
