package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/llm-optimization-gateway/internal/policy"
)

const (
	// DefaultModelTokenLimit is the default maximum context window size (in tokens)
	// when no model-specific limit is configured.
	DefaultModelTokenLimit = 4096

	// DefaultWarningThresholdRatio is the ratio of the model limit at which a
	// warning is emitted. When estimated tokens exceed limit * ratio, a warning
	// is added to the decision.
	DefaultWarningThresholdRatio = 0.85
)

// ModelLimits maps model names to their maximum context window sizes in tokens.
var ModelLimits = map[string]int{
	"gpt-3.5-turbo":     4096,
	"gpt-3.5-turbo-16k": 16384,
	"gpt-4":             8192,
	"gpt-4-32k":         32768,
	"gpt-4-turbo":       128000,
	"gpt-4o":            128000,
	"gpt-4o-mini":       128000,
	"claude-3-opus":     200000,
	"claude-3-sonnet":   200000,
	"claude-3-haiku":    200000,
	"claude-3.5-sonnet": 200000,
	"gemini-1.5-pro":    1000000,
	"gemini-1.5-flash":  1000000,
	"gemini-pro":        32768,
}

// ContextSizeConfig configures the context size policy behaviour.
type ContextSizeConfig struct {
	// ModelTokenLimit overrides the auto-detected model limit.
	// If zero, the limit is looked up from ModelLimits or DefaultModelTokenLimit is used.
	ModelTokenLimit int `json:"model_token_limit,omitempty"`
	// WarningThresholdRatio is the fraction of the model limit that triggers a warning.
	// Must be between 0.0 and 1.0. Defaults to DefaultWarningThresholdRatio.
	WarningThresholdRatio float64 `json:"warning_threshold_ratio,omitempty"`
	// EnableDeduplication controls whether deduplication is recommended when
	// context exceeds the model limit. Defaults to true.
	EnableDeduplication bool `json:"enable_deduplication"`
}

// DefaultContextSizeConfig returns a ContextSizeConfig with sensible defaults.
func DefaultContextSizeConfig() ContextSizeConfig {
	return ContextSizeConfig{
		ModelTokenLimit:       0, // auto-detect
		WarningThresholdRatio: DefaultWarningThresholdRatio,
		EnableDeduplication:   true,
	}
}

