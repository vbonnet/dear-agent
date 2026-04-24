package daemon

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessHookErrors_RecordsInAccumulator(t *testing.T) {
	m := &SessionMonitor{
		accumulator: NewPatternAccumulator(30 * time.Minute),
	}

	content := `Some output
Hook PreToolUse:Bash denied this tool
More output
Hook PreToolUse:Bash denied this tool
`
	m.processHookErrors("session-1", content)

	total := m.accumulator.GetSessionTotal("session-1")
	assert.Equal(t, 2, total)
}

func TestProcessHookErrors_NilAccumulator(t *testing.T) {
	m := &SessionMonitor{}
	// Should not panic
	m.processHookErrors("session-1", "Hook PreToolUse:Bash denied this tool")
}

func TestProcessHookErrors_NoHookErrors(t *testing.T) {
	m := &SessionMonitor{
		accumulator: NewPatternAccumulator(30 * time.Minute),
	}

	m.processHookErrors("session-1", "regular output with no hook errors")
	assert.Equal(t, 0, m.accumulator.GetSessionTotal("session-1"))
}

func TestProcessHookErrors_UsesHookNameAsFallbackPatternID(t *testing.T) {
	m := &SessionMonitor{
		accumulator: NewPatternAccumulator(30 * time.Minute),
	}

	content := `Hook PreToolUse:Bash blocking error from command: "~/.claude/hooks/pretool-bash-blocker": some unknown reason`
	m.processHookErrors("session-1", content)

	// Should use "hook:pretool-bash-blocker" as pattern ID since no detector matched
	freq := m.accumulator.GetFrequency("session-1", "hook:pretool-bash-blocker")
	assert.Equal(t, 1, freq)
}

func TestProcessHookErrors_UsesToolAsFallbackWhenNoHookName(t *testing.T) {
	m := &SessionMonitor{
		accumulator: NewPatternAccumulator(30 * time.Minute),
	}

	content := "Hook PreToolUse:Write denied this tool"
	m.processHookErrors("session-1", content)

	freq := m.accumulator.GetFrequency("session-1", "hook:Write")
	assert.Equal(t, 1, freq)
}

func TestCrossSessionThreshold_FiresAt3Sessions(t *testing.T) {
	tmpDir := t.TempDir()

	sw, err := NewStreamErrorWriter(tmpDir)
	require.NoError(t, err)

	acc := NewPatternAccumulator(30 * time.Minute)
	m := &SessionMonitor{
		accumulator:           acc,
		streamWriter:          sw,
		CrossSessionThreshold: 3,
		logger:                noopLogger(),
	}

	hookLine := "Hook PreToolUse:Write denied this tool"

	// 2 sessions — should not fire
	m.processHookErrors("session-1", hookLine)
	m.processHookErrors("session-2", hookLine)
	assertPendingCount(t, tmpDir, 0)

	// 3rd session — threshold reached
	m.processHookErrors("session-3", hookLine)
	assertPendingCount(t, tmpDir, 1)

	// 4th session — should not re-emit (dedup in writer)
	m.processHookErrors("session-4", hookLine)
	assertPendingCount(t, tmpDir, 1)
}

func TestCrossSessionThreshold_DifferentPatterns(t *testing.T) {
	tmpDir := t.TempDir()

	sw, err := NewStreamErrorWriter(tmpDir)
	require.NoError(t, err)

	acc := NewPatternAccumulator(30 * time.Minute)
	m := &SessionMonitor{
		accumulator:           acc,
		streamWriter:          sw,
		CrossSessionThreshold: 3,
		logger:                noopLogger(),
	}

	// 3 sessions hit pattern A
	m.processHookErrors("s1", "Hook PreToolUse:Bash denied this tool")
	m.processHookErrors("s2", "Hook PreToolUse:Bash denied this tool")
	m.processHookErrors("s3", "Hook PreToolUse:Bash denied this tool")

	// 2 sessions hit pattern B — not enough
	m.processHookErrors("s1", "Hook PreToolUse:Write denied this tool")
	m.processHookErrors("s2", "Hook PreToolUse:Write denied this tool")

	items := readPendingItems(t, tmpDir)
	require.Len(t, items, 1)
	assert.Equal(t, "hook:Bash", items[0].PatternID)
}

