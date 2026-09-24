package benchmark

import (
	"context"
	"testing"
	"time"
)

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("DefaultConfig should be valid: %v", err)
	}
}

func TestConfig_Validate_MissingName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Name = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestConfig_Validate_MissingProvider(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProviderEndpoint = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing provider endpoint")
	}
}

func TestConfig_Validate_MissingGateway(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GatewayEndpoint = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing gateway endpoint")
	}
}

func TestConfig_Validate_MissingModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing model")
	}
}

func TestConfig_Validate_NoConcurrency(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Concurrency = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero concurrency")
	}
}

func TestConfig_Validate_NoIterations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Iterations = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero iterations")
	}
}

func TestConfig_Validate_NoWorkloads(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workloads = nil
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for no workloads")
	}
}

func TestConfig_Validate_NegativeTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Timeout = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative timeout")
	}
}

func TestConfig_Validate_NegativeCostPerInputToken(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CostPerInputToken = -0.001
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative cost per input token")
	}
}

func TestConfig_Validate_WorkloadMissingType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workloads = []WorkloadConfig{{Type: "", Weight: 1.0}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for workload missing type")
	}
}

func TestConfig_Validate_WorkloadNegativeWeight(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workloads = []WorkloadConfig{{Type: "test", Weight: -1.0}}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for workload negative weight")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Name == "" {
		t.Error("default config should have a name")
	}
	if cfg.Concurrency < 1 {
		t.Error("default config should have concurrency >= 1")
	}
	if cfg.Iterations < 1 {
		t.Error("default config should have iterations >= 1")
	}
	if len(cfg.Workloads) == 0 {
		t.Error("default config should have workloads")
	}
}

func TestNewHarness_NilConfig(t *testing.T) {
	_, err := NewHarness(nil, &MockSender{}, &MockSender{})
	if err == nil {
		t.Error("expected error for nil config")
	}
}

func TestNewHarness_NilBaseline(t *testing.T) {
	_, err := NewHarness(DefaultConfig(), nil, &MockSender{})
	if err == nil {
		t.Error("expected error for nil baseline")
	}
}

func TestNewHarness_NilGateway(t *testing.T) {
	_, err := NewHarness(DefaultConfig(), &MockSender{}, nil)
	if err == nil {
		t.Error("expected error for nil gateway")
	}
}

func TestNewHarness_Valid(t *testing.T) {
	h, err := NewHarness(DefaultConfig(), &MockSender{}, &MockSender{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h == nil {
		t.Error("expected non-nil harness")
	}
}

func TestHarness_RegisterGenerator(t *testing.T) {
	h, _ := NewHarness(DefaultConfig(), &MockSender{}, &MockSender{})
	gen := &testGenerator{name: "test"}
	h.RegisterGenerator(gen)

	if _, ok := h.generators["test"]; !ok {
		t.Error("generator not registered")
	}
}

func TestHarness_Run_MissingGenerator(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workloads = []WorkloadConfig{{Type: "nonexistent", Weight: 1.0}}
	h, _ := NewHarness(cfg, &MockSender{}, &MockSender{})

	_, err := h.Run(context.Background())
	if err == nil {
		t.Error("expected error for missing generator")
	}
}

func TestHarness_Run_Success(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Iterations = 3
	cfg.WarmupIterations = 1
	cfg.CooldownBetweenWorkloads = 0
	cfg.Workloads = []WorkloadConfig{{Type: "test", Weight: 1.0}}

	baseline := NewMockSender(MockSenderConfig{
		Mode:          RunModeBaseline,
		Latency:       10 * time.Millisecond,
		InputTokens:   100,
		OutputTokens:  50,
		CostPerInput:  0.00003,
		CostPerOutput: 0.00006,
	})
	gateway := NewMockSender(MockSenderConfig{
		Mode:          RunModeGateway,
		Latency:       5 * time.Millisecond,
		InputTokens:   100,
		OutputTokens:  50,
		CachedTokens:  80,
		CostPerInput:  0.00003,
		CostPerOutput: 0.00006,
	})

	h, err := NewHarness(cfg, baseline, gateway)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h.RegisterGenerator(&testGenerator{name: "test"})

	results, err := h.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results.WorkloadResults) != 1 {
		t.Errorf("expected 1 workload result, got %d", len(results.WorkloadResults))
	}

	wr := results.WorkloadResults[0]
	if len(wr.BaselineResults) != 3 {
		t.Errorf("expected 3 baseline results, got %d", len(wr.BaselineResults))
	}
	if len(wr.GatewayResults) != 3 {
		t.Errorf("expected 3 gateway results, got %d", len(wr.GatewayResults))
	}

	// Warmup (1) + measured (3) = 4 calls per sender.
	if baseline.CallCount() != 4 {
		t.Errorf("expected 4 baseline calls, got %d", baseline.CallCount())
	}
	if gateway.CallCount() != 4 {
		t.Errorf("expected 4 gateway calls, got %d", gateway.CallCount())
	}
}

func TestHarness_Run_Concurrent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Iterations = 6
	cfg.WarmupIterations = 0
	cfg.Concurrency = 3
	cfg.CooldownBetweenWorkloads = 0
	cfg.Workloads = []WorkloadConfig{{Type: "test", Weight: 1.0}}

	baseline := NewMockSender(MockSenderConfig{
		Mode:    RunModeBaseline,
		Latency: 1 * time.Millisecond,
	})
	gateway := NewMockSender(MockSenderConfig{
		Mode:    RunModeGateway,
		Latency: 1 * time.Millisecond,
	})

	h, _ := NewHarness(cfg, baseline, gateway)
	h.RegisterGenerator(&testGenerator{name: "test"})

	results, err := h.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wr := results.WorkloadResults[0]
	if len(wr.BaselineResults) != 6 {
		t.Errorf("expected 6 baseline results, got %d", len(wr.BaselineResults))
	}
	if len(wr.GatewayResults) != 6 {
		t.Errorf("expected 6 gateway results, got %d", len(wr.GatewayResults))
	}
}

