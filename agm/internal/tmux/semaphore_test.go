package tmux

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcquireTmuxSemaphore_Basic(t *testing.T) {
	// Reset semaphore for test isolation
	SetMaxConcurrentOps(5)
	defer SetMaxConcurrentOps(maxConcurrentTmuxOps)

	ctx := context.Background()

	// Should acquire without error
	err := acquireTmuxSemaphore(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, TmuxConcurrentOps())

	// Release
	releaseTmuxSemaphore()
	assert.Equal(t, 0, TmuxConcurrentOps())
}

func TestAcquireTmuxSemaphore_Exhaustion(t *testing.T) {
	// Set very small capacity
	SetMaxConcurrentOps(2)
	defer SetMaxConcurrentOps(maxConcurrentTmuxOps)

	ctx := context.Background()

	// Fill semaphore
	require.NoError(t, acquireTmuxSemaphore(ctx))
	require.NoError(t, acquireTmuxSemaphore(ctx))
	assert.Equal(t, 2, TmuxConcurrentOps())

	// Third acquire should timeout (use short context)
	shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err := acquireTmuxSemaphore(shortCtx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "overloaded")

	// Release both
	releaseTmuxSemaphore()
	releaseTmuxSemaphore()
	assert.Equal(t, 0, TmuxConcurrentOps())
}

func TestAcquireTmuxSemaphore_Concurrent(t *testing.T) {
	SetMaxConcurrentOps(10)
	defer SetMaxConcurrentOps(maxConcurrentTmuxOps)

	ctx := context.Background()
	var wg sync.WaitGroup
	var maxSeen atomic.Int32

	// Launch 20 goroutines competing for 10 slots
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := acquireTmuxSemaphore(ctx); err != nil {
				return
			}
			current := int32(TmuxConcurrentOps())
			for {
				old := maxSeen.Load()
				if current <= old || maxSeen.CompareAndSwap(old, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond) // Simulate work
			releaseTmuxSemaphore()
		}()
	}

	wg.Wait()

	// Max concurrent should never exceed capacity
	assert.LessOrEqual(t, int(maxSeen.Load()), 10,
		"Should never exceed semaphore capacity")
	assert.Equal(t, 0, TmuxConcurrentOps(),
		"All operations should be released")
}

func TestReleaseTmuxSemaphore_Safe(t *testing.T) {
	SetMaxConcurrentOps(5)
	defer SetMaxConcurrentOps(maxConcurrentTmuxOps)

	// Release without acquire should not block or panic
	releaseTmuxSemaphore()
	assert.Equal(t, 0, TmuxConcurrentOps())
}

func TestTmuxConcurrentOps(t *testing.T) {
	SetMaxConcurrentOps(5)
	defer SetMaxConcurrentOps(maxConcurrentTmuxOps)

	ctx := context.Background()
	assert.Equal(t, 0, TmuxConcurrentOps())

	acquireTmuxSemaphore(ctx)
	assert.Equal(t, 1, TmuxConcurrentOps())

	acquireTmuxSemaphore(ctx)
	assert.Equal(t, 2, TmuxConcurrentOps())

	releaseTmuxSemaphore()
	assert.Equal(t, 1, TmuxConcurrentOps())

	releaseTmuxSemaphore()
	assert.Equal(t, 0, TmuxConcurrentOps())
}