func TestStreamErrorItem_Format(t *testing.T) {
	tmpDir := t.TempDir()

	sw, err := NewStreamErrorWriter(tmpDir)
	require.NoError(t, err)

	acc := NewPatternAccumulator(30 * time.Minute)
	m := &SessionMonitor{
		accumulator:           acc,
		streamWriter:          sw,
		CrossSessionThreshold: 2,
		logger:                noopLogger(),
	}

	m.processHookErrors("alpha", "Hook PreToolUse:Bash denied this tool")
	m.processHookErrors("beta", "Hook PreToolUse:Bash denied this tool")

	items := readPendingItems(t, tmpDir)
	require.Len(t, items, 1)

	item := items[0]
	assert.Equal(t, "hook:Bash", item.PatternID)
	assert.Equal(t, 2, item.SessionCount)
	assert.Contains(t, item.Sessions, "alpha")
	assert.Contains(t, item.Sessions, "beta")
	assert.Equal(t, "astrocyte-daemon", item.Source)
	assert.NotEmpty(t, item.Timestamp)
	assert.NotEmpty(t, item.FirstSeen)
	assert.NotEmpty(t, item.LastSeen)
}

func TestGetCrossSessionCount(t *testing.T) {
	acc := NewPatternAccumulator(30 * time.Minute)
	acc.Record("s1", "pattern-a", "high", "cmd1")
	acc.Record("s2", "pattern-a", "high", "cmd2")
	acc.Record("s3", "pattern-b", "high", "cmd3")
	acc.Record("s1", "pattern-a", "high", "cmd4") // duplicate session

	assert.Equal(t, 2, acc.GetCrossSessionCount("pattern-a"))
	assert.Equal(t, 1, acc.GetCrossSessionCount("pattern-b"))
	assert.Equal(t, 0, acc.GetCrossSessionCount("nonexistent"))
}

func TestGetCrossSessionPatterns(t *testing.T) {
	acc := NewPatternAccumulator(30 * time.Minute)
	acc.Record("s1", "pattern-a", "high", "cmd1")
	acc.Record("s2", "pattern-a", "high", "cmd2")
	acc.Record("s3", "pattern-a", "high", "cmd3")
	acc.Record("s1", "pattern-b", "high", "cmd4")
	acc.Record("s2", "pattern-b", "high", "cmd5")

	patterns := acc.GetCrossSessionPatterns(3)
	assert.Equal(t, []string{"pattern-a"}, patterns)

	patterns = acc.GetCrossSessionPatterns(2)
	assert.Len(t, patterns, 2)
	assert.Contains(t, patterns, "pattern-a")
	assert.Contains(t, patterns, "pattern-b")
}

func TestGetCrossSessionCount_RespectsWindow(t *testing.T) {
	acc := NewPatternAccumulator(10 * time.Millisecond)
	acc.Record("s1", "pattern-a", "high", "cmd1")
	acc.Record("s2", "pattern-a", "high", "cmd2")

	time.Sleep(15 * time.Millisecond)

	assert.Equal(t, 0, acc.GetCrossSessionCount("pattern-a"))
}

func TestStreamErrorWriter_Deduplication(t *testing.T) {
	tmpDir := t.TempDir()
	sw, err := NewStreamErrorWriter(tmpDir)
	require.NoError(t, err)

	item := &StreamErrorItem{
		PatternID:    "test-pattern",
		SessionCount: 3,
		Sessions:     []string{"s1", "s2", "s3"},
		Timestamp:    time.Now().Format(time.RFC3339),
		Source:       "test",
	}

	require.NoError(t, sw.WriteItem(item))
	require.NoError(t, sw.WriteItem(item)) // duplicate

	items := readPendingItems(t, tmpDir)
	assert.Len(t, items, 1, "duplicate should be suppressed")
}

func TestBuildStreamErrorItem_CollectsAllSessions(t *testing.T) {
	acc := NewPatternAccumulator(30 * time.Minute)
	acc.Record("s1", "pat-x", "high", "cmd1")
	acc.Record("s2", "pat-x", "high", "cmd2")
	acc.Record("s3", "pat-x", "medium", "cmd3")

	item := BuildStreamErrorItem(acc, "pat-x")

	assert.Equal(t, "pat-x", item.PatternID)
	assert.Equal(t, 3, item.SessionCount)
	assert.ElementsMatch(t, []string{"s1", "s2", "s3"}, item.Sessions)
	assert.Equal(t, "astrocyte-daemon", item.Source)
}

