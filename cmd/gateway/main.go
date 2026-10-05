// Package main provides the entry point for the LLM Optimization Gateway server.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/llm-optimization-gateway/internal/api"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("gateway: %v", err)
	}
}

func run() error {
	port := envInt("GATEWAY_PORT", 8080)
	metricsPort := envInt("METRICS_PORT", 9090)
	readTimeout := envDuration("READ_TIMEOUT", 30*time.Second)
	writeTimeout := envDuration("WRITE_TIMEOUT", 60*time.Second)
	idleTimeout := envDuration("IDLE_TIMEOUT", 120*time.Second)
	shutdownTimeout := envDuration("SHUTDOWN_TIMEOUT", 30*time.Second)

	log.Printf("LLM Optimization Gateway version=%s build=%s", version, buildTime)
	log.Printf("Configuration: port=%d metrics_port=%d", port, metricsPort)

	// Create API router
	router := api.NewRouter()

	// Main HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      router,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// Metrics/health server
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/health", healthHandler)
	metricsMux.HandleFunc("/metrics", metricsHandler)

	metricsSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", metricsPort),
		Handler:      metricsMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Channel for fatal server errors
	errCh := make(chan error, 2)

	// Start metrics server
	go func() {
		log.Printf("Metrics server listening on :%d", metricsPort)
		if err := metricsSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("metrics server: %w", err)
		}
	}()

	// Start main server
	go func() {
		log.Printf("Gateway server listening on :%d", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("gateway server: %w", err)
		}
	}()

	// Wait for interrupt signal or fatal error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received signal %v, shutting down...", sig)
	case err := <-errCh:
		log.Printf("Server error: %v, shutting down...", err)
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	log.Printf("Shutting down with %v timeout...", shutdownTimeout)

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Gateway server shutdown error: %v", err)
	}
	if err := metricsSrv.Shutdown(ctx); err != nil {
		log.Printf("Metrics server shutdown error: %v", err)
	}

	log.Println("Gateway stopped")
	return nil
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","version":"%s"}`, version)
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "# LLM Optimization Gateway Metrics")
	fmt.Fprintln(w, "gateway_up 1")
}

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func envDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultVal
}
