package context

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// CanonicalVersion identifies the canonicalization algorithm version.
// Changing the algorithm requires bumping this version so that consumers
// can detect and handle the change (e.g., invalidate caches).
type CanonicalVersion string

const (
	// CanonicalV1 is the initial canonicalization algorithm.
	CanonicalV1 CanonicalVersion = "canonical-v1"
)

// CurrentCanonicalVersion is the algorithm version used by default.
const CurrentCanonicalVersion = CanonicalV1

// ErrUnsupportedVersion is returned when an unknown canonicalization version is requested.
var ErrUnsupportedVersion = errors.New("unsupported canonicalization version")

// CanonicalResult holds the output of a canonicalization operation.
type CanonicalResult struct {
	// Version is the canonicalization algorithm version used.
	Version CanonicalVersion `json:"version"`
	// CanonicalJSON is the deterministic JSON representation.
	CanonicalJSON []byte `json:"canonical_json"`
	// Hash is the SHA-256 hash of the canonical JSON.
	Hash string `json:"hash"`
}

// Canonicalizer performs deterministic canonicalization of context data.
type Canonicalizer struct {
	version CanonicalVersion
}

// NewCanonicalizer creates a Canonicalizer with the specified algorithm version.
// Returns ErrUnsupportedVersion if the version is not recognized.
func NewCanonicalizer(version CanonicalVersion) (*Canonicalizer, error) {
	switch version {
	case CanonicalV1:
		return &Canonicalizer{version: version}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedVersion, version)
	}
}

// NewDefaultCanonicalizer creates a Canonicalizer using the current default version.
func NewDefaultCanonicalizer() *Canonicalizer {
	return &Canonicalizer{version: CurrentCanonicalVersion}
}

// Version returns the canonicalization algorithm version.
func (c *Canonicalizer) Version() CanonicalVersion {
	return c.version
}

// Canonicalize transforms arbitrary JSON-compatible data into a deterministic
// canonical form. The input can be any value that json.Unmarshal would produce
// when decoding into an interface{} (maps, slices, strings, numbers, bools, nil).
//
// The canonical form guarantees:
//   - Map keys are sorted alphabetically
//   - String values are whitespace-normalized and Unicode NFC-normalized
//   - Numeric values use a canonical representation
//   - Nested structures are recursively canonicalized
//   - The output is deterministic: same logical input always produces same output
func (c *Canonicalizer) Canonicalize(data interface{}) (*CanonicalResult, error) {
	canonical := canonicalizeValue(data)

	jsonBytes, err := marshalCanonical(canonical)
	if err != nil {
		return nil, fmt.Errorf("marshaling canonical JSON: %w", err)
	}

	hash := sha256.Sum256(jsonBytes)

	return &CanonicalResult{
		Version:       c.version,
		CanonicalJSON: jsonBytes,
		Hash:          hex.EncodeToString(hash[:]),
	}, nil
}

// CanonicalizeJSON takes raw JSON bytes, decodes them, canonicalizes the result,
// and returns the canonical form. This is the primary entry point for processing
// JSON input from external sources.
func (c *Canonicalizer) CanonicalizeJSON(rawJSON []byte) (*CanonicalResult, error) {
	var data interface{}
	decoder := json.NewDecoder(bytes.NewReader(rawJSON))
	decoder.UseNumber()
	if err := decoder.Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	// Convert json.Number values to float64 for uniform handling.
	data = convertNumbers(data)

	return c.Canonicalize(data)
}

// CanonicalizeFactors takes a slice of ContextFactor values and produces a
// canonical representation. Factors are converted to a map keyed by name,
// sorted, and canonicalized.
func (c *Canonicalizer) CanonicalizeFactors(factors []ContextFactor) (*CanonicalResult, error) {
	if len(factors) == 0 {
		return c.Canonicalize(map[string]interface{}{})
	}

	m := make(map[string]interface{}, len(factors))
	for _, f := range factors {
		m[f.Name()] = map[string]interface{}{
			"category": string(f.Category()),
			"value":    f.Value(),
			"weight":   float64(f.Weight()),
		}
	}
	return c.Canonicalize(m)
}

// HashJSON is a convenience method that canonicalizes JSON and returns only the hash.
func (c *Canonicalizer) HashJSON(rawJSON []byte) (string, error) {
	result, err := c.CanonicalizeJSON(rawJSON)
	if err != nil {
		return "", err
	}
	return result.Hash, nil
}

// canonicalizeValue recursively transforms a value into its canonical form.
func canonicalizeValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case map[string]interface{}:
		return canonicalizeMap(val)
	case []interface{}:
		return canonicalizeSlice(val)
	default:
		return normalizeValue(v)
	}
}

// canonicalizeMap produces a canonical map by sorting keys and recursively
// canonicalizing all values.
func canonicalizeMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		// Normalize the key (whitespace, Unicode).
		normalizedKey := normalizeString(k)
		result[normalizedKey] = canonicalizeValue(v)
	}
	return result
}

// canonicalizeSlice produces a canonical slice by recursively canonicalizing
// each element. Array order is preserved (important for conversation history
// and other ordered data).
func canonicalizeSlice(s []interface{}) []interface{} {
	result := make([]interface{}, len(s))
	for i, v := range s {
		result[i] = canonicalizeValue(v)
	}
	return result
}

// marshalCanonical serializes a value to JSON with sorted map keys and no
// unnecessary whitespace. This produces a single deterministic byte sequence
// for any logically equivalent input.
func marshalCanonical(v interface{}) ([]byte, error) {
	// json.Marshal already sorts map keys in Go 1.12+.
	// We use no indentation for a compact canonical form.
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	// json.Encoder.Encode appends a newline; remove it for canonical form.
	b := buf.Bytes()
	if len(b) > 0 && b[len(b)-1] == '\n' {
		b = b[:len(b)-1]
	}
	return b, nil
}

// convertNumbers recursively converts json.Number values to float64.
// This ensures uniform numeric handling throughout canonicalization.
func convertNumbers(v interface{}) interface{} {
	switch val := v.(type) {
	case json.Number:
		f, err := val.Float64()
		if err != nil {
			// If it can't be converted to float64, keep as string.
			return val.String()
		}
		return f
	case map[string]interface{}:
		for k, child := range val {
			val[k] = convertNumbers(child)
		}
		return val
	case []interface{}:
		for i, child := range val {
			val[i] = convertNumbers(child)
		}
		return val
	default:
		return v
	}
}
