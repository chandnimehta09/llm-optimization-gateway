package context

import (
	"fmt"
	"sort"
	"strings"
)

// InvalidationScope defines the scope of cache invalidation.
type InvalidationScope string

const (
	// ScopeFull means all cache entries matching the filter are invalidated.
	ScopeFull InvalidationScope = "full"
	// ScopePartial means only some cache entries are invalidated based on criteria.
	ScopePartial InvalidationScope = "partial"
	// ScopeNone means no invalidation is needed.
	ScopeNone InvalidationScope = "none"
)

// InvalidationDecision is the result of evaluating invalidation rules against changes.
type InvalidationDecision struct {
	// ShouldInvalidate is true if the cache should be invalidated.
	ShouldInvalidate bool `json:"should_invalidate"`
	// Scope indicates whether the invalidation is full or partial.
	Scope InvalidationScope `json:"scope"`
	// Reason is a human-readable explanation of the decision.
	Reason string `json:"reason"`
	// AffectedCategories lists the factor categories that triggered invalidation.
	AffectedCategories []FactorCategory `json:"affected_categories,omitempty"`
	// MatchedRules lists the names of rules that triggered the decision.
	MatchedRules []string `json:"matched_rules,omitempty"`
	// WeightedScore is the aggregate weighted score of all changes (0.0–1.0).
	WeightedScore float64 `json:"weighted_score"`
}

// InvalidationRule evaluates changed factors and returns an InvalidationDecision.
type InvalidationRule interface {
	// Name returns the rule's unique identifier.
	Name() string
	// Evaluate assesses the changed factors and returns a decision.
	Evaluate(changes []ChangedFactor) InvalidationDecision
}

// InvalidationThreshold is the minimum weighted score to trigger invalidation.
const InvalidationThreshold = 0.1

// RepositoryChangeRule invalidates all cache entries when repository state changes.
type RepositoryChangeRule struct{}

// Name returns the rule name.
func (r *RepositoryChangeRule) Name() string { return "repository_change" }

// Evaluate checks for repository changes and returns full invalidation.
func (r *RepositoryChangeRule) Evaluate(changes []ChangedFactor) InvalidationDecision {
	for _, c := range changes {
		if c.Category == FactorCategoryRepository {
			return InvalidationDecision{
				ShouldInvalidate:   true,
				Scope:              ScopeFull,
				Reason:             fmt.Sprintf("repository change detected: %s (%s)", c.Name, c.ChangeType),
				AffectedCategories: []FactorCategory{FactorCategoryRepository},
				MatchedRules:       []string{r.Name()},
				WeightedScore:      1.0,
			}
		}
	}
	return InvalidationDecision{Scope: ScopeNone}
}

// DependencyChangeRule invalidates cache entries when dependency lock files change.
type DependencyChangeRule struct{}

// Name returns the rule name.
func (r *DependencyChangeRule) Name() string { return "dependency_change" }

// Evaluate checks for dependency lock changes and returns partial invalidation.
func (r *DependencyChangeRule) Evaluate(changes []ChangedFactor) InvalidationDecision {
	var depChanges []string
	for _, c := range changes {
		if c.Category == FactorCategoryDependency {
			depChanges = append(depChanges, c.Name)
		}
	}
	if len(depChanges) == 0 {
		return InvalidationDecision{Scope: ScopeNone}
	}
	return InvalidationDecision{
		ShouldInvalidate:   true,
		Scope:              ScopePartial,
		Reason:             fmt.Sprintf("dependency lock changes detected: %s", strings.Join(depChanges, ", ")),
		AffectedCategories: []FactorCategory{FactorCategoryDependency},
		MatchedRules:       []string{r.Name()},
		WeightedScore:      float64(DefaultWeights[FactorCategoryDependency]),
	}
}

// ToolSchemaChangeRule invalidates cache entries that use tools with changed schemas.
type ToolSchemaChangeRule struct{}

// Name returns the rule name.
func (r *ToolSchemaChangeRule) Name() string { return "tool_schema_change" }

