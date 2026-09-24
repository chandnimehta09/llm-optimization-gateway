// Package rules provides concrete policy rule implementations for the
// LLM optimization gateway. It includes model routing, provider selection,
// and model mapping logic.
package rules

import (
	"fmt"
	"strings"
)

// RoutingStrategy defines how model routing decisions are made.
type RoutingStrategy string

const (
	// RoutingStrategyDirect routes to the explicitly requested model/provider.
	RoutingStrategyDirect RoutingStrategy = "direct"
	// RoutingStrategyCostOptimized selects the cheapest provider for the model.
	RoutingStrategyCostOptimized RoutingStrategy = "cost_optimized"
	// RoutingStrategyLatencyOptimized selects the lowest-latency provider.
	RoutingStrategyLatencyOptimized RoutingStrategy = "latency_optimized"
	// RoutingStrategyCapabilityBased routes based on required capabilities.
	RoutingStrategyCapabilityBased RoutingStrategy = "capability_based"
)

// ModelInfo describes a model's properties for routing decisions.
type ModelInfo struct {
	// CanonicalName is the model's canonical identifier.
	CanonicalName string `json:"canonical_name"`
	// Provider is the default provider for this model.
	Provider string `json:"provider"`
	// CostPer1KTokens is the cost per 1K tokens (input) in USD.
	CostPer1KTokens float64 `json:"cost_per_1k_tokens"`
	// AverageLatencyMs is the expected average latency in milliseconds.
	AverageLatencyMs float64 `json:"average_latency_ms"`
	// Capabilities lists the capabilities this model supports.
	Capabilities []string `json:"capabilities"`
	// Deprecated marks whether the model is deprecated.
	Deprecated bool `json:"deprecated"`
	// ReplacementModel is the suggested replacement when deprecated.
	ReplacementModel string `json:"replacement_model,omitempty"`
	// MaxContextTokens is the maximum context window size.
	MaxContextTokens int `json:"max_context_tokens"`
}

// ProviderConfig describes provider-level routing configuration.
type ProviderConfig struct {
	// Name is the provider identifier.
	Name string `json:"name"`
	// Priority is the routing priority (lower is higher priority).
	Priority int `json:"priority"`
	// Healthy indicates whether the provider is currently available.
	Healthy bool `json:"healthy"`
	// RateLimitRemaining is the estimated remaining rate limit capacity (0.0–1.0).
	RateLimitRemaining float64 `json:"rate_limit_remaining"`
	// CostMultiplier adjusts the cost weight for this provider.
	CostMultiplier float64 `json:"cost_multiplier"`
	// Models lists canonical model names available from this provider.
	Models []string `json:"models"`
}

// RoutingConfig holds the complete routing configuration.
type RoutingConfig struct {
	// ModelAliases maps alias names to canonical model names.
	ModelAliases map[string]string `json:"model_aliases"`
	// Models maps canonical model names to their info.
	Models map[string]ModelInfo `json:"models"`
	// Providers maps provider names to their config.
	Providers map[string]ProviderConfig `json:"providers"`
	// EquivalentModels maps a canonical model to equivalent models on other providers.
	EquivalentModels map[string][]ModelEquivalent `json:"equivalent_models"`
	// DefaultStrategy is the routing strategy when none is specified.
	DefaultStrategy RoutingStrategy `json:"default_strategy"`
	// DefaultProvider is the fallback provider.
	DefaultProvider string `json:"default_provider"`
	// DefaultModel is the fallback model.
	DefaultModel string `json:"default_model"`
}

// ModelEquivalent defines a model on another provider that is functionally
// equivalent to a source model.
type ModelEquivalent struct {
	// Model is the canonical name of the equivalent model.
	Model string `json:"model"`
	// Provider is the provider that hosts this equivalent model.
	Provider string `json:"provider"`
	// SimilarityScore indicates how similar this model is (0.0–1.0).
	SimilarityScore float64 `json:"similarity_score"`
}

// RoutingDecision is the output of the routing evaluation.
type RoutingDecision struct {
	// Model is the resolved canonical model name.
	Model string `json:"model"`
	// Provider is the selected provider.
	Provider string `json:"provider"`
	// FallbackProviders lists backup providers in order of preference.
	FallbackProviders []string `json:"fallback_providers,omitempty"`
	// Strategy is the routing strategy that was applied.
	Strategy RoutingStrategy `json:"strategy"`
	// Reasoning explains the routing decision.
	Reasoning string `json:"reasoning"`
	// Warnings lists non-fatal issues encountered.
	Warnings []string `json:"warnings,omitempty"`
	// AliasResolved indicates if the model was resolved from an alias.
	AliasResolved bool `json:"alias_resolved"`
	// OriginalModel is the model name as originally requested.
	OriginalModel string `json:"original_model"`
	// DeprecationWarning is set when the resolved model is deprecated.
	DeprecationWarning string `json:"deprecation_warning,omitempty"`
}

