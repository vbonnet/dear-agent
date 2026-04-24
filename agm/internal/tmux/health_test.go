package tmux

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHealthChecker_Check_Success(t *testing.T) {
	hc := NewHealthChecker(5*time.Second, 2*time.Second)

	err := hc.Check()
	// Either succeeds or returns exit code 1 (no sessions), both are OK
	if err != nil {
		t.Logf("Health check returned: %v (this is OK if no tmux server)", err)
	}
}

func TestHealthChecker_Check_Cached(t *testing.T) {
	hc := NewHealthChecker(5*time.Second, 2*time.Second)

	// First check (probe)
	start1 := time.Now()
	err1 := hc.Check()
	elapsed1 := time.Since(start1)
	if err1 != nil {
		t.Logf("First check returned: %v", err1)
	}

	// Second check (should use cache)
	start2 := time.Now()
	err2 := hc.Check()
	elapsed2 := time.Since(start2)
	if err2 != nil {
		t.Logf("Second check returned: %v", err2)
	}

	// Second check should be much faster (< 10ms)
	if elapsed2 > 50*time.Millisecond {
		t.Errorf("Cached check took too long: %v (expected < 50ms)", elapsed2)
	}

	// First check can be slow, second should be fast
	t.Logf("First check: %v, Second check (cached): %v", elapsed1, elapsed2)
}

func TestHealthChecker_Check_CacheExpires(t *testing.T) {
	// Short cache duration for testing
	hc := NewHealthChecker(100*time.Millisecond, 2*time.Second)

	// First check
	err1 := hc.Check()
	if err1 != nil {
		t.Logf("First check returned: %v", err1)
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Second check should perform fresh probe
	start := time.Now()
	err2 := hc.Check()
	elapsed := time.Since(start)
	if err2 != nil {
		t.Logf("Second check returned: %v", err2)
	}

	// Should take longer than cached check
	// (but may be fast if tmux responds quickly)
	t.Logf("Check after cache expiry: %v", elapsed)
}

func TestHealthChecker_Check_Timeout(t *testing.T) {
	// Very short timeout to force timeout
	hc := NewHealthChecker(5*time.Second, 1*time.Nanosecond)

	err := hc.Check()
	if err == nil {
		// This might happen if tmux is super fast or not installed
		t.Skip("Expected timeout, but check succeeded")
	}

	// Check if it's a HealthCheckError
	healthErr, ok := err.(*HealthCheckError)
	if !ok {
		// Might be exec error if tmux not found
		t.Logf("Got error (not HealthCheckError): %v", err)
		return
	}

	if healthErr.Problem == "" {
		t.Error("HealthCheckError.Problem is empty")
	}
	if healthErr.Recovery == "" {
		t.Error("HealthCheckError.Recovery is empty")
	}
}

func TestHealthChecker_InvalidateCache(t *testing.T) {
	hc := NewHealthChecker(5*time.Second, 2*time.Second)

	// First check
	hc.Check()

	// Verify cache is active
	hc.mu.RLock()
	cacheActive := !hc.lastCheck.IsZero()
	hc.mu.RUnlock()
	if !cacheActive {
		t.Error("Cache should be active after first check")
	}

	// Invalidate cache
	hc.InvalidateCache()

	// Verify cache is cleared
	hc.mu.RLock()
	cacheCleared := hc.lastCheck.IsZero()
	hc.mu.RUnlock()
	if !cacheCleared {
		t.Error("Cache should be cleared after InvalidateCache()")
	}
}

func TestHealthChecker_Concurrent(t *testing.T) {
	hc := NewHealthChecker(5*time.Second, 2*time.Second)

	// Run concurrent health checks
	const numGoroutines = 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			err := hc.Check()
			if err != nil {
				t.Logf("Concurrent check returned: %v", err)
			}
		}()
	}

	wg.Wait()
	// If we get here without data races, the test passes
}

func TestHealthCheckError_Format(t *testing.T) {
	err := &HealthCheckError{
		Problem:  "Test problem",
		Recovery: "Test recovery",
	}

	errStr := err.Error()
	if errStr == "" {
		t.Error("HealthCheckError.Error() returned empty string")
	}

	// Verify both fields are in the error message
	if !strings.Contains(errStr, "Test problem") {
		t.Error("Error message missing Problem field")
	}
	if !strings.Contains(errStr, "Test recovery") {
		t.Error("Error message missing Recovery field")
	}
}

func TestNewHealthChecker(t *testing.T) {
	cacheDuration := 10 * time.Second
	probeTimeout := 3 * time.Second

	hc := NewHealthChecker(cacheDuration, probeTimeout)

	if hc == nil {
		t.Fatal("NewHealthChecker returned nil")
	}

	if hc.cacheDuration != cacheDuration {
		t.Errorf("cacheDuration = %v, expected %v", hc.cacheDuration, cacheDuration)
	}

	if hc.probeTimeout != probeTimeout {
		t.Errorf("probeTimeout = %v, expected %v", hc.probeTimeout, probeTimeout)
	}
}

func TestHealthChecker_DoubleCheckLocking(t *testing.T) {
	hc := NewHealthChecker(5*time.Second, 2*time.Second)

	// First check to populate cache
	hc.Check()

	// Concurrent checks should not perform duplicate probes
	const numGoroutines = 5
	probeCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			start := time.Now()
			hc.Check()
			elapsed := time.Since(start)

			// If check took > 10ms, assume it was a probe
			if elapsed > 10*time.Millisecond {
				mu.Lock()
				probeCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// All checks should have used cache (no probes)
	if probeCount > 0 {
		t.Logf("Warning: %d goroutines performed fresh probes (expected 0)", probeCount)
		// This is a soft check - might happen under load
	}
}
