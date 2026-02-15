package tmux

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractLastCommand tests command extraction from pane content.
func TestExtractLastCommand(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "bash command header",
			content: `Some output
Bash command:
ls -la /tmp`,
			expected: "ls -la /tmp",
		},
		{
			name: "running command header",
			content: `Previous output
Running command:
git status`,
			expected: "git status",
		},
		{
			name: "executing header",
			content: `Output here
Executing:
npm test`,
			expected: "npm test",
		},
		{
			name:     "no command",
			content:  `Just some regular output without commands`,
			expected: "",
		},
		{
			name:     "empty content",
			content:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{
				Content: tt.content,
			}

			result := pane.ExtractLastCommand()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDetectPermissionPrompt tests permission prompt detection.
func TestDetectPermissionPrompt(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "y/n prompt",
			content:  "Do you want to proceed? (y/n)",
			expected: true,
		},
		{
			name:     "bracket style",
			content:  "Allow this operation? [y/n]",
			expected: true,
		},
		{
			name:     "allow to pattern",
			content:  "Allow Claude to execute this command?",
			expected: true,
		},
		{
			name:     "permission to pattern",
			content:  "Permission to read file?",
			expected: true,
		},
		{
			name:     "proceed pattern",
			content:  "Ready to proceed?",
			expected: true,
		},
		{
			name:     "continue pattern",
			content:  "Continue with this action?",
			expected: true,
		},
		{
			name:     "no permission prompt",
			content:  "Just normal output without prompts",
			expected: false,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{
				Content: tt.content,
			}

			result := pane.DetectPermissionPrompt()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDetectStuckIndicators tests comprehensive stuck detection.
func TestDetectStuckIndicators(t *testing.T) {
	tests := []struct {
		name               string
		content            string
		expectedIndicators map[string]bool
	}{
		{
			name:    "mustering pattern",
			content: "✻ Mustering...",
			expectedIndicators: map[string]bool{
				"mustering":        true,
				"waiting":          false,
				"permission_prompt": false,
				"completed":        false,
				"idle_prompt":      false,
				"zero_token_waiting": false,
			},
		},
		{
			name:    "thinking pattern",
			content: "✶ Thinking...",
			expectedIndicators: map[string]bool{
				"mustering":        false,
				"waiting":          true,
				"permission_prompt": false,
				"completed":        false,
				"idle_prompt":      false,
				"zero_token_waiting": true,
			},
		},
		{
			name:    "completed with checkmark",
			content: "✅ Task completed successfully",
			expectedIndicators: map[string]bool{
				"mustering":        false,
				"waiting":          false,
				"permission_prompt": false,
				"completed":        true,
				"idle_prompt":      false,
				"zero_token_waiting": false,
			},
		},
		{
			name:    "idle prompt",
			content: "Ready for next command ❯",
			expectedIndicators: map[string]bool{
				"mustering":        false,
				"waiting":          false,
				"permission_prompt": false,
				"completed":        false,
				"idle_prompt":      true,
				"zero_token_waiting": false,
			},
		},
		{
			name:    "permission prompt",
			content: "Allow this action? (y/n)",
			expectedIndicators: map[string]bool{
				"mustering":        false,
				"waiting":          false,
				"permission_prompt": true,
				"completed":        false,
				"idle_prompt":      false,
				"zero_token_waiting": false,
			},
		},
		{
			name:    "waiting with idle prompt (not stuck)",
			content: "✶ Processing... ❯",
			expectedIndicators: map[string]bool{
				"mustering":        false,
				"waiting":          true,
				"permission_prompt": false,
				"completed":        false,
				"idle_prompt":      true,
				"zero_token_waiting": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{
				Content: tt.content,
			}

			indicators := pane.DetectStuckIndicators()

			for key, expected := range tt.expectedIndicators {
				assert.Equal(t, expected, indicators[key],
					"indicator %s mismatch", key)
			}
		})
	}
}

// TestIsStuck tests simple stuck detection logic.
func TestIsStuck(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "stuck mustering",
			content:  "✻ Mustering...",
			expected: true,
		},
		{
			name:     "stuck waiting",
			content:  "✶ Thinking...",
			expected: true,
		},
		{
			name:     "not stuck - completed",
			content:  "✅ Task completed",
			expected: false,
		},
		{
			name:     "not stuck - idle prompt",
			content:  "Ready ❯",
			expected: false,
		},
		{
			name:     "not stuck - waiting with idle",
			content:  "✶ Processing... ❯",
			expected: false,
		},
		{
			name:     "not stuck - normal output",
			content:  "Just regular output",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{
				Content: tt.content,
			}

			result := pane.IsStuck()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetStuckReason tests stuck reason detection.
func TestGetStuckReason(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "mustering",
			content:  "✻ Mustering...",
			expected: "stuck_mustering",
		},
		{
			name:     "zero token waiting",
			content:  "✶ Thinking...",
			expected: "stuck_zero_token_waiting",
		},
		{
			name:     "permission prompt",
			content:  "Allow this? (y/n)",
			expected: "stuck_permission_prompt",
		},
		{
			name:     "general waiting",
			content:  "✢ Processing...",
			expected: "stuck_waiting",
		},
		{
			name:     "not stuck",
			content:  "✅ Complete ❯",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{
				Content: tt.content,
			}

			result := pane.GetStuckReason()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetRecentContent tests recent content extraction.
func TestGetRecentContent(t *testing.T) {
	pane := &PaneInfo{
		Content: "0123456789ABCDEFGHIJ",
	}

	assert.Equal(t, "GHIJ", pane.getRecentContent(4))
	assert.Equal(t, "0123456789ABCDEFGHIJ", pane.getRecentContent(100))
	assert.Equal(t, "", (&PaneInfo{Content: ""}).getRecentContent(10))
}

// TestCapturePaneInfo_Integration tests pane info capture.
func TestCapturePaneInfo_Integration(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	sessionName := "astrocyte-test-capture"
	createTestSession(t, sessionName)
	defer cleanupTestSession(t, sessionName)

	client := NewClient()
	pane, err := CapturePaneInfo(client, sessionName)

	require.NoError(t, err)
	assert.Equal(t, sessionName, pane.SessionName)
	assert.NotEmpty(t, pane.Content)
	assert.GreaterOrEqual(t, pane.CursorX, 0)
	assert.GreaterOrEqual(t, pane.CursorY, 0)
	assert.False(t, pane.CapturedAt.IsZero())
}

// TestCapturePaneInfo_NonExistentSession tests error handling.
func TestCapturePaneInfo_NonExistentSession(t *testing.T) {
	if !isTmuxAvailable() {
		t.Skip("tmux not available, skipping integration test")
	}

	client := NewClient()
	_, err := CapturePaneInfo(client, "nonexistent-xyz-123")

	assert.Error(t, err)
}

// TestHasCompletionLanguage tests completion detection.
func TestHasCompletionLanguage(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "checkmark",
			content:  "✅",
			expected: true,
		},
		{
			name:     "check symbol",
			content:  "✓",
			expected: true,
		},
		{
			name:     "task completed",
			content:  "Task completed successfully",
			expected: true,
		},
		{
			name:     "ready to proceed",
			content:  "Ready to proceed with next step",
			expected: true,
		},
		{
			name:     "what would you like",
			content:  "What would you like me to do?",
			expected: true,
		},
		{
			name:     "no completion",
			content:  "Still working on task",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{
				Content: tt.content,
			}

			result := pane.hasCompletionLanguage()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestHasIdlePrompt tests idle prompt detection.
func TestHasIdlePrompt(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "has idle prompt",
			content:  "Ready ❯",
			expected: true,
		},
		{
			name:     "no idle prompt",
			content:  "Still processing",
			expected: false,
		},
		{
			name:     "idle prompt in middle (not at end)",
			content:  "Previous ❯ more content after",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{
				Content: tt.content,
			}

			result := pane.hasIdlePrompt()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Benchmark tests

func BenchmarkDetectStuckIndicators(b *testing.B) {
	pane := &PaneInfo{
		Content: "✶ Thinking... some more content here with various patterns",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pane.DetectStuckIndicators()
	}
}

func BenchmarkExtractLastCommand(b *testing.B) {
	pane := &PaneInfo{
		Content: `Previous output
Some more lines
Bash command:
git status --short`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pane.ExtractLastCommand()
	}
}

// TestPaneInfo_RealWorldPatterns tests with realistic content.
func TestPaneInfo_RealWorldPatterns(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectStuck    bool
		expectedReason string
	}{
		{
			name: "real mustering output",
			content: `
▸ Session astrocyte-test started
✻ Mustering...
Initializing session context
`,
			expectStuck:    true,
			expectedReason: "stuck_mustering",
		},
		{
			name: "real thinking output",
			content: `
$ agm start my-task
▸ Session my-task started
✶ Thinking...
`,
			expectStuck:    true,
			expectedReason: "stuck_zero_token_waiting",
		},
		{
			name: "completed task",
			content: `
Bash command:
git commit -m "Update docs"
[main abc123] Update docs
 1 file changed, 10 insertions(+)
✅ Task completed successfully
Ready to proceed ❯
`,
			expectStuck:    false,
			expectedReason: "",
		},
		{
			name: "normal work in progress",
			content: `
$ npm test
Running tests...
Test suite executing...
`,
			expectStuck:    false,
			expectedReason: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{
				Content:     tt.content,
				SessionName: "test-session",
				CapturedAt:  time.Now(),
			}

			stuck := pane.IsStuck()
			reason := pane.GetStuckReason()

			assert.Equal(t, tt.expectStuck, stuck,
				"stuck detection mismatch")
			assert.Equal(t, tt.expectedReason, reason,
				"reason mismatch")
		})
	}
}
