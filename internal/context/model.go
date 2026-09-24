package context

// ModelIdentity captures the identity of the LLM model being invoked.
type ModelIdentity struct {
	// Name is the canonical model name (e.g., "gpt-4", "claude-3-opus").
	Name string `json:"name"`
	// Version is the model version or snapshot identifier.
	Version string `json:"version,omitempty"`
	// Provider is the model provider (e.g., "openai", "anthropic", "google").
	Provider string `json:"provider"`
}

// ModelParameters captures the generation parameters that affect output determinism.
type ModelParameters struct {
	// Temperature controls randomness (0.0–2.0).
	Temperature *float64 `json:"temperature,omitempty"`
	// TopP is the nucleus sampling parameter (0.0–1.0).
	TopP *float64 `json:"top_p,omitempty"`
	// MaxTokens is the maximum number of tokens to generate.
	MaxTokens *int `json:"max_tokens,omitempty"`
	// TopK limits the number of tokens considered at each step.
	TopK *int `json:"top_k,omitempty"`
	// FrequencyPenalty penalizes repeated tokens.
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	// PresencePenalty penalizes tokens that have appeared at all.
	PresencePenalty *float64 `json:"presence_penalty,omitempty"`
	// Stop contains stop sequences.
	Stop []string `json:"stop,omitempty"`
	// Seed is the random seed for deterministic generation.
	Seed *int64 `json:"seed,omitempty"`
}

// PromptIdentity captures the identity of the prompt or prompt template being used.
type PromptIdentity struct {
	// TemplateHash is the SHA-256 hash of the prompt template content.
	TemplateHash string `json:"template_hash"`
	// TemplateVersion is a human-readable version identifier for the template.
	TemplateVersion string `json:"template_version,omitempty"`
	// SystemPromptHash is the SHA-256 hash of the system prompt content.
	SystemPromptHash string `json:"system_prompt_hash,omitempty"`
}

// RepositoryIdentity captures the identity of the code repository.
type RepositoryIdentity struct {
	// URL is the repository URL.
	URL string `json:"url"`
	// Branch is the active branch name.
	Branch string `json:"branch"`
	// CommitHash is the full commit SHA.
	CommitHash string `json:"commit_hash"`
}

// FileHash associates a file path with its content hash.
type FileHash struct {
	// Path is the file's path relative to the repository root.
	Path string `json:"path"`
	// Hash is the SHA-256 hash of the file contents.
	Hash string `json:"hash"`
}

// DependencyLock captures the identity of dependency lock files.
type DependencyLock struct {
	// Type identifies the lock file format (e.g., "go.sum", "package-lock.json").
	Type string `json:"type"`
	// Hash is the SHA-256 hash of the lock file contents.
	Hash string `json:"hash"`
}

// ToolSchemaVersion captures the version of a tool's schema.
type ToolSchemaVersion struct {
	// Name is the tool name.
	Name string `json:"name"`
	// Version is the schema version string.
	Version string `json:"version"`
	// SchemaHash is the SHA-256 hash of the tool schema definition.
	SchemaHash string `json:"schema_hash"`
}

// ToolOutputIdentity captures the identity of a tool's output.
type ToolOutputIdentity struct {
	// ToolName is the name of the tool that produced the output.
	ToolName string `json:"tool_name"`
	// OutputHash is the SHA-256 hash of the tool output.
	OutputHash string `json:"output_hash"`
}

// ConversationState captures the state of the conversation.
type ConversationState struct {
	// TurnCount is the number of conversational turns.
	TurnCount int `json:"turn_count"`
	// HistoryHash is the SHA-256 hash of the conversation history.
	HistoryHash string `json:"history_hash"`
}

// RetrievedContext captures the identity of RAG-retrieved documents.
type RetrievedContext struct {
	// DocumentHashes maps document identifiers to their content hashes.
	DocumentHashes map[string]string `json:"document_hashes"`
	// QueryHash is the SHA-256 hash of the retrieval query.
	QueryHash string `json:"query_hash,omitempty"`
}
