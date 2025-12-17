package test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/lock"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
)

// BenchmarkLockAcquireRelease measures the performance of lock acquire + release cycle
func BenchmarkLockAcquireRelease(b *testing.B) {
	tmpDir := b.TempDir()
	lockPath := filepath.Join(tmpDir, "bench.lock")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l, err := lock.New(lockPath)
		if err != nil {
			b.Fatal(err)
		}
		if err := l.TryLock(); err != nil {
			b.Fatal(err)
		}
		if err := l.Unlock(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHealthCheckCached measures performance of cached health checks
func BenchmarkHealthCheckCached(b *testing.B) {
	hc := tmux.NewHealthChecker(5*time.Second, 2*time.Second)

	// Prime the cache
	hc.Check()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hc.Check()
	}
}

// BenchmarkHealthCheckUncached measures performance of uncached health checks (fresh probes)
func BenchmarkHealthCheckUncached(b *testing.B) {
	hc := tmux.NewHealthChecker(1*time.Nanosecond, 2*time.Second) // No caching

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hc.Check()
	}
}

// BenchmarkTimeoutWrapperOverhead measures the overhead of the timeout wrapper
func BenchmarkTimeoutWrapperOverhead(b *testing.B) {
	ctx := context.Background()
	timeout := 5 * time.Second

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := tmux.RunWithTimeout(ctx, timeout, "echo", "test")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLockContention measures lock performance under contention
func BenchmarkLockContention(b *testing.B) {
	tmpDir := b.TempDir()
	lockPath := filepath.Join(tmpDir, "contention.lock")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			l, err := lock.New(lockPath)
			if err != nil {
				b.Fatal(err)
			}
			// Try to acquire (will fail if someone else has it)
			l.TryLock()
			l.Unlock()
		}
	})
}

// BenchmarkHealthCheckConcurrent measures concurrent health check performance
func BenchmarkHealthCheckConcurrent(b *testing.B) {
	hc := tmux.NewHealthChecker(5*time.Second, 2*time.Second)

	// Prime the cache
	hc.Check()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			hc.Check()
		}
	})
}

// BenchmarkCommandWithTimeout_FastCommand measures timeout wrapper overhead on fast commands
func BenchmarkCommandWithTimeout_FastCommand(b *testing.B) {
	ctx := context.Background()
	timeout := 5 * time.Second

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd, cancel := tmux.CommandWithTimeout(ctx, timeout, "echo", "fast")
		err := cmd.Run()
		cancel()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLockNew measures the cost of creating a new lock object
func BenchmarkLockNew(b *testing.B) {
	tmpDir := b.TempDir()
	lockPath := filepath.Join(tmpDir, "new.lock")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l, err := lock.New(lockPath)
		if err != nil {
			b.Fatal(err)
		}
		l.Unlock()
	}
}

// BenchmarkLockTryLock_Uncontended measures lock acquisition when no contention
func BenchmarkLockTryLock_Uncontended(b *testing.B) {
	tmpDir := b.TempDir()
	lockPath := filepath.Join(tmpDir, "uncontended.lock")

	l, err := lock.New(lockPath)
	if err != nil {
		b.Fatal(err)
	}
	defer l.Unlock()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := l.TryLock(); err != nil {
			b.Fatal(err)
		}
		if err := l.Unlock(); err != nil {
			b.Fatal(err)
		}
		// Recreate to get fresh file descriptor
		l, _ = lock.New(lockPath)
	}
}

// BenchmarkHealthCheckInvalidate measures cache invalidation performance
func BenchmarkHealthCheckInvalidate(b *testing.B) {
	hc := tmux.NewHealthChecker(5*time.Second, 2*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hc.InvalidateCache()
	}
}
