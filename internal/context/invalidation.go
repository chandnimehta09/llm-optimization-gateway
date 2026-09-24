package context

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// InvalidationReason describes why a cache entry was invalidated.
type InvalidationReason string

const (
	// ReasonRepositoryChange indicates invalidation due to a repository commit change.
	ReasonRepositoryChange InvalidationReason = "repository_change"
	// ReasonDependencyChange indicates invalidation due to dependency lock file changes.
	ReasonDependencyChange InvalidationReason = "dependency_change"
	// ReasonToolSchemaChange indicates invalidation due to tool schema version changes.
	ReasonToolSchemaChange InvalidationReason = "tool_schema_change"
	// ReasonSystemPromptChange indicates invalidation due to system prompt version changes.
	ReasonSystemPromptChange InvalidationReason = "system_prompt_change"
	// ReasonConfigurationChange indicates invalidation due to configuration changes.
	ReasonConfigurationChange InvalidationReason = "configuration_change"
	// ReasonWeightedThreshold indicates invalidation due to aggregate weighted change threshold.
	ReasonWeightedThreshold InvalidationReason = "weighted_threshold"
	// ReasonCustomRule indicates invalidation triggered by a custom rule.
	ReasonCustomRule InvalidationReason = "custom_rule"
	// ReasonSelectiveInvalidation indicates a manual selective invalidation.
	ReasonSelectiveInvalidation InvalidationReason = "selective_invalidation"
)

// InvalidationEvent records a single invalidation occurrence for observability.
type InvalidationEvent struct {
	// ID is a unique identifier for this event.
	ID string `json:"id"`
	// Timestamp is when the invalidation occurred.
	Timestamp time.Time `json:"timestamp"`
	// Reason is why the invalidation was triggered.
	Reason InvalidationReason `json:"reason"`
	// Decision is the full invalidation decision from the rule engine.
	Decision InvalidationDecision `json:"decision"`
	// AffectedKeys lists the cache keys that were invalidated.
	AffectedKeys []string `json:"affected_keys"`
	// Changes lists the detected context changes that triggered this event.
	Changes []ChangedFactor `json:"changes,omitempty"`
}

// InvalidationFilter defines criteria for selective cache invalidation.
type InvalidationFilter struct {
	// Namespace filters by cache key namespace/prefix.
	Namespace string `json:"namespace,omitempty"`
	// Model filters by model name.
	Model string `json:"model,omitempty"`
	// Before filters entries created before this time.
	Before *time.Time `json:"before,omitempty"`
	// After filters entries created after this time.
	After *time.Time `json:"after,omitempty"`
	// Categories filters by factor categories that triggered invalidation.
	Categories []FactorCategory `json:"categories,omitempty"`
}

// Matches returns true if the given entry metadata and key match the filter criteria.
func (f *InvalidationFilter) Matches(key string, model string, createdAt time.Time) bool {
	if f.Namespace != "" && !strings.HasPrefix(key, f.Namespace) {
		return false
	}
	if f.Model != "" && model != f.Model {
		return false
	}
	if f.Before != nil && !createdAt.Before(*f.Before) {
		return false
	}
	if f.After != nil && !createdAt.After(*f.After) {
		return false
	}
	return true
}

// CacheEntry provides the minimal interface for checking a cached entry during invalidation.
type CacheEntry struct {
	// Key is the cache key.
	Key string
	// Model is the model name associated with this entry.
	Model string
	// CreatedAt is when the entry was created.
	CreatedAt time.Time
	// ContextIdentity is the context identity snapshot at cache time.
	ContextIdentity *ContextIdentity
	// HasToolCalls indicates whether the cached response contains tool calls.
	HasToolCalls bool
}

// InvalidationEventHandler is a callback invoked when an invalidation event occurs.
type InvalidationEventHandler func(event InvalidationEvent)

// Invalidator coordinates change detection, rule evaluation, and cache invalidation.
type Invalidator struct {
	detector     *ChangeDetector
	engine       *RuleEngine
	mu           sync.RWMutex
	events       []InvalidationEvent
	maxEvents    int
	eventCounter int64
	handlers     []InvalidationEventHandler
}

// NewInvalidator creates a new Invalidator with default rules and change detection.
func NewInvalidator() *Invalidator {
	return &Invalidator{
		detector:  NewChangeDetector(),
		engine:    NewRuleEngine(),
		events:    make([]InvalidationEvent, 0),
		maxEvents: 1000,
	}
}

