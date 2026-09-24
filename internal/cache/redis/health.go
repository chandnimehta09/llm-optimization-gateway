package redis

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HealthStatus represents the health state of the Redis connection.
type HealthStatus int

const (
	// HealthStatusUp indicates the Redis connection is healthy.
	HealthStatusUp HealthStatus = iota
	// HealthStatusDown indicates the Redis connection is unhealthy.
	HealthStatusDown
	// HealthStatusDegraded indicates the connection is working but with issues.
	HealthStatusDegraded
)

// String returns a human-readable representation of the health status.
func (s HealthStatus) String() string {
	switch s {
	case HealthStatusUp:
		return "up"
	case HealthStatusDown:
		return "down"
	case HealthStatusDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// HealthReport contains the result of a health check.
type HealthReport struct {
	// Status is the current health status.
	Status HealthStatus `json:"status"`
	// Latency is the round-trip time of the PING command.
	Latency time.Duration `json:"latency"`
	// Message contains details about the health check result.
	Message string `json:"message,omitempty"`
	// LastChecked is when the health check was performed.
	LastChecked time.Time `json:"last_checked"`
}

// healthChecker manages periodic health checks for the Redis connection.
type healthChecker struct {
	cache    *Cache
	interval time.Duration

	mu         sync.RWMutex
	lastReport HealthReport

	stopCh chan struct{}
	done   chan struct{}
}

// newHealthChecker creates a new health checker. If interval is zero,
// periodic checks are disabled but manual checks still work.
func newHealthChecker(c *Cache, interval time.Duration) *healthChecker {
	hc := &healthChecker{
		cache:    c,
		interval: interval,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
		lastReport: HealthReport{
			Status:      HealthStatusDown,
			Message:     "not yet checked",
			LastChecked: time.Time{},
		},
	}
	// If periodic checks are disabled, mark done immediately so Stop()
	// never blocks.
	if interval <= 0 {
		close(hc.done)
	}
	return hc
}

// Start begins periodic health checking. Does nothing if interval is zero.
func (h *healthChecker) Start() {
	if h.interval <= 0 {
		return
	}

	go func() {
		defer close(h.done)

		// Run an initial check immediately.
		h.check()

		ticker := time.NewTicker(h.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				h.check()
			case <-h.stopCh:
				return
			}
		}
	}()
}

// Stop halts periodic health checking.
func (h *healthChecker) Stop() {
	select {
	case <-h.stopCh:
		// Already stopped.
	default:
		close(h.stopCh)
	}
	<-h.done
}

// check performs a single health check and updates the last report.
func (h *healthChecker) check() {
	report := h.Check(context.Background())
	h.mu.Lock()
	h.lastReport = report
	h.mu.Unlock()
}

// Check performs a health check by sending a PING to Redis and returns
// the result. This can be called manually regardless of periodic checking.
func (h *healthChecker) Check(ctx context.Context) HealthReport {
	start := time.Now()

	result := h.cache.client.Ping(ctx)
	latency := time.Since(start)

	report := HealthReport{
		Latency:     latency,
		LastChecked: time.Now(),
	}

	if err := result.Err(); err != nil {
		report.Status = HealthStatusDown
		report.Message = fmt.Sprintf("ping failed: %v", err)
	} else {
		report.Status = HealthStatusUp
		report.Message = "ok"
	}

	return report
}

// LastReport returns the most recent health check result.
func (h *healthChecker) LastReport() HealthReport {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastReport
}
