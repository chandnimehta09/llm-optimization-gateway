package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestFB_Record(t *testing.T) {
	fh := NewFallbackHandler(DefaultFallbackConfig())
	fh.RecordBypass(BypassCircuitOpen, "get", "k", nil)
	if fh.EventCount() != 1 {
		t.Error("1")
	}
	ev := fh.Events()
	if ev[0].Reason != BypassCircuitOpen || ev[0].Operation != "get" {
		t.Error("event")
	}
}
func TestFB_Error(t *testing.T) {
	fh := NewFallbackHandler(DefaultFallbackConfig())
	e := errors.New("conn")
	fh.RecordBypass(BypassCacheError, "set", "k", e)
	if fh.Events()[0].Error != e {
		t.Error("err")
	}
}
func TestFB_ByReason(t *testing.T) {
	fh := NewFallbackHandler(DefaultFallbackConfig())
	fh.RecordBypass(BypassCircuitOpen, "get", "k1", nil)
	fh.RecordBypass(BypassCircuitOpen, "get", "k2", nil)
	fh.RecordBypass(BypassCacheError, "set", "k3", nil)
	m := fh.EventsByReason()
	if m[BypassCircuitOpen] != 2 || m[BypassCacheError] != 1 {
		t.Error("counts")
	}
}
func TestFB_Max(t *testing.T) {
	fh := NewFallbackHandler(FallbackConfig{MaxEvents: 10})
	for i := 0; i < 20; i++ {
		fh.RecordBypass(BypassCircuitOpen, "get", "k", nil)
	}
	if fh.EventCount() > 10 {
		t.Error("max")
	}
}
func TestFB_Clear(t *testing.T) {
	fh := NewFallbackHandler(DefaultFallbackConfig())
	fh.RecordBypass(BypassCircuitOpen, "get", "k", nil)
	fh.ClearEvents()
	if fh.EventCount() != 0 {
		t.Error("0")
	}
}
func TestFB_Callback(t *testing.T) {
	var mu sync.Mutex
	n := 0
	fh := NewFallbackHandler(FallbackConfig{MaxEvents: 100, OnBypass: func(BypassEvent) { mu.Lock(); n++; mu.Unlock() }})
	fh.RecordBypass(BypassCircuitOpen, "get", "k", nil)
	mu.Lock()
	defer mu.Unlock()
	if n != 1 {
		t.Error("cb")
	}
}
func TestFB_Get(t *testing.T) {
	e, f, err := NewFallbackHandler(DefaultFallbackConfig()).FallbackGet(BypassCircuitOpen, "k", nil)
	if e != nil || f || err != nil {
		t.Error("get")
	}
}
func TestFB_Set(t *testing.T) {
	if NewFallbackHandler(DefaultFallbackConfig()).FallbackSet(BypassCacheError, "k", nil) != nil {
		t.Error("set")
	}
}
func TestFB_Del(t *testing.T) {
	if NewFallbackHandler(DefaultFallbackConfig()).FallbackDelete(BypassCircuitOpen, "k", nil) != nil {
		t.Error("del")
	}
}
func TestFB_Exists(t *testing.T) {
	e, err := NewFallbackHandler(DefaultFallbackConfig()).FallbackExists(BypassCacheUnavailable, "k", nil)
	if e || err != nil {
		t.Error("exists")
	}
}
func TestBE_String(t *testing.T) {
	e := &BypassEvent{Reason: BypassCircuitOpen, Timestamp: time.Now(), Operation: "get", Key: "k"}
	if e.String() == "" {
		t.Error("empty")
	}
	e2 := &BypassEvent{Reason: BypassCacheError, Timestamp: time.Now(), Operation: "set", Key: "k", Error: errors.New("e")}
	if e2.String() == "" {
		t.Error("empty2")
	}
}
func TestFB_Defaults(t *testing.T) {
	if DefaultFallbackConfig().MaxEvents != 1000 {
		t.Error("1000")
	}
}
func TestFB_Concurrent(t *testing.T) {
	fh := NewFallbackHandler(FallbackConfig{MaxEvents: 100})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); fh.RecordBypass(BypassCircuitOpen, "get", "k", nil) }()
	}
	wg.Wait()
}

type failingCache struct {
	mu                   sync.Mutex
	callCount, failAfter int
	failErr              error
	entries              map[string]*Entry
	recovered            bool
}

