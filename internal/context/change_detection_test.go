package context

import (
	"testing"
)

func TestChangeDetector_DetectChanges_NoChanges(t *testing.T) {
	detector := NewChangeDetector()
	identity := buildInvTestIdentity()

	changes := detector.DetectChanges(identity, identity)
	if len(changes) != 0 {
		t.Errorf("expected no changes for identical identities, got %d", len(changes))
	}
}

func TestChangeDetector_DetectChanges_NilInputs(t *testing.T) {
	detector := NewChangeDetector()
	identity := buildInvTestIdentity()

	if changes := detector.DetectChanges(nil, identity); changes != nil {
		t.Error("expected nil changes when cached is nil")
	}
	if changes := detector.DetectChanges(identity, nil); changes != nil {
		t.Error("expected nil changes when current is nil")
	}
	if changes := detector.DetectChanges(nil, nil); changes != nil {
		t.Error("expected nil changes when both are nil")
	}
}

func TestChangeDetector_DetectChanges_RepositoryCommitChange(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Repository.CommitHash = "def456"

	changes := detector.DetectChanges(cached, current)
	if len(changes) == 0 {
		t.Fatal("expected changes for repository commit change")
	}

	found := false
	for _, c := range changes {
		if c.Name == "repository.commit_hash" {
			found = true
			if c.Category != FactorCategoryRepository {
				t.Errorf("expected category %s, got %s", FactorCategoryRepository, c.Category)
			}
			if c.ChangeType != ChangeTypeModified {
				t.Errorf("expected change type %s, got %s", ChangeTypeModified, c.ChangeType)
			}
			if c.OldValue != "abc123" {
				t.Errorf("expected old value 'abc123', got %q", c.OldValue)
			}
			if c.NewValue != "def456" {
				t.Errorf("expected new value 'def456', got %q", c.NewValue)
			}
		}
	}
	if !found {
		t.Error("expected to find repository.commit_hash change")
	}
}

func TestChangeDetector_DetectChanges_ModelChange(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Model.Name = "gpt-4-turbo"

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "model.name" && c.OldValue == "gpt-4" && c.NewValue == "gpt-4-turbo" {
			found = true
		}
	}
	if !found {
		t.Error("expected to find model.name change")
	}
}

func TestChangeDetector_DetectChanges_PromptChange(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Prompt.SystemPromptHash = "newhash456"

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "prompt.system_prompt_hash" {
			found = true
			if c.Category != FactorCategoryPrompt {
				t.Errorf("expected category %s, got %s", FactorCategoryPrompt, c.Category)
			}
		}
	}
	if !found {
		t.Error("expected to find prompt.system_prompt_hash change")
	}
}

func TestChangeDetector_DetectChanges_DependencyChange(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.DependencyLocks = []DependencyLock{
		{Type: "go.sum", Hash: "newhash"},
	}

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "dependency.go.sum" && c.ChangeType == ChangeTypeModified {
			found = true
		}
	}
	if !found {
		t.Error("expected to find dependency.go.sum change")
	}
}

func TestChangeDetector_DetectChanges_ToolSchemaChange(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.ToolSchemas = []ToolSchemaVersion{
		{Name: "search", Version: "v2", SchemaHash: "newschema"},
	}

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "tool_schema.search" && c.ChangeType == ChangeTypeModified {
			found = true
			if c.OldValue != "v1" {
				t.Errorf("expected old value 'v1', got %q", c.OldValue)
			}
			if c.NewValue != "v2" {
				t.Errorf("expected new value 'v2', got %q", c.NewValue)
			}
		}
	}
	if !found {
		t.Error("expected to find tool_schema.search change")
	}
}

func TestChangeDetector_DetectChanges_FileAdded(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.FileHashes = append(current.FileHashes, FileHash{Path: "new.go", Hash: "filehash"})

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "file.new.go" && c.ChangeType == ChangeTypeAdded {
			found = true
		}
	}
	if !found {
		t.Error("expected to find file.new.go addition")
	}
}

func TestChangeDetector_DetectChanges_FileRemoved(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.FileHashes = nil

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "file.main.go" && c.ChangeType == ChangeTypeRemoved {
			found = true
		}
	}
	if !found {
		t.Error("expected to find file.main.go removal")
	}
}

