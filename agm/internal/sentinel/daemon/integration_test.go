package daemon

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/sentinel/tmux"
	"github.com/vbonnet/dear-agent/pkg/enforcement"
)

// TestDetectorWithEnforcement_Integration tests detector integration with enforcement patterns.
func TestDetectorWithEnforcement_Integration(t *testing.T) {
	// Load enforcement patterns
	patterns, err := enforcement.LoadPatterns("../../pkg/enforcement/testdata/patterns.yaml")
	if err != nil {
		// If test patterns don't exist, skip this test
		t.Skip("enforcement test patterns not available")
	}

	enforcementDetector, err := enforcement.NewDetector(patterns)
	require.NoError(t, err)

	// Create stuck session detector
	stuckDetector := NewStuckSessionDetector()

	// Test case: Session with permission prompt (enforcement pattern)
	t.Run("enforcement pattern detection", func(t *testing.T) {
		pane := &tmux.PaneInfo{
			SessionName: "test-enforcement",
			Content:     "Allow Claude to execute this command? (y/n)",
			CapturedAt:  time.Now(),
		}

		// Both detectors should identify this
		stuck, reason := stuckDetector.IsSessionStuck(pane)
		assert.True(t, stuck)
		assert.Equal(t, "stuck_permission_prompt", reason)

		// Enforcement detector can also check for violations in content
		violation := enforcementDetector.Detect(pane.Content)
		if violation != nil {
			// If a permission-related violation pattern exists, it should match
			t.Logf("Enforcement violation detected: %s", violation.ID)
		}
	})
}

// TestMonitoringWorkflow_Simulation simulates a full monitoring workflow.
func TestMonitoringWorkflow_Simulation(t *testing.T) {
	detector := NewStuckSessionDetector()
	sessionName := "workflow-test"

	// Simulate monitoring cycle 1: Normal operation
	t.Run("cycle 1 - normal", func(t *testing.T) {
		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "Working on task... some output here",
			CursorX:     10,
			CursorY:     20,
			CapturedAt:  time.Now(),
		}

		detector.TrackSession(sessionName, pane.CursorX, pane.CursorY)

		info := detector.DetectStuckSession(pane)
		assert.Nil(t, info, "should not be stuck in normal operation")
	})

	// Simulate monitoring cycle 2: Cursor moved, still working
	t.Run("cycle 2 - cursor moved", func(t *testing.T) {
		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "More output, processing...",
			CursorX:     15,
			CursorY:     25,
			CapturedAt:  time.Now(),
		}

		detector.TrackSession(sessionName, pane.CursorX, pane.CursorY)

		info := detector.DetectStuckSession(pane)
		assert.Nil(t, info)
	})

	// Simulate monitoring cycle 3: Session stuck
	t.Run("cycle 3 - stuck", func(t *testing.T) {
		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "✶ Thinking...",
			CursorX:     15,
			CursorY:     25,
			CapturedAt:  time.Now(),
		}

		info := detector.DetectStuckSession(pane)
		assert.NotNil(t, info, "should detect stuck session")
		assert.Equal(t, "stuck_zero_token_waiting", info.Reason)
	})

	// Simulate monitoring cycle 4: Recovery successful
	t.Run("cycle 4 - recovered", func(t *testing.T) {
		pane := &tmux.PaneInfo{
			SessionName: sessionName,
			Content:     "✅ Task completed successfully\nReady for next command ❯",
			CursorX:     0,
			CursorY:     30,
			CapturedAt:  time.Now(),
		}

		detector.TrackSession(sessionName, pane.CursorX, pane.CursorY)

		info := detector.DetectStuckSession(pane)
		assert.Nil(t, info, "should not be stuck after recovery")
	})
}

