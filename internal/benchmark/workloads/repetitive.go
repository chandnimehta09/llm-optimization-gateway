package workloads

import (
	"github.com/llm-optimization-gateway/internal/benchmark"
)

// RepetitiveGenerator produces the exact same request every iteration,
// exercising exact cache hit paths.
type RepetitiveGenerator struct {
	systemPrompt string
	userMessage  string
}

// NewRepetitiveGenerator creates a repetitive workload generator.
func NewRepetitiveGenerator(cfg benchmark.WorkloadConfig) *RepetitiveGenerator {
	return &RepetitiveGenerator{
		systemPrompt: paramString(cfg.Params, "system_prompt",
			"You are a helpful coding assistant. Always provide clear, concise answers with code examples."),
		userMessage: paramString(cfg.Params, "user_message",
			"Explain the difference between a mutex and a semaphore in Go. Include a short code example for each."),
	}
}

// Name returns the workload type name.
func (g *RepetitiveGenerator) Name() string {
	return "repetitive"
}

// Generate returns the same request every time.
func (g *RepetitiveGenerator) Generate(_ int) benchmark.WorkloadRequest {
	return benchmark.WorkloadRequest{
		SystemPrompt: g.systemPrompt,
		Messages: []benchmark.WorkloadMessage{
			{Role: "user", Content: g.userMessage},
		},
	}
}
