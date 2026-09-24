package routing

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/llm-optimization-gateway/internal/policy"
	"github.com/llm-optimization-gateway/internal/request"
)

// --- Mock Provider ---

type mockProvider struct {
	name      string
	sendFunc  func(ctx context.Context, req *request.NormalizedRequest) (*ProviderResponse, error)
	callCount int
	mu        sync.Mutex
}

func newMockProvider(name string, sendFunc func(ctx context.Context, req *request.NormalizedRequest) (*ProviderResponse, error)) *mockProvider {
	return &mockProvider{name: name, sendFunc: sendFunc}
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Send(ctx context.Context, req *request.NormalizedRequest) (*ProviderResponse, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	return m.sendFunc(ctx, req)
}
func (m *mockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func successResponse(provider string) func(context.Context, *request.NormalizedRequest) (*ProviderResponse, error) {
	return func(_ context.Context, _ *request.NormalizedRequest) (*ProviderResponse, error) {
		return &ProviderResponse{
			StatusCode:   200,
			Body:         []byte(`{"choices":[{"message":{"content":"hello"}}]}`),
			Provider:     provider,
			Model:        "gpt-4",
			InputTokens:  10,
			OutputTokens: 5,
		}, nil
	}
}

func errorResponse(msg string) func(context.Context, *request.NormalizedRequest) (*ProviderResponse, error) {
	return func(_ context.Context, _ *request.NormalizedRequest) (*ProviderResponse, error) {
		return nil, fmt.Errorf("%s", msg)
	}
}

func serverErrorResponse(provider string, statusCode int) func(context.Context, *request.NormalizedRequest) (*ProviderResponse, error) {
	return func(_ context.Context, _ *request.NormalizedRequest) (*ProviderResponse, error) {
		return &ProviderResponse{
			StatusCode: statusCode,
			Body:       []byte(`{"error":"server error"}`),
			Provider:   provider,
		}, nil
	}
}

func testRequest() *request.NormalizedRequest {
	return &request.NormalizedRequest{
		ID:    "test-req-001",
		Model: "gpt-4",
		Messages: []request.Message{
			{Role: "user", Content: "hello"},
		},
		InputFormat: request.InputFormatOpenAI,
		CreatedAt:   time.Now(),
	}
}

func testDecision(provider string) *policy.PolicyDecision {
	return &policy.PolicyDecision{
		SelectedProvider: provider,
		SelectedModel:    "gpt-4",
		PolicyVersion:    "1.0.0",
		Confidence:       1.0,
		EvaluatedAt:      time.Now(),
	}
}

// --- Capabilities Tests ---

func TestNewProviderCapabilities(t *testing.T) {
	caps := NewProviderCapabilities(
		[]string{"gpt-4", "gpt-3.5-turbo"},
		[]Feature{FeatureStreaming, FeatureTools},
		RateLimit{RequestsPerMinute: 100, TokensPerMinute: 10000, RemainingRequests: 50, RemainingTokens: 5000},
		Pricing{InputCostPer1KTokens: 0.03, OutputCostPer1KTokens: 0.06},
	)

	if !caps.SupportsModel("gpt-4") {
		t.Error("expected gpt-4 support")
	}
	if !caps.SupportsModel("GPT-4") {
		t.Error("expected case-insensitive model lookup")
	}
	if caps.SupportsModel("claude-3") {
		t.Error("should not support claude-3")
	}
	if !caps.SupportsFeature(FeatureStreaming) {
		t.Error("expected streaming support")
	}
	if caps.SupportsFeature(FeatureVision) {
		t.Error("should not support vision")
	}
	if len(caps.Models()) != 2 {
		t.Errorf("expected 2 models, got %d", len(caps.Models()))
	}
	if len(caps.Features()) != 2 {
		t.Errorf("expected 2 features, got %d", len(caps.Features()))
	}
}

func TestRateLimit_HasCapacity(t *testing.T) {
	rl := RateLimit{RequestsPerMinute: 100, TokensPerMinute: 10000, RemainingRequests: 50, RemainingTokens: 5000}
	if !rl.HasCapacity() {
		t.Error("expected capacity")
	}
	rl.RemainingRequests = 0
	if rl.HasCapacity() {
		t.Error("expected no capacity with 0 remaining requests")
	}
}

func TestRateLimit_UsagePercent(t *testing.T) {
	rl := RateLimit{RequestsPerMinute: 100, TokensPerMinute: 10000, RemainingRequests: 50, RemainingTokens: 5000}
	usage := rl.UsagePercent()
	if usage != 0.5 {
		t.Errorf("expected 0.5 usage, got %f", usage)
	}

	empty := RateLimit{}
	if empty.UsagePercent() != 0 {
		t.Error("expected 0 usage for empty rate limit")
	}
}

func TestProviderCapabilities_AddRemoveModel(t *testing.T) {
	caps := NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{})
	caps.AddModel("gpt-4")
	if !caps.SupportsModel("gpt-4") {
		t.Error("expected gpt-4 after add")
	}
	caps.RemoveModel("gpt-4")
	if caps.SupportsModel("gpt-4") {
		t.Error("should not support gpt-4 after remove")
	}
}

func TestProviderCapabilities_UpdateRateLimit(t *testing.T) {
	caps := NewProviderCapabilities(nil, nil, RateLimit{RemainingRequests: 100, RemainingTokens: 100}, Pricing{})
	rl := caps.RateLimits()
	if rl.RemainingRequests != 100 {
		t.Errorf("expected 100, got %d", rl.RemainingRequests)
	}
	caps.UpdateRateLimit(RateLimit{RemainingRequests: 50, RemainingTokens: 50})
	rl = caps.RateLimits()
	if rl.RemainingRequests != 50 {
		t.Errorf("expected 50, got %d", rl.RemainingRequests)
	}
}

func TestProviderCapabilities_String(t *testing.T) {
	caps := NewProviderCapabilities([]string{"gpt-4"}, []Feature{FeatureStreaming}, RateLimit{}, Pricing{})
	s := caps.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

// --- Registry Tests ---

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := NewRegistry(nil)
	prov := newMockProvider("openai", successResponse("openai"))
	caps := NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{})

	if err := reg.Register(prov, caps); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	got, err := reg.Get("openai")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Name() != "openai" {
		t.Errorf("expected openai, got %s", got.Name())
	}
}

