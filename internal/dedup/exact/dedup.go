// Package exact implements exact string matching deduplication.
// It removes content items whose string values are identical to previously
// seen items, preserving the order of first occurrence.
package exact

import (
	"context"
	"strings"
	"time"

	"github.com/llm-optimization-gateway/internal/dedup"
)

// Deduplicator implements exact string matching deduplication.
type Deduplicator struct {
	config dedup.Config
}

// New creates a new exact match deduplicator with the given configuration.
func New(config dedup.Config) *Deduplicator {
	return &Deduplicator{config: config}
}

// Name returns the strategy name.
func (d *Deduplicator) Name() string {
	return "exact"
}

// Deduplicate removes items with exactly matching string values.
// It preserves order of first occurrence and tracks statistics about
// removed duplicates and space/token savings.
func (d *Deduplicator) Deduplicate(ctx context.Context, content *dedup.Content) (*dedup.DeduplicatedContent, error) {
	if content == nil {
		return nil, dedup.ErrNilContent
	}

	start := time.Now()

	if len(content.Items) == 0 {
		return &dedup.DeduplicatedContent{
			Items:             []dedup.ContentItem{},
			RemovedDuplicates: nil,
			Stats: dedup.DedupStats{
				OriginalCount:     0,
				DeduplicatedCount: 0,
				DuplicatesRemoved: 0,
				DuplicateCounts:   map[string]int{},
				BytesSaved:        0,
				Duration:          time.Since(start),
				Strategy:          d.Name(),
			},
		}, nil
	}

	result := deduplicateItems(content.Items, d.config)
	result.Stats.Duration = time.Since(start)
	result.Stats.Strategy = d.Name()

	return result, nil
}

// deduplicateItems performs the actual deduplication on a slice of content items.
func deduplicateItems(items []dedup.ContentItem, config dedup.Config) *dedup.DeduplicatedContent {
	seen := make(map[string]int, len(items)) // key -> index in kept slice
	kept := make([]dedup.ContentItem, 0, len(items))
	removed := make([]dedup.DuplicateInfo, 0)
	dupCounts := make(map[string]int)
	var bytesSaved int64

	eligibleTypes := buildTypeSet(config.ContentTypes)

	for i, item := range items {
		// Skip items below minimum length.
		if config.MinContentLength > 0 && len(item.Value) < config.MinContentLength {
			kept = append(kept, item)
			continue
		}

		// Skip items of non-eligible types.
		if len(eligibleTypes) > 0 {
			if _, ok := eligibleTypes[item.Type]; !ok {
				kept = append(kept, item)
				continue
			}
		}

		key := normalizeKey(item.Value, config.CaseSensitive)

		if origIdx, exists := seen[key]; exists {
			// Duplicate found — record it.
			removed = append(removed, dedup.DuplicateInfo{
				OriginalIndex: origIdx,
				RemovedIndex:  i,
				Value:         item.Value,
				Type:          item.Type,
			})
			dupCounts[item.Value]++
			bytesSaved += int64(len(item.Value))
		} else {
			// First occurrence — keep it.
			seen[key] = len(kept)
			kept = append(kept, item)
		}
	}

	return &dedup.DeduplicatedContent{
		Items:             kept,
		RemovedDuplicates: removed,
		Stats: dedup.DedupStats{
			OriginalCount:        len(items),
			DeduplicatedCount:    len(kept),
			DuplicatesRemoved:    len(removed),
			DuplicateCounts:      dupCounts,
			BytesSaved:           bytesSaved,
			EstimatedTokensSaved: estimateTokensSaved(bytesSaved),
		},
	}
}

// normalizeKey returns the comparison key for a content value.
func normalizeKey(value string, caseSensitive bool) string {
	if caseSensitive {
		return value
	}
	return strings.ToLower(value)
}

// buildTypeSet creates a set from a slice of content types for O(1) lookup.
func buildTypeSet(types []dedup.ContentType) map[dedup.ContentType]struct{} {
	if len(types) == 0 {
		return nil
	}
	set := make(map[dedup.ContentType]struct{}, len(types))
	for _, t := range types {
		set[t] = struct{}{}
	}
	return set
}

// estimateTokensSaved approximates token savings using the ~4 chars per token heuristic.
func estimateTokensSaved(bytesSaved int64) int64 {
	if bytesSaved <= 0 {
		return 0
	}
	return (bytesSaved + 3) / 4 // round up
}
