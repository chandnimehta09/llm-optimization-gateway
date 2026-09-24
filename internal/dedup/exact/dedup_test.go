package exact

import (
	"context"
	"testing"

	"github.com/llm-optimization-gateway/internal/dedup"
)

func TestDeduplicate_DuplicatesRemoved(t *testing.T) {
	// A + A + B → A + B
	d := New(dedup.DefaultConfig())
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "doc1", Type: dedup.ContentTypeDocument},
			{Value: "doc1", Type: dedup.ContentTypeDocument},
			{Value: "doc2", Type: dedup.ContentTypeDocument},
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Value != "doc1" {
		t.Errorf("expected first item to be 'doc1', got %q", result.Items[0].Value)
	}
	if result.Items[1].Value != "doc2" {
		t.Errorf("expected second item to be 'doc2', got %q", result.Items[1].Value)
	}

	if result.Stats.DuplicatesRemoved != 1 {
		t.Errorf("expected 1 duplicate removed, got %d", result.Stats.DuplicatesRemoved)
	}
	if result.Stats.DuplicateCounts["doc1"] != 1 {
		t.Errorf("expected duplicate_count[doc1]=1, got %d", result.Stats.DuplicateCounts["doc1"])
	}
	if result.Stats.OriginalCount != 3 {
		t.Errorf("expected original count 3, got %d", result.Stats.OriginalCount)
	}
	if result.Stats.DeduplicatedCount != 2 {
		t.Errorf("expected deduplicated count 2, got %d", result.Stats.DeduplicatedCount)
	}
}

func TestDeduplicate_NoDuplicates(t *testing.T) {
	// A + B (no duplicates) → A + B unchanged
	d := New(dedup.DefaultConfig())
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "doc1", Type: dedup.ContentTypeDocument},
			{Value: "doc2", Type: dedup.ContentTypeDocument},
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Stats.DuplicatesRemoved != 0 {
		t.Errorf("expected 0 duplicates removed, got %d", result.Stats.DuplicatesRemoved)
	}
	if result.Stats.BytesSaved != 0 {
		t.Errorf("expected 0 bytes saved, got %d", result.Stats.BytesSaved)
	}
}

func TestDeduplicate_EmptyInput(t *testing.T) {
	// Empty input → empty output
	d := New(dedup.DefaultConfig())
	content := &dedup.Content{
		Items: []dedup.ContentItem{},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result.Items))
	}
	if result.Stats.OriginalCount != 0 {
		t.Errorf("expected original count 0, got %d", result.Stats.OriginalCount)
	}
	if result.Stats.DuplicatesRemoved != 0 {
		t.Errorf("expected 0 duplicates removed, got %d", result.Stats.DuplicatesRemoved)
	}
}

func TestDeduplicate_NilContent(t *testing.T) {
	d := New(dedup.DefaultConfig())
	_, err := d.Deduplicate(context.Background(), nil)
	if err != dedup.ErrNilContent {
		t.Errorf("expected ErrNilContent, got %v", err)
	}
}

func TestDeduplicate_CaseSensitive(t *testing.T) {
	// Case-sensitive mode: "Hello" and "hello" are different.
	config := dedup.Config{
		Enabled:       true,
		Strategy:      "exact",
		CaseSensitive: true,
	}
	d := New(config)
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "Hello", Type: dedup.ContentTypeDocument},
			{Value: "hello", Type: dedup.ContentTypeDocument},
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items (case-sensitive), got %d", len(result.Items))
	}
	if result.Stats.DuplicatesRemoved != 0 {
		t.Errorf("expected 0 duplicates removed, got %d", result.Stats.DuplicatesRemoved)
	}
}

func TestDeduplicate_CaseInsensitive(t *testing.T) {
	// Case-insensitive mode: "Hello" and "hello" are treated as duplicates.
	config := dedup.Config{
		Enabled:       true,
		Strategy:      "exact",
		CaseSensitive: false,
	}
	d := New(config)
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "Hello", Type: dedup.ContentTypeDocument},
			{Value: "hello", Type: dedup.ContentTypeDocument},
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item (case-insensitive), got %d", len(result.Items))
	}
	if result.Items[0].Value != "Hello" {
		t.Errorf("expected first occurrence 'Hello' to be kept, got %q", result.Items[0].Value)
	}
	if result.Stats.DuplicatesRemoved != 1 {
		t.Errorf("expected 1 duplicate removed, got %d", result.Stats.DuplicatesRemoved)
	}
}

func TestDeduplicate_PreservesOrder(t *testing.T) {
	d := New(dedup.DefaultConfig())
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "cherry", Type: dedup.ContentTypeDocument},
			{Value: "apple", Type: dedup.ContentTypeDocument},
			{Value: "banana", Type: dedup.ContentTypeDocument},
			{Value: "apple", Type: dedup.ContentTypeDocument},
			{Value: "cherry", Type: dedup.ContentTypeDocument},
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"cherry", "apple", "banana"}
	if len(result.Items) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(result.Items))
	}
	for i, want := range expected {
		if result.Items[i].Value != want {
			t.Errorf("item[%d]: expected %q, got %q", i, want, result.Items[i].Value)
		}
	}
}