func TestChangeDetector_DetectChanges_ConversationAdded(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	cached.Conversation = nil
	current := buildInvTestIdentity()
	current.Conversation = &ConversationState{TurnCount: 3, HistoryHash: "convhash"}

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "conversation" && c.ChangeType == ChangeTypeAdded {
			found = true
		}
	}
	if !found {
		t.Error("expected to find conversation addition")
	}
}

func TestChangeDetector_DetectChanges_ConversationRemoved(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	cached.Conversation = &ConversationState{TurnCount: 3, HistoryHash: "convhash"}
	current := buildInvTestIdentity()
	current.Conversation = nil

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "conversation" && c.ChangeType == ChangeTypeRemoved {
			found = true
		}
	}
	if !found {
		t.Error("expected to find conversation removal")
	}
}

func TestChangeDetector_DetectChanges_CustomFactors(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	cached.CustomFactors = map[string]string{"env": "prod", "region": "us-east-1"}
	current := buildInvTestIdentity()
	current.CustomFactors = map[string]string{"env": "staging", "zone": "a"}

	changes := detector.DetectChanges(cached, current)
	changeMap := make(map[string]ChangedFactor)
	for _, c := range changes {
		changeMap[c.Name] = c
	}

	if c, ok := changeMap["custom.env"]; !ok {
		t.Error("expected custom.env change")
	} else if c.ChangeType != ChangeTypeModified || c.OldValue != "prod" || c.NewValue != "staging" {
		t.Errorf("unexpected custom.env change: %+v", c)
	}

	if c, ok := changeMap["custom.region"]; !ok {
		t.Error("expected custom.region change")
	} else if c.ChangeType != ChangeTypeRemoved {
		t.Errorf("expected removed, got %s", c.ChangeType)
	}

	if c, ok := changeMap["custom.zone"]; !ok {
		t.Error("expected custom.zone change")
	} else if c.ChangeType != ChangeTypeAdded {
		t.Errorf("expected added, got %s", c.ChangeType)
	}
}

func TestChangeDetector_DetectChanges_ParameterChange(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	newTemp := 0.9
	current.Parameters.Temperature = &newTemp

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "params.temperature" && c.ChangeType == ChangeTypeModified {
			found = true
		}
	}
	if !found {
		t.Error("expected to find params.temperature change")
	}
}

func TestChangeDetector_DetectChanges_ParameterAdded(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	cached.Parameters.Temperature = nil
	current := buildInvTestIdentity()

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "params.temperature" && c.ChangeType == ChangeTypeAdded {
			found = true
		}
	}
	if !found {
		t.Error("expected to find params.temperature addition")
	}
}

func TestChangeDetector_DetectChanges_ParameterRemoved(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Parameters.Temperature = nil

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "params.temperature" && c.ChangeType == ChangeTypeRemoved {
			found = true
		}
	}
	if !found {
		t.Error("expected to find params.temperature removal")
	}
}

func TestChangeDetector_DetectChanges_RepositoryAdded(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	cached.Repository = nil
	current := buildInvTestIdentity()

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "repository" && c.ChangeType == ChangeTypeAdded {
			found = true
		}
	}
	if !found {
		t.Error("expected to find repository addition")
	}
}

func TestChangeDetector_DetectChanges_RepositoryRemoved(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Repository = nil

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "repository" && c.ChangeType == ChangeTypeRemoved {
			found = true
		}
	}
	if !found {
		t.Error("expected to find repository removal")
	}
}

func TestChangeDetector_DetectChanges_ToolOutputChange(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.ToolOutputs = []ToolOutputIdentity{
		{ToolName: "search", OutputHash: "newoutput"},
	}

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "tool_output.search" && c.ChangeType == ChangeTypeModified {
			found = true
		}
	}
	if !found {
		t.Error("expected to find tool_output.search change")
	}
}

func TestChangeDetector_HasChanges(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()

	if detector.HasChanges(cached, current) {
		t.Error("expected no changes for identical identities")
	}

	current.Model.Name = "different"
	if !detector.HasChanges(cached, current) {
		t.Error("expected changes for different model")
	}
}

