//go:build integration
// +build integration

package sandbox_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
	"golang.org/x/sync/errgroup"
)

// LoadTestMetrics tracks performance metrics during load tests.
type LoadTestMetrics struct {
	mu               sync.Mutex
	creationTimes    []time.Duration
	validationTimes  []time.Duration
	destroyTimes     []time.Duration
	peakFDs          int
	peakMounts       int
	peakMemoryBytes  uint64
	totalCreated     int64
	totalDestroyed   int64
	totalErrors      int64
	concurrencyLevel int
	startTime        time.Time
	endTime          time.Time
}

// RecordCreation records a successful sandbox creation.
func (m *LoadTestMetrics) RecordCreation(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creationTimes = append(m.creationTimes, duration)
	atomic.AddInt64(&m.totalCreated, 1)
}

// RecordValidation records a validation operation.
func (m *LoadTestMetrics) RecordValidation(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validationTimes = append(m.validationTimes, duration)
}

// RecordDestroy records a successful sandbox destruction.
func (m *LoadTestMetrics) RecordDestroy(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyTimes = append(m.destroyTimes, duration)
	atomic.AddInt64(&m.totalDestroyed, 1)
}

// RecordError increments the error counter.
func (m *LoadTestMetrics) RecordError() {
	atomic.AddInt64(&m.totalErrors, 1)
}

// UpdatePeakResources updates peak resource usage.
func (m *LoadTestMetrics) UpdatePeakResources(fds, mounts int, memBytes uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if fds > m.peakFDs {
		m.peakFDs = fds
	}
	if mounts > m.peakMounts {
		m.peakMounts = mounts
	}
	if memBytes > m.peakMemoryBytes {
		m.peakMemoryBytes = memBytes
	}
}

// Report generates a performance report.
func (m *LoadTestMetrics) Report(t *testing.T) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()

	totalDuration := m.endTime.Sub(m.startTime)

	t.Logf("\n=== Load Test Results (Concurrency: %d) ===", m.concurrencyLevel)
	t.Logf("Total Duration: %v", totalDuration)
	t.Logf("Created: %d, Destroyed: %d, Errors: %d",
		m.totalCreated, m.totalDestroyed, m.totalErrors)

	if len(m.creationTimes) > 0 {
		avg, p50, p95, p99 := calculatePercentiles(m.creationTimes)
		t.Logf("Creation Times: avg=%v, p50=%v, p95=%v, p99=%v",
			avg, p50, p95, p99)
	}

	if len(m.validationTimes) > 0 {
		avg, p50, p95, p99 := calculatePercentiles(m.validationTimes)
		t.Logf("Validation Times: avg=%v, p50=%v, p95=%v, p99=%v",
			avg, p50, p95, p99)
	}

	if len(m.destroyTimes) > 0 {
		avg, p50, p95, p99 := calculatePercentiles(m.destroyTimes)
		t.Logf("Destroy Times: avg=%v, p50=%v, p95=%v, p99=%v",
			avg, p50, p95, p99)
	}

	t.Logf("Peak Resources: FDs=%d, Mounts=%d, Memory=%d MB",
		m.peakFDs, m.peakMounts, m.peakMemoryBytes/(1024*1024))

	throughput := float64(m.totalCreated) / totalDuration.Seconds()
	t.Logf("Throughput: %.2f sandboxes/second", throughput)
}

