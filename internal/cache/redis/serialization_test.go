package redis

import (
	"strings"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/cache"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newTestEntry(content string, ttl time.Duration) *cache.Entry {
	return &cache.Entry{
		Response: cache.CompletionResponse{
			ID:           "resp-001",
			Model:        "gpt-4",
			Content:      content,
			FinishReason: "stop",
			ToolCalls: []cache.ToolCallResponse{
				{
					ID:           "tc-1",
					Type:         "function",
					FunctionName: "get_weather",
					FunctionArgs: `{"city":"Seattle"}`,
				},
			},
			Usage: cache.UsageInfo{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now(),
			TTL:       ttl,
			HitCount:  0,
			Tokens:    150,
			Cost:      0.0045,
		},
		ContextHash: "sha256:abc123",
		Provider:    "openai",
		Model:       "gpt-4",
		Tags:        []string{"user:42", "session:xyz"},
	}
}

// largeContent returns a string of at least the given number of bytes.
func largeContent(minBytes int) string {
	return strings.Repeat("A", minBytes)
}

// ---------------------------------------------------------------------------
// Unit tests for serializer
// ---------------------------------------------------------------------------

func TestSerializer_MarshalUnmarshal_NoCompression(t *testing.T) {
	s := newSerializer(false, 1024)
	entry := newTestEntry("hello world", 5*time.Minute)

	data, err := s.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Marshal() returned empty data")
	}

	// Without compression, data should be plain JSON (not gzip magic).
	if isCompressed(data) {
		t.Error("expected uncompressed data without compression enabled")
	}

	restored, err := s.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if restored.Response.Content != entry.Response.Content {
		t.Errorf("Content = %q, want %q", restored.Response.Content, entry.Response.Content)
	}
	if restored.Response.ID != entry.Response.ID {
		t.Errorf("ID = %q, want %q", restored.Response.ID, entry.Response.ID)
	}
	if restored.Provider != entry.Provider {
		t.Errorf("Provider = %q, want %q", restored.Provider, entry.Provider)
	}
	if restored.Model != entry.Model {
		t.Errorf("Model = %q, want %q", restored.Model, entry.Model)
	}
	if restored.ContextHash != entry.ContextHash {
		t.Errorf("ContextHash = %q, want %q", restored.ContextHash, entry.ContextHash)
	}
	if len(restored.Tags) != len(entry.Tags) {
		t.Errorf("Tags length = %d, want %d", len(restored.Tags), len(entry.Tags))
	}
	if restored.Metadata.TTL != entry.Metadata.TTL {
		t.Errorf("TTL = %v, want %v", restored.Metadata.TTL, entry.Metadata.TTL)
	}
	if restored.Metadata.Tokens != entry.Metadata.Tokens {
		t.Errorf("Tokens = %d, want %d", restored.Metadata.Tokens, entry.Metadata.Tokens)
	}
	if restored.Response.Usage.TotalTokens != entry.Response.Usage.TotalTokens {
		t.Errorf("TotalTokens = %d, want %d", restored.Response.Usage.TotalTokens, entry.Response.Usage.TotalTokens)
	}
	if len(restored.Response.ToolCalls) != 1 {
		t.Fatalf("ToolCalls length = %d, want 1", len(restored.Response.ToolCalls))
	}
	if restored.Response.ToolCalls[0].FunctionName != "get_weather" {
		t.Errorf("ToolCalls[0].FunctionName = %q, want %q", restored.Response.ToolCalls[0].FunctionName, "get_weather")
	}
}

func TestSerializer_MarshalUnmarshal_WithCompression(t *testing.T) {
	s := newSerializer(true, 1024) // compress entries >= 1KB

	// Create entry with large content to trigger compression.
	entry := newTestEntry(largeContent(2048), 5*time.Minute)

	data, err := s.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	// Data should be compressed (gzip magic bytes).
	if !isCompressed(data) {
		t.Error("expected compressed data for large entry with compression enabled")
	}

	restored, err := s.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if restored.Response.Content != entry.Response.Content {
		t.Errorf("Content length = %d, want %d", len(restored.Response.Content), len(entry.Response.Content))
	}
	if restored.Provider != entry.Provider {
		t.Errorf("Provider = %q, want %q", restored.Provider, entry.Provider)
	}
}

func TestSerializer_SmallEntryNotCompressed(t *testing.T) {
	s := newSerializer(true, 1024) // compress only entries >= 1KB

	entry := newTestEntry("small", 5*time.Minute)

	data, err := s.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	// Small entry should not be compressed even when compression is enabled.
	if isCompressed(data) {
		t.Error("small entry should not be compressed")
	}

	restored, err := s.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if restored.Response.Content != "small" {
		t.Errorf("Content = %q, want %q", restored.Response.Content, "small")
	}
}

