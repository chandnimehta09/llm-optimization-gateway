package policy

import "context"

// PolicyEngine defines the interface for making deterministic optimization
// decisions. Implementations evaluate the full request context—including
// cache status, deduplication stats, and provider health—to produce a
// PolicyDecision that guides the gateway's behaviour.
//
// Implementations must be safe for concurrent use and deterministic: the
// same PolicyInput must always produce the same PolicyDecision.
type PolicyEngine interface {
	// Evaluate inspects the given PolicyInput and returns a PolicyDecision
	// describing how the gateway should handle the request.
	Evaluate(ctx context.Context, input *PolicyInput) (*PolicyDecision, error)
}

// RuleEngine extends PolicyEngine with rule management capabilities.
// It allows inspecting and validating the rule set at runtime.
type RuleEngine interface {
	PolicyEngine

	// Rules returns the ordered list of rules currently loaded.
	Rules() []Rule

	// Version returns the version string of the loaded policy.
	Version() string

	// Validate checks the rule set for conflicts and errors without
	// evaluating against a specific input.
	Validate() []error
}
