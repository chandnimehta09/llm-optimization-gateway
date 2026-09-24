package context

import (
	"sync"
	"testing"
	"time"
)

func TestInvalidator_CheckAndInvalidate_NoChanges(t *testing.T) {
	inv := NewInvalidator()
	identity := buildInvTestIdentity()
	entries := []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now()}}

	keys, event := inv.CheckAndInvalidate(identity, identity, entries)
	if keys != nil {
		t.Errorf("expected no keys, got %v", keys)
	}
	if event != nil {
		t.Error("expected no event")
	}
}

func TestInvalidator_CheckAndInvalidate_RepositoryCommitChange(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Repository.CommitHash = "def456"

	entries := []CacheEntry{
		{Key: "key1", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached},
		{Key: "key2", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached},
	}

	keys, event := inv.CheckAndInvalidate(cached, current, entries)
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
	if event == nil {
		t.Fatal("expected event")
	}
	if event.Reason != ReasonRepositoryChange {
		t.Errorf("expected %s, got %s", ReasonRepositoryChange, event.Reason)
	}
	if len(event.AffectedKeys) != 2 {
		t.Errorf("expected 2 affected keys, got %d", len(event.AffectedKeys))
	}
	if event.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestInvalidator_CheckAndInvalidate_ToolSchemaChange(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.ToolSchemas = []ToolSchemaVersion{{Name: "search", Version: "v2", SchemaHash: "newschema"}}

	entries := []CacheEntry{
		{Key: "tool-key", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached, HasToolCalls: true},
		{Key: "no-tool-key", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached, HasToolCalls: false},
	}

	keys, event := inv.CheckAndInvalidate(cached, current, entries)
	if event == nil {
		t.Fatal("expected event")
	}
	// Full scope from ToolSchemaChangeRule.
	if len(keys) != 2 {
		t.Errorf("expected 2 keys (full scope), got %d: %v", len(keys), keys)
	}
}

func TestInvalidator_CheckAndInvalidate_SystemPromptChange(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Prompt.SystemPromptHash = "newsyshash"

	entries := []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached}}

	keys, event := inv.CheckAndInvalidate(cached, current, entries)
	if len(keys) != 1 {
		t.Errorf("expected 1, got %d", len(keys))
	}
	if event == nil {
		t.Fatal("expected event")
	}
	if event.Reason != ReasonSystemPromptChange {
		t.Errorf("expected %s, got %s", ReasonSystemPromptChange, event.Reason)
	}
}

func TestInvalidator_CheckAndInvalidate_DependencyChange(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.DependencyLocks[0].Hash = "newhash"

	entries := []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached}}

	keys, event := inv.CheckAndInvalidate(cached, current, entries)
	if len(keys) == 0 {
		t.Error("expected invalidated keys")
	}
	if event == nil {
		t.Fatal("expected event")
	}
}

func TestInvalidator_CheckAndInvalidate_ConfigurationChange(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	cached.CustomFactors = map[string]string{"env": "prod"}
	current := buildInvTestIdentity()
	current.CustomFactors = map[string]string{"env": "staging"}

	entries := []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached}}

	keys, event := inv.CheckAndInvalidate(cached, current, entries)
	if len(keys) == 0 {
		t.Error("expected invalidated keys")
	}
	if event == nil {
		t.Fatal("expected event")
	}
}

func TestInvalidator_SelectiveInvalidate_ByNamespace(t *testing.T) {
	inv := NewInvalidator()
	entries := []CacheEntry{
		{Key: "ns1:key1", Model: "gpt-4", CreatedAt: time.Now()},
		{Key: "ns1:key2", Model: "gpt-4", CreatedAt: time.Now()},
		{Key: "ns2:key3", Model: "gpt-4", CreatedAt: time.Now()},
	}

	keys, event := inv.SelectiveInvalidate(entries, InvalidationFilter{Namespace: "ns1:"})
	if len(keys) != 2 {
		t.Errorf("expected 2, got %d: %v", len(keys), keys)
	}
	if event == nil {
		t.Fatal("expected event")
	}
	if event.Reason != ReasonSelectiveInvalidation {
		t.Errorf("expected %s, got %s", ReasonSelectiveInvalidation, event.Reason)
	}
}

