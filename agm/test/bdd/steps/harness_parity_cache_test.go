package steps

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBDDBehaviorSuiteCacheExecutesOnce(t *testing.T) {
	var cache bddBehaviorSuiteCache
	var calls atomic.Int32
	testErr := errors.New("test failure")
	executionErr := errors.New("execution failure")

	const workers = 32
	results := make(chan bddBehaviorSuiteResult, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			results <- cache.load(func() bddBehaviorSuiteResult {
				calls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return bddBehaviorSuiteResult{
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

func TestCodexHookReviewBehaviorSuiteTimeoutCoversAggregateContention(t *testing.T) {
	if codexHookReviewBehaviorSuiteTimeout < 2*time.Minute {
		t.Fatalf("codexHookReviewBehaviorSuiteTimeout = %v, want at least 2m", codexHookReviewBehaviorSuiteTimeout)
	}
}
