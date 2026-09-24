package context

import (
	"fmt"
	"sort"
)

// sortMapKeys returns the keys of a map[string]interface{} in sorted order.
func sortMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortStringSlice returns a sorted copy of a string slice.
func sortStringSlice(s []string) []string {
	sorted := make([]string, len(s))
	copy(sorted, s)
	sort.Strings(sorted)
	return sorted
}

// sortSlice sorts a slice of arbitrary values using a canonical comparison.
// This is used for array/list values in the canonicalization process where
// order independence is desired (e.g., tag lists, feature flags).
//
// For ordered arrays (like conversation history), sorting should NOT be applied.
// The caller is responsible for deciding whether to sort based on context.
func sortSlice(s []interface{}) []interface{} {
	sorted := make([]interface{}, len(s))
	copy(sorted, s)
	sort.SliceStable(sorted, func(i, j int) bool {
		return canonicalLess(sorted[i], sorted[j])
	})
	return sorted
}

// canonicalLess defines a total ordering over arbitrary JSON-compatible values.
// The ordering is: null < bool < number < string < array < object.
// Within each type, natural ordering is used.
func canonicalLess(a, b interface{}) bool {
	ta := typeOrder(a)
	tb := typeOrder(b)
	if ta != tb {
		return ta < tb
	}

	switch va := a.(type) {
	case nil:
		return false // nil == nil, not less
	case bool:
		vb := b.(bool)
		if va == vb {
			return false
		}
		return !va && vb // false < true
	case float64:
		return va < b.(float64)
	case string:
		return va < b.(string)
	case []interface{}:
		vb := b.([]interface{})
		return sliceLess(va, vb)
	case map[string]interface{}:
		vb := b.(map[string]interface{})
		return mapLess(va, vb)
	default:
		// Fallback: compare string representations.
		return fmt.Sprintf("%v", a) < fmt.Sprintf("%v", b)
	}
}

// typeOrder assigns a numeric rank to each JSON type for cross-type comparison.
func typeOrder(v interface{}) int {
	switch v.(type) {
	case nil:
		return 0
	case bool:
		return 1
	case float64:
		return 2
	case string:
		return 3
	case []interface{}:
		return 4
	case map[string]interface{}:
		return 5
	default:
		return 6
	}
}

// sliceLess compares two slices lexicographically.
func sliceLess(a, b []interface{}) bool {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if canonicalLess(a[i], b[i]) {
			return true
		}
		if canonicalLess(b[i], a[i]) {
			return false
		}
	}
	return len(a) < len(b)
}

// mapLess compares two maps by their sorted key-value pairs.
func mapLess(a, b map[string]interface{}) bool {
	aKeys := sortMapKeys(a)
	bKeys := sortMapKeys(b)

	minLen := len(aKeys)
	if len(bKeys) < minLen {
		minLen = len(bKeys)
	}

	for i := 0; i < minLen; i++ {
		if aKeys[i] < bKeys[i] {
			return true
		}
		if aKeys[i] > bKeys[i] {
			return false
		}
		// Same key, compare values.
		if canonicalLess(a[aKeys[i]], b[bKeys[i]]) {
			return true
		}
		if canonicalLess(b[bKeys[i]], a[aKeys[i]]) {
			return false
		}
	}
	return len(aKeys) < len(bKeys)
}
