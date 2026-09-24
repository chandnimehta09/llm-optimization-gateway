package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ContextIdentity is the top-level data model that captures all factors
// affecting LLM output determinism. Two requests with identical context
// identities are expected to produce the same output.
type ContextIdentity struct {
	// ID is the computed SHA-256 fingerprint of all contributing factors.
	ID string `json:"id"`
	// Version is the schema version of this identity model.
	Version string `json:"version"`
	// CreatedAt is the timestamp when this identity was computed.
	CreatedAt time.Time `json:"created_at"`

	// Model captures the LLM model identity.
	Model ModelIdentity `json:"model"`
	// Parameters captures model generation parameters.
	Parameters ModelParameters `json:"parameters"`
	// Prompt captures the prompt/template identity.
	Prompt PromptIdentity `json:"prompt"`
	// Repository captures the source repository identity.
	Repository *RepositoryIdentity `json:"repository,omitempty"`
	// FileHashes contains hashes of relevant source files.
	FileHashes []FileHash `json:"file_hashes,omitempty"`
	// DependencyLocks captures dependency lock file hashes.
	DependencyLocks []DependencyLock `json:"dependency_locks,omitempty"`
	// ToolSchemas captures tool schema versions.
	ToolSchemas []ToolSchemaVersion `json:"tool_schemas,omitempty"`
	// ToolOutputs captures tool output identities.
	ToolOutputs []ToolOutputIdentity `json:"tool_outputs,omitempty"`
	// Conversation captures the conversation state.
	Conversation *ConversationState `json:"conversation,omitempty"`
	// RetrievedContexts captures RAG document identities.
	RetrievedContexts []RetrievedContext `json:"retrieved_contexts,omitempty"`
	// CustomFactors holds any registered custom factors as serialized values.
	CustomFactors map[string]string `json:"custom_factors,omitempty"`
}

// identityVersion is the current schema version of the context identity model.
const identityVersion = "1.0.0"

