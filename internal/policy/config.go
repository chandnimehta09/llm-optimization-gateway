package policy

import (
	"fmt"
	"sort"
	"time"
)

// RuleAction defines the action a rule takes when its conditions match.
type RuleAction string

const (
	// RuleActionAllow permits the optimization described by the rule.
	RuleActionAllow RuleAction = "allow"
	// RuleActionDeny prevents the optimization described by the rule.
	RuleActionDeny RuleAction = "deny"
	// RuleActionOverride overrides a previous rule's decision.
	RuleActionOverride RuleAction = "override"
)

// RuleTarget identifies which optimization dimension a rule affects.
type RuleTarget string

const (
	// RuleTargetCache targets the cache optimization decision.
	RuleTargetCache RuleTarget = "cache"
	// RuleTargetPrefixCache targets the prefix-cache optimization decision.
	RuleTargetPrefixCache RuleTarget = "prefix_cache"
	// RuleTargetDedup targets the deduplication optimization decision.
	RuleTargetDedup RuleTarget = "dedup"
	// RuleTargetRouting targets the model/provider routing decision.
	RuleTargetRouting RuleTarget = "routing"
)

// ConditionOperator defines how a condition value is compared.
type ConditionOperator string

const (
	// ConditionOpEquals checks for exact equality.
	ConditionOpEquals ConditionOperator = "equals"
	// ConditionOpNotEquals checks for inequality.
	ConditionOpNotEquals ConditionOperator = "not_equals"
	// ConditionOpContains checks if the field contains the value.
	ConditionOpContains ConditionOperator = "contains"
	// ConditionOpGreaterThan checks if the field is numerically greater.
	ConditionOpGreaterThan ConditionOperator = "greater_than"
	// ConditionOpLessThan checks if the field is numerically less.
	ConditionOpLessThan ConditionOperator = "less_than"
	// ConditionOpExists checks if the field is present and non-empty.
	ConditionOpExists ConditionOperator = "exists"
)

// Condition is a single predicate evaluated against a PolicyInput.
type Condition struct {
	// Field is the dot-separated path to the input field (e.g., "request.model", "cache_status").
	Field string `json:"field"`
	// Operator is the comparison operation.
	Operator ConditionOperator `json:"operator"`
	// Value is the expected value for comparison. Interpretation depends on Operator.
	Value string `json:"value"`
}

// Rule is a single policy rule that maps conditions to an action on a target.
// Rules are evaluated in priority order (lower number = higher priority).
type Rule struct {
	// ID is a unique identifier for the rule.
	ID string `json:"id"`
	// Name is a human-readable name for the rule.
	Name string `json:"name"`
	// Description explains what the rule does and why.
	Description string `json:"description,omitempty"`
	// Priority determines evaluation order. Lower values are evaluated first.
	// Rules with the same priority are evaluated in definition order.
	Priority int `json:"priority"`
	// Conditions are the predicates that must all be true for the rule to match.
	// An empty Conditions slice means the rule always matches (catch-all).
	Conditions []Condition `json:"conditions"`
	// Target identifies which optimization dimension this rule affects.
	Target RuleTarget `json:"target"`
	// Action is what to do when conditions match.
	Action RuleAction `json:"action"`
	// Value is an optional value associated with the action (e.g., a model name for routing).
	Value string `json:"value,omitempty"`
	// Enabled controls whether the rule is active.
	Enabled bool `json:"enabled"`
}

