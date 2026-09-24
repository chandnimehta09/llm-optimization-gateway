package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// providerRequest is the OpenAI-compatible request format sent to providers.
type providerRequest struct {
	Model       string            `json:"model"`
	Messages    []providerMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
}

// providerMessage is an OpenAI-compatible message.
type providerMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// providerResponse is the OpenAI-compatible response format.
type providerResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		CachedTokens        int `json:"cached_tokens,omitempty"`
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens,omitempty"`
		} `json:"prompt_tokens_details,omitempty"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// HTTPSender sends benchmark requests over HTTP to an endpoint.
type HTTPSender struct {
	endpoint      string
	apiKey        string
	model         string
	client        *http.Client
	mode          RunMode
	costPerInput  float64
	costPerOutput float64
}

// HTTPSenderConfig configures an HTTP sender.
type HTTPSenderConfig struct {
	Endpoint      string
	APIKey        string
	Model         string
	Mode          RunMode
	Timeout       time.Duration
	CostPerInput  float64
	CostPerOutput float64
	Client        *http.Client
}

// NewHTTPSender creates a new HTTP-based benchmark sender.
func NewHTTPSender(cfg HTTPSenderConfig) *HTTPSender {
	client := cfg.Client
	if client == nil {
		client = &http.Client{
			Timeout: cfg.Timeout,
		}
	}
	return &HTTPSender{
		endpoint:      cfg.Endpoint,
		apiKey:        cfg.APIKey,
		model:         cfg.Model,
		client:        client,
		mode:          cfg.Mode,
		costPerInput:  cfg.CostPerInput,
		costPerOutput: cfg.CostPerOutput,
	}
}

// Send executes a benchmark request via HTTP.
func (s *HTTPSender) Send(ctx context.Context, req WorkloadRequest) RequestResult {
	start := time.Now()
	result := RequestResult{
		Mode:      s.mode,
		Timestamp: start,
	}

	model := s.model
	if req.Model != "" {
		model = req.Model
	}

	// Build the provider request.
	msgs := make([]providerMessage, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		msgs = append(msgs, providerMessage{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, providerMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	pr := providerRequest{
		Model:       model,
		Messages:    msgs,
		MaxTokens:   256,
		Temperature: 0.0,
	}

	body, err := json.Marshal(pr)
	if err != nil {
		result.Error = fmt.Sprintf("marshal error: %v", err)
		result.Latency = time.Since(start)
		return result
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		result.Error = fmt.Sprintf("request creation error: %v", err)
		result.Latency = time.Since(start)
		return result
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		result.Error = fmt.Sprintf("request error: %v", err)
		result.Latency = time.Since(start)
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Latency = time.Since(start)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("read body error: %v", err)
		return result
	}

	var pr2 providerResponse
	if err := json.Unmarshal(respBody, &pr2); err != nil {
		result.Error = fmt.Sprintf("unmarshal error: %v", err)
		return result
	}

	if pr2.Error != nil {
		result.Error = pr2.Error.Message
		return result
	}

	result.InputTokens = pr2.Usage.PromptTokens
	result.OutputTokens = pr2.Usage.CompletionTokens
	if pr2.Usage.PromptTokensDetails != nil {
		result.CachedTokens = pr2.Usage.PromptTokensDetails.CachedTokens
	} else {
		result.CachedTokens = pr2.Usage.CachedTokens
	}

	result.Cost = float64(result.InputTokens)*s.costPerInput +
		float64(result.OutputTokens)*s.costPerOutput

	return result
}

// MockSender is a sender that returns configurable results for testing.
type MockSender struct {
	mode          RunMode
	results       []RequestResult
	callCount     int
	costPerInput  float64
	costPerOutput float64
	latency       time.Duration
	inputTokens   int
	outputTokens  int
	cachedTokens  int
}

// MockSenderConfig configures a mock sender.
type MockSenderConfig struct {
	Mode          RunMode
	CostPerInput  float64
	CostPerOutput float64
	Latency       time.Duration
	InputTokens   int
	OutputTokens  int
	CachedTokens  int
}

// NewMockSender creates a new mock sender for testing.
func NewMockSender(cfg MockSenderConfig) *MockSender {
	return &MockSender{
		mode:          cfg.Mode,
		costPerInput:  cfg.CostPerInput,
		costPerOutput: cfg.CostPerOutput,
		latency:       cfg.Latency,
		inputTokens:   cfg.InputTokens,
		outputTokens:  cfg.OutputTokens,
		cachedTokens:  cfg.CachedTokens,
	}
}

// Send returns a mock result.
func (m *MockSender) Send(_ context.Context, _ WorkloadRequest) RequestResult {
	m.callCount++
	result := RequestResult{
		Mode:         m.mode,
		Latency:      m.latency,
		InputTokens:  m.inputTokens,
		OutputTokens: m.outputTokens,
		CachedTokens: m.cachedTokens,
		StatusCode:   200,
		Timestamp:    time.Now(),
		Cost: float64(m.inputTokens)*m.costPerInput +
			float64(m.outputTokens)*m.costPerOutput,
	}
	m.results = append(m.results, result)
	return result
}

// CallCount returns the number of times Send was called.
func (m *MockSender) CallCount() int {
	return m.callCount
}

// Results returns all results produced by this mock.
func (m *MockSender) Results() []RequestResult {
	return m.results
}
