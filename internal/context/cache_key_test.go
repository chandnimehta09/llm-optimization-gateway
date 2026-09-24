package context

import (
	"fmt"
	"strings"
	"testing"
)

func TestCacheKeyBuild_BasicFormat(t *testing.T) {
	builder := NewCacheKeyBuilder()
	hash := "abc123def456"

	key, err := builder.Build("gpt-4", hash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "gw:v1:model:gpt-4:abc123def456"
	if key.Raw != expected {
		t.Errorf("expected key %q, got %q", expected, key.Raw)
	}
	if key.GatewayPrefix != DefaultGatewayPrefix {
		t.Errorf("expected prefix %q, got %q", DefaultGatewayPrefix, key.GatewayPrefix)
	}
	if key.Version != DefaultCacheVersion {
		t.Errorf("expected version %q, got %q", DefaultCacheVersion, key.Version)
	}
	if key.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", key.Model)
	}
	if key.ContextHash != hash {
		t.Errorf("expected hash %q, got %q", hash, key.ContextHash)
	}
	if key.Truncated {
		t.Error("expected key not to be truncated")
	}
	if key.IsComposite() {
		t.Error("expected key not to be composite")
	}
}

func TestCacheKeyBuild_CustomPrefix(t *testing.T) {
	builder := NewCacheKeyBuilder(WithGatewayPrefix("mygateway"))
	key, err := builder.Build("gpt-4", "hash123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "mygateway:v1:model:gpt-4:hash123"
	if key.Raw != expected {
		t.Errorf("expected key %q, got %q", expected, key.Raw)
	}
}

func TestCacheKeyBuild_CustomVersion(t *testing.T) {
	builder := NewCacheKeyBuilder(WithCacheVersion("v2"))
	key, err := builder.Build("gpt-4", "hash123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "gw:v2:model:gpt-4:hash123"
	if key.Raw != expected {
		t.Errorf("expected key %q, got %q", expected, key.Raw)
	}
}

func TestCacheKeyBuild_EmptyModelError(t *testing.T) {
	builder := NewCacheKeyBuilder()
	_, err := builder.Build("", "hash123")
	if err == nil {
		t.Fatal("expected error for empty model")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Errorf("expected error about model, got %v", err)
	}
}

func TestCacheKeyBuild_EmptyHashError(t *testing.T) {
	builder := NewCacheKeyBuilder()
	_, err := builder.Build("gpt-4", "")
	if err == nil {
		t.Fatal("expected error for empty hash")
	}
	if !strings.Contains(err.Error(), "hash") {
		t.Errorf("expected error about hash, got %v", err)
	}
}

func TestCacheKeyBuild_ModelWithColons(t *testing.T) {
	builder := NewCacheKeyBuilder()
	key, err := builder.Build("anthropic:claude-3", "hash123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Colon in model name should be sanitized.
	if strings.Count(key.Raw, "model") != 1 {
		t.Errorf("expected exactly one 'model' marker in key %q", key.Raw)
	}
	if key.Model != "anthropic_claude-3" {
		t.Errorf("expected sanitized model %q, got %q", "anthropic_claude-3", key.Model)
	}
}

func TestCacheKeyBuild_WithNamespace(t *testing.T) {
	ns := NewNamespace(WithTenant("acme"))
	builder := NewCacheKeyBuilder(WithNamespace(ns))
	key, err := builder.Build("gpt-4", "hash123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "tenant:acme:gw:v1:model:gpt-4:hash123"
	if key.Raw != expected {
		t.Errorf("expected key %q, got %q", expected, key.Raw)
	}
	if key.Namespace == nil {
		t.Fatal("expected namespace to be set")
	}
	if key.Namespace.Tenant != "acme" {
		t.Errorf("expected tenant %q, got %q", "acme", key.Namespace.Tenant)
	}
}

func TestCacheKeyBuild_WithMultiDimensionNamespace(t *testing.T) {
	ns := NewNamespace(WithTenant("acme"), WithEnvironment("prod"), WithApplication("api"))
	builder := NewCacheKeyBuilder(WithNamespace(ns))
	key, err := builder.Build("gpt-4", "hash123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "tenant:acme:env:prod:app:api:gw:v1:model:gpt-4:hash123"
	if key.Raw != expected {
		t.Errorf("expected key %q, got %q", expected, key.Raw)
	}
}

func TestCacheKeyBuild_FromIdentity(t *testing.T) {
	identity := &ContextIdentity{
		Model: ModelIdentity{
			Name:     "gpt-4",
			Provider: "openai",
			Version:  "2024-01",
		},
		Prompt: PromptIdentity{
			TemplateHash:    "tmplhash",
			TemplateVersion: "1.0",
		},
	}

	builder := NewCacheKeyBuilder()
	key, err := builder.BuildFromIdentity(identity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if key.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", key.Model)
	}
	if key.ContextHash == "" {
		t.Error("expected non-empty context hash")
	}
	// Verify the hash matches the identity's computed ID.
	expectedHash := identity.ComputeID()
	if key.ContextHash != expectedHash {
		t.Errorf("expected hash %q, got %q", expectedHash, key.ContextHash)
	}
}

func TestCacheKeyBuild_FromNilIdentityError(t *testing.T) {
	builder := NewCacheKeyBuilder()
	_, err := builder.BuildFromIdentity(nil)
	if err == nil {
		t.Fatal("expected error for nil identity")
	}
}

func TestCacheKeyBuild_Truncation(t *testing.T) {
	builder := NewCacheKeyBuilder(WithMaxKeyLength(40))
	longHash := strings.Repeat("a", 200)

	key, err := builder.Build("gpt-4", longHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !key.Truncated {
		t.Error("expected key to be truncated")
	}
	if len(key.Raw) > 40 {
		t.Errorf("expected key length <= 40, got %d: %q", len(key.Raw), key.Raw)
	}
}

func TestCacheKeyBuild_TruncationCollisionResistance(t *testing.T) {
	builder := NewCacheKeyBuilder(WithMaxKeyLength(60))

	// Two different long hashes should produce different truncated keys.
	hash1 := strings.Repeat("a", 200)
	hash2 := strings.Repeat("b", 200)

	key1, err := builder.Build("gpt-4", hash1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	key2, err := builder.Build("gpt-4", hash2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if key1.Raw == key2.Raw {
		t.Errorf("truncated keys should differ for different hashes: %q", key1.Raw)
	}
}

func TestCacheKeyBuild_CompositeKeys(t *testing.T) {
	builder := NewCacheKeyBuilder()
	keys, err := builder.BuildComposite("gpt-4", "hash123", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	for i, key := range keys {
		if !key.IsComposite() {
			t.Errorf("key %d should be composite", i)
		}
		if key.PartIndex != i+1 {
			t.Errorf("key %d: expected part index %d, got %d", i, i+1, key.PartIndex)
		}
		if key.TotalParts != 3 {
			t.Errorf("key %d: expected total parts 3, got %d", i, key.TotalParts)
		}
		if !strings.Contains(key.Raw, fmt.Sprintf("part%d_3", i+1)) {
			t.Errorf("key %d: expected part info in key %q", i, key.Raw)
		}
	}

	// All keys should be unique.
	seen := make(map[string]bool)
	for _, key := range keys {
		if seen[key.Raw] {
			t.Errorf("duplicate composite key: %q", key.Raw)
		}
		seen[key.Raw] = true
	}
}

func TestCacheKeyBuild_CompositeInvalidParts(t *testing.T) {
	builder := NewCacheKeyBuilder()

	_, err := builder.BuildComposite("gpt-4", "hash", 0)
	if err == nil {
		t.Fatal("expected error for zero parts")
	}

	_, err = builder.BuildComposite("gpt-4", "hash", -1)
	if err == nil {
		t.Fatal("expected error for negative parts")
	}

	_, err = builder.BuildComposite("", "hash", 3)
	if err == nil {
		t.Fatal("expected error for empty model")
	}

	_, err = builder.BuildComposite("gpt-4", "", 3)
	if err == nil {
		t.Fatal("expected error for empty hash")
	}
}

func TestParseCacheKey_Basic(t *testing.T) {
	raw := "gw:v1:model:gpt-4:abc123"
	key, err := ParseCacheKey(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if key.GatewayPrefix != "gw" {
		t.Errorf("expected prefix %q, got %q", "gw", key.GatewayPrefix)
	}
	if key.Version != "v1" {
		t.Errorf("expected version %q, got %q", "v1", key.Version)
	}
	if key.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", key.Model)
	}
	if key.ContextHash != "abc123" {
		t.Errorf("expected hash %q, got %q", "abc123", key.ContextHash)
	}
	if key.Namespace != nil {
		t.Error("expected no namespace")
	}
	if key.IsComposite() {
		t.Error("expected non-composite key")
	}
}

func TestParseCacheKey_WithNamespace(t *testing.T) {
	raw := "tenant:acme:gw:v1:model:gpt-4:abc123"
	key, err := ParseCacheKey(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if key.Namespace == nil {
		t.Fatal("expected namespace to be set")
	}
	if key.Namespace.Tenant != "acme" {
		t.Errorf("expected tenant %q, got %q", "acme", key.Namespace.Tenant)
	}
	if key.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", key.Model)
	}
	if key.ContextHash != "abc123" {
		t.Errorf("expected hash %q, got %q", "abc123", key.ContextHash)
	}
}

func TestParseCacheKey_WithMultiDimensionNamespace(t *testing.T) {
	raw := "tenant:acme:env:prod:app:api:gw:v1:model:gpt-4:abc123"
	key, err := ParseCacheKey(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if key.Namespace == nil {
		t.Fatal("expected namespace to be set")
	}
	if key.Namespace.Tenant != "acme" {
		t.Errorf("expected tenant %q, got %q", "acme", key.Namespace.Tenant)
	}
	if key.Namespace.Environment != "prod" {
		t.Errorf("expected environment %q, got %q", "prod", key.Namespace.Environment)
	}
	if key.Namespace.Application != "api" {
		t.Errorf("expected application %q, got %q", "api", key.Namespace.Application)
	}
	if key.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", key.Model)
	}
}

func TestParseCacheKey_CompositeKey(t *testing.T) {
	raw := "gw:v1:model:gpt-4:abc123:part2_5"
	key, err := ParseCacheKey(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !key.IsComposite() {
		t.Error("expected composite key")
	}
	if key.PartIndex != 2 {
		t.Errorf("expected part index 2, got %d", key.PartIndex)
	}
	if key.TotalParts != 5 {
		t.Errorf("expected total parts 5, got %d", key.TotalParts)
	}
}

func TestParseCacheKey_EmptyKeyError(t *testing.T) {
	_, err := ParseCacheKey("")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestParseCacheKey_TooFewSegmentsError(t *testing.T) {
	_, err := ParseCacheKey("gw:v1:model")
	if err == nil {
		t.Fatal("expected error for too few segments")
	}
}

func TestParseCacheKey_MissingModelError(t *testing.T) {
	_, err := ParseCacheKey("gw:v1:something:gpt-4:hash")
	if err == nil {
		t.Fatal("expected error for missing model keyword")
	}
}

func TestCacheKeyBuild_RoundTrip(t *testing.T) {
	// Build a key, parse it, verify fields match.
	ns := NewNamespace(WithTenant("acme"), WithEnvironment("prod"))
	builder := NewCacheKeyBuilder(
		WithGatewayPrefix("mygateway"),
		WithCacheVersion("v2"),
		WithNamespace(ns),
	)

	key, err := builder.Build("claude-3-opus", "deadbeef1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parsed, err := ParseCacheKey(key.Raw)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if parsed.GatewayPrefix != key.GatewayPrefix {
		t.Errorf("prefix mismatch: %q vs %q", parsed.GatewayPrefix, key.GatewayPrefix)
	}
	if parsed.Version != key.Version {
		t.Errorf("version mismatch: %q vs %q", parsed.Version, key.Version)
	}
	if parsed.Model != key.Model {
		t.Errorf("model mismatch: %q vs %q", parsed.Model, key.Model)
	}
	if parsed.ContextHash != key.ContextHash {
		t.Errorf("hash mismatch: %q vs %q", parsed.ContextHash, key.ContextHash)
	}
	if parsed.Namespace == nil {
		t.Fatal("expected namespace to be parsed")
	}
	if parsed.Namespace.Tenant != "acme" {
		t.Errorf("expected tenant %q, got %q", "acme", parsed.Namespace.Tenant)
	}
	if parsed.Namespace.Environment != "prod" {
		t.Errorf("expected environment %q, got %q", "prod", parsed.Namespace.Environment)
	}
}

func TestCacheKeyString(t *testing.T) {
	key := &CacheKey{Raw: "gw:v1:model:gpt-4:hash"}
	if key.String() != "gw:v1:model:gpt-4:hash" {
		t.Errorf("expected String() to return Raw, got %q", key.String())
	}
}

func TestCacheKeyBuild_MaxKeyLengthRespected(t *testing.T) {
	// Ensure that even with namespace + long hash, the key stays under the limit.
	ns := NewNamespace(WithTenant("very-long-tenant-name"), WithEnvironment("prod"), WithApplication("my-application"))
	builder := NewCacheKeyBuilder(
		WithNamespace(ns),
		WithMaxKeyLength(80),
	)

	longHash := strings.Repeat("f", 100)
	key, err := builder.Build("gpt-4-turbo", longHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(key.Raw) > 80 {
		t.Errorf("key length %d exceeds max 80: %q", len(key.Raw), key.Raw)
	}
	if !key.Truncated {
		t.Error("expected key to be truncated")
	}
}

func TestCacheKeyBuild_IntegrationWithCacheInterface(t *testing.T) {
	// Simulate building cache keys from context identities and looking them up.
	identity := &ContextIdentity{
		Model: ModelIdentity{
			Name:     "gpt-4",
			Provider: "openai",
			Version:  "2024-01",
		},
		Prompt: PromptIdentity{
			TemplateHash:     "abc",
			TemplateVersion:  "1.0",
			SystemPromptHash: "def",
		},
	}

	// Build key.
	ns := NewNamespace(WithTenant("acme"), WithEnvironment("prod"))
	builder := NewCacheKeyBuilder(WithNamespace(ns))
	key, err := builder.BuildFromIdentity(identity)
	if err != nil {
		t.Fatalf("build error: %v", err)
	}

	// Simulate cache store (using a map).
	cache := make(map[string]string)
	cache[key.Raw] = "cached-response-data"

	// Parse the key back and verify we can retrieve the cached value.
	parsed, err := ParseCacheKey(key.Raw)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if val, ok := cache[parsed.Raw]; !ok {
		t.Error("expected to find cached value")
	} else if val != "cached-response-data" {
		t.Errorf("expected cached value %q, got %q", "cached-response-data", val)
	}

	// Build the same key from the same identity - should match.
	key2, err := builder.BuildFromIdentity(identity)
	if err != nil {
		t.Fatalf("second build error: %v", err)
	}
	if key2.Raw != key.Raw {
		t.Errorf("same identity should produce same key: %q vs %q", key.Raw, key2.Raw)
	}

	// Modify identity - key should change.
	identity.Prompt.TemplateHash = "xyz"
	key3, err := builder.BuildFromIdentity(identity)
	if err != nil {
		t.Fatalf("third build error: %v", err)
	}
	if key3.Raw == key.Raw {
		t.Error("different identity should produce different key")
	}

	// The old key should not match the new one.
	if _, ok := cache[key3.Raw]; ok {
		t.Error("should not find cached value for changed identity")
	}
}

func TestCacheKeyBuild_DifferentModels(t *testing.T) {
	builder := NewCacheKeyBuilder()
	hash := "samehash"

	key1, _ := builder.Build("gpt-4", hash)
	key2, _ := builder.Build("claude-3-opus", hash)

	if key1.Raw == key2.Raw {
		t.Error("different models should produce different keys")
	}
}

func TestCacheKeyBuild_DifferentNamespacesDifferentKeys(t *testing.T) {
	hash := "samehash"

	b1 := NewCacheKeyBuilder(WithNamespace(NewNamespace(WithTenant("acme"))))
	b2 := NewCacheKeyBuilder(WithNamespace(NewNamespace(WithTenant("globex"))))
	b3 := NewCacheKeyBuilder()

	key1, _ := b1.Build("gpt-4", hash)
	key2, _ := b2.Build("gpt-4", hash)
	key3, _ := b3.Build("gpt-4", hash)

	if key1.Raw == key2.Raw {
		t.Error("different tenants should produce different keys")
	}
	if key1.Raw == key3.Raw {
		t.Error("namespaced and non-namespaced keys should differ")
	}
}