// ComputeID calculates the identity fingerprint from all contributing factors.
// The fingerprint is a SHA-256 hash of a deterministic representation of all fields.
func (ci *ContextIdentity) ComputeID() string {
	h := sha256.New()

	// Model identity
	h.Write([]byte("model.name=" + ci.Model.Name + "\n"))
	h.Write([]byte("model.version=" + ci.Model.Version + "\n"))
	h.Write([]byte("model.provider=" + ci.Model.Provider + "\n"))

	// Parameters - include only set values for determinism
	if ci.Parameters.Temperature != nil {
		h.Write([]byte(fmt.Sprintf("params.temperature=%g\n", *ci.Parameters.Temperature)))
	}
	if ci.Parameters.TopP != nil {
		h.Write([]byte(fmt.Sprintf("params.top_p=%g\n", *ci.Parameters.TopP)))
	}
	if ci.Parameters.MaxTokens != nil {
		h.Write([]byte(fmt.Sprintf("params.max_tokens=%d\n", *ci.Parameters.MaxTokens)))
	}
	if ci.Parameters.TopK != nil {
		h.Write([]byte(fmt.Sprintf("params.top_k=%d\n", *ci.Parameters.TopK)))
	}
	if ci.Parameters.FrequencyPenalty != nil {
		h.Write([]byte(fmt.Sprintf("params.frequency_penalty=%g\n", *ci.Parameters.FrequencyPenalty)))
	}
	if ci.Parameters.PresencePenalty != nil {
		h.Write([]byte(fmt.Sprintf("params.presence_penalty=%g\n", *ci.Parameters.PresencePenalty)))
	}
	if ci.Parameters.Seed != nil {
		h.Write([]byte(fmt.Sprintf("params.seed=%d\n", *ci.Parameters.Seed)))
	}
	if len(ci.Parameters.Stop) > 0 {
		sorted := make([]string, len(ci.Parameters.Stop))
		copy(sorted, ci.Parameters.Stop)
		sort.Strings(sorted)
		h.Write([]byte("params.stop=" + strings.Join(sorted, ",") + "\n"))
	}

	// Prompt identity
	h.Write([]byte("prompt.template_hash=" + ci.Prompt.TemplateHash + "\n"))
	h.Write([]byte("prompt.template_version=" + ci.Prompt.TemplateVersion + "\n"))
	h.Write([]byte("prompt.system_prompt_hash=" + ci.Prompt.SystemPromptHash + "\n"))

	// Repository identity
	if ci.Repository != nil {
		h.Write([]byte("repo.url=" + ci.Repository.URL + "\n"))
		h.Write([]byte("repo.branch=" + ci.Repository.Branch + "\n"))
		h.Write([]byte("repo.commit=" + ci.Repository.CommitHash + "\n"))
	}

	// File hashes - sorted by path for determinism
	if len(ci.FileHashes) > 0 {
		sorted := make([]FileHash, len(ci.FileHashes))
		copy(sorted, ci.FileHashes)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Path < sorted[j].Path
		})
		for _, fh := range sorted {
			h.Write([]byte("file." + fh.Path + "=" + fh.Hash + "\n"))
		}
	}

	// Dependency locks - sorted by type
	if len(ci.DependencyLocks) > 0 {
		sorted := make([]DependencyLock, len(ci.DependencyLocks))
		copy(sorted, ci.DependencyLocks)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Type < sorted[j].Type
		})
		for _, dl := range sorted {
			h.Write([]byte("dep." + dl.Type + "=" + dl.Hash + "\n"))
		}
	}

	// Tool schemas - sorted by name
	if len(ci.ToolSchemas) > 0 {
		sorted := make([]ToolSchemaVersion, len(ci.ToolSchemas))
		copy(sorted, ci.ToolSchemas)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Name < sorted[j].Name
		})
		for _, ts := range sorted {
			h.Write([]byte("tool_schema." + ts.Name + "=" + ts.Version + ":" + ts.SchemaHash + "\n"))
		}
	}

	// Tool outputs - sorted by name
	if len(ci.ToolOutputs) > 0 {
		sorted := make([]ToolOutputIdentity, len(ci.ToolOutputs))
		copy(sorted, ci.ToolOutputs)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].ToolName < sorted[j].ToolName
		})
		for _, to := range sorted {
			h.Write([]byte("tool_output." + to.ToolName + "=" + to.OutputHash + "\n"))
		}
	}

	// Conversation state
	if ci.Conversation != nil {
		h.Write([]byte(fmt.Sprintf("conversation.turn_count=%d\n", ci.Conversation.TurnCount)))
		h.Write([]byte("conversation.history_hash=" + ci.Conversation.HistoryHash + "\n"))
	}

	// Retrieved contexts - sorted by query hash
	if len(ci.RetrievedContexts) > 0 {
		sorted := make([]RetrievedContext, len(ci.RetrievedContexts))
		copy(sorted, ci.RetrievedContexts)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].QueryHash < sorted[j].QueryHash
		})
		for _, rc := range sorted {
			h.Write([]byte("retrieval.query=" + rc.QueryHash + "\n"))
			docKeys := make([]string, 0, len(rc.DocumentHashes))
			for k := range rc.DocumentHashes {
				docKeys = append(docKeys, k)
			}
			sort.Strings(docKeys)
			for _, k := range docKeys {
				h.Write([]byte("retrieval.doc." + k + "=" + rc.DocumentHashes[k] + "\n"))
			}
		}
	}

	// Custom factors - sorted by key
	if len(ci.CustomFactors) > 0 {
		keys := make([]string, 0, len(ci.CustomFactors))
		for k := range ci.CustomFactors {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte("custom." + k + "=" + ci.CustomFactors[k] + "\n"))
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// Equal reports whether two context identities have the same fingerprint.
// Both identities have their IDs recomputed before comparison to ensure accuracy.
func (ci *ContextIdentity) Equal(other *ContextIdentity) bool {
	if ci == nil || other == nil {
		return ci == other
	}
	return ci.ComputeID() == other.ComputeID()
}

// Diff returns a list of human-readable differences between two identities.
// It compares each section and reports which fields differ.
func (ci *ContextIdentity) Diff(other *ContextIdentity) []string {
	if other == nil {
		return []string{"other identity is nil"}
	}

	var diffs []string

	// Model
	if ci.Model != other.Model {
		if ci.Model.Name != other.Model.Name {
			diffs = append(diffs, fmt.Sprintf("model.name: %q vs %q", ci.Model.Name, other.Model.Name))
		}
		if ci.Model.Version != other.Model.Version {
			diffs = append(diffs, fmt.Sprintf("model.version: %q vs %q", ci.Model.Version, other.Model.Version))
		}
		if ci.Model.Provider != other.Model.Provider {
			diffs = append(diffs, fmt.Sprintf("model.provider: %q vs %q", ci.Model.Provider, other.Model.Provider))
		}
	}

	// Prompt
	if ci.Prompt != other.Prompt {
		if ci.Prompt.TemplateHash != other.Prompt.TemplateHash {
			diffs = append(diffs, "prompt.template_hash differs")
		}
		if ci.Prompt.TemplateVersion != other.Prompt.TemplateVersion {
			diffs = append(diffs, fmt.Sprintf("prompt.template_version: %q vs %q", ci.Prompt.TemplateVersion, other.Prompt.TemplateVersion))
		}
		if ci.Prompt.SystemPromptHash != other.Prompt.SystemPromptHash {
			diffs = append(diffs, "prompt.system_prompt_hash differs")
		}
	}

	// Repository
	if ci.Repository != nil && other.Repository != nil {
		if *ci.Repository != *other.Repository {
			if ci.Repository.URL != other.Repository.URL {
				diffs = append(diffs, fmt.Sprintf("repository.url: %q vs %q", ci.Repository.URL, other.Repository.URL))
			}
			if ci.Repository.Branch != other.Repository.Branch {
				diffs = append(diffs, fmt.Sprintf("repository.branch: %q vs %q", ci.Repository.Branch, other.Repository.Branch))
			}
			if ci.Repository.CommitHash != other.Repository.CommitHash {
				diffs = append(diffs, fmt.Sprintf("repository.commit: %q vs %q", ci.Repository.CommitHash, other.Repository.CommitHash))
			}
		}
	} else if (ci.Repository == nil) != (other.Repository == nil) {
		diffs = append(diffs, "repository: present in one identity but not the other")
	}

	// Conversation
	if ci.Conversation != nil && other.Conversation != nil {
		if ci.Conversation.TurnCount != other.Conversation.TurnCount {
			diffs = append(diffs, fmt.Sprintf("conversation.turn_count: %d vs %d", ci.Conversation.TurnCount, other.Conversation.TurnCount))
		}
		if ci.Conversation.HistoryHash != other.Conversation.HistoryHash {
			diffs = append(diffs, "conversation.history_hash differs")
		}
	} else if (ci.Conversation == nil) != (other.Conversation == nil) {
		diffs = append(diffs, "conversation: present in one identity but not the other")
	}

	// File hashes
	if len(ci.FileHashes) != len(other.FileHashes) {
		diffs = append(diffs, fmt.Sprintf("file_hashes count: %d vs %d", len(ci.FileHashes), len(other.FileHashes)))
	}

	// Dependency locks
	if len(ci.DependencyLocks) != len(other.DependencyLocks) {
		diffs = append(diffs, fmt.Sprintf("dependency_locks count: %d vs %d", len(ci.DependencyLocks), len(other.DependencyLocks)))
	}

	// Tool schemas
	if len(ci.ToolSchemas) != len(other.ToolSchemas) {
		diffs = append(diffs, fmt.Sprintf("tool_schemas count: %d vs %d", len(ci.ToolSchemas), len(other.ToolSchemas)))
	}

	// Tool outputs
	if len(ci.ToolOutputs) != len(other.ToolOutputs) {
		diffs = append(diffs, fmt.Sprintf("tool_outputs count: %d vs %d", len(ci.ToolOutputs), len(other.ToolOutputs)))
	}

	// Retrieved contexts
	if len(ci.RetrievedContexts) != len(other.RetrievedContexts) {
		diffs = append(diffs, fmt.Sprintf("retrieved_contexts count: %d vs %d", len(ci.RetrievedContexts), len(other.RetrievedContexts)))
	}

	// Custom factors
	if len(ci.CustomFactors) != len(other.CustomFactors) {
		diffs = append(diffs, fmt.Sprintf("custom_factors count: %d vs %d", len(ci.CustomFactors), len(other.CustomFactors)))
	} else {
		for k, v := range ci.CustomFactors {
			if ov, ok := other.CustomFactors[k]; !ok {
				diffs = append(diffs, fmt.Sprintf("custom_factors: key %q missing in other", k))
			} else if v != ov {
				diffs = append(diffs, fmt.Sprintf("custom_factors.%s: %q vs %q", k, v, ov))
			}
		}
	}

	return diffs
}

// MarshalJSON implements custom JSON marshaling to ensure the ID is always current.
func (ci *ContextIdentity) MarshalJSON() ([]byte, error) {
	// Recompute ID before serialization.
	ci.ID = ci.ComputeID()
	// Use an alias to avoid infinite recursion.
	type Alias ContextIdentity
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(ci),
	})
}