func TestBuildStreamErrorItem_TracksFirstAndLastSeen(t *testing.T) {
	acc := NewPatternAccumulator(30 * time.Minute)

	// Manually inject violations with distinct timestamps to avoid timing flakiness
	acc.mu.Lock()
	sv1 := &SessionViolations{
		Violations: []AccumulatedViolation{{
			PatternID: "pat-y", Severity: "high", Command: "cmd1",
			Timestamp: time.Now().Add(-10 * time.Second),
		}},
	}
	sv2 := &SessionViolations{
		Violations: []AccumulatedViolation{{
			PatternID: "pat-y", Severity: "high", Command: "cmd2",
			Timestamp: time.Now(),
		}},
	}
	acc.sessions["s1"] = sv1
	acc.sessions["s2"] = sv2
	acc.mu.Unlock()

	item := BuildStreamErrorItem(acc, "pat-y")

	assert.NotEmpty(t, item.FirstSeen)
	assert.NotEmpty(t, item.LastSeen)
	assert.NotEqual(t, item.FirstSeen, item.LastSeen, "first and last seen should differ")
}

func TestBuildStreamErrorItem_IgnoresOtherPatterns(t *testing.T) {
	acc := NewPatternAccumulator(30 * time.Minute)
	acc.Record("s1", "pat-a", "high", "cmd1")
	acc.Record("s2", "pat-b", "high", "cmd2")
	acc.Record("s3", "pat-a", "high", "cmd3")

	item := BuildStreamErrorItem(acc, "pat-a")

	assert.Equal(t, 2, item.SessionCount)
	assert.ElementsMatch(t, []string{"s1", "s3"}, item.Sessions)
}

func TestCheckCrossSessionThresholds_NilWriter(t *testing.T) {
	m := &SessionMonitor{
		accumulator:           NewPatternAccumulator(30 * time.Minute),
		streamWriter:          nil, // no writer
		CrossSessionThreshold: 3,
		logger:                noopLogger(),
	}

	// Record 3 sessions — should not panic with nil writer
	m.accumulator.Record("s1", "pat", "high", "cmd")
	m.accumulator.Record("s2", "pat", "high", "cmd")
	m.accumulator.Record("s3", "pat", "high", "cmd")
	m.checkCrossSessionThresholds() // should be a no-op
}

func TestCheckCrossSessionThresholds_DefaultThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	sw, err := NewStreamErrorWriter(tmpDir)
	require.NoError(t, err)

	m := &SessionMonitor{
		accumulator:           NewPatternAccumulator(30 * time.Minute),
		streamWriter:          sw,
		CrossSessionThreshold: 0, // should default to 3
		logger:                noopLogger(),
	}

	m.accumulator.Record("s1", "pat", "high", "cmd")
	m.accumulator.Record("s2", "pat", "high", "cmd")
	m.checkCrossSessionThresholds()
	assertPendingCount(t, tmpDir, 0) // only 2 sessions, default threshold is 3

	m.accumulator.Record("s3", "pat", "high", "cmd")
	m.checkCrossSessionThresholds()
	assertPendingCount(t, tmpDir, 1) // 3 sessions now
}

func TestCheckCrossSessionThresholds_MultiplePatternsCrossed(t *testing.T) {
	tmpDir := t.TempDir()
	sw, err := NewStreamErrorWriter(tmpDir)
	require.NoError(t, err)

	m := &SessionMonitor{
		accumulator:           NewPatternAccumulator(30 * time.Minute),
		streamWriter:          sw,
		CrossSessionThreshold: 2,
		logger:                noopLogger(),
	}

	// Both patterns cross threshold of 2
	m.accumulator.Record("s1", "pat-a", "high", "cmd")
	m.accumulator.Record("s2", "pat-a", "high", "cmd")
	m.accumulator.Record("s1", "pat-b", "high", "cmd")
	m.accumulator.Record("s2", "pat-b", "high", "cmd")

	m.checkCrossSessionThresholds()

	items := readPendingItems(t, tmpDir)
	assert.Len(t, items, 2)
	patternIDs := []string{items[0].PatternID, items[1].PatternID}
	assert.ElementsMatch(t, []string{"pat-a", "pat-b"}, patternIDs)
}

// --- helpers ---

func assertPendingCount(t *testing.T, dir string, expected int) {
	t.Helper()
	items := readPendingItems(t, dir)
	assert.Len(t, items, expected)
}

func readPendingItems(t *testing.T, dir string) []StreamErrorItem {
	t.Helper()
	path := filepath.Join(dir, "pending.jsonl")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)

	var items []StreamErrorItem
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var item StreamErrorItem
		require.NoError(t, json.Unmarshal([]byte(line), &item))
		items = append(items, item)
	}
	return items
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
