package tmux

import (
	"strings"
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

// TestDetectPermissionPrompt_ClaudeCodeUI tests detection of Claude Code's actual
// permission prompt UI patterns, which differ from generic (y/n) prompts.
func TestDetectPermissionPrompt_ClaudeCodeUI(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "Claude Code Allow Bash prompt",
			content: `  Allow Bash
  ls -la /tmp
  (y)es | (n)o | (Y)es, don't ask again for tool | (N)o, don't ask again for tool`,
			expected: true,
		},
		{
			name: "Claude Code Allow Read prompt",
			content: `  Allow Read
  file_path="/home/user/secret.txt"
  (y)es | (n)o`,
			expected: true,
		},
		{
			name: "Claude Code Allow Edit prompt",
			content: `  Allow Edit
  file_path="/tmp/test.go" old_string="foo" new_string="bar"
  (y)es | (n)o | (A)llow in this session`,
			expected: true,
		},
		{
			name: "Claude Code Allow Write prompt",
			content: `  Allow Write
  file_path="/tmp/new.go"
  (Y)es | (N)o`,
			expected: true,
		},
		{
			name: "Claude Code Allow Agent prompt",
			content: `  Allow Agent
  prompt="explore the codebase"
  (y)es | (n)o`,
			expected: true,
		},
		{
			name:     "don't ask again text",
			content:  "Press Y to allow. Don't ask again for this tool.",
			expected: true,
		},
		{
			name:     "Allow in this session option",
			content:  "(A)llow in this session",
			expected: true,
		},
		{
			name:     "not a permission prompt - Allow in prose",
			content:  "We should allow users to configure this setting",
			expected: false,
		},
		{
			name:     "not a permission prompt - normal code output",
			content:  "Running tests... 15 passed, 0 failed",
			expected: false,
		},
		{
			name: "Claude Code prompt with long parameter content",
			content: `Some previous output here that is long enough to push the prompt further down

  Allow Bash
  GOWORK=off go test -v -run TestPermission ./internal/sentinel/...
  (y)es | (n)o | (Y)es, don't ask again for tool`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{Content: tt.content}
			result := pane.DetectPermissionPrompt()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDetectPermissionPrompt_AllToolNames tests that all Claude Code tool names are detected.
func TestDetectPermissionPrompt_AllToolNames(t *testing.T) {
	tools := []string{"Bash", "Read", "Edit", "Write", "Glob", "Grep", "Agent", "Skill", "NotebookEdit", "WebFetch", "WebSearch"}

	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			content := "  Allow " + tool + "\n  some_param=\"value\"\n  (y)es | (n)o"
			pane := &PaneInfo{Content: content}
			assert.True(t, pane.DetectPermissionPrompt(),
				"should detect permission prompt for tool: %s", tool)
		})
	}
}

