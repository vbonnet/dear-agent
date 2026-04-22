package daemon

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldLog_FirstOccurrence(t *testing.T) {
	d := NewIncidentDeduplicator(15 * time.Minute)

	result := d.ShouldLog("session-1", "cursor_frozen")
	assert.True(t, result, "first occurrence should always be logged")
}

func TestShouldLog_DuplicateWithinCooldown(t *testing.T) {
	d := NewIncidentDeduplicator(15 * time.Minute)

	first := d.ShouldLog("session-1", "cursor_frozen")
	require.True(t, first)

	second := d.ShouldLog("session-1", "cursor_frozen")
	assert.False(t, second, "duplicate within cooldown should be suppressed")
}

func TestShouldLog_DuplicateAfterCooldown(t *testing.T) {
	d := NewIncidentDeduplicator(10 * time.Millisecond)

	first := d.ShouldLog("session-1", "cursor_frozen")
	require.True(t, first)

	time.Sleep(15 * time.Millisecond)

	second := d.ShouldLog("session-1", "cursor_frozen")
	assert.True(t, second, "should allow logging after cooldown expires")
}

func TestShouldLog_DifferentSymptom(t *testing.T) {
	d := NewIncidentDeduplicator(15 * time.Minute)

	first := d.ShouldLog("session-1", "cursor_frozen")
	require.True(t, first)

	second := d.ShouldLog("session-1", "high_cpu")
	assert.True(t, second, "different symptom for same session should be logged")
}

func TestShouldLog_DifferentSession(t *testing.T) {
	d := NewIncidentDeduplicator(15 * time.Minute)

	first := d.ShouldLog("session-1", "cursor_frozen")
	require.True(t, first)

	second := d.ShouldLog("session-2", "cursor_frozen")
	assert.True(t, second, "same symptom for different session should be logged")
}

func TestCleanup(t *testing.T) {
	d := NewIncidentDeduplicator(10 * time.Millisecond)

	// Log an entry
	d.ShouldLog("session-old", "cursor_frozen")

	// Wait for 2x cooldown so it becomes stale
	time.Sleep(25 * time.Millisecond)

	// Log a recent entry
	d.ShouldLog("session-new", "cursor_frozen")

	// Run cleanup
	d.Cleanup()

	// Old entry should have been purged - logging it again should return true
	result := d.ShouldLog("session-old", "cursor_frozen")
	assert.True(t, result, "old entry should have been cleaned up")

	// Recent entry should still be tracked - logging it again should return false
	result = d.ShouldLog("session-new", "cursor_frozen")
	assert.False(t, result, "recent entry should still be tracked after cleanup")
}

func TestConcurrentAccess(t *testing.T) {
	d := NewIncidentDeduplicator(15 * time.Minute)

	var wg sync.WaitGroup
	const goroutines = 50

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			session := "session-1"
			if id%2 == 0 {
				session = "session-2"
			}
			d.ShouldLog(session, "cursor_frozen")
			d.Cleanup()
		}(i)
	}

	wg.Wait()

	// If we get here without a race condition panic, the test passes.
	// Verify the deduplicator is still functional.
	result := d.ShouldLog("session-3", "new_symptom")
	assert.True(t, result, "deduplicator should still work after concurrent access")
}
