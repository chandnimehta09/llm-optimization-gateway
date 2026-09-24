package context

import (
	"fmt"
	"sort"
)

// ChangeType classifies the nature of a context change.
type ChangeType string

const (
	// ChangeTypeAdded indicates a new factor was introduced.
	ChangeTypeAdded ChangeType = "added"
	// ChangeTypeRemoved indicates a factor was removed.
	ChangeTypeRemoved ChangeType = "removed"
	// ChangeTypeModified indicates a factor's value changed.
	ChangeTypeModified ChangeType = "modified"
)

// ChangedFactor describes a single factor that changed between two context identities.
type ChangedFactor struct {
	// Name is the factor name (e.g., "repository.commit_hash").
	Name string `json:"name"`
	// Category is the factor's category.
	Category FactorCategory `json:"category"`
	// ChangeType describes the nature of the change.
	ChangeType ChangeType `json:"change_type"`
	// OldValue is the previous value (empty for additions).
	OldValue string `json:"old_value,omitempty"`
	// NewValue is the current value (empty for removals).
	NewValue string `json:"new_value,omitempty"`
	// Weight is the factor's importance weight.
	Weight FactorWeight `json:"weight"`
}

// ChangeDetector compares two context identities and identifies changed factors.
type ChangeDetector struct{}

// NewChangeDetector creates a new ChangeDetector.
func NewChangeDetector() *ChangeDetector {
	return &ChangeDetector{}
}

// DetectChanges compares a cached context identity with the current one and
// returns all changed factors. The returned slice is sorted by category then name.
func (d *ChangeDetector) DetectChanges(cached, current *ContextIdentity) []ChangedFactor {
	if cached == nil || current == nil {
		return nil
	}

	var changes []ChangedFactor

	changes = append(changes, d.detectModelChanges(cached, current)...)
	changes = append(changes, d.detectParameterChanges(cached, current)...)
	changes = append(changes, d.detectPromptChanges(cached, current)...)
	changes = append(changes, d.detectRepositoryChanges(cached, current)...)
	changes = append(changes, d.detectFileChanges(cached, current)...)
	changes = append(changes, d.detectDependencyChanges(cached, current)...)
	changes = append(changes, d.detectToolSchemaChanges(cached, current)...)
	changes = append(changes, d.detectToolOutputChanges(cached, current)...)
	changes = append(changes, d.detectConversationChanges(cached, current)...)
	changes = append(changes, d.detectCustomFactorChanges(cached, current)...)

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Category != changes[j].Category {
			return changes[i].Category < changes[j].Category
		}
		return changes[i].Name < changes[j].Name
	})

	return changes
}

// HasChanges returns true if there are any differences between cached and current.
func (d *ChangeDetector) HasChanges(cached, current *ContextIdentity) bool {
	return len(d.DetectChanges(cached, current)) > 0
}

// ChangedCategories returns the set of categories that have changes.
func (d *ChangeDetector) ChangedCategories(cached, current *ContextIdentity) []FactorCategory {
	changes := d.DetectChanges(cached, current)
	seen := make(map[FactorCategory]bool)
	var categories []FactorCategory
	for _, c := range changes {
		if !seen[c.Category] {
			seen[c.Category] = true
			categories = append(categories, c.Category)
		}
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i] < categories[j]
	})
	return categories
}

// MaxChangeWeight returns the highest weight among all changed factors.
// Returns 0 if there are no changes.
func (d *ChangeDetector) MaxChangeWeight(cached, current *ContextIdentity) FactorWeight {
	changes := d.DetectChanges(cached, current)
	var maxWeight FactorWeight
	for _, c := range changes {
		if c.Weight > maxWeight {
			maxWeight = c.Weight
		}
	}
	return maxWeight
}

