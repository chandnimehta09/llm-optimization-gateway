// Package testcases provides reusable, table-driven test case definitions for
// context identity behavior. Each case specifies two identity builder
// configurations and the expected relationship between the resulting IDs.
package testcases

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/llm-optimization-gateway/internal/context/testcases/fixtures"
)

// IdentityFieldSet describes all the configurable fields for building a
// ContextIdentity via the Builder API.
type IdentityFieldSet struct {
	ModelName     string
	ModelProvider string
	ModelVersion  string

	// Parameters (nil means not set)
	Temperature      *float64
	TopP             *float64
	MaxTokens        *int
	TopK             *int
	FrequencyPenalty *float64
	PresencePenalty  *float64
	Seed             *int64
	Stop             []string

	// Prompt
	PromptTemplateHash    string
	PromptTemplateVersion string
	SystemPromptHash      string

	// Repository (nil means no repository)
	Repository *RepositoryField

	// File hashes
	FileHashes []FileHashField

	// Dependencies
	DependencyLocks []DependencyLockField

	// Tool schemas
	ToolSchemas []ToolSchemaField

	// Tool outputs
	ToolOutputs []ToolOutputField

	// Conversation (nil means no conversation)
	Conversation *ConversationField

	// Custom factors
	CustomFactors map[string]string
}

// RepositoryField holds repository identity fields.
type RepositoryField struct {
	URL        string
	Branch     string
	CommitHash string
}

// FileHashField holds a file path and its hash.
type FileHashField struct {
	Path string
	Hash string
}

// DependencyLockField holds a dependency type and its hash.
type DependencyLockField struct {
	Type string
	Hash string
}

// ToolSchemaField holds tool schema identity fields.
type ToolSchemaField struct {
	Name       string
	Version    string
	SchemaHash string
}

// ToolOutputField holds tool output identity fields.
type ToolOutputField struct {
	ToolName   string
	OutputHash string
}

// ConversationField holds conversation state fields.
type ConversationField struct {
	TurnCount   int
	HistoryHash string
}

// ExpectedResult describes the expected relationship between two identities.
type ExpectedResult int

const (
	// ExpectSameID means both identity field sets should produce the same ID.
	ExpectSameID ExpectedResult = iota
	// ExpectDifferentID means the identity field sets should produce different IDs.
	ExpectDifferentID
)

// IdentityTestCase is a single test case for identity behavior.
type IdentityTestCase struct {
	// Name is a human-readable name for the test case.
	Name string
	// Description explains what the test case verifies.
	Description string
	// A is the first identity field set.
	A IdentityFieldSet
	// B is the second identity field set.
	B IdentityFieldSet
	// Expected is the expected relationship between A and B identities.
	Expected ExpectedResult
}

// Float64 creates a float64 pointer.
func Float64(f float64) *float64 { return &f }

// Int creates an int pointer.
func Int(i int) *int { return &i }

// Int64 creates an int64 pointer.
func Int64(i int64) *int64 { return &i }

// HashString computes a SHA-256 hex hash of a string.
func HashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// BaseFieldSet returns a standard baseline identity field set used as the
// starting point for most test cases.
func BaseFieldSet() IdentityFieldSet {
	return IdentityFieldSet{
		ModelName:             "gpt-4",
		ModelProvider:         "openai",
		ModelVersion:          "0613",
		Temperature:           Float64(0.7),
		TopP:                  Float64(0.9),
		MaxTokens:             Int(4096),
		PromptTemplateHash:    HashString(fixtures.SamplePrompts["code_review"]),
		PromptTemplateVersion: "v1.0",
		SystemPromptHash:      HashString(fixtures.SampleSystemPrompts["default"]),
		Repository: &RepositoryField{
			URL:        fixtures.SampleRepositories["main_branch"].URL,
			Branch:     fixtures.SampleRepositories["main_branch"].Branch,
			CommitHash: fixtures.SampleRepositories["main_branch"].Commit,
		},
		FileHashes: []FileHashField{
			{Path: fixtures.SampleFileHashes["go_project"][0].Path, Hash: fixtures.SampleFileHashes["go_project"][0].Hash},
			{Path: fixtures.SampleFileHashes["go_project"][1].Path, Hash: fixtures.SampleFileHashes["go_project"][1].Hash},
		},
		DependencyLocks: []DependencyLockField{
			{Type: fixtures.SampleDependencies["go_deps"][0].Type, Hash: fixtures.SampleDependencies["go_deps"][0].Hash},
		},
		ToolSchemas: []ToolSchemaField{
			{Name: fixtures.SampleToolSchemas["search_v1"][0].Name, Version: fixtures.SampleToolSchemas["search_v1"][0].Version, SchemaHash: fixtures.SampleToolSchemas["search_v1"][0].SchemaHash},
		},
		Conversation: &ConversationField{
			TurnCount:   fixtures.SampleConversations["multi_turn"].TurnCount,
			HistoryHash: fixtures.SampleConversations["multi_turn"].HistoryHash,
		},
	}
}

