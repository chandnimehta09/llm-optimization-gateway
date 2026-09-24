package context

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Default constants for cache key construction.
const (
	// DefaultGatewayPrefix is the default gateway identifier used as the key prefix.
	DefaultGatewayPrefix = "gw"

	// DefaultCacheVersion is the default cache key schema version.
	DefaultCacheVersion = "v1"

	// KeySeparator is the delimiter between key segments.
	KeySeparator = ":"

	// MaxKeyLength is the practical Redis key length limit (250 chars).
	MaxKeyLength = 250

	// truncatedHashLength is the length of the truncated hash used when
	// the full key exceeds MaxKeyLength. We keep enough hex chars to make
	// collisions practically impossible.
	truncatedHashLength = 16
)

// CacheKey represents a fully constructed cache key with its metadata.
type CacheKey struct {
	// Raw is the full key string ready for use with a cache backend.
	Raw string `json:"raw"`
	// GatewayPrefix is the gateway identifier prefix.
	GatewayPrefix string `json:"gateway_prefix"`
	// Version is the cache key schema version.
	Version string `json:"version"`
	// Model is the model identifier segment.
	Model string `json:"model"`
	// ContextHash is the context identity hash segment.
	ContextHash string `json:"context_hash"`
	// Namespace is the optional namespace applied to the key.
	Namespace *Namespace `json:"namespace,omitempty"`
	// PartIndex is the optional part index for composite keys (0 means not a composite key).
	PartIndex int `json:"part_index,omitempty"`
	// TotalParts is the total number of parts for composite keys (0 means not a composite key).
	TotalParts int `json:"total_parts,omitempty"`
	// Truncated indicates whether the key was truncated to fit within MaxKeyLength.
	Truncated bool `json:"truncated,omitempty"`
}

// CacheKeyBuilder constructs cache keys from context identities.
type CacheKeyBuilder struct {
	gatewayPrefix string
	cacheVersion  string
	namespace     *Namespace
	maxKeyLength  int
}

// CacheKeyBuilderOption is a functional option for configuring a CacheKeyBuilder.
type CacheKeyBuilderOption func(*CacheKeyBuilder)

// WithGatewayPrefix sets a custom gateway prefix.
func WithGatewayPrefix(prefix string) CacheKeyBuilderOption {
	return func(b *CacheKeyBuilder) {
		b.gatewayPrefix = prefix
	}
}

// WithCacheVersion sets a custom cache version for key construction.
func WithCacheVersion(version string) CacheKeyBuilderOption {
	return func(b *CacheKeyBuilder) {
		b.cacheVersion = version
	}
}

// WithNamespace sets the namespace for constructed keys.
func WithNamespace(ns *Namespace) CacheKeyBuilderOption {
	return func(b *CacheKeyBuilder) {
		b.namespace = ns
	}
}

// WithMaxKeyLength sets a custom maximum key length.
func WithMaxKeyLength(maxLen int) CacheKeyBuilderOption {
	return func(b *CacheKeyBuilder) {
		if maxLen > 0 {
			b.maxKeyLength = maxLen
		}
	}
}