// NewInvalidatorWithEngine creates a new Invalidator with a custom rule engine.
func NewInvalidatorWithEngine(engine *RuleEngine) *Invalidator {
	return &Invalidator{
		detector:  NewChangeDetector(),
		engine:    engine,
		events:    make([]InvalidationEvent, 0),
		maxEvents: 1000,
	}
}

// SetMaxEvents sets the maximum number of invalidation events to retain.
func (inv *Invalidator) SetMaxEvents(max int) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	inv.maxEvents = max
	if len(inv.events) > max {
		inv.events = inv.events[len(inv.events)-max:]
	}
}

// OnInvalidation registers a handler to be called when invalidation events occur.
func (inv *Invalidator) OnInvalidation(handler InvalidationEventHandler) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	inv.handlers = append(inv.handlers, handler)
}

// CheckAndInvalidate compares a cached context identity against the current one,
// evaluates invalidation rules, and returns the affected cache keys that should
// be deleted. The entries slice contains all candidate cache entries to check.
func (inv *Invalidator) CheckAndInvalidate(
	cached *ContextIdentity,
	current *ContextIdentity,
	entries []CacheEntry,
) ([]string, *InvalidationEvent) {
	changes := inv.detector.DetectChanges(cached, current)
	if len(changes) == 0 {
		return nil, nil
	}

	decision := inv.engine.Evaluate(changes)
	if !decision.ShouldInvalidate {
		return nil, nil
	}

	var affectedKeys []string
	for _, entry := range entries {
		if inv.shouldInvalidateEntry(entry, decision, changes) {
			affectedKeys = append(affectedKeys, entry.Key)
		}
	}

	if len(affectedKeys) == 0 {
		return nil, nil
	}

	sort.Strings(affectedKeys)

	reason := inv.decisionToReason(decision)
	event := inv.recordEvent(reason, decision, affectedKeys, changes)

	return affectedKeys, &event
}

// SelectiveInvalidate invalidates cache entries matching the given filter.
// Returns the keys of invalidated entries.
func (inv *Invalidator) SelectiveInvalidate(
	entries []CacheEntry,
	filter InvalidationFilter,
) ([]string, *InvalidationEvent) {
	var affectedKeys []string
	for _, entry := range entries {
		if filter.Matches(entry.Key, entry.Model, entry.CreatedAt) {
			if len(filter.Categories) > 0 && !inv.entryMatchesCategories(entry, filter.Categories) {
				continue
			}
			affectedKeys = append(affectedKeys, entry.Key)
		}
	}

	if len(affectedKeys) == 0 {
		return nil, nil
	}

	sort.Strings(affectedKeys)

	decision := InvalidationDecision{
		ShouldInvalidate:   true,
		Scope:              ScopePartial,
		Reason:             fmt.Sprintf("selective invalidation: %s", formatFilter(filter)),
		AffectedCategories: filter.Categories,
		MatchedRules:       []string{"selective"},
	}

	event := inv.recordEvent(ReasonSelectiveInvalidation, decision, affectedKeys, nil)

	return affectedKeys, &event
}

// Events returns a copy of all recorded invalidation events.
func (inv *Invalidator) Events() []InvalidationEvent {
	inv.mu.RLock()
	defer inv.mu.RUnlock()
	result := make([]InvalidationEvent, len(inv.events))
	copy(result, inv.events)
	return result
}

// EventsSince returns events that occurred after the given timestamp.
func (inv *Invalidator) EventsSince(since time.Time) []InvalidationEvent {
	inv.mu.RLock()
	defer inv.mu.RUnlock()
	var result []InvalidationEvent
	for _, e := range inv.events {
		if e.Timestamp.After(since) {
			result = append(result, e)
		}
	}
	return result
}

// ClearEvents removes all recorded invalidation events.
func (inv *Invalidator) ClearEvents() {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	inv.events = make([]InvalidationEvent, 0)
}

// RuleEngine returns the underlying rule engine for customization.
func (inv *Invalidator) RuleEngine() *RuleEngine {
	return inv.engine
}

// ChangeDetector returns the underlying change detector.
func (inv *Invalidator) ChangeDetector() *ChangeDetector {
	return inv.detector
}

