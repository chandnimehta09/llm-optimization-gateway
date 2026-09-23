package mock

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/llm-optimization-gateway/internal/provider"
)

// Provider is a mock LLM provider that returns deterministic responses
// based on configuration. It implements the provider.Provider interface
// and is useful for testing and development without incurring LLM costs.
type Provider struct {
	cfg MockProviderConfig

	// callCount tracks the number of Complete and CompleteStream calls.
	callCount atomic.Int64

	// mu protects lastRequest.
	mu          sync.RWMutex
	lastRequest *provider.CompletionRequest

	// rng is the random source for error simulation. Protected by rngMu.
	rngMu sync.Mutex
	rng   *rand.Rand
}

// New creates a new mock Provider with the given configuration.
func New(cfg MockProviderConfig) *Provider {
	cfg = cfg.withDefaults()
	return &Provider{
		cfg: cfg,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Name returns the unique identifier for this mock provider.
func (p *Provider) Name() string {
	return p.cfg.Name
}

// Complete sends a synchronous completion request and returns the mock response.
func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	p.callCount.Add(1)
	p.mu.Lock()
	p.lastRequest = req
	p.mu.Unlock()

	// Simulate latency.
	if p.cfg.Latency > 0 {
		select {
		case <-time.After(p.cfg.Latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Simulate errors.
	if p.shouldError() {
		return nil, &provider.ProviderError{
			Provider: p.cfg.Name,
			Code:     500,
			Message:  "simulated error",
		}
	}

	return generateResponse(p.cfg, req), nil
}

// CompleteStream sends a streaming completion request and returns a channel
// of response chunks.
func (p *Provider) CompleteStream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
	p.callCount.Add(1)
	p.mu.Lock()
	p.lastRequest = req
	p.mu.Unlock()

	// Simulate latency before starting the stream.
	if p.cfg.Latency > 0 {
		select {
		case <-time.After(p.cfg.Latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Simulate errors.
	if p.shouldError() {
		return nil, &provider.ProviderError{
			Provider: p.cfg.Name,
			Code:     500,
			Message:  "simulated stream error",
		}
	}

	// Generate the full response, then split it into chunks.
	resp := generateResponse(p.cfg, req)
	content := resp.Choices[0].Message.Content
	model := resp.Model
	id := resp.ID

	ch := make(chan provider.StreamChunk)
	go func() {
		defer close(ch)

		chunkSize := p.cfg.StreamChunkSize
		firstChunk := true

		for i := 0; i < len(content); i += chunkSize {
			end := i + chunkSize
			if end > len(content) {
				end = len(content)
			}

			chunk := provider.StreamChunk{
				ID:       id,
				Model:    model,
				Provider: p.cfg.Name,
				Delta: provider.MessageDelta{
					Content: content[i:end],
				},
			}
			if firstChunk {
				chunk.Delta.Role = provider.RoleAssistant
				firstChunk = false
			}

			select {
			case ch <- chunk:
			case <-ctx.Done():
				ch <- provider.StreamChunk{
					ID:       id,
					Model:    model,
					Provider: p.cfg.Name,
					Error:    ctx.Err(),
				}
				return
			}

			// Delay between chunks.
			if p.cfg.StreamChunkDelay > 0 && end < len(content) {
				select {
				case <-time.After(p.cfg.StreamChunkDelay):
				case <-ctx.Done():
					ch <- provider.StreamChunk{
						ID:       id,
						Model:    model,
						Provider: p.cfg.Name,
						Error:    ctx.Err(),
					}
					return
				}
			}
		}

		// Send final chunk with finish reason and usage.
		finalChunk := provider.StreamChunk{
			ID:           id,
			Model:        model,
			Provider:     p.cfg.Name,
			FinishReason: provider.FinishReasonStop,
			Usage:        &resp.Usage,
		}
		select {
		case ch <- finalChunk:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// Models returns the list of model identifiers available from this provider.
func (p *Provider) Models() []string {
	return []string{p.cfg.Model}
}

// IsHealthy always returns true for the mock provider.
func (p *Provider) IsHealthy(_ context.Context) bool {
	return true
}

// APIFormat returns "openai" since the mock behaves like an OpenAI provider.
func (p *Provider) APIFormat() provider.APIFormat {
	return provider.APIFormatOpenAI
}

// CallCount returns the total number of Complete and CompleteStream calls.
func (p *Provider) CallCount() int64 {
	return p.callCount.Load()
}

// LastRequest returns the most recent CompletionRequest received.
func (p *Provider) LastRequest() *provider.CompletionRequest {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.lastRequest
}

// Reset clears the call count and last request.
func (p *Provider) Reset() {
	p.callCount.Store(0)
	p.mu.Lock()
	p.lastRequest = nil
	p.mu.Unlock()
}

// shouldError returns true if a simulated error should be triggered.
func (p *Provider) shouldError() bool {
	if p.cfg.ErrorRate <= 0 {
		return false
	}
	if p.cfg.ErrorRate >= 1.0 {
		return true
	}
	p.rngMu.Lock()
	defer p.rngMu.Unlock()
	return p.rng.Float64() < p.cfg.ErrorRate
}

// Compile-time interface satisfaction check.
var _ provider.Provider = (*Provider)(nil)

// SetErrorRate updates the error rate for the mock provider.
func (p *Provider) SetErrorRate(rate float64) {
	p.cfg.ErrorRate = rate
}

// SetLatency updates the simulated latency.
func (p *Provider) SetLatency(d time.Duration) {
	p.cfg.Latency = d
}

// SetResponseMode updates the response mode.
func (p *Provider) SetResponseMode(mode ResponseMode) {
	p.cfg.ResponseMode = mode
}

// SetFixedResponse updates the fixed response text.
func (p *Provider) SetFixedResponse(resp string) {
	p.cfg.FixedResponse = resp
}

// Config returns the current mock provider configuration.
func (p *Provider) Config() MockProviderConfig {
	return p.cfg
}

// NewWithSeed creates a mock Provider with a deterministic random seed.
// Useful for reproducible tests involving error rate simulation.
func NewWithSeed(cfg MockProviderConfig, seed int64) *Provider {
	cfg = cfg.withDefaults()
	return &Provider{
		cfg: cfg,
		rng: rand.New(rand.NewSource(seed)),
	}
}

// String returns a human-readable description of the mock provider.
func (p *Provider) String() string {
	return fmt.Sprintf("MockProvider{name=%s, model=%s, mode=%s}", p.cfg.Name, p.cfg.Model, p.cfg.ResponseMode)
}