// TestDetectPermissionPrompt_LargerContentWindow tests that the 1000-char window
// catches permission prompts that would be missed with a 500-char window.
func TestDetectPermissionPrompt_LargerContentWindow(t *testing.T) {
	// Build content where the "Allow Bash" is more than 500 chars from the end
	// but within 1000 chars
	padding := strings.Repeat("x", 600)
	content := "  Allow Bash\n  ls -la\n  (y)es | (n)o\n" + padding

	pane := &PaneInfo{Content: content}
	// The permission prompt is at the start, but with 1000-char window from end,
	// it should still be detected since total content < 1000
	assert.True(t, pane.DetectPermissionPrompt())

	// Now with content > 1000 chars where prompt is too far from end
	largePadding := strings.Repeat("normal output line\n", 100)
	farContent := "  Allow Bash\n  ls -la\n  (y)es | (n)o\n" + largePadding
	pane2 := &PaneInfo{Content: farContent}
	// The prompt is now far from end — should NOT detect (content has moved past it)
	assert.False(t, pane2.DetectPermissionPrompt())
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
				"mustering":          true,
				"waiting":            false,
				"permission_prompt":  false,
				"completed":          false,
				"idle_prompt":        false,
				"zero_token_waiting": false,
			},
		},
		{
			name:    "thinking pattern",
			content: "✶ Thinking...",
			expectedIndicators: map[string]bool{
				"mustering":          false,
				"waiting":            true,
				"permission_prompt":  false,
				"completed":          false,
				"idle_prompt":        false,
				"zero_token_waiting": true,
			},
		},
		{
			name:    "completed with checkmark",
			content: "✅ Task completed successfully",
			expectedIndicators: map[string]bool{
				"mustering":          false,
				"waiting":            false,
				"permission_prompt":  false,
				"completed":          true,
				"idle_prompt":        false,
				"zero_token_waiting": false,
			},
		},
		{
			name:    "idle prompt",
			content: "Ready for next command ❯",
			expectedIndicators: map[string]bool{
				"mustering":          false,
				"waiting":            false,
				"permission_prompt":  false,
				"completed":          false,
				"idle_prompt":        true,
				"zero_token_waiting": false,
			},
		},
		{
			name:    "permission prompt",
			content: "Allow this action? (y/n)",
			expectedIndicators: map[string]bool{
				"mustering":          false,
				"waiting":            false,
				"permission_prompt":  true,
				"completed":          false,
				"idle_prompt":        false,
				"zero_token_waiting": false,
			},
		},
		{
			name:    "waiting with idle prompt (not stuck)",
			content: "✶ Processing... ❯",
			expectedIndicators: map[string]bool{
				"mustering":          false,
				"waiting":            true,
				"permission_prompt":  false,
				"completed":          false,
				"idle_prompt":        true,
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
			expected: "stuck_zero_token_waiting", // Waiting without idle prompt = zero token waiting
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

// TestPatternOverlap tests that mustering and waiting patterns don't overlap.
func TestPatternOverlap(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		expectMustering bool
		expectWaiting   bool
		expectOnlyOne   bool
		description     string
	}{
		{
			name:            "mustering should NOT trigger waiting",
			content:         "✻ Mustering...",
			expectMustering: true,
			expectWaiting:   false,
			expectOnlyOne:   true,
			description:     "Mustering pattern should only match mustering, not waiting",
		},
		{
			name:            "evaporating should NOT trigger waiting",
			content:         "✶ Evaporating...",
			expectMustering: true,
			expectWaiting:   false,
			expectOnlyOne:   true,
			description:     "Evaporating pattern should only match mustering, not waiting",
		},
		{
			name:            "thinking should only trigger waiting",
			content:         "✶ Thinking...",
			expectMustering: false,
			expectWaiting:   true,
			expectOnlyOne:   true,
			description:     "Thinking pattern should only match waiting, not mustering",
		},
		{
			name:            "processing should only trigger waiting",
			content:         "✢ Processing...",
			expectMustering: false,
			expectWaiting:   true,
			expectOnlyOne:   true,
			description:     "Processing pattern should only match waiting, not mustering",
		},
		{
			name:            "working should only trigger waiting",
			content:         "✻ Working...",
			expectMustering: false,
			expectWaiting:   true,
			expectOnlyOne:   true,
			description:     "Working pattern should only match waiting, not mustering",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{
				Content: tt.content,
			}

			indicators := pane.DetectStuckIndicators()

			assert.Equal(t, tt.expectMustering, indicators["mustering"],
				"%s: mustering detection failed", tt.description)
			assert.Equal(t, tt.expectWaiting, indicators["waiting"],
				"%s: waiting detection failed", tt.description)

			if tt.expectOnlyOne {
				// Verify only one is true (no overlap)
				count := 0
				if indicators["mustering"] {
					count++
				}
				if indicators["waiting"] {
					count++
				}
				assert.Equal(t, 1, count,
					"%s: should trigger exactly one pattern, not both", tt.description)
			}
		})
	}
}

// TestMusteringPatternSpecificity tests mustering patterns are specific.
func TestMusteringPatternSpecificity(t *testing.T) {
	musteringContent := []string{
		"✻ Mustering...",
		"✶ Evaporating...",
		"✢ Mustering...",
	}

	for _, content := range musteringContent {
		t.Run(content, func(t *testing.T) {
			pane := &PaneInfo{Content: content}
			indicators := pane.DetectStuckIndicators()

			assert.True(t, indicators["mustering"],
				"Should detect mustering for: %s", content)
			assert.False(t, indicators["waiting"],
				"Should NOT detect waiting for mustering pattern: %s", content)
		})
	}
}

