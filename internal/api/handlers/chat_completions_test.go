package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/llm-optimization-gateway/internal/api/models"
)

func TestChatCompletionsHandler_ValidRequest(t *testing.T) {
	handler := NewChatCompletionsHandler()

	reqBody := models.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []models.ChatMessage{
			{Role: "user", Content: "Hello, world!"},
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	reqID := resp.Header.Get("X-Request-ID")
	if reqID == "" {
		t.Error("expected X-Request-ID header to be present")
	}

	var chatResp models.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !strings.HasPrefix(chatResp.ID, "chatcmpl-") {
		t.Errorf("expected ID to start with 'chatcmpl-', got %q", chatResp.ID)
	}
	if chatResp.Object != "chat.completion" {
		t.Errorf("expected object 'chat.completion', got %q", chatResp.Object)
	}
	if chatResp.Created == 0 {
		t.Error("expected created timestamp to be non-zero")
	}
	if chatResp.Model != "gpt-3.5-turbo" {
		t.Errorf("expected model 'gpt-3.5-turbo', got %q", chatResp.Model)
	}
	if len(chatResp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(chatResp.Choices))
	}
	if chatResp.Choices[0].Index != 0 {
		t.Errorf("expected choice index 0, got %d", chatResp.Choices[0].Index)
	}
	if chatResp.Choices[0].Message.Role != "assistant" {
		t.Errorf("expected message role 'assistant', got %q", chatResp.Choices[0].Message.Role)
	}
	if chatResp.Choices[0].Message.Content == "" {
		t.Error("expected message content to be non-empty")
	}
	if chatResp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", chatResp.Choices[0].FinishReason)
	}
	if chatResp.Usage.PromptTokens == 0 {
		t.Error("expected prompt_tokens > 0")
	}
	if chatResp.Usage.CompletionTokens == 0 {
		t.Error("expected completion_tokens > 0")
	}
	if chatResp.Usage.TotalTokens != chatResp.Usage.PromptTokens+chatResp.Usage.CompletionTokens {
		t.Error("expected total_tokens = prompt_tokens + completion_tokens")
	}
}

func TestChatCompletionsHandler_MissingModel(t *testing.T) {
	handler := NewChatCompletionsHandler()

	reqBody := models.ChatCompletionRequest{
		Messages: []models.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	reqID := resp.Header.Get("X-Request-ID")
	if reqID == "" {
		t.Error("expected X-Request-ID header to be present on error response")
	}

	var apiErr struct {
		Error struct {
			Message string  `json:"message"`
			Type    string  `json:"type"`
			Param   *string `json:"param"`
			Code    *string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}

	if apiErr.Error.Message == "" {
		t.Error("expected error message to be non-empty")
	}
	if apiErr.Error.Type != "invalid_request_error" {
		t.Errorf("expected error type 'invalid_request_error', got %q", apiErr.Error.Type)
	}
}

func TestChatCompletionsHandler_EmptyMessages(t *testing.T) {
	handler := NewChatCompletionsHandler()

	reqBody := models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Result().StatusCode)
	}
}

func TestChatCompletionsHandler_InvalidJSON(t *testing.T) {
	handler := NewChatCompletionsHandler()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("not json"))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Result().StatusCode)
	}

	var apiErr struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	json.NewDecoder(w.Body).Decode(&apiErr)
	if apiErr.Error.Type != "invalid_request_error" {
		t.Errorf("expected error type 'invalid_request_error', got %q", apiErr.Error.Type)
	}
}

func TestChatCompletionsHandler_MethodNotAllowed(t *testing.T) {
	handler := NewChatCompletionsHandler()

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/v1/chat/completions", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Result().StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("expected status 405 for %s, got %d", method, w.Result().StatusCode)
			}
		})
	}
}

func TestChatCompletionsHandler_StreamingNotSupported(t *testing.T) {
	handler := NewChatCompletionsHandler()

	reqBody := models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{{Role: "user", Content: "Hello"}},
		Stream:   true,
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusNotImplemented {
		t.Errorf("expected status 501 for streaming, got %d", w.Result().StatusCode)
	}
}

func TestChatCompletionsHandler_ResponseIDUniqueness(t *testing.T) {
	handler := NewChatCompletionsHandler()

	reqBody := models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{{Role: "user", Content: "Hello"}},
	}

	ids := make(map[string]bool)
	for i := 0; i < 10; i++ {
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		var chatResp models.ChatCompletionResponse
		json.NewDecoder(w.Body).Decode(&chatResp)
		if ids[chatResp.ID] {
			t.Errorf("duplicate response ID: %s", chatResp.ID)
		}
		ids[chatResp.ID] = true
	}
}

func TestChatCompletionsHandler_InvalidRole(t *testing.T) {
	handler := NewChatCompletionsHandler()

	reqBody := models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{{Role: "invalid", Content: "Hello"}},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Result().StatusCode)
	}
}

func TestChatCompletionsHandler_MultipleMessages(t *testing.T) {
	handler := NewChatCompletionsHandler()

	reqBody := models.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []models.ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "What is 1+1?"},
			{Role: "assistant", Content: "2"},
			{Role: "user", Content: "And 2+2?"},
		},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Result().StatusCode)
	}

	var chatResp models.ChatCompletionResponse
	json.NewDecoder(w.Body).Decode(&chatResp)
	if chatResp.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got %q", chatResp.Model)
	}
}

func TestIntegration_ChatCompletions_ValidRequest(t *testing.T) {
	handler := NewChatCompletionsHandler()
	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	reqBody := models.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []models.ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Say hello"},
		},
	}

	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header")
	}

	var chatResp models.ChatCompletionResponse
	json.NewDecoder(resp.Body).Decode(&chatResp)

	if !strings.HasPrefix(chatResp.ID, "chatcmpl-") {
		t.Errorf("expected ID prefix 'chatcmpl-', got %q", chatResp.ID)
	}
	if chatResp.Object != "chat.completion" {
		t.Errorf("expected object 'chat.completion', got %q", chatResp.Object)
	}
	if chatResp.Model != "gpt-3.5-turbo" {
		t.Errorf("expected model 'gpt-3.5-turbo', got %q", chatResp.Model)
	}
	if len(chatResp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	if chatResp.Usage.TotalTokens == 0 {
		t.Error("expected total_tokens > 0")
	}
}

func TestIntegration_ChatCompletions_InvalidRequest(t *testing.T) {
	handler := NewChatCompletionsHandler()
	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	reqBody := models.ChatCompletionRequest{
		Messages: []models.ChatMessage{{Role: "user", Content: "Hello"}},
	}

	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	var apiErr struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&apiErr)

	if apiErr.Error.Type != "invalid_request_error" {
		t.Errorf("expected error type 'invalid_request_error', got %q", apiErr.Error.Type)
	}
	if apiErr.Error.Message == "" {
		t.Error("expected non-empty error message")
	}
}

func TestIntegration_ChatCompletions_ErrorResponseFormat(t *testing.T) {
	handler := NewChatCompletionsHandler()
	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", handler)
	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}

	var raw map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&raw)

	errObj, ok := raw["error"]
	if !ok {
		t.Fatal("error response missing top-level 'error' key")
	}
	errMap, ok := errObj.(map[string]interface{})
	if !ok {
		t.Fatal("'error' is not an object")
	}

	for _, field := range []string{"message", "type", "param", "code"} {
		if _, exists := errMap[field]; !exists {
			t.Errorf("error object missing required field %q", field)
		}
	}

	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header on error response")
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
}