// providerCandidate is a helper for sorting providers by priority.
type providerCandidate struct {
	name     string
	priority int
}

// ResolveModelAlias resolves a model name through the alias table. If the
// model is an alias, the canonical name is returned. Otherwise the original
// name is returned unchanged.
func ResolveModelAlias(model string, config *RoutingConfig) (canonical string, wasAlias bool) {
	if config == nil || config.ModelAliases == nil {
		return model, false
	}
	if resolved, ok := config.ModelAliases[model]; ok {
		return resolved, true
	}
	// Try case-insensitive lookup.
	lower := strings.ToLower(model)
	for alias, canonical := range config.ModelAliases {
		if strings.ToLower(alias) == lower {
			return canonical, true
		}
	}
	return model, false
}

// LookupModelInfo retrieves the ModelInfo for a canonical model name.
// Returns the info and true if found, or zero value and false otherwise.
func LookupModelInfo(model string, config *RoutingConfig) (ModelInfo, bool) {
	if config == nil || config.Models == nil {
		return ModelInfo{}, false
	}
	info, ok := config.Models[model]
	return info, ok
}

// CheckDeprecation checks if a model is deprecated and returns a warning
// message if so, along with the replacement model name.
func CheckDeprecation(model string, config *RoutingConfig) (deprecated bool, replacement string, warning string) {
	info, ok := LookupModelInfo(model, config)
	if !ok {
		return false, "", ""
	}
	if !info.Deprecated {
		return false, "", ""
	}
	replacement = info.ReplacementModel
	if replacement != "" {
		warning = fmt.Sprintf("model %q is deprecated, consider using %q instead", model, replacement)
	} else {
		warning = fmt.Sprintf("model %q is deprecated", model)
	}
	return true, replacement, warning
}

// FindEquivalentModels returns models on other providers that are functionally
// equivalent to the given model.
func FindEquivalentModels(model string, config *RoutingConfig) []ModelEquivalent {
	if config == nil || config.EquivalentModels == nil {
		return nil
	}
	return config.EquivalentModels[model]
}

// SelectProviderForModel determines which provider should serve a model based
// on the routing config. It returns the provider name and whether the model
// was found in any provider's model list.
func SelectProviderForModel(model string, config *RoutingConfig) (provider string, found bool) {
	if config == nil {
		return "", false
	}

	// First, check if the model info specifies a default provider.
	if info, ok := config.Models[model]; ok && info.Provider != "" {
		if pc, ok := config.Providers[info.Provider]; ok && pc.Healthy {
			return info.Provider, true
		}
	}

	// Find all healthy providers that support this model, sorted by priority.
	var candidates []providerCandidate

	for name, pc := range config.Providers {
		if !pc.Healthy {
			continue
		}
		for _, m := range pc.Models {
			if m == model {
				candidates = append(candidates, providerCandidate{name: name, priority: pc.Priority})
				break
			}
		}
	}

	if len(candidates) == 0 {
		return "", false
	}

	// Select by lowest priority number (highest priority).
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.priority < best.priority {
			best = c
		}
	}
	return best.name, true
}