// TestHasAskUserQuestion tests AskUserQuestion detection.
func TestHasAskUserQuestion(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name: "numbered option list",
			content: `What would you like to do?

  1. Fix the bug
  2. Add a feature
  3. Refactor code

Enter to select`,
			expected: true,
		},
		{
			name:     "enter to select prompt",
			content:  "Use arrow keys to navigate, Enter to select",
			expected: true,
		},
		{
			name:     "enter to confirm prompt",
			content:  "Press Enter to confirm your choice",
			expected: true,
		},
		{
			name:     "select an option",
			content:  "Please select an option from the list above",
			expected: true,
		},
		{
			name:     "choose a number",
			content:  "Choose a number to proceed",
			expected: true,
		},
		{
			name:     "plan approval",
			content:  "Do you approve this plan?",
			expected: true,
		},
		{
			name:     "arrow keys navigation",
			content:  "Use arrow keys to navigate the menu",
			expected: true,
		},
		{
			name:     "type a number",
			content:  "Type a number to select",
			expected: true,
		},
		{
			name:     "enter your choice",
			content:  "Enter your choice below",
			expected: true,
		},
		{
			name:     "normal output - not ask user",
			content:  "Running tests... all passed",
			expected: false,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
		{
			name:     "single numbered item should not match",
			content:  "1. First point only",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{Content: tt.content}
			result := pane.hasAskUserQuestion()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAskUserQuestionExemptsFromStuck tests that AskUserQuestion prevents stuck detection.
func TestAskUserQuestionExemptsFromStuck(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		expectStuck  bool
		expectReason string
	}{
		{
			name: "spinner with ask user question - not stuck",
			content: `✶ Thinking...

What would you like to do?
  1. Fix the bug
  2. Add a feature

Enter to select`,
			expectStuck:  false,
			expectReason: "",
		},
		{
			name: "ask user question without spinner - not stuck",
			content: `Please select an option:
  1. Option A
  2. Option B
  3. Option C`,
			expectStuck:  false,
			expectReason: "",
		},
		{
			name:         "spinner without ask user - stuck",
			content:      "✶ Thinking...",
			expectStuck:  true,
			expectReason: "stuck_zero_token_waiting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pane := &PaneInfo{Content: tt.content}

			assert.Equal(t, tt.expectStuck, pane.IsStuck(), "IsStuck mismatch")
			assert.Equal(t, tt.expectReason, pane.GetStuckReason(), "GetStuckReason mismatch")
		})
	}
}

// TestAskUserQuestionIndicator tests the ask_user_question indicator in DetectStuckIndicators.
func TestAskUserQuestionIndicator(t *testing.T) {
	t.Run("ask user question with spinner", func(t *testing.T) {
		pane := &PaneInfo{
			Content: "✶ Thinking...\n\nSelect an option:\n  1. Yes\n  2. No\n\nEnter to select",
		}
		indicators := pane.DetectStuckIndicators()

		assert.True(t, indicators["ask_user_question"])
		assert.True(t, indicators["waiting"])
		// zero_token_waiting should be false because ask_user_question exempts it
		assert.False(t, indicators["zero_token_waiting"])
	})

	t.Run("no ask user question", func(t *testing.T) {
		pane := &PaneInfo{Content: "✶ Thinking..."}
		indicators := pane.DetectStuckIndicators()

		assert.False(t, indicators["ask_user_question"])
		assert.True(t, indicators["zero_token_waiting"])
	})
}

// TestWaitingPatternSpecificity tests waiting patterns don't match mustering.
func TestWaitingPatternSpecificity(t *testing.T) {
	waitingContent := []string{
		"✶ Thinking...",
		"✢ Processing...",
		"✻ Working...",
		"· Waiting...",
	}

	for _, content := range waitingContent {
		t.Run(content, func(t *testing.T) {
			pane := &PaneInfo{Content: content}
			indicators := pane.DetectStuckIndicators()

			assert.False(t, indicators["mustering"],
				"Should NOT detect mustering for waiting pattern: %s", content)
			assert.True(t, indicators["waiting"],
				"Should detect waiting for: %s", content)
		})
	}
}
