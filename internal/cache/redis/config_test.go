package redis

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Mode != ModeStandalone {
		t.Errorf("Mode = %v, want ModeStandalone", cfg.Mode)
	}
	if cfg.Addr != "localhost:6379" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, "localhost:6379")
	}
	if cfg.DB != 0 {
		t.Errorf("DB = %d, want 0", cfg.DB)
	}
	if cfg.PoolSize != 10 {
		t.Errorf("PoolSize = %d, want 10", cfg.PoolSize)
	}
	if cfg.MinIdleConns != 2 {
		t.Errorf("MinIdleConns = %d, want 2", cfg.MinIdleConns)
	}
	if cfg.DialTimeout != 5*time.Second {
		t.Errorf("DialTimeout = %v, want 5s", cfg.DialTimeout)
	}
	if cfg.ReadTimeout != 3*time.Second {
		t.Errorf("ReadTimeout = %v, want 3s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != 3*time.Second {
		t.Errorf("WriteTimeout = %v, want 3s", cfg.WriteTimeout)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.KeyPrefix != "llmcache:" {
		t.Errorf("KeyPrefix = %q, want %q", cfg.KeyPrefix, "llmcache:")
	}
	if cfg.EnableCompression {
		t.Error("EnableCompression should be false by default")
	}
	if cfg.CompressionThreshold != 1024 {
		t.Errorf("CompressionThreshold = %d, want 1024", cfg.CompressionThreshold)
	}
	if cfg.HealthCheckInterval != 30*time.Second {
		t.Errorf("HealthCheckInterval = %v, want 30s", cfg.HealthCheckInterval)
	}
}

func TestConfig_Validate_Standalone(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid standalone config: Validate() error: %v", err)
	}

	cfg.Addr = ""
	if err := cfg.Validate(); err == nil {
		t.Error("standalone with empty addr: expected error")
	}
}

func TestConfig_Validate_Cluster(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeCluster
	cfg.Addrs = []string{"host1:6379", "host2:6379"}

	if err := cfg.Validate(); err != nil {
		t.Errorf("valid cluster config: Validate() error: %v", err)
	}

	cfg.Addrs = nil
	cfg.Addr = ""
	if err := cfg.Validate(); err == nil {
		t.Error("cluster with no addresses: expected error")
	}
}

func TestConfig_Validate_Sentinel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeSentinel
	cfg.SentinelMasterName = "mymaster"
	cfg.Addrs = []string{"sentinel1:26379"}

	if err := cfg.Validate(); err != nil {
		t.Errorf("valid sentinel config: Validate() error: %v", err)
	}

	cfg.SentinelMasterName = ""
	if err := cfg.Validate(); err == nil {
		t.Error("sentinel without master name: expected error")
	}

	cfg.SentinelMasterName = "mymaster"
	cfg.Addrs = nil
	cfg.Addr = ""
	if err := cfg.Validate(); err == nil {
		t.Error("sentinel without addresses: expected error")
	}
}

func TestConfig_Validate_InvalidDB(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DB = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative DB: expected error")
	}

	cfg.DB = 16
	if err := cfg.Validate(); err == nil {
		t.Error("DB > 15: expected error")
	}
}

func TestConfig_Validate_NegativePoolSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PoolSize = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative PoolSize: expected error")
	}
}

func TestConfig_Validate_NegativeCompressionThreshold(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CompressionThreshold = -1
	if err := cfg.Validate(); err == nil {
		t.Error("negative CompressionThreshold: expected error")
	}
}

func TestConfig_Validate_UnknownMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = Mode(99)
	if err := cfg.Validate(); err == nil {
		t.Error("unknown mode: expected error")
	}
}

func TestConfig_Addrs(t *testing.T) {
	cfg := Config{Addrs: []string{"a:1", "b:2"}, Addr: "c:3"}
	got := cfg.addrs()
	if len(got) != 2 || got[0] != "a:1" {
		t.Errorf("addrs() with Addrs set = %v, want [a:1, b:2]", got)
	}

	cfg2 := Config{Addr: "c:3"}
	got2 := cfg2.addrs()
	if len(got2) != 1 || got2[0] != "c:3" {
		t.Errorf("addrs() fallback to Addr = %v, want [c:3]", got2)
	}

	cfg3 := Config{}
	got3 := cfg3.addrs()
	if got3 != nil {
		t.Errorf("addrs() with no addresses = %v, want nil", got3)
	}
}

func TestMode_String(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeStandalone, "standalone"},
		{ModeCluster, "cluster"},
		{ModeSentinel, "sentinel"},
		{Mode(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}
