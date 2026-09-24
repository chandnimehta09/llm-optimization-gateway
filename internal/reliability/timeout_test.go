package reliability

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTimeoutConfig_DefaultValues(t *testing.T) {
	c := DefaultTimeoutConfig()
	if c.Phases[PhaseTotal] != DefaultTotalTimeout {
		t.Errorf("expected total timeout %v, got %v", DefaultTotalTimeout, c.Phases[PhaseTotal])
	}
	if c.Phases[PhaseConnection] != DefaultConnectionTimeout {
		t.Errorf("expected connection timeout %v, got %v", DefaultConnectionTimeout, c.Phases[PhaseConnection])
	}
}

func TestTimeoutConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TimeoutConfig
		wantErr bool
	}{
		{
			name:    "valid default",
			config:  DefaultTimeoutConfig(),
			wantErr: false,
		},
		{
			name: "negative timeout",
			config: TimeoutConfig{
				Phases: map[TimeoutPhase]time.Duration{PhaseTotal: -1},
			},
			wantErr: true,
		},
		{
			name: "invalid phase",
			config: TimeoutConfig{
				Phases: map[TimeoutPhase]time.Duration{"invalid": time.Second},
			},
			wantErr: true,
		},
		{
			name: "negative provider override",
			config: TimeoutConfig{
				Phases: DefaultPhaseTimeouts(),
				ProviderOverrides: map[string]map[TimeoutPhase]time.Duration{
					"openai": {PhaseTotal: -1},
				},
			},
			wantErr: true,
		},
		{
			name: "negative model override",
			config: TimeoutConfig{
				Phases: DefaultPhaseTimeouts(),
				ModelOverrides: map[string]map[TimeoutPhase]time.Duration{
					"gpt-4": {PhaseTotal: -1},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestTimeoutConfig_ResolveTimeout(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: 60 * time.Second,
			PhaseRead:  30 * time.Second,
		},
		ProviderOverrides: map[string]map[TimeoutPhase]time.Duration{
			"openai": {PhaseTotal: 45 * time.Second},
		},
		ModelOverrides: map[string]map[TimeoutPhase]time.Duration{
			"gpt-4": {PhaseTotal: 90 * time.Second},
		},
	}

	tests := []struct {
		name     string
		phase    TimeoutPhase
		provider string
		model    string
		override *time.Duration
		expected time.Duration
	}{
		{
			name:     "default phase timeout",
			phase:    PhaseTotal,
			expected: 60 * time.Second,
		},
		{
			name:     "provider override",
			phase:    PhaseTotal,
			provider: "openai",
			expected: 45 * time.Second,
		},
		{
			name:     "model override takes priority over provider",
			phase:    PhaseTotal,
			provider: "openai",
			model:    "gpt-4",
			expected: 90 * time.Second,
		},
		{
			name:     "request override takes highest priority",
			phase:    PhaseTotal,
			provider: "openai",
			model:    "gpt-4",
			override: durationPtr(120 * time.Second),
			expected: 120 * time.Second,
		},
		{
			name:     "fallback to default for unmatched provider",
			phase:    PhaseTotal,
			provider: "anthropic",
			expected: 60 * time.Second,
		},
		{
			name:     "unset phase returns 0",
			phase:    PhaseDNS,
			expected: 0,
		},
		{
			name:     "provider override for non-overridden phase falls through",
			phase:    PhaseRead,
			provider: "openai",
			expected: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.ResolveTimeout(tt.phase, tt.provider, tt.model, tt.override)
			if got != tt.expected {
				t.Errorf("ResolveTimeout() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTimeoutManager_WithTimeout(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: 5 * time.Second,
		},
	}
	mgr := NewTimeoutManager(config)

	ctx, cancel, timeout := mgr.WithTimeout(context.Background(), PhaseTotal, "", "", nil)
	defer cancel()

	if timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", timeout)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected context to have deadline")
	}
	remaining := time.Until(deadline)
	if remaining > 5*time.Second || remaining < 4*time.Second {
		t.Errorf("unexpected remaining time: %v", remaining)
	}
}

func TestTimeoutManager_WithTimeout_ZeroDuration(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{},
	}
	mgr := NewTimeoutManager(config)

	ctx, cancel, timeout := mgr.WithTimeout(context.Background(), PhaseTotal, "", "", nil)
	defer cancel()

	if timeout != 0 {
		t.Errorf("expected timeout 0, got %v", timeout)
	}
	if _, ok := ctx.Deadline(); ok {
		t.Error("context should not have a deadline when timeout is 0")
	}
}

func TestTimeoutManager_WithTimeout_RespectsExistingDeadline(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: 10 * time.Second,
		},
	}
	mgr := NewTimeoutManager(config)

	// Create a context with a shorter deadline.
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer parentCancel()

	_, cancel, timeout := mgr.WithTimeout(parentCtx, PhaseTotal, "", "", nil)
	defer cancel()

	// Should use the shorter of the two.
	if timeout > 2*time.Second {
		t.Errorf("expected timeout <= 2s (parent deadline), got %v", timeout)
	}
}

