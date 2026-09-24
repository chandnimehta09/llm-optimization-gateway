package workloads

import (
	"fmt"

	"github.com/llm-optimization-gateway/internal/benchmark"
)

// ContextChangeGenerator produces the same prompt but changes the context
// (system prompt or prior conversation) each iteration, testing partial
// cache invalidation scenarios.
type ContextChangeGenerator struct {
	basePrompt    string
	contextPrefix string
	contextTokens int
}

// NewContextChangeGenerator creates a context change workload generator.
func NewContextChangeGenerator(cfg benchmark.WorkloadConfig) *ContextChangeGenerator {
	return &ContextChangeGenerator{
		basePrompt: paramString(cfg.Params, "base_prompt",
			"Based on the context above, what are the key improvements we should make?"),
		contextPrefix: paramString(cfg.Params, "context_prefix",
			"Project status update version"),
		contextTokens: paramInt(cfg.Params, "context_tokens", 500),
	}
}

// Name returns the workload type name.
func (g *ContextChangeGenerator) Name() string {
	return "context_change"
}

// Generate produces a request with the same user prompt but different context.
func (g *ContextChangeGenerator) Generate(iteration int) benchmark.WorkloadRequest {
	context := fmt.Sprintf("%s %d: ", g.contextPrefix, iteration)
	context = generateText(context, g.contextTokens)

	return benchmark.WorkloadRequest{
		SystemPrompt: "You are a project management assistant analyzing status updates.",
		Messages: []benchmark.WorkloadMessage{
			{Role: "user", Content: context},
			{Role: "assistant", Content: "I've reviewed the context. What would you like to know?"},
			{Role: "user", Content: g.basePrompt},
		},
	}
}
