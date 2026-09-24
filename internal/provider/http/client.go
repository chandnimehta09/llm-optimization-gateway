package http

import (
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// client wraps an *http.Client with logging and the provider configuration.
type client struct {
	httpClient   *http.Client
	streamClient *http.Client
	config       *Config
	logger       *log.Logger
}

// newClient creates a new HTTP client configured for connection pooling and keep-alive.
func newClient(cfg *Config, logger *log.Logger) *client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:     cfg.IdleConnTimeout,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:   true,
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}

	// Streaming client uses longer timeout and a separate transport clone
	// to avoid contention with non-streaming requests.
	streamTransport := transport.Clone()
	streamClient := &http.Client{
		Transport: streamTransport,
		Timeout:   cfg.StreamTimeout,
	}

	return &client{
		httpClient:   httpClient,
		streamClient: streamClient,
		config:       cfg,
		logger:       logger,
	}
}

// Do executes a non-streaming HTTP request and returns the response.
// The caller is responsible for closing the response body.
func (c *client) Do(req *http.Request) (*http.Response, error) {
	c.applyHeaders(req)
	c.logRequest(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	c.logResponse(resp)
	return resp, nil
}

// DoStream executes a streaming HTTP request and returns the response.
// The caller is responsible for closing the response body.
func (c *client) DoStream(req *http.Request) (*http.Response, error) {
	c.applyHeaders(req)
	c.logRequest(req)

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return nil, err
	}

	c.logResponse(resp)
	return resp, nil
}

// applyHeaders sets common headers and custom headers from config.
func (c *client) applyHeaders(req *http.Request) {
	// Set content type if not already set.
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Apply custom headers from config.
	for k, v := range c.config.CustomHeaders {
		req.Header.Set(k, v)
	}
}

// logRequest logs the HTTP request at debug level.
func (c *client) logRequest(req *http.Request) {
	if c.logger != nil {
		c.logger.Printf("[DEBUG] HTTP %s %s", req.Method, req.URL.String())
	}
}

// logResponse logs the HTTP response at debug level.
func (c *client) logResponse(resp *http.Response) {
	if c.logger != nil {
		c.logger.Printf("[DEBUG] HTTP response: %d %s", resp.StatusCode, resp.Status)
	}
}

// Close drains idle connections.
func (c *client) Close() {
	c.httpClient.CloseIdleConnections()
	c.streamClient.CloseIdleConnections()
}

// readBody reads and returns the full response body, then closes it.
func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