func TestRegistry_RegisterNilProvider(t *testing.T) {
	reg := NewRegistry(nil)
	err := reg.Register(nil, nil)
	if err == nil {
		t.Error("expected error for nil provider")
	}
}

func TestRegistry_RegisterNilCapabilities(t *testing.T) {
	reg := NewRegistry(nil)
	prov := newMockProvider("openai", successResponse("openai"))
	err := reg.Register(prov, nil)
	if err == nil {
		t.Error("expected error for nil capabilities")
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Error("expected error for missing provider")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	reg := NewRegistry(nil)
	prov := newMockProvider("openai", successResponse("openai"))
	caps := NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{})
	_ = reg.Register(prov, caps)

	reg.Unregister("openai")
	_, err := reg.Get("openai")
	if err == nil {
		t.Error("expected error after unregister")
	}
}

func TestRegistry_ListProviders(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("azure", successResponse("azure")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	providers := reg.ListProviders()
	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}

func TestRegistry_FindProvidersForModel(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(
		newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}),
	)
	_ = reg.Register(
		newMockProvider("azure", successResponse("azure")),
		NewProviderCapabilities([]string{"gpt-4", "gpt-3.5"}, nil, RateLimit{}, Pricing{}),
	)

	providers := reg.FindProvidersForModel("gpt-4")
	if len(providers) != 2 {
		t.Errorf("expected 2 providers for gpt-4, got %d", len(providers))
	}

	providers = reg.FindProvidersForModel("gpt-3.5")
	if len(providers) != 1 {
		t.Errorf("expected 1 provider for gpt-3.5, got %d", len(providers))
	}
}

func TestRegistry_FindProvidersWithFeature(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(
		newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities(nil, []Feature{FeatureStreaming, FeatureTools}, RateLimit{}, Pricing{}),
	)
	_ = reg.Register(
		newMockProvider("azure", successResponse("azure")),
		NewProviderCapabilities(nil, []Feature{FeatureStreaming}, RateLimit{}, Pricing{}),
	)

	providers := reg.FindProvidersWithFeature(FeatureStreaming)
	if len(providers) != 2 {
		t.Errorf("expected 2 providers for streaming, got %d", len(providers))
	}

	providers = reg.FindProvidersWithFeature(FeatureTools)
	if len(providers) != 1 {
		t.Errorf("expected 1 provider for tools, got %d", len(providers))
	}
}

func TestRegistry_IsHealthy_NoHealthChecker(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(
		newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}),
	)

	if !reg.IsHealthy("openai") {
		t.Error("expected healthy without health checker")
	}
	if reg.IsHealthy("nonexistent") {
		t.Error("expected unhealthy for missing provider")
	}
}

func TestRegistry_GetCapabilities(t *testing.T) {
	reg := NewRegistry(nil)
	caps := NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{})
	_ = reg.Register(newMockProvider("openai", successResponse("openai")), caps)

	got, err := reg.GetCapabilities("openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.SupportsModel("gpt-4") {
		t.Error("expected gpt-4 support in capabilities")
	}

	_, err = reg.GetCapabilities("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent provider capabilities")
	}
}

func TestRegistry_Size(t *testing.T) {
	reg := NewRegistry(nil)
	if reg.Size() != 0 {
		t.Error("expected 0 size")
	}
	_ = reg.Register(newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	if reg.Size() != 1 {
		t.Error("expected 1 size")
	}
}

func TestRegistry_String(t *testing.T) {
	reg := NewRegistry(nil)
	s := reg.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

// --- Health Checker Tests ---

func TestHealthChecker_BasicFlow(t *testing.T) {
	checkCounts := make(map[string]int)
	checker := ProviderHealthCheckFunc(func(_ context.Context, provider string) error {
		checkCounts[provider]++
		return nil
	})

	hc := NewHealthChecker(DefaultHealthCheckConfig(), checker)
	hc.RegisterProvider("openai")

	report := hc.CheckNow(context.Background(), "openai")
	if report.Status != HealthStatusHealthy {
		t.Errorf("expected healthy, got %s", report.Status)
	}
	if !report.IsHealthy() {
		t.Error("expected IsHealthy() true")
	}
}

func TestHealthChecker_ConsecutiveFailures(t *testing.T) {
	config := HealthCheckConfig{
		Interval:           time.Hour,
		Timeout:            time.Second,
		DegradedThreshold:  1,
		UnhealthyThreshold: 3,
	}
	checker := ProviderHealthCheckFunc(func(_ context.Context, _ string) error {
		return fmt.Errorf("connection refused")
	})

	hc := NewHealthChecker(config, checker)
	hc.RegisterProvider("openai")

	// First failure -> degraded.
	report := hc.CheckNow(context.Background(), "openai")
	if report.Status != HealthStatusDegraded {
		t.Errorf("expected degraded after 1 failure, got %s", report.Status)
	}
	if report.ConsecutiveFailures != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", report.ConsecutiveFailures)
	}

	// Second failure -> still degraded.
	report = hc.CheckNow(context.Background(), "openai")
	if report.Status != HealthStatusDegraded {
		t.Errorf("expected degraded after 2 failures, got %s", report.Status)
	}

	// Third failure -> unhealthy.
	report = hc.CheckNow(context.Background(), "openai")
	if report.Status != HealthStatusUnhealthy {
		t.Errorf("expected unhealthy after 3 failures, got %s", report.Status)
	}
}

func TestHealthChecker_Recovery(t *testing.T) {
	failCount := 0
	checker := ProviderHealthCheckFunc(func(_ context.Context, _ string) error {
		failCount++
		if failCount <= 3 {
			return fmt.Errorf("error")
		}
		return nil
	})

	config := HealthCheckConfig{
		Interval:           time.Hour,
		Timeout:            time.Second,
		DegradedThreshold:  1,
		UnhealthyThreshold: 3,
	}
	hc := NewHealthChecker(config, checker)
	hc.RegisterProvider("openai")

	// Drive to unhealthy.
	for i := 0; i < 3; i++ {
		hc.CheckNow(context.Background(), "openai")
	}
	report, _ := hc.GetReport("openai")
	if report.Status != HealthStatusUnhealthy {
		t.Fatalf("expected unhealthy, got %s", report.Status)
	}

	// Recover.
	report = hc.CheckNow(context.Background(), "openai")
	if report.Status != HealthStatusHealthy {
		t.Errorf("expected healthy after recovery, got %s", report.Status)
	}
	if report.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 failures after recovery, got %d", report.ConsecutiveFailures)
	}
}

