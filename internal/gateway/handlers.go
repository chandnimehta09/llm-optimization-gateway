package gateway

import (
	"encoding/json"
	"net/http"
)

// HealthResponse represents the response body for the health endpoint.
type HealthResponse struct {
	Status string `json:"status"`
}

// ReadyResponse represents the response body for the readiness endpoint.
type ReadyResponse struct {
	Ready bool `json:"ready"`
}

// registerRoutes registers all gateway HTTP routes on the given mux.
func registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/ready", handleReady)
}

// handleHealth responds with the health status of the gateway.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(HealthResponse{Status: "healthy"})
}

// handleReady responds with the readiness status of the gateway.
func handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ReadyResponse{Ready: true})
}