// Builder constructs a ContextIdentity incrementally.
type Builder struct {
	identity *ContextIdentity
	registry *FactorRegistry
}

// NewBuilder creates a new identity builder with required model fields.
func NewBuilder(modelName, modelProvider string) *Builder {
	return &Builder{
		identity: &ContextIdentity{
			Version:   identityVersion,
			CreatedAt: time.Now().UTC(),
			Model: ModelIdentity{
				Name:     modelName,
				Provider: modelProvider,
			},
		},
	}
}

// WithModelVersion sets the model version.
func (b *Builder) WithModelVersion(version string) *Builder {
	b.identity.Model.Version = version
	return b
}

// WithTemperature sets the temperature parameter.
func (b *Builder) WithTemperature(t float64) *Builder {
	b.identity.Parameters.Temperature = &t
	return b
}

// WithTopP sets the top_p parameter.
func (b *Builder) WithTopP(p float64) *Builder {
	b.identity.Parameters.TopP = &p
	return b
}

// WithMaxTokens sets the max_tokens parameter.
func (b *Builder) WithMaxTokens(n int) *Builder {
	b.identity.Parameters.MaxTokens = &n
	return b
}

// WithTopK sets the top_k parameter.
func (b *Builder) WithTopK(k int) *Builder {
	b.identity.Parameters.TopK = &k
	return b
}

