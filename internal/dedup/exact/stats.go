package exact

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/llm-optimization-gateway/internal/dedup"
)

// FormatStats returns a human-readable summary of deduplication statistics.
func FormatStats(stats *dedup.DedupStats) string {
	if stats == nil {
		return "no deduplication stats available"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "strategy=%s", stats.Strategy)
	fmt.Fprintf(&b, " original=%d", stats.OriginalCount)
	fmt.Fprintf(&b, " deduplicated=%d", stats.DeduplicatedCount)
	fmt.Fprintf(&b, " removed=%d", stats.DuplicatesRemoved)
	fmt.Fprintf(&b, " bytes_saved=%d", stats.BytesSaved)
	fmt.Fprintf(&b, " tokens_saved=%d", stats.EstimatedTokensSaved)
	fmt.Fprintf(&b, " duration=%s", stats.Duration)
	return b.String()
}

// StatsToMetadata converts deduplication stats to a flat string map
// suitable for inclusion in request metadata or response headers.
func StatsToMetadata(stats *dedup.DedupStats) map[string]string {
	if stats == nil {
		return nil
	}
	m := map[string]string{
		"dedup_strategy":           stats.Strategy,
		"dedup_original_count":     strconv.Itoa(stats.OriginalCount),
		"dedup_deduplicated_count": strconv.Itoa(stats.DeduplicatedCount),
		"dedup_items_removed":      strconv.Itoa(stats.DuplicatesRemoved),
		"dedup_bytes_saved":        strconv.FormatInt(stats.BytesSaved, 10),
		"dedup_tokens_saved":       strconv.FormatInt(stats.EstimatedTokensSaved, 10),
	}
	return m
}
