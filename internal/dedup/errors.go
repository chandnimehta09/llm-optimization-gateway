package dedup

import "errors"

var (
	// ErrNilContent is returned when a nil Content is passed to Deduplicate.
	ErrNilContent = errors.New("dedup: content must not be nil")
	// ErrInvalidMinLength is returned when MinContentLength is negative.
	ErrInvalidMinLength = errors.New("dedup: min_content_length must be non-negative")
	// ErrUnknownStrategy is returned when an unsupported strategy is specified.
	ErrUnknownStrategy = errors.New("dedup: unknown deduplication strategy")
)
