package context

import (
	"fmt"
	"strconv"
	"strings"
)

// CacheKeyVersion represents a parsed cache key version string (e.g., "v1", "v2").
type CacheKeyVersion struct {
	// Major is the major version number.
	Major int `json:"major"`
	// Raw is the original version string.
	Raw string `json:"raw"`
}

// ParseCacheKeyVersion parses a version string like "v1" into a CacheKeyVersion.
func ParseCacheKeyVersion(raw string) (*CacheKeyVersion, error) {
	if raw == "" {
		return nil, fmt.Errorf("version string must not be empty")
	}
	if !strings.HasPrefix(raw, "v") {
		return nil, fmt.Errorf("version string %q must start with 'v'", raw)
	}

	numStr := raw[1:]
	major, err := strconv.Atoi(numStr)
	if err != nil {
		return nil, fmt.Errorf("invalid version number in %q: %w", raw, err)
	}
	if major < 1 {
		return nil, fmt.Errorf("version number must be >= 1, got %d", major)
	}

	return &CacheKeyVersion{
		Major: major,
		Raw:   raw,
	}, nil
}

// String returns the version string (e.g., "v1").
func (v *CacheKeyVersion) String() string {
	return v.Raw
}

// Next returns the next version (e.g., v1 -> v2).
func (v *CacheKeyVersion) Next() *CacheKeyVersion {
	next := v.Major + 1
	return &CacheKeyVersion{
		Major: next,
		Raw:   fmt.Sprintf("v%d", next),
	}
}

// IsCompatible checks whether the given version is compatible with this version.
// Versions are only compatible if they have the same major number.
func (v *CacheKeyVersion) IsCompatible(other *CacheKeyVersion) bool {
	if v == nil || other == nil {
		return false
	}
	return v.Major == other.Major
}

// VersionedKeyMigrator helps migrate cache keys from one version to another.
// It provides utilities to identify stale keys and generate replacement keys.
type VersionedKeyMigrator struct {
	builder *CacheKeyBuilder
	fromVer *CacheKeyVersion
	toVer   *CacheKeyVersion
}

// NewVersionedKeyMigrator creates a migrator that translates keys from
// fromVersion to toVersion. The builder's cache version is set to toVersion.
func NewVersionedKeyMigrator(fromVersion, toVersion string, opts ...CacheKeyBuilderOption) (*VersionedKeyMigrator, error) {
	from, err := ParseCacheKeyVersion(fromVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid from version: %w", err)
	}
	to, err := ParseCacheKeyVersion(toVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid to version: %w", err)
	}

	// Ensure we're actually upgrading.
	if to.Major <= from.Major {
		return nil, fmt.Errorf("to version %s must be greater than from version %s", toVersion, fromVersion)
	}

	allOpts := append([]CacheKeyBuilderOption{WithCacheVersion(toVersion)}, opts...)
	builder := NewCacheKeyBuilder(allOpts...)

	return &VersionedKeyMigrator{
		builder: builder,
		fromVer: from,
		toVer:   to,
	}, nil
}

// NeedsMigration returns true if the given key uses the old version.
func (m *VersionedKeyMigrator) NeedsMigration(key *CacheKey) bool {
	parsed, err := ParseCacheKeyVersion(key.Version)
	if err != nil {
		return false
	}
	return parsed.Major == m.fromVer.Major
}

// Migrate generates a new key in the target version from the metadata
// of the old key. The context hash and model are preserved.
func (m *VersionedKeyMigrator) Migrate(oldKey *CacheKey) (*CacheKey, error) {
	if oldKey == nil {
		return nil, fmt.Errorf("old key must not be nil")
	}

	if oldKey.Namespace != nil {
		m.builder.namespace = oldKey.Namespace
	}

	if oldKey.IsComposite() {
		keys, err := m.builder.BuildComposite(oldKey.Model, oldKey.ContextHash, oldKey.TotalParts)
		if err != nil {
			return nil, fmt.Errorf("migrating composite key: %w", err)
		}
		if oldKey.PartIndex > 0 && oldKey.PartIndex <= len(keys) {
			return keys[oldKey.PartIndex-1], nil
		}
		return nil, fmt.Errorf("part index %d out of range for %d parts", oldKey.PartIndex, oldKey.TotalParts)
	}

	return m.builder.Build(oldKey.Model, oldKey.ContextHash)
}

// FromVersion returns the source version.
func (m *VersionedKeyMigrator) FromVersion() *CacheKeyVersion {
	return m.fromVer
}

// ToVersion returns the target version.
func (m *VersionedKeyMigrator) ToVersion() *CacheKeyVersion {
	return m.toVer
}

// IsVersionStale checks whether a key's version is older than the current version.
// This is useful for determining if cached data should be refreshed.
func IsVersionStale(key *CacheKey, currentVersion string) (bool, error) {
	if key == nil {
		return false, fmt.Errorf("key must not be nil")
	}
	current, err := ParseCacheKeyVersion(currentVersion)
	if err != nil {
		return false, fmt.Errorf("invalid current version: %w", err)
	}
	keyVer, err := ParseCacheKeyVersion(key.Version)
	if err != nil {
		return false, fmt.Errorf("invalid key version: %w", err)
	}
	return keyVer.Major < current.Major, nil
}

// KeyVersionFromRaw extracts just the version component from a raw key string
// without doing a full parse.
func KeyVersionFromRaw(raw string) (string, error) {
	parts := strings.Split(raw, KeySeparator)

	// Find "model" marker and version is two positions before it.
	for i, p := range parts {
		if p == "model" && i >= 2 {
			return parts[i-1], nil
		}
	}
	return "", fmt.Errorf("cannot extract version from key %q", raw)
}