// PolicyConfig holds the complete policy configuration including rules,
// version metadata, and default behaviour.
type PolicyConfig struct {
	// Version is the semantic version of this policy configuration.
	Version string `json:"version"`
	// Name is the policy configuration name.
	Name string `json:"name"`
	// Description is a human-readable description of the policy.
	Description string `json:"description,omitempty"`
	// Rules is the set of policy rules.
	Rules []Rule `json:"rules"`
	// Defaults contains the default optimization decisions when no rules match.
	Defaults DefaultDecisions `json:"defaults"`
	// CreatedAt is when this policy version was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when this policy version was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultDecisions specifies the default optimization behaviour when no
// matching rules are found.
type DefaultDecisions struct {
	// EnableCache is the default cache decision.
	EnableCache bool `json:"enable_cache"`
	// EnablePrefixCache is the default prefix-cache decision.
	EnablePrefixCache bool `json:"enable_prefix_cache"`
	// EnableDedup is the default deduplication decision.
	EnableDedup bool `json:"enable_dedup"`
	// DefaultModel is the fallback model when no routing rule matches.
	DefaultModel string `json:"default_model"`
	// DefaultProvider is the fallback provider when no routing rule matches.
	DefaultProvider string `json:"default_provider"`
}

// SortedRules returns the rules sorted by priority (ascending).
// Rules with equal priority retain their original order.
func (c *PolicyConfig) SortedRules() []Rule {
	sorted := make([]Rule, len(c.Rules))
	copy(sorted, c.Rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	return sorted
}

// EnabledRules returns only the rules that are enabled, sorted by priority.
func (c *PolicyConfig) EnabledRules() []Rule {
	var enabled []Rule
	for _, r := range c.Rules {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	sort.SliceStable(enabled, func(i, j int) bool {
		return enabled[i].Priority < enabled[j].Priority
	})
	return enabled
}

// Validate checks the policy configuration for structural errors.
func (c *PolicyConfig) Validate() []error {
	var errs []error

	if c.Version == "" {
		errs = append(errs, fmt.Errorf("policy config: version must not be empty"))
	}
	if c.Name == "" {
		errs = append(errs, fmt.Errorf("policy config: name must not be empty"))
	}
	if c.Defaults.DefaultModel == "" {
		errs = append(errs, fmt.Errorf("policy config: defaults.default_model must not be empty"))
	}
	if c.Defaults.DefaultProvider == "" {
		errs = append(errs, fmt.Errorf("policy config: defaults.default_provider must not be empty"))
	}

	ruleIDs := make(map[string]bool)
	for i, r := range c.Rules {
		if r.ID == "" {
			errs = append(errs, fmt.Errorf("policy config: rule[%d] must have an id", i))
		} else if ruleIDs[r.ID] {
			errs = append(errs, fmt.Errorf("policy config: duplicate rule id %q", r.ID))
		} else {
			ruleIDs[r.ID] = true
		}
		if r.Name == "" {
			errs = append(errs, fmt.Errorf("policy config: rule[%d] (%s) must have a name", i, r.ID))
		}
		if r.Target == "" {
			errs = append(errs, fmt.Errorf("policy config: rule[%d] (%s) must have a target", i, r.ID))
		}
		if r.Action == "" {
			errs = append(errs, fmt.Errorf("policy config: rule[%d] (%s) must have an action", i, r.ID))
		}
		for j, cond := range r.Conditions {
			if cond.Field == "" {
				errs = append(errs, fmt.Errorf("policy config: rule[%d] (%s) condition[%d] must have a field", i, r.ID, j))
			}
			if cond.Operator == "" {
				errs = append(errs, fmt.Errorf("policy config: rule[%d] (%s) condition[%d] must have an operator", i, r.ID, j))
			}
		}
	}

	return errs
}

// NewDefaultConfig creates a PolicyConfig with sensible defaults.
func NewDefaultConfig(defaultModel, defaultProvider string) *PolicyConfig {
	now := time.Now()
	return &PolicyConfig{
		Version:     "1.0.0",
		Name:        "default",
		Description: "Default policy configuration",
		Rules:       nil,
		Defaults: DefaultDecisions{
			EnableCache:       true,
			EnablePrefixCache: false,
			EnableDedup:       true,
			DefaultModel:      defaultModel,
			DefaultProvider:   defaultProvider,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