func TestHealthChecker_UnregisteredProvider(t *testing.T) {
	checker := ProviderHealthCheckFunc(func(_ context.Context, _ string) error { return nil })
	hc := NewHealthChecker(DefaultHealthCheckConfig(), checker)

	report := hc.CheckNow(context.Background(), "nonexistent")
	if report.Status != HealthStatusUnknown {
		t.Errorf("expected unknown, got %s", report.Status)
	}
}

func TestHealthChecker_GetAllReports(t *testing.T) {
	checker := ProviderHealthCheckFunc(func(_ context.Context, _ string) error { return nil })
	hc := NewHealthChecker(DefaultHealthCheckConfig(), checker)
	hc.RegisterProvider("openai")
	hc.RegisterProvider("azure")

	hc.CheckNow(context.Background(), "openai")
	hc.CheckNow(context.Background(), "azure")

	reports := hc.GetAllReports()
	if len(reports) != 2 {
		t.Errorf("expected 2 reports, got %d", len(reports))
	}
}

func TestHealthChecker_StartStop(t *testing.T) {
	checker := ProviderHealthCheckFunc(func(_ context.Context, _ string) error { return nil })
	config := HealthCheckConfig{
		Interval:           50 * time.Millisecond,
		Timeout:            time.Second,
		UnhealthyThreshold: 3,
		DegradedThreshold:  1,
	}
	hc := NewHealthChecker(config, checker)
	hc.RegisterProvider("openai")

	hc.Start()
	// Wait for at least one check cycle.
	time.Sleep(100 * time.Millisecond)
	hc.Stop()

	report, ok := hc.GetReport("openai")
	if !ok {
		t.Fatal("expected report for openai")
	}
	if report.Status != HealthStatusHealthy {
		t.Errorf("expected healthy after check cycle, got %s", report.Status)
	}
}

func TestHealthChecker_String(t *testing.T) {
	checker := ProviderHealthCheckFunc(func(_ context.Context, _ string) error { return nil })
	hc := NewHealthChecker(DefaultHealthCheckConfig(), checker)
	s := hc.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

// --- Routing Context Tests ---

func TestRoutingContext_RecordAttempt(t *testing.T) {
	rc := NewRoutingContext(testRequest(), testDecision("openai"))

	if rc.AttemptCount() != 0 {
		t.Error("expected 0 attempts initially")
	}

	rc.RecordAttempt(ProviderAttempt{
		Provider:      "openai",
		AttemptNumber: 1,
		StartTime:     time.Now(),
		EndTime:       time.Now(),
	})

	if rc.AttemptCount() != 1 {
		t.Error("expected 1 attempt")
	}

	last := rc.LastAttempt()
	if last == nil || last.Provider != "openai" {
		t.Error("expected last attempt to be openai")
	}
}

func TestRoutingContext_AllErrors(t *testing.T) {
	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	rc.RecordAttempt(ProviderAttempt{Provider: "openai", Error: fmt.Errorf("err1")})
	rc.RecordAttempt(ProviderAttempt{Provider: "azure"}) // no error
	rc.RecordAttempt(ProviderAttempt{Provider: "gcp", Error: fmt.Errorf("err2")})

	errs := rc.AllErrors()
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}
}

func TestRoutingContext_Finish(t *testing.T) {
	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	if !rc.EndTime.IsZero() {
		t.Error("expected zero end time before finish")
	}

	rc.Finish(nil)
	if rc.EndTime.IsZero() {
		t.Error("expected non-zero end time after finish")
	}
	if rc.FinalError != nil {
		t.Error("expected nil final error")
	}
}

func TestRoutingContext_Metadata(t *testing.T) {
	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	rc.SetMetadata("key", "value")

	v, ok := rc.GetMetadata("key")
	if !ok || v != "value" {
		t.Error("expected metadata key=value")
	}

	_, ok = rc.GetMetadata("missing")
	if ok {
		t.Error("expected missing key to return false")
	}
}

func TestRoutingContext_Summary(t *testing.T) {
	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	s := rc.Summary()
	if s == "" {
		t.Error("expected non-empty summary")
	}
}