// Evaluate checks for tool schema changes and returns full invalidation for tool-using entries.
func (r *ToolSchemaChangeRule) Evaluate(changes []ChangedFactor) InvalidationDecision {
	var toolChanges []string
	for _, c := range changes {
		if c.Category == FactorCategoryTool && strings.HasPrefix(c.Name, "tool_schema.") {
			toolChanges = append(toolChanges, c.Name)
		}
	}
	if len(toolChanges) == 0 {
		return InvalidationDecision{Scope: ScopeNone}
	}
	return InvalidationDecision{
		ShouldInvalidate:   true,
		Scope:              ScopeFull,
		Reason:             fmt.Sprintf("tool schema changes detected: %s", strings.Join(toolChanges, ", ")),
		AffectedCategories: []FactorCategory{FactorCategoryTool},
		MatchedRules:       []string{r.Name()},
		WeightedScore:      float64(DefaultWeights[FactorCategoryTool]),
	}
}

// SystemPromptChangeRule invalidates cache entries when the system prompt changes.
type SystemPromptChangeRule struct{}

// Name returns the rule name.
func (r *SystemPromptChangeRule) Name() string { return "system_prompt_change" }

// Evaluate checks for system prompt changes and returns full invalidation.
func (r *SystemPromptChangeRule) Evaluate(changes []ChangedFactor) InvalidationDecision {
	for _, c := range changes {
		if c.Category == FactorCategoryPrompt && c.Name == "prompt.system_prompt_hash" {
			return InvalidationDecision{
				ShouldInvalidate:   true,
				Scope:              ScopeFull,
				Reason:             "system prompt changed",
				AffectedCategories: []FactorCategory{FactorCategoryPrompt},
				MatchedRules:       []string{r.Name()},
				WeightedScore:      float64(DefaultWeights[FactorCategoryPrompt]),
			}
		}
	}
	return InvalidationDecision{Scope: ScopeNone}
}

// ConfigurationChangeRule invalidates relevant cache entries when configuration changes.
type ConfigurationChangeRule struct{}

// Name returns the rule name.
func (r *ConfigurationChangeRule) Name() string { return "configuration_change" }

// Evaluate checks for custom factor changes (configuration) and returns partial invalidation.
func (r *ConfigurationChangeRule) Evaluate(changes []ChangedFactor) InvalidationDecision {
	var configChanges []string
	for _, c := range changes {
		if c.Category == FactorCategoryCustom {
			configChanges = append(configChanges, c.Name)
		}
	}
	if len(configChanges) == 0 {
		return InvalidationDecision{Scope: ScopeNone}
	}
	return InvalidationDecision{
		ShouldInvalidate:   true,
		Scope:              ScopePartial,
		Reason:             fmt.Sprintf("configuration changes detected: %s", strings.Join(configChanges, ", ")),
		AffectedCategories: []FactorCategory{FactorCategoryCustom},
		MatchedRules:       []string{r.Name()},
		WeightedScore:      float64(DefaultWeights[FactorCategoryCustom]),
	}
}

// WeightedChangeRule applies weighted scoring to determine if aggregate changes
// exceed the invalidation threshold.
type WeightedChangeRule struct {
	// Threshold is the minimum weighted score to trigger invalidation (0.0–1.0).
	Threshold float64
}

// Name returns the rule name.
func (r *WeightedChangeRule) Name() string { return "weighted_change" }

// Evaluate computes a weighted score from all changes and decides based on threshold.
func (r *WeightedChangeRule) Evaluate(changes []ChangedFactor) InvalidationDecision {
	if len(changes) == 0 {
		return InvalidationDecision{Scope: ScopeNone}
	}

	threshold := r.Threshold
	if threshold <= 0 {
		threshold = InvalidationThreshold
	}

	var totalWeight, changedWeight float64
	affectedCats := make(map[FactorCategory]bool)

	for _, c := range changes {
		changedWeight += float64(c.Weight)
		affectedCats[c.Category] = true
	}

	for _, w := range DefaultWeights {
		totalWeight += float64(w)
	}

	score := 0.0
	if totalWeight > 0 {
		score = changedWeight / totalWeight
	}

	categories := make([]FactorCategory, 0, len(affectedCats))
	for cat := range affectedCats {
		categories = append(categories, cat)
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i] < categories[j]
	})

	if score >= threshold {
		scope := ScopePartial
		if score >= 0.8 {
			scope = ScopeFull
		}
		return InvalidationDecision{
			ShouldInvalidate:   true,
			Scope:              scope,
			Reason:             fmt.Sprintf("weighted change score %.3f exceeds threshold %.3f", score, threshold),
			AffectedCategories: categories,
			MatchedRules:       []string{r.Name()},
			WeightedScore:      score,
		}
	}

	return InvalidationDecision{
		Scope:         ScopeNone,
		WeightedScore: score,
		Reason:        fmt.Sprintf("weighted change score %.3f below threshold %.3f", score, threshold),
	}
}