// NewCacheKeyBuilder creates a new CacheKeyBuilder with the given options.
func NewCacheKeyBuilder(opts ...CacheKeyBuilderOption) *CacheKeyBuilder {
	b := &CacheKeyBuilder{
		gatewayPrefix: DefaultGatewayPrefix,
		cacheVersion:  DefaultCacheVersion,
		maxKeyLength:  MaxKeyLength,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Build constructs a CacheKey from a context identity hash and model identifier.
// The resulting key format is: [namespace:]<prefix>:<version>:model:<model>:<context_hash>
func (b *CacheKeyBuilder) Build(model, contextHash string) (*CacheKey, error) {
	if model == "" {
		return nil, errors.New("model identifier must not be empty")
	}
	if contextHash == "" {
		return nil, errors.New("context hash must not be empty")
	}

	// Sanitize model name: replace separators that would conflict with key format.
	sanitizedModel := sanitizeKeySegment(model)

	key := &CacheKey{
		GatewayPrefix: b.gatewayPrefix,
		Version:       b.cacheVersion,
		Model:         sanitizedModel,
		ContextHash:   contextHash,
		Namespace:     b.namespace,
	}

	key.Raw = b.formatKey(key)

	// Enforce key length limit.
	if len(key.Raw) > b.maxKeyLength {
		key = b.truncateKey(key)
	}

	return key, nil
}

// BuildFromIdentity constructs a CacheKey directly from a ContextIdentity.
// It computes the identity hash and uses the model name as the model identifier.
func (b *CacheKeyBuilder) BuildFromIdentity(identity *ContextIdentity) (*CacheKey, error) {
	if identity == nil {
		return nil, errors.New("context identity must not be nil")
	}
	hash := identity.ComputeID()
	return b.Build(identity.Model.Name, hash)
}

// BuildComposite constructs a set of cache keys for partial caching, where
// each part of the response can be cached independently. The totalParts
// parameter specifies how many parts the response is divided into.
func (b *CacheKeyBuilder) BuildComposite(model, contextHash string, totalParts int) ([]*CacheKey, error) {
	if totalParts <= 0 {
		return nil, errors.New("total parts must be greater than zero")
	}
	if model == "" {
		return nil, errors.New("model identifier must not be empty")
	}
	if contextHash == "" {
		return nil, errors.New("context hash must not be empty")
	}

	keys := make([]*CacheKey, totalParts)
	sanitizedModel := sanitizeKeySegment(model)

	for i := 0; i < totalParts; i++ {
		partHash := computePartHash(contextHash, i, totalParts)
		key := &CacheKey{
			GatewayPrefix: b.gatewayPrefix,
			Version:       b.cacheVersion,
			Model:         sanitizedModel,
			ContextHash:   partHash,
			Namespace:     b.namespace,
			PartIndex:     i + 1,
			TotalParts:    totalParts,
		}
		key.Raw = b.formatCompositeKey(key)

		if len(key.Raw) > b.maxKeyLength {
			key = b.truncateKey(key)
		}
		keys[i] = key
	}
	return keys, nil
}

// formatKey produces the raw key string for a non-composite key.
func (b *CacheKeyBuilder) formatKey(key *CacheKey) string {
	segments := b.baseSegments(key)
	segments = append(segments, key.ContextHash)
	return strings.Join(segments, KeySeparator)
}

// formatCompositeKey produces the raw key string for a composite key.
func (b *CacheKeyBuilder) formatCompositeKey(key *CacheKey) string {
	segments := b.baseSegments(key)
	segments = append(segments, key.ContextHash)
	segments = append(segments, fmt.Sprintf("part%d_%d", key.PartIndex, key.TotalParts))
	return strings.Join(segments, KeySeparator)
}

// baseSegments returns the common prefix segments for any key.
func (b *CacheKeyBuilder) baseSegments(key *CacheKey) []string {
	var segments []string
	if key.Namespace != nil {
		segments = append(segments, key.Namespace.Prefix())
	}
	segments = append(segments, key.GatewayPrefix, key.Version, "model", key.Model)
	return segments
}

// truncateKey shortens the key to fit within maxKeyLength while preserving
// collision resistance by hashing the full key to produce a suffix.
func (b *CacheKeyBuilder) truncateKey(key *CacheKey) *CacheKey {
	fullKey := key.Raw

	// Hash the full key to produce a collision-resistant truncated suffix.
	h := sha256.Sum256([]byte(fullKey))
	hashSuffix := hex.EncodeToString(h[:])[:truncatedHashLength]

	// Build a truncated key: keep the base segments and append the hash suffix.
	var segments []string
	if key.Namespace != nil {
		segments = append(segments, key.Namespace.Prefix())
	}
	segments = append(segments, key.GatewayPrefix, key.Version, "model", key.Model)
	prefix := strings.Join(segments, KeySeparator)

	// Available space for the hash suffix + separator.
	available := b.maxKeyLength - len(prefix) - len(KeySeparator)
	if available < truncatedHashLength {
		// Even the prefix is too long; truncate the prefix as well.
		maxPrefix := b.maxKeyLength - truncatedHashLength - len(KeySeparator)
		if maxPrefix < 0 {
			maxPrefix = 0
		}
		if maxPrefix < len(prefix) {
			prefix = prefix[:maxPrefix]
		}
	}

	truncated := prefix + KeySeparator + hashSuffix
	return &CacheKey{
		Raw:           truncated,
		GatewayPrefix: key.GatewayPrefix,
		Version:       key.Version,
		Model:         key.Model,
		ContextHash:   hashSuffix,
		Namespace:     key.Namespace,
		PartIndex:     key.PartIndex,
		TotalParts:    key.TotalParts,
		Truncated:     true,
	}
}

// ParseCacheKey extracts metadata from a raw cache key string.
// It recognizes both namespaced and non-namespaced keys.
func ParseCacheKey(raw string) (*CacheKey, error) {
	if raw == "" {
		return nil, errors.New("cache key must not be empty")
	}

	parts := strings.Split(raw, KeySeparator)
	if len(parts) < 5 {
		return nil, fmt.Errorf("invalid cache key format: expected at least 5 segments, got %d", len(parts))
	}

	key := &CacheKey{Raw: raw}

	// Detect the "model" keyword which always precedes the model identifier.
	// A key without namespace: prefix:version:model:<model>:<hash>[:<partInfo>]
	// A key with namespace:    <ns_type>:<ns_value>[...]:prefix:version:model:<model>:<hash>[:<partInfo>]
	modelIdx := -1
	for i, p := range parts {
		if p == "model" {
			modelIdx = i
			break
		}
	}

	if modelIdx == -1 {
		return nil, errors.New("invalid cache key format: missing 'model' segment")
	}

	if modelIdx < 2 {
		return nil, fmt.Errorf("invalid cache key format: 'model' at position %d, need at least prefix and version before it", modelIdx)
	}

	// Everything before (modelIdx-2) is namespace.
	nsEnd := modelIdx - 2
	if nsEnd > 0 {
		nsParts := parts[:nsEnd]
		ns, err := ParseNamespacePrefix(strings.Join(nsParts, KeySeparator))
		if err != nil {
			return nil, fmt.Errorf("invalid namespace in cache key: %w", err)
		}
		key.Namespace = ns
	}

	key.GatewayPrefix = parts[modelIdx-2]
	key.Version = parts[modelIdx-1]

	// After "model" keyword, next segment is model name.
	if modelIdx+1 >= len(parts) {
		return nil, errors.New("invalid cache key format: missing model identifier after 'model'")
	}
	key.Model = parts[modelIdx+1]

	// After model name, the next segment is the context hash.
	if modelIdx+2 >= len(parts) {
		return nil, errors.New("invalid cache key format: missing context hash")
	}
	key.ContextHash = parts[modelIdx+2]

	// Check for composite key part info.
	if modelIdx+3 < len(parts) {
		partInfo := parts[modelIdx+3]
		var partIdx, totalParts int
		if n, _ := fmt.Sscanf(partInfo, "part%d_%d", &partIdx, &totalParts); n == 2 {
			key.PartIndex = partIdx
			key.TotalParts = totalParts
		}
	}

	return key, nil
}

// String returns the raw key string.
func (k *CacheKey) String() string {
	return k.Raw
}

// IsComposite returns true if this is a composite (partial caching) key.
func (k *CacheKey) IsComposite() bool {
	return k.TotalParts > 0
}

// sanitizeKeySegment replaces characters that conflict with the key separator.
func sanitizeKeySegment(s string) string {
	return strings.ReplaceAll(s, KeySeparator, "_")
}

// computePartHash generates a unique hash for a specific part of a composite key.
func computePartHash(baseHash string, partIndex, totalParts int) string {
	h := sha256.New()
	h.Write([]byte(baseHash))
	h.Write([]byte(fmt.Sprintf(":part:%d:%d", partIndex, totalParts)))
	return hex.EncodeToString(h.Sum(nil))
}
