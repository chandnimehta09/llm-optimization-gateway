package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

// Config holds configuration for the telemetry subsystem.
type Config struct {
	// ServiceName is the name used to identify this service in traces and metrics.
	ServiceName string

	// ServiceVersion is the version string reported in telemetry data.
	ServiceVersion string

	// OTLPEndpoint is the OTLP collector endpoint for trace export (e.g. "localhost:4317").
	// If empty, a no-op tracer provider is used.
	OTLPEndpoint string

	// LogLevel sets the minimum log level. Valid values: "debug", "info", "warn", "error".
	LogLevel string

	// MetricsPath is the HTTP path for the Prometheus scrape endpoint. Defaults to "/metrics".
	MetricsPath string
}

// Telemetry holds all initialized telemetry components.
type Telemetry struct {
	Logger         *slog.Logger
	Metrics        *Metrics
	TracerShutdown ShutdownFunc
	MetricsHandler http.Handler
}

// Init initializes all telemetry components: tracing, metrics, and logging.
// It returns a Telemetry instance and a combined shutdown function.
func Init(ctx context.Context, cfg Config) (*Telemetry, ShutdownFunc, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "llm-optimization-gateway"
	}
	if cfg.MetricsPath == "" {
		cfg.MetricsPath = "/metrics"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

	// Initialize tracing.
	tracerShutdown, err := InitTracing(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("telemetry: init tracing: %w", err)
	}

	// Initialize metrics.
	metrics, metricsHandler, err := InitMetrics(cfg)
	if err != nil {
		_ = tracerShutdown(ctx)
		return nil, nil, fmt.Errorf("telemetry: init metrics: %w", err)
	}

	// Initialize logger.
	logger := InitLogger(cfg)

	t := &Telemetry{
		Logger:         logger,
		Metrics:        metrics,
		TracerShutdown: tracerShutdown,
		MetricsHandler: metricsHandler,
	}

	combined := CombineShutdownFuncs(tracerShutdown)

	logger.Info("telemetry initialized",
		slog.String("service", cfg.ServiceName),
		slog.String("otlp_endpoint", cfg.OTLPEndpoint),
		slog.String("log_level", cfg.LogLevel),
	)

	return t, combined, nil
}
