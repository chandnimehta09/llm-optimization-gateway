package context

import (
	"testing"
)

func TestRepositoryChangeRule_Evaluate(t *testing.T) {
	rule := &RepositoryChangeRule{}

	t.Run("no changes", func(t *testing.T) {
		decision := rule.Evaluate(nil)
		if decision.ShouldInvalidate {
			t.Error("expected no invalidation")
		}
	})

	t.Run("repository commit change triggers full invalidation", func(t *testing.T) {
		changes := []ChangedFactor{{
			Name: "repository.commit_hash", Category: FactorCategoryRepository,
			ChangeType: ChangeTypeModified, OldValue: "abc123", NewValue: "def456",
			Weight: DefaultWeights[FactorCategoryRepository],
		}}
		decision := rule.Evaluate(changes)
		if !decision.ShouldInvalidate {
			t.Error("expected invalidation")
		}
		if decision.Scope != ScopeFull {
			t.Errorf("expected full, got %s", decision.Scope)
		}
	})

	t.Run("non-repository changes do not trigger", func(t *testing.T) {
		changes := []ChangedFactor{{Name: "model.name", Category: FactorCategoryModel}}
		decision := rule.Evaluate(changes)
		if decision.ShouldInvalidate {
			t.Error("expected no invalidation for model change")
		}
	})
}

func TestDependencyChangeRule_Evaluate(t *testing.T) {
	rule := &DependencyChangeRule{}

	t.Run("dependency change triggers partial invalidation", func(t *testing.T) {
		changes := []ChangedFactor{{
			Name: "dependency.go.sum", Category: FactorCategoryDependency,
			ChangeType: ChangeTypeModified, Weight: DefaultWeights[FactorCategoryDependency],
		}}
		decision := rule.Evaluate(changes)
		if !decision.ShouldInvalidate {
			t.Error("expected invalidation")
		}
		if decision.Scope != ScopePartial {
			t.Errorf("expected partial, got %s", decision.Scope)
		}
	})
}

func TestToolSchemaChangeRule_Evaluate(t *testing.T) {
	rule := &ToolSchemaChangeRule{}

	t.Run("tool schema change triggers full invalidation", func(t *testing.T) {
		changes := []ChangedFactor{{
			Name: "tool_schema.search", Category: FactorCategoryTool,
			ChangeType: ChangeTypeModified, OldValue: "v1", NewValue: "v2",
			Weight: DefaultWeights[FactorCategoryTool],
		}}
		decision := rule.Evaluate(changes)
		if !decision.ShouldInvalidate {
			t.Error("expected invalidation")
		}
		if decision.Scope != ScopeFull {
			t.Errorf("expected full, got %s", decision.Scope)
		}
	})

	t.Run("tool output change does not trigger", func(t *testing.T) {
		changes := []ChangedFactor{{Name: "tool_output.search", Category: FactorCategoryTool}}
		decision := rule.Evaluate(changes)
		if decision.ShouldInvalidate {
			t.Error("expected no invalidation for tool output only")
		}
	})
}

func TestSystemPromptChangeRule_Evaluate(t *testing.T) {
	rule := &SystemPromptChangeRule{}

	t.Run("system prompt change triggers full invalidation", func(t *testing.T) {
		changes := []ChangedFactor{{
			Name: "prompt.system_prompt_hash", Category: FactorCategoryPrompt,
			ChangeType: ChangeTypeModified, Weight: DefaultWeights[FactorCategoryPrompt],
		}}
		decision := rule.Evaluate(changes)
		if !decision.ShouldInvalidate {
			t.Error("expected invalidation")
		}
		if decision.Scope != ScopeFull {
			t.Errorf("expected full, got %s", decision.Scope)
		}
	})

	t.Run("template change does not trigger", func(t *testing.T) {
		changes := []ChangedFactor{{Name: "prompt.template_hash", Category: FactorCategoryPrompt}}
		decision := rule.Evaluate(changes)
		if decision.ShouldInvalidate {
			t.Error("expected no invalidation for template change")
		}
	})
}

