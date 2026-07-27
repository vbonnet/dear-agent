package steps

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAgyLifecycleBehaviorSuiteCacheExecutesOnce(t *testing.T) {
	var cache agyLifecycleBehaviorSuiteCache
	var calls atomic.Int32
	testErr := errors.New("test failure")
	executionErr := errors.New("execution failure")

	const workers = 32
	results := make(chan agyLifecycleBehaviorSuiteResult, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			results <- cache.load(func() agyLifecycleBehaviorSuiteResult {
				calls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return agyLifecycleBehaviorSuiteResult{
					output:       "suite output",
					testErr:      testErr,
					executionErr: executionErr,
				}
			})
		})
	}
	wg.Wait()
	close(results)

	if got := calls.Load(); got != 1 {
		t.Fatalf("suite executions = %d, want 1", got)
	}
	for result := range results {
		if result.output != "suite output" {
			t.Fatalf("output = %q, want suite output", result.output)
		}
		if !errors.Is(result.testErr, testErr) {
			t.Fatalf("test error = %v, want %v", result.testErr, testErr)
		}
		if !errors.Is(result.executionErr, executionErr) {
			t.Fatalf("execution error = %v, want %v", result.executionErr, executionErr)
		}
	}
}
