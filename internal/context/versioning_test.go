package context

import (
	"testing"
)

func TestParseCacheKeyVersion(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		major   int
		wantErr bool
	}{
		{name: "v1", raw: "v1", major: 1},
		{name: "v2", raw: "v2", major: 2},
		{name: "v10", raw: "v10", major: 10},
		{name: "v99", raw: "v99", major: 99},
		{name: "empty string", raw: "", wantErr: true},
		{name: "no v prefix", raw: "1", wantErr: true},
		{name: "invalid number", raw: "vabc", wantErr: true},
		{name: "v0", raw: "v0", wantErr: true},
		{name: "negative", raw: "v-1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := ParseCacheKeyVersion(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCacheKeyVersion(%q) error = %v, wantErr = %v", tt.raw, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if v.Major != tt.major {
				t.Errorf("major: expected %d, got %d", tt.major, v.Major)
			}
			if v.Raw != tt.raw {
				t.Errorf("raw: expected %q, got %q", tt.raw, v.Raw)
			}
		})
	}
}

func TestCacheKeyVersion_String(t *testing.T) {
	v := &CacheKeyVersion{Major: 3, Raw: "v3"}
	if v.String() != "v3" {
		t.Errorf("expected %q, got %q", "v3", v.String())
	}
}

func TestCacheKeyVersion_Next(t *testing.T) {
	v := &CacheKeyVersion{Major: 1, Raw: "v1"}
	next := v.Next()
	if next.Major != 2 {
		t.Errorf("expected major 2, got %d", next.Major)
	}
	if next.Raw != "v2" {
		t.Errorf("expected raw %q, got %q", "v2", next.Raw)
	}
}

func TestCacheKeyVersion_IsCompatible(t *testing.T) {
	v1 := &CacheKeyVersion{Major: 1, Raw: "v1"}
	v1b := &CacheKeyVersion{Major: 1, Raw: "v1"}
	v2 := &CacheKeyVersion{Major: 2, Raw: "v2"}

	if !v1.IsCompatible(v1b) {
		t.Error("same major versions should be compatible")
	}
	if v1.IsCompatible(v2) {
		t.Error("different major versions should not be compatible")
	}
	if v1.IsCompatible(nil) {
		t.Error("nil should not be compatible")
	}
	if (*CacheKeyVersion)(nil).IsCompatible(v1) {
		t.Error("nil should not be compatible")
	}
}

func TestNewVersionedKeyMigrator(t *testing.T) {
	m, err := NewVersionedKeyMigrator("v1", "v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.FromVersion().Major != 1 {
		t.Errorf("expected from version 1, got %d", m.FromVersion().Major)
	}
	if m.ToVersion().Major != 2 {
		t.Errorf("expected to version 2, got %d", m.ToVersion().Major)
	}
}

func TestNewVersionedKeyMigrator_InvalidVersions(t *testing.T) {
	_, err := NewVersionedKeyMigrator("invalid", "v2")
	if err == nil {
		t.Fatal("expected error for invalid from version")
	}

	_, err = NewVersionedKeyMigrator("v1", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid to version")
	}

	_, err = NewVersionedKeyMigrator("v2", "v1")
	if err == nil {
		t.Fatal("expected error for downgrade")
	}

	_, err = NewVersionedKeyMigrator("v1", "v1")
	if err == nil {
		t.Fatal("expected error for same version")
	}
}

func TestVersionedKeyMigrator_NeedsMigration(t *testing.T) {
	m, err := NewVersionedKeyMigrator("v1", "v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	oldKey := &CacheKey{Version: "v1"}
	newKey := &CacheKey{Version: "v2"}
	otherKey := &CacheKey{Version: "v3"}

	if !m.NeedsMigration(oldKey) {
		t.Error("v1 key should need migration")
	}
	if m.NeedsMigration(newKey) {
		t.Error("v2 key should not need migration")
	}
	if m.NeedsMigration(otherKey) {
		t.Error("v3 key should not need migration")
	}
}

func TestVersionedKeyMigrator_Migrate(t *testing.T) {
	m, err := NewVersionedKeyMigrator("v1", "v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	oldKey := &CacheKey{
		Raw:           "gw:v1:model:gpt-4:abc123",
		GatewayPrefix: "gw",
		Version:       "v1",
		Model:         "gpt-4",
		ContextHash:   "abc123",
	}

	newKey, err := m.Migrate(oldKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newKey.Version != "v2" {
		t.Errorf("expected version %q, got %q", "v2", newKey.Version)
	}
	if newKey.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", newKey.Model)
	}
	if newKey.ContextHash != "abc123" {
		t.Errorf("expected hash %q, got %q", "abc123", newKey.ContextHash)
	}

	expectedRaw := "gw:v2:model:gpt-4:abc123"
	if newKey.Raw != expectedRaw {
		t.Errorf("expected raw %q, got %q", expectedRaw, newKey.Raw)
	}
}

func TestVersionedKeyMigrator_MigrateWithNamespace(t *testing.T) {
	m, err := NewVersionedKeyMigrator("v1", "v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	oldKey := &CacheKey{
		Raw:           "tenant:acme:gw:v1:model:gpt-4:abc123",
		GatewayPrefix: "gw",
		Version:       "v1",
		Model:         "gpt-4",
		ContextHash:   "abc123",
		Namespace:     NewNamespace(WithTenant("acme")),
	}

	newKey, err := m.Migrate(oldKey)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if newKey.Version != "v2" {
		t.Errorf("expected version %q, got %q", "v2", newKey.Version)
	}
	if newKey.Namespace == nil {
		t.Fatal("expected namespace to be preserved")
	}
	if newKey.Namespace.Tenant != "acme" {
		t.Errorf("expected tenant %q, got %q", "acme", newKey.Namespace.Tenant)
	}
}

func TestVersionedKeyMigrator_MigrateNilKeyError(t *testing.T) {
	m, err := NewVersionedKeyMigrator("v1", "v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.Migrate(nil)
	if err == nil {
		t.Fatal("expected error for nil key")
	}
}

func TestIsVersionStale(t *testing.T) {
	key := &CacheKey{Version: "v1"}

	stale, err := IsVersionStale(key, "v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !stale {
		t.Error("v1 should be stale when current is v2")
	}

	stale, err = IsVersionStale(key, "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stale {
		t.Error("v1 should not be stale when current is v1")
	}
}

func TestIsVersionStale_Errors(t *testing.T) {
	_, err := IsVersionStale(nil, "v1")
	if err == nil {
		t.Fatal("expected error for nil key")
	}

	key := &CacheKey{Version: "v1"}
	_, err = IsVersionStale(key, "bad")
	if err == nil {
		t.Fatal("expected error for invalid current version")
	}

	badKey := &CacheKey{Version: "bad"}
	_, err = IsVersionStale(badKey, "v1")
	if err == nil {
		t.Fatal("expected error for invalid key version")
	}
}

func TestKeyVersionFromRaw(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "basic key", raw: "gw:v1:model:gpt-4:hash", want: "v1"},
		{name: "v2 key", raw: "gw:v2:model:gpt-4:hash", want: "v2"},
		{name: "namespaced key", raw: "tenant:acme:gw:v1:model:gpt-4:hash", want: "v1"},
		{name: "no model marker", raw: "gw:v1:gpt-4:hash", wantErr: true},
		{name: "model at start", raw: "model:gpt-4:hash", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := KeyVersionFromRaw(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("KeyVersionFromRaw(%q) error = %v, wantErr = %v", tt.raw, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
