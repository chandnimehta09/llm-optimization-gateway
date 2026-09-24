package context

import (
	"encoding/json"
	"errors"
	"math"
	"math/rand"
	"testing"
)

// --- Normalize Tests ---

func TestNormalizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no change needed",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "trim leading/trailing spaces",
			input:    "  hello world  ",
			expected: "hello world",
		},
		{
			name:     "collapse multiple spaces",
			input:    "hello    world",
			expected: "hello world",
		},
		{
			name:     "trim and collapse",
			input:    "  hello    world  ",
			expected: "hello world",
		},
		{
			name:     "tabs and newlines",
			input:    "hello\t\tworld\n",
			expected: "hello world",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only whitespace",
			input:    "   \t\n  ",
			expected: "",
		},
		{
			name:     "unicode NFC normalization - combining characters",
			input:    "e\u0301", // é as e + combining acute accent
			expected: "\u00e9",  // é as single precomposed character
		},
		{
			name:     "unicode already NFC",
			input:    "\u00e9", // é precomposed
			expected: "\u00e9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeString(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeString(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCollapseSpaces(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"a  b", "a b"},
		{"a   b   c", "a b c"},
		{"a b", "a b"},
		{"a\t\tb", "a b"},
		{"a\n\nb", "a b"},
		{"", ""},
	}

	for _, tt := range tests {
		got := collapseSpaces(tt.input)
		if got != tt.expected {
			t.Errorf("collapseSpaces(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizeFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected string
	}{
		{"positive integer", 42.0, "42"},
		{"positive decimal", 3.14, "3.14"},
		{"zero", 0.0, "0"},
		{"negative zero", math.Copysign(0, -1), "0"},
		{"negative value", -1.5, "-1.5"},
		{"very small", 0.001, "0.001"},
		{"NaN", math.NaN(), "NaN"},
		{"positive infinity", math.Inf(1), "+Inf"},
		{"negative infinity", math.Inf(-1), "-Inf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeFloat(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeFloat(%v) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNormalizeValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{"nil", nil, nil},
		{"string with spaces", "  hello  world  ", "hello world"},
		{"bool true", true, true},
		{"bool false", false, false},
		{"float", 3.14, 3.14},
		{"negative zero float", math.Copysign(0, -1), float64(0)},
		{"integer-valued float", 42.0, 42.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeValue(tt.input)
			// Special handling for float comparison.
			if gf, ok := got.(float64); ok {
				ef, eok := tt.expected.(float64)
				if !eok {
					t.Errorf("normalizeValue(%v) = %v (float64), want %v", tt.input, got, tt.expected)
					return
				}
				if math.Float64bits(gf) != math.Float64bits(ef) {
					t.Errorf("normalizeValue(%v) = %v, want %v", tt.input, got, tt.expected)
				}
				return
			}
			if got != tt.expected {
				t.Errorf("normalizeValue(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// --- Sort Tests ---

func TestSortMapKeys(t *testing.T) {
	m := map[string]interface{}{
		"zebra":  1,
		"apple":  2,
		"mango":  3,
		"banana": 4,
		"cherry": 5,
	}

	keys := sortMapKeys(m)
	expected := []string{"apple", "banana", "cherry", "mango", "zebra"}

	if len(keys) != len(expected) {
		t.Fatalf("sortMapKeys returned %d keys, want %d", len(keys), len(expected))
	}
	for i, k := range keys {
		if k != expected[i] {
			t.Errorf("sortMapKeys[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

func TestSortStringSlice(t *testing.T) {
	input := []string{"banana", "apple", "cherry"}
	sorted := sortStringSlice(input)

	// Verify original is not modified.
	if input[0] != "banana" {
		t.Error("sortStringSlice modified the original slice")
	}

	expected := []string{"apple", "banana", "cherry"}
	for i, s := range sorted {
		if s != expected[i] {
			t.Errorf("sortStringSlice[%d] = %q, want %q", i, s, expected[i])
		}
	}
}

func TestSortSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    []interface{}
		expected []interface{}
	}{
		{
			name:     "strings",
			input:    []interface{}{"banana", "apple", "cherry"},
			expected: []interface{}{"apple", "banana", "cherry"},
		},
		{
			name:     "numbers",
			input:    []interface{}{3.0, 1.0, 2.0},
			expected: []interface{}{1.0, 2.0, 3.0},
		},
		{
			name:     "mixed types",
			input:    []interface{}{3.0, "hello", true, nil},
			expected: []interface{}{nil, true, 3.0, "hello"},
		},
		{
			name:     "empty slice",
			input:    []interface{}{},
			expected: []interface{}{},
		},
		{
			name:     "booleans",
			input:    []interface{}{true, false, true},
			expected: []interface{}{false, true, true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortSlice(tt.input)
			if len(got) != len(tt.expected) {
				t.Fatalf("sortSlice returned %d elements, want %d", len(got), len(tt.expected))
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("sortSlice[%d] = %v, want %v", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestCanonicalLess(t *testing.T) {
	tests := []struct {
		name     string
		a, b     interface{}
		expected bool
	}{
		{"nil < bool", nil, true, true},
		{"bool < number", false, 1.0, true},
		{"number < string", 1.0, "a", true},
		{"string < array", "a", []interface{}{1.0}, true},
		{"false < true", false, true, true},
		{"true !< false", true, false, false},
		{"1 < 2", 1.0, 2.0, true},
		{"a < b", "a", "b", true},
		{"equal strings", "a", "a", false},
		{"equal numbers", 1.0, 1.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalLess(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("canonicalLess(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

// --- Canonicalize Tests ---

func TestCanonicalizerVersion(t *testing.T) {
	c := NewDefaultCanonicalizer()
	if c.Version() != CanonicalV1 {
		t.Errorf("default version = %q, want %q", c.Version(), CanonicalV1)
	}
}

func TestNewCanonicalizerUnsupportedVersion(t *testing.T) {
	_, err := NewCanonicalizer("canonical-v99")
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestCanonicalizeKeySorting(t *testing.T) {
	c := NewDefaultCanonicalizer()

	// Two maps with same keys in different order.
	data1 := map[string]interface{}{
		"zebra": 1.0,
		"apple": 2.0,
		"mango": 3.0,
	}
	data2 := map[string]interface{}{
		"mango": 3.0,
		"zebra": 1.0,
		"apple": 2.0,
	}

	r1, err := c.Canonicalize(data1)
	if err != nil {
		t.Fatalf("Canonicalize data1: %v", err)
	}
	r2, err := c.Canonicalize(data2)
	if err != nil {
		t.Fatalf("Canonicalize data2: %v", err)
	}

	if string(r1.CanonicalJSON) != string(r2.CanonicalJSON) {
		t.Errorf("key order affected output:\n  data1: %s\n  data2: %s",
			r1.CanonicalJSON, r2.CanonicalJSON)
	}
	if r1.Hash != r2.Hash {
		t.Errorf("key order affected hash: %s != %s", r1.Hash, r2.Hash)
	}

	// Verify keys are actually sorted in output.
	var decoded map[string]interface{}
	if err := json.Unmarshal(r1.CanonicalJSON, &decoded); err != nil {
		t.Fatalf("unmarshal canonical JSON: %v", err)
	}

	// Verify the JSON byte sequence has keys in order.
	expectedJSON := `{"apple":2,"mango":3,"zebra":1}`
	if string(r1.CanonicalJSON) != expectedJSON {
		t.Errorf("canonical JSON = %s, want %s", r1.CanonicalJSON, expectedJSON)
	}
}

func TestCanonicalizeArrayPreservesOrder(t *testing.T) {
	c := NewDefaultCanonicalizer()

	// Arrays preserve insertion order (important for conversation history).
	data := []interface{}{"third", "first", "second"}

	result, err := c.Canonicalize(data)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}

	expected := `["third","first","second"]`
	if string(result.CanonicalJSON) != expected {
		t.Errorf("canonical JSON = %s, want %s", result.CanonicalJSON, expected)
	}
}

func TestCanonicalizeNestedStructures(t *testing.T) {
	c := NewDefaultCanonicalizer()

	data := map[string]interface{}{
		"z_outer": map[string]interface{}{
			"z_inner": "value1",
			"a_inner": "value2",
		},
		"a_outer": []interface{}{
			map[string]interface{}{
				"c_key": 3.0,
				"a_key": 1.0,
			},
		},
	}

	result, err := c.Canonicalize(data)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}

	expected := `{"a_outer":[{"a_key":1,"c_key":3}],"z_outer":{"a_inner":"value2","z_inner":"value1"}}`
	if string(result.CanonicalJSON) != expected {
		t.Errorf("canonical JSON = %s\n  want          %s", result.CanonicalJSON, expected)
	}
}

func TestCanonicalizeStringNormalization(t *testing.T) {
	c := NewDefaultCanonicalizer()

	// Input with extra whitespace.
	data1 := map[string]interface{}{
		"name": "  hello   world  ",
	}
	// Input with normal whitespace.
	data2 := map[string]interface{}{
		"name": "hello world",
	}

	r1, err := c.Canonicalize(data1)
	if err != nil {
		t.Fatalf("Canonicalize data1: %v", err)
	}
	r2, err := c.Canonicalize(data2)
	if err != nil {
		t.Fatalf("Canonicalize data2: %v", err)
	}

	if r1.Hash != r2.Hash {
		t.Errorf("whitespace normalization failed: different hashes\n  data1: %s\n  data2: %s",
			r1.CanonicalJSON, r2.CanonicalJSON)
	}
}

func TestCanonicalizeUnicodeNormalization(t *testing.T) {
	c := NewDefaultCanonicalizer()

	// NFD: e + combining acute accent.
	data1 := map[string]interface{}{
		"name": "caf\u0065\u0301",
	}
	// NFC: precomposed é.
	data2 := map[string]interface{}{
		"name": "caf\u00e9",
	}

	r1, err := c.Canonicalize(data1)
	if err != nil {
		t.Fatalf("Canonicalize data1: %v", err)
	}
	r2, err := c.Canonicalize(data2)
	if err != nil {
		t.Fatalf("Canonicalize data2: %v", err)
	}

	if r1.Hash != r2.Hash {
		t.Errorf("unicode normalization failed: different hashes\n  data1: %s\n  data2: %s",
			r1.CanonicalJSON, r2.CanonicalJSON)
	}
}

func TestCanonicalizeEmptyArrayVsNull(t *testing.T) {
	c := NewDefaultCanonicalizer()

	data1 := map[string]interface{}{
		"items": []interface{}{},
	}
	data2 := map[string]interface{}{
		"items": nil,
	}

	r1, err := c.Canonicalize(data1)
	if err != nil {
		t.Fatalf("Canonicalize data1: %v", err)
	}
	r2, err := c.Canonicalize(data2)
	if err != nil {
		t.Fatalf("Canonicalize data2: %v", err)
	}

	// Empty array and null should produce different canonical forms.
	if r1.Hash == r2.Hash {
		t.Error("empty array and null should have different hashes")
	}
	if string(r1.CanonicalJSON) == string(r2.CanonicalJSON) {
		t.Error("empty array and null should have different canonical JSON")
	}

	expectedArray := `{"items":[]}`
	expectedNull := `{"items":null}`
	if string(r1.CanonicalJSON) != expectedArray {
		t.Errorf("empty array canonical = %s, want %s", r1.CanonicalJSON, expectedArray)
	}
	if string(r2.CanonicalJSON) != expectedNull {
		t.Errorf("null canonical = %s, want %s", r2.CanonicalJSON, expectedNull)
	}
}

func TestCanonicalizeNumericPrecision(t *testing.T) {
	c := NewDefaultCanonicalizer()

	// Values that should be considered equivalent.
	data1 := map[string]interface{}{
		"value": 1.0,
	}
	data2 := map[string]interface{}{
		"value": 1.0,
	}

	r1, err := c.Canonicalize(data1)
	if err != nil {
		t.Fatalf("Canonicalize data1: %v", err)
	}
	r2, err := c.Canonicalize(data2)
	if err != nil {
		t.Fatalf("Canonicalize data2: %v", err)
	}

	if r1.Hash != r2.Hash {
		t.Errorf("numeric precision failed: %s != %s", r1.Hash, r2.Hash)
	}
}

func TestCanonicalizeNegativeZero(t *testing.T) {
	c := NewDefaultCanonicalizer()

	data1 := map[string]interface{}{
		"value": 0.0,
	}
	data2 := map[string]interface{}{
		"value": math.Copysign(0, -1),
	}

	r1, err := c.Canonicalize(data1)
	if err != nil {
		t.Fatalf("Canonicalize data1: %v", err)
	}
	r2, err := c.Canonicalize(data2)
	if err != nil {
		t.Fatalf("Canonicalize data2: %v", err)
	}

	if r1.Hash != r2.Hash {
		t.Errorf("negative zero normalization failed:\n  +0: %s\n  -0: %s",
			r1.CanonicalJSON, r2.CanonicalJSON)
	}
}

// --- JSON Round-trip Tests ---

func TestCanonicalizeJSON(t *testing.T) {
	c := NewDefaultCanonicalizer()

	input := `{"b": 2, "a": 1, "c": {"z": 26, "a": 1}}`

	result, err := c.CanonicalizeJSON([]byte(input))
	if err != nil {
		t.Fatalf("CanonicalizeJSON: %v", err)
	}

	expected := `{"a":1,"b":2,"c":{"a":1,"z":26}}`
	if string(result.CanonicalJSON) != expected {
		t.Errorf("CanonicalizeJSON = %s, want %s", result.CanonicalJSON, expected)
	}
}

func TestCanonicalizeJSONRoundtrip(t *testing.T) {
	c := NewDefaultCanonicalizer()

	// Canonicalize, then re-canonicalize the output.
	input := `{"z": [3, 1, 2], "a": {"y": true, "x": false}}`

	r1, err := c.CanonicalizeJSON([]byte(input))
	if err != nil {
		t.Fatalf("first CanonicalizeJSON: %v", err)
	}

	r2, err := c.CanonicalizeJSON(r1.CanonicalJSON)
	if err != nil {
		t.Fatalf("second CanonicalizeJSON: %v", err)
	}

	if r1.Hash != r2.Hash {
		t.Errorf("roundtrip changed hash: %s != %s", r1.Hash, r2.Hash)
	}
	if string(r1.CanonicalJSON) != string(r2.CanonicalJSON) {
		t.Errorf("roundtrip changed JSON:\n  first:  %s\n  second: %s",
			r1.CanonicalJSON, r2.CanonicalJSON)
	}
}

func TestCanonicalizeJSONInvalid(t *testing.T) {
	c := NewDefaultCanonicalizer()

	_, err := c.CanonicalizeJSON([]byte("not valid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// --- CanonicalizeFactors Tests ---

func TestCanonicalizeFactors(t *testing.T) {
	c := NewDefaultCanonicalizer()

	factors1 := []ContextFactor{
		StaticFactor{FactorName: "z_factor", FactorCategory: FactorCategoryCustom, FactorValue: "z_val"},
		StaticFactor{FactorName: "a_factor", FactorCategory: FactorCategoryModel, FactorValue: "a_val"},
	}
	factors2 := []ContextFactor{
		StaticFactor{FactorName: "a_factor", FactorCategory: FactorCategoryModel, FactorValue: "a_val"},
		StaticFactor{FactorName: "z_factor", FactorCategory: FactorCategoryCustom, FactorValue: "z_val"},
	}

	r1, err := c.CanonicalizeFactors(factors1)
	if err != nil {
		t.Fatalf("CanonicalizeFactors factors1: %v", err)
	}
	r2, err := c.CanonicalizeFactors(factors2)
	if err != nil {
		t.Fatalf("CanonicalizeFactors factors2: %v", err)
	}

	if r1.Hash != r2.Hash {
		t.Errorf("factor order affected hash:\n  factors1: %s\n  factors2: %s",
			r1.CanonicalJSON, r2.CanonicalJSON)
	}
}

func TestCanonicalizeFactorsEmpty(t *testing.T) {
	c := NewDefaultCanonicalizer()

	result, err := c.CanonicalizeFactors(nil)
	if err != nil {
		t.Fatalf("CanonicalizeFactors nil: %v", err)
	}

	if string(result.CanonicalJSON) != "{}" {
		t.Errorf("empty factors = %s, want {}", result.CanonicalJSON)
	}
}

// --- Property-based Tests ---

func TestCanonicalizeOrderIndependence(t *testing.T) {
	c := NewDefaultCanonicalizer()

	// Build a map with known keys and values.
	keys := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"}
	values := []interface{}{1.0, "two", true, nil, 5.0, "six"}

	// Create the base map.
	baseMap := make(map[string]interface{})
	for i, k := range keys {
		baseMap[k] = values[i]
	}

	// Canonicalize the base.
	baseResult, err := c.Canonicalize(baseMap)
	if err != nil {
		t.Fatalf("Canonicalize base: %v", err)
	}

	// Create many random permutations and verify they all produce the same hash.
	rng := rand.New(rand.NewSource(42))
	for iter := 0; iter < 100; iter++ {
		permuted := make(map[string]interface{})
		perm := rng.Perm(len(keys))
		for _, idx := range perm {
			permuted[keys[idx]] = values[idx]
		}

		result, err := c.Canonicalize(permuted)
		if err != nil {
			t.Fatalf("Canonicalize permutation %d: %v", iter, err)
		}

		if result.Hash != baseResult.Hash {
			t.Errorf("permutation %d produced different hash: %s != %s",
				iter, result.Hash, baseResult.Hash)
		}
	}
}

func TestCanonicalizeStability(t *testing.T) {
	c := NewDefaultCanonicalizer()

	data := map[string]interface{}{
		"model": map[string]interface{}{
			"name":     "gpt-4",
			"provider": "openai",
			"version":  "2024-01-01",
		},
		"params": map[string]interface{}{
			"temperature": 0.7,
			"max_tokens":  1024.0,
		},
		"tags": []interface{}{"production", "v2"},
	}

	// Run canonicalization 1000 times and verify stability.
	var firstHash string
	var firstJSON string
	for i := 0; i < 1000; i++ {
		result, err := c.Canonicalize(data)
		if err != nil {
			t.Fatalf("Canonicalize iteration %d: %v", i, err)
		}
		if i == 0 {
			firstHash = result.Hash
			firstJSON = string(result.CanonicalJSON)
		} else {
			if result.Hash != firstHash {
				t.Fatalf("stability broken at iteration %d: hash %s != %s", i, result.Hash, firstHash)
			}
			if string(result.CanonicalJSON) != firstJSON {
				t.Fatalf("stability broken at iteration %d: JSON changed", i)
			}
		}
	}
}

func TestCanonicalizeNestedOrderIndependence(t *testing.T) {
	c := NewDefaultCanonicalizer()

	data1 := map[string]interface{}{
		"z": map[string]interface{}{
			"z_inner": map[string]interface{}{
				"value": 1.0,
				"name":  "test",
			},
			"a_inner": 2.0,
		},
		"a": "first",
	}

	data2 := map[string]interface{}{
		"a": "first",
		"z": map[string]interface{}{
			"a_inner": 2.0,
			"z_inner": map[string]interface{}{
				"name":  "test",
				"value": 1.0,
			},
		},
	}

	r1, err := c.Canonicalize(data1)
	if err != nil {
		t.Fatalf("Canonicalize data1: %v", err)
	}
	r2, err := c.Canonicalize(data2)
	if err != nil {
		t.Fatalf("Canonicalize data2: %v", err)
	}

	if r1.Hash != r2.Hash {
		t.Errorf("nested order independence failed:\n  data1: %s\n  data2: %s",
			r1.CanonicalJSON, r2.CanonicalJSON)
	}
}

// --- HashJSON Convenience Tests ---

func TestHashJSON(t *testing.T) {
	c := NewDefaultCanonicalizer()

	input1 := `{"b": 2, "a": 1}`
	input2 := `{"a": 1, "b": 2}`

	hash1, err := c.HashJSON([]byte(input1))
	if err != nil {
		t.Fatalf("HashJSON input1: %v", err)
	}
	hash2, err := c.HashJSON([]byte(input2))
	if err != nil {
		t.Fatalf("HashJSON input2: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("HashJSON order dependence: %s != %s", hash1, hash2)
	}

	// Different data should produce different hash.
	hash3, err := c.HashJSON([]byte(`{"a": 1, "b": 3}`))
	if err != nil {
		t.Fatalf("HashJSON input3: %v", err)
	}
	if hash1 == hash3 {
		t.Error("different data produced same hash")
	}
}

// --- Canonical Result Tests ---

func TestCanonicalResultVersionField(t *testing.T) {
	c := NewDefaultCanonicalizer()
	result, err := c.Canonicalize("test")
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if result.Version != CanonicalV1 {
		t.Errorf("result version = %q, want %q", result.Version, CanonicalV1)
	}
}

func TestCanonicalResultHashNonEmpty(t *testing.T) {
	c := NewDefaultCanonicalizer()
	result, err := c.Canonicalize(nil)
	if err != nil {
		t.Fatalf("Canonicalize nil: %v", err)
	}
	if result.Hash == "" {
		t.Error("hash should not be empty")
	}
	if len(result.Hash) != 64 {
		t.Errorf("hash length = %d, want 64 (SHA-256 hex)", len(result.Hash))
	}
}

// --- Edge Cases ---

func TestCanonicalizePrimitiveTypes(t *testing.T) {
	c := NewDefaultCanonicalizer()

	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"nil", nil, "null"},
		{"true", true, "true"},
		{"false", false, "false"},
		{"string", "hello", `"hello"`},
		{"number", 42.0, "42"},
		{"zero", 0.0, "0"},
		{"empty string", "", `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := c.Canonicalize(tt.input)
			if err != nil {
				t.Fatalf("Canonicalize: %v", err)
			}
			if string(result.CanonicalJSON) != tt.expected {
				t.Errorf("canonical JSON = %s, want %s", result.CanonicalJSON, tt.expected)
			}
		})
	}
}

func TestCanonicalizeDeeplyNested(t *testing.T) {
	c := NewDefaultCanonicalizer()

	// Build a deeply nested structure.
	var data interface{} = "leaf"
	for i := 0; i < 10; i++ {
		data = map[string]interface{}{
			"level": float64(i),
			"child": data,
		}
	}

	result, err := c.Canonicalize(data)
	if err != nil {
		t.Fatalf("Canonicalize deeply nested: %v", err)
	}
	if result.Hash == "" {
		t.Error("hash should not be empty for deeply nested structure")
	}

	// Verify stability.
	result2, err := c.Canonicalize(data)
	if err != nil {
		t.Fatalf("second Canonicalize: %v", err)
	}
	if result.Hash != result2.Hash {
		t.Error("deeply nested structure is not stable")
	}
}

func TestCanonicalizeEmptyMap(t *testing.T) {
	c := NewDefaultCanonicalizer()

	result, err := c.Canonicalize(map[string]interface{}{})
	if err != nil {
		t.Fatalf("Canonicalize empty map: %v", err)
	}
	if string(result.CanonicalJSON) != "{}" {
		t.Errorf("empty map = %s, want {}", result.CanonicalJSON)
	}
}

func TestCanonicalizeEmptySlice(t *testing.T) {
	c := NewDefaultCanonicalizer()

	result, err := c.Canonicalize([]interface{}{})
	if err != nil {
		t.Fatalf("Canonicalize empty slice: %v", err)
	}
	if string(result.CanonicalJSON) != "[]" {
		t.Errorf("empty slice = %s, want []", result.CanonicalJSON)
	}
}

func TestCanonicalizeStringEscaping(t *testing.T) {
	c := NewDefaultCanonicalizer()

	data := map[string]interface{}{
		"special": "hello \"world\" \\ \n \t",
	}

	result, err := c.Canonicalize(data)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}

	// Verify the result is valid JSON.
	var check interface{}
	if err := json.Unmarshal(result.CanonicalJSON, &check); err != nil {
		t.Errorf("canonical JSON is not valid: %v", err)
	}

	// Verify stability.
	result2, err := c.Canonicalize(data)
	if err != nil {
		t.Fatalf("second Canonicalize: %v", err)
	}
	if result.Hash != result2.Hash {
		t.Error("string escaping is not stable")
	}
}

func TestCanonicalizeJSONWithNumbers(t *testing.T) {
	c := NewDefaultCanonicalizer()

	// JSON with various numeric formats.
	input := `{"int": 42, "float": 3.14, "exp": 1e10, "neg": -0.5}`
	result, err := c.CanonicalizeJSON([]byte(input))
	if err != nil {
		t.Fatalf("CanonicalizeJSON: %v", err)
	}

	// Re-canonicalize to verify stability.
	result2, err := c.CanonicalizeJSON(result.CanonicalJSON)
	if err != nil {
		t.Fatalf("second CanonicalizeJSON: %v", err)
	}
	if result.Hash != result2.Hash {
		t.Errorf("numeric roundtrip changed hash:\n  first:  %s\n  second: %s",
			result.CanonicalJSON, result2.CanonicalJSON)
	}
}

// --- errors import needed for TestNewCanonicalizerUnsupportedVersion ---
// Import is already included via the errors.Is check.

func TestSliceLess(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []interface{}
		expected bool
	}{
		{"shorter < longer", []interface{}{1.0}, []interface{}{1.0, 2.0}, true},
		{"equal length equal values", []interface{}{1.0}, []interface{}{1.0}, false},
		{"first element less", []interface{}{1.0, 3.0}, []interface{}{2.0, 1.0}, true},
		{"empty < non-empty", []interface{}{}, []interface{}{1.0}, true},
		{"both empty", []interface{}{}, []interface{}{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sliceLess(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("sliceLess(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.expected)
			}
		})
	}
}

func TestMapLess(t *testing.T) {
	a := map[string]interface{}{"a": 1.0}
	b := map[string]interface{}{"b": 1.0}
	if !mapLess(a, b) {
		t.Error("expected map with key 'a' < map with key 'b'")
	}

	c := map[string]interface{}{"a": 1.0}
	d := map[string]interface{}{"a": 2.0}
	if !mapLess(c, d) {
		t.Error("expected map with value 1 < map with value 2 for same key")
	}
}

// --- Errors import for test ---
// The `errors` package is used in TestNewCanonicalizerUnsupportedVersion above.
