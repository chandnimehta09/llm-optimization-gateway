package models

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateChatCompletionRequest_Valid(t *testing.T) {
	req := &ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) != 0 {
		t.Errorf("expected no validation errors, got %d", len(errs))
	}
}

func TestValidateChatCompletionRequest_MissingModelDirect(t *testing.T) {
	req := &ChatCompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "Hello"}},
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
}

func TestValidateChatCompletionRequest_EmptyMessagesDirect(t *testing.T) {
	req := &ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []ChatMessage{},
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
}

func TestValidateChatCompletionRequest_AllValidRolesDirect(t *testing.T) {
	for _, role := range []string{"system", "user", "assistant", "function"} {
		t.Run(role, func(t *testing.T) {
			req := &ChatCompletionRequest{
				Model:    "gpt-3.5-turbo",
				Messages: []ChatMessage{{Role: role, Content: "Hello"}},
			}
			errs := ValidateChatCompletionRequest(req)
			if len(errs) != 0 {
				t.Errorf("expected no errors for role '%s', got %v", role, errs)
			}
		})
	}
}

func TestValidateChatCompletionRequest_TemperatureRange(t *testing.T) {
	tests := []struct {
		name        string
		temperature float64
		expectError bool
	}{
		{"too low", -0.1, true},
		{"too high", 2.1, true},
		{"min valid", 0, false},
		{"max valid", 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			temp := tt.temperature
			req := &ChatCompletionRequest{
				Model:       "gpt-3.5-turbo",
				Messages:    []ChatMessage{{Role: "user", Content: "Hello"}},
				Temperature: &temp,
			}
			errs := ValidateChatCompletionRequest(req)
			hasError := len(errs) > 0
			if hasError != tt.expectError {
				t.Errorf("expected error=%v, got error=%v", tt.expectError, hasError)
			}
		})
	}
}

func TestValidationError_ErrorMethodDirect(t *testing.T) {
	err := &ValidationError{Field: "model", Message: "model is required"}
	if err.Error() != "model: model is required" {
		t.Errorf("unexpected error string: %q", err.Error())
	}
}

func TestWriteErrorResponse_FormatDirect(t *testing.T) {
	w := httptest.NewRecorder()
	param := "model"
	code := "invalid_request_error"

	WriteErrorResponse(w, "test-req-id", http.StatusBadRequest, "model is required", "invalid_request_error", &param, &code)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}
	if reqID := resp.Header.Get("X-Request-ID"); reqID != "test-req-id" {
		t.Errorf("expected X-Request-ID 'test-req-id', got %q", reqID)
	}

	var apiErr APIError
	json.NewDecoder(resp.Body).Decode(&apiErr)
	if apiErr.Error.Message != "model is required" {
		t.Errorf("expected message 'model is required', got %q", apiErr.Error.Message)
	}
}

func TestWriteErrorResponse_NilParamAndCodeDirect(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErrorResponse(w, "test-req-id", http.StatusInternalServerError, "internal error", "server_error", nil, nil)

	var apiErr APIError
	json.NewDecoder(w.Body).Decode(&apiErr)
	if apiErr.Error.Param != nil {
		t.Errorf("expected nil param, got %v", apiErr.Error.Param)
	}
	if apiErr.Error.Code != nil {
		t.Errorf("expected nil code, got %v", apiErr.Error.Code)
	}
}

func TestWriteErrorResponse_JSONStructureDirect(t *testing.T) {
	w := httptest.NewRecorder()
	WriteErrorResponse(w, "req-123", http.StatusBadRequest, "test error", "invalid_request_error", nil, nil)

	var raw map[string]interface{}
	json.NewDecoder(w.Body).Decode(&raw)

	errObj, ok := raw["error"]
	if !ok {
		t.Fatal("response missing 'error' key")
	}
	errMap, ok := errObj.(map[string]interface{})
	if !ok {
		t.Fatal("'error' is not an object")
	}

	for _, field := range []string{"message", "type", "param", "code"} {
		if _, exists := errMap[field]; !exists {
			t.Errorf("error object missing field %q", field)
		}
	}
}
