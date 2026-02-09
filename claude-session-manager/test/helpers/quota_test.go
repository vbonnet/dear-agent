package helpers

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAPIQuota(t *testing.T) {
	quota := GetAPIQuota()
	assert.NotNil(t, quota, "GetAPIQuota should return non-nil quota")

	// Verify it returns the same instance (singleton)
	quota2 := GetAPIQuota()
	assert.Equal(t, quota, quota2, "GetAPIQuota should return same instance")
}

func TestAPIQuota_Consume_Basic(t *testing.T) {
	quota := GetAPIQuota()
	defer quota.Reset() // Cleanup

	// First consume should succeed
	ok := quota.Consume()
	assert.True(t, ok, "First Consume should succeed")

	// Remaining should decrease
	remaining := quota.Remaining()
	assert.Equal(t, 19, remaining, "Remaining should be 19 after 1 consume")
}

func TestAPIQuota_Exhaustion(t *testing.T) {
	quota := GetAPIQuota()
	defer quota.Reset() // Cleanup

	// Consume all quota
	for i := range 20 {
		ok := quota.Consume()
		assert.True(t, ok, "Consume #%d should succeed", i+1)
	}

	// Verify remaining is 0
	remaining := quota.Remaining()
	assert.Equal(t, 0, remaining, "Remaining should be 0 after exhausting quota")

	// Next consume should fail
	ok := quota.Consume()
	assert.False(t, ok, "Consume should fail when quota exhausted")

	// Remaining should still be 0
	remaining = quota.Remaining()
	assert.Equal(t, 0, remaining, "Remaining should still be 0 after failed consume")
}

func TestAPIQuota_Reset(t *testing.T) {
	quota := GetAPIQuota()
	defer quota.Reset() // Cleanup

	// Consume some quota
	for range 5 {
		quota.Consume()
	}

	// Verify consumed
	remaining := quota.Remaining()
	assert.Equal(t, 15, remaining, "Remaining should be 15 after 5 consumes")

	// Reset quota
	quota.Reset()

	// Verify reset to full
	remaining = quota.Remaining()
	assert.Equal(t, 20, remaining, "Remaining should be 20 after reset")

	// Verify can consume again
	ok := quota.Consume()
	assert.True(t, ok, "Consume should succeed after reset")
}

func TestAPIQuota_Concurrent(t *testing.T) {
	quota := GetAPIQuota()
	defer quota.Reset() // Cleanup

	// Run 100 concurrent consumers
	const numGoroutines = 100
	var wg sync.WaitGroup
	successCount := 0
	var mu sync.Mutex

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			if quota.Consume() {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Verify exactly 20 succeeded (quota limit)
	assert.Equal(t, 20, successCount, "Exactly 20 concurrent consumes should succeed")

	// Verify remaining is 0
	remaining := quota.Remaining()
	assert.Equal(t, 0, remaining, "Remaining should be 0 after concurrent exhaustion")
}

func TestAPIQuota_Remaining(t *testing.T) {
	quota := GetAPIQuota()
	defer quota.Reset() // Cleanup

	// Initial remaining should be 20
	remaining := quota.Remaining()
	assert.Equal(t, 20, remaining, "Initial remaining should be 20")

	// Consume 3 times
	quota.Consume()
	quota.Consume()
	quota.Consume()

	// Remaining should be 17
	remaining = quota.Remaining()
	assert.Equal(t, 17, remaining, "Remaining should be 17 after 3 consumes")
}

func TestAPIQuota_ConcurrentRemaining(t *testing.T) {
	quota := GetAPIQuota()
	defer quota.Reset() // Cleanup

	// Run concurrent Remaining calls (should not panic or race)
	const numGoroutines = 50
	var wg sync.WaitGroup

	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			_ = quota.Remaining()
		}()
	}

	wg.Wait()

	// Test passes if no race detected
	assert.True(t, true, "Concurrent Remaining calls should not race")
}
