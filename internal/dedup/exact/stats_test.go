package exact

import (
	"strings"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/dedup"
)

func TestFormatStats(t *testing.T) {
	stats := &dedup.DedupStats{
		OriginalCount:        5,
		DeduplicatedCount:    3,
		DuplicatesRemoved:    2,
		BytesSaved:           100,
		EstimatedTokensSaved: 25,
		Duration:             5 * time.Millisecond,
		Strategy:             "exact",
	}

	result := FormatStats(stats)

	if !strings.Contains(result, "strategy=exact") {
		t.Errorf("expected strategy in output, got %q", result)
	}
	if !strings.Contains(result, "original=5") {
		t.Errorf("expected original=5, got %q", result)
	}
	if !strings.Contains(result, "removed=2") {
		t.Errorf("expected removed=2, got %q", result)
	}
	if !strings.Contains(result, "bytes_saved=100") {
		t.Errorf("expected bytes_saved=100, got %q", result)
	}
}

func TestFormatStats_Nil(t *testing.T) {
	result := FormatStats(nil)
	if result != "no deduplication stats available" {
		t.Errorf("unexpected result for nil stats: %q", result)
	}
}

func TestStatsToMetadata(t *testing.T) {
	stats := &dedup.DedupStats{
		OriginalCount:        10,
		DeduplicatedCount:    7,
		DuplicatesRemoved:    3,
		BytesSaved:           200,
		EstimatedTokensSaved: 50,
		Strategy:             "exact",
	}

	meta := StatsToMetadata(stats)

	if meta["dedup_strategy"] != "exact" {
		t.Errorf("expected strategy 'exact', got %q", meta["dedup_strategy"])
	}
	if meta["dedup_original_count"] != "10" {
		t.Errorf("expected original_count '10', got %q", meta["dedup_original_count"])
	}
	if meta["dedup_items_removed"] != "3" {
		t.Errorf("expected items_removed '3', got %q", meta["dedup_items_removed"])
	}
	if meta["dedup_bytes_saved"] != "200" {
		t.Errorf("expected bytes_saved '200', got %q", meta["dedup_bytes_saved"])
	}
	if meta["dedup_tokens_saved"] != "50" {
		t.Errorf("expected tokens_saved '50', got %q", meta["dedup_tokens_saved"])
	}
}

func TestStatsToMetadata_Nil(t *testing.T) {
	meta := StatsToMetadata(nil)
	if meta != nil {
		t.Errorf("expected nil metadata for nil stats, got %v", meta)
	}
}
