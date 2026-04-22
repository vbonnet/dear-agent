package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectFriction_BasicPatterns(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantID    string
		wantCount int
	}{
		{"same as always", "same error as always happening here", "friction:same_as_always", 1},
		{"this keeps happening", "this keeps happening in production", "friction:keeps_happening", 1},
		{"recurring issue", "this is a recurring issue with auth", "friction:recurring_issue", 1},
		{"every session hits this", "every session hits this bug", "friction:every_session", 1},
		{"happens every time", "it happens every time we deploy", "friction:every_time", 1},
		{"same error again", "same error again with the parser", "friction:same_error_again", 1},
		{"workaround for", "applying workaround for the timeout", "friction:workaround", 1},
		{"workaround again", "using workaround again", "friction:workaround", 1},
		{"known issue", "this is a known issue", "friction:known_issue", 1},
		{"keeps failing", "the test keeps failing", "friction:keeps_failing", 1},
		{"keeps breaking", "this keeps breaking after updates", "friction:keeps_failing", 1},
		{"hitting this bug again", "hitting this bug again", "friction:hit_again", 1},
		{"always fails here", "it always fails here in CI", "friction:always_fails", 1},
		{"not again", "oh not again", "friction:not_again", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := DetectFriction(tt.input)
			require.Len(t, signals, tt.wantCount)
			if tt.wantCount > 0 {
				assert.Equal(t, tt.wantID, signals[0].PatternID)
				assert.NotEmpty(t, signals[0].Description)
				assert.NotEmpty(t, signals[0].Raw)
			}
		})
	}
}

func TestDetectFriction_CaseInsensitive(t *testing.T) {
	signals := DetectFriction("This Keeps Happening")
	require.Len(t, signals, 1)
	assert.Equal(t, "friction:keeps_happening", signals[0].PatternID)
}

func TestDetectFriction_NoMatch(t *testing.T) {
	signals := DetectFriction("everything is working fine, no issues at all")
	assert.Empty(t, signals)
}

func TestDetectFriction_MultipleLines(t *testing.T) {
	content := `line one
this keeps happening
some normal output
same error again with parsing
more output`

	signals := DetectFriction(content)
	require.Len(t, signals, 2)
	ids := []string{signals[0].PatternID, signals[1].PatternID}
	assert.Contains(t, ids, "friction:keeps_happening")
	assert.Contains(t, ids, "friction:same_error_again")
}

func TestDetectFriction_DeduplicatesWithinScan(t *testing.T) {
	content := `this keeps happening
more output
this keeps happening again on another line`

	signals := DetectFriction(content)
	// Same pattern ID should only appear once per scan
	require.Len(t, signals, 1)
	assert.Equal(t, "friction:keeps_happening", signals[0].PatternID)
}

func TestDetectFriction_EmptyContent(t *testing.T) {
	assert.Empty(t, DetectFriction(""))
	assert.Empty(t, DetectFriction("\n\n\n"))
}

// --- StreamFrictionWriter tests ---

func TestStreamFrictionWriter_WriteItem(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewStreamFrictionWriter(tmpDir)
	require.NoError(t, err)

	item := &StreamFrictionItem{
		PatternID:              "friction:keeps_happening",
		Description:            "Recurring issue: this keeps happening",
		OccurrenceCount:        5,
		SessionCount:           3,
		Sessions:               []string{"s1", "s2", "s3"},
		FirstSeen:              time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
		LastSeen:               time.Now().Format(time.RFC3339),
		Timestamp:              time.Now().Format(time.RFC3339),
		Source:                 "astrocyte-friction-detector",
		SuggestedInvestigation: "Search incident logs",
	}

	require.NoError(t, w.WriteItem(item))

	items := readFrictionItems(t, tmpDir)
	require.Len(t, items, 1)
	assert.Equal(t, "friction:keeps_happening", items[0].PatternID)
	assert.Equal(t, 5, items[0].OccurrenceCount)
	assert.Equal(t, 3, items[0].SessionCount)
	assert.Equal(t, "astrocyte-friction-detector", items[0].Source)
	assert.NotEmpty(t, items[0].SuggestedInvestigation)
}

func TestStreamFrictionWriter_Deduplication(t *testing.T) {
	tmpDir := t.TempDir()
	w, err := NewStreamFrictionWriter(tmpDir)
	require.NoError(t, err)

	item := &StreamFrictionItem{
		PatternID: "friction:test",
		Source:    "test",
	}

	require.NoError(t, w.WriteItem(item))
	require.NoError(t, w.WriteItem(item)) // duplicate

	items := readFrictionItems(t, tmpDir)
	assert.Len(t, items, 1, "duplicate should be suppressed")
}

func TestBuildStreamFrictionItem(t *testing.T) {
	acc := NewPatternAccumulator(30 * time.Minute)
	acc.Record("s1", "friction:keeps_happening", "medium", "this keeps happening")
	acc.Record("s2", "friction:keeps_happening", "medium", "this keeps happening")
	acc.Record("s1", "friction:keeps_happening", "medium", "this keeps happening again")

	item := BuildStreamFrictionItem(acc, "friction:keeps_happening", "test description")

	assert.Equal(t, "friction:keeps_happening", item.PatternID)
	assert.Equal(t, "test description", item.Description)
	assert.Equal(t, 3, item.OccurrenceCount) // total across all sessions
	assert.Equal(t, 2, item.SessionCount)
	assert.Contains(t, item.Sessions, "s1")
	assert.Contains(t, item.Sessions, "s2")
	assert.Equal(t, "astrocyte-friction-detector", item.Source)
	assert.NotEmpty(t, item.SuggestedInvestigation)
}

