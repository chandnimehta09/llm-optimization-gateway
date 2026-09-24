package context

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/llm-optimization-gateway/internal/context/testcases"
	"github.com/llm-optimization-gateway/internal/context/testcases/fixtures"
)

// --- Unit tests for identity creation ---

func TestNewBuilder(t *testing.T) {
	identity, err := NewBuilder("gpt-4", "openai").Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity.Model.Name != "gpt-4" {
		t.Errorf("expected model name %q, got %q", "gpt-4", identity.Model.Name)
	}
	if identity.Model.Provider != "openai" {
		t.Errorf("expected provider %q, got %q", "openai", identity.Model.Provider)
	}
	if identity.Version != identityVersion {
		t.Errorf("expected version %q, got %q", identityVersion, identity.Version)
	}
	if identity.ID == "" {
		t.Error("expected non-empty identity ID")
	}
	if identity.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestBuilderFullIdentity(t *testing.T) {
	identity, err := NewBuilder("claude-3-opus", "anthropic").
		WithModelVersion("20240229").
		WithTemperature(0.7).
		WithTopP(0.9).
		WithMaxTokens(4096).
		WithTopK(40).
		WithFrequencyPenalty(0.5).
		WithPresencePenalty(0.3).
		WithSeed(42).
		WithStop([]string{"STOP", "END"}).
		WithPrompt("abc123", "v2.1", "def456").
		WithRepository("https://github.com/example/repo", "main", "abc123def").
		WithFileHash("main.go", "hash1").
		WithFileHash("go.mod", "hash2").
		WithDependencyLock("go.sum", "lockHash1").
		WithDependencyLock("package-lock.json", "lockHash2").
		WithToolSchema("search", "v1", "schemaHash1").
		WithToolOutput("search", "outputHash1").
		WithConversation(5, "convHash").
		WithRetrievedContext("queryHash1", map[string]string{"doc1": "docHash1", "doc2": "docHash2"}).
		WithCustomFactor("environment", "production").
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all fields
	if identity.Model.Version != "20240229" {
		t.Errorf("expected version %q, got %q", "20240229", identity.Model.Version)
	}
	if identity.Parameters.Temperature == nil || *identity.Parameters.Temperature != 0.7 {
		t.Error("temperature not set correctly")
	}
	if identity.Parameters.TopP == nil || *identity.Parameters.TopP != 0.9 {
		t.Error("top_p not set correctly")
	}
	if identity.Parameters.MaxTokens == nil || *identity.Parameters.MaxTokens != 4096 {
		t.Error("max_tokens not set correctly")
	}
	if identity.Parameters.TopK == nil || *identity.Parameters.TopK != 40 {
		t.Error("top_k not set correctly")
	}
	if identity.Parameters.FrequencyPenalty == nil || *identity.Parameters.FrequencyPenalty != 0.5 {
		t.Error("frequency_penalty not set correctly")
	}
	if identity.Parameters.PresencePenalty == nil || *identity.Parameters.PresencePenalty != 0.3 {
		t.Error("presence_penalty not set correctly")
	}
	if identity.Parameters.Seed == nil || *identity.Parameters.Seed != 42 {
		t.Error("seed not set correctly")
	}
	if len(identity.Parameters.Stop) != 2 {
		t.Errorf("expected 2 stop sequences, got %d", len(identity.Parameters.Stop))
	}
	if identity.Prompt.TemplateHash != "abc123" {
		t.Errorf("expected template hash %q, got %q", "abc123", identity.Prompt.TemplateHash)
	}
	if identity.Repository == nil || identity.Repository.CommitHash != "abc123def" {
		t.Error("repository not set correctly")
	}
	if len(identity.FileHashes) != 2 {
		t.Errorf("expected 2 file hashes, got %d", len(identity.FileHashes))
	}
	if len(identity.DependencyLocks) != 2 {
		t.Errorf("expected 2 dependency locks, got %d", len(identity.DependencyLocks))
	}
	if len(identity.ToolSchemas) != 1 {
		t.Errorf("expected 1 tool schema, got %d", len(identity.ToolSchemas))
	}
	if len(identity.ToolOutputs) != 1 {
		t.Errorf("expected 1 tool output, got %d", len(identity.ToolOutputs))
	}
	if identity.Conversation == nil || identity.Conversation.TurnCount != 5 {
		t.Error("conversation not set correctly")
	}
	if len(identity.RetrievedContexts) != 1 {
		t.Errorf("expected 1 retrieved context, got %d", len(identity.RetrievedContexts))
	}
	if identity.CustomFactors["environment"] != "production" {
		t.Errorf("expected custom factor 'environment' = 'production', got %q", identity.CustomFactors["environment"])
	}
	if identity.ID == "" {
		t.Error("expected computed identity ID")
	}
}

// --- Unit tests for factor extraction ---

func TestStaticFactor(t *testing.T) {
	f := StaticFactor{
		FactorName:     "test_factor",
		FactorCategory: FactorCategoryCustom,
		FactorValue:    "test_value",
		FactorWeight:   WeightHigh,
	}

	if f.Name() != "test_factor" {
		t.Errorf("expected name %q, got %q", "test_factor", f.Name())
	}
	if f.Category() != FactorCategoryCustom {
		t.Errorf("expected category %q, got %q", FactorCategoryCustom, f.Category())
	}
	if f.Value() != "test_value" {
		t.Errorf("expected value %q, got %q", "test_value", f.Value())
	}
	if f.Weight() != WeightHigh {
		t.Errorf("expected weight %v, got %v", WeightHigh, f.Weight())
	}
}

func TestStaticFactorDefaultWeight(t *testing.T) {
	f := StaticFactor{
		FactorName:     "model_factor",
		FactorCategory: FactorCategoryModel,
		FactorValue:    "value",
		FactorWeight:   0, // should fall back to category default
	}

	if f.Weight() != WeightCritical {
		t.Errorf("expected default weight %v for model category, got %v", WeightCritical, f.Weight())
	}
}

func TestStaticFactorUnknownCategoryDefaultWeight(t *testing.T) {
	f := StaticFactor{
		FactorName:     "unknown_factor",
		FactorCategory: FactorCategory("unknown"),
		FactorValue:    "value",
		FactorWeight:   0, // should fall back to WeightMedium
	}

	if f.Weight() != WeightMedium {
		t.Errorf("expected fallback weight %v, got %v", WeightMedium, f.Weight())
	}
}

func TestFactorRegistry(t *testing.T) {
	registry := NewFactorRegistry()

	registry.Register("env", func() ([]ContextFactor, error) {
		return []ContextFactor{
			StaticFactor{
				FactorName:     "runtime_env",
				FactorCategory: FactorCategoryCustom,
				FactorValue:    "production",
			},
		}, nil
	})

	registry.Register("version", func() ([]ContextFactor, error) {
		return []ContextFactor{
			StaticFactor{
				FactorName:     "app_version",
				FactorCategory: FactorCategoryCustom,
				FactorValue:    "1.2.3",
			},
		}, nil
	})

	factors, err := registry.Extract()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(factors) != 2 {
		t.Fatalf("expected 2 factors, got %d", len(factors))
	}

	// Factors should be in sorted order by extractor name (env before version)
	if factors[0].Name() != "runtime_env" {
		t.Errorf("expected first factor 'runtime_env', got %q", factors[0].Name())
	}
	if factors[1].Name() != "app_version" {
		t.Errorf("expected second factor 'app_version', got %q", factors[1].Name())
	}
}

func TestFactorRegistryError(t *testing.T) {
	registry := NewFactorRegistry()
	registry.Register("broken", func() ([]ContextFactor, error) {
		return nil, fmt.Errorf("extraction failed")
	})

	_, err := registry.Extract()
	if err == nil {
		t.Fatal("expected error from broken extractor")
	}
}

func TestFactorRegistryUnregister(t *testing.T) {
	registry := NewFactorRegistry()
	registry.Register("test", func() ([]ContextFactor, error) {
		return []ContextFactor{
			StaticFactor{FactorName: "test"},
		}, nil
	})

	existed := registry.Unregister("test")
	if !existed {
		t.Error("expected Unregister to return true for existing extractor")
	}

	existed = registry.Unregister("nonexistent")
	if existed {
		t.Error("expected Unregister to return false for non-existing extractor")
	}

	factors, err := registry.Extract()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(factors) != 0 {
		t.Errorf("expected 0 factors after unregister, got %d", len(factors))
	}
}

func TestFactorRegistryNames(t *testing.T) {
	registry := NewFactorRegistry()
	registry.Register("beta", func() ([]ContextFactor, error) { return nil, nil })
	registry.Register("alpha", func() ([]ContextFactor, error) { return nil, nil })
	registry.Register("gamma", func() ([]ContextFactor, error) { return nil, nil })

	names := registry.Names()
	expected := []string{"alpha", "beta", "gamma"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, n := range names {
		if n != expected[i] {
			t.Errorf("expected name[%d] = %q, got %q", i, expected[i], n)
		}
	}
}

func TestBuilderWithRegistry(t *testing.T) {
	registry := NewFactorRegistry()
	registry.Register("env", func() ([]ContextFactor, error) {
		return []ContextFactor{
			StaticFactor{
				FactorName:     "runtime_env",
				FactorCategory: FactorCategoryCustom,
				FactorValue:    "staging",
			},
		}, nil
	})

	identity, err := NewBuilder("gpt-4", "openai").
		WithRegistry(registry).
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if identity.CustomFactors["runtime_env"] != "staging" {
		t.Errorf("expected custom factor 'runtime_env' = 'staging', got %q", identity.CustomFactors["runtime_env"])
	}
}

func TestBuilderWithRegistryError(t *testing.T) {
	registry := NewFactorRegistry()
	registry.Register("broken", func() ([]ContextFactor, error) {
		return nil, fmt.Errorf("broken extractor")
	})

	_, err := NewBuilder("gpt-4", "openai").
		WithRegistry(registry).
		Build()
	if err == nil {
		t.Fatal("expected error from broken registry")
	}
}

func TestHashFactors(t *testing.T) {
	factors := []ContextFactor{
		StaticFactor{FactorName: "b", FactorValue: "2"},
		StaticFactor{FactorName: "a", FactorValue: "1"},
	}

	hash1 := HashFactors(factors)
	if hash1 == "" {
		t.Fatal("expected non-empty hash")
	}

	// Reverse order should produce same hash (sorted internally)
	factors2 := []ContextFactor{
		StaticFactor{FactorName: "a", FactorValue: "1"},
		StaticFactor{FactorName: "b", FactorValue: "2"},
	}

	hash2 := HashFactors(factors2)
	if hash1 != hash2 {
		t.Errorf("expected same hash regardless of order, got %q and %q", hash1, hash2)
	}
}

func TestHashFactorsEmpty(t *testing.T) {
	hash := HashFactors(nil)
	if hash != "" {
		t.Errorf("expected empty hash for nil factors, got %q", hash)
	}

	hash = HashFactors([]ContextFactor{})
	if hash != "" {
		t.Errorf("expected empty hash for empty factors, got %q", hash)
	}
}

func TestHashFactorsDifferentValues(t *testing.T) {
	factors1 := []ContextFactor{
		StaticFactor{FactorName: "key", FactorValue: "value1"},
	}
	factors2 := []ContextFactor{
		StaticFactor{FactorName: "key", FactorValue: "value2"},
	}

	hash1 := HashFactors(factors1)
	hash2 := HashFactors(factors2)
	if hash1 == hash2 {
		t.Error("expected different hashes for different values")
	}
}

func TestFormatFactorSummary(t *testing.T) {
	factors := []ContextFactor{
		StaticFactor{FactorName: "model_name", FactorCategory: FactorCategoryModel, FactorValue: "gpt-4", FactorWeight: WeightCritical},
		StaticFactor{FactorName: "env", FactorCategory: FactorCategoryCustom, FactorValue: "prod", FactorWeight: WeightMedium},
	}

	summary := FormatFactorSummary(factors)
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	// Should contain both categories
	if !containsSubstring(summary, "[custom]") {
		t.Error("expected summary to contain [custom]")
	}
	if !containsSubstring(summary, "[model]") {
		t.Error("expected summary to contain [model]")
	}
}

// --- Acceptance criteria: serialization roundtrip ---

func TestSerializationRoundtrip(t *testing.T) {
	original, err := NewBuilder("gpt-4", "openai").
		WithModelVersion("0613").
		WithTemperature(0.7).
		WithTopP(0.9).
		WithMaxTokens(2048).
		WithSeed(42).
		WithStop([]string{"END"}).
		WithPrompt("tmplHash", "v1", "sysHash").
		WithRepository("https://github.com/test/repo", "main", "abc123").
		WithFileHash("main.go", "fileHash1").
		WithDependencyLock("go.sum", "depHash1").
		WithToolSchema("search", "v1", "schemaHash").
		WithToolOutput("search", "outputHash").
		WithConversation(3, "histHash").
		WithRetrievedContext("qHash", map[string]string{"doc1": "dHash1"}).
		WithCustomFactor("env", "test").
		Build()
	if err != nil {
		t.Fatalf("unexpected error building identity: %v", err)
	}

	// Serialize to JSON
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("unexpected error marshaling: %v", err)
	}

	// Deserialize from JSON
	var restored ContextIdentity
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unexpected error unmarshaling: %v", err)
	}

	// The restored identity should have the same ID
	if restored.ID != original.ID {
		t.Errorf("ID mismatch after roundtrip: %q vs %q", original.ID, restored.ID)
	}

	// Recompute ID on restored should match
	recomputedID := restored.ComputeID()
	if recomputedID != original.ID {
		t.Errorf("recomputed ID mismatch: original %q, recomputed %q", original.ID, recomputedID)
	}

	// Verify fields survived roundtrip
	if restored.Model.Name != "gpt-4" {
		t.Errorf("model name mismatch: %q", restored.Model.Name)
	}
	if restored.Model.Version != "0613" {
		t.Errorf("model version mismatch: %q", restored.Model.Version)
	}
	if restored.Parameters.Temperature == nil || *restored.Parameters.Temperature != 0.7 {
		t.Error("temperature not preserved")
	}
	if restored.Repository == nil || restored.Repository.CommitHash != "abc123" {
		t.Error("repository not preserved")
	}
	if len(restored.FileHashes) != 1 {
		t.Errorf("expected 1 file hash, got %d", len(restored.FileHashes))
	}
	if restored.Conversation == nil || restored.Conversation.TurnCount != 3 {
		t.Error("conversation not preserved")
	}
	if restored.CustomFactors["env"] != "test" {
		t.Error("custom factor not preserved")
	}
}

