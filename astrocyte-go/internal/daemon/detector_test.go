package daemon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vbonnet/ai-tools/astrocyte/internal/tmux"
)

// TestNewSessionHistory tests session history creation.
func TestNewSessionHistory(t *testing.T) {
	history := NewSessionHistory(10)

	assert.NotNil(t, history)
	assert.Equal(t, 10, history.maxHistory)
	assert.Equal(t, 0, len(history.cursorPositions))
}

// TestAddSnapshot tests adding cursor snapshots.
func TestAddSnapshot(t *testing.T) {
	history := NewSessionHistory(5)

	// Add snapshots
	now := time.Now()
	history.AddSnapshot(10, 20, now)
	history.AddSnapshot(11, 21, now.Add(1*time.Second))
	history.AddSnapshot(12, 22, now.Add(2*time.Second))

	assert.Equal(t, 3, len(history.cursorPositions))
	assert.Equal(t, 10, history.cursorPositions[0].X)
	assert.Equal(t, 20, history.cursorPositions[0].Y)
}

// TestAddSnapshot_MaxHistory tests history limit enforcement.
func TestAddSnapshot_MaxHistory(t *testing.T) {
	history := NewSessionHistory(3)

	now := time.Now()
	for i := 0; i < 10; i++ {
		history.AddSnapshot(i, i*10, now.Add(time.Duration(i)*time.Second))
	}

	// Should only keep last 3
	assert.Equal(t, 3, len(history.cursorPositions))
	assert.Equal(t, 7, history.cursorPositions[0].X)
	assert.Equal(t, 9, history.cursorPositions[2].X)
}

// TestIsCursorFrozen tests cursor freeze detection.
func TestIsCursorFrozen(t *testing.T) {
	tests := []struct {
		name       string
		snapshots  []CursorSnapshot
		duration   time.Duration
		expectFreeze bool
	}{
		{
			name: "cursor frozen",
			snapshots: []CursorSnapshot{
				{X: 10, Y: 20, Timestamp: time.Now().Add(-5 * time.Minute)},
				{X: 10, Y: 20, Timestamp: time.Now().Add(-3 * time.Minute)},
				{X: 10, Y: 20, Timestamp: time.Now().Add(-1 * time.Minute)},
			},
			duration:   10 * time.Minute,
			expectFreeze: true,
		},
		{
			name: "cursor moved",
			snapshots: []CursorSnapshot{
				{X: 10, Y: 20, Timestamp: time.Now().Add(-5 * time.Minute)},
				{X: 11, Y: 20, Timestamp: time.Now().Add(-3 * time.Minute)},
				{X: 12, Y: 20, Timestamp: time.Now().Add(-1 * time.Minute)},
			},
			duration:   10 * time.Minute,
			expectFreeze: false,
		},
		{
			name: "insufficient snapshots",
			snapshots: []CursorSnapshot{
				{X: 10, Y: 20, Timestamp: time.Now().Add(-1 * time.Minute)},
			},
			duration:   10 * time.Minute,
			expectFreeze: false,
		},
		{
			name: "snapshots outside duration window",
			snapshots: []CursorSnapshot{
				{X: 10, Y: 20, Timestamp: time.Now().Add(-20 * time.Minute)},
				{X: 10, Y: 20, Timestamp: time.Now().Add(-15 * time.Minute)},
			},
			duration:   10 * time.Minute,
			expectFreeze: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := &SessionHistory{
				cursorPositions: tt.snapshots,
				maxHistory:      10,
			}

			result := history.IsCursorFrozen(tt.duration)
			assert.Equal(t, tt.expectFreeze, result)
		})
	}
}

// TestNewStuckSessionDetector tests detector creation.
func TestNewStuckSessionDetector(t *testing.T) {
	detector := NewStuckSessionDetector()

	assert.NotNil(t, detector)
	assert.NotNil(t, detector.sessionHistories)
	assert.Equal(t, 20, detector.MusteringTimeout)
	assert.Equal(t, 15, detector.ZeroTokenWaitingTimeout)
	assert.Equal(t, 30, detector.CursorFrozenTimeout)
	assert.Equal(t, 10, detector.PermissionPromptDuration)
}

// TestTrackSession tests session tracking.
func TestTrackSession(t *testing.T) {
	detector := NewStuckSessionDetector()

	detector.TrackSession("test-session", 10, 20)
	detector.TrackSession("test-session", 11, 21)

	history, exists := detector.sessionHistories["test-session"]
	assert.True(t, exists)
	assert.Equal(t, 2, len(history.cursorPositions))
}