func (d *ChangeDetector) detectModelChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryModel]

	if cached.Model.Name != current.Model.Name {
		changes = append(changes, ChangedFactor{
			Name:       "model.name",
			Category:   FactorCategoryModel,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Model.Name,
			NewValue:   current.Model.Name,
			Weight:     weight,
		})
	}
	if cached.Model.Version != current.Model.Version {
		changes = append(changes, ChangedFactor{
			Name:       "model.version",
			Category:   FactorCategoryModel,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Model.Version,
			NewValue:   current.Model.Version,
			Weight:     weight,
		})
	}
	if cached.Model.Provider != current.Model.Provider {
		changes = append(changes, ChangedFactor{
			Name:       "model.provider",
			Category:   FactorCategoryModel,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Model.Provider,
			NewValue:   current.Model.Provider,
			Weight:     weight,
		})
	}
	return changes
}

func (d *ChangeDetector) detectParameterChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryModel]

	changes = append(changes, detectFloatPtrChange("params.temperature", FactorCategoryModel, weight, cached.Parameters.Temperature, current.Parameters.Temperature)...)
	changes = append(changes, detectFloatPtrChange("params.top_p", FactorCategoryModel, weight, cached.Parameters.TopP, current.Parameters.TopP)...)
	changes = append(changes, detectIntPtrChange("params.max_tokens", FactorCategoryModel, weight, cached.Parameters.MaxTokens, current.Parameters.MaxTokens)...)
	changes = append(changes, detectIntPtrChange("params.top_k", FactorCategoryModel, weight, cached.Parameters.TopK, current.Parameters.TopK)...)
	changes = append(changes, detectFloatPtrChange("params.frequency_penalty", FactorCategoryModel, weight, cached.Parameters.FrequencyPenalty, current.Parameters.FrequencyPenalty)...)
	changes = append(changes, detectFloatPtrChange("params.presence_penalty", FactorCategoryModel, weight, cached.Parameters.PresencePenalty, current.Parameters.PresencePenalty)...)
	changes = append(changes, detectInt64PtrChange("params.seed", FactorCategoryModel, weight, cached.Parameters.Seed, current.Parameters.Seed)...)

	cachedStop := fmt.Sprintf("%v", cached.Parameters.Stop)
	currentStop := fmt.Sprintf("%v", current.Parameters.Stop)
	if cachedStop != currentStop {
		changes = append(changes, ChangedFactor{
			Name:       "params.stop",
			Category:   FactorCategoryModel,
			ChangeType: ChangeTypeModified,
			OldValue:   cachedStop,
			NewValue:   currentStop,
			Weight:     weight,
		})
	}

	return changes
}

func (d *ChangeDetector) detectPromptChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryPrompt]

	if cached.Prompt.TemplateHash != current.Prompt.TemplateHash {
		changes = append(changes, ChangedFactor{
			Name:       "prompt.template_hash",
			Category:   FactorCategoryPrompt,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Prompt.TemplateHash,
			NewValue:   current.Prompt.TemplateHash,
			Weight:     weight,
		})
	}
	if cached.Prompt.TemplateVersion != current.Prompt.TemplateVersion {
		changes = append(changes, ChangedFactor{
			Name:       "prompt.template_version",
			Category:   FactorCategoryPrompt,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Prompt.TemplateVersion,
			NewValue:   current.Prompt.TemplateVersion,
			Weight:     weight,
		})
	}
	if cached.Prompt.SystemPromptHash != current.Prompt.SystemPromptHash {
		changes = append(changes, ChangedFactor{
			Name:       "prompt.system_prompt_hash",
			Category:   FactorCategoryPrompt,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Prompt.SystemPromptHash,
			NewValue:   current.Prompt.SystemPromptHash,
			Weight:     weight,
		})
	}
	return changes
}