// --- Acceptance criteria: different repository commits produce different identities ---

func TestDifferentRepositoryCommitsProduceDifferentIdentities(t *testing.T) {
	build := func(commit string) *ContextIdentity {
		id, err := NewBuilder("gpt-4", "openai").
			WithPrompt("tmplHash", "v1", "sysHash").
			WithRepository("https://github.com/test/repo", "main", commit).
			Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return id
	}

	id1 := build("commit-aaa111")
	id2 := build("commit-bbb222")

	if id1.ID == id2.ID {
		t.Error("expected different identity IDs for different repository commits")
	}

	if id1.Equal(id2) {
		t.Error("expected identities to not be equal")
	}
}

// --- Acceptance criteria: same context produces identical identities ---

func TestSameContextProducesIdenticalIdentities(t *testing.T) {
	build := func() *ContextIdentity {
		id, err := NewBuilder("gpt-4", "openai").
			WithTemperature(0.7).
			WithPrompt("tmplHash", "v1", "sysHash").
			WithRepository("https://github.com/test/repo", "main", "abc123").
			WithFileHash("main.go", "fileHash").
			Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return id
	}

	id1 := build()
	id2 := build()

	if id1.ID != id2.ID {
		t.Errorf("expected identical IDs for same context, got %q and %q", id1.ID, id2.ID)
	}

	if !id1.Equal(id2) {
		t.Error("expected identities to be equal")
	}
}

