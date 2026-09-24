package dedup

import "time"

// ContentType identifies the type of content being deduplicated.
type ContentType string

const (
	// ContentTypeDocument represents document content (e.g., retrieved context).
	ContentTypeDocument ContentType = "document"
	// ContentTypeChunk represents text chunks from chunked content.
	ContentTypeChunk ContentType = "chunk"
	// ContentTypeToolOutput represents output from tool calls.
	ContentTypeToolOutput ContentType = "tool_output"
	// ContentTypeCodeBlock represents code blocks in content.
	ContentTypeCodeBlock ContentType = "code_block"
	// ContentTypeInstruction represents system or user instructions.
	ContentTypeInstruction ContentType = "instruction"
)

// ContentItem represents a single piece of content to be deduplicated.
type ContentItem struct {
	// ID is an optional identifier for the content item.
	ID string `json:"id,omitempty"`
	// Type classifies what kind of content this is.
	Type ContentType `json:"type"`
	// Value is the string content to deduplicate against.
	Value string `json:"value"`
	// Metadata holds additional context about the content item.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Content is the input to a deduplication operation.
type Content struct {
	// Items is the ordered list of content items to deduplicate.
	Items []ContentItem `json:"items"`
	// RequestID associates this content with a specific request.
	RequestID string `json:"request_id,omitempty"`
}

// DuplicateInfo records information about a removed duplicate.
type DuplicateInfo struct {
	// OriginalIndex is the index of the first occurrence that was kept.
	OriginalIndex int `json:"original_index"`
	// RemovedIndex is the index of the duplicate that was removed.
	RemovedIndex int `json:"removed_index"`
	// Value is the duplicated content string.
	Value string `json:"value"`
	// Type is the content type of the removed duplicate.
	Type ContentType `json:"type"`
}

// DeduplicatedContent is the output of a deduplication operation.
type DeduplicatedContent struct {
	// Items is the deduplicated list of content items, in first-occurrence order.
	Items []ContentItem `json:"items"`
	// RemovedDuplicates lists all duplicates that were removed.
	RemovedDuplicates []DuplicateInfo `json:"removed_duplicates,omitempty"`
	// Stats contains statistics about the deduplication operation.
	Stats DedupStats `json:"stats"`
}

// DedupStats contains statistics from a deduplication operation.
type DedupStats struct {
	// OriginalCount is the number of items before deduplication.
	OriginalCount int `json:"original_count"`
	// DeduplicatedCount is the number of items after deduplication.
	DeduplicatedCount int `json:"deduplicated_count"`
	// DuplicatesRemoved is the total number of duplicate items removed.
	DuplicatesRemoved int `json:"duplicates_removed"`
	// DuplicateCounts maps content values to the number of times they were duplicated.
	DuplicateCounts map[string]int `json:"duplicate_counts,omitempty"`
	// BytesSaved is the total bytes saved by removing duplicates.
	BytesSaved int64 `json:"bytes_saved"`
	// EstimatedTokensSaved is the estimated tokens saved (approx 4 chars per token).
	EstimatedTokensSaved int64 `json:"estimated_tokens_saved"`
	// Duration is how long the deduplication took.
	Duration time.Duration `json:"duration"`
	// Strategy is the name of the deduplication strategy used.
	Strategy string `json:"strategy"`
}
