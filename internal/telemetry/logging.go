package telemetry

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

const (
	// requestIDKey is the context key for the request ID.
	requestIDKey contextKey = "request_id"
)

// InitLogger creates a structured JSON logger with the configured log level.
// The logger is set as the default slog logger.
func InitLogger(cfg Config) *slog.Logger {
	level := parseLogLevel(cfg.LogLevel)

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger
}

// WithRequestID returns a new context with the given request ID stored.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromContext extracts the request ID from the context.
// Returns an empty string if no request ID is present.
func RequestIDFromContext(ctx context.Context) string {
	v, ok := ctx.Value(requestIDKey).(string)
	if !ok {
		return ""
	}
	return v
}

// LoggerFromContext returns a logger enriched with the request_id from the context.
// If no request ID is present, the base logger is returned unchanged.
func LoggerFromContext(ctx context.Context, base *slog.Logger) *slog.Logger {
	reqID := RequestIDFromContext(ctx)
	if reqID == "" {
		return base
	}
	return base.With(slog.String("request_id", reqID))
}

// parseLogLevel converts a string log level to slog.Level.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
