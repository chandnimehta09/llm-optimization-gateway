package workloads

import (
	"fmt"

	"github.com/llm-optimization-gateway/internal/benchmark"
)

// StaticPrefixGenerator produces requests with a large shared static prefix
// and varying suffixes. This exercises prefix caching optimizations.
type StaticPrefixGenerator struct {
	prefixTokens int
	prefix       string
	questions    []string
}

// NewStaticPrefixGenerator creates a static prefix workload generator.
func NewStaticPrefixGenerator(cfg benchmark.WorkloadConfig) *StaticPrefixGenerator {
	prefixTokens := paramInt(cfg.Params, "prefix_tokens", 2000)
	prefix := generateText("This is the project documentation that provides context for all questions. ", prefixTokens)

	questions := []string{
		"What is the main architecture pattern used in this project?",
		"How does the caching layer work?",
		"What are the performance characteristics of the routing engine?",
		"Describe the error handling strategy.",
		"What monitoring and observability features are available?",
		"How does the circuit breaker pattern protect against failures?",
		"What are the retry policies and backoff strategies?",
		"How is request normalization implemented?",
		"Describe the policy engine and rule evaluation.",
		"What are the deployment requirements?",
	}

	return &StaticPrefixGenerator{
		prefixTokens: prefixTokens,
		prefix:       prefix,
		questions:    questions,
	}
}

// Name returns the workload type name.
func (g *StaticPrefixGenerator) Name() string {
	return "static_prefix"
}

// Generate produces a request with a large static prefix and a varying question.
func (g *StaticPrefixGenerator) Generate(iteration int) benchmark.WorkloadRequest {
	question := g.questions[iteration%len(g.questions)]

	return benchmark.WorkloadRequest{
		SystemPrompt: fmt.Sprintf("You are a documentation expert. Here is the full project documentation:\n\n%s", g.prefix),
		Messages: []benchmark.WorkloadMessage{
			{Role: "user", Content: question},
		},
	}
}
