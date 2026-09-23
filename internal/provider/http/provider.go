package http

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/llm-optimization-gateway/internal/provider"
)

// Compile-time interface satisfaction check.
var _ provider.Provider = (*Provider)(nil)

// Provider is a generic HTTP-based provider that supports both OpenAI and Gemini APIs.
type Provider struct {
	config  *Config
	client  *client
	builder RequestBuilder
	parser  ResponseParser
	logger  *log.Logger
}

// New creates a new HTTP provider with the given configuration.
func New(cfg *Config) (*Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid provider config: %w", err)
	}

	logger := log.New(log.Writer(), fmt.Sprintf("[provider:%s] ", cfg.Name), log.LstdFlags)

	builder, err := newRequestBuilder(cfg.APIFormat)
	if err != nil {
		return nil, err
	}

	parser, err := newResponseParser(cfg.APIFormat)
	if err != nil {
		return nil, err
	}

	return &Provider{
		config:  cfg,
		client:  newClient(cfg, logger),
		builder: builder,
		parser:  parser,
		logger:  logger,
	}, nil
}

// Name returns the unique identifier for this provider instance.
func (p *Provider) Name() string {
	return p.config.Name
}

// Complete sends a synchronous completion request and returns the full response.
func (p *Provider) Complete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			p.logger.Printf("[DEBUG] Retry attempt %d/%d", attempt, p.config.MaxRetries)
		}

		resp, err := p.doComplete(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Don't retry on non-transient errors.
		if provider.IsAuthenticationError(err) || provider.IsContextLengthError(err) {
			return nil, err
		}

		// Don't retry on rate limit errors (let caller handle backoff).
		if provider.IsRateLimitError(err) {
			return nil, err
		}

		// Check context cancellation.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}

	return nil, lastErr
}

// doComplete executes a single completion request without retry.
func (p *Provider) doComplete(ctx context.Context, req *provider.CompletionRequest) (*provider.CompletionResponse, error) {
	httpReq, err := p.builder.BuildCompleteRequest(p.config, req)
	if err != nil {
		return nil, &provider.ProviderError{
			Provider: p.config.Name,
			Code:     0,
			Message:  "failed to build request",
			Err:      err,
		}
	}
	httpReq = httpReq.WithContext(ctx)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{
			Provider: p.config.Name,
			Code:     0,
			Message:  "HTTP request failed",
			Err:      err,
		}
	}

	return p.parser.ParseCompletion(resp, p.config.Name)
}

// CompleteStream sends a streaming completion request and returns a channel of chunks.
func (p *Provider) CompleteStream(ctx context.Context, req *provider.CompletionRequest) (<-chan provider.StreamChunk, error) {
	httpReq, err := p.builder.BuildStreamRequest(p.config, req)
	if err != nil {
		return nil, &provider.ProviderError{
			Provider: p.config.Name,
			Code:     0,
			Message:  "failed to build stream request",
			Err:      err,
		}
	}
	httpReq = httpReq.WithContext(ctx)

	resp, err := p.client.DoStream(httpReq)
	if err != nil {
		return nil, &provider.ProviderError{
			Provider: p.config.Name,
			Code:     0,
			Message:  "HTTP stream request failed",
			Err:      err,
		}
	}

	// Check for error response.
	if resp.StatusCode != 200 {
		parseErr := p.parser.ParseError(resp, p.config.Name)
		return nil, parseErr
	}

	ch := make(chan provider.StreamChunk, 64)

	go func() {
		defer close(ch)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		// Increase buffer size for large chunks.
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			chunk, err := p.parser.ParseStreamChunk(line, p.config.Name)
			if err != nil {
				if err == io.EOF {
					return
				}
				// Send error as a chunk.
				select {
				case ch <- provider.StreamChunk{Error: err}:
				case <-ctx.Done():
					return
				}
				return
			}

			if chunk == nil {
				continue
			}

			select {
			case ch <- *chunk:
			case <-ctx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case ch <- provider.StreamChunk{
				Error: &provider.ProviderError{
					Provider: p.config.Name,
					Code:     0,
					Message:  "stream read error",
					Err:      err,
				},
			}:
			case <-ctx.Done():
			}
		}
	}()

	return ch, nil
}

// Models returns the list of model identifiers available from this provider.
func (p *Provider) Models() []string {
	return p.config.Models
}

// IsHealthy checks whether the provider is reachable by hitting a lightweight endpoint.
func (p *Provider) IsHealthy(ctx context.Context) bool {
	httpReq, err := p.builder.BuildHealthRequest(p.config)
	if err != nil {
		p.logger.Printf("[WARN] Failed to build health check request: %v", err)
		return false
	}
	httpReq = httpReq.WithContext(ctx)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		p.logger.Printf("[WARN] Health check failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	// Drain the body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode == 200
}

// APIFormat returns the native API format this provider uses.
func (p *Provider) APIFormat() provider.APIFormat {
	return p.config.APIFormat
}

// Close releases resources held by the provider.
func (p *Provider) Close() {
	p.client.Close()
}
