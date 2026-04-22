package tmux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// HealthChecker manages tmux server health checks with caching
type HealthChecker struct {
	mu            sync.RWMutex
	lastCheck     time.Time
	lastResult    error
	cacheDuration time.Duration
	probeTimeout  time.Duration
}

// HealthCheckError represents a health check failure with recovery guidance
type HealthCheckError struct {
	Problem  string
	Recovery string
}

func (e *HealthCheckError) Error() string {
	msg := fmt.Sprintf("Error: %s\n\n", e.Problem)
	if e.Recovery != "" {
		msg += fmt.Sprintf("%s\n", e.Recovery)
	}
	return msg
}

// NewHealthChecker creates a new health checker with specified cache duration and probe timeout
func NewHealthChecker(cacheDuration time.Duration, probeTimeout time.Duration) *HealthChecker {
	return &HealthChecker{
		cacheDuration: cacheDuration,
		probeTimeout:  probeTimeout,
	}
}

// Check performs a health check on the tmux server.
// Returns cached result if recent (within cacheDuration), otherwise performs a fresh probe.
func (hc *HealthChecker) Check() error {
	// Fast path: read lock for cache check
	hc.mu.RLock()
	if time.Since(hc.lastCheck) < hc.cacheDuration && hc.lastResult == nil {
		hc.mu.RUnlock()
		return nil
	}
	hc.mu.RUnlock()

	// Slow path: write lock for probe
	hc.mu.Lock()
	defer hc.mu.Unlock()

	// Double-check after acquiring write lock (avoid duplicate probes)
	if time.Since(hc.lastCheck) < hc.cacheDuration && hc.lastResult == nil {
		return nil
	}

	// Perform health probe: check AGM socket (our primary socket)
	ctx, cancel := context.WithTimeout(context.Background(), hc.probeTimeout)
	defer cancel()

	// Check AGM socket specifically (not default tmux socket)
	socketPath := GetSocketPath()
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "list-sessions")
	err := cmd.Run()

	// Update cache
	hc.lastCheck = time.Now()

	// Check if timeout occurred
	if ctx.Err() == context.DeadlineExceeded {
		hc.lastResult = &HealthCheckError{
			Problem:  "tmux server not responding (may be hung)",
			Recovery: "Recovery:\n  pkill -9 tmux    # Kill hung tmux server",
		}
		return hc.lastResult
	}

	// Store result (nil for success, error for failure)
	// Note: exit code 1 means no sessions, which is OK (server is responsive)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		hc.lastResult = nil
		return nil
	}

	hc.lastResult = err
	return err
}

// InvalidateCache forces the next Check to perform a fresh probe
func (hc *HealthChecker) InvalidateCache() {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.lastCheck = time.Time{} // Zero time = cache invalid
}