func (inv *Invalidator) shouldInvalidateEntry(entry CacheEntry, decision InvalidationDecision, changes []ChangedFactor) bool {
	switch decision.Scope {
	case ScopeFull:
		return true
	case ScopePartial:
		return inv.entryAffectedByChanges(entry, changes, decision.AffectedCategories)
	default:
		return false
	}
}

func (inv *Invalidator) entryAffectedByChanges(entry CacheEntry, changes []ChangedFactor, affectedCategories []FactorCategory) bool {
	catSet := make(map[FactorCategory]bool, len(affectedCategories))
	for _, cat := range affectedCategories {
		catSet[cat] = true
	}

	if catSet[FactorCategoryTool] && entry.HasToolCalls {
		return true
	}
	if catSet[FactorCategoryDependency] {
		return true
	}
	if catSet[FactorCategoryCustom] {
		return true
	}
	if catSet[FactorCategoryFile] && entry.ContextIdentity != nil {
		for _, change := range changes {
			if change.Category == FactorCategoryFile {
				return true
			}
		}
	}

	return false
}

func (inv *Invalidator) entryMatchesCategories(entry CacheEntry, categories []FactorCategory) bool {
	if entry.ContextIdentity == nil {
		return true
	}
	for _, cat := range categories {
		switch cat {
		case FactorCategoryTool:
			if entry.HasToolCalls || len(entry.ContextIdentity.ToolSchemas) > 0 {
				return true
			}
		case FactorCategoryRepository:
			if entry.ContextIdentity.Repository != nil {
				return true
			}
		case FactorCategoryDependency:
			if len(entry.ContextIdentity.DependencyLocks) > 0 {
				return true
			}
		case FactorCategoryPrompt:
			if entry.ContextIdentity.Prompt.TemplateHash != "" || entry.ContextIdentity.Prompt.SystemPromptHash != "" {
				return true
			}
		case FactorCategoryModel:
			if entry.ContextIdentity.Model.Name != "" {
				return true
			}
		default:
			return true
		}
	}
	return false
}

func (inv *Invalidator) decisionToReason(decision InvalidationDecision) InvalidationReason {
	for _, rule := range decision.MatchedRules {
		switch rule {
		case "repository_change":
			return ReasonRepositoryChange
		case "dependency_change":
			return ReasonDependencyChange
		case "tool_schema_change":
			return ReasonToolSchemaChange
		case "system_prompt_change":
			return ReasonSystemPromptChange
		case "configuration_change":
			return ReasonConfigurationChange
		case "weighted_change":
			return ReasonWeightedThreshold
		}
	}
	return ReasonCustomRule
}

func (inv *Invalidator) recordEvent(
	reason InvalidationReason,
	decision InvalidationDecision,
	affectedKeys []string,
	changes []ChangedFactor,
) InvalidationEvent {
	inv.mu.Lock()

	inv.eventCounter++
	event := InvalidationEvent{
		ID:           fmt.Sprintf("inv-%d", inv.eventCounter),
		Timestamp:    time.Now().UTC(),
		Reason:       reason,
		Decision:     decision,
		AffectedKeys: affectedKeys,
		Changes:      changes,
	}

	inv.events = append(inv.events, event)

	if len(inv.events) > inv.maxEvents {
		inv.events = inv.events[len(inv.events)-inv.maxEvents:]
	}

	handlers := make([]InvalidationEventHandler, len(inv.handlers))
	copy(handlers, inv.handlers)
	inv.mu.Unlock()

	for _, h := range handlers {
		h(event)
	}

	return event
}

func formatFilter(f InvalidationFilter) string {
	var parts []string
	if f.Namespace != "" {
		parts = append(parts, fmt.Sprintf("namespace=%s", f.Namespace))
	}
	if f.Model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", f.Model))
	}
	if f.Before != nil {
		parts = append(parts, fmt.Sprintf("before=%s", f.Before.Format(time.RFC3339)))
	}
	if f.After != nil {
		parts = append(parts, fmt.Sprintf("after=%s", f.After.Format(time.RFC3339)))
	}
	if len(f.Categories) > 0 {
		cats := make([]string, len(f.Categories))
		for i, c := range f.Categories {
			cats[i] = string(c)
		}
		parts = append(parts, fmt.Sprintf("categories=[%s]", strings.Join(cats, ",")))
	}
	if len(parts) == 0 {
		return "all"
	}
	return strings.Join(parts, " ")
}
