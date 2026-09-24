package policy

import (
	"fmt"
	"time"
)

// Validate checks that the PolicyDecision is internally consistent.
// It returns an error describing the first inconsistency found, or nil.
func (d *PolicyDecision) Validate() error {
	if d.SelectedModel == "" {
		return fmt.Errorf("policy decision: selected_model must not be empty")
	}
	if d.SelectedProvider == "" {
		return fmt.Errorf("policy decision: selected_provider must not be empty")
	}
	if d.Confidence < 0 || d.Confidence > 1 {
		return fmt.Errorf("policy decision: confidence must be between 0.0 and 1.0, got %f", d.Confidence)
	}
	if d.PolicyVersion == "" {
		return fmt.Errorf("policy decision: policy_version must not be empty")
	}
	if d.EvaluatedAt.IsZero() {
		return fmt.Errorf("policy decision: evaluated_at must not be zero")
	}
	if d.UseExactCache && d.UsePrefixCache {
		return fmt.Errorf("policy decision: use_exact_cache and use_prefix_cache are mutually exclusive")
	}
	return nil
}

// HasWarnings returns true if the decision includes any warnings.
func (d *PolicyDecision) HasWarnings() bool {
	return len(d.Warnings) > 0
}

// AddWarning appends a warning message to the decision.
func (d *PolicyDecision) AddWarning(msg string) {
	d.Warnings = append(d.Warnings, msg)
}

// AddAuditEntry appends an audit trail entry for a rule evaluation.
func (d *PolicyDecision) AddAuditEntry(ruleID, ruleName string, matched bool, effect, reason string) {
	d.AuditTrail = append(d.AuditTrail, AuditEntry{
		RuleID:    ruleID,
		RuleName:  ruleName,
		Matched:   matched,
		Effect:    effect,
		Reason:    reason,
		Timestamp: time.Now(),
	})
}

// MatchedRuleIDs returns the IDs of all rules that matched during evaluation.
func (d *PolicyDecision) MatchedRuleIDs() []string {
	var ids []string
	for _, entry := range d.AuditTrail {
		if entry.Matched {
			ids = append(ids, entry.RuleID)
		}
	}
	return ids
}

// NewDecision creates a PolicyDecision with required fields pre-populated.
func NewDecision(model, provider, policyVersion string) *PolicyDecision {
	return &PolicyDecision{
		SelectedModel:    model,
		SelectedProvider: provider,
		PolicyVersion:    policyVersion,
		Confidence:       1.0,
		EvaluatedAt:      time.Now(),
	}
}
