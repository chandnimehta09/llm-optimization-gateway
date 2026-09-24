package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestInitLogger_DefaultLevel(t *testing.T) {
	cfg := Config{LogLevel: ""}
	logger := InitLogger(cfg)
	if logger == nil {
		t.Fatal("InitLogger() returned nil")
	}
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected info level to be enabled with default config")
	}
}

func TestInitLogger_DebugLevel(t *testing.T) {
	cfg := Config{LogLevel: "debug"}
	logger := InitLogger(cfg)
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("expected debug level to be enabled")
	}
}

func TestInitLogger_ErrorLevel(t *testing.T) {
	cfg := Config{LogLevel: "error"}
	logger := InitLogger(cfg)
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected info level to be disabled when error level is set")
	}
	if !logger.Enabled(context.Background(), slog.LevelError) {
		t.Error("expected error level to be enabled")
	}
}

func TestInitLogger_WarnLevel(t *testing.T) {
	cfg := Config{LogLevel: "warn"}
	logger := InitLogger(cfg)
	if logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("expected info level to be disabled when warn level is set")
	}
	if !logger.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("expected warn level to be enabled")
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
		{"  info  ", slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLogLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestWithRequestID_And_RequestIDFromContext(t *testing.T) {
	ctx := context.Background()
	reqID := "test-request-123"

	ctx = WithRequestID(ctx, reqID)
	got := RequestIDFromContext(ctx)
	if got != reqID {
		t.Errorf("RequestIDFromContext() = %q, want %q", got, reqID)
	}
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	got := RequestIDFromContext(ctx)
	if got != "" {
		t.Errorf("RequestIDFromContext() = %q, want empty string", got)
	}
}

func TestLoggerFromContext_WithRequestID(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	base := slog.New(handler)

	ctx := WithRequestID(context.Background(), "req-abc-123")
	logger := LoggerFromContext(ctx, base)

	logger.Info("test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output as JSON: %v", err)
	}

	if logEntry["request_id"] != "req-abc-123" {
		t.Errorf("log entry request_id = %v, want %q", logEntry["request_id"], "req-abc-123")
	}
	if logEntry["msg"] != "test message" {
		t.Errorf("log entry msg = %v, want %q", logEntry["msg"], "test message")
	}
}

func TestLoggerFromContext_WithoutRequestID(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	base := slog.New(handler)

	ctx := context.Background()
	logger := LoggerFromContext(ctx, base)

	logger.Info("test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log output as JSON: %v", err)
	}

	if _, exists := logEntry["request_id"]; exists {
		t.Error("log entry should not contain request_id when context has none")
	}
}

func TestLoggerOutputIsJSON(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(handler)

	logger.Info("json test", slog.String("key", "value"))

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("log output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}

	if logEntry["key"] != "value" {
		t.Errorf("log entry key = %v, want %q", logEntry["key"], "value")
	}
}
