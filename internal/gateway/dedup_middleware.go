package gateway

import (
	"context"
	"net/http"
	"strconv"

	"github.com/llm-optimization-gateway/internal/dedup"
	"github.com/llm-optimization-gateway/internal/dedup/exact"
	"github.com/llm-optimization-gateway/internal/request"
)

// DedupMiddleware wraps an http.Handler and applies content deduplication
// to incoming requests. It sits in the pipeline after normalization and
// before cache lookup.
type DedupMiddleware struct {
	next      http.Handler
	processor *DedupProcessor
}

// NewDedupMiddleware creates a new deduplication middleware wrapping the given handler.
func NewDedupMiddleware(next http.Handler, config dedup.Config) *DedupMiddleware {
	return &DedupMiddleware{
		next:      next,
		processor: NewDedupProcessor(config),
	}
}

// ServeHTTP applies deduplication to the request and passes it downstream.
// Deduplication results are stored in the request context and added as
// response headers.
func (m *DedupMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !m.processor.config.Enabled {
		m.next.ServeHTTP(w, r)
		return
	}

	// Extract the normalized request from context (set by normalization stage).
	nr := NormalizedRequestFromContext(r.Context())
	if nr == nil {
		m.next.ServeHTTP(w, r)
		return
	}

	// Process deduplication on messages.
	dedupResult := m.processor.ProcessMessages(nr.Messages)

	// Update the normalized request with deduplicated messages.
	nr.Messages = dedupResult.Messages

	// Add dedup metadata to the normalized request.
	if nr.Metadata == nil {
		nr.Metadata = make(map[string]string)
	}
	statsMetadata := exact.StatsToMetadata(dedupResult.Stats)
	for k, v := range statsMetadata {
		nr.Metadata[k] = v
	}

	// Store the dedup result in context for downstream handlers.
	ctx := contextWithDedupResult(r.Context(), dedupResult)
	ctx = contextWithNormalizedRequest(ctx, nr)
	r = r.WithContext(ctx)

	// Add dedup headers to the response.
	if dedupResult.Stats != nil && dedupResult.Stats.DuplicatesRemoved > 0 {
		w.Header().Set("X-Dedup-Items-Removed", strconv.Itoa(dedupResult.Stats.DuplicatesRemoved))
		w.Header().Set("X-Dedup-Bytes-Saved", strconv.FormatInt(dedupResult.Stats.BytesSaved, 10))
		w.Header().Set("X-Dedup-Tokens-Saved", strconv.FormatInt(dedupResult.Stats.EstimatedTokensSaved, 10))
		w.Header().Set("X-Dedup-Strategy", dedupResult.Stats.Strategy)
	}

	m.next.ServeHTTP(w, r)
}

// dedupContextKey is an unexported type for dedup-specific context keys.
type dedupContextKey int

const (
	normalizedRequestCtxKey dedupContextKey = iota
	dedupResultCtxKey
)

// NormalizedRequestFromContext retrieves the NormalizedRequest from the context.
func NormalizedRequestFromContext(ctx context.Context) *request.NormalizedRequest {
	nr, _ := ctx.Value(normalizedRequestCtxKey).(*request.NormalizedRequest)
	return nr
}

// contextWithNormalizedRequest stores a NormalizedRequest in the context.
func contextWithNormalizedRequest(ctx context.Context, nr *request.NormalizedRequest) context.Context {
	return context.WithValue(ctx, normalizedRequestCtxKey, nr)
}

// DedupResultFromContext retrieves the deduplication result from the context.
func DedupResultFromContext(ctx context.Context) *DedupResult {
	dr, _ := ctx.Value(dedupResultCtxKey).(*DedupResult)
	return dr
}

// contextWithDedupResult stores a DedupResult in the context.
func contextWithDedupResult(ctx context.Context, dr *DedupResult) context.Context {
	return context.WithValue(ctx, dedupResultCtxKey, dr)
}
