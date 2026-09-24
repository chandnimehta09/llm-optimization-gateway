package cache

import (
	"errors"
	"fmt"
)

// CacheMissError is returned when a key is not found in the cache.
type CacheMissError struct {
	// Key is the cache key that was not found.
	Key string
}

// Error implements the error interface.
func (e *CacheMissError) Error() string {
	return fmt.Sprintf("cache miss: key %q not found", e.Key)
}

// CacheExpiredError is returned when a cache entry exists but has expired.
type CacheExpiredError struct {
	// Key is the cache key that has expired.
	Key string
	// ExpiredAt is the time at which the entry expired.
	ExpiredAt string
}

// Error implements the error interface.
func (e *CacheExpiredError) Error() string {
	return fmt.Sprintf("cache expired: key %q expired at %s", e.Key, e.ExpiredAt)
}

// CacheCorruptedError is returned when a cache entry exists but cannot be
// deserialized or is otherwise invalid.
type CacheCorruptedError struct {
	// Key is the cache key with corrupted data.
	Key string
	// Reason describes why the entry is considered corrupted.
	Reason string
}

// Error implements the error interface.
func (e *CacheCorruptedError) Error() string {
	return fmt.Sprintf("cache corrupted: key %q: %s", e.Key, e.Reason)
}

// IsCacheMiss reports whether the error is a CacheMissError.
func IsCacheMiss(err error) bool {
	_, ok := err.(*CacheMissError)
	return ok
}

// IsCacheExpired reports whether the error is a CacheExpiredError.
func IsCacheExpired(err error) bool {
	_, ok := err.(*CacheExpiredError)
	return ok
}

// IsCacheCorrupted reports whether the error is a CacheCorruptedError.
func IsCacheCorrupted(err error) bool {
	_, ok := err.(*CacheCorruptedError)
	return ok
}

type CacheFailureError struct {
	Operation string
	Cause     error
}

func (e *CacheFailureError) Error() string {
	return fmt.Sprintf("cache failure during %s: %v", e.Operation, e.Cause)
}

func (e *CacheFailureError) Unwrap() error { return e.Cause }

func IsCacheFailure(err error) bool {
	var cfe *CacheFailureError
	return errors.As(err, &cfe)
}

func ClassifyCacheError(err error) (reason BypassReason, transient bool) {
	if err == nil {
		return "", false
	}
	if IsCacheMiss(err) || IsCacheExpired(err) {
		return "", false
	}
	if IsCacheCorrupted(err) {
		return BypassCacheError, false
	}
	if IsCacheFailure(err) {
		return BypassCacheError, true
	}
	return BypassCacheError, true
}
