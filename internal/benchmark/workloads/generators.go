// Package workloads provides workload generators for the benchmark harness.
// Each generator produces a sequence of requests that exercise different
// caching and optimization patterns.
package workloads

import (
	"fmt"

	"github.com/llm-optimization-gateway/internal/benchmark"
)

// Registry holds all available workload generators.
type Registry struct {
	generators map[string]func(cfg benchmark.WorkloadConfig) benchmark.WorkloadGenerator
}

// NewRegistry creates a new workload generator registry with all built-in generators.
func NewRegistry() *Registry {
	r := &Registry{
		generators: make(map[string]func(cfg benchmark.WorkloadConfig) benchmark.WorkloadGenerator),
	}
	r.Register("repetitive", func(cfg benchmark.WorkloadConfig) benchmark.WorkloadGenerator {
		return NewRepetitiveGenerator(cfg)
	})
	r.Register("context_change", func(cfg benchmark.WorkloadConfig) benchmark.WorkloadGenerator {
		return NewContextChangeGenerator(cfg)
	})
	r.Register("static_prefix", func(cfg benchmark.WorkloadConfig) benchmark.WorkloadGenerator {
		return NewStaticPrefixGenerator(cfg)
	})
	r.Register("repository_context", func(cfg benchmark.WorkloadConfig) benchmark.WorkloadGenerator {
		return NewRepositoryContextGenerator(cfg)
	})
	r.Register("random", func(cfg benchmark.WorkloadConfig) benchmark.WorkloadGenerator {
		return NewRandomGenerator(cfg)
	})
	return r
}

// Register adds a generator factory to the registry.
func (r *Registry) Register(name string, factory func(cfg benchmark.WorkloadConfig) benchmark.WorkloadGenerator) {
	r.generators[name] = factory
}

// Create instantiates a generator for the given workload config.
func (r *Registry) Create(cfg benchmark.WorkloadConfig) (benchmark.WorkloadGenerator, error) {
	factory, ok := r.generators[cfg.Type]
	if !ok {
		return nil, fmt.Errorf("unknown workload type: %q", cfg.Type)
	}
	return factory(cfg), nil
}

// Types returns all registered workload type names.
func (r *Registry) Types() []string {
	types := make([]string, 0, len(r.generators))
	for name := range r.generators {
		types = append(types, name)
	}
	return types
}

// paramString extracts a string parameter from the workload config params.
func paramString(params map[string]interface{}, key, defaultVal string) string {
	if params == nil {
		return defaultVal
	}
	v, ok := params[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}

// paramInt extracts an int parameter from the workload config params.
func paramInt(params map[string]interface{}, key string, defaultVal int) int {
	if params == nil {
		return defaultVal
	}
	v, ok := params[key]
	if !ok {
		return defaultVal
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	default:
		return defaultVal
	}
}

// generateText creates filler text of approximately the given token count.
// Assumes ~4 chars per token.
func generateText(prefix string, approxTokens int) string {
	targetLen := approxTokens * 4
	text := prefix
	word := "lorem "
	for len(text) < targetLen {
		text += word
	}
	return text[:min(len(text), targetLen)]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
