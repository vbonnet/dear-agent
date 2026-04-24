//go:build integration
// +build integration

package sandbox_test

import (
	"context"
	"errors"
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

// TestConcurrentCreate_10 creates 10 sandboxes in parallel.
// Verifies no resource conflicts and all sandboxes are functional.
func TestConcurrentCreate_10(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	const numSandboxes = 10
	testConcurrentCreate(t, provider, numSandboxes)
}

// TestConcurrentCreate_50 creates 50 sandboxes in parallel.
// Tests higher concurrency levels for resource management.
func TestConcurrentCreate_50(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	if testing.Short() {
		t.Skip("Skipping 50-sandbox test in short mode")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	const numSandboxes = 50
	testConcurrentCreate(t, provider, numSandboxes)
}

// testConcurrentCreate is a helper that creates N sandboxes concurrently.
func testConcurrentCreate(t *testing.T, provider sandbox.Provider, numSandboxes int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Track resource usage before test
	fdsBefore := countOpenFileDescriptors(t)
	mountsBefore := countMounts(t)

	// Create shared lower directory for all sandboxes
	lowerDir := t.TempDir()
	testFile := filepath.Join(lowerDir, "test.txt")
	err := os.WriteFile(testFile, []byte("shared content"), 0644)
	require.NoError(t, err)

	// Use errgroup for concurrent operations with error handling
	g, gctx := errgroup.WithContext(ctx)
	sandboxes := make([]*sandbox.Sandbox, numSandboxes)
	var successCount atomic.Int64

	// Create sandboxes concurrently
	for i := 0; i < numSandboxes; i++ {
		i := i // Capture loop variable
		g.Go(func() error {
			workspaceDir := t.TempDir()

			sb, err := provider.Create(gctx, sandbox.SandboxRequest{
				SessionID:    fmt.Sprintf("concurrent-%d", i),
				LowerDirs:    []string{lowerDir},
				WorkspaceDir: workspaceDir,
			})
			if err != nil {
				return fmt.Errorf("create sandbox %d failed: %w", i, err)
			}

			sandboxes[i] = sb
			successCount.Add(1)
			return nil
		})
	}

	// Wait for all creates to complete
	err = g.Wait()
	if err != nil {
		// Check if error is due to mount permissions
		if strings.Contains(err.Error(), "must be superuser") ||
			strings.Contains(err.Error(), "permission denied") {
			t.Skipf("Skipping test - requires mount permissions: %v", err)
		}
		require.NoError(t, err, "All sandbox creates should succeed")
	}
	assert.Equal(t, int64(numSandboxes), successCount.Load(),
		"All sandboxes should be created successfully")

	// Verify all sandboxes are unique and functional
	t.Run("VerifyUniqueness", func(t *testing.T) {
		mergedPaths := make(map[string]bool)
		upperPaths := make(map[string]bool)

		for i, sb := range sandboxes {
			require.NotNil(t, sb, "Sandbox %d should not be nil", i)

			// Check for duplicate paths (would indicate collision)
			if mergedPaths[sb.MergedPath] {
				t.Errorf("Duplicate merged path detected: %s", sb.MergedPath)
			}
			mergedPaths[sb.MergedPath] = true

			if upperPaths[sb.UpperPath] {
				t.Errorf("Duplicate upper path detected: %s", sb.UpperPath)
			}
			upperPaths[sb.UpperPath] = true

			// Verify paths exist
			_, err := os.Stat(sb.MergedPath)
			assert.NoError(t, err, "Sandbox %d merged path should exist", i)

			// Verify test file is visible
			mergedTestFile := filepath.Join(sb.MergedPath, "test.txt")
			content, err := os.ReadFile(mergedTestFile)
			if err == nil {
				assert.Equal(t, "shared content", string(content),
					"Sandbox %d should see shared content", i)
			}
		}
	})

	// Verify all sandboxes can be validated concurrently
	t.Run("ConcurrentValidation", func(t *testing.T) {
		g, gctx := errgroup.WithContext(ctx)

		for i, sb := range sandboxes {
			i, sb := i, sb
			g.Go(func() error {
				if err := provider.Validate(gctx, sb.ID); err != nil {
					return fmt.Errorf("validate sandbox %d failed: %w", i, err)
				}
				return nil
			})
		}

		err := g.Wait()
		assert.NoError(t, err, "All validations should succeed")
	})

	// Clean up all sandboxes concurrently
	t.Run("ConcurrentCleanup", func(t *testing.T) {
		g, gctx := errgroup.WithContext(ctx)

		for i, sb := range sandboxes {
			i, sb := i, sb
			g.Go(func() error {
				if err := provider.Destroy(gctx, sb.ID); err != nil {
					return fmt.Errorf("destroy sandbox %d failed: %w", i, err)
				}
				return nil
			})
		}

		err := g.Wait()
		assert.NoError(t, err, "All destroys should succeed")
	})

	// Verify resource cleanup
	t.Run("ResourceCleanup", func(t *testing.T) {
		// Wait a bit for kernel cleanup
		time.Sleep(500 * time.Millisecond)

		fdsAfter := countOpenFileDescriptors(t)
		mountsAfter := countMounts(t)

		// Allow some tolerance for file descriptors (kernel may hold some temporarily)
		fdDelta := fdsAfter - fdsBefore
		if fdDelta > 10 {
			t.Logf("WARNING: File descriptor leak detected: before=%d, after=%d, delta=%d",
				fdsBefore, fdsAfter, fdDelta)
		}

		// Mounts should be completely cleaned up
		mountDelta := mountsAfter - mountsBefore
		assert.Equal(t, 0, mountDelta,
			"All mounts should be cleaned up: before=%d, after=%d", mountsBefore, mountsAfter)

		t.Logf("Resource cleanup verified: FDs delta=%d, mounts delta=%d", fdDelta, mountDelta)
	})
}

// TestConcurrentOperations tests mixed create/destroy operations.
// Simulates real-world usage with concurrent creates, validates, and destroys.
func TestConcurrentOperations(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	if testing.Short() {
		t.Skip("Skipping mixed operations test in short mode")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	lowerDir := t.TempDir()
	testFile := filepath.Join(lowerDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	require.NoError(t, err)

	const numOperations = 30
	var createCount, destroyCount atomic.Int64

	g, gctx := errgroup.WithContext(ctx)
	var mu sync.Mutex
	activeSandboxes := make(map[string]*sandbox.Sandbox)

	// Launch concurrent operations
	for i := 0; i < numOperations; i++ {
		i := i
		g.Go(func() error {
			// Alternate between create and destroy operations
			if i%2 == 0 {
				// Create operation
				workspaceDir := t.TempDir()
				sb, err := provider.Create(gctx, sandbox.SandboxRequest{
					SessionID:    fmt.Sprintf("mixed-%d", i),
					LowerDirs:    []string{lowerDir},
					WorkspaceDir: workspaceDir,
				})
				if err != nil {
					return fmt.Errorf("create %d failed: %w", i, err)
				}

				mu.Lock()
				activeSandboxes[sb.ID] = sb
				mu.Unlock()

				createCount.Add(1)

				// Sleep a bit to keep sandbox alive
				time.Sleep(100 * time.Millisecond)
			} else {
				// Destroy operation - pick a random sandbox to destroy
				time.Sleep(50 * time.Millisecond) // Let some sandboxes get created

				mu.Lock()
				var targetID string
				for id := range activeSandboxes {
					targetID = id
					break
				}
				if targetID != "" {
					delete(activeSandboxes, targetID)
				}
				mu.Unlock()

				if targetID != "" {
					if err := provider.Destroy(gctx, targetID); err != nil {
						return fmt.Errorf("destroy %s failed: %w", targetID, err)
					}
					destroyCount.Add(1)
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

	t.Logf("Operations completed: creates=%d, destroys=%d",
		createCount.Load(), destroyCount.Load())

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
}

// TestConcurrentIsolation verifies sandboxes don't interfere with each other.
// Each sandbox modifies files independently without affecting others.
func TestConcurrentIsolation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create shared lower directory
	lowerDir := t.TempDir()
	sharedFile := filepath.Join(lowerDir, "shared.txt")
	originalContent := []byte("original shared content")
	err = os.WriteFile(sharedFile, originalContent, 0644)
	require.NoError(t, err)

	const numSandboxes = 20
	sandboxes := make([]*sandbox.Sandbox, numSandboxes)
	g, gctx := errgroup.WithContext(ctx)

	// Create all sandboxes
	for i := 0; i < numSandboxes; i++ {
		i := i
		g.Go(func() error {
			workspaceDir := t.TempDir()
			sb, err := provider.Create(gctx, sandbox.SandboxRequest{
				SessionID:    fmt.Sprintf("isolation-%d", i),
				LowerDirs:    []string{lowerDir},
				WorkspaceDir: workspaceDir,
			})
			if err != nil {
				return err
			}
			sandboxes[i] = sb
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
	require.NoError(t, err, "All sandboxes should be created")
	defer func() {
		for _, sb := range sandboxes {
			if sb != nil {
				_ = provider.Destroy(ctx, sb.ID)
			}
		}
	}()

	// Modify files concurrently in each sandbox
	g, gctx = errgroup.WithContext(ctx)
	for i := 0; i < numSandboxes; i++ {
		i := i
		sb := sandboxes[i]
		g.Go(func() error {
			// Each sandbox writes unique content
			mergedFile := filepath.Join(sb.MergedPath, "shared.txt")
			uniqueContent := []byte(fmt.Sprintf("modified by sandbox %d", i))
			return os.WriteFile(mergedFile, uniqueContent, 0644)
		})
	}

	err = g.Wait()
	require.NoError(t, err, "All modifications should succeed")

	// Verify each sandbox has its own unique content
	for i, sb := range sandboxes {
		mergedFile := filepath.Join(sb.MergedPath, "shared.txt")
		content, err := os.ReadFile(mergedFile)
		require.NoError(t, err, "Sandbox %d should be readable", i)

		expectedContent := fmt.Sprintf("modified by sandbox %d", i)
		assert.Equal(t, expectedContent, string(content),
			"Sandbox %d should have unique content", i)
	}

	// Verify original file is unchanged
	content, err := os.ReadFile(sharedFile)
	require.NoError(t, err)
	assert.Equal(t, originalContent, content, "Original file should be unchanged")
}

// TestConcurrentCleanup verifies all resources are properly cleaned up
// after concurrent sandbox operations.
func TestConcurrentCleanup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	lowerDir := t.TempDir()
	workspaceBase := t.TempDir()

	const numSandboxes = 25
	sandboxes := make([]*sandbox.Sandbox, numSandboxes)

	// Record initial state
	fdsBefore := countOpenFileDescriptors(t)
	mountsBefore := countMounts(t)

	// Create sandboxes
	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < numSandboxes; i++ {
		i := i
		g.Go(func() error {
			workspaceDir := filepath.Join(workspaceBase, fmt.Sprintf("sandbox-%d", i))
			if err := os.MkdirAll(workspaceDir, 0755); err != nil {
				return err
			}

			sb, err := provider.Create(gctx, sandbox.SandboxRequest{
				SessionID:    fmt.Sprintf("cleanup-%d", i),
				LowerDirs:    []string{lowerDir},
				WorkspaceDir: workspaceDir,
			})
			if err != nil {
				return err
			}
			sandboxes[i] = sb
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
	require.NoError(t, err)

	// Verify sandboxes are active
	mountsDuring := countMounts(t)
	t.Logf("Mounts during test: before=%d, during=%d, delta=%d",
		mountsBefore, mountsDuring, mountsDuring-mountsBefore)

	// Destroy all sandboxes concurrently
	g, gctx = errgroup.WithContext(ctx)
	for i := 0; i < numSandboxes; i++ {
		i := i
		sb := sandboxes[i]
		g.Go(func() error {
			return provider.Destroy(gctx, sb.ID)
		})
	}

	err = g.Wait()
	assert.NoError(t, err, "All cleanups should succeed")

	// Verify complete cleanup
	time.Sleep(500 * time.Millisecond) // Allow kernel cleanup time

	fdsAfter := countOpenFileDescriptors(t)
	mountsAfter := countMounts(t)

	// Check for resource leaks
	fdDelta := fdsAfter - fdsBefore
	mountDelta := mountsAfter - mountsBefore

	if fdDelta > 10 {
		t.Logf("WARNING: Possible file descriptor leak: delta=%d", fdDelta)
	}

	assert.Equal(t, 0, mountDelta,
		"All mounts should be cleaned up: before=%d, after=%d", mountsBefore, mountsAfter)

	// Verify directories are cleaned up
	for i := 0; i < numSandboxes; i++ {
		sb := sandboxes[i]
		_, err := os.Stat(sb.MergedPath)
		if !os.IsNotExist(err) {
			t.Errorf("Sandbox %d merged path still exists: %s", i, sb.MergedPath)
		}
	}

	t.Logf("Cleanup verification: FDs delta=%d, mounts delta=%d", fdDelta, mountDelta)
}

// TestRaceConditions runs with -race flag to detect data races.
// This test primarily validates the provider's internal state management.
func TestRaceConditions(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux only")
	}

	provider, err := sandbox.NewProviderForPlatform("overlayfs")
	if err != nil {
		t.Skipf("OverlayFS not available: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	lowerDir := t.TempDir()

	const numGoroutines = 20
	const operationsPerGoroutine = 10

	g, gctx := errgroup.WithContext(ctx)

	// Launch goroutines that perform rapid create/validate/destroy cycles
	for i := 0; i < numGoroutines; i++ {
		i := i
		g.Go(func() error {
			for j := 0; j < operationsPerGoroutine; j++ {
				workspaceDir := t.TempDir()
				sessionID := fmt.Sprintf("race-%d-%d", i, j)

				// Create
				sb, err := provider.Create(gctx, sandbox.SandboxRequest{
					SessionID:    sessionID,
					LowerDirs:    []string{lowerDir},
					WorkspaceDir: workspaceDir,
				})
				if err != nil {
					return fmt.Errorf("create failed: %w", err)
				}

				// Validate
				if err := provider.Validate(gctx, sb.ID); err != nil {
					return fmt.Errorf("validate failed: %w", err)
				}

				// Destroy
				if err := provider.Destroy(gctx, sb.ID); err != nil {
					return fmt.Errorf("destroy failed: %w", err)
				}

				// Validate after destroy (should fail)
				err = provider.Validate(gctx, sb.ID)
				if err == nil {
					return fmt.Errorf("validate should fail after destroy")
				}

				var sbErr *sandbox.Error
				if !errors.As(err, &sbErr) || sbErr.Code != sandbox.ErrCodeSandboxNotFound {
					return fmt.Errorf("expected ErrCodeSandboxNotFound, got: %v", err)
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
	assert.NoError(t, err, "All race test operations should succeed")

	t.Logf("Race test completed: %d goroutines × %d operations = %d total operations",
		numGoroutines, operationsPerGoroutine, numGoroutines*operationsPerGoroutine)
}

// Helper functions

// countOpenFileDescriptors returns the number of open file descriptors for this process.
func countOpenFileDescriptors(t *testing.T) int {
	t.Helper()

	fdDir := fmt.Sprintf("/proc/%d/fd", os.Getpid())
	entries, err := os.ReadDir(fdDir)
	if err != nil {
		t.Logf("Warning: could not read fd directory: %v", err)
		return 0
	}

	return len(entries)
}

// countMounts returns the number of mounts in /proc/mounts.
func countMounts(t *testing.T) int {
	t.Helper()

	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		t.Logf("Warning: could not read /proc/mounts: %v", err)
		return 0
	}

	count := 0
	for _, line := range string(data) {
		if line == '\n' {
			count++
		}
	}

	return count
}