func TestInvalidator_SelectiveInvalidate_ByModel(t *testing.T) {
	inv := NewInvalidator()
	entries := []CacheEntry{
		{Key: "key1", Model: "gpt-4", CreatedAt: time.Now()},
		{Key: "key2", Model: "claude-3", CreatedAt: time.Now()},
		{Key: "key3", Model: "gpt-4", CreatedAt: time.Now()},
	}

	keys, _ := inv.SelectiveInvalidate(entries, InvalidationFilter{Model: "gpt-4"})
	if len(keys) != 2 {
		t.Errorf("expected 2, got %d: %v", len(keys), keys)
	}
}

func TestInvalidator_SelectiveInvalidate_ByTimeRange(t *testing.T) {
	inv := NewInvalidator()
	now := time.Now()
	cutoff := now.Add(-90 * time.Minute)

	entries := []CacheEntry{
		{Key: "old", Model: "gpt-4", CreatedAt: now.Add(-2 * time.Hour)},
		{Key: "mid", Model: "gpt-4", CreatedAt: now.Add(-1 * time.Hour)},
		{Key: "new", Model: "gpt-4", CreatedAt: now},
	}

	keys, _ := inv.SelectiveInvalidate(entries, InvalidationFilter{Before: &cutoff})
	if len(keys) != 1 || keys[0] != "old" {
		t.Errorf("expected [old], got %v", keys)
	}
}

func TestInvalidator_SelectiveInvalidate_ByCategory(t *testing.T) {
	inv := NewInvalidator()
	identity := buildInvTestIdentity()

	entries := []CacheEntry{
		{Key: "tool-entry", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: identity, HasToolCalls: true},
		{Key: "no-tool-entry", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: &ContextIdentity{Model: ModelIdentity{Name: "gpt-4"}}},
	}

	keys, _ := inv.SelectiveInvalidate(entries, InvalidationFilter{Categories: []FactorCategory{FactorCategoryTool}})
	if len(keys) != 1 || keys[0] != "tool-entry" {
		t.Errorf("expected [tool-entry], got %v", keys)
	}
}

func TestInvalidator_SelectiveInvalidate_NoMatches(t *testing.T) {
	inv := NewInvalidator()
	entries := []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now()}}

	keys, event := inv.SelectiveInvalidate(entries, InvalidationFilter{Model: "claude-3"})
	if keys != nil {
		t.Errorf("expected nil, got %v", keys)
	}
	if event != nil {
		t.Error("expected no event")
	}
}

func TestInvalidator_Events(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Repository.CommitHash = "newcommit"

	inv.CheckAndInvalidate(cached, current, []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now()}})

	events := inv.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1, got %d", len(events))
	}
	e := events[0]
	if e.ID == "" {
		t.Error("missing ID")
	}
	if e.Timestamp.IsZero() {
		t.Error("missing timestamp")
	}
	if e.Reason == "" {
		t.Error("missing reason")
	}
	if len(e.AffectedKeys) == 0 {
		t.Error("missing affected keys")
	}
	if len(e.Changes) == 0 {
		t.Error("missing changes")
	}
}

func TestInvalidator_EventsSince(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	entries := []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now()}}

	c1 := buildInvTestIdentity()
	c1.Repository.CommitHash = "commit1"
	inv.CheckAndInvalidate(cached, c1, entries)

	midpoint := time.Now().UTC()
	time.Sleep(10 * time.Millisecond)

	c2 := buildInvTestIdentity()
	c2.Repository.CommitHash = "commit2"
	inv.CheckAndInvalidate(cached, c2, entries)

	events := inv.EventsSince(midpoint)
	if len(events) != 1 {
		t.Errorf("expected 1 since midpoint, got %d", len(events))
	}
}

