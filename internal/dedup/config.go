package dedup

// Config holds configuration for the deduplication system.
type Config struct {
	// Enabled controls whether deduplication is active.
	Enabled bool `json:"enabled"`
	// Strategy is the deduplication strategy to use (e.g., "exact").
	Strategy string `json:"strategy"`
	// CaseSensitive controls whether string comparisons are case-sensitive.
	// When false, "Hello" and "hello" are treated as duplicates.
	CaseSensitive bool `json:"case_sensitive"`
	// MinContentLength is the minimum string length to consider for deduplication.
	// Content shorter than this is always kept. Zero means no minimum.
	MinContentLength int `json:"min_content_length"`
	// ContentTypes restricts deduplication to specific content types.
	// An empty slice means all content types are eligible.
	ContentTypes []ContentType `json:"content_types,omitempty"`
}

// DefaultConfig returns the default deduplication configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:          true,
		Strategy:         "exact",
		CaseSensitive:    true,
		MinContentLength: 0,
		ContentTypes:     nil, // all types
	}
}

// Validate checks that the configuration is valid.
func (c Config) Validate() error {
	if c.MinContentLength < 0 {
		return ErrInvalidMinLength
	}
	switch c.Strategy {
	case "exact", "":
		// valid
	default:
		return ErrUnknownStrategy
	}
	return nil
}
