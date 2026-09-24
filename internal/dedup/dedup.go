// Package dedup provides content deduplication for the LLM optimization gateway.
// It removes duplicate strings and chunks from request content before sending
// to LLM providers, reducing token usage and costs.
package dedup

import "context"

// Deduplicator defines the interface for content deduplication strategies.
// Implementations must be safe for concurrent use.
type Deduplicator interface {
	// Deduplicate removes duplicate content items from the input.
	// It preserves the order of first occurrence and tracks statistics
	// about removed duplicates and space savings.
	Deduplicate(ctx context.Context, content *Content) (*DeduplicatedContent, error)

	// Name returns the name of the deduplication strategy.
	Name() string
}
