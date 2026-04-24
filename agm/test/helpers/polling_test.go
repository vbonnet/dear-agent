package helpers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoll_ImmediateSuccess(t *testing.T) {
	called := false
	err := Poll(context.Background(), DefaultPollConfig(), func() (bool, error) {
		called = true
		return true, nil
	})

	require.NoError(t, err)
	assert.True(t, called, "Condition should be called at least once")
}

func TestPoll_EventualSuccess(t *testing.T) {
	attempts := 0
	err := Poll(context.Background(), DefaultPollConfig(), func() (bool, error) {
		attempts++
		return attempts >= 3, nil
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, attempts, 3, "Should retry until condition succeeds")
}

func TestPoll_Timeout(t *testing.T) {
	config := PollConfig{
		Timeout:  200 * time.Millisecond,
		Interval: 50 * time.Millisecond,
	}

	start := time.Now()
	err := Poll(context.Background(), config, func() (bool, error) {
		return false, nil // Never succeeds
	})

	elapsed := time.Since(start)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "polling timeout")
	assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond)
}

func TestPoll_ConditionError(t *testing.T) {
	expectedErr := errors.New("condition failed")
	err := Poll(context.Background(), DefaultPollConfig(), func() (bool, error) {
		return false, expectedErr
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "condition check failed")
}

func TestPoll_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	err := Poll(ctx, DefaultPollConfig(), func() (bool, error) {
		return false, nil // Never succeeds
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "polling cancelled")
}

func TestPollUntil_Success(t *testing.T) {
	attempts := 0
	err := PollUntil(func() (bool, error) {
		attempts++
		return attempts >= 2, nil
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, attempts, 2)
}

func TestPollUntilWithTimeout_Success(t *testing.T) {
	attempts := 0
	err := PollUntilWithTimeout(1*time.Second, func() (bool, error) {
		attempts++
		return attempts >= 2, nil
	})

	require.NoError(t, err)
	assert.GreaterOrEqual(t, attempts, 2)
}

func TestPollUntilWithTimeout_Timeout(t *testing.T) {
	err := PollUntilWithTimeout(200*time.Millisecond, func() (bool, error) {
		return false, nil // Never succeeds
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "polling timeout")
}

func TestDefaultPollConfig(t *testing.T) {
	config := DefaultPollConfig()
	assert.Equal(t, 5*time.Second, config.Timeout)
	assert.Equal(t, 100*time.Millisecond, config.Interval)
}

// Benchmark to verify Poll is more efficient than busy-waiting
func BenchmarkPoll(b *testing.B) {
	for i := 0; i < b.N; i++ {
		attempts := 0
		_ = Poll(context.Background(), DefaultPollConfig(), func() (bool, error) {
			attempts++
			return attempts >= 5, nil
		})
	}
}