// DifferentiationCases returns the core set of test cases that verify identity
// changes when a single factor differs.
func DifferentiationCases() []IdentityTestCase {
	base := BaseFieldSet()

	return []IdentityTestCase{
		{
			Name:        "same_prompt_different_commit",
			Description: "Same prompt with different repository commit produces different identity",
			A:           base,
			B: withModification(base, func(f *IdentityFieldSet) {
				f.Repository = &RepositoryField{
					URL:        fixtures.SampleRepositories["same_repo_new_commit"].URL,
					Branch:     fixtures.SampleRepositories["same_repo_new_commit"].Branch,
					CommitHash: fixtures.SampleRepositories["same_repo_new_commit"].Commit,
				}
			}),
			Expected: ExpectDifferentID,
		},
		{
			Name:        "same_prompt_different_model",
			Description: "Same prompt with different model name produces different identity",
			A:           base,
			B: withModification(base, func(f *IdentityFieldSet) {
				f.ModelName = "claude-3-opus"
				f.ModelProvider = "anthropic"
			}),
			Expected: ExpectDifferentID,
		},
		{
			Name:        "same_prompt_different_system_prompt",
			Description: "Same prompt template with different system prompt produces different identity",
			A:           base,
			B: withModification(base, func(f *IdentityFieldSet) {
				f.SystemPromptHash = HashString(fixtures.SampleSystemPrompts["code"])
			}),
			Expected: ExpectDifferentID,
		},
		{
			Name:        "same_prompt_different_dependencies",
			Description: "Same prompt with different dependency locks produces different identity",
			A:           base,
			B: withModification(base, func(f *IdentityFieldSet) {
				f.DependencyLocks = []DependencyLockField{
					{Type: fixtures.SampleDependencies["updated_go_deps"][0].Type, Hash: fixtures.SampleDependencies["updated_go_deps"][0].Hash},
				}
			}),
			Expected: ExpectDifferentID,
		},
		{
			Name:        "same_prompt_different_temperature",
			Description: "Same prompt with different temperature produces different identity",
			A:           base,
			B: withModification(base, func(f *IdentityFieldSet) {
				f.Temperature = Float64(0.2)
			}),
			Expected: ExpectDifferentID,
		},
		{
			Name:        "same_everything_same_identity",
			Description: "Identical configurations produce the same identity",
			A:           base,
			B:           base,
			Expected:    ExpectSameID,
		},
		{
			Name:        "same_prompt_different_tool_schemas",
			Description: "Same prompt with different tool schemas produces different identity",
			A:           base,
			B: withModification(base, func(f *IdentityFieldSet) {
				f.ToolSchemas = []ToolSchemaField{
					{Name: fixtures.SampleToolSchemas["search_v2"][0].Name, Version: fixtures.SampleToolSchemas["search_v2"][0].Version, SchemaHash: fixtures.SampleToolSchemas["search_v2"][0].SchemaHash},
				}
			}),
			Expected: ExpectDifferentID,
		},
		{
			Name:        "same_prompt_different_conversation_state",
			Description: "Same prompt with different conversation state produces different identity",
			A:           base,
			B: withModification(base, func(f *IdentityFieldSet) {
				f.Conversation = &ConversationField{
					TurnCount:   fixtures.SampleConversations["long_conversation"].TurnCount,
					HistoryHash: fixtures.SampleConversations["long_conversation"].HistoryHash,
				}
			}),
			Expected: ExpectDifferentID,
		},
	}
}

