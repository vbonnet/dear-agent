package daemon

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPatternAccumulator(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	require.NotNil(t, a)
	assert.Equal(t, 30*time.Minute, a.window)
}

func TestRecord_SingleViolation(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	a.Record("session-1", "cd-command", "high", "cd /tmp")

	total := a.GetSessionTotal("session-1")
	assert.Equal(t, 1, total)
}

func TestRecord_MultipleViolations(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	a.Record("session-1", "cd-command", "high", "cd /tmp")
	a.Record("session-1", "file-operations", "high", "cat foo.txt")
	a.Record("session-1", "cd-command", "high", "cd /var")

	total := a.GetSessionTotal("session-1")
	assert.Equal(t, 3, total)
}

func TestGetFrequency(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	a.Record("session-1", "cd-command", "high", "cd /tmp")
	a.Record("session-1", "cd-command", "high", "cd /var")
	a.Record("session-1", "file-operations", "high", "cat foo.txt")

	freq := a.GetFrequency("session-1", "cd-command")
	assert.Equal(t, 2, freq)

	freq = a.GetFrequency("session-1", "file-operations")
	assert.Equal(t, 1, freq)
}

func TestGetFrequency_UnknownSession(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)

	freq := a.GetFrequency("nonexistent", "cd-command")
	assert.Equal(t, 0, freq)
}

func TestGetFrequency_OutsideWindow(t *testing.T) {
	a := NewPatternAccumulator(10 * time.Millisecond)
	a.Record("session-1", "cd-command", "high", "cd /tmp")

	time.Sleep(15 * time.Millisecond)

	freq := a.GetFrequency("session-1", "cd-command")
	assert.Equal(t, 0, freq, "violations outside window should not be counted")
}

func TestGetSessionTotal(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	a.Record("session-1", "cd-command", "high", "cd /tmp")
	a.Record("session-1", "file-operations", "high", "cat foo.txt")

	assert.Equal(t, 2, a.GetSessionTotal("session-1"))
	assert.Equal(t, 0, a.GetSessionTotal("session-2"))
}

func TestGetTopPatterns(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	a.Record("session-1", "cd-command", "high", "cd /tmp")
	a.Record("session-1", "cd-command", "high", "cd /var")
	a.Record("session-1", "cd-command", "high", "cd /home")
	a.Record("session-1", "file-operations", "high", "cat foo.txt")
	a.Record("session-1", "file-operations", "high", "cat bar.txt")
	a.Record("session-1", "git-add-all", "critical", "git add .")

	top := a.GetTopPatterns("session-1", 2)
	require.Len(t, top, 2)
	assert.Equal(t, "cd-command", top[0].PatternID)
	assert.Equal(t, 3, top[0].Count)
	assert.Equal(t, "file-operations", top[1].PatternID)
	assert.Equal(t, 2, top[1].Count)
}

func TestGetTopPatterns_UnknownSession(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	top := a.GetTopPatterns("nonexistent", 5)
	assert.Nil(t, top)
}

func TestGetTopPatterns_RequestMoreThanAvailable(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	a.Record("session-1", "cd-command", "high", "cd /tmp")

	top := a.GetTopPatterns("session-1", 10)
	require.Len(t, top, 1)
	assert.Equal(t, "cd-command", top[0].PatternID)
}

func TestShouldEscalate(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	a.Record("session-1", "cd-command", "high", "cd /tmp")
	a.Record("session-1", "file-operations", "high", "cat foo.txt")

	assert.False(t, a.ShouldEscalate("session-1", 3))
	assert.True(t, a.ShouldEscalate("session-1", 2))
	assert.True(t, a.ShouldEscalate("session-1", 1))
}

func TestShouldEscalate_UnknownSession(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)
	assert.False(t, a.ShouldEscalate("nonexistent", 1))
}

func TestAccumulatorCleanup(t *testing.T) {
	a := NewPatternAccumulator(10 * time.Millisecond)

	a.Record("session-old", "cd-command", "high", "cd /tmp")
	time.Sleep(25 * time.Millisecond)
	a.Record("session-new", "file-operations", "high", "cat foo.txt")

	a.Cleanup()

	assert.Equal(t, 0, a.GetSessionTotal("session-old"), "old session should be cleaned up")
	assert.Equal(t, 1, a.GetSessionTotal("session-new"), "new session should be retained")
}

func TestAccumulatorCleanup_RemovesEmptySessions(t *testing.T) {
	a := NewPatternAccumulator(10 * time.Millisecond)

	a.Record("session-1", "cd-command", "high", "cd /tmp")
	time.Sleep(25 * time.Millisecond)

	a.Cleanup()

	a.mu.Lock()
	_, exists := a.sessions["session-1"]
	a.mu.Unlock()
	assert.False(t, exists, "empty session should be removed from map")
}

func TestAccumulatorConcurrentAccess(t *testing.T) {
	a := NewPatternAccumulator(30 * time.Minute)

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
			a.Record(session, "cd-command", "high", "cd /tmp")
			a.GetFrequency(session, "cd-command")
			a.GetSessionTotal(session)
			a.GetTopPatterns(session, 3)
			a.ShouldEscalate(session, 5)
			a.Cleanup()
		}(i)
	}

	wg.Wait()

	// If we get here without a race condition panic, the test passes.
	total := a.GetSessionTotal("session-1") + a.GetSessionTotal("session-2")
	assert.GreaterOrEqual(t, total, 0, "accumulator should still work after concurrent access")
}