func TestWithRoutingContext(t *testing.T) {
	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	ctx := WithRoutingContext(context.Background(), rc)

	got := RoutingContextFromContext(ctx)
	if got != rc {
		t.Error("expected same routing context from context")
	}

	got = RoutingContextFromContext(context.Background())
	if got != nil {
		t.Error("expected nil from empty context")
	}
}

// --- Priority Tests ---

func TestPriorityList_Ordered(t *testing.T) {
	pl := NewPriorityList([]ProviderPriority{
		{Provider: "azure", Priority: 2},
		{Provider: "openai", Priority: 1},
		{Provider: "gcp", Priority: 3},
	})

	ordered := pl.Ordered()
	if len(ordered) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(ordered))
	}
	if ordered[0] != "openai" || ordered[1] != "azure" || ordered[2] != "gcp" {
		t.Errorf("unexpected order: %v", ordered)
	}
}

func TestPriorityList_SetPriority(t *testing.T) {
	pl := NewPriorityList([]ProviderPriority{
		{Provider: "openai", Priority: 1},
		{Provider: "azure", Priority: 2},
	})

	pl.SetPriority("azure", 0)
	ordered := pl.Ordered()
	if ordered[0] != "azure" {
		t.Errorf("expected azure first, got %s", ordered[0])
	}

	// Add new provider.
	pl.SetPriority("gcp", 3)
	if pl.Size() != 3 {
		t.Errorf("expected 3 providers, got %d", pl.Size())
	}
}

func TestPriorityList_Remove(t *testing.T) {
	pl := NewPriorityList([]ProviderPriority{
		{Provider: "openai", Priority: 1},
		{Provider: "azure", Priority: 2},
	})

	pl.Remove("openai")
	if pl.Size() != 1 {
		t.Errorf("expected 1 provider, got %d", pl.Size())
	}
}

func TestPriorityList_NextAfter(t *testing.T) {
	pl := NewPriorityList([]ProviderPriority{
		{Provider: "openai", Priority: 1},
		{Provider: "azure", Priority: 2},
		{Provider: "gcp", Priority: 3},
	})

	next, ok := pl.NextAfter("openai")
	if !ok || next != "azure" {
		t.Errorf("expected azure after openai, got %s", next)
	}

	_, ok = pl.NextAfter("gcp")
	if ok {
		t.Error("expected no next after last provider")
	}

	_, ok = pl.NextAfter("nonexistent")
	if ok {
		t.Error("expected no next for nonexistent provider")
	}
}

func TestPriorityList_GetPriority(t *testing.T) {
	pl := NewPriorityList([]ProviderPriority{
		{Provider: "openai", Priority: 1},
	})

	if pl.GetPriority("openai") != 1 {
		t.Error("expected priority 1 for openai")
	}
	if pl.GetPriority("nonexistent") != -1 {
		t.Error("expected -1 for nonexistent")
	}
}

// --- FallbackChain Tests ---

func TestFallbackChain_ShouldFallback(t *testing.T) {
	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{{Provider: "openai", Priority: 1}}),
		DefaultFallbackChainConfig(),
	)

	if chain.ShouldFallback(nil, 200) {
		t.Error("should not fallback on 200")
	}
	if !chain.ShouldFallback(fmt.Errorf("error"), 0) {
		t.Error("should fallback on error")
	}
	if !chain.ShouldFallback(nil, 500) {
		t.Error("should fallback on 500")
	}
	if !chain.ShouldFallback(nil, 429) {
		t.Error("should fallback on 429")
	}
	if !chain.ShouldFallback(nil, 503) {
		t.Error("should fallback on 503")
	}
}

func TestFallbackChain_ShouldFallback_OnlyRateLimit(t *testing.T) {
	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{{Provider: "openai", Priority: 1}}),
		FallbackChainConfig{
			MaxAttempts: 3,
			Conditions:  []FallbackCondition{FallbackOnRateLimit},
		},
	)

	// Not a rate limit error — should not fallback.
	if chain.ShouldFallback(nil, 500) {
		t.Error("should not fallback on 500 with only rate limit condition")
	}
	// Rate limit error — should fallback.
	if !chain.ShouldFallback(nil, 429) {
		t.Error("should fallback on 429")
	}
}

func TestFallbackChain_MaxAttempts(t *testing.T) {
	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{
			{Provider: "a", Priority: 1},
			{Provider: "b", Priority: 2},
			{Provider: "c", Priority: 3},
			{Provider: "d", Priority: 4},
		}),
		FallbackChainConfig{MaxAttempts: 2, Conditions: []FallbackCondition{FallbackOnError}},
	)

	if chain.MaxAttempts() != 2 {
		t.Errorf("expected max attempts 2, got %d", chain.MaxAttempts())
	}
}

func TestFallbackChain_MaxAttempts_ExceedsChainLength(t *testing.T) {
	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{
			{Provider: "a", Priority: 1},
		}),
		FallbackChainConfig{MaxAttempts: 10, Conditions: []FallbackCondition{FallbackOnError}},
	)

	if chain.MaxAttempts() != 1 {
		t.Errorf("expected max attempts capped at 1, got %d", chain.MaxAttempts())
	}
}

func TestFallbackChain_String(t *testing.T) {
	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{{Provider: "openai", Priority: 1}}),
		DefaultFallbackChainConfig(),
	)
	s := chain.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

// --- Fallback Executor Tests ---