// --- Property-based tests for identity uniqueness ---

func TestIdentityUniquenessModelName(t *testing.T) {
	models := []string{"gpt-4", "gpt-3.5-turbo", "claude-3-opus", "gemini-pro"}
	ids := make(map[string]string)

	for _, model := range models {
		identity, err := NewBuilder(model, "provider").
			WithPrompt("hash", "v1", "sys").
			Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if prev, exists := ids[identity.ID]; exists {
			t.Errorf("collision: model %q produced same ID as model %q", model, prev)
		}
		ids[identity.ID] = model
	}
}

func TestIdentityUniquenessTemperature(t *testing.T) {
	temps := []float64{0.0, 0.1, 0.5, 0.7, 1.0, 2.0}
	ids := make(map[string]float64)

	for _, temp := range temps {
		identity, err := NewBuilder("gpt-4", "openai").
			WithTemperature(temp).
			WithPrompt("hash", "v1", "sys").
			Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if prev, exists := ids[identity.ID]; exists {
			t.Errorf("collision: temperature %v produced same ID as %v", temp, prev)
		}
		ids[identity.ID] = temp
	}
}

func TestIdentityUniquenessFileHashes(t *testing.T) {
	identity1, err := NewBuilder("gpt-4", "openai").
		WithFileHash("main.go", "hash_a").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	identity2, err := NewBuilder("gpt-4", "openai").
		WithFileHash("main.go", "hash_b").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if identity1.ID == identity2.ID {
		t.Error("expected different IDs for different file hashes")
	}
}

func TestIdentityUniquenessCustomFactors(t *testing.T) {
	identity1, err := NewBuilder("gpt-4", "openai").
		WithCustomFactor("env", "production").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	identity2, err := NewBuilder("gpt-4", "openai").
		WithCustomFactor("env", "staging").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if identity1.ID == identity2.ID {
		t.Error("expected different IDs for different custom factor values")
	}
}

func TestIdentityUniquenessConversationState(t *testing.T) {
	identity1, err := NewBuilder("gpt-4", "openai").
		WithConversation(3, "histHashA").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	identity2, err := NewBuilder("gpt-4", "openai").
		WithConversation(4, "histHashA").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	identity3, err := NewBuilder("gpt-4", "openai").
		WithConversation(3, "histHashB").
		Build()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if identity1.ID == identity2.ID {
		t.Error("expected different IDs for different turn counts")
	}
	if identity1.ID == identity3.ID {
		t.Error("expected different IDs for different history hashes")
	}
}

// --- Tests for identity comparison ---

func TestCompareExactMatch(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").
		WithTemperature(0.7).
		WithPrompt("hash", "v1", "sys").
		WithRepository("url", "main", "abc123").
		Build()

	identity2, _ := NewBuilder("gpt-4", "openai").
		WithTemperature(0.7).
		WithPrompt("hash", "v1", "sys").
		WithRepository("url", "main", "abc123").
		Build()

	result := identity1.Compare(identity2)
	if !result.IsExactMatch {
		t.Error("expected exact match")
	}
	if result.Similarity != 1.0 {
		t.Errorf("expected similarity 1.0, got %f", result.Similarity)
	}
	if len(result.Differences) != 0 {
		t.Errorf("expected no differences, got %v", result.Differences)
	}
}

func TestComparePartialMatch(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").
		WithTemperature(0.7).
		WithPrompt("hash", "v1", "sys").
		WithRepository("url", "main", "commit1").
		Build()

	identity2, _ := NewBuilder("gpt-4", "openai").
		WithTemperature(0.7).
		WithPrompt("hash", "v1", "sys").
		WithRepository("url", "main", "commit2").
		Build()

	result := identity1.Compare(identity2)
	if result.IsExactMatch {
		t.Error("expected non-exact match")
	}
	if result.Similarity <= 0.0 || result.Similarity >= 1.0 {
		t.Errorf("expected partial similarity, got %f", result.Similarity)
	}
	if len(result.Differences) == 0 {
		t.Error("expected some differences")
	}
}

func TestCompareNilOther(t *testing.T) {
	identity, _ := NewBuilder("gpt-4", "openai").Build()

	result := identity.Compare(nil)
	if result.IsExactMatch {
		t.Error("expected non-match for nil comparison")
	}
	if result.Similarity != 0.0 {
		t.Errorf("expected 0.0 similarity for nil, got %f", result.Similarity)
	}
}

