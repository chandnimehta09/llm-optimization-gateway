package workloads

import (
	"fmt"
	"math/rand"

	"github.com/llm-optimization-gateway/internal/benchmark"
)

// RandomGenerator produces random requests with varying prompts, contexts,
// and message counts. This serves as a baseline workload where caching
// optimizations provide minimal benefit.
type RandomGenerator struct {
	rng         *rand.Rand
	maxMessages int
	maxTokens   int
	topics      []string
}

// NewRandomGenerator creates a random workload generator.
func NewRandomGenerator(cfg benchmark.WorkloadConfig) *RandomGenerator {
	seed := int64(paramInt(cfg.Params, "seed", 42))
	return &RandomGenerator{
		rng:         rand.New(rand.NewSource(seed)),
		maxMessages: paramInt(cfg.Params, "max_messages", 5),
		maxTokens:   paramInt(cfg.Params, "max_tokens", 200),
		topics: []string{
			"machine learning",
			"distributed systems",
			"database optimization",
			"API design",
			"security best practices",
			"cloud architecture",
			"microservices",
			"testing strategies",
			"performance tuning",
			"deployment automation",
			"data pipelines",
			"real-time processing",
			"containerization",
			"monitoring and alerting",
			"code review practices",
		},
	}
}

// Name returns the workload type name.
func (g *RandomGenerator) Name() string {
	return "random"
}

// Generate produces a random request.
func (g *RandomGenerator) Generate(iteration int) benchmark.WorkloadRequest {
	topic := g.topics[g.rng.Intn(len(g.topics))]
	numMessages := 1 + g.rng.Intn(g.maxMessages)
	tokens := 50 + g.rng.Intn(g.maxTokens)

	messages := make([]benchmark.WorkloadMessage, 0, numMessages)
	for i := 0; i < numMessages; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		content := generateText(
			fmt.Sprintf("Iteration %d, message %d about %s: ", iteration, i, topic),
			tokens/(numMessages),
		)
		messages = append(messages, benchmark.WorkloadMessage{
			Role:    role,
			Content: content,
		})
	}

	// Ensure the last message is from the user.
	if len(messages) > 0 && messages[len(messages)-1].Role != "user" {
		messages = append(messages, benchmark.WorkloadMessage{
			Role:    "user",
			Content: fmt.Sprintf("Tell me more about %s in the context of iteration %d.", topic, iteration),
		})
	}

	systemPrompts := []string{
		"You are a helpful technical assistant.",
		"You are an expert software architect.",
		"You are a senior developer reviewing code.",
		"You are a DevOps engineer troubleshooting issues.",
		"You are a data scientist analyzing results.",
	}

	return benchmark.WorkloadRequest{
		SystemPrompt: systemPrompts[g.rng.Intn(len(systemPrompts))],
		Messages:     messages,
	}
}
