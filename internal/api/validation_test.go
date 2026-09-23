package api

import (
	"testing"

	"github.com/llm-optimization-gateway/internal/api/models"
)

func TestValidateChatCompletionRequest_ValidRequest(t *testing.T) {
	req := &models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{{Role: "user", Content: "Hello"}},
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) != 0 {
		t.Errorf("expected no validation errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateChatCompletionRequest_MissingModel(t *testing.T) {
	req := &models.ChatCompletionRequest{
		Messages: []models.ChatMessage{{Role: "user", Content: "Hello"}},
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, err := range errs {
		if err.Field == "model" {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for 'model' field")
	}
}

func TestValidateChatCompletionRequest_EmptyMessages(t *testing.T) {
	req := &models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{},
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
}

func TestValidateChatCompletionRequest_NilMessages(t *testing.T) {
	req := &models.ChatCompletionRequest{Model: "gpt-3.5-turbo"}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
}

func TestValidateChatCompletionRequest_InvalidRole(t *testing.T) {
	req := &models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{{Role: "invalid_role", Content: "Hello"}},
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
}

func TestValidateChatCompletionRequest_MissingRole(t *testing.T) {
	req := &models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{{Content: "Hello"}},
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
}

func TestValidateChatCompletionRequest_TemperatureOutOfRange(t *testing.T) {
	tests := []struct {
		name        string
		temperature float64
		expectError bool
	}{
		{"too low", -0.1, true},
		{"too high", 2.1, true},
		{"min valid", 0, false},
		{"max valid", 2, false},
		{"mid valid", 1.0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			temp := tt.temperature
			req := &models.ChatCompletionRequest{
				Model:       "gpt-3.5-turbo",
				Messages:    []models.ChatMessage{{Role: "user", Content: "Hello"}},
				Temperature: &temp,
			}
			errs := ValidateChatCompletionRequest(req)
			hasError := false
			for _, err := range errs {
				if err.Field == "temperature" {
					hasError = true
				}
			}
			if hasError != tt.expectError {
				t.Errorf("temperature=%f: expected error=%v, got error=%v", tt.temperature, tt.expectError, hasError)
			}
		})
	}
}

func TestValidateChatCompletionRequest_TopPOutOfRange(t *testing.T) {
	tests := []struct {
		name        string
		topP        float64
		expectError bool
	}{
		{"too low", -0.1, true},
		{"too high", 1.1, true},
		{"min valid", 0, false},
		{"max valid", 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topP := tt.topP
			req := &models.ChatCompletionRequest{
				Model:    "gpt-3.5-turbo",
				Messages: []models.ChatMessage{{Role: "user", Content: "Hello"}},
				TopP:     &topP,
			}
			errs := ValidateChatCompletionRequest(req)
			hasError := false
			for _, err := range errs {
				if err.Field == "top_p" {
					hasError = true
				}
			}
			if hasError != tt.expectError {
				t.Errorf("top_p=%f: expected error=%v, got error=%v", tt.topP, tt.expectError, hasError)
			}
		})
	}
}

func TestValidateChatCompletionRequest_MaxTokensInvalid(t *testing.T) {
	maxTokens := 0
	req := &models.ChatCompletionRequest{
		Model:     "gpt-3.5-turbo",
		Messages:  []models.ChatMessage{{Role: "user", Content: "Hello"}},
		MaxTokens: &maxTokens,
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for max_tokens=0")
	}
}

func TestValidateChatCompletionRequest_NInvalid(t *testing.T) {
	n := 0
	req := &models.ChatCompletionRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []models.ChatMessage{{Role: "user", Content: "Hello"}},
		N:        &n,
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation errors for n=0")
	}
}

func TestValidateChatCompletionRequest_PresencePenaltyOutOfRange(t *testing.T) {
	pp := 2.5
	req := &models.ChatCompletionRequest{
		Model:           "gpt-3.5-turbo",
		Messages:        []models.ChatMessage{{Role: "user", Content: "Hello"}},
		PresencePenalty: &pp,
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation error for presence_penalty out of range")
	}
}

func TestValidateChatCompletionRequest_FrequencyPenaltyOutOfRange(t *testing.T) {
	fp := -2.5
	req := &models.ChatCompletionRequest{
		Model:            "gpt-3.5-turbo",
		Messages:         []models.ChatMessage{{Role: "user", Content: "Hello"}},
		FrequencyPenalty: &fp,
	}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) == 0 {
		t.Fatal("expected validation error for frequency_penalty out of range")
	}
}

func TestValidateChatCompletionRequest_MultipleErrors(t *testing.T) {
	req := &models.ChatCompletionRequest{}
	errs := ValidateChatCompletionRequest(req)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 validation errors, got %d", len(errs))
	}
}

func TestValidateChatCompletionRequest_AllValidRoles(t *testing.T) {
	for _, role := range []string{"system", "user", "assistant", "function"} {
		t.Run(role, func(t *testing.T) {
			req := &models.ChatCompletionRequest{
				Model:    "gpt-3.5-turbo",
				Messages: []models.ChatMessage{{Role: role, Content: "Hello"}},
			}
			errs := ValidateChatCompletionRequest(req)
			if len(errs) != 0 {
				t.Errorf("expected no errors for role '%s', got %v", role, errs)
			}
		})
	}
}

func TestValidationError_ErrorMethod(t *testing.T) {
	err := &ValidationError{Field: "model", Message: "model is required"}
	expected := "model: model is required"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