func TestCompareDifferentModels(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").Build()
	identity2, _ := NewBuilder("claude-3", "anthropic").Build()

	result := identity1.Compare(identity2)
	if result.IsExactMatch {
		t.Error("expected non-match for different models")
	}
	if result.Similarity == 1.0 {
		t.Error("expected less than perfect similarity for different models")
	}
}

func TestEqualNilHandling(t *testing.T) {
	identity, _ := NewBuilder("gpt-4", "openai").Build()

	if identity.Equal(nil) {
		t.Error("expected non-nil identity to not equal nil")
	}

	var nilIdentity *ContextIdentity
	if nilIdentity.Equal(nil) != true {
		t.Error("expected nil to equal nil")
	}
}

func TestDiffReportsRepositoryDifferences(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").
		WithRepository("url", "main", "commit1").
		Build()

	identity2, _ := NewBuilder("gpt-4", "openai").
		WithRepository("url", "main", "commit2").
		Build()

	diffs := identity1.Diff(identity2)
	if len(diffs) == 0 {
		t.Fatal("expected diffs for different commits")
	}

	found := false
	for _, d := range diffs {
		if containsSubstring(d, "repository.commit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected diff about repository.commit, got %v", diffs)
	}
}

func TestDiffReportsConversationPresenceMismatch(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").
		WithConversation(3, "hash").
		Build()

	identity2, _ := NewBuilder("gpt-4", "openai").Build()

	diffs := identity1.Diff(identity2)
	found := false
	for _, d := range diffs {
		if containsSubstring(d, "conversation") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected diff about conversation presence, got %v", diffs)
	}
}

func TestDiffNilOther(t *testing.T) {
	identity, _ := NewBuilder("gpt-4", "openai").Build()
	diffs := identity.Diff(nil)
	if len(diffs) != 1 || diffs[0] != "other identity is nil" {
		t.Errorf("expected nil diff message, got %v", diffs)
	}
}

// --- Test deterministic ordering of file hashes ---

func TestFileHashOrderDoesNotAffectID(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").
		WithFileHash("b.go", "hash_b").
		WithFileHash("a.go", "hash_a").
		Build()

	identity2, _ := NewBuilder("gpt-4", "openai").
		WithFileHash("a.go", "hash_a").
		WithFileHash("b.go", "hash_b").
		Build()

	if identity1.ID != identity2.ID {
		t.Errorf("expected same ID regardless of file hash order, got %q and %q", identity1.ID, identity2.ID)
	}
}

func TestDependencyLockOrderDoesNotAffectID(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").
		WithDependencyLock("package-lock.json", "hash1").
		WithDependencyLock("go.sum", "hash2").
		Build()

	identity2, _ := NewBuilder("gpt-4", "openai").
		WithDependencyLock("go.sum", "hash2").
		WithDependencyLock("package-lock.json", "hash1").
		Build()

	if identity1.ID != identity2.ID {
		t.Errorf("expected same ID regardless of dep lock order, got %q and %q", identity1.ID, identity2.ID)
	}
}

func TestToolSchemaOrderDoesNotAffectID(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").
		WithToolSchema("search", "v1", "hash1").
		WithToolSchema("browse", "v2", "hash2").
		Build()

	identity2, _ := NewBuilder("gpt-4", "openai").
		WithToolSchema("browse", "v2", "hash2").
		WithToolSchema("search", "v1", "hash1").
		Build()

	if identity1.ID != identity2.ID {
		t.Errorf("expected same ID regardless of tool schema order, got %q and %q", identity1.ID, identity2.ID)
	}
}

func TestStopOrderDoesNotAffectID(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").
		WithStop([]string{"END", "STOP"}).
		Build()

	identity2, _ := NewBuilder("gpt-4", "openai").
		WithStop([]string{"STOP", "END"}).
		Build()

	if identity1.ID != identity2.ID {
		t.Errorf("expected same ID regardless of stop sequence order")
	}
}

// --- Test JSON marshaling recomputes ID ---

func TestMarshalJSONRecomputesID(t *testing.T) {
	identity, _ := NewBuilder("gpt-4", "openai").Build()
	originalID := identity.ID

	// Tamper with the ID
	identity.ID = "tampered"

	data, err := json.Marshal(identity)
	if err != nil {
		t.Fatalf("unexpected marshal error: %v", err)
	}

	var restored ContextIdentity
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if restored.ID != originalID {
		t.Errorf("expected marshaling to recompute ID to %q, got %q", originalID, restored.ID)
	}
}

// --- Test Compare weighted similarity ---

func TestCompareSimilarityScoreRange(t *testing.T) {
	identity1, _ := NewBuilder("gpt-4", "openai").
		WithTemperature(0.7).
		WithPrompt("hash1", "v1", "sys1").
		WithRepository("url", "main", "commit1").
		WithConversation(3, "histHash1").
		Build()

	// Completely different
	identity2, _ := NewBuilder("claude-3", "anthropic").
		WithTemperature(0.2).
		WithPrompt("hash2", "v2", "sys2").
		WithRepository("url2", "dev", "commit2").
		WithConversation(10, "histHash2").
		Build()

	result := identity1.Compare(identity2)
	if result.Similarity < 0.0 || result.Similarity > 1.0 {
		t.Errorf("similarity out of range: %f", result.Similarity)
	}
}

// helper
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// =============================================================================
// Helper: build identity from testcases.IdentityFieldSet
// =============================================================================

func buildIdentityFromFieldSet(t *testing.T, fs testcases.IdentityFieldSet) *ContextIdentity {
	t.Helper()
	b := NewBuilder(fs.ModelName, fs.ModelProvider)
	if fs.ModelVersion != "" {
		b.WithModelVersion(fs.ModelVersion)
	}
	if fs.Temperature != nil {
		b.WithTemperature(*fs.Temperature)
	}
	if fs.TopP != nil {
		b.WithTopP(*fs.TopP)
	}
	if fs.MaxTokens != nil {
		b.WithMaxTokens(*fs.MaxTokens)
	}
	if fs.TopK != nil {
		b.WithTopK(*fs.TopK)
	}
	if fs.FrequencyPenalty != nil {
		b.WithFrequencyPenalty(*fs.FrequencyPenalty)
	}
	if fs.PresencePenalty != nil {
		b.WithPresencePenalty(*fs.PresencePenalty)
	}
	if fs.Seed != nil {
		b.WithSeed(*fs.Seed)
	}
	if fs.Stop != nil {
		b.WithStop(fs.Stop)
	}
	if fs.PromptTemplateHash != "" || fs.PromptTemplateVersion != "" || fs.SystemPromptHash != "" {
		b.WithPrompt(fs.PromptTemplateHash, fs.PromptTemplateVersion, fs.SystemPromptHash)
	}
	if fs.Repository != nil {
		b.WithRepository(fs.Repository.URL, fs.Repository.Branch, fs.Repository.CommitHash)
	}
	for _, fh := range fs.FileHashes {
		b.WithFileHash(fh.Path, fh.Hash)
	}
	for _, dl := range fs.DependencyLocks {
		b.WithDependencyLock(dl.Type, dl.Hash)
	}
	for _, ts := range fs.ToolSchemas {
		b.WithToolSchema(ts.Name, ts.Version, ts.SchemaHash)
	}
	for _, to := range fs.ToolOutputs {
		b.WithToolOutput(to.ToolName, to.OutputHash)
	}
	if fs.Conversation != nil {
		b.WithConversation(fs.Conversation.TurnCount, fs.Conversation.HistoryHash)
	}
	for k, v := range fs.CustomFactors {
		b.WithCustomFactor(k, v)
	}
	identity, err := b.Build()
	if err != nil {
		t.Fatalf("failed to build identity: %v", err)
	}
	return identity
}