// EmptyNullCases returns test cases for empty vs null value handling.
func EmptyNullCases() []IdentityTestCase {
	return []IdentityTestCase{
		{Name: "empty_system_prompt_vs_absent", Description: "Empty system prompt hash and absent system prompt hash produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", SystemPromptHash: ""}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai"}, Expected: ExpectSameID},
		{Name: "nil_repository_vs_omitted", Description: "Nil repository and omitted repository produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", Repository: nil}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai"}, Expected: ExpectSameID},
		{Name: "nil_conversation_vs_omitted", Description: "Nil conversation and omitted conversation produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", Conversation: nil}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai"}, Expected: ExpectSameID},
		{Name: "empty_file_hashes_vs_nil", Description: "Empty file hashes slice and nil file hashes produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", FileHashes: []FileHashField{}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", FileHashes: nil}, Expected: ExpectSameID},
		{Name: "empty_tool_schemas_vs_nil", Description: "Empty tool schemas slice and nil tool schemas produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", ToolSchemas: []ToolSchemaField{}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", ToolSchemas: nil}, Expected: ExpectSameID},
		{Name: "empty_dependency_locks_vs_nil", Description: "Empty dependency locks slice and nil dependency locks produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", DependencyLocks: []DependencyLockField{}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", DependencyLocks: nil}, Expected: ExpectSameID},
		{Name: "empty_custom_factors_vs_nil", Description: "Empty custom factors map and nil custom factors produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", CustomFactors: map[string]string{}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", CustomFactors: nil}, Expected: ExpectSameID},
		{Name: "nil_temperature_vs_omitted", Description: "Nil temperature pointer and omitted temperature produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", Temperature: nil}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai"}, Expected: ExpectSameID},
		{Name: "zero_conversation_vs_no_conversation", Description: "Zero-value conversation produces different identity from no conversation", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", Conversation: &ConversationField{TurnCount: 0, HistoryHash: ""}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", Conversation: nil}, Expected: ExpectDifferentID},
		{Name: "zero_repository_vs_no_repository", Description: "Zero-value repository produces different identity from no repository", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", Repository: &RepositoryField{URL: "", Branch: "", CommitHash: ""}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", Repository: nil}, Expected: ExpectDifferentID},
	}
}

// AdditionalDifferentiationCases returns extra test cases covering more edge cases.
func AdditionalDifferentiationCases() []IdentityTestCase {
	base := BaseFieldSet()

	return []IdentityTestCase{
		{Name: "same_model_different_version", Description: "Same model name with different version produces different identity", A: base, B: withModification(base, func(f *IdentityFieldSet) { f.ModelVersion = "0314" }), Expected: ExpectDifferentID},
		{Name: "same_model_name_different_provider", Description: "Same model name with different provider produces different identity", A: base, B: withModification(base, func(f *IdentityFieldSet) { f.ModelProvider = "azure" }), Expected: ExpectDifferentID},
		{Name: "same_prompt_different_top_p", Description: "Same prompt with different top_p produces different identity", A: base, B: withModification(base, func(f *IdentityFieldSet) { f.TopP = Float64(0.5) }), Expected: ExpectDifferentID},
		{Name: "same_prompt_different_max_tokens", Description: "Same prompt with different max_tokens produces different identity", A: base, B: withModification(base, func(f *IdentityFieldSet) { f.MaxTokens = Int(2048) }), Expected: ExpectDifferentID},
		{Name: "same_prompt_different_seed", Description: "Same prompt with different seed produces different identity", A: withModification(base, func(f *IdentityFieldSet) { f.Seed = Int64(42) }), B: withModification(base, func(f *IdentityFieldSet) { f.Seed = Int64(99) }), Expected: ExpectDifferentID},
		{Name: "same_prompt_different_stop_sequences", Description: "Same prompt with different stop sequences produces different identity", A: withModification(base, func(f *IdentityFieldSet) { f.Stop = []string{"END"} }), B: withModification(base, func(f *IdentityFieldSet) { f.Stop = []string{"STOP", "DONE"} }), Expected: ExpectDifferentID},
		{Name: "same_prompt_different_file_hashes", Description: "Same prompt with different file hashes produces different identity", A: base, B: withModification(base, func(f *IdentityFieldSet) {
			f.FileHashes = []FileHashField{
				{Path: fixtures.SampleFileHashes["go_project_modified"][0].Path, Hash: fixtures.SampleFileHashes["go_project_modified"][0].Hash},
				{Path: fixtures.SampleFileHashes["go_project_modified"][1].Path, Hash: fixtures.SampleFileHashes["go_project_modified"][1].Hash},
			}
		}), Expected: ExpectDifferentID},
		{Name: "same_prompt_different_custom_factors", Description: "Same prompt with different custom factors produces different identity", A: withModification(base, func(f *IdentityFieldSet) { f.CustomFactors = map[string]string{"env": "production"} }), B: withModification(base, func(f *IdentityFieldSet) { f.CustomFactors = map[string]string{"env": "staging"} }), Expected: ExpectDifferentID},
		{Name: "different_tool_schema_count", Description: "Different number of tool schemas produces different identity", A: withModification(base, func(f *IdentityFieldSet) {
			f.ToolSchemas = []ToolSchemaField{{Name: "search", Version: "v1", SchemaHash: "hash1"}}
		}), B: withModification(base, func(f *IdentityFieldSet) {
			f.ToolSchemas = []ToolSchemaField{{Name: "search", Version: "v1", SchemaHash: "hash1"}, {Name: "browse", Version: "v1", SchemaHash: "hash2"}}
		}), Expected: ExpectDifferentID},
		{Name: "repository_present_vs_absent", Description: "Repository present vs absent produces different identity", A: base, B: withModification(base, func(f *IdentityFieldSet) { f.Repository = nil }), Expected: ExpectDifferentID},
		{Name: "conversation_present_vs_absent", Description: "Conversation present vs absent produces different identity", A: base, B: withModification(base, func(f *IdentityFieldSet) { f.Conversation = nil }), Expected: ExpectDifferentID},
		{Name: "same_prompt_different_frequency_penalty", Description: "Same prompt with different frequency penalty produces different identity", A: withModification(base, func(f *IdentityFieldSet) { f.FrequencyPenalty = Float64(0.0) }), B: withModification(base, func(f *IdentityFieldSet) { f.FrequencyPenalty = Float64(0.5) }), Expected: ExpectDifferentID},
		{Name: "same_prompt_different_presence_penalty", Description: "Same prompt with different presence penalty produces different identity", A: withModification(base, func(f *IdentityFieldSet) { f.PresencePenalty = Float64(0.0) }), B: withModification(base, func(f *IdentityFieldSet) { f.PresencePenalty = Float64(0.8) }), Expected: ExpectDifferentID},
		{Name: "same_hash_different_prompt_version", Description: "Same prompt hash with different template version produces different identity", A: base, B: withModification(base, func(f *IdentityFieldSet) { f.PromptTemplateVersion = "v2.0" }), Expected: ExpectDifferentID},
	}
}

// OrderIndependenceCases returns test cases verifying that field order doesn't
// affect identity computation.
func OrderIndependenceCases() []IdentityTestCase {
	return []IdentityTestCase{
		{Name: "file_hash_order_independence", Description: "File hashes in different order produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", FileHashes: []FileHashField{{Path: "a.go", Hash: "hash_a"}, {Path: "b.go", Hash: "hash_b"}, {Path: "c.go", Hash: "hash_c"}}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", FileHashes: []FileHashField{{Path: "c.go", Hash: "hash_c"}, {Path: "a.go", Hash: "hash_a"}, {Path: "b.go", Hash: "hash_b"}}}, Expected: ExpectSameID},
		{Name: "dependency_lock_order_independence", Description: "Dependency locks in different order produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", DependencyLocks: []DependencyLockField{{Type: "go.sum", Hash: "hash1"}, {Type: "package-lock.json", Hash: "hash2"}}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", DependencyLocks: []DependencyLockField{{Type: "package-lock.json", Hash: "hash2"}, {Type: "go.sum", Hash: "hash1"}}}, Expected: ExpectSameID},
		{Name: "tool_schema_order_independence", Description: "Tool schemas in different order produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", ToolSchemas: []ToolSchemaField{{Name: "search", Version: "v1", SchemaHash: "hash1"}, {Name: "browse", Version: "v2", SchemaHash: "hash2"}}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", ToolSchemas: []ToolSchemaField{{Name: "browse", Version: "v2", SchemaHash: "hash2"}, {Name: "search", Version: "v1", SchemaHash: "hash1"}}}, Expected: ExpectSameID},
		{Name: "stop_sequence_order_independence", Description: "Stop sequences in different order produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", Stop: []string{"END", "STOP", "DONE"}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", Stop: []string{"STOP", "DONE", "END"}}, Expected: ExpectSameID},
		{Name: "custom_factor_order_independence", Description: "Custom factors with different insertion order produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", CustomFactors: map[string]string{"env": "production", "region": "us-east-1", "version": "1.0"}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", CustomFactors: map[string]string{"version": "1.0", "env": "production", "region": "us-east-1"}}, Expected: ExpectSameID},
		{Name: "tool_output_order_independence", Description: "Tool outputs in different order produce the same identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", ToolOutputs: []ToolOutputField{{ToolName: "search", OutputHash: "hash1"}, {ToolName: "calc", OutputHash: "hash2"}}}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", ToolOutputs: []ToolOutputField{{ToolName: "calc", OutputHash: "hash2"}, {ToolName: "search", OutputHash: "hash1"}}}, Expected: ExpectSameID},
	}
}

// LargeContextCases returns test cases for large context handling.
func LargeContextCases() []IdentityTestCase {
	largeFileHashes := make([]FileHashField, 1000)
	for i := range largeFileHashes {
		largeFileHashes[i] = FileHashField{
			Path: fmt.Sprintf("file_%04d.go", i),
			Hash: HashString(fmt.Sprintf("content_of_file_%04d", i)),
		}
	}

	largeToolSchemas := make([]ToolSchemaField, 100)
	for i := range largeToolSchemas {
		largeToolSchemas[i] = ToolSchemaField{
			Name:       fmt.Sprintf("tool_%03d", i),
			Version:    "v1.0",
			SchemaHash: HashString(fmt.Sprintf("schema_%03d", i)),
		}
	}

	largeCustomFactors := make(map[string]string, 200)
	for i := 0; i < 200; i++ {
		largeCustomFactors[fmt.Sprintf("factor_%03d", i)] = HashString(fmt.Sprintf("value_%03d", i))
	}

	return []IdentityTestCase{
		{Name: "large_file_hash_set", Description: "Large number of file hashes still produces deterministic identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", FileHashes: largeFileHashes}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", FileHashes: largeFileHashes}, Expected: ExpectSameID},
		{Name: "large_tool_schema_set", Description: "Large number of tool schemas still produces deterministic identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", ToolSchemas: largeToolSchemas}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", ToolSchemas: largeToolSchemas}, Expected: ExpectSameID},
		{Name: "large_custom_factor_set", Description: "Large number of custom factors still produces deterministic identity", A: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", CustomFactors: largeCustomFactors}, B: IdentityFieldSet{ModelName: "gpt-4", ModelProvider: "openai", CustomFactors: largeCustomFactors}, Expected: ExpectSameID},
	}
}

// AllCases returns all test cases combined.
func AllCases() []IdentityTestCase {
	var all []IdentityTestCase
	all = append(all, DifferentiationCases()...)
	all = append(all, EmptyNullCases()...)
	all = append(all, AdditionalDifferentiationCases()...)
	all = append(all, OrderIndependenceCases()...)
	all = append(all, LargeContextCases()...)
	return all
}

// withModification returns a copy of the field set with a modification applied.
func withModification(base IdentityFieldSet, modify func(*IdentityFieldSet)) IdentityFieldSet {
	modified := base

	if base.FileHashes != nil {
		modified.FileHashes = make([]FileHashField, len(base.FileHashes))
		copy(modified.FileHashes, base.FileHashes)
	}
	if base.DependencyLocks != nil {
		modified.DependencyLocks = make([]DependencyLockField, len(base.DependencyLocks))
		copy(modified.DependencyLocks, base.DependencyLocks)
	}
	if base.ToolSchemas != nil {
		modified.ToolSchemas = make([]ToolSchemaField, len(base.ToolSchemas))
		copy(modified.ToolSchemas, base.ToolSchemas)
	}
	if base.ToolOutputs != nil {
		modified.ToolOutputs = make([]ToolOutputField, len(base.ToolOutputs))
		copy(modified.ToolOutputs, base.ToolOutputs)
	}
	if base.Stop != nil {
		modified.Stop = make([]string, len(base.Stop))
		copy(modified.Stop, base.Stop)
	}
	if base.CustomFactors != nil {
		modified.CustomFactors = make(map[string]string, len(base.CustomFactors))
		for k, v := range base.CustomFactors {
			modified.CustomFactors[k] = v
		}
	}
	if base.Repository != nil {
		repoCopy := *base.Repository
		modified.Repository = &repoCopy
	}
	if base.Conversation != nil {
		convCopy := *base.Conversation
		modified.Conversation = &convCopy
	}
	if base.Temperature != nil {
		tempCopy := *base.Temperature
		modified.Temperature = &tempCopy
	}
	if base.TopP != nil {
		topPCopy := *base.TopP
		modified.TopP = &topPCopy
	}
	if base.MaxTokens != nil {
		maxTCopy := *base.MaxTokens
		modified.MaxTokens = &maxTCopy
	}
	if base.TopK != nil {
		topKCopy := *base.TopK
		modified.TopK = &topKCopy
	}
	if base.FrequencyPenalty != nil {
		fpCopy := *base.FrequencyPenalty
		modified.FrequencyPenalty = &fpCopy
	}
	if base.PresencePenalty != nil {
		ppCopy := *base.PresencePenalty
		modified.PresencePenalty = &ppCopy
	}
	if base.Seed != nil {
		seedCopy := *base.Seed
		modified.Seed = &seedCopy
	}

	modify(&modified)
	return modified
}
