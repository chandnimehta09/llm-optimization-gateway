// Package context provides a context identity model that captures all factors
// affecting LLM output determinism. It enables the gateway to determine when
// cached responses are valid by tracking model parameters, repository state,
// prompt versions, tool schemas, and other contributing factors.
package context

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// FactorCategory classifies context factors by their domain.
type FactorCategory string

const (
	// FactorCategoryModel covers model identity and parameters.
	FactorCategoryModel FactorCategory = "model"
	// FactorCategoryPrompt covers prompt templates and system prompts.
	FactorCategoryPrompt FactorCategory = "prompt"
	// FactorCategoryRepository covers repository state (URL, branch, commit).
	FactorCategoryRepository FactorCategory = "repository"
	// FactorCategoryFile covers individual file content hashes.
	FactorCategoryFile FactorCategory = "file"
	// FactorCategoryDependency covers dependency lock state.
	FactorCategoryDependency FactorCategory = "dependency"
	// FactorCategoryTool covers tool schema and output identity.
	FactorCategoryTool FactorCategory = "tool"
	// FactorCategoryConversation covers conversation state.
	FactorCategoryConversation FactorCategory = "conversation"
	// FactorCategoryRetrieval covers RAG document identity.
	FactorCategoryRetrieval FactorCategory = "retrieval"
	// FactorCategoryCustom covers user-registered custom factors.
	FactorCategoryCustom FactorCategory = "custom"
)

// FactorWeight defines the importance of a factor in identity comparison.
// Higher weights mean the factor has more influence on cache invalidation.
type FactorWeight float64

const (
	// WeightCritical indicates the factor must match exactly for identity equality.
	WeightCritical FactorWeight = 1.0
	// WeightHigh indicates the factor is very important for identity matching.
	WeightHigh FactorWeight = 0.8
	// WeightMedium indicates moderate importance.
	WeightMedium FactorWeight = 0.5
	// WeightLow indicates low importance; differences may still allow partial matching.
	WeightLow FactorWeight = 0.2
)

// DefaultWeights maps factor categories to their default importance weights.
var DefaultWeights = map[FactorCategory]FactorWeight{
	FactorCategoryModel:        WeightCritical,
	FactorCategoryPrompt:       WeightCritical,
	FactorCategoryRepository:   WeightHigh,
	FactorCategoryFile:         WeightHigh,
	FactorCategoryDependency:   WeightMedium,
	FactorCategoryTool:         WeightHigh,
	FactorCategoryConversation: WeightCritical,
	FactorCategoryRetrieval:    WeightHigh,
	FactorCategoryCustom:       WeightMedium,
}

// ContextFactor is the interface for any factor that contributes to context identity.
// Custom factors can be registered to extend the identity model with domain-specific
// information.
type ContextFactor interface {
	// Name returns a unique identifier for this factor.
	Name() string
	// Category returns the factor's category for grouping and weight lookup.
	Category() FactorCategory
	// Value returns the factor's current value as a string for hashing.
	Value() string
	// Weight returns the importance weight for this factor.
	// Return 0 to use the default weight for the factor's category.
	Weight() FactorWeight
}

// StaticFactor is a simple implementation of ContextFactor with fixed values.
type StaticFactor struct {
	// FactorName is the unique name of this factor.
	FactorName string `json:"name"`
	// FactorCategory is the category this factor belongs to.
	FactorCategory FactorCategory `json:"category"`
	// FactorValue is the string value used for hashing.
	FactorValue string `json:"value"`
	// FactorWeight is the importance weight (0 means use category default).
	FactorWeight FactorWeight `json:"weight"`
}

// Name returns the factor name.
func (f StaticFactor) Name() string { return f.FactorName }

// Category returns the factor category.
func (f StaticFactor) Category() FactorCategory { return f.FactorCategory }

// Value returns the factor value.
func (f StaticFactor) Value() string { return f.FactorValue }

// Weight returns the factor weight, falling back to the category default if zero.
func (f StaticFactor) Weight() FactorWeight {
	if f.FactorWeight == 0 {
		if w, ok := DefaultWeights[f.FactorCategory]; ok {
			return w
		}
		return WeightMedium
	}
	return f.FactorWeight
}

// FactorRegistry holds registered custom factor extractors.
type FactorRegistry struct {
	extractors map[string]FactorExtractor
}

// FactorExtractor is a function that produces context factors from a given input.
type FactorExtractor func() ([]ContextFactor, error)

// NewFactorRegistry creates an empty factor registry.
func NewFactorRegistry() *FactorRegistry {
	return &FactorRegistry{
		extractors: make(map[string]FactorExtractor),
	}
}

// Register adds a named factor extractor. If an extractor with the same name
// already exists, it is replaced.
func (r *FactorRegistry) Register(name string, extractor FactorExtractor) {
	r.extractors[name] = extractor
}

// Unregister removes a named factor extractor. Returns true if it existed.
func (r *FactorRegistry) Unregister(name string) bool {
	_, existed := r.extractors[name]
	delete(r.extractors, name)
	return existed
}

// Extract runs all registered extractors and returns the combined factors.
// If any extractor fails, the error is returned and no partial results are given.
func (r *FactorRegistry) Extract() ([]ContextFactor, error) {
	var factors []ContextFactor
	// Sort extractor names for deterministic ordering.
	names := make([]string, 0, len(r.extractors))
	for name := range r.extractors {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		extracted, err := r.extractors[name]()
		if err != nil {
			return nil, fmt.Errorf("factor extractor %q failed: %w", name, err)
		}
		factors = append(factors, extracted...)
	}
	return factors, nil
}

// Names returns the sorted names of all registered extractors.
func (r *FactorRegistry) Names() []string {
	names := make([]string, 0, len(r.extractors))
	for name := range r.extractors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// HashFactors computes a SHA-256 hash from a slice of context factors.
// Factors are sorted by name before hashing to ensure deterministic output.
func HashFactors(factors []ContextFactor) string {
	if len(factors) == 0 {
		return ""
	}

	// Sort factors by name for determinism.
	sorted := make([]ContextFactor, len(factors))
	copy(sorted, factors)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name() < sorted[j].Name()
	})

	h := sha256.New()
	for _, f := range sorted {
		// Write "name=value\n" for each factor.
		line := f.Name() + "=" + f.Value() + "\n"
		h.Write([]byte(line))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// FormatFactorSummary returns a human-readable summary of factors grouped by category.
func FormatFactorSummary(factors []ContextFactor) string {
	grouped := make(map[FactorCategory][]ContextFactor)
	for _, f := range factors {
		grouped[f.Category()] = append(grouped[f.Category()], f)
	}

	categories := make([]string, 0, len(grouped))
	for cat := range grouped {
		categories = append(categories, string(cat))
	}
	sort.Strings(categories)

	var b strings.Builder
	for _, cat := range categories {
		b.WriteString(fmt.Sprintf("[%s]\n", cat))
		catFactors := grouped[FactorCategory(cat)]
		sort.Slice(catFactors, func(i, j int) bool {
			return catFactors[i].Name() < catFactors[j].Name()
		})
		for _, f := range catFactors {
			b.WriteString(fmt.Sprintf("  %s = %s (weight: %.1f)\n", f.Name(), f.Value(), f.Weight()))
		}
	}
	return b.String()
}