func TestChangeDetector_ChangedCategories(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Model.Name = "gpt-4-turbo"
	current.Repository.CommitHash = "newcommit"

	cats := detector.ChangedCategories(cached, current)
	if len(cats) != 2 {
		t.Fatalf("expected 2 changed categories, got %d: %v", len(cats), cats)
	}

	catMap := make(map[FactorCategory]bool)
	for _, c := range cats {
		catMap[c] = true
	}
	if !catMap[FactorCategoryModel] {
		t.Error("expected model category")
	}
	if !catMap[FactorCategoryRepository] {
		t.Error("expected repository category")
	}
}

func TestChangeDetector_MaxChangeWeight(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()

	if w := detector.MaxChangeWeight(cached, current); w != 0 {
		t.Errorf("expected 0, got %v", w)
	}

	current.DependencyLocks[0].Hash = "newhash"
	w := detector.MaxChangeWeight(cached, current)
	if w != DefaultWeights[FactorCategoryDependency] {
		t.Errorf("expected weight %v, got %v", DefaultWeights[FactorCategoryDependency], w)
	}
}

func TestChangeDetector_DetectChanges_SortedOutput(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.Model.Name = "gpt-5"
	current.Repository.CommitHash = "newcommit"
	current.DependencyLocks[0].Hash = "newhash"

	changes := detector.DetectChanges(cached, current)
	for i := 1; i < len(changes); i++ {
		if changes[i].Category < changes[i-1].Category {
			t.Errorf("not sorted by category: %s < %s", changes[i].Category, changes[i-1].Category)
		}
		if changes[i].Category == changes[i-1].Category && changes[i].Name < changes[i-1].Name {
			t.Errorf("not sorted by name: %s < %s", changes[i].Name, changes[i-1].Name)
		}
	}
}

func TestChangeDetector_DetectChanges_DependencyAdded(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.DependencyLocks = append(current.DependencyLocks, DependencyLock{Type: "package-lock.json", Hash: "npmhash"})

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "dependency.package-lock.json" && c.ChangeType == ChangeTypeAdded {
			found = true
		}
	}
	if !found {
		t.Error("expected dependency.package-lock.json addition")
	}
}

func TestChangeDetector_DetectChanges_ToolSchemaAdded(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.ToolSchemas = append(current.ToolSchemas, ToolSchemaVersion{Name: "calculator", Version: "v1", SchemaHash: "calchash"})

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "tool_schema.calculator" && c.ChangeType == ChangeTypeAdded {
			found = true
		}
	}
	if !found {
		t.Error("expected tool_schema.calculator addition")
	}
}

func TestChangeDetector_DetectChanges_ToolSchemaRemoved(t *testing.T) {
	detector := NewChangeDetector()
	cached := buildInvTestIdentity()
	current := buildInvTestIdentity()
	current.ToolSchemas = nil

	changes := detector.DetectChanges(cached, current)
	found := false
	for _, c := range changes {
		if c.Name == "tool_schema.search" && c.ChangeType == ChangeTypeRemoved {
			found = true
		}
	}
	if !found {
		t.Error("expected tool_schema.search removal")
	}
}

// buildInvTestIdentity creates a fully populated ContextIdentity for invalidation tests.
func buildInvTestIdentity() *ContextIdentity {
	temp := 0.7
	return &ContextIdentity{
		Version: "1.0.0",
		Model: ModelIdentity{
			Name:     "gpt-4",
			Version:  "2024-01",
			Provider: "openai",
		},
		Parameters: ModelParameters{
			Temperature: &temp,
		},
		Prompt: PromptIdentity{
			TemplateHash:     "tmplhash123",
			TemplateVersion:  "v1",
			SystemPromptHash: "syshash123",
		},
		Repository: &RepositoryIdentity{
			URL:        "https://github.com/example/repo",
			Branch:     "main",
			CommitHash: "abc123",
		},
		FileHashes: []FileHash{
			{Path: "main.go", Hash: "filehash1"},
		},
		DependencyLocks: []DependencyLock{
			{Type: "go.sum", Hash: "dephash1"},
		},
		ToolSchemas: []ToolSchemaVersion{
			{Name: "search", Version: "v1", SchemaHash: "schemahash1"},
		},
		ToolOutputs: []ToolOutputIdentity{
			{ToolName: "search", OutputHash: "outputhash1"},
		},
	}
}