func newFailingCache(n int) *failingCache {
	return &failingCache{failAfter: n, failErr: errors.New("redis: connection refused"), entries: make(map[string]*Entry)}
}
func (fc *failingCache) shouldFail() bool {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.callCount++
	if fc.recovered {
		return false
	}
	return fc.callCount > fc.failAfter
}
func (fc *failingCache) recover() { fc.mu.Lock(); fc.recovered = true; fc.mu.Unlock() }
func (fc *failingCache) Get(ctx context.Context, key string) (*Entry, bool, error) {
	if fc.shouldFail() {
		return nil, false, fc.failErr
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	e, ok := fc.entries[key]
	return e, ok, nil
}
func (fc *failingCache) Set(ctx context.Context, key string, entry *Entry, opts ...Option) error {
	if fc.shouldFail() {
		return fc.failErr
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.entries[key] = entry
	return nil
}
func (fc *failingCache) Delete(ctx context.Context, key string) error {
	if fc.shouldFail() {
		return fc.failErr
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	delete(fc.entries, key)
	return nil
}
func (fc *failingCache) Exists(ctx context.Context, key string) (bool, error) {
	if fc.shouldFail() {
		return false, fc.failErr
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	_, ok := fc.entries[key]
	return ok, nil
}
func (fc *failingCache) Close() error { return nil }

func TestRC_GetOK(t *testing.T) {
	m := newMemoryCache()
	m.Set(context.Background(), "k", newTestEntry("c", 5*time.Minute))
	rc := NewResilientCache(m, DefaultResilientCacheConfig())
	defer rc.Close()
	_, f, e := rc.Get(context.Background(), "k")
	if e != nil || !f {
		t.Error("get")
	}
}
func TestRC_GetFB(t *testing.T) {
	rc := NewResilientCache(newFailingCache(0), DefaultResilientCacheConfig())
	defer rc.Close()
	_, f, e := rc.Get(context.Background(), "k")
	if e != nil || f {
		t.Error("fb")
	}
}
func TestRC_SetFB(t *testing.T) {
	rc := NewResilientCache(newFailingCache(0), DefaultResilientCacheConfig())
	defer rc.Close()
	if rc.Set(context.Background(), "k", newTestEntry("c", 5*time.Minute)) != nil {
		t.Error("set")
	}
}
func TestRC_DelFB(t *testing.T) {
	rc := NewResilientCache(newFailingCache(0), DefaultResilientCacheConfig())
	defer rc.Close()
	if rc.Delete(context.Background(), "k") != nil {
		t.Error("del")
	}
}
func TestRC_ExFB(t *testing.T) {
	rc := NewResilientCache(newFailingCache(0), DefaultResilientCacheConfig())
	defer rc.Close()
	e, err := rc.Exists(context.Background(), "k")
	if e || err != nil {
		t.Error("ex")
	}
}
func TestRC_Opens(t *testing.T) {
	rc := NewResilientCache(newFailingCache(0), ResilientCacheConfig{CircuitBreaker: CircuitBreakerConfig{FailureThreshold: 3, ResetTimeout: time.Minute}, Fallback: DefaultFallbackConfig(), HealthMonitor: DefaultHealthMonitorConfig()})
	defer rc.Close()
	for i := 0; i < 3; i++ {
		rc.Get(context.Background(), "k")
	}
	if rc.CircuitBreaker().State() != CircuitOpen {
		t.Error("open")
	}
	rc.Get(context.Background(), "k")
	found := false
	for _, e := range rc.Fallback().Events() {
		if e.Reason == BypassCircuitOpen {
			found = true
		}
	}
	if !found {
		t.Error("bypass")
	}
}
func TestRC_Access(t *testing.T) {
	m := newMemoryCache()
	rc := NewResilientCache(m, DefaultResilientCacheConfig())
	defer rc.Close()
	if rc.Inner() != m || rc.CircuitBreaker() == nil || rc.Fallback() == nil || rc.HealthMonitor() == nil {
		t.Error("accessors")
	}
}
func TestRC_Close(t *testing.T) {
	if NewResilientCache(newMemoryCache(), DefaultResilientCacheConfig()).Close() != nil {
		t.Error("close")
	}
}
func TestRC_Defaults(t *testing.T) {
	c := DefaultResilientCacheConfig()
	if c.CircuitBreaker.FailureThreshold != 5 || c.Fallback.MaxEvents != 1000 || c.HealthMonitor.WindowSize != 100 {
		t.Error("defaults")
	}
}

func TestChaos_Out(t *testing.T) {
	rc := NewResilientCache(newFailingCache(0), ResilientCacheConfig{CircuitBreaker: CircuitBreakerConfig{FailureThreshold: 3, ResetTimeout: 100 * time.Millisecond}, Fallback: DefaultFallbackConfig(), HealthMonitor: DefaultHealthMonitorConfig()})
	defer rc.Close()
	ctx := context.Background()
	e := newTestEntry("c", 5*time.Minute)
	for i := 0; i < 5; i++ {
		if _, _, err := rc.Get(ctx, "k"); err != nil {
			t.Error("get")
		}
		if err := rc.Set(ctx, "k", e); err != nil {
			t.Error("set")
		}
	}
	if rc.CircuitBreaker().State() != CircuitOpen {
		t.Error("open")
	}
	if rc.HealthMonitor().Snapshot().TotalFailures == 0 {
		t.Error("failures")
	}
}
func TestChaos_Rec(t *testing.T) {
	inner := newFailingCache(0)
	rc := NewResilientCache(inner, ResilientCacheConfig{CircuitBreaker: CircuitBreakerConfig{FailureThreshold: 3, ResetTimeout: 50 * time.Millisecond}, Fallback: DefaultFallbackConfig(), HealthMonitor: DefaultHealthMonitorConfig()})
	defer rc.Close()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		rc.Get(ctx, "k")
	}
	if rc.CircuitBreaker().State() != CircuitOpen {
		t.Fatal("open")
	}
	inner.recover()
	time.Sleep(60 * time.Millisecond)
	e := newTestEntry("c", 5*time.Minute)
	if rc.Set(ctx, "k", e) != nil {
		t.Error("set")
	}
	if rc.CircuitBreaker().State() != CircuitClosed {
		t.Error("closed")
	}
	_, f, _ := rc.Get(ctx, "k")
	if !f {
		t.Error("found")
	}
}
func TestInteg_Full(t *testing.T) {
	inner := newFailingCache(5)
	rc := NewResilientCache(inner, ResilientCacheConfig{CircuitBreaker: CircuitBreakerConfig{FailureThreshold: 3, ResetTimeout: 50 * time.Millisecond}, Fallback: DefaultFallbackConfig(), HealthMonitor: DefaultHealthMonitorConfig()})
	defer rc.Close()
	ctx := context.Background()
	e := newTestEntry("c", 5*time.Minute)
	rc.Set(ctx, "k1", e)
	g, f, _ := rc.Get(ctx, "k1")
	if !f || g == nil {
		t.Error("phase1")
	}
	for i := 0; i < 10; i++ {
		rc.Get(ctx, "m")
		rc.Set(ctx, "f", e)
	}
	if rc.CircuitBreaker().State() != CircuitOpen {
		t.Error("open")
	}
	if rc.HealthMonitor().Snapshot().State == HealthHealthy {
		t.Error("healthy")
	}
	inner.recover()
	time.Sleep(60 * time.Millisecond)
	rc.Set(ctx, "r", e)
	if rc.CircuitBreaker().State() != CircuitClosed {
		t.Error("closed")
	}
	if rc.HealthMonitor().Snapshot().RecoveryTime == 0 {
		t.Error("recovery")
	}
}

func TestClassify_Nil(t *testing.T) {
	r, tr := ClassifyCacheError(nil)
	if r != "" || tr {
		t.Error("nil")
	}
}
func TestClassify_Miss(t *testing.T) {
	r, tr := ClassifyCacheError(&CacheMissError{Key: "k"})
	if r != "" || tr {
		t.Error("miss")
	}
}
func TestClassify_Exp(t *testing.T) {
	r, tr := ClassifyCacheError(&CacheExpiredError{Key: "k"})
	if r != "" || tr {
		t.Error("exp")
	}
}
func TestClassify_Corr(t *testing.T) {
	r, tr := ClassifyCacheError(&CacheCorruptedError{Key: "k", Reason: "b"})
	if r != BypassCacheError || tr {
		t.Error("corr")
	}
}
func TestClassify_Fail(t *testing.T) {
	r, tr := ClassifyCacheError(&CacheFailureError{Operation: "g", Cause: errors.New("e")})
	if r != BypassCacheError || !tr {
		t.Error("fail")
	}
}
func TestClassify_Unk(t *testing.T) {
	r, tr := ClassifyCacheError(errors.New("x"))
	if r != BypassCacheError || !tr {
		t.Error("unk")
	}
}
func TestCFE(t *testing.T) {
	c := errors.New("cr")
	e := &CacheFailureError{Operation: "get", Cause: c}
	if e.Error() != "cache failure during get: cr" {
		t.Error("msg")
	}
	if e.Unwrap() != c {
		t.Error("unwrap")
	}
	if !IsCacheFailure(e) {
		t.Error("is")
	}
	if IsCacheFailure(errors.New("x")) {
		t.Error("not")
	}
}
