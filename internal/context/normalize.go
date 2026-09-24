package context

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// normalizeString applies canonical string normalization:
//   - Trims leading/trailing whitespace
//   - Collapses multiple consecutive whitespace characters into a single space
//   - Normalizes Unicode to NFC form
func normalizeString(s string) string {
	s = strings.TrimSpace(s)
	s = collapseSpaces(s)
	s = norm.NFC.String(s)
	return s
}

// collapseSpacesRe matches two or more consecutive whitespace characters.
var collapseSpacesRe = regexp.MustCompile(`\s{2,}`)

// collapseSpaces replaces runs of whitespace with a single space.
func collapseSpaces(s string) string {
	return collapseSpacesRe.ReplaceAllString(s, " ")
}

// normalizeFloat produces a canonical string representation of a float64 value.
// It handles special cases (NaN, infinities, negative zero) and uses the
// shortest representation that round-trips exactly.
func normalizeFloat(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "+Inf"
	}
	if math.IsInf(f, -1) {
		return "-Inf"
	}
	// Normalize negative zero to positive zero.
	if f == 0 {
		return "0"
	}
	// Use %g for the shortest representation that distinguishes the value.
	return fmt.Sprintf("%g", f)
}

// normalizeValue applies type-appropriate normalization to an arbitrary value.
// Strings are whitespace-normalized and NFC-normalized.
// Numbers are formatted canonically.
// Nil values pass through unchanged.
// Maps and slices are handled recursively by the canonicalization layer.
func normalizeValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		return normalizeString(val)
	case float64:
		// JSON numbers are always float64 when unmarshaled into interface{}.
		// We keep them as float64 but ensure canonical representation when serialized.
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return normalizeFloat(val)
		}
		// Normalize -0 to 0.
		if val == 0 {
			return float64(0)
		}
		return val
	case bool:
		return val
	default:
		// For maps, slices, and other types, return as-is.
		// Recursive normalization is handled by canonicalizeValue.
		return val
	}
}