func TestDeduplicate_MultipleDuplicates(t *testing.T) {
	d := New(dedup.DefaultConfig())
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "a", Type: dedup.ContentTypeDocument},
			{Value: "a", Type: dedup.ContentTypeDocument},
			{Value: "a", Type: dedup.ContentTypeDocument},
			{Value: "b", Type: dedup.ContentTypeChunk},
			{Value: "b", Type: dedup.ContentTypeChunk},
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Stats.DuplicatesRemoved != 3 {
		t.Errorf("expected 3 duplicates removed, got %d", result.Stats.DuplicatesRemoved)
	}
	if result.Stats.DuplicateCounts["a"] != 2 {
		t.Errorf("expected duplicate_count[a]=2, got %d", result.Stats.DuplicateCounts["a"])
	}
	if result.Stats.DuplicateCounts["b"] != 1 {
		t.Errorf("expected duplicate_count[b]=1, got %d", result.Stats.DuplicateCounts["b"])
	}
}

func TestDeduplicate_MinContentLength(t *testing.T) {
	config := dedup.Config{
		Enabled:          true,
		Strategy:         "exact",
		CaseSensitive:    true,
		MinContentLength: 5,
	}
	d := New(config)
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "hi", Type: dedup.ContentTypeDocument}, // too short, kept
			{Value: "hi", Type: dedup.ContentTypeDocument}, // too short, kept
			{Value: "hello world", Type: dedup.ContentTypeDocument},
			{Value: "hello world", Type: dedup.ContentTypeDocument}, // duplicate, removed
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result.Items))
	}
	if result.Stats.DuplicatesRemoved != 1 {
		t.Errorf("expected 1 duplicate removed, got %d", result.Stats.DuplicatesRemoved)
	}
}

func TestDeduplicate_ContentTypeFilter(t *testing.T) {
	config := dedup.Config{
		Enabled:       true,
		Strategy:      "exact",
		CaseSensitive: true,
		ContentTypes:  []dedup.ContentType{dedup.ContentTypeDocument},
	}
	d := New(config)
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "doc", Type: dedup.ContentTypeDocument},
			{Value: "doc", Type: dedup.ContentTypeDocument}, // dup, removed
			{Value: "code", Type: dedup.ContentTypeCodeBlock},
			{Value: "code", Type: dedup.ContentTypeCodeBlock}, // not eligible, kept
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result.Items))
	}
	if result.Stats.DuplicatesRemoved != 1 {
		t.Errorf("expected 1 duplicate removed, got %d", result.Stats.DuplicatesRemoved)
	}
}

func TestDeduplicate_BytesAndTokensSaved(t *testing.T) {
	d := New(dedup.DefaultConfig())
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "twelve chars", Type: dedup.ContentTypeDocument}, // 12 bytes
			{Value: "twelve chars", Type: dedup.ContentTypeDocument}, // duplicate
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Stats.BytesSaved != 12 {
		t.Errorf("expected 12 bytes saved, got %d", result.Stats.BytesSaved)
	}
	// (12 + 3) / 4 = 3 tokens.
	if result.Stats.EstimatedTokensSaved != 3 {
		t.Errorf("expected 3 tokens saved, got %d", result.Stats.EstimatedTokensSaved)
	}
}

func TestDeduplicate_RemovedDuplicatesTracked(t *testing.T) {
	d := New(dedup.DefaultConfig())
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "first", Type: dedup.ContentTypeDocument},
			{Value: "second", Type: dedup.ContentTypeChunk},
			{Value: "first", Type: dedup.ContentTypeDocument},
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.RemovedDuplicates) != 1 {
		t.Fatalf("expected 1 removed duplicate, got %d", len(result.RemovedDuplicates))
	}
	dup := result.RemovedDuplicates[0]
	if dup.OriginalIndex != 0 {
		t.Errorf("expected original index 0, got %d", dup.OriginalIndex)
	}
	if dup.RemovedIndex != 2 {
		t.Errorf("expected removed index 2, got %d", dup.RemovedIndex)
	}
	if dup.Value != "first" {
		t.Errorf("expected value 'first', got %q", dup.Value)
	}
	if dup.Type != dedup.ContentTypeDocument {
		t.Errorf("expected type 'document', got %q", dup.Type)
	}
}

func TestDeduplicate_DifferentTypesSameValue(t *testing.T) {
	// Same value but different types — still deduped (matching by string value).
	d := New(dedup.DefaultConfig())
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "same", Type: dedup.ContentTypeDocument},
			{Value: "same", Type: dedup.ContentTypeChunk},
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].Type != dedup.ContentTypeDocument {
		t.Errorf("expected first occurrence type 'document', got %q", result.Items[0].Type)
	}
}

func TestName(t *testing.T) {
	d := New(dedup.DefaultConfig())
	if d.Name() != "exact" {
		t.Errorf("expected name 'exact', got %q", d.Name())
	}
}

func TestDeduplicate_StatsFields(t *testing.T) {
	d := New(dedup.DefaultConfig())
	content := &dedup.Content{
		Items: []dedup.ContentItem{
			{Value: "a", Type: dedup.ContentTypeDocument},
			{Value: "b", Type: dedup.ContentTypeDocument},
			{Value: "a", Type: dedup.ContentTypeDocument},
		},
	}

	result, err := d.Deduplicate(context.Background(), content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Stats.Strategy != "exact" {
		t.Errorf("expected strategy 'exact', got %q", result.Stats.Strategy)
	}
	if result.Stats.Duration <= 0 {
		t.Errorf("expected positive duration, got %v", result.Stats.Duration)
	}
}