func TestSerializer_MarshalNilEntry(t *testing.T) {
	s := newSerializer(false, 1024)

	_, err := s.Marshal(nil)
	if err == nil {
		t.Error("expected error for nil entry")
	}
}

func TestSerializer_UnmarshalEmptyData(t *testing.T) {
	s := newSerializer(false, 1024)

	_, err := s.Unmarshal(nil)
	if err == nil {
		t.Error("expected error for nil data")
	}

	_, err = s.Unmarshal([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
}

func TestSerializer_UnmarshalInvalidJSON(t *testing.T) {
	s := newSerializer(false, 1024)

	_, err := s.Unmarshal([]byte("not valid json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSerializer_UnmarshalCorruptedGzip(t *testing.T) {
	s := newSerializer(false, 1024)

	// Start with gzip magic bytes but garbage data.
	data := []byte{0x1f, 0x8b, 0x00, 0x00, 0x00}

	_, err := s.Unmarshal(data)
	if err == nil {
		t.Error("expected error for corrupted gzip data")
	}
}

func TestSerializer_CompressionReducesSize(t *testing.T) {
	s := newSerializer(true, 1024)
	sNoCompress := newSerializer(false, 1024)

	// Create entry with highly compressible content.
	entry := newTestEntry(largeContent(4096), 5*time.Minute)

	compressed, err := s.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal() with compression error: %v", err)
	}

	uncompressed, err := sNoCompress.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal() without compression error: %v", err)
	}

	if len(compressed) >= len(uncompressed) {
		t.Errorf("compressed size (%d) should be smaller than uncompressed (%d)", len(compressed), len(uncompressed))
	}
}

func TestSerializer_RoundTripPreservesAllFields(t *testing.T) {
	s := newSerializer(false, 1024)

	original := &cache.Entry{
		Response: cache.CompletionResponse{
			ID:           "resp-roundtrip",
			Model:        "claude-3-opus",
			Content:      "round trip test content",
			FinishReason: "length",
			ToolCalls: []cache.ToolCallResponse{
				{ID: "tc-a", Type: "function", FunctionName: "search", FunctionArgs: `{"q":"test"}`},
				{ID: "tc-b", Type: "function", FunctionName: "fetch", FunctionArgs: `{"url":"http://example.com"}`},
			},
			Usage: cache.UsageInfo{
				PromptTokens:     200,
				CompletionTokens: 100,
				TotalTokens:      300,
			},
		},
		Metadata: cache.EntryMetadata{
			CreatedAt: time.Now().Truncate(time.Millisecond),
			TTL:       10 * time.Minute,
			HitCount:  5,
			Tokens:    300,
			Cost:      0.025,
		},
		ContextHash: "sha256:xyz789",
		Provider:    "anthropic",
		Model:       "claude-3-opus",
		Tags:        []string{"env:staging", "team:platform", "user:99"},
	}

	data, err := s.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}

	restored, err := s.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	// Verify all fields.
	if restored.Response.ID != original.Response.ID {
		t.Errorf("Response.ID = %q, want %q", restored.Response.ID, original.Response.ID)
	}
	if restored.Response.Model != original.Response.Model {
		t.Errorf("Response.Model = %q, want %q", restored.Response.Model, original.Response.Model)
	}
	if restored.Response.Content != original.Response.Content {
		t.Errorf("Response.Content = %q, want %q", restored.Response.Content, original.Response.Content)
	}
	if restored.Response.FinishReason != original.Response.FinishReason {
		t.Errorf("Response.FinishReason = %q, want %q", restored.Response.FinishReason, original.Response.FinishReason)
	}
	if restored.Response.Usage.PromptTokens != original.Response.Usage.PromptTokens {
		t.Errorf("PromptTokens = %d, want %d", restored.Response.Usage.PromptTokens, original.Response.Usage.PromptTokens)
	}
	if restored.Response.Usage.CompletionTokens != original.Response.Usage.CompletionTokens {
		t.Errorf("CompletionTokens = %d, want %d", restored.Response.Usage.CompletionTokens, original.Response.Usage.CompletionTokens)
	}
	if restored.Metadata.TTL != original.Metadata.TTL {
		t.Errorf("TTL = %v, want %v", restored.Metadata.TTL, original.Metadata.TTL)
	}
	if restored.Metadata.HitCount != original.Metadata.HitCount {
		t.Errorf("HitCount = %d, want %d", restored.Metadata.HitCount, original.Metadata.HitCount)
	}
	if restored.Metadata.Tokens != original.Metadata.Tokens {
		t.Errorf("Tokens = %d, want %d", restored.Metadata.Tokens, original.Metadata.Tokens)
	}
	if restored.Metadata.Cost != original.Metadata.Cost {
		t.Errorf("Cost = %f, want %f", restored.Metadata.Cost, original.Metadata.Cost)
	}
	if restored.ContextHash != original.ContextHash {
		t.Errorf("ContextHash = %q, want %q", restored.ContextHash, original.ContextHash)
	}
	if restored.Provider != original.Provider {
		t.Errorf("Provider = %q, want %q", restored.Provider, original.Provider)
	}
	if restored.Model != original.Model {
		t.Errorf("Model = %q, want %q", restored.Model, original.Model)
	}
	if len(restored.Tags) != len(original.Tags) {
		t.Fatalf("Tags length = %d, want %d", len(restored.Tags), len(original.Tags))
	}
	for i := range original.Tags {
		if restored.Tags[i] != original.Tags[i] {
			t.Errorf("Tags[%d] = %q, want %q", i, restored.Tags[i], original.Tags[i])
		}
	}
	if len(restored.Response.ToolCalls) != len(original.Response.ToolCalls) {
		t.Fatalf("ToolCalls length = %d, want %d", len(restored.Response.ToolCalls), len(original.Response.ToolCalls))
	}
	for i := range original.Response.ToolCalls {
		if restored.Response.ToolCalls[i].ID != original.Response.ToolCalls[i].ID {
			t.Errorf("ToolCalls[%d].ID = %q, want %q", i, restored.Response.ToolCalls[i].ID, original.Response.ToolCalls[i].ID)
		}
		if restored.Response.ToolCalls[i].FunctionName != original.Response.ToolCalls[i].FunctionName {
			t.Errorf("ToolCalls[%d].FunctionName = %q, want %q", i, restored.Response.ToolCalls[i].FunctionName, original.Response.ToolCalls[i].FunctionName)
		}
	}
}

func TestIsCompressed(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"gzip magic", []byte{0x1f, 0x8b, 0x08}, true},
		{"plain text", []byte(`{"key":"value"}`), false},
		{"empty", []byte{}, false},
		{"single byte", []byte{0x1f}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCompressed(tt.data); got != tt.want {
				t.Errorf("isCompressed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompressDecompress(t *testing.T) {
	original := []byte("hello world this is a compression round trip test")

	compressed, err := compress(original)
	if err != nil {
		t.Fatalf("compress() error: %v", err)
	}
	if len(compressed) == 0 {
		t.Fatal("compress() returned empty data")
	}

	decompressed, err := decompress(compressed)
	if err != nil {
		t.Fatalf("decompress() error: %v", err)
	}

	if string(decompressed) != string(original) {
		t.Errorf("decompress(compress(data)) = %q, want %q", decompressed, original)
	}
}

// ---------------------------------------------------------------------------
// Benchmark tests for serialization performance
// ---------------------------------------------------------------------------

func BenchmarkSerializer_Marshal_Small(b *testing.B) {
	s := newSerializer(false, 1024)
	entry := newTestEntry("small content", 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Marshal(entry)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerializer_Marshal_Large(b *testing.B) {
	s := newSerializer(false, 1024)
	entry := newTestEntry(largeContent(4096), 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Marshal(entry)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerializer_Marshal_LargeCompressed(b *testing.B) {
	s := newSerializer(true, 1024)
	entry := newTestEntry(largeContent(4096), 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Marshal(entry)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerializer_Unmarshal_Small(b *testing.B) {
	s := newSerializer(false, 1024)
	entry := newTestEntry("small content", 5*time.Minute)
	data, _ := s.Marshal(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Unmarshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerializer_Unmarshal_Large(b *testing.B) {
	s := newSerializer(false, 1024)
	entry := newTestEntry(largeContent(4096), 5*time.Minute)
	data, _ := s.Marshal(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Unmarshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerializer_Unmarshal_LargeCompressed(b *testing.B) {
	s := newSerializer(true, 1024)
	entry := newTestEntry(largeContent(4096), 5*time.Minute)
	data, _ := s.Marshal(entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := s.Unmarshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerializer_RoundTrip_Small(b *testing.B) {
	s := newSerializer(false, 1024)
	entry := newTestEntry("small content", 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := s.Marshal(entry)
		if err != nil {
			b.Fatal(err)
		}
		_, err = s.Unmarshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSerializer_RoundTrip_LargeCompressed(b *testing.B) {
	s := newSerializer(true, 1024)
	entry := newTestEntry(largeContent(4096), 5*time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := s.Marshal(entry)
		if err != nil {
			b.Fatal(err)
		}
		_, err = s.Unmarshal(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}