// TestIsSessionStuck tests stuck session detection.
func TestIsSessionStuck(t *testing.T) {
	tests := []struct {
		name          string
		paneContent   string
		expectStuck   bool
		expectedReason string
	}{
		{
			name:          "stuck mustering",
			paneContent:   "✻ Mustering...",
			expectStuck:   true,
			expectedReason: "stuck_mustering",
		},
		{
			name:          "stuck zero token waiting",
			paneContent:   "✶ Thinking...",
			expectStuck:   true,
			expectedReason: "stuck_zero_token_waiting",
		},
		{
			name:          "stuck permission prompt",
			paneContent:   "Allow this action? (y/n)",
			expectStuck:   true,
			expectedReason: "stuck_permission_prompt",
		},
		{
			name:          "not stuck - completed",
			paneContent:   "✅ Task completed ❯",
			expectStuck:   false,
			expectedReason: "",
		},
		{
			name:          "not stuck - idle",
			paneContent:   "Ready ❯",
			expectStuck:   false,
			expectedReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detector := NewStuckSessionDetector()
			pane := &tmux.PaneInfo{
				SessionName: "test-session",
				Content:     tt.paneContent,
				CapturedAt:  time.Now(),
			}

			stuck, reason := detector.IsSessionStuck(pane)
			assert.Equal(t, tt.expectStuck, stuck)
			assert.Equal(t, tt.expectedReason, reason)
		})
	}
}

// TestIsSessionStuck_CursorFrozen tests cursor freeze detection.
func TestIsSessionStuck_CursorFrozen(t *testing.T) {
	detector := NewStuckSessionDetector()
	detector.CursorFrozenTimeout = 1 // 1 minute for testing

	sessionName := "frozen-session"

	// Track cursor at same position over time
	baseTime := time.Now().Add(-2 * time.Minute)
	detector.sessionHistories[sessionName] = &SessionHistory{
		cursorPositions: []CursorSnapshot{
			{X: 10, Y: 20, Timestamp: baseTime},
			{X: 10, Y: 20, Timestamp: baseTime.Add(30 * time.Second)},
			{X: 10, Y: 20, Timestamp: baseTime.Add(60 * time.Second)},
		},
		maxHistory: 10,
	}

	pane := &tmux.PaneInfo{
		SessionName: sessionName,
		Content:     "Some content without completion",
		CursorX:     10,
		CursorY:     20,
		CapturedAt:  time.Now(),
	}

	stuck, reason := detector.IsSessionStuck(pane)
	assert.True(t, stuck, "should detect frozen cursor")
	assert.Equal(t, "cursor_frozen", reason)
}

// TestIsSessionStuck_CursorFrozenButCompleted tests false positive prevention.
func TestIsSessionStuck_CursorFrozenButCompleted(t *testing.T) {
	detector := NewStuckSessionDetector()
	detector.CursorFrozenTimeout = 1 // 1 minute

	sessionName := "completed-session"

	// Cursor frozen but task completed
	baseTime := time.Now().Add(-2 * time.Minute)
	detector.sessionHistories[sessionName] = &SessionHistory{
		cursorPositions: []CursorSnapshot{
			{X: 10, Y: 20, Timestamp: baseTime},
			{X: 10, Y: 20, Timestamp: baseTime.Add(60 * time.Second)},
		},
		maxHistory: 10,
	}

	pane := &tmux.PaneInfo{
		SessionName: sessionName,
		Content:     "✅ Task completed successfully",
		CursorX:     10,
		CursorY:     20,
		CapturedAt:  time.Now(),
	}

	stuck, _ := detector.IsSessionStuck(pane)
	assert.False(t, stuck, "should not detect stuck when task completed")
}

// TestGetStuckReason tests stuck reason retrieval.
func TestGetStuckReason(t *testing.T) {
	detector := NewStuckSessionDetector()

	pane := &tmux.PaneInfo{
		SessionName: "test-session",
		Content:     "✶ Thinking...",
		CapturedAt:  time.Now(),
	}

	reason := detector.GetStuckReason(pane)
	assert.Equal(t, "stuck_zero_token_waiting", reason)
}

// TestDetectStuckSession tests comprehensive detection.
func TestDetectStuckSession(t *testing.T) {
	detector := NewStuckSessionDetector()

	t.Run("stuck session", func(t *testing.T) {
		pane := &tmux.PaneInfo{
			SessionName: "stuck-session",
			Content:     "✻ Mustering...",
			CursorX:     10,
			CursorY:     20,
			CapturedAt:  time.Now(),
		}

		info := detector.DetectStuckSession(pane)
		assert.NotNil(t, info)
		assert.Equal(t, "stuck-session", info.SessionName)
		assert.Equal(t, "stuck_mustering", info.Reason)
		assert.NotNil(t, info.Indicators)
		assert.Equal(t, 10, info.CursorX)
		assert.Equal(t, 20, info.CursorY)
	})

	t.Run("not stuck session", func(t *testing.T) {
		pane := &tmux.PaneInfo{
			SessionName: "healthy-session",
			Content:     "✅ Complete ❯",
			CapturedAt:  time.Now(),
		}

		info := detector.DetectStuckSession(pane)
		assert.Nil(t, info)
	})
}