// =============================================================================
// Table-driven test cases
// =============================================================================

func TestTableDrivenDifferentiationCases(t *testing.T) {
	for _, tc := range testcases.DifferentiationCases() {
		t.Run(tc.Name, func(t *testing.T) {
			idA := buildIdentityFromFieldSet(t, tc.A)
			idB := buildIdentityFromFieldSet(t, tc.B)
			switch tc.Expected {
			case testcases.ExpectSameID:
				if idA.ID != idB.ID {
					t.Errorf("expected same ID, got A=%q B=%q", idA.ID, idB.ID)
				}
			case testcases.ExpectDifferentID:
				if idA.ID == idB.ID {
					t.Errorf("expected different IDs, both got %q", idA.ID)
				}
			}
			// Verify determinism
			idA2 := buildIdentityFromFieldSet(t, tc.A)
			idB2 := buildIdentityFromFieldSet(t, tc.B)
			if idA.ID != idA2.ID {
				t.Errorf("identity A is not deterministic: %q vs %q", idA.ID, idA2.ID)
			}
			if idB.ID != idB2.ID {
				t.Errorf("identity B is not deterministic: %q vs %q", idB.ID, idB2.ID)
			}
		})
	}
}

func TestTableDrivenEmptyNullCases(t *testing.T) {
	for _, tc := range testcases.EmptyNullCases() {
		t.Run(tc.Name, func(t *testing.T) {
			idA := buildIdentityFromFieldSet(t, tc.A)
			idB := buildIdentityFromFieldSet(t, tc.B)
			switch tc.Expected {
			case testcases.ExpectSameID:
				if idA.ID != idB.ID {
					t.Errorf("expected same ID, got A=%q B=%q", idA.ID, idB.ID)
				}
			case testcases.ExpectDifferentID:
				if idA.ID == idB.ID {
					t.Errorf("expected different IDs, both got %q", idA.ID)
				}
			}
		})
	}
}

func TestTableDrivenAdditionalDifferentiationCases(t *testing.T) {
	for _, tc := range testcases.AdditionalDifferentiationCases() {
		t.Run(tc.Name, func(t *testing.T) {
			idA := buildIdentityFromFieldSet(t, tc.A)
			idB := buildIdentityFromFieldSet(t, tc.B)
			switch tc.Expected {
			case testcases.ExpectSameID:
				if idA.ID != idB.ID {
					t.Errorf("expected same ID, got A=%q B=%q", idA.ID, idB.ID)
				}
			case testcases.ExpectDifferentID:
				if idA.ID == idB.ID {
					t.Errorf("expected different IDs, both got %q", idA.ID)
				}
			}
			idA2 := buildIdentityFromFieldSet(t, tc.A)
			idB2 := buildIdentityFromFieldSet(t, tc.B)
			if idA.ID != idA2.ID {
				t.Errorf("identity A is not deterministic")
			}
			if idB.ID != idB2.ID {
				t.Errorf("identity B is not deterministic")
			}
		})
	}
}

func TestTableDrivenOrderIndependenceCases(t *testing.T) {
	for _, tc := range testcases.OrderIndependenceCases() {
		t.Run(tc.Name, func(t *testing.T) {
			idA := buildIdentityFromFieldSet(t, tc.A)
			idB := buildIdentityFromFieldSet(t, tc.B)
			if tc.Expected == testcases.ExpectSameID && idA.ID != idB.ID {
				t.Errorf("expected same ID, got A=%q B=%q", idA.ID, idB.ID)
			}
			if tc.Expected == testcases.ExpectDifferentID && idA.ID == idB.ID {
				t.Errorf("expected different IDs, both got %q", idA.ID)
			}
		})
	}
}

func TestTableDrivenLargeContextCases(t *testing.T) {
	for _, tc := range testcases.LargeContextCases() {
		t.Run(tc.Name, func(t *testing.T) {
			idA := buildIdentityFromFieldSet(t, tc.A)
			idB := buildIdentityFromFieldSet(t, tc.B)
			if tc.Expected == testcases.ExpectSameID && idA.ID != idB.ID {
				t.Errorf("expected same ID, got A=%q B=%q", idA.ID, idB.ID)
			}
			if tc.Expected == testcases.ExpectDifferentID && idA.ID == idB.ID {
				t.Errorf("expected different IDs, both got %q", idA.ID)
			}
		})
	}
}

func TestAllTableDrivenCases(t *testing.T) {
	cases := testcases.AllCases()
	if len(cases) == 0 {
		t.Fatal("expected at least one test case")
	}
	t.Logf("Running %d table-driven test cases", len(cases))
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			idA := buildIdentityFromFieldSet(t, tc.A)
			idB := buildIdentityFromFieldSet(t, tc.B)
			switch tc.Expected {
			case testcases.ExpectSameID:
				if idA.ID != idB.ID {
					t.Errorf("[%s] expected same ID, got A=%q B=%q\nDescription: %s", tc.Name, idA.ID, idB.ID, tc.Description)
				}
			case testcases.ExpectDifferentID:
				if idA.ID == idB.ID {
					t.Errorf("[%s] expected different IDs, both got %q\nDescription: %s", tc.Name, idA.ID, tc.Description)
				}
			}
		})
	}
}

// =============================================================================
// Hash stability across runs
// =============================================================================

func TestHashStabilityAcrossRuns(t *testing.T) {
	build := func() string {
		id, err := NewBuilder("gpt-4", "openai").WithModelVersion("0613").WithTemperature(0.7).WithTopP(0.9).WithMaxTokens(4096).WithSeed(42).WithStop([]string{"END", "STOP"}).WithPrompt("tmplHash", "v1.0", "sysHash").WithRepository("https://github.com/example/repo", "main", "abc123").WithFileHash("main.go", "fileHash1").WithFileHash("util.go", "fileHash2").WithDependencyLock("go.sum", "depHash").WithToolSchema("search", "v1", "schemaHash").WithToolOutput("search", "outputHash").WithConversation(5, "convHash").WithCustomFactor("env", "production").Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return id.ID
	}
	first := build()
	for i := 0; i < 100; i++ {
		if id := build(); id != first {
			t.Fatalf("hash instability on iteration %d: expected %q, got %q", i, first, id)
		}
	}
}

