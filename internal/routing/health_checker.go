package routing

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// HealthStatus represents the health state of a provider.
type HealthStatus string

const (
	// HealthStatusHealthy indicates the provider is operating normally.
	HealthStatusHealthy HealthStatus = "healthy"
	// HealthStatusDegraded indicates the provider is experiencing issues.
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusUnhealthy indicates the provider is not available.
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	// HealthStatusUnknown indicates the provider's health is not yet determined.
	HealthStatusUnknown HealthStatus = "unknown"
)

// HealthReport holds the result of a health check for a provider.
type HealthReport struct {
	// Provider is the name of the provider.
	Provider string `json:"provider"`
	// Status is the current health status.
	Status HealthStatus `json:"status"`
	// Latency is the health check response time.
	Latency time.Duration `json:"latency"`
	// LastChecked is when the health was last checked.
	LastChecked time.Time `json:"last_checked"`
	// ConsecutiveFailures is the number of consecutive check failures.
	ConsecutiveFailures int `json:"consecutive_failures"`
	// Error is the last error encountered during health check.
	Error string `json:"error,omitempty"`
}

// IsHealthy returns true if the provider is healthy or degraded.
func (hr *HealthReport) IsHealthy() bool {
	return hr.Status == HealthStatusHealthy || hr.Status == HealthStatusDegraded
}

// HealthCheckConfig configures the health checker.
type HealthCheckConfig struct {
	// Interval is the time between health checks.
	Interval time.Duration `json:"interval"`
	// Timeout is the maximum time to wait for a health check response.
	Timeout time.Duration `json:"timeout"`
	// UnhealthyThreshold is the number of consecutive failures before
	// marking a provider as unhealthy.
	UnhealthyThreshold int `json:"unhealthy_threshold"`
	// DegradedThreshold is the number of consecutive failures before
	// marking a provider as degraded.
	DegradedThreshold int `json:"degraded_threshold"`
}

// DefaultHealthCheckConfig returns a HealthCheckConfig with sensible defaults.
func DefaultHealthCheckConfig() HealthCheckConfig {
	return HealthCheckConfig{
		Interval:           30 * time.Second,
		Timeout:            5 * time.Second,
		UnhealthyThreshold: 3,
		DegradedThreshold:  1,
	}
}

// HealthChecker performs periodic health checks on registered providers
// and caches the results. It is safe for concurrent use.
type HealthChecker struct {
	mu      sync.RWMutex
	config  HealthCheckConfig
	reports map[string]*HealthReport
	checker ProviderHealthChecker
	cancel  context.CancelFunc
	done    chan struct{}
}

// ProviderHealthChecker is the interface that providers implement to
// support health checking.
type ProviderHealthChecker interface {
	// CheckHealth performs a health check on the named provider, returning
	// an error if the provider is not healthy.
	CheckHealth(ctx context.Context, provider string) error
}

// ProviderHealthCheckFunc is an adapter to allow the use of ordinary
// functions as ProviderHealthChecker.
type ProviderHealthCheckFunc func(ctx context.Context, provider string) error

// CheckHealth calls f(ctx, provider).
func (f ProviderHealthCheckFunc) CheckHealth(ctx context.Context, provider string) error {
	return f(ctx, provider)
}

// NewHealthChecker creates a new HealthChecker with the given config and checker function.
func NewHealthChecker(config HealthCheckConfig, checker ProviderHealthChecker) *HealthChecker {
	return &HealthChecker{
		config:  config,
		reports: make(map[string]*HealthReport),
		checker: checker,
	}
}

// RegisterProvider registers a provider for health checking.
func (hc *HealthChecker) RegisterProvider(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	if _, exists := hc.reports[name]; !exists {
		hc.reports[name] = &HealthReport{
			Provider: name,
			Status:   HealthStatusUnknown,
		}
	}
}

// UnregisterProvider removes a provider from health checking.
func (hc *HealthChecker) UnregisterProvider(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	delete(hc.reports, name)
}