func TestFallbackExecutor_PrimarySuccess(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{{Provider: "openai", Priority: 1}}),
		DefaultFallbackChainConfig(),
	)

	var events []FallbackEvent
	exec := NewFallbackExecutor(reg, chain, func(e FallbackEvent) {
		events = append(events, e)
	})

	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	result, err := exec.Execute(context.Background(), rc, "openai")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FinalProvider != "openai" {
		t.Errorf("expected openai, got %s", result.FinalProvider)
	}
	if result.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", result.Attempts)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 fallback events, got %d", len(events))
	}
}

func TestFallbackExecutor_FallbackToSecondProvider(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", errorResponse("openai down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("azure", successResponse("azure")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{
			{Provider: "openai", Priority: 1},
			{Provider: "azure", Priority: 2},
		}),
		DefaultFallbackChainConfig(),
	)

	var events []FallbackEvent
	exec := NewFallbackExecutor(reg, chain, func(e FallbackEvent) {
		events = append(events, e)
	})

	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	result, err := exec.Execute(context.Background(), rc, "openai")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FinalProvider != "azure" {
		t.Errorf("expected azure, got %s", result.FinalProvider)
	}
	if result.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", result.Attempts)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 fallback event, got %d", len(events))
	}
	if events[0].FromProvider != "openai" || events[0].ToProvider != "azure" {
		t.Errorf("unexpected fallback event: %+v", events[0])
	}
}

func TestFallbackExecutor_AllProvidersFail(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", errorResponse("openai down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("azure", errorResponse("azure down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{
			{Provider: "openai", Priority: 1},
			{Provider: "azure", Priority: 2},
		}),
		DefaultFallbackChainConfig(),
	)

	exec := NewFallbackExecutor(reg, chain, nil)
	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	result, err := exec.Execute(context.Background(), rc, "openai")

	if err == nil {
		t.Fatal("expected error when all providers fail")
	}

	allErr, ok := err.(*AllProvidersFailedError)
	if !ok {
		t.Fatalf("expected AllProvidersFailedError, got %T", err)
	}
	if len(allErr.Errors) != 2 {
		t.Errorf("expected 2 provider errors, got %d", len(allErr.Errors))
	}
	if result.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", result.Attempts)
	}
}

func TestFallbackExecutor_MaxAttemptsRespected(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("a", errorResponse("a down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("b", errorResponse("b down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("c", successResponse("c")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{
			{Provider: "a", Priority: 1},
			{Provider: "b", Priority: 2},
			{Provider: "c", Priority: 3},
		}),
		FallbackChainConfig{MaxAttempts: 2, Conditions: []FallbackCondition{FallbackOnError}},
	)

	exec := NewFallbackExecutor(reg, chain, nil)
	rc := NewRoutingContext(testRequest(), testDecision("a"))
	_, err := exec.Execute(context.Background(), rc, "a")

	if err == nil {
		t.Fatal("expected error when max attempts exhausted before success")
	}
	if rc.AttemptCount() != 2 {
		t.Errorf("expected 2 attempts, got %d", rc.AttemptCount())
	}
}

func TestFallbackExecutor_ServerErrorFallback(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", serverErrorResponse("openai", 500)),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("azure", successResponse("azure")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{
			{Provider: "openai", Priority: 1},
			{Provider: "azure", Priority: 2},
		}),
		DefaultFallbackChainConfig(),
	)

	exec := NewFallbackExecutor(reg, chain, nil)
	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	result, err := exec.Execute(context.Background(), rc, "openai")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FinalProvider != "azure" {
		t.Errorf("expected azure, got %s", result.FinalProvider)
	}
}

func TestFallbackExecutor_ContextCancellation(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", func(_ context.Context, _ *request.NormalizedRequest) (*ProviderResponse, error) {
		time.Sleep(100 * time.Millisecond)
		return nil, fmt.Errorf("slow")
	}), NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	chain := NewFallbackChain(
		NewPriorityList([]ProviderPriority{{Provider: "openai", Priority: 1}}),
		DefaultFallbackChainConfig(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	exec := NewFallbackExecutor(reg, chain, nil)
	rc := NewRoutingContext(testRequest(), testDecision("openai"))
	_, err := exec.Execute(ctx, rc, "openai")

	if err == nil {
		t.Error("expected error on context cancellation")
	}
}

// --- Provider Error Tests ---

func TestProviderError_Error(t *testing.T) {
	pe := ProviderError{Provider: "openai", Message: "failed", StatusCode: 500}
	expected := "provider openai: status=500: failed"
	if pe.Error() != expected {
		t.Errorf("expected %q, got %q", expected, pe.Error())
	}

	pe2 := ProviderError{Provider: "openai", Message: "failed"}
	expected2 := "provider openai: failed"
	if pe2.Error() != expected2 {
		t.Errorf("expected %q, got %q", expected2, pe2.Error())
	}
}

func TestProviderError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	pe := ProviderError{Provider: "openai", Err: inner, Message: "failed"}
	if pe.Unwrap() != inner {
		t.Error("expected unwrap to return inner error")
	}
}

func TestAllProvidersFailedError(t *testing.T) {
	err := &AllProvidersFailedError{
		Errors: []ProviderError{
			{Provider: "openai", Message: "down"},
			{Provider: "azure", Message: "timeout"},
		},
	}
	s := err.Error()
	if s == "" {
		t.Error("expected non-empty error string")
	}
}

// --- Executor Tests ---