// WithFrequencyPenalty sets the frequency penalty parameter.
func (b *Builder) WithFrequencyPenalty(p float64) *Builder {
	b.identity.Parameters.FrequencyPenalty = &p
	return b
}

// WithPresencePenalty sets the presence penalty parameter.
func (b *Builder) WithPresencePenalty(p float64) *Builder {
	b.identity.Parameters.PresencePenalty = &p
	return b
}

// WithSeed sets the seed parameter.
func (b *Builder) WithSeed(seed int64) *Builder {
	b.identity.Parameters.Seed = &seed
	return b
}

// WithStop sets the stop sequences.
func (b *Builder) WithStop(stop []string) *Builder {
	b.identity.Parameters.Stop = stop
	return b
}

// WithPrompt sets the prompt identity.
func (b *Builder) WithPrompt(templateHash, templateVersion, systemPromptHash string) *Builder {
	b.identity.Prompt = PromptIdentity{
		TemplateHash:     templateHash,
		TemplateVersion:  templateVersion,
		SystemPromptHash: systemPromptHash,
	}
	return b
}

// WithRepository sets the repository identity.
func (b *Builder) WithRepository(url, branch, commitHash string) *Builder {
	b.identity.Repository = &RepositoryIdentity{
		URL:        url,
		Branch:     branch,
		CommitHash: commitHash,
	}
	return b
}

