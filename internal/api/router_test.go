package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/llm-optimization-gateway/internal/api/models"
)

func TestNewRouter_ChatCompletionsRoute(t *testing.T) {
	router := NewRouter()

	reqBody := models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{{Role: "user", Content: "Hello"}},
	}

	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for /v1/chat/completions, got %d", w.Result().StatusCode)
	}

	var chatResp models.ChatCompletionResponse
	json.NewDecoder(w.Body).Decode(&chatResp)

	if !strings.HasPrefix(chatResp.ID, "chatcmpl-") {
		t.Errorf("expected ID prefix 'chatcmpl-', got %q", chatResp.ID)
	}
}