// CustomRule wraps a user-provided evaluation function as an InvalidationRule.
type CustomRule struct {
	// RuleName is the unique identifier for this custom rule.
	RuleName string
	// EvaluateFunc is the custom evaluation function.
	EvaluateFunc func(changes []ChangedFactor) InvalidationDecision
}

// Name returns the custom rule name.
func (r *CustomRule) Name() string { return r.RuleName }

// Evaluate delegates to the custom evaluation function.
func (r *CustomRule) Evaluate(changes []ChangedFactor) InvalidationDecision {
	return r.EvaluateFunc(changes)
}

// RuleEngine manages a set of invalidation rules and evaluates them against changes.
type RuleEngine struct {
	rules []InvalidationRule
}

// NewRuleEngine creates a rule engine with the default set of invalidation rules.
func NewRuleEngine() *RuleEngine {
	return &RuleEngine{
		rules: []InvalidationRule{
			&RepositoryChangeRule{},
			&DependencyChangeRule{},
			&ToolSchemaChangeRule{},
			&SystemPromptChangeRule{},
			&ConfigurationChangeRule{},
			&WeightedChangeRule{Threshold: InvalidationThreshold},
		},
	}
}

// NewEmptyRuleEngine creates a rule engine with no rules.
func NewEmptyRuleEngine() *RuleEngine {
	return &RuleEngine{}
}

// AddRule registers an additional invalidation rule.
func (e *RuleEngine) AddRule(rule InvalidationRule) {
	e.rules = append(e.rules, rule)
}

// RemoveRule removes a rule by name. Returns true if the rule was found and removed.
func (e *RuleEngine) RemoveRule(name string) bool {
	for i, r := range e.rules {
		if r.Name() == name {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			return true
		}
	}
	return false
}

// Rules returns the names of all registered rules.
func (e *RuleEngine) Rules() []string {
	names := make([]string, len(e.rules))
	for i, r := range e.rules {
		names[i] = r.Name()
	}
	return names
}

// Evaluate runs all registered rules against the given changes and returns
// the most severe invalidation decision. Full > Partial > None.
func (e *RuleEngine) Evaluate(changes []ChangedFactor) InvalidationDecision {
	if len(changes) == 0 {
		return InvalidationDecision{Scope: ScopeNone, Reason: "no changes detected"}
	}

	var bestDecision InvalidationDecision
	var allMatchedRules []string
	allAffectedCats := make(map[FactorCategory]bool)

	for _, rule := range e.rules {
		decision := rule.Evaluate(changes)
		if !decision.ShouldInvalidate {
			continue
		}

		allMatchedRules = append(allMatchedRules, decision.MatchedRules...)
		for _, cat := range decision.AffectedCategories {
			allAffectedCats[cat] = true
		}

		if !bestDecision.ShouldInvalidate || scopePriority(decision.Scope) > scopePriority(bestDecision.Scope) {
			bestDecision = decision
		}
		if decision.WeightedScore > bestDecision.WeightedScore {
			bestDecision.WeightedScore = decision.WeightedScore
		}
	}

	if !bestDecision.ShouldInvalidate {
		return InvalidationDecision{Scope: ScopeNone, Reason: "no rules triggered invalidation"}
	}

	bestDecision.MatchedRules = deduplicateStrings(allMatchedRules)
	categories := make([]FactorCategory, 0, len(allAffectedCats))
	for cat := range allAffectedCats {
		categories = append(categories, cat)
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i] < categories[j]
	})
	bestDecision.AffectedCategories = categories

	return bestDecision
}

func scopePriority(s InvalidationScope) int {
	switch s {
	case ScopeFull:
		return 2
	case ScopePartial:
		return 1
	default:
		return 0
	}
}

func deduplicateStrings(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	var result []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	sort.Strings(result)
	return result
}