func TestHashStabilityMinimalIdentity(t *testing.T) {
	build := func() string {
		id, err := NewBuilder("gpt-4", "openai").Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return id.ID
	}
	first := build()
	for i := 0; i < 50; i++ {
		if id := build(); id != first {
			t.Fatalf("hash instability on iteration %d", i)
		}
	}
}

func TestHashStabilityLargeContext(t *testing.T) {
	build := func() string {
		b := NewBuilder("gpt-4", "openai")
		for i := 0; i < 500; i++ {
			b.WithFileHash(fmt.Sprintf("file_%04d.go", i), fmt.Sprintf("hash_%04d", i))
		}
		for i := 0; i < 50; i++ {
			b.WithToolSchema(fmt.Sprintf("tool_%03d", i), "v1", fmt.Sprintf("schema_%03d", i))
		}
		for i := 0; i < 100; i++ {
			b.WithCustomFactor(fmt.Sprintf("factor_%03d", i), fmt.Sprintf("value_%03d", i))
		}
		id, err := b.Build()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return id.ID
	}
	first := build()
	for i := 0; i < 10; i++ {
		if id := build(); id != first {
			t.Fatalf("hash instability with large context on iteration %d", i)
		}
	}
}

// =============================================================================
// Canonicalization edge cases
// =============================================================================

func TestCanonicalizationWhitespaceInPromptHash(t *testing.T) {
	id1, _ := NewBuilder("gpt-4", "openai").WithPrompt("hash_with_space ", "v1", "sys").Build()
	id2, _ := NewBuilder("gpt-4", "openai").WithPrompt("hash_with_space", "v1", "sys").Build()
	if id1.ID == id2.ID {
		t.Error("expected different IDs for prompt hashes with trailing whitespace difference")
	}
}

func TestCanonicalizationUnicodeInCustomFactors(t *testing.T) {
	id1, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("greeting", "héllo").Build()
	id2, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("greeting", "hello").Build()
	if id1.ID == id2.ID {
		t.Error("expected different IDs for different Unicode content")
	}
	id3, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("greeting", "héllo").Build()
	if id1.ID != id3.ID {
		t.Error("expected same ID for identical Unicode content")
	}
}

func TestCanonicalizationSpecialCharactersInPaths(t *testing.T) {
	id1, _ := NewBuilder("gpt-4", "openai").WithFileHash("path/with spaces/file.go", "hash").Build()
	id2, _ := NewBuilder("gpt-4", "openai").WithFileHash("path/with_spaces/file.go", "hash").Build()
	if id1.ID == id2.ID {
		t.Error("expected different IDs for different file paths")
	}
}

func TestCanonicalizationEmptyStringValues(t *testing.T) {
	id1, _ := NewBuilder("gpt-4", "openai").WithModelVersion("").WithPrompt("", "", "").Build()
	id2, _ := NewBuilder("gpt-4", "openai").WithModelVersion("").WithPrompt("", "", "").Build()
	if id1.ID != id2.ID {
		t.Error("expected same ID for identical empty string fields")
	}
}

func TestCanonicalizationNewlinesInValues(t *testing.T) {
	id1, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("key", "line1\nline2").Build()
	id2, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("key", "line1\rline2").Build()
	id3, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("key", "line1\r\nline2").Build()
	ids := map[string]string{id1.ID: "\\n", id2.ID: "\\r", id3.ID: "\\r\\n"}
	if len(ids) != 3 {
		t.Errorf("expected 3 unique IDs for different newline characters, got %d", len(ids))
	}
}

func TestCanonicalizationKeyValueSeparatorInValues(t *testing.T) {
	id1, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("key", "value=extra").Build()
	id2, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("key", "value").Build()
	if id1.ID == id2.ID {
		t.Error("expected different IDs when value contains '=' separator")
	}
}

func TestCanonicalizationVeryLongValues(t *testing.T) {
	longValue := strings.Repeat("a", 100000)
	id1, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("key", longValue).Build()
	id2, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("key", longValue).Build()
	if id1.ID != id2.ID {
		t.Error("expected same ID for identical long values")
	}
	id3, _ := NewBuilder("gpt-4", "openai").WithCustomFactor("key", longValue+"b").Build()
	if id1.ID == id3.ID {
		t.Error("expected different IDs for different long values")
	}
}

func TestCanonicalizationFloatingPointEdgeCases(t *testing.T) {
	temp1 := 0.1 + 0.2
	temp2 := 0.3
	id1, _ := NewBuilder("gpt-4", "openai").WithTemperature(temp1).Build()
	id1a, _ := NewBuilder("gpt-4", "openai").WithTemperature(temp1).Build()
	if id1.ID != id1a.ID {
		t.Error("floating point edge case: same temp value not deterministic")
	}
	id2, _ := NewBuilder("gpt-4", "openai").WithTemperature(temp2).Build()
	id2a, _ := NewBuilder("gpt-4", "openai").WithTemperature(temp2).Build()
	if id2.ID != id2a.ID {
		t.Error("floating point edge case: same temp value not deterministic")
	}
}

func TestCanonicalizationZeroTemperatureVsNil(t *testing.T) {
	id1, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.0).Build()
	id2, _ := NewBuilder("gpt-4", "openai").Build()
	if id1.ID == id2.ID {
		t.Error("expected different IDs for temperature=0.0 vs no temperature")
	}
}

// =============================================================================
// Invalidation scenarios
// =============================================================================

func TestInvalidationScenarioCommitChange(t *testing.T) {
	cached, _ := NewBuilder("gpt-4", "openai").WithPrompt("hash", "v1", "sys").WithRepository("url", "main", "commit_old").Build()
	current, _ := NewBuilder("gpt-4", "openai").WithPrompt("hash", "v1", "sys").WithRepository("url", "main", "commit_new").Build()
	detector := NewChangeDetector()
	changes := detector.DetectChanges(cached, current)
	if len(changes) == 0 {
		t.Fatal("expected changes for commit change")
	}
	foundCommitChange := false
	for _, c := range changes {
		if c.Category == FactorCategoryRepository && c.Name == "repository.commit_hash" {
			foundCommitChange = true
			if c.OldValue != "commit_old" {
				t.Errorf("expected old value 'commit_old', got %q", c.OldValue)
			}
			if c.NewValue != "commit_new" {
				t.Errorf("expected new value 'commit_new', got %q", c.NewValue)
			}
		}
	}
	if !foundCommitChange {
		t.Error("expected repository.commit_hash change in detected changes")
	}
}