// TestSessionStuckInfo_String tests string representation.
func TestSessionStuckInfo_String(t *testing.T) {
	info := &SessionStuckInfo{
		SessionName: "test-session",
		Reason:      "stuck_mustering",
		CursorX:     10,
		CursorY:     20,
		LastCommand: "git status",
		DetectedAt:  time.Now(),
	}

	str := info.String()
	assert.Contains(t, str, "test-session")
	assert.Contains(t, str, "stuck_mustering")
	assert.Contains(t, str, "10,20")
	assert.Contains(t, str, "git status")
}

// TestDetector_MultipleSessions tests tracking multiple sessions.
func TestDetector_MultipleSessions(t *testing.T) {
	detector := NewStuckSessionDetector()

	// Track multiple sessions
	detector.TrackSession("session-1", 10, 20)
	detector.TrackSession("session-2", 30, 40)
	detector.TrackSession("session-1", 11, 21)

	assert.Equal(t, 2, len(detector.sessionHistories))
	assert.Equal(t, 2, len(detector.sessionHistories["session-1"].cursorPositions))
	assert.Equal(t, 1, len(detector.sessionHistories["session-2"].cursorPositions))
}

// TestDetector_CustomThresholds tests custom threshold configuration.
func TestDetector_CustomThresholds(t *testing.T) {
	detector := NewStuckSessionDetector()

	// Customize thresholds
	detector.MusteringTimeout = 10
	detector.ZeroTokenWaitingTimeout = 5
	detector.CursorFrozenTimeout = 15
	detector.PermissionPromptDuration = 3

	assert.Equal(t, 10, detector.MusteringTimeout)
	assert.Equal(t, 5, detector.ZeroTokenWaitingTimeout)
	assert.Equal(t, 15, detector.CursorFrozenTimeout)
	assert.Equal(t, 3, detector.PermissionPromptDuration)
}

// Benchmark tests

func BenchmarkIsSessionStuck(b *testing.B) {
	detector := NewStuckSessionDetector()
	pane := &tmux.PaneInfo{
		SessionName: "bench-session",
		Content:     "✶ Thinking...",
		CapturedAt:  time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = detector.IsSessionStuck(pane)
	}
}

func BenchmarkDetectStuckSession(b *testing.B) {
	detector := NewStuckSessionDetector()
	pane := &tmux.PaneInfo{
		SessionName: "bench-session",
		Content:     "✻ Mustering... lots of content here to make it realistic",
		CursorX:     10,
		CursorY:     20,
		CapturedAt:  time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = detector.DetectStuckSession(pane)
	}
}

// TestDetector_Integration tests full detection workflow.
func TestDetector_Integration(t *testing.T) {
	detector := NewStuckSessionDetector()
	sessionName := "integration-test"

	// Simulate monitoring over time
	baseTime := time.Now().Add(-10 * time.Minute)

	// Snapshot 1: Normal operation
	detector.TrackSession(sessionName, 10, 20)

	// Snapshot 2: Cursor moved
	detector.TrackSession(sessionName, 15, 20)

	// Snapshot 3: Session stuck
	pane := &tmux.PaneInfo{
		SessionName: sessionName,
		Content:     "✶ Thinking...",
		CursorX:     15,
		CursorY:     20,
		CapturedAt:  baseTime.Add(5 * time.Minute),
	}

	stuck, reason := detector.IsSessionStuck(pane)
	assert.True(t, stuck)
	assert.Equal(t, "stuck_zero_token_waiting", reason)

	info := detector.DetectStuckSession(pane)
	assert.NotNil(t, info)
	assert.Equal(t, sessionName, info.SessionName)
}

// TestDetector_EdgeCases tests edge cases.
func TestDetector_EdgeCases(t *testing.T) {
	detector := NewStuckSessionDetector()

	t.Run("empty content", func(t *testing.T) {
		pane := &tmux.PaneInfo{
			SessionName: "empty",
			Content:     "",
			CapturedAt:  time.Now(),
		}

		stuck, _ := detector.IsSessionStuck(pane)
		assert.False(t, stuck)
	})

	t.Run("very long content", func(t *testing.T) {
		// Generate long content
		longContent := ""
		for i := 0; i < 1000; i++ {
			longContent += "Line " + string(rune(i)) + "\n"
		}
		longContent += "✶ Thinking..."

		pane := &tmux.PaneInfo{
			SessionName: "long",
			Content:     longContent,
			CapturedAt:  time.Now(),
		}

		stuck, reason := detector.IsSessionStuck(pane)
		assert.True(t, stuck)
		assert.Equal(t, "stuck_zero_token_waiting", reason)
	})

	t.Run("special characters", func(t *testing.T) {
		pane := &tmux.PaneInfo{
			SessionName: "special",
			Content:     "✶ 处理中... ✶ Обработка... ✶ معالجة...",
			CapturedAt:  time.Now(),
		}

		stuck, _ := detector.IsSessionStuck(pane)
		assert.True(t, stuck)
	})
}