// GetReport returns the cached health report for a provider.
func (hc *HealthChecker) GetReport(name string) (*HealthReport, bool) {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	r, ok := hc.reports[name]
	if !ok {
		return nil, false
	}
	// Return a copy.
	copy := *r
	return &copy, true
}

// GetAllReports returns health reports for all registered providers.
func (hc *HealthChecker) GetAllReports() map[string]*HealthReport {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	result := make(map[string]*HealthReport, len(hc.reports))
	for k, v := range hc.reports {
		copy := *v
		result[k] = &copy
	}
	return result
}

// IsHealthy returns whether the named provider is healthy.
func (hc *HealthChecker) IsHealthy(name string) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	r, ok := hc.reports[name]
	if !ok {
		return false
	}
	return r.IsHealthy()
}

// CheckNow performs an immediate health check on the named provider.
func (hc *HealthChecker) CheckNow(ctx context.Context, name string) *HealthReport {
	hc.mu.RLock()
	report, exists := hc.reports[name]
	hc.mu.RUnlock()

	if !exists {
		return &HealthReport{
			Provider:    name,
			Status:      HealthStatusUnknown,
			LastChecked: time.Now(),
			Error:       "provider not registered",
		}
	}

	checkCtx, cancel := context.WithTimeout(ctx, hc.config.Timeout)
	defer cancel()

	start := time.Now()
	err := hc.checker.CheckHealth(checkCtx, name)
	latency := time.Since(start)

	hc.mu.Lock()
	defer hc.mu.Unlock()

	report.LastChecked = time.Now()
	report.Latency = latency

	if err != nil {
		report.ConsecutiveFailures++
		report.Error = err.Error()
		if report.ConsecutiveFailures >= hc.config.UnhealthyThreshold {
			report.Status = HealthStatusUnhealthy
		} else if report.ConsecutiveFailures >= hc.config.DegradedThreshold {
			report.Status = HealthStatusDegraded
		}
	} else {
		report.ConsecutiveFailures = 0
		report.Error = ""
		report.Status = HealthStatusHealthy
	}

	copy := *report
	return &copy
}

// Start begins periodic health checks in the background. It is safe to
// call Start multiple times; subsequent calls are no-ops.
func (hc *HealthChecker) Start() {
	hc.mu.Lock()
	if hc.cancel != nil {
		hc.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	hc.cancel = cancel
	hc.done = make(chan struct{})
	hc.mu.Unlock()

	go hc.loop(ctx)
}

// Stop stops periodic health checks. It blocks until the check loop exits.
func (hc *HealthChecker) Stop() {
	hc.mu.Lock()
	if hc.cancel == nil {
		hc.mu.Unlock()
		return
	}
	cancel := hc.cancel
	done := hc.done
	hc.cancel = nil
	hc.mu.Unlock()

	cancel()
	<-done
}

// loop runs the periodic health check cycle.
func (hc *HealthChecker) loop(ctx context.Context) {
	defer close(hc.done)

	// Initial check on all providers.
	hc.checkAll(ctx)

	ticker := time.NewTicker(hc.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.checkAll(ctx)
		}
	}
}

// checkAll runs health checks on all registered providers.
func (hc *HealthChecker) checkAll(ctx context.Context) {
	hc.mu.RLock()
	names := make([]string, 0, len(hc.reports))
	for name := range hc.reports {
		names = append(names, name)
	}
	hc.mu.RUnlock()

	for _, name := range names {
		if ctx.Err() != nil {
			return
		}
		hc.CheckNow(ctx, name)
	}
}

// String returns a summary of all provider health states.
func (hc *HealthChecker) String() string {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	result := fmt.Sprintf("health_checker: %d providers", len(hc.reports))
	for name, report := range hc.reports {
		result += fmt.Sprintf(" [%s=%s]", name, report.Status)
	}
	return result
}