func TestInvalidator_ClearEvents(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Repository.CommitHash = "newcommit"
	inv.CheckAndInvalidate(cached, current, []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now()}})

	inv.ClearEvents()
	if len(inv.Events()) != 0 {
		t.Error("expected 0 after clear")
	}
}

func TestInvalidator_SetMaxEvents(t *testing.T) {
	inv := NewInvalidator()
	inv.SetMaxEvents(2)
	cached := buildInvTestIdentity()
	entries := []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now()}}

	for i := 0; i < 5; i++ {
		c := buildInvTestIdentity()
		c.Repository.CommitHash = "commit" + string(rune('a'+i))
		inv.CheckAndInvalidate(cached, c, entries)
	}

	if len(inv.Events()) > 2 {
		t.Errorf("expected <=2 events, got %d", len(inv.Events()))
	}
}

func TestInvalidator_OnInvalidation(t *testing.T) {
	inv := NewInvalidator()
	var mu sync.Mutex
	var received []InvalidationEvent

	inv.OnInvalidation(func(event InvalidationEvent) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, event)
	})

	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Repository.CommitHash = "newcommit"
	inv.CheckAndInvalidate(cached, current, []CacheEntry{{Key: "key1", Model: "gpt-4", CreatedAt: time.Now()}})

	mu.Lock()
	count := len(received)
	mu.Unlock()
	if count != 1 {
		t.Errorf("expected 1 notification, got %d", count)
	}
}