func TestInvalidationScenarioToolSchemaUpgrade(t *testing.T) {
	cached, _ := NewBuilder("gpt-4", "openai").WithToolSchema("search", "v1.0", "hash_v1").Build()
	current, _ := NewBuilder("gpt-4", "openai").WithToolSchema("search", "v2.0", "hash_v2").Build()
	detector := NewChangeDetector()
	changes := detector.DetectChanges(cached, current)
	if len(changes) == 0 {
		t.Fatal("expected changes for tool schema upgrade")
	}
	hasToolChange := false
	for _, c := range changes {
		if c.Category == FactorCategoryTool {
			hasToolChange = true
		}
	}
	if !hasToolChange {
		t.Error("expected tool category change in detected changes")
	}
}

func TestInvalidationScenarioSystemPromptChange(t *testing.T) {
	cached, _ := NewBuilder("gpt-4", "openai").WithPrompt("tmplHash", "v1", "old_sys_hash").Build()
	current, _ := NewBuilder("gpt-4", "openai").WithPrompt("tmplHash", "v1", "new_sys_hash").Build()
	detector := NewChangeDetector()
	changes := detector.DetectChanges(cached, current)
	if len(changes) == 0 {
		t.Fatal("expected changes for system prompt change")
	}
	hasPromptChange := false
	for _, c := range changes {
		if c.Category == FactorCategoryPrompt {
			hasPromptChange = true
		}
	}
	if !hasPromptChange {
		t.Error("expected prompt category change in detected changes")
	}
}

func TestInvalidationScenarioDependencyUpdate(t *testing.T) {
	cached, _ := NewBuilder("gpt-4", "openai").WithDependencyLock("go.sum", "old_dep_hash").Build()
	current, _ := NewBuilder("gpt-4", "openai").WithDependencyLock("go.sum", "new_dep_hash").Build()
	detector := NewChangeDetector()
	changes := detector.DetectChanges(cached, current)
	if len(changes) == 0 {
		t.Fatal("expected changes for dependency update")
	}
	hasDepChange := false
	for _, c := range changes {
		if c.Category == FactorCategoryDependency {
			hasDepChange = true
		}
	}
	if !hasDepChange {
		t.Error("expected dependency category change in detected changes")
	}
}

func TestInvalidationScenarioModelChange(t *testing.T) {
	cached, _ := NewBuilder("gpt-4", "openai").Build()
	current, _ := NewBuilder("gpt-4-turbo", "openai").Build()
	detector := NewChangeDetector()
	changes := detector.DetectChanges(cached, current)
	if len(changes) == 0 {
		t.Fatal("expected changes for model change")
	}
	hasModelChange := false
	for _, c := range changes {
		if c.Category == FactorCategoryModel {
			hasModelChange = true
		}
	}
	if !hasModelChange {
		t.Error("expected model category change in detected changes")
	}
}

func TestInvalidationScenarioNoChanges(t *testing.T) {
	identity, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash", "v1", "sys").WithRepository("url", "main", "commit").Build()
	duplicate, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash", "v1", "sys").WithRepository("url", "main", "commit").Build()
	detector := NewChangeDetector()
	changes := detector.DetectChanges(identity, duplicate)
	if len(changes) != 0 {
		t.Errorf("expected no changes for identical identities, got %d changes", len(changes))
	}
}

func TestInvalidationScenarioMultipleChanges(t *testing.T) {
	cached, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash1", "v1", "sys1").WithRepository("url", "main", "commit1").WithDependencyLock("go.sum", "dep1").Build()
	current, _ := NewBuilder("gpt-4-turbo", "openai").WithTemperature(0.2).WithPrompt("hash2", "v2", "sys2").WithRepository("url", "main", "commit2").WithDependencyLock("go.sum", "dep2").Build()
	detector := NewChangeDetector()
	changes := detector.DetectChanges(cached, current)
	categories := make(map[FactorCategory]bool)
	for _, c := range changes {
		categories[c.Category] = true
	}
	for _, expected := range []FactorCategory{FactorCategoryModel, FactorCategoryPrompt, FactorCategoryRepository, FactorCategoryDependency} {
		if !categories[expected] {
			t.Errorf("expected %s category in changes", expected)
		}
	}
}

// =============================================================================
// Fixture integration tests
// =============================================================================

func TestFixturePromptDifferentiation(t *testing.T) {
	ids := make(map[string]string)
	for name, prompt := range fixtures.SamplePrompts {
		if prompt == "" {
			continue
		}
		hash := testcases.HashString(prompt)
		id, err := NewBuilder("gpt-4", "openai").WithPrompt(hash, "v1", "sys").Build()
		if err != nil {
			t.Fatalf("unexpected error for prompt %q: %v", name, err)
		}
		if prevName, exists := ids[id.ID]; exists {
			t.Errorf("collision between prompts %q and %q", name, prevName)
		}
		ids[id.ID] = name
	}
}

func TestFixtureRepositoryDifferentiation(t *testing.T) {
	ids := make(map[string]string)
	for name, repo := range fixtures.SampleRepositories {
		id, err := NewBuilder("gpt-4", "openai").WithRepository(repo.URL, repo.Branch, repo.Commit).Build()
		if err != nil {
			t.Fatalf("unexpected error for repo %q: %v", name, err)
		}
		if prevName, exists := ids[id.ID]; exists {
			t.Errorf("collision between repos %q and %q", name, prevName)
		}
		ids[id.ID] = name
	}
}

func TestFixtureToolSchemaDifferentiation(t *testing.T) {
	ids := make(map[string]string)
	for name, schemas := range fixtures.SampleToolSchemas {
		if len(schemas) == 0 {
			continue
		}
		b := NewBuilder("gpt-4", "openai")
		for _, ts := range schemas {
			b.WithToolSchema(ts.Name, ts.Version, ts.SchemaHash)
		}
		id, err := b.Build()
		if err != nil {
			t.Fatalf("unexpected error for tool schema set %q: %v", name, err)
		}
		if prevName, exists := ids[id.ID]; exists {
			t.Errorf("collision between tool schema sets %q and %q", name, prevName)
		}
		ids[id.ID] = name
	}
}

func TestFixtureConversationDifferentiation(t *testing.T) {
	ids := make(map[string]string)
	for name, conv := range fixtures.SampleConversations {
		id, err := NewBuilder("gpt-4", "openai").WithConversation(conv.TurnCount, conv.HistoryHash).Build()
		if err != nil {
			t.Fatalf("unexpected error for conversation %q: %v", name, err)
		}
		if prevName, exists := ids[id.ID]; exists {
			t.Errorf("collision between conversations %q and %q", name, prevName)
		}
		ids[id.ID] = name
	}
}