// ContextSizeDecision contains the result of evaluating the context size policy.
type ContextSizeDecision struct {
	// OptimizationNeeded indicates whether optimization should be applied.
	OptimizationNeeded bool `json:"optimization_needed"`
	// Reasoning explains why the decision was made.
	Reasoning string `json:"reasoning"`
	// EstimatedTokens is the token estimate for the input.
	EstimatedTokens TokenEstimate `json:"estimated_tokens"`
	// ModelLimit is the resolved model token limit.
	ModelLimit int `json:"model_limit"`
	// WarningThreshold is the computed warning threshold in tokens.
	WarningThreshold int `json:"warning_threshold"`
	// TokenOverflow is how many tokens the context exceeds the model limit.
	// Zero or negative means no overflow.
	TokenOverflow int `json:"token_overflow"`
	// RecommendDeduplication is true when deduplication should be applied.
	RecommendDeduplication bool `json:"recommend_deduplication"`
	// EstimatedTokenSavings is the estimated tokens that dedup could save,
	// based on DedupStats if available.
	EstimatedTokenSavings int64 `json:"estimated_token_savings"`
	// Warnings contains non-fatal issues.
	Warnings []string `json:"warnings,omitempty"`
	// EvaluatedAt is when the decision was produced.
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// ContextSizePolicy evaluates whether a request's context size requires
// optimization. It checks estimated token counts against model limits and
// recommends strategies like deduplication to bring context within bounds.
type ContextSizePolicy struct {
	config ContextSizeConfig
}

// NewContextSizePolicy creates a new context size policy with the given configuration.
func NewContextSizePolicy(config ContextSizeConfig) *ContextSizePolicy {
	return &ContextSizePolicy{config: config}
}

// Evaluate assesses the given policy input and returns a ContextSizeDecision.
// It estimates tokens, resolves the model's limit, and determines whether
// optimization is needed.
func (p *ContextSizePolicy) Evaluate(_ context.Context, input *policy.PolicyInput) (*ContextSizeDecision, error) {
	if input == nil {
		return nil, fmt.Errorf("context size policy: input must not be nil")
	}
	if input.Request == nil {
		return nil, fmt.Errorf("context size policy: input.request must not be nil")
	}

	estimate := EstimateTokens(input.Request)
	modelLimit := p.resolveModelLimit(input.Request.Model)
	warningThreshold := p.computeWarningThreshold(modelLimit)

	decision := &ContextSizeDecision{
		EstimatedTokens:  estimate,
		ModelLimit:       modelLimit,
		WarningThreshold: warningThreshold,
		EvaluatedAt:      time.Now(),
	}

	// Calculate token overflow.
	overflow := estimate.TotalTokens - modelLimit
	if overflow > 0 {
		decision.TokenOverflow = overflow
	}

	// Calculate dedup savings if stats available.
	if input.DedupStats != nil {
		decision.EstimatedTokenSavings = input.DedupStats.EstimatedTokensSaved
	}

	// Apply decision rules.
	switch {
	case estimate.TotalTokens > modelLimit:
		decision.OptimizationNeeded = true
		decision.Reasoning = fmt.Sprintf(
			"estimated %d tokens exceeds model limit of %d tokens (overflow: %d tokens); optimization required",
			estimate.TotalTokens, modelLimit, overflow,
		)
		if p.config.EnableDeduplication {
			decision.RecommendDeduplication = true
		}

	case estimate.TotalTokens > warningThreshold:
		decision.OptimizationNeeded = false
		decision.Reasoning = fmt.Sprintf(
			"estimated %d tokens exceeds warning threshold of %d tokens (%.0f%% of %d limit); consider optimization",
			estimate.TotalTokens, warningThreshold,
			p.resolveWarningRatio()*100, modelLimit,
		)
		decision.Warnings = append(decision.Warnings, fmt.Sprintf(
			"context size at %.1f%% of model limit (%d/%d tokens)",
			float64(estimate.TotalTokens)/float64(modelLimit)*100,
			estimate.TotalTokens, modelLimit,
		))
		// Recommend dedup proactively when nearing limit.
		if p.config.EnableDeduplication {
			decision.RecommendDeduplication = true
		}

	default:
		decision.OptimizationNeeded = false
		decision.Reasoning = fmt.Sprintf(
			"estimated %d tokens is within model limit of %d tokens; no optimization needed",
			estimate.TotalTokens, modelLimit,
		)
	}

	return decision, nil
}

// ApplyToDecision integrates the context size evaluation result into a
// PolicyDecision, setting deduplication flags, warnings, and audit entries.
func (p *ContextSizePolicy) ApplyToDecision(csDecision *ContextSizeDecision, policyDecision *policy.PolicyDecision) {
	if csDecision == nil || policyDecision == nil {
		return
	}

	if csDecision.RecommendDeduplication {
		policyDecision.ApplyDeduplication = true
	}

	for _, w := range csDecision.Warnings {
		policyDecision.AddWarning(w)
	}

	effect := "no_action"
	if csDecision.OptimizationNeeded {
		effect = "optimize_context"
	} else if len(csDecision.Warnings) > 0 {
		effect = "warn"
	}

	policyDecision.AddAuditEntry(
		"context-size-check",
		"Context Size Policy",
		csDecision.OptimizationNeeded,
		effect,
		csDecision.Reasoning,
	)
}

// resolveModelLimit determines the token limit for the given model.
func (p *ContextSizePolicy) resolveModelLimit(model string) int {
	// Explicit config override takes highest priority.
	if p.config.ModelTokenLimit > 0 {
		return p.config.ModelTokenLimit
	}

	// Look up known model limits.
	if limit, ok := ModelLimits[model]; ok {
		return limit
	}

	return DefaultModelTokenLimit
}

// resolveWarningRatio returns the warning threshold ratio, using the default if unset.
func (p *ContextSizePolicy) resolveWarningRatio() float64 {
	if p.config.WarningThresholdRatio > 0 && p.config.WarningThresholdRatio <= 1.0 {
		return p.config.WarningThresholdRatio
	}
	return DefaultWarningThresholdRatio
}

// computeWarningThreshold calculates the absolute token count at which
// a warning should be issued.
func (p *ContextSizePolicy) computeWarningThreshold(modelLimit int) int {
	ratio := p.resolveWarningRatio()
	return int(float64(modelLimit) * ratio)
}