func TestConfigurationChangeRule_Evaluate(t *testing.T) {
	rule := &ConfigurationChangeRule{}

	t.Run("custom factor change triggers partial invalidation", func(t *testing.T) {
		changes := []ChangedFactor{{
			Name: "custom.env", Category: FactorCategoryCustom,
			ChangeType: ChangeTypeModified, Weight: DefaultWeights[FactorCategoryCustom],
		}}
		decision := rule.Evaluate(changes)
		if !decision.ShouldInvalidate {
			t.Error("expected invalidation")
		}
		if decision.Scope != ScopePartial {
			t.Errorf("expected partial, got %s", decision.Scope)
		}
	})
}

func TestWeightedChangeRule_Evaluate(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		rule := &WeightedChangeRule{Threshold: 0.1}
		decision := rule.Evaluate(nil)
		if decision.ShouldInvalidate {
			t.Error("expected no invalidation")
		}
	})

	t.Run("above threshold triggers invalidation", func(t *testing.T) {
		rule := &WeightedChangeRule{Threshold: 0.1}
		changes := []ChangedFactor{{Name: "model.name", Category: FactorCategoryModel, Weight: WeightCritical}}
		decision := rule.Evaluate(changes)
		if !decision.ShouldInvalidate {
			t.Error("expected invalidation")
		}
	})

	t.Run("high score triggers full scope", func(t *testing.T) {
		rule := &WeightedChangeRule{Threshold: 0.01}
		changes := []ChangedFactor{
			{Name: "a", Category: FactorCategoryModel, Weight: WeightCritical},
			{Name: "b", Category: FactorCategoryPrompt, Weight: WeightCritical},
			{Name: "c", Category: FactorCategoryRepository, Weight: WeightHigh},
			{Name: "d", Category: FactorCategoryFile, Weight: WeightHigh},
			{Name: "e", Category: FactorCategoryTool, Weight: WeightHigh},
			{Name: "f", Category: FactorCategoryConversation, Weight: WeightCritical},
			{Name: "g", Category: FactorCategoryRetrieval, Weight: WeightHigh},
			{Name: "h", Category: FactorCategoryDependency, Weight: WeightMedium},
			{Name: "i", Category: FactorCategoryCustom, Weight: WeightMedium},
		}
		decision := rule.Evaluate(changes)
		if decision.Scope != ScopeFull {
			t.Errorf("expected full, got %s (score: %f)", decision.Scope, decision.WeightedScore)
		}
	})

	t.Run("default threshold", func(t *testing.T) {
		rule := &WeightedChangeRule{}
		changes := []ChangedFactor{{Name: "model.name", Category: FactorCategoryModel, Weight: WeightCritical}}
		decision := rule.Evaluate(changes)
		if !decision.ShouldInvalidate {
			t.Error("expected invalidation with default threshold")
		}
	})
}

func TestCustomRule_Evaluate(t *testing.T) {
	rule := &CustomRule{
		RuleName: "my_custom_rule",
		EvaluateFunc: func(changes []ChangedFactor) InvalidationDecision {
			for _, c := range changes {
				if c.Name == "custom.feature_flag" {
					return InvalidationDecision{
						ShouldInvalidate: true, Scope: ScopeFull,
						Reason: "feature flag changed", MatchedRules: []string{"my_custom_rule"},
					}
				}
			}
			return InvalidationDecision{Scope: ScopeNone}
		},
	}

	if rule.Name() != "my_custom_rule" {
		t.Errorf("expected 'my_custom_rule', got %q", rule.Name())
	}

	t.Run("triggers on match", func(t *testing.T) {
		decision := rule.Evaluate([]ChangedFactor{{Name: "custom.feature_flag", Category: FactorCategoryCustom}})
		if !decision.ShouldInvalidate {
			t.Error("expected invalidation")
		}
	})

	t.Run("no trigger on non-match", func(t *testing.T) {
		decision := rule.Evaluate([]ChangedFactor{{Name: "custom.other", Category: FactorCategoryCustom}})
		if decision.ShouldInvalidate {
			t.Error("expected no invalidation")
		}
	})
}

