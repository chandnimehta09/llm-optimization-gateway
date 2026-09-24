package reliability

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if err := c.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	c := DefaultRetryConfig()
	if c.MaxAttempts != 3 {
		t.Errorf("expected max_attempts=3, got %d", c.MaxAttempts)
	}
	if c.InitialDelay != 100*time.Millisecond {
		t.Errorf("expected initial_delay=100ms, got %v", c.InitialDelay)
	}
	if c.MaxDelay != 10*time.Second {
		t.Errorf("expected max_delay=10s, got %v", c.MaxDelay)
	}
	if c.Multiplier != 2.0 {
		t.Errorf("expected multiplier=2.0, got %f", c.Multiplier)
	}
	if c.JitterMode != JitterFull {
		t.Errorf("expected jitter_mode=full, got %s", c.JitterMode)
	}
}

func TestRetryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  RetryConfig
		wantErr bool
	}{
		{
			name:    "valid default",
			config:  DefaultRetryConfig(),
			wantErr: false,
		},
		{
			name:    "max_attempts zero",
			config:  RetryConfig{MaxAttempts: 0, InitialDelay: time.Second, MaxDelay: 10 * time.Second, Multiplier: 2.0, JitterMode: JitterNone},
			wantErr: true,
		},
		{
			name:    "negative initial delay",
			config:  RetryConfig{MaxAttempts: 3, InitialDelay: -1, MaxDelay: 10 * time.Second, Multiplier: 2.0, JitterMode: JitterNone},
			wantErr: true,
		},
		{
			name:    "initial_delay > max_delay",
			config:  RetryConfig{MaxAttempts: 3, InitialDelay: 20 * time.Second, MaxDelay: 10 * time.Second, Multiplier: 2.0, JitterMode: JitterNone},
			wantErr: true,
		},
		{
			name:    "multiplier < 1",
			config:  RetryConfig{MaxAttempts: 3, InitialDelay: time.Second, MaxDelay: 10 * time.Second, Multiplier: 0.5, JitterMode: JitterNone},
			wantErr: true,
		},
		{
			name:    "unknown jitter mode",
			config:  RetryConfig{MaxAttempts: 3, InitialDelay: time.Second, MaxDelay: 10 * time.Second, Multiplier: 2.0, JitterMode: "unknown"},
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

func TestCircuitBreakerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  CircuitBreakerConfig
		wantErr bool
	}{
		{
			name:    "valid default",
			config:  DefaultCircuitBreakerConfig(),
			wantErr: false,
		},
		{
			name:    "disabled skips validation",
			config:  CircuitBreakerConfig{Enabled: false},
			wantErr: false,
		},
		{
			name:    "zero failure threshold",
			config:  CircuitBreakerConfig{Enabled: true, FailureThreshold: 0, ResetTimeout: time.Second, HalfOpenMaxAttempts: 1, SuccessThreshold: 1},
			wantErr: true,
		},
		{
			name:    "zero reset timeout",
			config:  CircuitBreakerConfig{Enabled: true, FailureThreshold: 5, ResetTimeout: 0, HalfOpenMaxAttempts: 1, SuccessThreshold: 1},
			wantErr: true,
		},
		{
			name:    "success_threshold > half_open_max",
			config:  CircuitBreakerConfig{Enabled: true, FailureThreshold: 5, ResetTimeout: time.Second, HalfOpenMaxAttempts: 1, SuccessThreshold: 5},
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

func TestRetryBudgetConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  RetryBudgetConfig
		wantErr bool
	}{
		{
			name:    "valid default",
			config:  DefaultRetryBudgetConfig(),
			wantErr: false,
		},
		{
			name:    "disabled skips validation",
			config:  RetryBudgetConfig{Enabled: false},
			wantErr: false,
		},
		{
			name:    "zero max retries",
			config:  RetryBudgetConfig{Enabled: true, MaxRetries: 0, Window: time.Second},
			wantErr: true,
		},
		{
			name:    "zero window",
			config:  RetryBudgetConfig{Enabled: true, MaxRetries: 10, Window: 0},
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

func TestConfig_Validate_CompoundErrors(t *testing.T) {
	c := Config{
		Retry: RetryConfig{MaxAttempts: 0, InitialDelay: -1, Multiplier: 0.5, JitterMode: JitterNone},
		Timeout: TimeoutConfig{
			Phases: map[TimeoutPhase]time.Duration{PhaseTotal: -1},
		},
		CircuitBreaker: CircuitBreakerConfig{Enabled: true, FailureThreshold: 0, ResetTimeout: 0, HalfOpenMaxAttempts: 0, SuccessThreshold: 0},
	}
	err := c.Validate()
	if err == nil {
		t.Fatal("expected validation errors")
	}
}