func TestCompareWithDifferentFixtureRepositories(t *testing.T) {
	base := func(repo fixtures.RepositoryFixture) *ContextIdentity {
		id, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash", "v1", "sys").WithRepository(repo.URL, repo.Branch, repo.Commit).Build()
		return id
	}
	id1 := base(fixtures.SampleRepositories["main_branch"])
	id2 := base(fixtures.SampleRepositories["same_repo_new_commit"])
	result := id1.Compare(id2)
	if result.IsExactMatch {
		t.Error("expected non-exact match for different commits")
	}
	if result.Similarity <= 0 {
		t.Error("expected positive similarity")
	}
	if result.Similarity >= 1.0 {
		t.Error("expected less than perfect similarity")
	}
}

func TestDiffWithFixtureConversationChanges(t *testing.T) {
	conv1 := fixtures.SampleConversations["single_turn"]
	conv2 := fixtures.SampleConversations["multi_turn"]
	id1, _ := NewBuilder("gpt-4", "openai").WithConversation(conv1.TurnCount, conv1.HistoryHash).Build()
	id2, _ := NewBuilder("gpt-4", "openai").WithConversation(conv2.TurnCount, conv2.HistoryHash).Build()
	diffs := id1.Diff(id2)
	if len(diffs) == 0 {
		t.Fatal("expected diffs for different conversation states")
	}
	hasConvDiff := false
	for _, d := range diffs {
		if strings.Contains(d, "conversation") {
			hasConvDiff = true
			break
		}
	}
	if !hasConvDiff {
		t.Errorf("expected conversation diff, got: %v", diffs)
	}
}

// =============================================================================
// ID format and serialization edge cases
// =============================================================================

func TestIdentityIDFormat(t *testing.T) {
	id, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash", "v1", "sys").Build()
	if len(id.ID) != 64 {
		t.Errorf("expected ID length 64 (SHA-256 hex), got %d", len(id.ID))
	}
	for _, c := range id.ID {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("unexpected character %q in ID", c)
		}
	}
}

func TestIdentityIDConsistentAfterSerialization(t *testing.T) {
	original, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash", "v1", "sys").WithRepository("url", "main", "commit").WithConversation(3, "histHash").Build()
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var restored ContextIdentity
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if restored.ComputeID() != original.ComputeID() {
		t.Error("ID changed after serialization roundtrip")
	}
}

// =============================================================================
// Performance benchmarks
// =============================================================================

func BenchmarkComputeIDMinimal(b *testing.B) {
	id, _ := NewBuilder("gpt-4", "openai").Build()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id.ComputeID()
	}
}

func BenchmarkComputeIDFull(b *testing.B) {
	id, _ := NewBuilder("claude-3-opus", "anthropic").WithModelVersion("20240229").WithTemperature(0.7).WithTopP(0.9).WithMaxTokens(4096).WithTopK(40).WithFrequencyPenalty(0.5).WithPresencePenalty(0.3).WithSeed(42).WithStop([]string{"STOP", "END", "DONE"}).WithPrompt("tmplHash", "v2.1", "sysHash").WithRepository("https://github.com/example/repo", "main", "abc123").WithFileHash("main.go", "hash1").WithFileHash("util.go", "hash2").WithFileHash("handler.go", "hash3").WithDependencyLock("go.sum", "depHash1").WithDependencyLock("package-lock.json", "depHash2").WithToolSchema("search", "v1", "schemaHash1").WithToolSchema("browse", "v2", "schemaHash2").WithToolOutput("search", "outputHash1").WithConversation(10, "convHash").WithRetrievedContext("queryHash", map[string]string{"doc1": "h1", "doc2": "h2"}).WithCustomFactor("env", "prod").WithCustomFactor("region", "us-east-1").Build()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id.ComputeID()
	}
}

func BenchmarkComputeIDLargeFileSet(b *testing.B) {
	builder := NewBuilder("gpt-4", "openai")
	for i := 0; i < 1000; i++ {
		builder.WithFileHash(fmt.Sprintf("file_%04d.go", i), fmt.Sprintf("hash_%04d", i))
	}
	id, _ := builder.Build()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id.ComputeID()
	}
}

func BenchmarkComputeIDLargeCustomFactors(b *testing.B) {
	builder := NewBuilder("gpt-4", "openai")
	for i := 0; i < 500; i++ {
		builder.WithCustomFactor(fmt.Sprintf("factor_%03d", i), fmt.Sprintf("value_%03d", i))
	}
	id, _ := builder.Build()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id.ComputeID()
	}
}

func BenchmarkBuildIdentity(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash", "v1", "sys").WithRepository("url", "main", "commit").WithFileHash("main.go", "hash1").WithDependencyLock("go.sum", "depHash").WithToolSchema("search", "v1", "schemaHash").WithConversation(5, "convHash").Build()
	}
}

func BenchmarkCompareIdentities(b *testing.B) {
	id1, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash1", "v1", "sys1").WithRepository("url", "main", "commit1").WithFileHash("main.go", "hash1").WithConversation(5, "convHash1").Build()
	id2, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash1", "v1", "sys1").WithRepository("url", "main", "commit2").WithFileHash("main.go", "hash1").WithConversation(5, "convHash1").Build()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id1.Compare(id2)
	}
}

func BenchmarkDetectChanges(b *testing.B) {
	cached, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash1", "v1", "sys1").WithRepository("url", "main", "commit1").WithFileHash("main.go", "fH1").WithDependencyLock("go.sum", "dH1").WithToolSchema("search", "v1", "sH1").WithConversation(5, "cH1").Build()
	current, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash2", "v1", "sys2").WithRepository("url", "main", "commit2").WithFileHash("main.go", "fH2").WithDependencyLock("go.sum", "dH2").WithToolSchema("search", "v2", "sH2").WithConversation(6, "cH2").Build()
	detector := NewChangeDetector()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.DetectChanges(cached, current)
	}
}

func BenchmarkHashStability(b *testing.B) {
	id, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash", "v1", "sys").WithRepository("url", "main", "commit").WithFileHash("main.go", "fH1").WithFileHash("util.go", "fH2").Build()
	expected := id.ComputeID()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got := id.ComputeID()
		if got != expected {
			b.Fatalf("hash instability: expected %q, got %q", expected, got)
		}
	}
}

func BenchmarkSerializationRoundtrip(b *testing.B) {
	id, _ := NewBuilder("gpt-4", "openai").WithTemperature(0.7).WithPrompt("hash", "v1", "sys").WithRepository("url", "main", "commit").WithFileHash("main.go", "hash1").WithDependencyLock("go.sum", "depHash").WithToolSchema("search", "v1", "schemaHash").WithConversation(5, "convHash").WithCustomFactor("env", "prod").Build()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := json.Marshal(id)
		var restored ContextIdentity
		_ = json.Unmarshal(data, &restored)
	}
}
