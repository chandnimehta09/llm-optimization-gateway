package cache

import (
	"sync"
	"time"
)

type HealthState int

const (
	HealthHealthy HealthState = iota
	HealthDegraded
	HealthUnhealthy
)

func (s HealthState) String() string {
	switch s {
	case HealthHealthy:
		return "healthy"
	case HealthDegraded:
		return "degraded"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

type HealthMonitorConfig struct {
	WindowSize         int
	DegradedThreshold  float64
	UnhealthyThreshold float64
	CheckInterval      time.Duration
}

func DefaultHealthMonitorConfig() HealthMonitorConfig {
	return HealthMonitorConfig{WindowSize: 100, DegradedThreshold: 0.9, UnhealthyThreshold: 0.5}
}

type HealthSnapshot struct {
	State               HealthState   `json:"state"`
	SuccessRate         float64       `json:"success_rate"`
	TotalSuccesses      int64         `json:"total_successes"`
	TotalFailures       int64         `json:"total_failures"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
	LastSuccessTime     time.Time     `json:"last_success_time,omitempty"`
	LastFailureTime     time.Time     `json:"last_failure_time,omitempty"`
	RecoveryTime        time.Duration `json:"recovery_time,omitempty"`
	Timestamp           time.Time     `json:"timestamp"`
}

type HealthMonitor struct {
	mu                                                sync.RWMutex
	config                                            HealthMonitorConfig
	window                                            []bool
	pos, filled                                       int
	totalSuccesses, totalFailures                     int64
	consecutiveFailures                               int
	lastSuccessTime, lastFailureTime, outageStartTime time.Time
	lastRecoveryTime                                  time.Duration
	inOutage                                          bool
	stopCh, done                                      chan struct{}
}

func NewHealthMonitor(c HealthMonitorConfig) *HealthMonitor {
	if c.WindowSize <= 0 {
		c.WindowSize = 100
	}
	if c.DegradedThreshold <= 0 {
		c.DegradedThreshold = 0.9
	}
	if c.UnhealthyThreshold <= 0 {
		c.UnhealthyThreshold = 0.5
	}
	hm := &HealthMonitor{config: c, window: make([]bool, c.WindowSize), stopCh: make(chan struct{}), done: make(chan struct{})}
	if c.CheckInterval <= 0 {
		close(hm.done)
	}
	return hm
}

func (hm *HealthMonitor) Start() {
	if hm.config.CheckInterval <= 0 {
		return
	}
	go func() {
		defer close(hm.done)
		t := time.NewTicker(hm.config.CheckInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = hm.Snapshot()
			case <-hm.stopCh:
				return
			}
		}
	}()
}

func (hm *HealthMonitor) Stop() {
	select {
	case <-hm.stopCh:
	default:
		close(hm.stopCh)
	}
	<-hm.done
}

func (hm *HealthMonitor) RecordSuccess() {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.totalSuccesses++
	hm.consecutiveFailures = 0
	hm.lastSuccessTime = time.Now()
	hm.window[hm.pos] = true
	hm.pos = (hm.pos + 1) % hm.config.WindowSize
	if hm.filled < hm.config.WindowSize {
		hm.filled++
	}
	if hm.inOutage {
		hm.lastRecoveryTime = time.Since(hm.outageStartTime)
		hm.inOutage = false
	}
}

func (hm *HealthMonitor) RecordFailure() {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.totalFailures++
	hm.consecutiveFailures++
	hm.lastFailureTime = time.Now()
	hm.window[hm.pos] = false
	hm.pos = (hm.pos + 1) % hm.config.WindowSize
	if hm.filled < hm.config.WindowSize {
		hm.filled++
	}
	if !hm.inOutage {
		hm.outageStartTime = time.Now()
		hm.inOutage = true
	}
}

func (hm *HealthMonitor) successRateLocked() float64 {
	if hm.filled == 0 {
		return 1.0
	}
	s := 0
	for i := 0; i < hm.filled; i++ {
		if hm.window[i] {
			s++
		}
	}
	return float64(s) / float64(hm.filled)
}

func (hm *HealthMonitor) stateLocked() HealthState {
	r := hm.successRateLocked()
	if r < hm.config.UnhealthyThreshold {
		return HealthUnhealthy
	}
	if r < hm.config.DegradedThreshold {
		return HealthDegraded
	}
	return HealthHealthy
}

func (hm *HealthMonitor) Snapshot() HealthSnapshot {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return HealthSnapshot{State: hm.stateLocked(), SuccessRate: hm.successRateLocked(),
		TotalSuccesses: hm.totalSuccesses, TotalFailures: hm.totalFailures,
		ConsecutiveFailures: hm.consecutiveFailures, LastSuccessTime: hm.lastSuccessTime,
		LastFailureTime: hm.lastFailureTime, RecoveryTime: hm.lastRecoveryTime, Timestamp: time.Now()}
}

func (hm *HealthMonitor) State() HealthState {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.stateLocked()
}
func (hm *HealthMonitor) SuccessRate() float64 {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.successRateLocked()
}
func (hm *HealthMonitor) RecoveryTime() time.Duration {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	return hm.lastRecoveryTime
}
func (hm *HealthMonitor) IsHealthy() bool { return hm.State() == HealthHealthy }