func (d *ChangeDetector) detectRepositoryChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryRepository]

	if cached.Repository == nil && current.Repository == nil {
		return nil
	}
	if cached.Repository == nil {
		changes = append(changes, ChangedFactor{
			Name:       "repository",
			Category:   FactorCategoryRepository,
			ChangeType: ChangeTypeAdded,
			NewValue:   current.Repository.URL,
			Weight:     weight,
		})
		return changes
	}
	if current.Repository == nil {
		changes = append(changes, ChangedFactor{
			Name:       "repository",
			Category:   FactorCategoryRepository,
			ChangeType: ChangeTypeRemoved,
			OldValue:   cached.Repository.URL,
			Weight:     weight,
		})
		return changes
	}

	if cached.Repository.URL != current.Repository.URL {
		changes = append(changes, ChangedFactor{
			Name:       "repository.url",
			Category:   FactorCategoryRepository,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Repository.URL,
			NewValue:   current.Repository.URL,
			Weight:     weight,
		})
	}
	if cached.Repository.Branch != current.Repository.Branch {
		changes = append(changes, ChangedFactor{
			Name:       "repository.branch",
			Category:   FactorCategoryRepository,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Repository.Branch,
			NewValue:   current.Repository.Branch,
			Weight:     weight,
		})
	}
	if cached.Repository.CommitHash != current.Repository.CommitHash {
		changes = append(changes, ChangedFactor{
			Name:       "repository.commit_hash",
			Category:   FactorCategoryRepository,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Repository.CommitHash,
			NewValue:   current.Repository.CommitHash,
			Weight:     weight,
		})
	}
	return changes
}

func (d *ChangeDetector) detectFileChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryFile]

	cachedFiles := make(map[string]string, len(cached.FileHashes))
	for _, f := range cached.FileHashes {
		cachedFiles[f.Path] = f.Hash
	}
	currentFiles := make(map[string]string, len(current.FileHashes))
	for _, f := range current.FileHashes {
		currentFiles[f.Path] = f.Hash
	}

	for path, oldHash := range cachedFiles {
		if newHash, ok := currentFiles[path]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "file." + path,
				Category:   FactorCategoryFile,
				ChangeType: ChangeTypeRemoved,
				OldValue:   oldHash,
				Weight:     weight,
			})
		} else if oldHash != newHash {
			changes = append(changes, ChangedFactor{
				Name:       "file." + path,
				Category:   FactorCategoryFile,
				ChangeType: ChangeTypeModified,
				OldValue:   oldHash,
				NewValue:   newHash,
				Weight:     weight,
			})
		}
	}

	for path, newHash := range currentFiles {
		if _, ok := cachedFiles[path]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "file." + path,
				Category:   FactorCategoryFile,
				ChangeType: ChangeTypeAdded,
				NewValue:   newHash,
				Weight:     weight,
			})
		}
	}

	return changes
}

func (d *ChangeDetector) detectDependencyChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryDependency]

	cachedDeps := make(map[string]string, len(cached.DependencyLocks))
	for _, dep := range cached.DependencyLocks {
		cachedDeps[dep.Type] = dep.Hash
	}
	currentDeps := make(map[string]string, len(current.DependencyLocks))
	for _, dep := range current.DependencyLocks {
		currentDeps[dep.Type] = dep.Hash
	}

	for depType, oldHash := range cachedDeps {
		if newHash, ok := currentDeps[depType]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "dependency." + depType,
				Category:   FactorCategoryDependency,
				ChangeType: ChangeTypeRemoved,
				OldValue:   oldHash,
				Weight:     weight,
			})
		} else if oldHash != newHash {
			changes = append(changes, ChangedFactor{
				Name:       "dependency." + depType,
				Category:   FactorCategoryDependency,
				ChangeType: ChangeTypeModified,
				OldValue:   oldHash,
				NewValue:   newHash,
				Weight:     weight,
			})
		}
	}
	for depType, newHash := range currentDeps {
		if _, ok := cachedDeps[depType]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "dependency." + depType,
				Category:   FactorCategoryDependency,
				ChangeType: ChangeTypeAdded,
				NewValue:   newHash,
				Weight:     weight,
			})
		}
	}

	return changes
}

