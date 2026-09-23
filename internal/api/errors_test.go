package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteErrorResponse_Format(t *testing.T) {
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
	if apiErr.Error.Type != "invalid_request_error" {
		t.Errorf("expected type 'invalid_request_error', got %q", apiErr.Error.Type)
	}
}

func TestWriteErrorResponse_NilParamAndCode(t *testing.T) {
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

func TestWriteErrorResponse_JSONStructure(t *testing.T) {
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
