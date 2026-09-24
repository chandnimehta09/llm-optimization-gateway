package prefix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ChatMessage is a single message in the vLLM chat completions request.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest is the request body for the vLLM /v1/chat/completions endpoint.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	Stop        []string      `json:"stop,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

// PromptTokensDetails contains details about prompt token processing.
type PromptTokensDetails struct {
	CachedTokens int64 `json:"cached_tokens"`
}

// UsageInfo contains token usage information from the vLLM response.
type UsageInfo struct {
	PromptTokens        int64                `json:"prompt_tokens"`
	CompletionTokens    int64                `json:"completion_tokens"`
	TotalTokens         int64                `json:"total_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// ChatCompletionChoice represents a single choice in the response.
type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// ChatCompletionResponse is the response body from the vLLM /v1/chat/completions endpoint.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   UsageInfo              `json:"usage"`
}

// CachedTokens returns the number of tokens served from the prefix cache.
// Returns 0 if the response does not include cached token details.
func (r *ChatCompletionResponse) CachedTokens() int64 {
	if r.Usage.PromptTokensDetails == nil {
		return 0
	}
	return r.Usage.PromptTokensDetails.CachedTokens
}

// PrefixCacheHitRate returns the fraction of prompt tokens that were cached.
// Returns 0 if there are no prompt tokens.
func (r *ChatCompletionResponse) PrefixCacheHitRate() float64 {
	if r.Usage.PromptTokens == 0 {
		return 0
	}
	return float64(r.CachedTokens()) / float64(r.Usage.PromptTokens)
}

// Client is an HTTP client for communicating with the vLLM server.
type Client struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
	maxRetries int
}

// NewClient creates a new vLLM client with the given configuration.
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("prefix client: config must not be nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: cfg.resolveTimeout(),
		},
		baseURL:    cfg.BaseURL,
		apiKey:     cfg.APIKey,
		maxRetries: cfg.resolveMaxRetries(),
	}, nil
}

// NewClientWithHTTP creates a Client using the provided http.Client.
// This is useful for testing with custom transports.
func NewClientWithHTTP(httpClient *http.Client, baseURL, apiKey string, maxRetries int) *Client {
	if maxRetries <= 0 {
		maxRetries = 2
	}
	return &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
		apiKey:     apiKey,
		maxRetries: maxRetries,
	}
}

// ChatCompletion sends a chat completion request to the vLLM server and
// returns the parsed response. It retries transient failures up to
// maxRetries times.
func (c *Client) ChatCompletion(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("prefix client: request must not be nil")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("prefix client: failed to marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Simple exponential backoff: 100ms, 200ms, 400ms, ...
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(100<<uint(attempt-1)) * time.Millisecond):
			}
		}

		resp, err := c.doRequest(ctx, body)
		if err != nil {
			lastErr = err
			// Retry on transient errors.
			if isRetryable(err) {
				continue
			}
			return nil, err
		}
		return resp, nil
	}

	return nil, fmt.Errorf("prefix client: all %d attempts failed: %w", c.maxRetries+1, lastErr)
}

// doRequest executes a single HTTP request to the vLLM chat completions endpoint.
func (c *Client) doRequest(ctx context.Context, body []byte) (*ChatCompletionResponse, error) {
	url := c.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("prefix client: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("prefix client: request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("prefix client: failed to read response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, &APIError{
			StatusCode: httpResp.StatusCode,
			Body:       string(respBody),
		}
	}

	var result ChatCompletionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("prefix client: failed to parse response: %w", err)
	}

	return &result, nil
}

// APIError represents a non-2xx response from the vLLM server.
type APIError struct {
	StatusCode int
	Body       string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("vllm api error (status %d): %s", e.StatusCode, e.Body)
}

// IsRetryable returns true if the API error represents a transient failure.
func (e *APIError) IsRetryable() bool {
	return e.StatusCode == 429 || e.StatusCode >= 500
}

// isRetryable checks whether an error is transient and worth retrying.
func isRetryable(err error) bool {
	var apiErr *APIError
	if isAPIError(err, &apiErr) {
		return apiErr.IsRetryable()
	}
	// Context errors are not retryable.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Other network errors are potentially retryable.
	return true
}

// isAPIError extracts an *APIError from an error chain.
func isAPIError(err error, target **APIError) bool {
	return errors.As(err, target)
}