// --- Integration: processFrictionSignals + cross-session threshold ---

func TestProcessFrictionSignals_RecordsInAccumulator(t *testing.T) {
	m := &SessionMonitor{
		frictionAcc: NewPatternAccumulator(30 * time.Minute),
		logger:      noopLogger(),
	}

	m.processFrictionSignals("session-1", "this keeps happening\nand same error again")
	total := m.frictionAcc.GetSessionTotal("session-1")
	assert.Equal(t, 2, total)
}

func TestProcessFrictionSignals_NilAccumulator(t *testing.T) {
	m := &SessionMonitor{logger: noopLogger()}
	// Should not panic
	m.processFrictionSignals("session-1", "this keeps happening")
}

func TestProcessFrictionSignals_NoSignals(t *testing.T) {
	m := &SessionMonitor{
		frictionAcc: NewPatternAccumulator(30 * time.Minute),
		logger:      noopLogger(),
	}

	m.processFrictionSignals("session-1", "normal output no friction")
	assert.Equal(t, 0, m.frictionAcc.GetSessionTotal("session-1"))
}

func TestFrictionThreshold_FiresAt3Sessions(t *testing.T) {
	tmpDir := t.TempDir()
	fw, err := NewStreamFrictionWriter(tmpDir)
	require.NoError(t, err)

	m := &SessionMonitor{
		frictionAcc:           NewPatternAccumulator(30 * time.Minute),
		frictionWriter:        fw,
		CrossSessionThreshold: 3,
		logger:                noopLogger(),
	}

	content := "this keeps happening"

	// 2 sessions — should not fire
	m.processFrictionSignals("s1", content)
	m.processFrictionSignals("s2", content)
	assertFrictionPendingCount(t, tmpDir, 0)

	// 3rd session — threshold reached
	m.processFrictionSignals("s3", content)
	assertFrictionPendingCount(t, tmpDir, 1)

	// 4th session — should not re-emit (dedup)
	m.processFrictionSignals("s4", content)
	assertFrictionPendingCount(t, tmpDir, 1)
}

func TestFrictionThreshold_DifferentPatterns(t *testing.T) {
	tmpDir := t.TempDir()
	fw, err := NewStreamFrictionWriter(tmpDir)
	require.NoError(t, err)

	m := &SessionMonitor{
		frictionAcc:           NewPatternAccumulator(30 * time.Minute),
		frictionWriter:        fw,
		CrossSessionThreshold: 3,
		logger:                noopLogger(),
	}

	// 3 sessions hit "keeps happening"
	m.processFrictionSignals("s1", "this keeps happening")
	m.processFrictionSignals("s2", "this keeps happening")
	m.processFrictionSignals("s3", "this keeps happening")

	// 2 sessions hit "recurring issue" — not enough
	m.processFrictionSignals("s1", "recurring issue here")
	m.processFrictionSignals("s2", "recurring issue here")

	items := readFrictionItems(t, tmpDir)
	require.Len(t, items, 1)
	assert.Equal(t, "friction:keeps_happening", items[0].PatternID)
}

func TestFrictionItem_Format(t *testing.T) {
	tmpDir := t.TempDir()
	fw, err := NewStreamFrictionWriter(tmpDir)
	require.NoError(t, err)

	m := &SessionMonitor{
		frictionAcc:           NewPatternAccumulator(30 * time.Minute),
		frictionWriter:        fw,
		CrossSessionThreshold: 2,
		logger:                noopLogger(),
	}

	m.processFrictionSignals("alpha", "this keeps happening")
	m.processFrictionSignals("beta", "this keeps happening")

	items := readFrictionItems(t, tmpDir)
	require.Len(t, items, 1)

	item := items[0]
	assert.Equal(t, "friction:keeps_happening", item.PatternID)
	assert.Equal(t, 2, item.SessionCount)
	assert.Contains(t, item.Sessions, "alpha")
	assert.Contains(t, item.Sessions, "beta")
	assert.Equal(t, "astrocyte-friction-detector", item.Source)
	assert.NotEmpty(t, item.Timestamp)
	assert.NotEmpty(t, item.FirstSeen)
	assert.NotEmpty(t, item.LastSeen)
	assert.NotEmpty(t, item.SuggestedInvestigation)
	assert.NotEmpty(t, item.Description)
}

// --- helpers ---

func assertFrictionPendingCount(t *testing.T, dir string, expected int) {
	t.Helper()
	items := readFrictionItems(t, dir)
	assert.Len(t, items, expected)
}

func readFrictionItems(t *testing.T, dir string) []StreamFrictionItem {
	t.Helper()
	path := filepath.Join(dir, "pending.jsonl")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)

	var items []StreamFrictionItem
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var item StreamFrictionItem
		require.NoError(t, json.Unmarshal([]byte(line), &item))
		items = append(items, item)
	}
	return items
}
