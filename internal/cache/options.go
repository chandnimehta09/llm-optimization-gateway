package cache

import "time"

// Priority represents the cache entry priority level. Higher-priority
// entries are less likely to be evicted when the cache is full.
type Priority int

const (
	// PriorityLow indicates a low-priority entry that can be evicted first.
	PriorityLow Priority = iota
	// PriorityNormal is the default priority for cache entries.
	PriorityNormal
	// PriorityHigh indicates a high-priority entry that should be retained longer.
	PriorityHigh
)

// String returns a human-readable representation of the priority.
func (p Priority) String() string {
	switch p {
	case PriorityLow:
		return "low"
	case PriorityNormal:
		return "normal"
	case PriorityHigh:
		return "high"
	default:
		return "unknown"
	}
}

// SetOptions holds the resolved options for a cache Set operation.
type SetOptions struct {
	// TTL overrides the default TTL for this entry. Zero means use the default.
	TTL time.Duration
	// Tags are labels for group invalidation.
	Tags []string
	// Priority sets the eviction priority for this entry.
	Priority Priority
}

// DefaultSetOptions returns SetOptions with sensible defaults.
func DefaultSetOptions() SetOptions {
	return SetOptions{
		Priority: PriorityNormal,
	}
}

// Option is a functional option that modifies SetOptions for a cache Set call.
type Option func(*SetOptions)

// WithTTL sets a custom TTL for the cache entry.
func WithTTL(ttl time.Duration) Option {
	return func(o *SetOptions) {
		o.TTL = ttl
	}
}

// WithTags sets tags on the cache entry for group invalidation.
func WithTags(tags ...string) Option {
	return func(o *SetOptions) {
		o.Tags = append(o.Tags, tags...)
	}
}

// WithPriority sets the eviction priority for the cache entry.
func WithPriority(p Priority) Option {
	return func(o *SetOptions) {
		o.Priority = p
	}
}

// ResolveOptions applies functional options to the default SetOptions and
// returns the resolved configuration.
func ResolveOptions(opts ...Option) SetOptions {
	o := DefaultSetOptions()
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
