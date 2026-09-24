package gateway

import (
	"github.com/llm-optimization-gateway/internal/dedup"
	"github.com/llm-optimization-gateway/internal/dedup/exact"
	"github.com/llm-optimization-gateway/internal/request"
)

// DedupResult holds the results of message deduplication in the gateway.
type DedupResult struct {
	// Messages is the deduplicated message list.
	Messages []request.Message
	// Stats contains deduplication statistics.
	Stats *dedup.DedupStats
	// ItemsRemoved is the number of duplicate items removed.
	ItemsRemoved int
}

// DedupProcessor applies content deduplication to request messages.
type DedupProcessor struct {
	config dedup.Config
}

// NewDedupProcessor creates a new DedupProcessor with the given config.
func NewDedupProcessor(config dedup.Config) *DedupProcessor {
	return &DedupProcessor{config: config}
}

// ProcessMessages deduplicates the given messages and returns the result.
func (p *DedupProcessor) ProcessMessages(messages []request.Message) *DedupResult {
	if !p.config.Enabled || len(messages) == 0 {
		stats := &dedup.DedupStats{
			OriginalCount:     len(messages),
			DeduplicatedCount: len(messages),
			DuplicateCounts:   map[string]int{},
			Strategy:          "exact",
		}
		return &DedupResult{
			Messages:     messages,
			Stats:        stats,
			ItemsRemoved: 0,
		}
	}

	dedupedMessages, stats := exact.DeduplicateMessages(messages, p.config)

	return &DedupResult{
		Messages:     dedupedMessages,
		Stats:        stats,
		ItemsRemoved: stats.DuplicatesRemoved,
	}
}