func (d *ChangeDetector) detectToolSchemaChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryTool]

	cachedTools := make(map[string]ToolSchemaVersion, len(cached.ToolSchemas))
	for _, t := range cached.ToolSchemas {
		cachedTools[t.Name] = t
	}
	currentTools := make(map[string]ToolSchemaVersion, len(current.ToolSchemas))
	for _, t := range current.ToolSchemas {
		currentTools[t.Name] = t
	}

	for name, oldTool := range cachedTools {
		if newTool, ok := currentTools[name]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "tool_schema." + name,
				Category:   FactorCategoryTool,
				ChangeType: ChangeTypeRemoved,
				OldValue:   oldTool.Version,
				Weight:     weight,
			})
		} else if oldTool.Version != newTool.Version || oldTool.SchemaHash != newTool.SchemaHash {
			changes = append(changes, ChangedFactor{
				Name:       "tool_schema." + name,
				Category:   FactorCategoryTool,
				ChangeType: ChangeTypeModified,
				OldValue:   oldTool.Version,
				NewValue:   newTool.Version,
				Weight:     weight,
			})
		}
	}
	for name, newTool := range currentTools {
		if _, ok := cachedTools[name]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "tool_schema." + name,
				Category:   FactorCategoryTool,
				ChangeType: ChangeTypeAdded,
				NewValue:   newTool.Version,
				Weight:     weight,
			})
		}
	}

	return changes
}

func (d *ChangeDetector) detectToolOutputChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryTool]

	cachedOutputs := make(map[string]string, len(cached.ToolOutputs))
	for _, t := range cached.ToolOutputs {
		cachedOutputs[t.ToolName] = t.OutputHash
	}
	currentOutputs := make(map[string]string, len(current.ToolOutputs))
	for _, t := range current.ToolOutputs {
		currentOutputs[t.ToolName] = t.OutputHash
	}

	for name, oldHash := range cachedOutputs {
		if newHash, ok := currentOutputs[name]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "tool_output." + name,
				Category:   FactorCategoryTool,
				ChangeType: ChangeTypeRemoved,
				OldValue:   oldHash,
				Weight:     weight,
			})
		} else if oldHash != newHash {
			changes = append(changes, ChangedFactor{
				Name:       "tool_output." + name,
				Category:   FactorCategoryTool,
				ChangeType: ChangeTypeModified,
				OldValue:   oldHash,
				NewValue:   newHash,
				Weight:     weight,
			})
		}
	}
	for name, newHash := range currentOutputs {
		if _, ok := cachedOutputs[name]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "tool_output." + name,
				Category:   FactorCategoryTool,
				ChangeType: ChangeTypeAdded,
				NewValue:   newHash,
				Weight:     weight,
			})
		}
	}

	return changes
}

func (d *ChangeDetector) detectConversationChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryConversation]

	if cached.Conversation == nil && current.Conversation == nil {
		return nil
	}
	if cached.Conversation == nil {
		changes = append(changes, ChangedFactor{
			Name:       "conversation",
			Category:   FactorCategoryConversation,
			ChangeType: ChangeTypeAdded,
			Weight:     weight,
		})
		return changes
	}
	if current.Conversation == nil {
		changes = append(changes, ChangedFactor{
			Name:       "conversation",
			Category:   FactorCategoryConversation,
			ChangeType: ChangeTypeRemoved,
			Weight:     weight,
		})
		return changes
	}

	if cached.Conversation.TurnCount != current.Conversation.TurnCount {
		changes = append(changes, ChangedFactor{
			Name:       "conversation.turn_count",
			Category:   FactorCategoryConversation,
			ChangeType: ChangeTypeModified,
			OldValue:   fmt.Sprintf("%d", cached.Conversation.TurnCount),
			NewValue:   fmt.Sprintf("%d", current.Conversation.TurnCount),
			Weight:     weight,
		})
	}
	if cached.Conversation.HistoryHash != current.Conversation.HistoryHash {
		changes = append(changes, ChangedFactor{
			Name:       "conversation.history_hash",
			Category:   FactorCategoryConversation,
			ChangeType: ChangeTypeModified,
			OldValue:   cached.Conversation.HistoryHash,
			NewValue:   current.Conversation.HistoryHash,
			Weight:     weight,
		})
	}

	return changes
}