func TestExecutor_Execute(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	exec := NewExecutor(reg, DefaultExecutorConfig())

	resp, err := exec.Execute(context.Background(), "openai", testRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != "openai" {
		t.Errorf("expected openai, got %s", resp.Provider)
	}
}

func TestExecutor_Execute_ProviderNotFound(t *testing.T) {
	reg := NewRegistry(nil)
	exec := NewExecutor(reg, DefaultExecutorConfig())

	_, err := exec.Execute(context.Background(), "nonexistent", testRequest())
	if err == nil {
		t.Error("expected error for missing provider")
	}
}

func TestExecutor_ExecuteWithAttempt(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	exec := NewExecutor(reg, DefaultExecutorConfig())
	rc := NewRoutingContext(testRequest(), testDecision("openai"))

	resp, err := exec.ExecuteWithAttempt(context.Background(), rc, "openai", 1, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != "openai" {
		t.Errorf("expected openai, got %s", resp.Provider)
	}
	if rc.AttemptCount() != 1 {
		t.Errorf("expected 1 attempt, got %d", rc.AttemptCount())
	}
}

// --- Engine Tests ---

func TestEngine_Route_Success(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}))

	engine := NewEngine(reg, DefaultEngineConfig())

	resp, err := engine.Route(context.Background(), testRequest(), testDecision("openai"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != "openai" {
		t.Errorf("expected openai, got %s", resp.Provider)
	}

	snap := engine.Metrics().Snapshot()
	if snap.TotalRequests != 1 {
		t.Errorf("expected 1 total request, got %d", snap.TotalRequests)
	}
	if snap.SuccessfulRequests != 1 {
		t.Errorf("expected 1 successful request, got %d", snap.SuccessfulRequests)
	}
	if snap.ProviderAttempts["openai"] != 1 {
		t.Errorf("expected 1 openai attempt, got %d", snap.ProviderAttempts["openai"])
	}
}

func TestEngine_Route_NilRequest(t *testing.T) {
	reg := NewRegistry(nil)
	engine := NewEngine(reg, DefaultEngineConfig())

	_, err := engine.Route(context.Background(), nil, testDecision("openai"))
	if err == nil {
		t.Error("expected error for nil request")
	}
}

func TestEngine_Route_NilDecision(t *testing.T) {
	reg := NewRegistry(nil)
	engine := NewEngine(reg, DefaultEngineConfig())

	_, err := engine.Route(context.Background(), testRequest(), nil)
	if err == nil {
		t.Error("expected error for nil decision")
	}
}

func TestEngine_Route_FallbackOnFailure(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", errorResponse("openai down")),
		NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("azure", successResponse("azure")),
		NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}))

	engine := NewEngine(reg, DefaultEngineConfig())

	var fallbackEvents []FallbackEvent
	engine.SetFallbackEventHandler(func(e FallbackEvent) {
		fallbackEvents = append(fallbackEvents, e)
	})

	resp, err := engine.Route(context.Background(), testRequest(), testDecision("openai"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != "azure" {
		t.Errorf("expected azure, got %s", resp.Provider)
	}

	snap := engine.Metrics().Snapshot()
	if snap.FallbackCount != 1 {
		t.Errorf("expected 1 fallback, got %d", snap.FallbackCount)
	}
	if snap.ProviderErrors["openai"] != 1 {
		t.Errorf("expected 1 openai error, got %d", snap.ProviderErrors["openai"])
	}

	if len(fallbackEvents) != 1 {
		t.Fatalf("expected 1 fallback event, got %d", len(fallbackEvents))
	}
	if fallbackEvents[0].FromProvider != "openai" {
		t.Errorf("expected fallback from openai, got %s", fallbackEvents[0].FromProvider)
	}
}

func TestEngine_Route_AllProvidersFail(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", errorResponse("openai down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("azure", errorResponse("azure down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	engine := NewEngine(reg, DefaultEngineConfig())

	_, err := engine.Route(context.Background(), testRequest(), testDecision("openai"))
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}

	_, ok := err.(*AllProvidersFailedError)
	if !ok {
		t.Errorf("expected AllProvidersFailedError, got %T: %v", err, err)
	}

	snap := engine.Metrics().Snapshot()
	if snap.FailedRequests != 1 {
		t.Errorf("expected 1 failed request, got %d", snap.FailedRequests)
	}
}

func TestEngine_RouteSimple(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", successResponse("openai")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	engine := NewEngine(reg, DefaultEngineConfig())

	resp, err := engine.RouteSimple(context.Background(), testRequest(), "openai")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != "openai" {
		t.Errorf("expected openai, got %s", resp.Provider)
	}
}

func TestEngine_RouteSimple_NilRequest(t *testing.T) {
	reg := NewRegistry(nil)
	engine := NewEngine(reg, DefaultEngineConfig())

	_, err := engine.RouteSimple(context.Background(), nil, "openai")
	if err == nil {
		t.Error("expected error for nil request")
	}
}

func TestEngine_RouteSimple_EmptyProvider(t *testing.T) {
	reg := NewRegistry(nil)
	engine := NewEngine(reg, DefaultEngineConfig())

	_, err := engine.RouteSimple(context.Background(), testRequest(), "")
	if err == nil {
		t.Error("expected error for empty provider")
	}
}

func TestEngine_String(t *testing.T) {
	reg := NewRegistry(nil)
	engine := NewEngine(reg, DefaultEngineConfig())
	s := engine.String()
	if s == "" {
		t.Error("expected non-empty string")
	}
}

// --- Integration Tests ---

func TestIntegration_MultipleProviders_FallbackChain(t *testing.T) {
	reg := NewRegistry(nil)

	callOrder := make([]string, 0)
	var mu sync.Mutex

	// Provider 1: fails
	_ = reg.Register(newMockProvider("primary", func(_ context.Context, _ *request.NormalizedRequest) (*ProviderResponse, error) {
		mu.Lock()
		callOrder = append(callOrder, "primary")
		mu.Unlock()
		return nil, fmt.Errorf("primary down")
	}), NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}))

	// Provider 2: fails
	_ = reg.Register(newMockProvider("secondary", func(_ context.Context, _ *request.NormalizedRequest) (*ProviderResponse, error) {
		mu.Lock()
		callOrder = append(callOrder, "secondary")
		mu.Unlock()
		return nil, fmt.Errorf("secondary down")
	}), NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}))

	// Provider 3: succeeds
	_ = reg.Register(newMockProvider("tertiary", func(_ context.Context, _ *request.NormalizedRequest) (*ProviderResponse, error) {
		mu.Lock()
		callOrder = append(callOrder, "tertiary")
		mu.Unlock()
		return &ProviderResponse{
			StatusCode: 200,
			Body:       []byte(`{"ok":true}`),
			Provider:   "tertiary",
		}, nil
	}), NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}))

	engine := NewEngine(reg, DefaultEngineConfig())

	var events []FallbackEvent
	engine.SetFallbackEventHandler(func(e FallbackEvent) {
		events = append(events, e)
	})

	resp, err := engine.Route(context.Background(), testRequest(), testDecision("primary"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != "tertiary" {
		t.Errorf("expected tertiary provider, got %s", resp.Provider)
	}

	// Verify call order: primary first, then others.
	if len(callOrder) < 3 {
		t.Fatalf("expected 3 calls, got %d", len(callOrder))
	}
	if callOrder[0] != "primary" {
		t.Errorf("expected primary first, got %s", callOrder[0])
	}

	// Verify fallback events.
	if len(events) != 2 {
		t.Errorf("expected 2 fallback events, got %d", len(events))
	}

	// Verify metrics.
	snap := engine.Metrics().Snapshot()
	if snap.TotalRequests != 1 {
		t.Errorf("expected 1 total, got %d", snap.TotalRequests)
	}
	if snap.SuccessfulRequests != 1 {
		t.Errorf("expected 1 success, got %d", snap.SuccessfulRequests)
	}
	if snap.FallbackCount != 2 {
		t.Errorf("expected 2 fallbacks, got %d", snap.FallbackCount)
	}
}

func TestIntegration_ProviderFailure_MetricsRecorded(t *testing.T) {
	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", errorResponse("openai error")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	engine := NewEngine(reg, DefaultEngineConfig())

	_, err := engine.Route(context.Background(), testRequest(), testDecision("openai"))
	if err == nil {
		t.Fatal("expected error")
	}

	snap := engine.Metrics().Snapshot()
	if snap.TotalRequests != 1 {
		t.Errorf("expected 1 total request, got %d", snap.TotalRequests)
	}
	if snap.FailedRequests != 1 {
		t.Errorf("expected 1 failed request, got %d", snap.FailedRequests)
	}
	if snap.ProviderErrors["openai"] != 1 {
		t.Errorf("expected 1 openai error, got %d", snap.ProviderErrors["openai"])
	}
}

func TestIntegration_AcceptanceCriteria_PolicyDecisionRouting(t *testing.T) {
	// Given policy decision with provider "openai"
	// When routing engine executes
	// Then request is sent to openai provider
	// And routing metrics are recorded

	reg := NewRegistry(nil)
	openai := newMockProvider("openai", successResponse("openai"))
	_ = reg.Register(openai, NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}))

	engine := NewEngine(reg, DefaultEngineConfig())
	decision := testDecision("openai")

	resp, err := engine.Route(context.Background(), testRequest(), decision)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != "openai" {
		t.Errorf("expected openai, got %s", resp.Provider)
	}
	if openai.CallCount() != 1 {
		t.Errorf("expected openai called once, got %d", openai.CallCount())
	}
	snap := engine.Metrics().Snapshot()
	if snap.TotalRequests != 1 || snap.SuccessfulRequests != 1 {
		t.Error("metrics not recorded correctly")
	}
}

