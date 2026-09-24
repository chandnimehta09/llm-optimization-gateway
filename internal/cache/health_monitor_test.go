package cache

import (
	"sync"
	"testing"
	"time"
)

func TestHM_Initial(t *testing.T) {
	hm := NewHealthMonitor(DefaultHealthMonitorConfig())
	if hm.State() != HealthHealthy {
		t.Error("healthy")
	}
	if hm.SuccessRate() != 1.0 {
		t.Error("1.0")
	}
}
func TestHM_AllSuccess(t *testing.T) {
	hm := NewHealthMonitor(DefaultHealthMonitorConfig())
	for i := 0; i < 10; i++ {
		hm.RecordSuccess()
	}
	if hm.SuccessRate() != 1.0 || hm.State() != HealthHealthy {
		t.Error("healthy")
	}
}
func TestHM_AllFail(t *testing.T) {
	hm := NewHealthMonitor(HealthMonitorConfig{WindowSize: 10, DegradedThreshold: 0.9, UnhealthyThreshold: 0.5})
	for i := 0; i < 10; i++ {
		hm.RecordFailure()
	}
	if hm.SuccessRate() != 0.0 || hm.State() != HealthUnhealthy {
		t.Error("unhealthy")
	}
}
func TestHM_Degraded(t *testing.T) {
	hm := NewHealthMonitor(HealthMonitorConfig{WindowSize: 10, DegradedThreshold: 0.9, UnhealthyThreshold: 0.5})
	for i := 0; i < 7; i++ {
		hm.RecordSuccess()
	}
	for i := 0; i < 3; i++ {
		hm.RecordFailure()
	}
	if hm.State() != HealthDegraded {
		t.Error("degraded")
	}
}
func TestHM_Window(t *testing.T) {
	hm := NewHealthMonitor(HealthMonitorConfig{WindowSize: 5, DegradedThreshold: 0.9, UnhealthyThreshold: 0.5})
	for i := 0; i < 5; i++ {
		hm.RecordFailure()
	}
	if hm.SuccessRate() != 0.0 {
		t.Error("0")
	}
	for i := 0; i < 5; i++ {
		hm.RecordSuccess()
	}
	if hm.SuccessRate() != 1.0 {
		t.Error("1")
	}
}
func TestHM_Snapshot(t *testing.T) {
	hm := NewHealthMonitor(DefaultHealthMonitorConfig())
	hm.RecordSuccess()
	hm.RecordSuccess()
	hm.RecordFailure()
	s := hm.Snapshot()
	if s.TotalSuccesses != 2 || s.TotalFailures != 1 || s.ConsecutiveFailures != 1 {
		t.Error("snap")
	}
}
func TestHM_Recovery(t *testing.T) {
	hm := NewHealthMonitor(DefaultHealthMonitorConfig())
	hm.RecordFailure()
	time.Sleep(50 * time.Millisecond)
	hm.RecordSuccess()
	if hm.RecoveryTime() < 50*time.Millisecond {
		t.Error("recovery")
	}
}
func TestHM_ConsecReset(t *testing.T) {
	hm := NewHealthMonitor(DefaultHealthMonitorConfig())
	hm.RecordFailure()
	hm.RecordFailure()
	hm.RecordFailure()
	if hm.Snapshot().ConsecutiveFailures != 3 {
		t.Error("3")
	}
	hm.RecordSuccess()
	if hm.Snapshot().ConsecutiveFailures != 0 {
		t.Error("0")
	}
}
func TestHM_Times(t *testing.T) {
	hm := NewHealthMonitor(DefaultHealthMonitorConfig())
	hm.RecordSuccess()
	if hm.Snapshot().LastSuccessTime.IsZero() {
		t.Error("success")
	}
	hm.RecordFailure()
	if hm.Snapshot().LastFailureTime.IsZero() {
		t.Error("failure")
	}
}
func TestHM_IsHealthy(t *testing.T) {
	hm := NewHealthMonitor(HealthMonitorConfig{WindowSize: 5, DegradedThreshold: 0.9, UnhealthyThreshold: 0.5})
	if !hm.IsHealthy() {
		t.Error("yes")
	}
	for i := 0; i < 5; i++ {
		hm.RecordFailure()
	}
	if hm.IsHealthy() {
		t.Error("no")
	}
}
func TestHM_Defaults(t *testing.T) {
	c := DefaultHealthMonitorConfig()
	if c.WindowSize != 100 || c.DegradedThreshold != 0.9 || c.UnhealthyThreshold != 0.5 {
		t.Error("defaults")
	}
}
func TestHS_String(t *testing.T) {
	if HealthHealthy.String() != "healthy" || HealthDegraded.String() != "degraded" || HealthUnhealthy.String() != "unhealthy" || HealthState(99).String() != "unknown" {
		t.Error("strings")
	}
}
func TestHM_Concurrent(t *testing.T) {
	hm := NewHealthMonitor(HealthMonitorConfig{WindowSize: 10, DegradedThreshold: 0.9, UnhealthyThreshold: 0.5})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); hm.RecordSuccess() }()
		go func() { defer wg.Done(); hm.RecordFailure() }()
		go func() { defer wg.Done(); _ = hm.Snapshot() }()
	}
	wg.Wait()
}
func TestHM_StartStop(t *testing.T) {
	hm := NewHealthMonitor(HealthMonitorConfig{WindowSize: 10, DegradedThreshold: 0.9, UnhealthyThreshold: 0.5, CheckInterval: 10 * time.Millisecond})
	hm.Start()
	time.Sleep(50 * time.Millisecond)
	hm.Stop()
	hm.Stop()
}
func TestHM_StopNoStart(t *testing.T) { NewHealthMonitor(DefaultHealthMonitorConfig()).Stop() }
func TestHM_MultiOutage(t *testing.T) {
	hm := NewHealthMonitor(DefaultHealthMonitorConfig())
	hm.RecordFailure()
	time.Sleep(30 * time.Millisecond)
	hm.RecordSuccess()
	if hm.RecoveryTime() < 30*time.Millisecond {
		t.Error("first")
	}
	hm.RecordFailure()
	time.Sleep(50 * time.Millisecond)
	hm.RecordSuccess()
	if hm.RecoveryTime() < 50*time.Millisecond {
		t.Error("second")
	}
}
