package daemon

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vbonnet/dear-agent/agm/internal/sentinel/tmux"
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
	for i := range 10 {
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
		name         string
		snapshots    []CursorSnapshot
		duration     time.Duration
		expectFreeze bool
	}{
		{
			name: "cursor frozen",
			snapshots: []CursorSnapshot{
				{X: 10, Y: 20, Timestamp: time.Now().Add(-5 * time.Minute)},
				{X: 10, Y: 20, Timestamp: time.Now().Add(-3 * time.Minute)},
				{X: 10, Y: 20, Timestamp: time.Now().Add(-1 * time.Minute)},
			},
			duration:     10 * time.Minute,
			expectFreeze: true,
		},
		{
			name: "cursor moved",
			snapshots: []CursorSnapshot{
				{X: 10, Y: 20, Timestamp: time.Now().Add(-5 * time.Minute)},
				{X: 11, Y: 20, Timestamp: time.Now().Add(-3 * time.Minute)},
				{X: 12, Y: 20, Timestamp: time.Now().Add(-1 * time.Minute)},
			},
			duration:     10 * time.Minute,
			expectFreeze: false,
		},
		{
			name: "insufficient snapshots",
			snapshots: []CursorSnapshot{
				{X: 10, Y: 20, Timestamp: time.Now().Add(-1 * time.Minute)},
			},
			duration:     10 * time.Minute,
			expectFreeze: false,
		},
		{
			name: "snapshots outside duration window",
			snapshots: []CursorSnapshot{
				{X: 10, Y: 20, Timestamp: time.Now().Add(-20 * time.Minute)},
				{X: 10, Y: 20, Timestamp: time.Now().Add(-15 * time.Minute)},
			},
			duration:     10 * time.Minute,
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
	assert.Equal(t, 2, detector.PermissionPromptDuration)
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
		name           string
		paneContent    string
		expectStuck    bool
		expectedReason string
	}{
		{
			name:           "stuck mustering",
			paneContent:    "✻ Mustering...",
			expectStuck:    true,
			expectedReason: "stuck_mustering",
		},
		{
			name:           "stuck zero token waiting",
			paneContent:    "✶ Thinking...",
			expectStuck:    true,
			expectedReason: "stuck_zero_token_waiting",
		},
		// NOTE: permission prompt detection is tested separately in
		// TestIsSessionStuck_PermissionPromptTokenAware because it
		// requires content staleness history to be set up.
		{
			name:           "not stuck - completed",
			paneContent:    "✅ Task completed ❯",
			expectStuck:    false,
			expectedReason: "",
		},
		{
			name:           "not stuck - idle",
			paneContent:    "Ready ❯",
			expectStuck:    false,
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

// TestIsSessionStuck_AskUserQuestion tests that AskUserQuestion exempts from stuck.
func TestIsSessionStuck_AskUserQuestion(t *testing.T) {
	tests := []struct {
		name           string
		paneContent    string
		expectStuck    bool
		expectedReason string
	}{
		{
			name:           "ask user question with spinner - not stuck",
			paneContent:    "✶ Thinking...\n\nWhat do you want?\n  1. Option A\n  2. Option B\n\nEnter to select",
			expectStuck:    false,
			expectedReason: "",
		},
		{
			name:           "ask user question plan approval - not stuck",
			paneContent:    "Do you approve this plan?\n  1. Yes\n  2. No",
			expectStuck:    false,
			expectedReason: "",
		},
		{
			name:           "ask user question with mustering - not stuck",
			paneContent:    "✻ Mustering...\n\nSelect an option:\n  1. First\n  2. Second",
			expectStuck:    false,
			expectedReason: "",
		},
		{
			name:           "no ask user question with spinner - stuck",
			paneContent:    "✶ Thinking...",
			expectStuck:    true,
			expectedReason: "stuck_zero_token_waiting",
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

// TestDetectStuckSession_AskUserQuestion tests DetectStuckSession returns nil for AskUserQuestion.
func TestDetectStuckSession_AskUserQuestion(t *testing.T) {
	detector := NewStuckSessionDetector()
	pane := &tmux.PaneInfo{
		SessionName: "ask-user-session",
		Content:     "✶ Thinking...\n\nChoose an option:\n  1. Fix\n  2. Skip\n\nEnter to select",
		CursorX:     10,
		CursorY:     20,
		CapturedAt:  time.Now(),
	}

	info := detector.DetectStuckSession(pane)
	assert.Nil(t, info, "should not detect stuck when AskUserQuestion is present")
}

// TestIsSessionStuck_CursorFrozenWithAskUser tests cursor freeze exemption with AskUserQuestion.
func TestIsSessionStuck_CursorFrozenWithAskUser(t *testing.T) {
	detector := NewStuckSessionDetector()
	detector.CursorFrozenTimeout = 1

	sessionName := "frozen-but-asking"

	baseTime := time.Now().Add(-50 * time.Second)
	detector.sessionHistories[sessionName] = &SessionHistory{
		cursorPositions: []CursorSnapshot{
			{X: 10, Y: 20, Timestamp: baseTime},
			{X: 10, Y: 20, Timestamp: baseTime.Add(25 * time.Second)},
			{X: 10, Y: 20, Timestamp: baseTime.Add(45 * time.Second)},
		},
		maxHistory: 10,
	}

	pane := &tmux.PaneInfo{
		SessionName: sessionName,
		Content:     "Pick an item:\n  1. Alpha\n  2. Beta\n\nEnter to confirm",
		CursorX:     10,
		CursorY:     20,
		CapturedAt:  time.Now(),
	}

	stuck, _ := detector.IsSessionStuck(pane)
	assert.False(t, stuck, "should not detect stuck when AskUserQuestion present even with frozen cursor")
}

// TestIsSessionStuck_CursorFrozen tests cursor freeze detection.
func TestIsSessionStuck_CursorFrozen(t *testing.T) {
	detector := NewStuckSessionDetector()
	detector.CursorFrozenTimeout = 1 // 1 minute for testing

	sessionName := "frozen-session"

	// Track cursor at same position over time (use timestamps within 1-minute window)
	baseTime := time.Now().Add(-50 * time.Second) // Start 50 seconds ago (within 60s window)
	detector.sessionHistories[sessionName] = &SessionHistory{
		cursorPositions: []CursorSnapshot{
			{X: 10, Y: 20, Timestamp: baseTime},                       // 50s ago
			{X: 10, Y: 20, Timestamp: baseTime.Add(25 * time.Second)}, // 25s ago
			{X: 10, Y: 20, Timestamp: baseTime.Add(45 * time.Second)}, // 5s ago
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

	// Cursor frozen but task completed (use timestamps within 1-minute window)
	baseTime := time.Now().Add(-55 * time.Second) // Start 55 seconds ago (within 60s window)
	detector.sessionHistories[sessionName] = &SessionHistory{
		cursorPositions: []CursorSnapshot{
			{X: 10, Y: 20, Timestamp: baseTime},                       // 55s ago
			{X: 10, Y: 20, Timestamp: baseTime.Add(50 * time.Second)}, // 5s ago
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
		var longContent strings.Builder
		for i := range 1000 {
			longContent.WriteString("Line " + string(rune(i)) + "\n")
		}
		longContent.WriteString("✶ Thinking...")

		pane := &tmux.PaneInfo{
			SessionName: "long",
			Content:     longContent.String(),
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

// TestIsSessionStuck_PermissionPromptTokenAware tests token-consumption-based
// permission prompt detection. A permission prompt is only flagged as stuck when
// content has stopped changing (no tokens being produced).
func TestIsSessionStuck_PermissionPromptTokenAware(t *testing.T) {
	t.Run("permission prompt with stale content is stuck", func(t *testing.T) {
		detector := NewStuckSessionDetector()
		detector.PermissionPromptDuration = 2 // 2 minutes
		sessionName := "perm-stale"

		// Simulate 3 checks over ~2 minutes with same content length (stale)
		contentLen := 42
		history := NewSessionHistory(10)
		history.AddContentSnapshot(contentLen, time.Now().Add(-150*time.Second))
		history.AddContentSnapshot(contentLen, time.Now().Add(-90*time.Second))
		history.AddContentSnapshot(contentLen, time.Now().Add(-30*time.Second))
		detector.sessionHistories[sessionName] = history

		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "Allow this action? (y/n)",
			CapturedAt:  time.Now(),
		}

		stuck, reason := detector.IsSessionStuck(pane)
		assert.True(t, stuck, "permission prompt with stale content should be stuck")
		assert.Equal(t, "stuck_permission_prompt", reason)
	})

	t.Run("permission prompt with changing content is NOT stuck", func(t *testing.T) {
		detector := NewStuckSessionDetector()
		detector.PermissionPromptDuration = 2
		sessionName := "perm-active"

		// Content length is changing — tokens are being produced
		history := NewSessionHistory(10)
		history.AddContentSnapshot(100, time.Now().Add(-150*time.Second))
		history.AddContentSnapshot(200, time.Now().Add(-90*time.Second))
		history.AddContentSnapshot(300, time.Now().Add(-30*time.Second))
		detector.sessionHistories[sessionName] = history

		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "Allow this action? (y/n)",
			CapturedAt:  time.Now(),
		}

		stuck, _ := detector.IsSessionStuck(pane)
		assert.False(t, stuck, "permission prompt with actively changing content should NOT be stuck")
	})

	t.Run("permission prompt with no history is NOT stuck", func(t *testing.T) {
		detector := NewStuckSessionDetector()
		detector.PermissionPromptDuration = 2

		pane := &tmux.PaneInfo{
			SessionName: "perm-no-history",
			Content:     "Allow this action? (y/n)",
			CapturedAt:  time.Now(),
		}

		stuck, _ := detector.IsSessionStuck(pane)
		assert.False(t, stuck, "permission prompt with no content history should NOT be stuck (need evidence)")
	})

	t.Run("permission prompt detected within 2 minutes", func(t *testing.T) {
		detector := NewStuckSessionDetector()
		detector.PermissionPromptDuration = 2
		sessionName := "perm-fast"

		// Content stale for exactly 2 minutes — should trigger
		contentLen := 50
		history := NewSessionHistory(10)
		history.AddContentSnapshot(contentLen, time.Now().Add(-2*time.Minute))
		history.AddContentSnapshot(contentLen, time.Now().Add(-1*time.Minute))
		history.AddContentSnapshot(contentLen, time.Now())
		detector.sessionHistories[sessionName] = history

		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "Allow this action? (y/n)",
			CapturedAt:  time.Now(),
		}

		stuck, reason := detector.IsSessionStuck(pane)
		assert.True(t, stuck, "should detect stuck within 2-minute window")
		assert.Equal(t, "stuck_permission_prompt", reason)
	})
}

// TestIsContentStale tests the content staleness detection.
func TestIsContentStale(t *testing.T) {
	t.Run("stale content", func(t *testing.T) {
		history := NewSessionHistory(10)
		history.AddContentSnapshot(100, time.Now().Add(-4*time.Minute))
		history.AddContentSnapshot(100, time.Now().Add(-2*time.Minute))
		history.AddContentSnapshot(100, time.Now().Add(-1*time.Minute))

		assert.True(t, history.IsContentStale(5*time.Minute))
	})

	t.Run("changing content", func(t *testing.T) {
		history := NewSessionHistory(10)
		history.AddContentSnapshot(100, time.Now().Add(-5*time.Minute))
		history.AddContentSnapshot(150, time.Now().Add(-3*time.Minute))
		history.AddContentSnapshot(200, time.Now().Add(-1*time.Minute))

		assert.False(t, history.IsContentStale(5*time.Minute))
	})

	t.Run("insufficient snapshots", func(t *testing.T) {
		history := NewSessionHistory(10)
		history.AddContentSnapshot(100, time.Now().Add(-1*time.Minute))

		assert.False(t, history.IsContentStale(5*time.Minute))
	})

	t.Run("snapshots outside window", func(t *testing.T) {
		history := NewSessionHistory(10)
		history.AddContentSnapshot(100, time.Now().Add(-20*time.Minute))
		history.AddContentSnapshot(100, time.Now().Add(-15*time.Minute))

		assert.False(t, history.IsContentStale(5*time.Minute))
	})
}

// TestNewStuckSessionDetector_PermissionPromptDefault verifies 2-minute default.
func TestNewStuckSessionDetector_PermissionPromptDefault(t *testing.T) {
	detector := NewStuckSessionDetector()
	assert.Equal(t, 2, detector.PermissionPromptDuration,
		"PermissionPromptDuration should default to 2 minutes for fast detection")
	assert.Equal(t, 3, detector.PermissionEscalationDuration,
		"PermissionEscalationDuration should default to 3 minutes")
	assert.NotNil(t, detector.permissionFirstSeen,
		"permissionFirstSeen map should be initialized")
}

// TestIsSessionStuck_PermissionPromptEscalation tests that permission prompts
// that persist past the escalation threshold produce the escalation reason.
func TestIsSessionStuck_PermissionPromptEscalation(t *testing.T) {
	t.Run("permission prompt within escalation window returns normal reason", func(t *testing.T) {
		detector := NewStuckSessionDetector()
		detector.PermissionPromptDuration = 2
		detector.PermissionEscalationDuration = 3
		sessionName := "perm-normal"

		// Simulate stale content
		contentLen := 50
		history := NewSessionHistory(10)
		history.AddContentSnapshot(contentLen, time.Now().Add(-150*time.Second))
		history.AddContentSnapshot(contentLen, time.Now().Add(-90*time.Second))
		history.AddContentSnapshot(contentLen, time.Now().Add(-30*time.Second))
		detector.sessionHistories[sessionName] = history

		// Pre-set first-seen to 2 minutes ago (within 3-minute escalation window)
		detector.permissionFirstSeen[sessionName] = time.Now().Add(-2 * time.Minute)

		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "  Allow Bash\n  ls -la\n  (y)es | (n)o",
			CapturedAt:  time.Now(),
		}

		stuck, reason := detector.IsSessionStuck(pane)
		assert.True(t, stuck)
		assert.Equal(t, "stuck_permission_prompt", reason)
	})

	t.Run("permission prompt past escalation window returns escalate reason", func(t *testing.T) {
		detector := NewStuckSessionDetector()
		detector.PermissionPromptDuration = 2
		detector.PermissionEscalationDuration = 3
		sessionName := "perm-escalate"

		// Simulate stale content — snapshots must be within the 2-minute window
		// and span at least 1 minute (duration/2) with same content length
		contentLen := 50
		history := NewSessionHistory(10)
		history.AddContentSnapshot(contentLen, time.Now().Add(-110*time.Second))
		history.AddContentSnapshot(contentLen, time.Now().Add(-60*time.Second))
		history.AddContentSnapshot(contentLen, time.Now().Add(-10*time.Second))
		detector.sessionHistories[sessionName] = history

		// Pre-set first-seen to 4 minutes ago (past 3-minute escalation window)
		detector.permissionFirstSeen[sessionName] = time.Now().Add(-4 * time.Minute)

		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "  Allow Bash\n  ls -la\n  (y)es | (n)o",
			CapturedAt:  time.Now(),
		}

		stuck, reason := detector.IsSessionStuck(pane)
		assert.True(t, stuck)
		assert.Equal(t, "stuck_permission_prompt_escalate", reason)
	})

	t.Run("permission prompt cleared resets first-seen tracking", func(t *testing.T) {
		detector := NewStuckSessionDetector()
		sessionName := "perm-cleared"

		// Set first-seen
		detector.permissionFirstSeen[sessionName] = time.Now().Add(-5 * time.Minute)

		// Pane no longer has permission prompt
		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "Normal output, no permission prompt",
			CapturedAt:  time.Now(),
		}

		detector.IsSessionStuck(pane)

		// Verify tracking was cleared
		_, exists := detector.permissionFirstSeen[sessionName]
		assert.False(t, exists, "permissionFirstSeen should be cleared when prompt disappears")
	})

	t.Run("first detection sets first-seen time", func(t *testing.T) {
		detector := NewStuckSessionDetector()
		sessionName := "perm-new"

		// No prior tracking
		_, exists := detector.permissionFirstSeen[sessionName]
		assert.False(t, exists)

		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "  Allow Read\n  file_path=\"test.go\"\n  (y)es | (n)o",
			CapturedAt:  time.Now(),
		}

		detector.IsSessionStuck(pane)

		// Now first-seen should be set
		firstSeen, exists := detector.permissionFirstSeen[sessionName]
		assert.True(t, exists, "first-seen should be set on first detection")
		assert.WithinDuration(t, time.Now(), firstSeen, 2*time.Second)
	})
}

// TestStrategyForSymptom_PermissionEscalation tests the escalation symptom maps to manual.
func TestStrategyForSymptom_PermissionEscalation(t *testing.T) {
	strategy := StrategyForSymptom("stuck_permission_prompt_escalate")
	assert.Equal(t, RecoveryManual, strategy,
		"escalated permission prompts should use manual strategy (needs human/orchestrator)")
}