// SelectFallbackProviders returns an ordered list of fallback providers
// for a model, excluding the primary provider.
func SelectFallbackProviders(model, primaryProvider string, config *RoutingConfig) []string {
	if config == nil {
		return nil
	}

	var candidates []providerCandidate

	// Check direct model support.
	for name, pc := range config.Providers {
		if name == primaryProvider || !pc.Healthy {
			continue
		}
		for _, m := range pc.Models {
			if m == model {
				candidates = append(candidates, providerCandidate{name: name, priority: pc.Priority})
				break
			}
		}
	}

	// Also check equivalent models.
	equivalents := FindEquivalentModels(model, config)
	for _, eq := range equivalents {
		if eq.Provider == primaryProvider {
			continue
		}
		if pc, ok := config.Providers[eq.Provider]; ok && pc.Healthy {
			// Check if already in candidates.
			alreadyAdded := false
			for _, c := range candidates {
				if c.name == eq.Provider {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				candidates = append(candidates, providerCandidate{name: eq.Provider, priority: pc.Priority})
			}
		}
	}

	// Sort by priority (insertion sort, stable for small slices).
	for i := 1; i < len(candidates); i++ {
		key := candidates[i]
		j := i - 1
		for j >= 0 && candidates[j].priority > key.priority {
			candidates[j+1] = candidates[j]
			j--
		}
		candidates[j+1] = key
	}

	result := make([]string, len(candidates))
	for i, c := range candidates {
		result[i] = c.name
	}
	return result
}

// SelectCostOptimalProvider selects the provider with the lowest effective
// cost for a given model. The effective cost is the model's cost per 1K
// tokens multiplied by the provider's cost multiplier.
func SelectCostOptimalProvider(model string, config *RoutingConfig) (provider string, found bool) {
	if config == nil {
		return "", false
	}

	modelInfo, hasModelInfo := config.Models[model]

	type candidate struct {
		name string
		cost float64
	}
	var candidates []candidate

	for provName, pc := range config.Providers {
		if !pc.Healthy || pc.RateLimitRemaining <= 0 {
			continue
		}
		for _, m := range pc.Models {
			if m == model {
				cost := modelInfo.CostPer1KTokens
				if !hasModelInfo {
					cost = 1.0 // default cost
				}
				if pc.CostMultiplier > 0 {
					cost *= pc.CostMultiplier
				}
				candidates = append(candidates, candidate{name: provName, cost: cost})
				break
			}
		}
	}

	// Also check equivalent models with their own costs.
	if equivs, ok := config.EquivalentModels[model]; ok {
		for _, eq := range equivs {
			pc, provOk := config.Providers[eq.Provider]
			if !provOk || !pc.Healthy || pc.RateLimitRemaining <= 0 {
				continue
			}
			eqInfo, eqOk := config.Models[eq.Model]
			cost := 1.0
			if eqOk {
				cost = eqInfo.CostPer1KTokens
			}
			if pc.CostMultiplier > 0 {
				cost *= pc.CostMultiplier
			}
			candidates = append(candidates, candidate{name: eq.Provider, cost: cost})
		}
	}

	if len(candidates) == 0 {
		return "", false
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.cost < best.cost {
			best = c
		}
	}
	return best.name, true
}

// SelectLatencyOptimalProvider selects the provider with the lowest average
// latency for a given model.
func SelectLatencyOptimalProvider(model string, config *RoutingConfig) (provider string, found bool) {
	if config == nil {
		return "", false
	}

	type candidate struct {
		name    string
		latency float64
	}
	var candidates []candidate

	for provName, pc := range config.Providers {
		if !pc.Healthy || pc.RateLimitRemaining <= 0 {
			continue
		}
		for _, m := range pc.Models {
			if m == model {
				// Use model-level latency if available, otherwise estimate from provider info.
				latency := 0.0
				if info, ok := config.Models[model]; ok && info.AverageLatencyMs > 0 {
					latency = info.AverageLatencyMs
				}
				candidates = append(candidates, candidate{name: provName, latency: latency})
				break
			}
		}
	}

	if len(candidates) == 0 {
		return "", false
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.latency < best.latency || (c.latency == best.latency && c.name < best.name) {
			best = c
		}
	}
	return best.name, true
}

// ProviderSupportsCapabilities checks whether a provider supports all the
// requested capabilities.
func ProviderSupportsCapabilities(providerName string, requiredCaps []string, config *RoutingConfig) bool {
	if config == nil || len(requiredCaps) == 0 {
		return true
	}

	// Check model capabilities through model info.
	pc, ok := config.Providers[providerName]
	if !ok {
		return false
	}

	// Build set of capabilities from models hosted by this provider.
	capSet := make(map[string]bool)
	for _, modelName := range pc.Models {
		if info, ok := config.Models[modelName]; ok {
			for _, cap := range info.Capabilities {
				capSet[cap] = true
			}
		}
	}

	for _, req := range requiredCaps {
		if !capSet[req] {
			return false
		}
	}
	return true
}

// SelectProviderByCapabilities selects the highest-priority healthy provider
// that supports all required capabilities.
func SelectProviderByCapabilities(model string, requiredCaps []string, config *RoutingConfig) (provider string, found bool) {
	if config == nil {
		return "", false
	}

	type candidate struct {
		name     string
		priority int
	}
	var candidates []candidate

	for provName, pc := range config.Providers {
		if !pc.Healthy || pc.RateLimitRemaining <= 0 {
			continue
		}
		// Check if provider has the model.
		hasModel := false
		for _, m := range pc.Models {
			if m == model {
				hasModel = true
				break
			}
		}
		if !hasModel {
			continue
		}
		// Check capabilities.
		if ProviderSupportsCapabilities(provName, requiredCaps, config) {
			candidates = append(candidates, candidate{name: provName, priority: pc.Priority})
		}
	}

	if len(candidates) == 0 {
		return "", false
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.priority < best.priority {
			best = c
		}
	}
	return best.name, true
}

// NewDefaultRoutingConfig creates a RoutingConfig with standard OpenAI and
// Gemini model definitions.
func NewDefaultRoutingConfig() *RoutingConfig {
	return &RoutingConfig{
		ModelAliases: map[string]string{
			"fast-model":    "gpt-3.5-turbo",
			"smart-model":   "gpt-4",
			"cheap-model":   "gpt-3.5-turbo",
			"best-model":    "gpt-4",
			"gemini":        "gemini-pro",
			"gemini-latest": "gemini-1.5-pro",
		},
		Models: map[string]ModelInfo{
			"gpt-4": {
				CanonicalName:    "gpt-4",
				Provider:         "openai",
				CostPer1KTokens:  0.03,
				AverageLatencyMs: 500,
				Capabilities:     []string{"chat", "function_calling", "vision"},
				MaxContextTokens: 8192,
			},
			"gpt-4-turbo": {
				CanonicalName:    "gpt-4-turbo",
				Provider:         "openai",
				CostPer1KTokens:  0.01,
				AverageLatencyMs: 400,
				Capabilities:     []string{"chat", "function_calling", "vision", "json_mode"},
				MaxContextTokens: 128000,
			},
			"gpt-3.5-turbo": {
				CanonicalName:    "gpt-3.5-turbo",
				Provider:         "openai",
				CostPer1KTokens:  0.0005,
				AverageLatencyMs: 200,
				Capabilities:     []string{"chat", "function_calling"},
				MaxContextTokens: 16385,
			},
			"gemini-pro": {
				CanonicalName:    "gemini-pro",
				Provider:         "google",
				CostPer1KTokens:  0.00025,
				AverageLatencyMs: 300,
				Capabilities:     []string{"chat", "function_calling"},
				MaxContextTokens: 32768,
			},
			"gemini-1.5-pro": {
				CanonicalName:    "gemini-1.5-pro",
				Provider:         "google",
				CostPer1KTokens:  0.00125,
				AverageLatencyMs: 350,
				Capabilities:     []string{"chat", "function_calling", "vision", "long_context"},
				MaxContextTokens: 1048576,
			},
			"gemini-1.5-flash": {
				CanonicalName:    "gemini-1.5-flash",
				Provider:         "google",
				CostPer1KTokens:  0.000075,
				AverageLatencyMs: 150,
				Capabilities:     []string{"chat", "function_calling"},
				MaxContextTokens: 1048576,
			},
		},
		Providers: map[string]ProviderConfig{
			"openai": {
				Name:               "openai",
				Priority:           1,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     1.0,
				Models:             []string{"gpt-4", "gpt-4-turbo", "gpt-3.5-turbo"},
			},
			"google": {
				Name:               "google",
				Priority:           2,
				Healthy:            true,
				RateLimitRemaining: 1.0,
				CostMultiplier:     1.0,
				Models:             []string{"gemini-pro", "gemini-1.5-pro", "gemini-1.5-flash"},
			},
		},
		EquivalentModels: map[string][]ModelEquivalent{
			"gpt-4": {
				{Model: "gemini-1.5-pro", Provider: "google", SimilarityScore: 0.85},
			},
			"gpt-3.5-turbo": {
				{Model: "gemini-pro", Provider: "google", SimilarityScore: 0.80},
				{Model: "gemini-1.5-flash", Provider: "google", SimilarityScore: 0.75},
			},
			"gemini-pro": {
				{Model: "gpt-3.5-turbo", Provider: "openai", SimilarityScore: 0.80},
			},
			"gemini-1.5-pro": {
				{Model: "gpt-4", Provider: "openai", SimilarityScore: 0.85},
			},
		},
		DefaultStrategy: RoutingStrategyDirect,
		DefaultProvider:  "openai",
		DefaultModel:     "gpt-3.5-turbo",
	}
}