func TestRuleEngine_Evaluate(t *testing.T) {
	t.Run("no changes", func(t *testing.T) {
		engine := NewRuleEngine()
		decision := engine.Evaluate(nil)
		if decision.ShouldInvalidate {
			t.Error("expected no invalidation")
		}
	})

	t.Run("repository change triggers full", func(t *testing.T) {
		engine := NewRuleEngine()
		changes := []ChangedFactor{{
			Name: "repository.commit_hash", Category: FactorCategoryRepository,
			ChangeType: ChangeTypeModified, Weight: DefaultWeights[FactorCategoryRepository],
		}}
		decision := engine.Evaluate(changes)
		if !decision.ShouldInvalidate || decision.Scope != ScopeFull {
			t.Errorf("expected full invalidation, got shouldInvalidate=%v scope=%s", decision.ShouldInvalidate, decision.Scope)
		}
	})

	t.Run("aggregates matched rules", func(t *testing.T) {
		engine := NewRuleEngine()
		changes := []ChangedFactor{
			{Name: "repository.commit_hash", Category: FactorCategoryRepository, Weight: DefaultWeights[FactorCategoryRepository]},
			{Name: "dependency.go.sum", Category: FactorCategoryDependency, Weight: DefaultWeights[FactorCategoryDependency]},
		}
		decision := engine.Evaluate(changes)
		if len(decision.MatchedRules) < 2 {
			t.Errorf("expected >=2 matched rules, got %d: %v", len(decision.MatchedRules), decision.MatchedRules)
		}
	})

	t.Run("most severe scope wins", func(t *testing.T) {
		engine := NewEmptyRuleEngine()
		engine.AddRule(&DependencyChangeRule{})
		engine.AddRule(&RepositoryChangeRule{})
		changes := []ChangedFactor{
			{Name: "repository.commit_hash", Category: FactorCategoryRepository, Weight: WeightHigh},
			{Name: "dependency.go.sum", Category: FactorCategoryDependency, Weight: WeightMedium},
		}
		decision := engine.Evaluate(changes)
		if decision.Scope != ScopeFull {
			t.Errorf("expected full, got %s", decision.Scope)
		}
	})
}

func TestRuleEngine_AddRemoveRule(t *testing.T) {
	engine := NewEmptyRuleEngine()
	if len(engine.Rules()) != 0 {
		t.Error("expected empty")
	}

	engine.AddRule(&RepositoryChangeRule{})
	if len(engine.Rules()) != 1 {
		t.Errorf("expected 1, got %d", len(engine.Rules()))
	}

	if !engine.RemoveRule("repository_change") {
		t.Error("expected removal success")
	}
	if len(engine.Rules()) != 0 {
		t.Error("expected 0 after removal")
	}
	if engine.RemoveRule("nonexistent") {
		t.Error("expected false for nonexistent")
	}
}

func TestRuleEngine_DefaultRules(t *testing.T) {
	engine := NewRuleEngine()
	rules := engine.Rules()
	expected := map[string]bool{
		"repository_change": false, "dependency_change": false,
		"tool_schema_change": false, "system_prompt_change": false,
		"configuration_change": false, "weighted_change": false,
	}
	for _, name := range rules {
		if _, ok := expected[name]; ok {
			expected[name] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("missing default rule %q", name)
		}
	}
}

func TestRuleEngine_CustomRuleRegistration(t *testing.T) {
	engine := NewEmptyRuleEngine()
	engine.AddRule(&CustomRule{
		RuleName: "test_rule",
		EvaluateFunc: func(changes []ChangedFactor) InvalidationDecision {
			return InvalidationDecision{ShouldInvalidate: true, Scope: ScopeFull, Reason: "test", MatchedRules: []string{"test_rule"}}
		},
	})
	changes := []ChangedFactor{{Name: "anything", Category: FactorCategoryCustom, Weight: WeightMedium}}
	decision := engine.Evaluate(changes)
	if !decision.ShouldInvalidate {
		t.Error("expected custom rule to trigger")
	}
}