// TestMultiSessionMonitoring_Simulation simulates monitoring multiple sessions.
func TestMultiSessionMonitoring_Simulation(t *testing.T) {
	detector := NewStuckSessionDetector()

	sessions := []struct {
		name    string
		content string
		stuck   bool
		reason  string
	}{
		{
			name:    "session-healthy-1",
			content: "✅ Task completed ❯",
			stuck:   false,
		},
		{
			name:    "session-stuck-1",
			content: "✶ Thinking...",
			stuck:   true,
			reason:  "stuck_zero_token_waiting",
		},
		{
			name:    "session-healthy-2",
			content: "Working on task with output",
			stuck:   false,
		},
		{
			name:    "session-stuck-2",
			content: "✻ Mustering...",
			stuck:   true,
			reason:  "stuck_mustering",
		},
		{
			name:    "session-stuck-3",
			content: "Allow this operation? (y/n)",
			stuck:   false, // permission prompt requires stale content history to confirm stuck
			reason:  "",
		},
	}

	for _, s := range sessions {
		t.Run(s.name, func(t *testing.T) {
			pane := &tmux.PaneInfo{
				SessionName: s.name,
				Content:     s.content,
				CursorX:     10,
				CursorY:     20,
				CapturedAt:  time.Now(),
			}

			detector.TrackSession(s.name, pane.CursorX, pane.CursorY)

			info := detector.DetectStuckSession(pane)

			if s.stuck {
				assert.NotNil(t, info, "session should be detected as stuck")
				if info != nil {
					assert.Equal(t, s.reason, info.Reason)
				}
			} else {
				assert.Nil(t, info, "session should not be detected as stuck")
			}
		})
	}
}

// TestLongRunningTask_FalsePositivePrevention tests prevention of false positives.
func TestLongRunningTask_FalsePositivePrevention(t *testing.T) {
	detector := NewStuckSessionDetector()
	detector.CursorFrozenTimeout = 1 // 1 minute for testing

	sessionName := "long-task"

	// Simulate long-running task with frozen cursor (within 1-minute window)
	baseTime := time.Now().Add(-55 * time.Second) // Start 55 seconds ago (within 60s window)
	detector.sessionHistories[sessionName] = &SessionHistory{
		cursorPositions: []CursorSnapshot{
			{X: 10, Y: 20, Timestamp: baseTime},                       // 55s ago
			{X: 10, Y: 20, Timestamp: baseTime.Add(50 * time.Second)}, // 5s ago (all within 60s window)
		},
		maxHistory: 10,
	}

	tests := []struct {
		name        string
		content     string
		expectStuck bool
		description string
	}{
		{
			name:        "completed task",
			content:     "Running long build...\n✅ Build completed successfully",
			expectStuck: false,
			description: "should not be stuck if task completed",
		},
		{
			name:        "idle prompt visible",
			content:     "Build finished\n❯",
			expectStuck: false,
			description: "should not be stuck if idle prompt shown",
		},
		{
			name:        "actually stuck",
			content:     "Running build...\n[no output for 30 minutes]",
			expectStuck: true,
			description: "should be stuck if truly frozen",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &tmux.PaneInfo{
				SessionName: sessionName,
				Content:     tt.content,
				CursorX:     10,
				CursorY:     20,
				CapturedAt:  time.Now(),
			}

			stuck, _ := detector.IsSessionStuck(pane)
			assert.Equal(t, tt.expectStuck, stuck, tt.description)
		})
	}
}