// WithFileHash adds a file hash to the identity.
func (b *Builder) WithFileHash(path, hash string) *Builder {
	b.identity.FileHashes = append(b.identity.FileHashes, FileHash{
		Path: path,
		Hash: hash,
	})
	return b
}

// WithDependencyLock adds a dependency lock hash to the identity.
func (b *Builder) WithDependencyLock(lockType, hash string) *Builder {
	b.identity.DependencyLocks = append(b.identity.DependencyLocks, DependencyLock{
		Type: lockType,
		Hash: hash,
	})
	return b
}

// WithToolSchema adds a tool schema version to the identity.
func (b *Builder) WithToolSchema(name, version, schemaHash string) *Builder {
	b.identity.ToolSchemas = append(b.identity.ToolSchemas, ToolSchemaVersion{
		Name:       name,
		Version:    version,
		SchemaHash: schemaHash,
	})
	return b
}

// WithToolOutput adds a tool output identity.
func (b *Builder) WithToolOutput(toolName, outputHash string) *Builder {
	b.identity.ToolOutputs = append(b.identity.ToolOutputs, ToolOutputIdentity{
		ToolName:   toolName,
		OutputHash: outputHash,
	})
	return b
}

// WithConversation sets the conversation state.
func (b *Builder) WithConversation(turnCount int, historyHash string) *Builder {
	b.identity.Conversation = &ConversationState{
		TurnCount:   turnCount,
		HistoryHash: historyHash,
	}
	return b
}

// WithRetrievedContext adds a RAG retrieved context.
func (b *Builder) WithRetrievedContext(queryHash string, documentHashes map[string]string) *Builder {
	b.identity.RetrievedContexts = append(b.identity.RetrievedContexts, RetrievedContext{
		QueryHash:      queryHash,
		DocumentHashes: documentHashes,
	})
	return b
}

// WithCustomFactor adds a custom factor key-value pair.
func (b *Builder) WithCustomFactor(key, value string) *Builder {
	if b.identity.CustomFactors == nil {
		b.identity.CustomFactors = make(map[string]string)
	}
	b.identity.CustomFactors[key] = value
	return b
}

// WithRegistry attaches a factor registry for automatic factor extraction.
func (b *Builder) WithRegistry(registry *FactorRegistry) *Builder {
	b.registry = registry
	return b
}

// Build finalizes and returns the ContextIdentity.
// If a registry is attached, it extracts custom factors from registered extractors.
func (b *Builder) Build() (*ContextIdentity, error) {
	if b.registry != nil {
		factors, err := b.registry.Extract()
		if err != nil {
			return nil, fmt.Errorf("extracting custom factors: %w", err)
		}
		if b.identity.CustomFactors == nil {
			b.identity.CustomFactors = make(map[string]string)
		}
		for _, f := range factors {
			b.identity.CustomFactors[f.Name()] = f.Value()
		}
	}

	b.identity.ID = b.identity.ComputeID()
	return b.identity, nil
}

// MatchResult describes the result of comparing two context identities.
type MatchResult struct {
	// IsExactMatch is true when both identities have the same fingerprint.
	IsExactMatch bool `json:"is_exact_match"`
	// Similarity is a weighted similarity score between 0.0 and 1.0.
	Similarity float64 `json:"similarity"`
	// Differences lists human-readable descriptions of differing fields.
	Differences []string `json:"differences,omitempty"`
}