func TestHarness_Run_ContextCancelled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Iterations = 100
	cfg.WarmupIterations = 0
	cfg.CooldownBetweenWorkloads = 1 * time.Second
	cfg.Workloads = []WorkloadConfig{
		{Type: "test", Weight: 1.0},
		{Type: "test2", Weight: 1.0},
	}

	baseline := NewMockSender(MockSenderConfig{Mode: RunModeBaseline})
	gateway := NewMockSender(MockSenderConfig{Mode: RunModeGateway})

	h, _ := NewHarness(cfg, baseline, gateway)
	h.RegisterGenerator(&testGenerator{name: "test"})
	h.RegisterGenerator(&testGenerator{name: "test2"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := h.Run(ctx)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestMockSender_Send(t *testing.T) {
	m := NewMockSender(MockSenderConfig{
		Mode:          RunModeBaseline,
		Latency:       10 * time.Millisecond,
		InputTokens:   100,
		OutputTokens:  50,
		CachedTokens:  30,
		CostPerInput:  0.00003,
		CostPerOutput: 0.00006,
	})

	req := WorkloadRequest{
		SystemPrompt: "test",
		Messages:     []WorkloadMessage{{Role: "user", Content: "hello"}},
	}

	result := m.Send(context.Background(), req)
	if result.Mode != RunModeBaseline {
		t.Errorf("expected baseline mode, got %s", result.Mode)
	}
	if result.Latency != 10*time.Millisecond {
		t.Errorf("expected 10ms latency, got %v", result.Latency)
	}
	if result.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", result.InputTokens)
	}
	if result.CachedTokens != 30 {
		t.Errorf("expected 30 cached tokens, got %d", result.CachedTokens)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected 200 status, got %d", result.StatusCode)
	}
	if m.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", m.CallCount())
	}

	expectedCost := 100*0.00003 + 50*0.00006
	if result.Cost != expectedCost {
		t.Errorf("expected cost %f, got %f", expectedCost, result.Cost)
	}
}

func TestHarness_Run_ResultsHaveTimestamps(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Iterations = 1
	cfg.WarmupIterations = 0
	cfg.CooldownBetweenWorkloads = 0
	cfg.Workloads = []WorkloadConfig{{Type: "test", Weight: 1.0}}

	baseline := NewMockSender(MockSenderConfig{Mode: RunModeBaseline})
	gateway := NewMockSender(MockSenderConfig{Mode: RunModeGateway})

	h, _ := NewHarness(cfg, baseline, gateway)
	h.RegisterGenerator(&testGenerator{name: "test"})

	results, err := h.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if results.StartTime.IsZero() {
		t.Error("start time should be set")
	}
	if results.EndTime.IsZero() {
		t.Error("end time should be set")
	}
	if results.Duration <= 0 {
		t.Error("duration should be positive")
	}
	if results.Config != cfg {
		t.Error("config should be preserved in results")
	}
}

// testGenerator is a simple generator for testing.
type testGenerator struct {
	name string
}

func (g *testGenerator) Name() string { return g.name }

func (g *testGenerator) Generate(iteration int) WorkloadRequest {
	return WorkloadRequest{
		SystemPrompt: "You are a test assistant.",
		Messages: []WorkloadMessage{
			{Role: "user", Content: "test message"},
		},
	}
}