// TestRealWorldScenarios tests realistic stuck session scenarios.
func TestRealWorldScenarios(t *testing.T) {
	detector := NewStuckSessionDetector()

	scenarios := []struct {
		name         string
		description  string
		paneContent  string
		expectStuck  bool
		expectReason string
	}{
		{
			name:        "0-token API stall",
			description: "Claude stuck in thinking with no token activity",
			paneContent: `
$ agm start task-123
▸ Session task-123 started
✶ Thinking...
[stuck here for 15+ minutes]
`,
			expectStuck:  true,
			expectReason: "stuck_zero_token_waiting",
		},
		{
			name:        "mustering hang",
			description: "Session stuck during initialization",
			paneContent: `
$ agm start new-task
▸ Session new-task started
✻ Mustering...
[stuck initializing]
`,
			expectStuck:  true,
			expectReason: "stuck_mustering",
		},
		{
			name:        "permission prompt ignored",
			description: "User didn't respond to permission prompt (needs content staleness to confirm)",
			paneContent: `
Claude wants to execute: git push origin main
Allow this operation? (y/n)
[waiting for user input]
`,
			expectStuck:  false, // token-aware: requires stale content history
			expectReason: "",
		},
		{
			name:        "legitimate long task",
			description: "npm test running for a long time (not stuck)",
			paneContent: `
$ npm test
Running test suite...
Test 1: PASS
Test 2: PASS
...
[tests running normally]
`,
			expectStuck:  false,
			expectReason: "",
		},
		{
			name:        "task completed successfully",
			description: "Session finished work and waiting for next command",
			paneContent: `
Bash command:
git commit -m "Fix bug"
[main abc123] Fix bug
 1 file changed, 5 insertions(+)
✅ Task completed successfully
Ready for next command ❯
`,
			expectStuck:  false,
			expectReason: "",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			pane := &tmux.PaneInfo{
				SessionName: "scenario-test",
				Content:     scenario.paneContent,
				CapturedAt:  time.Now(),
			}

			stuck, reason := detector.IsSessionStuck(pane)
			assert.Equal(t, scenario.expectStuck, stuck,
				"scenario: %s", scenario.description)
			assert.Equal(t, scenario.expectReason, reason,
				"reason mismatch for scenario: %s", scenario.description)
		})
	}
}

// TestHistoricalTracking tests session history over time.
func TestHistoricalTracking(t *testing.T) {
	detector := NewStuckSessionDetector()
	detector.CursorFrozenTimeout = 2 // 2 minutes

	sessionName := "historical-test"

	// Build up history over time (checking for 3-minute freeze)
	// Cursor at 11,20 needs to be frozen for 3+ minutes (180+ seconds)
	baseTime := time.Now().Add(-200 * time.Second) // Start 200 seconds (3m20s) ago

	snapshots := []struct {
		x    int
		y    int
		time time.Time
	}{
		{10, 20, baseTime},                        // 200s ago (different position)
		{11, 20, baseTime.Add(10 * time.Second)},  // 190s ago - cursor moved to 11,20
		{11, 20, baseTime.Add(50 * time.Second)},  // 150s ago - still at 11,20
		{11, 20, baseTime.Add(100 * time.Second)}, // 100s ago - still at 11,20
		{11, 20, baseTime.Add(190 * time.Second)}, // 10s ago - still at 11,20 (frozen 190s = 3m10s)
	}

	history := NewSessionHistory(10)
	for _, snap := range snapshots {
		history.AddSnapshot(snap.x, snap.y, snap.time)
	}

	detector.sessionHistories[sessionName] = history

	// Should detect freeze in last 3 minutes
	assert.True(t, history.IsCursorFrozen(3*time.Minute),
		"should detect cursor frozen for 3 minutes")

	// But not for 5 minutes (cursor moved 2 minutes ago)
	assert.False(t, history.IsCursorFrozen(5*time.Minute),
		"should not detect freeze for 5 minute window (cursor moved)")
}

// Benchmark comprehensive detection workflow
func BenchmarkFullDetectionWorkflow(b *testing.B) {
	detector := NewStuckSessionDetector()
	sessionName := "bench-workflow"

	// Setup history
	for i := 0; i < 5; i++ {
		detector.TrackSession(sessionName, 10, 20)
	}

	pane := &tmux.PaneInfo{
		SessionName: sessionName,
		Content:     "✶ Thinking... lots of output here to simulate realistic content",
		CursorX:     10,
		CursorY:     20,
		CapturedAt:  time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		info := detector.DetectStuckSession(pane)
		_ = info
	}
}
