package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type BypassReason string

const (
	BypassCircuitOpen      BypassReason = "circuit_open"
	BypassCacheUnavailable BypassReason = "cache_unavailable"
	BypassCacheError       BypassReason = "cache_error"
	BypassCacheDisabled    BypassReason = "cache_disabled"
)

type BypassEvent struct {
	Reason         BypassReason
	Timestamp      time.Time
	Operation, Key string
	Error          error
}

func (e *BypassEvent) String() string {
	if e.Error != nil {
		return fmt.Sprintf("[%s] bypass %s on %s key=%q: %v", e.Timestamp.Format(time.RFC3339), e.Reason, e.Operation, e.Key, e.Error)
	}
	return fmt.Sprintf("[%s] bypass %s on %s key=%q", e.Timestamp.Format(time.RFC3339), e.Reason, e.Operation, e.Key)
}

type FallbackHandler struct {
	mu        sync.RWMutex
	events    []BypassEvent
	maxEvents int
	onBypass  func(BypassEvent)
}
type FallbackConfig struct {
	MaxEvents int
	OnBypass  func(BypassEvent)
}

func DefaultFallbackConfig() FallbackConfig { return FallbackConfig{MaxEvents: 1000} }

func NewFallbackHandler(c FallbackConfig) *FallbackHandler {
	if c.MaxEvents <= 0 {
		c.MaxEvents = 1000
	}
	return &FallbackHandler{maxEvents: c.MaxEvents, onBypass: c.OnBypass}
}

func (f *FallbackHandler) RecordBypass(reason BypassReason, op, key string, err error) {
	ev := BypassEvent{Reason: reason, Timestamp: time.Now(), Operation: op, Key: key, Error: err}
	f.mu.Lock()
	if len(f.events) >= f.maxEvents {
		d := f.maxEvents / 4
		if d == 0 {
			d = 1
		}
		f.events = f.events[d:]
	}
	f.events = append(f.events, ev)
	f.mu.Unlock()
	if f.onBypass != nil {
		f.onBypass(ev)
	}
}

func (f *FallbackHandler) Events() []BypassEvent {
	f.mu.RLock()
	defer f.mu.RUnlock()
	r := make([]BypassEvent, len(f.events))
	copy(r, f.events)
	return r
}
func (f *FallbackHandler) EventCount() int { f.mu.RLock(); defer f.mu.RUnlock(); return len(f.events) }
func (f *FallbackHandler) EventsByReason() map[BypassReason]int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	m := make(map[BypassReason]int)
	for _, e := range f.events {
		m[e.Reason]++
	}
	return m
}
func (f *FallbackHandler) ClearEvents() { f.mu.Lock(); f.events = nil; f.mu.Unlock() }
func (f *FallbackHandler) FallbackGet(r BypassReason, k string, e error) (*Entry, bool, error) {
	f.RecordBypass(r, "get", k, e)
	return nil, false, nil
}
func (f *FallbackHandler) FallbackSet(r BypassReason, k string, e error) error {
	f.RecordBypass(r, "set", k, e)
	return nil
}
func (f *FallbackHandler) FallbackDelete(r BypassReason, k string, e error) error {
	f.RecordBypass(r, "delete", k, e)
	return nil
}
func (f *FallbackHandler) FallbackExists(r BypassReason, k string, e error) (bool, error) {
	f.RecordBypass(r, "exists", k, e)
	return false, nil
}

type ResilientCache struct {
	inner    Cache
	breaker  *CircuitBreaker
	fallback *FallbackHandler
	monitor  *HealthMonitor
}
type ResilientCacheConfig struct {
	CircuitBreaker CircuitBreakerConfig
	Fallback       FallbackConfig
	HealthMonitor  HealthMonitorConfig
}

func DefaultResilientCacheConfig() ResilientCacheConfig {
	return ResilientCacheConfig{CircuitBreaker: DefaultCircuitBreakerConfig(), Fallback: DefaultFallbackConfig(), HealthMonitor: DefaultHealthMonitorConfig()}
}

func NewResilientCache(inner Cache, c ResilientCacheConfig) *ResilientCache {
	return &ResilientCache{inner: inner, breaker: NewCircuitBreaker(c.CircuitBreaker), fallback: NewFallbackHandler(c.Fallback), monitor: NewHealthMonitor(c.HealthMonitor)}
}

func (rc *ResilientCache) Get(ctx context.Context, key string) (*Entry, bool, error) {
	if !rc.breaker.Allow() {
		return rc.fallback.FallbackGet(BypassCircuitOpen, key, nil)
	}
	entry, found, err := rc.inner.Get(ctx, key)
	if err != nil {
		if IsCacheMiss(err) || IsCacheExpired(err) {
			rc.breaker.RecordSuccess()
			rc.monitor.RecordSuccess()
			return entry, found, err
		}
		rc.breaker.RecordFailure()
		rc.monitor.RecordFailure()
		return rc.fallback.FallbackGet(BypassCacheError, key, err)
	}
	rc.breaker.RecordSuccess()
	rc.monitor.RecordSuccess()
	return entry, found, nil
}

func (rc *ResilientCache) Set(ctx context.Context, key string, entry *Entry, opts ...Option) error {
	if !rc.breaker.Allow() {
		return rc.fallback.FallbackSet(BypassCircuitOpen, key, nil)
	}
	if err := rc.inner.Set(ctx, key, entry, opts...); err != nil {
		rc.breaker.RecordFailure()
		rc.monitor.RecordFailure()
		return rc.fallback.FallbackSet(BypassCacheError, key, err)
	}
	rc.breaker.RecordSuccess()
	rc.monitor.RecordSuccess()
	return nil
}

func (rc *ResilientCache) Delete(ctx context.Context, key string) error {
	if !rc.breaker.Allow() {
		return rc.fallback.FallbackDelete(BypassCircuitOpen, key, nil)
	}
	if err := rc.inner.Delete(ctx, key); err != nil {
		rc.breaker.RecordFailure()
		rc.monitor.RecordFailure()
		return rc.fallback.FallbackDelete(BypassCacheError, key, err)
	}
	rc.breaker.RecordSuccess()
	rc.monitor.RecordSuccess()
	return nil
}

func (rc *ResilientCache) Exists(ctx context.Context, key string) (bool, error) {
	if !rc.breaker.Allow() {
		return rc.fallback.FallbackExists(BypassCircuitOpen, key, nil)
	}
	exists, err := rc.inner.Exists(ctx, key)
	if err != nil {
		rc.breaker.RecordFailure()
		rc.monitor.RecordFailure()
		return rc.fallback.FallbackExists(BypassCacheError, key, err)
	}
	rc.breaker.RecordSuccess()
	rc.monitor.RecordSuccess()
	return exists, nil
}

func (rc *ResilientCache) Close() error                    { rc.monitor.Stop(); return rc.inner.Close() }
func (rc *ResilientCache) CircuitBreaker() *CircuitBreaker { return rc.breaker }
func (rc *ResilientCache) Fallback() *FallbackHandler      { return rc.fallback }
func (rc *ResilientCache) HealthMonitor() *HealthMonitor   { return rc.monitor }
func (rc *ResilientCache) Inner() Cache                    { return rc.inner }
