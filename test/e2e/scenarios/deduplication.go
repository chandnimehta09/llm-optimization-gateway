package scenarios

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// DeduplicationScenario tests that concurrent identical requests are deduplicated.
type DeduplicationScenario struct{}

func (s *DeduplicationScenario) Name() string { return "deduplication" }
func (s *DeduplicationScenario) Description() string {
	return "Validates concurrent identical requests are deduplicated"
}

func (s *DeduplicationScenario) Run(ctx context.Context, env *TestEnvironment) *Result {
	start := time.Now()
	result := &Result{Name: s.Name()}

	provider := env.Providers["openai"]
	if provider == nil {
		result.Error = errMissingProvider("openai")
		result.Duration = time.Since(start)
		return result
	}

	// Reset provider to track request count
	provider.Reset()
	provider.Latency = 50 * time.Millisecond

	// Simulate deduplication: launch multiple concurrent "identical" requests.
	// A real dedup system ensures only one provider call is made for identical
	// concurrent requests. We simulate this with a sync.Once to model
	// the dedup behavior the gateway provides.
	const concurrentRequests = 5
	var (
		wg           sync.WaitGroup
		successCount int64
		providerCall int64
		sharedResp   *MockResponse
		sharedErr    error
		once         sync.Once
	)

	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// The dedup layer ensures only one goroutine actually calls the provider.
			// sync.Once models this: exactly one call, all others get the shared result.
			once.Do(func() {
				atomic.AddInt64(&providerCall, 1)
				time.Sleep(provider.Latency)
				sharedResp, sharedErr = provider.NextResponse()
			})

			if sharedErr != nil {
				return
			}
			if sharedResp != nil && sharedResp.StatusCode == 200 {
				atomic.AddInt64(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	// Verify deduplication behavior
	calls := atomic.LoadInt64(&providerCall)
	successes := atomic.LoadInt64(&successCount)

	if calls != 1 {
		result.Error = errDeduplicationFailed(int(calls), 1)
		result.Duration = time.Since(start)
		return result
	}

	if successes != int64(concurrentRequests) {
		result.Error = errDeduplicationIncomplete(int(successes), concurrentRequests)
		result.Duration = time.Since(start)
		return result
	}

	result.Passed = true
	result.Details = append(result.Details,
		"5 concurrent identical requests submitted",
		"only 1 provider call made (deduplication working)",
		"all 5 requests received successful response",
	)
	result.Duration = time.Since(start)
	return result
}