func TestInvalidator_ScenarioRepoChangeInvalidatesCache(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity() // commit abc123
	current := buildInvTestIdentity()
	current.Repository.CommitHash = "def456"

	entries := []CacheEntry{{
		Key: "cache:gpt-4:abc123", Model: "gpt-4",
		CreatedAt: time.Now().Add(-1 * time.Hour), ContextIdentity: cached,
	}}

	keys, event := inv.CheckAndInvalidate(cached, current, entries)

	if len(keys) != 1 || keys[0] != "cache:gpt-4:abc123" {
		t.Fatalf("expected [cache:gpt-4:abc123], got %v", keys)
	}
	if event == nil {
		t.Fatal("expected event")
	}
	if event.Reason != ReasonRepositoryChange {
		t.Errorf("expected %s, got %s", ReasonRepositoryChange, event.Reason)
	}
	if event.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestInvalidator_ScenarioToolSchemaV2InvalidatesToolResponses(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.ToolSchemas = []ToolSchemaVersion{{Name: "search", Version: "v2", SchemaHash: "newschema"}}

	entries := []CacheEntry{
		{Key: "cache:gpt-4:tool-response", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached, HasToolCalls: true},
		{Key: "cache:gpt-4:text-response", Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached, HasToolCalls: false},
	}

	keys, event := inv.CheckAndInvalidate(cached, current, entries)
	if len(keys) != 2 {
		t.Errorf("expected 2 (full scope), got %d: %v", len(keys), keys)
	}
	if event == nil {
		t.Fatal("expected event")
	}
}

func TestInvalidator_ScenarioEventObservability(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Repository.CommitHash = "newcommit"

	entries := []CacheEntry{
		{Key: "key1", Model: "gpt-4", CreatedAt: time.Now()},
		{Key: "key2", Model: "gpt-4", CreatedAt: time.Now()},
	}

	_, event := inv.CheckAndInvalidate(cached, current, entries)
	if event == nil {
		t.Fatal("expected event")
	}
	if event.Reason == "" {
		t.Error("missing reason")
	}
	if len(event.AffectedKeys) == 0 {
		t.Error("missing affected keys")
	}
	if event.Timestamp.IsZero() {
		t.Error("missing timestamp")
	}
	if event.Decision.Scope == "" {
		t.Error("missing scope")
	}
	if len(event.Changes) == 0 {
		t.Error("missing changes")
	}
}

func TestInvalidationFilter_Matches(t *testing.T) {
	now := time.Now()
	before := now.Add(-1 * time.Hour)

	t.Run("namespace", func(t *testing.T) {
		f := InvalidationFilter{Namespace: "ns1:"}
		if !f.Matches("ns1:key", "gpt-4", now) {
			t.Error("expected match")
		}
		if f.Matches("ns2:key", "gpt-4", now) {
			t.Error("expected no match")
		}
	})

	t.Run("model", func(t *testing.T) {
		f := InvalidationFilter{Model: "gpt-4"}
		if !f.Matches("key", "gpt-4", now) {
			t.Error("expected match")
		}
		if f.Matches("key", "claude-3", now) {
			t.Error("expected no match")
		}
	})

	t.Run("before", func(t *testing.T) {
		f := InvalidationFilter{Before: &now}
		if !f.Matches("key", "gpt-4", before) {
			t.Error("expected match")
		}
		if f.Matches("key", "gpt-4", now.Add(time.Hour)) {
			t.Error("expected no match")
		}
	})

	t.Run("after", func(t *testing.T) {
		f := InvalidationFilter{After: &before}
		if !f.Matches("key", "gpt-4", now) {
			t.Error("expected match")
		}
		if f.Matches("key", "gpt-4", before.Add(-time.Hour)) {
			t.Error("expected no match")
		}
	})

	t.Run("combined", func(t *testing.T) {
		f := InvalidationFilter{Namespace: "ns1:", Model: "gpt-4"}
		if !f.Matches("ns1:key", "gpt-4", now) {
			t.Error("expected match")
		}
		if f.Matches("ns1:key", "claude-3", now) {
			t.Error("expected no match")
		}
		if f.Matches("ns2:key", "gpt-4", now) {
			t.Error("expected no match")
		}
	})

	t.Run("empty matches all", func(t *testing.T) {
		f := InvalidationFilter{}
		if !f.Matches("any:key", "any-model", now) {
			t.Error("expected match")
		}
	})
}

func TestInvalidator_IntegrationWithCacheDelete(t *testing.T) {
	inv := NewInvalidator()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Repository.CommitHash = "newcommit"

	cacheStore := map[string]bool{"key1": true, "key2": true, "key3": true}
	entries := make([]CacheEntry, 0, len(cacheStore))
	for k := range cacheStore {
		entries = append(entries, CacheEntry{Key: k, Model: "gpt-4", CreatedAt: time.Now(), ContextIdentity: cached})
	}

	keysToDelete, event := inv.CheckAndInvalidate(cached, current, entries)
	if event == nil {
		t.Fatal("expected event")
	}
	for _, key := range keysToDelete {
		delete(cacheStore, key)
	}
	if len(cacheStore) != 0 {
		t.Errorf("expected all deleted, %d remaining", len(cacheStore))
	}
}

func TestInvalidator_RuleEngine(t *testing.T) {
	if NewInvalidator().RuleEngine() == nil {
		t.Error("expected non-nil")
	}
}

func TestInvalidator_ChangeDetector(t *testing.T) {
	if NewInvalidator().ChangeDetector() == nil {
		t.Error("expected non-nil")
	}
}

func TestInvalidatorWithEngine(t *testing.T) {
	engine := NewEmptyRuleEngine()
	engine.AddRule(&RepositoryChangeRule{})
	inv := NewInvalidatorWithEngine(engine)
	rules := inv.RuleEngine().Rules()
	if len(rules) != 1 || rules[0] != "repository_change" {
		t.Errorf("expected [repository_change], got %v", rules)
	}
}

func TestInvalidator_SelectiveInvalidate_ByAfterTime(t *testing.T) {
	inv := NewInvalidator()
	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)

	entries := []CacheEntry{
		{Key: "old", Model: "gpt-4", CreatedAt: now.Add(-2 * time.Hour)},
		{Key: "new", Model: "gpt-4", CreatedAt: now},
	}

	keys, _ := inv.SelectiveInvalidate(entries, InvalidationFilter{After: &cutoff})
	if len(keys) != 1 || keys[0] != "new" {
		t.Errorf("expected [new], got %v", keys)
	}
}