func TestIntegration_AcceptanceCriteria_FallbackOnFailure(t *testing.T) {
	// Given provider "openai" fails
	// And fallback provider "azure" is configured
	// When routing engine handles failure
	// Then request is sent to azure provider
	// And fallback event is logged

	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", errorResponse("openai down")),
		NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}))
	azure := newMockProvider("azure", successResponse("azure"))
	_ = reg.Register(azure, NewProviderCapabilities([]string{"gpt-4"}, nil, RateLimit{}, Pricing{}))

	engine := NewEngine(reg, DefaultEngineConfig())

	var events []FallbackEvent
	engine.SetFallbackEventHandler(func(e FallbackEvent) {
		events = append(events, e)
	})

	resp, err := engine.Route(context.Background(), testRequest(), testDecision("openai"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Provider != "azure" {
		t.Errorf("expected azure, got %s", resp.Provider)
	}
	if azure.CallCount() != 1 {
		t.Errorf("expected azure called once, got %d", azure.CallCount())
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 fallback event, got %d", len(events))
	}
	if events[0].FromProvider != "openai" || events[0].ToProvider != "azure" {
		t.Error("fallback event has wrong providers")
	}
}

func TestIntegration_AcceptanceCriteria_AllProvidersFail(t *testing.T) {
	// Given all providers fail
	// When fallback chain is exhausted
	// Then error is returned with all provider errors

	reg := NewRegistry(nil)
	_ = reg.Register(newMockProvider("openai", errorResponse("openai down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("azure", errorResponse("azure down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("gcp", errorResponse("gcp down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	engine := NewEngine(reg, DefaultEngineConfig())

	_, err := engine.Route(context.Background(), testRequest(), testDecision("openai"))
	if err == nil {
		t.Fatal("expected error when all providers fail")
	}

	allErr, ok := err.(*AllProvidersFailedError)
	if !ok {
		t.Fatalf("expected AllProvidersFailedError, got %T", err)
	}
	if len(allErr.Errors) != 3 {
		t.Errorf("expected 3 provider errors, got %d", len(allErr.Errors))
	}

	// Verify all providers are represented.
	providers := make(map[string]bool)
	for _, pe := range allErr.Errors {
		providers[pe.Provider] = true
	}
	for _, name := range []string{"openai", "azure", "gcp"} {
		if !providers[name] {
			t.Errorf("expected error from %s", name)
		}
	}
}

func TestIntegration_HealthCheckerWithRegistry(t *testing.T) {
	checker := ProviderHealthCheckFunc(func(_ context.Context, provider string) error {
		if provider == "unhealthy" {
			return fmt.Errorf("down")
		}
		return nil
	})
	hc := NewHealthChecker(DefaultHealthCheckConfig(), checker)
	reg := NewRegistry(hc)

	_ = reg.Register(newMockProvider("healthy", successResponse("healthy")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))
	_ = reg.Register(newMockProvider("unhealthy", errorResponse("down")),
		NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}))

	// Check health.
	hc.CheckNow(context.Background(), "healthy")
	hc.CheckNow(context.Background(), "unhealthy")

	if !reg.IsHealthy("healthy") {
		t.Error("expected healthy provider to be healthy")
	}
	// After a single failure with degraded threshold = 1, status is degraded.
	// Degraded is still considered "healthy" by IsHealthy().
	if !reg.IsHealthy("unhealthy") {
		// With default config, 1 failure = degraded, which is still "healthy".
		// Let's check more failures.
		hc.CheckNow(context.Background(), "unhealthy")
		hc.CheckNow(context.Background(), "unhealthy")
		if reg.IsHealthy("unhealthy") {
			t.Error("expected unhealthy provider after 3 failures")
		}
	}

	healthy := reg.FindHealthyProviders()
	found := false
	for _, h := range healthy {
		if h == "healthy" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'healthy' provider in healthy list")
	}
}

func TestReorderProviders(t *testing.T) {
	result := reorderProviders([]string{"a", "b", "c"}, "b")
	if result[0] != "b" {
		t.Errorf("expected b first, got %s", result[0])
	}
	if len(result) != 3 {
		t.Errorf("expected 3, got %d", len(result))
	}

	// Primary not in list.
	result = reorderProviders([]string{"a", "b"}, "x")
	if result[0] != "x" {
		t.Errorf("expected x first, got %s", result[0])
	}
	if len(result) != 3 {
		t.Errorf("expected 3, got %d", len(result))
	}

	// Empty list.
	result = reorderProviders(nil, "a")
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}

	// Empty primary.
	result = reorderProviders([]string{"a", "b"}, "")
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

// --- Routing Metrics Tests ---

func TestRoutingMetrics_Snapshot(t *testing.T) {
	rm := NewRoutingMetrics()
	rm.RecordRequest(true)
	rm.RecordRequest(true)
	rm.RecordRequest(false)
	rm.RecordAttempt("openai")
	rm.RecordAttempt("openai")
	rm.RecordAttempt("azure")
	rm.RecordError("openai")
	rm.RecordFallback()

	snap := rm.Snapshot()
	if snap.TotalRequests != 3 {
		t.Errorf("expected 3 total, got %d", snap.TotalRequests)
	}
	if snap.SuccessfulRequests != 2 {
		t.Errorf("expected 2 successful, got %d", snap.SuccessfulRequests)
	}
	if snap.FailedRequests != 1 {
		t.Errorf("expected 1 failed, got %d", snap.FailedRequests)
	}
	if snap.FallbackCount != 1 {
		t.Errorf("expected 1 fallback, got %d", snap.FallbackCount)
	}
	if snap.ProviderAttempts["openai"] != 2 {
		t.Errorf("expected 2 openai attempts, got %d", snap.ProviderAttempts["openai"])
	}
	if snap.ProviderErrors["openai"] != 1 {
		t.Errorf("expected 1 openai error, got %d", snap.ProviderErrors["openai"])
	}
}

// --- Concurrent Access Tests ---

func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewRegistry(nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := fmt.Sprintf("provider-%d", n%10)
			_ = reg.Register(
				newMockProvider(name, successResponse(name)),
				NewProviderCapabilities(nil, nil, RateLimit{}, Pricing{}),
			)
			_, _ = reg.Get(name)
			reg.ListProviders()
			reg.FindProvidersForModel("gpt-4")
		}(i)
	}
	wg.Wait()
}

func TestRoutingContext_ConcurrentAccess(t *testing.T) {
	rc := NewRoutingContext(testRequest(), testDecision("openai"))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rc.RecordAttempt(ProviderAttempt{
				Provider:      fmt.Sprintf("p-%d", n),
				AttemptNumber: n,
			})
			rc.AttemptCount()
			rc.LastAttempt()
			rc.AllErrors()
			rc.SetMetadata(fmt.Sprintf("key-%d", n), "value")
			rc.GetMetadata(fmt.Sprintf("key-%d", n))
		}(i)
	}
	wg.Wait()

	if rc.AttemptCount() != 100 {
		t.Errorf("expected 100 attempts, got %d", rc.AttemptCount())
	}
}

func TestRoutingMetrics_ConcurrentAccess(t *testing.T) {
	rm := NewRoutingMetrics()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rm.RecordRequest(true)
			rm.RecordAttempt("openai")
			rm.RecordError("openai")
			rm.RecordFallback()
			rm.Snapshot()
		}()
	}
	wg.Wait()

	snap := rm.Snapshot()
	if snap.TotalRequests != 100 {
		t.Errorf("expected 100 total, got %d", snap.TotalRequests)
	}
}