func TestTimeoutManager_Execute_Success(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: time.Second,
		},
	}
	mgr := NewTimeoutManager(config)

	err := mgr.Execute(context.Background(), PhaseTotal, "test", "", nil, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mgr.TimeoutCount() != 0 {
		t.Errorf("expected 0 timeouts, got %d", mgr.TimeoutCount())
	}
}

func TestTimeoutManager_Execute_Timeout(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: 50 * time.Millisecond,
		},
	}
	mgr := NewTimeoutManager(config)

	err := mgr.Execute(context.Background(), PhaseTotal, "test-provider", "", nil, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	if err == nil {
		t.Fatal("expected timeout error")
	}

	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected TimeoutError, got %T: %v", err, err)
	}
	if timeoutErr.Provider != "test-provider" {
		t.Errorf("expected provider 'test-provider', got %s", timeoutErr.Provider)
	}
	if timeoutErr.Phase != PhaseTotal {
		t.Errorf("expected phase 'total', got %s", timeoutErr.Phase)
	}

	if mgr.TimeoutCount() != 1 {
		t.Errorf("expected 1 timeout, got %d", mgr.TimeoutCount())
	}
}

func TestTimeoutManager_Execute_NoTimeout(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{},
	}
	mgr := NewTimeoutManager(config)

	err := mgr.Execute(context.Background(), PhaseTotal, "test", "", nil, func(ctx context.Context) error {
		// Should work fine without a timeout.
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTimeoutManager_AcceptanceCriteria_30sTimeout(t *testing.T) {
	// Given provider timeout = 30ms (scaled down for testing)
	// And provider takes 40ms
	// When request is made
	// Then timeout error is returned at ~30ms
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{},
		ProviderOverrides: map[string]map[TimeoutPhase]time.Duration{
			"slow-provider": {PhaseTotal: 30 * time.Millisecond},
		},
	}
	mgr := NewTimeoutManager(config)

	start := time.Now()
	err := mgr.Execute(context.Background(), PhaseTotal, "slow-provider", "", nil, func(ctx context.Context) error {
		// Simulate a provider that takes longer than the timeout.
		select {
		case <-time.After(100 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}

	var timeoutErr *TimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("expected TimeoutError, got %T: %v", err, err)
	}

	// Should have returned around 30ms, not 100ms.
	if elapsed > 80*time.Millisecond {
		t.Errorf("timeout should have triggered around 30ms, but elapsed %v", elapsed)
	}
}

func TestTimeoutManager_Events(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: 10 * time.Millisecond,
		},
	}
	mgr := NewTimeoutManager(config)

	_ = mgr.Execute(context.Background(), PhaseTotal, "provider-a", "", nil, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	events := mgr.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Provider != "provider-a" {
		t.Errorf("expected provider 'provider-a', got %s", events[0].Provider)
	}
	if events[0].Phase != PhaseTotal {
		t.Errorf("expected phase 'total', got %s", events[0].Phase)
	}
}

func TestTimeoutManager_ClearEvents(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: 10 * time.Millisecond,
		},
	}
	mgr := NewTimeoutManager(config)

	_ = mgr.Execute(context.Background(), PhaseTotal, "provider", "", nil, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	mgr.ClearEvents()
	if len(mgr.Events()) != 0 {
		t.Error("expected 0 events after clear")
	}
}

func TestTimeoutManager_UpdateConfig(t *testing.T) {
	config := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: time.Second,
		},
	}
	mgr := NewTimeoutManager(config)

	newConfig := TimeoutConfig{
		Phases: map[TimeoutPhase]time.Duration{
			PhaseTotal: 5 * time.Second,
		},
	}
	mgr.UpdateConfig(newConfig)

	_, _, timeout := mgr.WithTimeout(context.Background(), PhaseTotal, "", "", nil)
	if timeout != 5*time.Second {
		t.Errorf("expected 5s after config update, got %v", timeout)
	}
}

func TestTimeoutPhase_IsValid(t *testing.T) {
	tests := []struct {
		phase   TimeoutPhase
		isValid bool
	}{
		{PhaseConnection, true},
		{PhaseDNS, true},
		{PhaseTLS, true},
		{PhaseRead, true},
		{PhaseWrite, true},
		{PhaseTotal, true},
		{"unknown", false},
	}

	for _, tt := range tests {
		if got := tt.phase.IsValid(); got != tt.isValid {
			t.Errorf("%s.IsValid() = %v, want %v", tt.phase, got, tt.isValid)
		}
	}
}

func TestAllPhases(t *testing.T) {
	phases := AllPhases()
	if len(phases) != 6 {
		t.Errorf("expected 6 phases, got %d", len(phases))
	}
}

func TestTimeoutEvent_String(t *testing.T) {
	e := TimeoutEvent{
		Provider:   "openai",
		Phase:      PhaseTotal,
		Configured: 30 * time.Second,
		Elapsed:    31 * time.Second,
		Timestamp:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	s := e.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