// TestLoadTest_50Sandboxes creates and validates 50 sandboxes concurrently.
// This is the primary scale test for AGM sandbox swarm.
func TestLoadTest_50Sandboxes(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	if testing.Short() {
		t.Skip("Skipping load test in short mode")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	const numSandboxes = 50
	testLoadWithConcurrency(t, provider, numSandboxes)
}

// TestLoadTest_100Sandboxes stress tests with 100 concurrent sandboxes.
// This tests behavior at extreme concurrency levels.
func TestLoadTest_100Sandboxes(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	const numSandboxes = 100
	testLoadWithConcurrency(t, provider, numSandboxes)
}

// testLoadWithConcurrency is the core load test implementation.
// It creates N sandboxes concurrently, validates them, and cleans up while
// tracking detailed performance metrics.
func testLoadWithConcurrency(t *testing.T, provider sandbox.Provider, numSandboxes int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Initialize metrics
	metrics := &LoadTestMetrics{
		concurrencyLevel: numSandboxes,
		startTime:        time.Now(),
	}

	// Record baseline resources
	fdsBefore := countOpenFileDescriptors(t)
	mountsBefore := countMounts(t)
	memBefore := getMemoryUsage()

	t.Logf("Baseline: FDs=%d, Mounts=%d, Memory=%d MB",
		fdsBefore, mountsBefore, memBefore/(1024*1024))

	// Create shared lower directory
	lowerDir := t.TempDir()
	testFile := filepath.Join(lowerDir, "test.txt")
	err := os.WriteFile(testFile, []byte("shared test content"), 0644)
	require.NoError(t, err)

	// Storage for created sandboxes
	sandboxes := make([]*sandbox.Sandbox, numSandboxes)
	var sandboxMu sync.Mutex

	// Phase 1: Concurrent Creation
	t.Run("ConcurrentCreation", func(t *testing.T) {
		g, gctx := errgroup.WithContext(ctx)

		for i := 0; i < numSandboxes; i++ {
			i := i // Capture loop variable
			g.Go(func() error {
				start := time.Now()

				workspaceDir := t.TempDir()
				sb, err := provider.Create(gctx, sandbox.SandboxRequest{
					SessionID:    fmt.Sprintf("load-%d", i),
					LowerDirs:    []string{lowerDir},
					WorkspaceDir: workspaceDir,
				})

				if err != nil {
					metrics.RecordError()
					return fmt.Errorf("create sandbox %d failed: %w", i, err)
				}

				duration := time.Since(start)
				metrics.RecordCreation(duration)

				sandboxMu.Lock()
				sandboxes[i] = sb
				sandboxMu.Unlock()

				// Sample resource usage every 10th sandbox
				if i%10 == 0 {
					fds := countOpenFileDescriptors(t)
					mounts := countMounts(t)
					mem := getMemoryUsage()
					metrics.UpdatePeakResources(fds, mounts, mem)
				}

				return nil
			})
		}

		err = g.Wait()
		if err != nil {
			// Check if error is due to mount permissions
			if strings.Contains(err.Error(), "must be superuser") ||
				strings.Contains(err.Error(), "permission denied") {
				t.Skipf("Skipping test - requires mount permissions: %v", err)
			}
			require.NoError(t, err, "All sandbox creates should succeed")
		}

		assert.Equal(t, int64(numSandboxes), atomic.LoadInt64(&metrics.totalCreated),
			"All sandboxes should be created")

		t.Logf("Created %d sandboxes successfully", numSandboxes)
	})

	// Skip remaining phases if no sandboxes were created
	if atomic.LoadInt64(&metrics.totalCreated) == 0 {
		t.Skip("No sandboxes created, skipping remaining phases")
	}

	// Phase 2: Concurrent Validation
	t.Run("ConcurrentValidation", func(t *testing.T) {
		g, gctx := errgroup.WithContext(ctx)

		for i := 0; i < numSandboxes; i++ {
			i := i
			sb := sandboxes[i]
			g.Go(func() error {
				start := time.Now()

				if err := provider.Validate(gctx, sb.ID); err != nil {
					return fmt.Errorf("validate sandbox %d failed: %w", i, err)
				}

				duration := time.Since(start)
				metrics.RecordValidation(duration)

				// Verify sandbox is functional
				mergedTestFile := filepath.Join(sb.MergedPath, "test.txt")
				content, err := os.ReadFile(mergedTestFile)
				if err != nil {
					return fmt.Errorf("read test file in sandbox %d failed: %w", i, err)
				}

				if string(content) != "shared test content" {
					return fmt.Errorf("sandbox %d has wrong content", i)
				}

				return nil
			})
		}

		err := g.Wait()
		assert.NoError(t, err, "All validations should succeed")
		t.Logf("Validated %d sandboxes successfully", numSandboxes)
	})

	// Phase 3: Check for path uniqueness (no collisions)
	t.Run("VerifyUniqueness", func(t *testing.T) {
		mergedPaths := make(map[string]bool)
		upperPaths := make(map[string]bool)

		for i, sb := range sandboxes {
			require.NotNil(t, sb, "Sandbox %d should not be nil", i)

			if mergedPaths[sb.MergedPath] {
				t.Errorf("Duplicate merged path detected: %s", sb.MergedPath)
			}
			mergedPaths[sb.MergedPath] = true

			if upperPaths[sb.UpperPath] {
				t.Errorf("Duplicate upper path detected: %s", sb.UpperPath)
			}
			upperPaths[sb.UpperPath] = true
		}

		t.Logf("All %d sandboxes have unique paths", numSandboxes)
	})

	// Phase 4: Test concurrent writes (isolation check)
	t.Run("ConcurrentWrites", func(t *testing.T) {
		g, _ := errgroup.WithContext(ctx)

		for i := 0; i < numSandboxes; i++ {
			i := i
			sb := sandboxes[i]
			g.Go(func() error {
				// Write unique content in each sandbox
				testFile := filepath.Join(sb.MergedPath, "unique.txt")
				content := []byte(fmt.Sprintf("sandbox-%d-content", i))
				return os.WriteFile(testFile, content, 0644)
			})
		}

		err := g.Wait()
		assert.NoError(t, err, "All writes should succeed")

		// Verify each sandbox has its unique content
		for i, sb := range sandboxes {
			testFile := filepath.Join(sb.MergedPath, "unique.txt")
			content, err := os.ReadFile(testFile)
			require.NoError(t, err, "Read unique file in sandbox %d", i)
			expected := fmt.Sprintf("sandbox-%d-content", i)
			assert.Equal(t, expected, string(content),
				"Sandbox %d should have unique content", i)
		}

		t.Logf("Verified isolation across %d sandboxes", numSandboxes)
	})

	// Record peak resources after all operations
	fdsAfterOps := countOpenFileDescriptors(t)
	mountsAfterOps := countMounts(t)
	memAfterOps := getMemoryUsage()
	metrics.UpdatePeakResources(fdsAfterOps, mountsAfterOps, memAfterOps)

	t.Logf("After operations: FDs=%d, Mounts=%d, Memory=%d MB",
		fdsAfterOps, mountsAfterOps, memAfterOps/(1024*1024))

	// Phase 5: Concurrent Cleanup
	t.Run("ConcurrentCleanup", func(t *testing.T) {
		g, gctx := errgroup.WithContext(ctx)

		for i := 0; i < numSandboxes; i++ {
			i := i
			sb := sandboxes[i]
			g.Go(func() error {
				start := time.Now()

				if err := provider.Destroy(gctx, sb.ID); err != nil {
					return fmt.Errorf("destroy sandbox %d failed: %w", i, err)
				}

				duration := time.Since(start)
				metrics.RecordDestroy(duration)
				return nil
			})
		}

		err := g.Wait()
		assert.NoError(t, err, "All destroys should succeed")
		t.Logf("Destroyed %d sandboxes successfully", numSandboxes)
	})

	// Phase 6: Verify resource cleanup
	t.Run("ResourceCleanup", func(t *testing.T) {
		// Allow kernel time to clean up
		time.Sleep(1 * time.Second)

		fdsAfter := countOpenFileDescriptors(t)
		mountsAfter := countMounts(t)
		memAfter := getMemoryUsage()

		fdDelta := fdsAfter - fdsBefore
		mountDelta := mountsAfter - mountsBefore
		memDelta := int64(memAfter) - int64(memBefore)

		// Allow some tolerance for file descriptors
		if fdDelta > 20 {
			t.Logf("WARNING: Possible file descriptor leak: before=%d, after=%d, delta=%d",
				fdsBefore, fdsAfter, fdDelta)
		}

		// Mounts should be completely cleaned up
		assert.Equal(t, 0, mountDelta,
			"All mounts should be cleaned up: before=%d, after=%d", mountsBefore, mountsAfter)

		// Memory should not grow excessively (allow 50MB growth for allocations)
		if memDelta > 50*1024*1024 {
			t.Logf("WARNING: Significant memory growth: delta=%d MB", memDelta/(1024*1024))
		}

		t.Logf("Resource cleanup: FDs delta=%d, mounts delta=%d, memory delta=%d MB",
			fdDelta, mountDelta, memDelta/(1024*1024))
	})

	// Finalize metrics and generate report
	metrics.endTime = time.Now()
	metrics.Report(t)
}

// TestLoadTest_ConcurrentWorkload tests mixed create/validate/destroy operations
// under high concurrency. This simulates real-world AGM swarm behavior.
func TestLoadTest_ConcurrentWorkload(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	if testing.Short() {
		t.Skip("Skipping concurrent workload test in short mode")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	lowerDir := t.TempDir()
	testFile := filepath.Join(lowerDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Track active sandboxes with thread-safe map
	var mu sync.Mutex
	activeSandboxes := make(map[string]*sandbox.Sandbox)

	const numWorkers = 50
	const opsPerWorker = 10

	metrics := &LoadTestMetrics{
		concurrencyLevel: numWorkers,
		startTime:        time.Now(),
	}

	g, gctx := errgroup.WithContext(ctx)

	// Launch workers that perform mixed operations
	for i := 0; i < numWorkers; i++ {
		i := i
		g.Go(func() error {
			for j := 0; j < opsPerWorker; j++ {
				// Alternate between create and destroy
				if j%2 == 0 || len(activeSandboxes) == 0 {
					// Create operation
					start := time.Now()
					workspaceDir := t.TempDir()
					sb, err := provider.Create(gctx, sandbox.SandboxRequest{
						SessionID:    fmt.Sprintf("worker-%d-%d", i, j),
						LowerDirs:    []string{lowerDir},
						WorkspaceDir: workspaceDir,
					})
					if err != nil {
						metrics.RecordError()
						return fmt.Errorf("create failed: %w", err)
					}

					metrics.RecordCreation(time.Since(start))

					mu.Lock()
					activeSandboxes[sb.ID] = sb
					mu.Unlock()

					// Quick validation
					start = time.Now()
					if err := provider.Validate(gctx, sb.ID); err != nil {
						return fmt.Errorf("validate failed: %w", err)
					}
					metrics.RecordValidation(time.Since(start))

				} else {
					// Destroy operation - pick a random sandbox
					mu.Lock()
					var targetID string
					var targetSB *sandbox.Sandbox
					for id, sb := range activeSandboxes {
						targetID = id
						targetSB = sb
						break
					}
					if targetID != "" {
						delete(activeSandboxes, targetID)
					}
					mu.Unlock()

					if targetID != "" {
						start := time.Now()
						if err := provider.Destroy(gctx, targetID); err != nil {
							metrics.RecordError()
							return fmt.Errorf("destroy %s failed: %w", targetID, err)
						}
						metrics.RecordDestroy(time.Since(start))

						// Verify paths are cleaned up
						if _, err := os.Stat(targetSB.MergedPath); !os.IsNotExist(err) {
							return fmt.Errorf("sandbox path still exists after destroy: %s",
								targetSB.MergedPath)
						}
					}
				}

				// Sample resources periodically
				if j%5 == 0 {
					fds := countOpenFileDescriptors(t)
					mounts := countMounts(t)
					mem := getMemoryUsage()
					metrics.UpdatePeakResources(fds, mounts, mem)
				}
			}
			return nil
		})
	}

	err = g.Wait()
	if err != nil {
		// Check if error is due to mount permissions
		if strings.Contains(err.Error(), "must be superuser") ||
			strings.Contains(err.Error(), "permission denied") {
			t.Skipf("Skipping test - requires mount permissions: %v", err)
		}
	}
	assert.NoError(t, err, "All mixed operations should succeed")

	// Clean up remaining sandboxes
	mu.Lock()
	remaining := make([]*sandbox.Sandbox, 0, len(activeSandboxes))
	for _, sb := range activeSandboxes {
		remaining = append(remaining, sb)
	}
	mu.Unlock()

	for _, sb := range remaining {
		_ = provider.Destroy(ctx, sb.ID)
	}

	metrics.endTime = time.Now()
	metrics.Report(t)
}

// TestLoadTest_ResourceExhaustion tests system behavior when approaching limits.
// This validates graceful degradation and error handling.
func TestLoadTest_ResourceExhaustion(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	if testing.Short() {
		t.Skip("Skipping resource exhaustion test in short mode")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	lowerDir := t.TempDir()

	// Track sandboxes for cleanup
	var mu sync.Mutex
	allSandboxes := make([]*sandbox.Sandbox, 0, 200)

	// Try to create sandboxes until we hit a limit or reach 200
	const maxSandboxes = 200
	var createErrors atomic.Int64
	var successCount atomic.Int64

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(20) // Limit concurrency to avoid overwhelming system

	for i := 0; i < maxSandboxes; i++ {
		i := i
		g.Go(func() error {
			workspaceDir := t.TempDir()
			sb, err := provider.Create(gctx, sandbox.SandboxRequest{
				SessionID:    fmt.Sprintf("exhaust-%d", i),
				LowerDirs:    []string{lowerDir},
				WorkspaceDir: workspaceDir,
			})

			if err != nil {
				createErrors.Add(1)
				// Don't fail test - we expect errors near limits
				t.Logf("Create %d failed (expected): %v", i, err)
				return nil
			}

			successCount.Add(1)

			mu.Lock()
			allSandboxes = append(allSandboxes, sb)
			mu.Unlock()

			return nil
		})
	}

	err = g.Wait()
	if err != nil {
		if strings.Contains(err.Error(), "must be superuser") ||
			strings.Contains(err.Error(), "permission denied") {
			t.Skipf("Skipping test - requires mount permissions: %v", err)
		}
	}

	created := successCount.Load()
	errors := createErrors.Load()

	t.Logf("Resource exhaustion test: created=%d, errors=%d", created, errors)

	// Verify system is still healthy by creating one more sandbox
	t.Run("VerifySystemHealth", func(t *testing.T) {
		// Clean up half the sandboxes first
		mu.Lock()
		toCleanup := allSandboxes[:len(allSandboxes)/2]
		allSandboxes = allSandboxes[len(allSandboxes)/2:]
		mu.Unlock()

		for _, sb := range toCleanup {
			_ = provider.Destroy(ctx, sb.ID)
		}

		// Wait for cleanup
		time.Sleep(1 * time.Second)

		// Try creating a new sandbox
		workspaceDir := t.TempDir()
		sb, err := provider.Create(ctx, sandbox.SandboxRequest{
			SessionID:    "health-check",
			LowerDirs:    []string{lowerDir},
			WorkspaceDir: workspaceDir,
		})

		if err != nil && (strings.Contains(err.Error(), "must be superuser") ||
			strings.Contains(err.Error(), "permission denied")) {
			t.Skip("Mount permissions required")
		}

		assert.NoError(t, err, "System should recover after cleanup")

		if sb != nil {
			_ = provider.Destroy(ctx, sb.ID)
		}
	})

	// Clean up all remaining sandboxes
	mu.Lock()
	remaining := allSandboxes
	mu.Unlock()

	for _, sb := range remaining {
		_ = provider.Destroy(ctx, sb.ID)
	}

	t.Logf("Resource exhaustion test completed successfully")
}

// TestLoadTest_PerformanceDegradation measures operation times at different
// concurrency levels to identify performance degradation points.
func TestLoadTest_PerformanceDegradation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	if testing.Short() {
		t.Skip("Skipping performance degradation test in short mode")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	// Test at different concurrency levels
	concurrencyLevels := []int{10, 25, 50, 75, 100}

	results := make(map[int]*LoadTestMetrics)

	for _, level := range concurrencyLevels {
		t.Run(fmt.Sprintf("Concurrency_%d", level), func(t *testing.T) {
			metrics := &LoadTestMetrics{
				concurrencyLevel: level,
				startTime:        time.Now(),
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			lowerDir := t.TempDir()
			sandboxes := make([]*sandbox.Sandbox, level)

			// Create all sandboxes
			g, gctx := errgroup.WithContext(ctx)
			for i := 0; i < level; i++ {
				i := i
				g.Go(func() error {
					start := time.Now()
					workspaceDir := t.TempDir()
					sb, err := provider.Create(gctx, sandbox.SandboxRequest{
						SessionID:    fmt.Sprintf("perf-%d-%d", level, i),
						LowerDirs:    []string{lowerDir},
						WorkspaceDir: workspaceDir,
					})
					if err != nil {
						return err
					}

					metrics.RecordCreation(time.Since(start))
					sandboxes[i] = sb
					return nil
				})
			}

			err = g.Wait()
			if err != nil {
				if strings.Contains(err.Error(), "must be superuser") ||
					strings.Contains(err.Error(), "permission denied") {
					t.Skip("Mount permissions required")
				}
				t.Fatalf("Failed to create sandboxes: %v", err)
			}

			// Destroy all sandboxes
			g, gctx = errgroup.WithContext(ctx)
			for i := 0; i < level; i++ {
				i := i
				sb := sandboxes[i]
				g.Go(func() error {
					start := time.Now()
					err := provider.Destroy(gctx, sb.ID)
					if err != nil {
						return err
					}
					metrics.RecordDestroy(time.Since(start))
					return nil
				})
			}

			_ = g.Wait()

			metrics.endTime = time.Now()
			results[level] = metrics
			metrics.Report(t)
		})
	}

	// Analyze degradation across concurrency levels
	t.Run("AnalyzeDegradation", func(t *testing.T) {
		t.Logf("\n=== Performance Degradation Analysis ===")

		var prevAvgCreate time.Duration
		for _, level := range concurrencyLevels {
			metrics := results[level]
			if metrics == nil || len(metrics.creationTimes) == 0 {
				continue
			}

			avgCreate, _, _, _ := calculatePercentiles(metrics.creationTimes)

			degradation := ""
			if prevAvgCreate > 0 {
				pct := float64(avgCreate-prevAvgCreate) / float64(prevAvgCreate) * 100
				degradation = fmt.Sprintf(" (%.1f%% change)", pct)
			}

			t.Logf("Concurrency %d: avg creation=%v%s",
				level, avgCreate, degradation)

			prevAvgCreate = avgCreate
		}
	})
}

// Helper functions

// calculatePercentiles computes average and percentile statistics from durations.
func calculatePercentiles(durations []time.Duration) (avg, p50, p95, p99 time.Duration) {
	if len(durations) == 0 {
		return 0, 0, 0, 0
	}

	// Sort durations
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)

	// Simple bubble sort (sufficient for test data)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate average
	var sum time.Duration
	for _, d := range sorted {
		sum += d
	}
	avg = sum / time.Duration(len(sorted))

	// Calculate percentiles
	p50 = sorted[len(sorted)*50/100]
	p95 = sorted[len(sorted)*95/100]
	p99 = sorted[len(sorted)*99/100]

	return avg, p50, p95, p99
}

// getMemoryUsage returns current process memory usage in bytes.
func getMemoryUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// Return allocated heap memory
	return m.Alloc
}