// Compare performs a detailed comparison between two context identities,
// returning match status, similarity score, and specific differences.
func (ci *ContextIdentity) Compare(other *ContextIdentity) MatchResult {
	if other == nil {
		return MatchResult{
			IsExactMatch: false,
			Similarity:   0.0,
			Differences:  []string{"other identity is nil"},
		}
	}

	diffs := ci.Diff(other)
	isExact := len(diffs) == 0

	// Calculate weighted similarity.
	type weightedCheck struct {
		weight  FactorWeight
		matches bool
	}

	checks := []weightedCheck{
		{DefaultWeights[FactorCategoryModel], ci.Model == other.Model},
		{DefaultWeights[FactorCategoryPrompt], ci.Prompt == other.Prompt},
		{DefaultWeights[FactorCategoryConversation], conversationEqual(ci.Conversation, other.Conversation)},
	}

	// Repository
	repoMatch := (ci.Repository == nil && other.Repository == nil) ||
		(ci.Repository != nil && other.Repository != nil && *ci.Repository == *other.Repository)
	checks = append(checks, weightedCheck{DefaultWeights[FactorCategoryRepository], repoMatch})

	// Files
	fileMatch := fileHashesEqual(ci.FileHashes, other.FileHashes)
	checks = append(checks, weightedCheck{DefaultWeights[FactorCategoryFile], fileMatch})

	// Dependencies
	depMatch := depLocksEqual(ci.DependencyLocks, other.DependencyLocks)
	checks = append(checks, weightedCheck{DefaultWeights[FactorCategoryDependency], depMatch})

	// Tool schemas
	toolSchemaMatch := toolSchemasEqual(ci.ToolSchemas, other.ToolSchemas)
	checks = append(checks, weightedCheck{DefaultWeights[FactorCategoryTool], toolSchemaMatch})

	// Parameters match
	paramsMatch := paramsEqual(ci.Parameters, other.Parameters)
	checks = append(checks, weightedCheck{DefaultWeights[FactorCategoryModel], paramsMatch})

	var totalWeight, matchedWeight float64
	for _, c := range checks {
		totalWeight += float64(c.weight)
		if c.matches {
			matchedWeight += float64(c.weight)
		}
	}

	similarity := 0.0
	if totalWeight > 0 {
		similarity = matchedWeight / totalWeight
	}

	return MatchResult{
		IsExactMatch: isExact,
		Similarity:   similarity,
		Differences:  diffs,
	}
}

// --- helper comparison functions ---

func conversationEqual(a, b *ConversationState) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.TurnCount == b.TurnCount && a.HistoryHash == b.HistoryHash
}

func fileHashesEqual(a, b []FileHash) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]string, len(a))
	for _, f := range a {
		am[f.Path] = f.Hash
	}
	for _, f := range b {
		if am[f.Path] != f.Hash {
			return false
		}
	}
	return true
}

func depLocksEqual(a, b []DependencyLock) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]string, len(a))
	for _, d := range a {
		am[d.Type] = d.Hash
	}
	for _, d := range b {
		if am[d.Type] != d.Hash {
			return false
		}
	}
	return true
}

func toolSchemasEqual(a, b []ToolSchemaVersion) bool {
	if len(a) != len(b) {
		return false
	}
	am := make(map[string]string, len(a))
	for _, t := range a {
		am[t.Name] = t.Version + ":" + t.SchemaHash
	}
	for _, t := range b {
		if am[t.Name] != t.Version+":"+t.SchemaHash {
			return false
		}
	}
	return true
}

func paramsEqual(a, b ModelParameters) bool {
	if !floatPtrEqual(a.Temperature, b.Temperature) {
		return false
	}
	if !floatPtrEqual(a.TopP, b.TopP) {
		return false
	}
	if !intPtrEqual(a.MaxTokens, b.MaxTokens) {
		return false
	}
	if !intPtrEqual(a.TopK, b.TopK) {
		return false
	}
	if !floatPtrEqual(a.FrequencyPenalty, b.FrequencyPenalty) {
		return false
	}
	if !floatPtrEqual(a.PresencePenalty, b.PresencePenalty) {
		return false
	}
	if !int64PtrEqual(a.Seed, b.Seed) {
		return false
	}
	if len(a.Stop) != len(b.Stop) {
		return false
	}
	aCopy := make([]string, len(a.Stop))
	copy(aCopy, a.Stop)
	bCopy := make([]string, len(b.Stop))
	copy(bCopy, b.Stop)
	sort.Strings(aCopy)
	sort.Strings(bCopy)
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func int64PtrEqual(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
