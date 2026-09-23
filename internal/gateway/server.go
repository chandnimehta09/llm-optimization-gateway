package gateway

import (
	"context"
	"net"
	"net/http"
)

// Server wraps the HTTP server and its dependencies.
type Server struct {
	httpServer *http.Server
}

// NewServer creates a new gateway server configured to listen on the given port.
func NewServer(port string) *Server {
	mux := http.NewServeMux()

	registerRoutes(mux)

	return &Server{
		httpServer: &http.Server{
			Addr:    net.JoinHostPort("", port),
			Handler: mux,
		},
	}
}

// Start begins listening and serving HTTP requests.
// It returns http.ErrServerClosed when the server is shut down gracefully.
func (s *Server) Start() error {
	err := s.httpServer.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown gracefully shuts down the server without interrupting active connections.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the address the server is configured to listen on.
func (s *Server) Addr() string {
	return s.httpServer.Addr
}