func (d *ChangeDetector) detectCustomFactorChanges(cached, current *ContextIdentity) []ChangedFactor {
	var changes []ChangedFactor
	weight := DefaultWeights[FactorCategoryCustom]

	for k, oldVal := range cached.CustomFactors {
		if newVal, ok := current.CustomFactors[k]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "custom." + k,
				Category:   FactorCategoryCustom,
				ChangeType: ChangeTypeRemoved,
				OldValue:   oldVal,
				Weight:     weight,
			})
		} else if oldVal != newVal {
			changes = append(changes, ChangedFactor{
				Name:       "custom." + k,
				Category:   FactorCategoryCustom,
				ChangeType: ChangeTypeModified,
				OldValue:   oldVal,
				NewValue:   newVal,
				Weight:     weight,
			})
		}
	}
	for k, newVal := range current.CustomFactors {
		if _, ok := cached.CustomFactors[k]; !ok {
			changes = append(changes, ChangedFactor{
				Name:       "custom." + k,
				Category:   FactorCategoryCustom,
				ChangeType: ChangeTypeAdded,
				NewValue:   newVal,
				Weight:     weight,
			})
		}
	}

	return changes
}

// detectFloatPtrChange detects changes in optional float64 pointer values.
func detectFloatPtrChange(name string, category FactorCategory, weight FactorWeight, old, new *float64) []ChangedFactor {
	if old == nil && new == nil {
		return nil
	}
	if old == nil {
		return []ChangedFactor{{
			Name: name, Category: category, ChangeType: ChangeTypeAdded,
			NewValue: fmt.Sprintf("%g", *new), Weight: weight,
		}}
	}
	if new == nil {
		return []ChangedFactor{{
			Name: name, Category: category, ChangeType: ChangeTypeRemoved,
			OldValue: fmt.Sprintf("%g", *old), Weight: weight,
		}}
	}
	if *old != *new {
		return []ChangedFactor{{
			Name: name, Category: category, ChangeType: ChangeTypeModified,
			OldValue: fmt.Sprintf("%g", *old), NewValue: fmt.Sprintf("%g", *new), Weight: weight,
		}}
	}
	return nil
}

// detectIntPtrChange detects changes in optional int pointer values.
func detectIntPtrChange(name string, category FactorCategory, weight FactorWeight, old, new *int) []ChangedFactor {
	if old == nil && new == nil {
		return nil
	}
	if old == nil {
		return []ChangedFactor{{
			Name: name, Category: category, ChangeType: ChangeTypeAdded,
			NewValue: fmt.Sprintf("%d", *new), Weight: weight,
		}}
	}
	if new == nil {
		return []ChangedFactor{{
			Name: name, Category: category, ChangeType: ChangeTypeRemoved,
			OldValue: fmt.Sprintf("%d", *old), Weight: weight,
		}}
	}
	if *old != *new {
		return []ChangedFactor{{
			Name: name, Category: category, ChangeType: ChangeTypeModified,
			OldValue: fmt.Sprintf("%d", *old), NewValue: fmt.Sprintf("%d", *new), Weight: weight,
		}}
	}
	return nil
}

// detectInt64PtrChange detects changes in optional int64 pointer values.
func detectInt64PtrChange(name string, category FactorCategory, weight FactorWeight, old, new *int64) []ChangedFactor {
	if old == nil && new == nil {
		return nil
	}
	if old == nil {
		return []ChangedFactor{{
			Name: name, Category: category, ChangeType: ChangeTypeAdded,
			NewValue: fmt.Sprintf("%d", *new), Weight: weight,
		}}
	}
	if new == nil {
		return []ChangedFactor{{
			Name: name, Category: category, ChangeType: ChangeTypeRemoved,
			OldValue: fmt.Sprintf("%d", *old), Weight: weight,
		}}
	}
	if *old != *new {
		return []ChangedFactor{{
			Name: name, Category: category, ChangeType: ChangeTypeModified,
			OldValue: fmt.Sprintf("%d", *old), NewValue: fmt.Sprintf("%d", *new), Weight: weight,
		}}
	}
	return nil
}
