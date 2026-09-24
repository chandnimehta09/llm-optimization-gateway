// Package routing provides the routing engine for executing policy decisions,
// managing provider interactions, and handling provider fallback in the
// LLM optimization gateway.
package routing

import (
	"fmt"
	"strings"
	"sync"
)

// Feature represents a provider feature capability.
type Feature string

const (
	// FeatureStreaming indicates the provider supports streaming responses.
	FeatureStreaming Feature = "streaming"
	// FeatureTools indicates the provider supports tool/function calling.
	FeatureTools Feature = "tools"
	// FeatureVision indicates the provider supports vision/image inputs.
	FeatureVision Feature = "vision"
	// FeatureJSON indicates the provider supports JSON mode output.
	FeatureJSON Feature = "json_mode"
)

// RateLimit describes a provider's rate limiting state.
type RateLimit struct {
	// RequestsPerMinute is the maximum requests allowed per minute.
	RequestsPerMinute int `json:"requests_per_minute"`
	// TokensPerMinute is the maximum tokens allowed per minute.
	TokensPerMinute int `json:"tokens_per_minute"`
	// RemainingRequests is the estimated remaining request capacity.
	RemainingRequests int `json:"remaining_requests"`
	// RemainingTokens is the estimated remaining token capacity.
	RemainingTokens int `json:"remaining_tokens"`
}

// HasCapacity returns true if the rate limit has remaining capacity.
func (r RateLimit) HasCapacity() bool {
	return r.RemainingRequests > 0 && r.RemainingTokens > 0
}

// UsagePercent returns the percentage of rate limit consumed (0.0–1.0).
func (r RateLimit) UsagePercent() float64 {
	if r.RequestsPerMinute == 0 && r.TokensPerMinute == 0 {
		return 0
	}
	reqUsage := 0.0
	if r.RequestsPerMinute > 0 {
		reqUsage = 1.0 - float64(r.RemainingRequests)/float64(r.RequestsPerMinute)
	}
	tokUsage := 0.0
	if r.TokensPerMinute > 0 {
		tokUsage = 1.0 - float64(r.RemainingTokens)/float64(r.TokensPerMinute)
	}
	if reqUsage > tokUsage {
		return reqUsage
	}
	return tokUsage
}

// Pricing describes a provider's cost structure.
type Pricing struct {
	// InputCostPer1KTokens is the cost per 1K input tokens in USD.
	InputCostPer1KTokens float64 `json:"input_cost_per_1k_tokens"`
	// OutputCostPer1KTokens is the cost per 1K output tokens in USD.
	OutputCostPer1KTokens float64 `json:"output_cost_per_1k_tokens"`
}

// ProviderCapabilities tracks a provider's supported models, features,
// rate limits, and pricing.
type ProviderCapabilities struct {
	mu sync.RWMutex

	// models is the set of supported model names.
	models map[string]bool
	// features is the set of supported features.
	features map[Feature]bool
	// rateLimit is the current rate limit state.
	rateLimit RateLimit
	// pricing is the provider's cost structure.
	pricing Pricing
}

// NewProviderCapabilities creates a new ProviderCapabilities with the given
// models, features, rate limit, and pricing.
func NewProviderCapabilities(models []string, features []Feature, rateLimit RateLimit, pricing Pricing) *ProviderCapabilities {
	pc := &ProviderCapabilities{
		models:    make(map[string]bool, len(models)),
		features:  make(map[Feature]bool, len(features)),
		rateLimit: rateLimit,
		pricing:   pricing,
	}
	for _, m := range models {
		pc.models[strings.ToLower(m)] = true
	}
	for _, f := range features {
		pc.features[f] = true
	}
	return pc
}

// SupportsModel returns true if the provider supports the given model.
func (pc *ProviderCapabilities) SupportsModel(model string) bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.models[strings.ToLower(model)]
}

// SupportsFeature returns true if the provider supports the given feature.
func (pc *ProviderCapabilities) SupportsFeature(feature Feature) bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.features[feature]
}

// Models returns a copy of the supported model names.
func (pc *ProviderCapabilities) Models() []string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	models := make([]string, 0, len(pc.models))
	for m := range pc.models {
		models = append(models, m)
	}
	return models
}

// Features returns a copy of the supported features.
func (pc *ProviderCapabilities) Features() []Feature {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	features := make([]Feature, 0, len(pc.features))
	for f := range pc.features {
		features = append(features, f)
	}
	return features
}

// RateLimits returns the current rate limit state.
func (pc *ProviderCapabilities) RateLimits() RateLimit {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.rateLimit
}

// UpdateRateLimit updates the rate limit state.
func (pc *ProviderCapabilities) UpdateRateLimit(rl RateLimit) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.rateLimit = rl
}

// GetPricing returns the provider's pricing.
func (pc *ProviderCapabilities) GetPricing() Pricing {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.pricing
}

// AddModel adds a model to the supported set.
func (pc *ProviderCapabilities) AddModel(model string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.models[strings.ToLower(model)] = true
}

// RemoveModel removes a model from the supported set.
func (pc *ProviderCapabilities) RemoveModel(model string) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	delete(pc.models, strings.ToLower(model))
}

// String returns a human-readable summary of the capabilities.
func (pc *ProviderCapabilities) String() string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return fmt.Sprintf("models=%d features=%d rate_limit_usage=%.1f%%",
		len(pc.models), len(pc.features), pc.rateLimit.UsagePercent()*100)
}
