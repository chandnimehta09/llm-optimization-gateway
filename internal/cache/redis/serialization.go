package redis

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"

	"github.com/llm-optimization-gateway/internal/cache"
)

const (
	// compressionMagic is a two-byte prefix that identifies gzip-compressed data.
	// This allows us to detect compressed entries on read without external metadata.
	compressionMagic = "\x1f\x8b"
)

// serializer handles JSON encoding/decoding of cache entries with optional
// gzip compression for large payloads.
type serializer struct {
	enableCompression    bool
	compressionThreshold int
}

// newSerializer creates a serializer with the given compression settings.
func newSerializer(enableCompression bool, compressionThreshold int) *serializer {
	return &serializer{
		enableCompression:    enableCompression,
		compressionThreshold: compressionThreshold,
	}
}

// Marshal serializes a cache entry to bytes. If compression is enabled and the
// JSON payload exceeds the compression threshold, the data is gzip-compressed.
func (s *serializer) Marshal(entry *cache.Entry) ([]byte, error) {
	if entry == nil {
		return nil, fmt.Errorf("cannot marshal nil entry")
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	if s.enableCompression && len(data) >= s.compressionThreshold {
		compressed, err := compress(data)
		if err != nil {
			return nil, fmt.Errorf("compress: %w", err)
		}
		return compressed, nil
	}

	return data, nil
}

// Unmarshal deserializes bytes back into a cache entry. It automatically
// detects and decompresses gzip-compressed data.
func (s *serializer) Unmarshal(data []byte) (*cache.Entry, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("cannot unmarshal empty data")
	}

	// Detect gzip magic bytes.
	if isCompressed(data) {
		decompressed, err := decompress(data)
		if err != nil {
			return nil, fmt.Errorf("decompress: %w", err)
		}
		data = decompressed
	}

	var entry cache.Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	return &entry, nil
}

// isCompressed checks if the data starts with the gzip magic bytes.
func isCompressed(data []byte) bool {
	return len(data) >= 2 && data[0] == compressionMagic[0] && data[1] == compressionMagic[1]
}

// compress applies gzip compression to the input data.
func compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}

	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompress applies gzip decompression to the input data.
func decompress(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return decompressed, nil
}
