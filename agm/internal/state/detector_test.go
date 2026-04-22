package state

import (
	"strings"
	"testing"
	"time"
)

func TestDetector_DetectState_Ready(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Claude prompt at end",
			output: "Previous output here\n❯ ",
		},
		{
			name:   "Claude prompt with trailing space",
			output: "Some text\n❯  ",
		},
		{
			name:   "Multi-line with prompt",
			output: "Line 1\nLine 2\nLine 3\n❯ ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateReady {
				t.Errorf("Expected StateReady, got %v", result.State)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_DetectState_Thinking(t *testing.T) {
	detector := NewDetector()

	// All spinner characters
	spinners := []rune{'⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'}

	for _, spinner := range spinners {
		t.Run(string(spinner), func(t *testing.T) {
			output := "Processing your request " + string(spinner) + " please wait"
			result := detector.DetectState(output, time.Now())

			if result.State != StateThinking {
				t.Errorf("Expected StateThinking for spinner %c, got %v", spinner, result.State)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}

			if !strings.Contains(result.Evidence, string(spinner)) {
				t.Errorf("Expected evidence to contain spinner, got: %s", result.Evidence)
			}
		})
	}
}

func TestDetector_DetectState_BlockedAuth(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Standard y/N prompt",
			output: "Do you want to continue? (y/N): ",
		},
		{
			name:   "Capital Y/n",
			output: "Approve this action? (Y/n): ",
		},
		{
			name:   "Reversed n/Y",
			output: "Confirm deletion? (n/Y): ",
		},
		{
			name:   "With context",
			output: "This will modify 5 files. Proceed? (y/N): ",
		},
		{
			name:   "Case insensitive",
			output: "Allow access to filesystem? (Y/N): ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateBlockedAuth {
				t.Errorf("Expected StateBlockedAuth, got %v", result.State)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_DetectState_BlockedInput(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name: "Question keyword with numbered options",
			output: `Which approach should I use?
1. Option A
2. Option B
3. Option C`,
		},
		{
			name: "Choose keyword with lettered options",
			output: `Choose a color:
A. Red
B. Blue
C. Green`,
		},
		{
			name:   "Bracketed options",
			output: "Select mode: [Development] [Production] [Staging]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateBlockedInput {
				t.Errorf("Expected StateBlockedInput, got %v", result.State)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_DetectState_Stuck(t *testing.T) {
	detector := NewDetector()
	detector.SetStuckThreshold(60 * time.Second)

	// Simulate last output 90 seconds ago
	lastOutputTime := time.Now().Add(-90 * time.Second)

	output := "Working on something..." // No prompt, no spinner

	result := detector.DetectState(output, lastOutputTime)

	if result.State != StateStuck {
		t.Errorf("Expected StateStuck, got %v", result.State)
	}

	if result.Confidence != "medium" {
		t.Errorf("Expected medium confidence, got %s", result.Confidence)
	}

	if !strings.Contains(result.Evidence, "No tokens") {
		t.Errorf("Expected evidence about no tokens, got: %s", result.Evidence)
	}
}

func TestDetector_DetectState_NotStuckWhenReady(t *testing.T) {
	detector := NewDetector()
	detector.SetStuckThreshold(60 * time.Second)

	// Even if last output was long ago, if we're at ready prompt, not stuck
	lastOutputTime := time.Now().Add(-120 * time.Second)
	output := "Previous work completed\n❯ "

	result := detector.DetectState(output, lastOutputTime)

	if result.State != StateReady {
		t.Errorf("Expected StateReady (not stuck), got %v", result.State)
	}
}

func TestDetector_DetectState_Unknown(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Plain text",
			output: "Just some regular output",
		},
		{
			name:   "Partial prompt",
			output: "Here is the result",
		},
		{
			name:   "Empty output",
			output: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateUnknown {
				t.Errorf("Expected StateUnknown, got %v", result.State)
			}

			if result.Confidence != "low" {
				t.Errorf("Expected low confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_PriorityOrder(t *testing.T) {
	detector := NewDetector()

	// Priority order: Ready > WaitingAgent > Looping > Thinking > Blocked > Stuck > Unknown
	// A spinner takes priority over blocked_auth. Output has both a spinner and
	// a y/N prompt but no ❯ at end, so ready doesn't match, then thinking wins.
	output := "Processing ⣾ Do you want to continue? (y/N): "

	result := detector.DetectState(output, time.Now())

	if result.State != StateThinking {
		t.Errorf("Expected StateThinking (spinner takes priority over blocked_auth), got %v", result.State)
	}
}

func TestDetector_ReadyTakesPriorityOverBlockedInput(t *testing.T) {
	detector := NewDetector()

	// Claude's response contains numbered list but prompt is visible at bottom
	output := "Which approach should I use?\n1. Option A\n2. Option B\n3. Option C\n\nI went with option 1.\n❯ "

	result := detector.DetectState(output, time.Now())

	if result.State != StateReady {
		t.Errorf("Expected StateReady (prompt visible takes priority over numbered list), got %v", result.State)
	}
}

func TestDetector_ReadyTakesPriorityOverBlockedAuth(t *testing.T) {
	detector := NewDetector()

	// Claude's response mentions y/N but prompt is visible at bottom
	output := "I answered (y/N) with yes.\n❯ "

	result := detector.DetectState(output, time.Now())

	if result.State != StateReady {
		t.Errorf("Expected StateReady (prompt visible takes priority over y/N in content), got %v", result.State)
	}
}

func TestState_IsBlocked(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateBlockedAuth, true},
		{StateBlockedInput, true},
		{StateReady, false},
		{StateThinking, false},
		{StateStuck, false},
		{StateUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if tt.state.IsBlocked() != tt.expected {
				t.Errorf("IsBlocked() for %s: expected %v, got %v",
					tt.state, tt.expected, tt.state.IsBlocked())
			}
		})
	}
}

func TestState_IsActive(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateThinking, true},
		{StateWaitingAgent, true},
		{StateLooping, true},
		{StateReady, false},
		{StateBlockedAuth, false},
		{StateBlockedInput, false},
		{StateStuck, false},
		{StateUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if tt.state.IsActive() != tt.expected {
				t.Errorf("IsActive() for %s: expected %v, got %v",
					tt.state, tt.expected, tt.state.IsActive())
			}
		})
	}
}

func TestState_IsIdle(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateReady, true},
		{StateThinking, false},
		{StateBlockedAuth, false},
		{StateBlockedInput, false},
		{StateStuck, false},
		{StateUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if tt.state.IsIdle() != tt.expected {
				t.Errorf("IsIdle() for %s: expected %v, got %v",
					tt.state, tt.expected, tt.state.IsIdle())
			}
		})
	}
}

func TestDetector_CustomStuckThreshold(t *testing.T) {
	detector := NewDetector()

	// Set custom threshold of 30 seconds
	detector.SetStuckThreshold(30 * time.Second)

	lastOutputTime := time.Now().Add(-45 * time.Second)
	output := "Working..."

	result := detector.DetectState(output, lastOutputTime)

	if result.State != StateStuck {
		t.Errorf("Expected StateStuck with 30s threshold, got %v", result.State)
	}
}

func TestDetector_DetectState_WaitingAgent(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Agent launched with spinner",
			output: "I'll use the Agent tool ⣾ Agent: explore-code running",
		},
		{
			name:   "Launching sub-agent with spinner",
			output: "⣽ Launching sub-agent to search the codebase",
		},
		{
			name:   "Agent working with spinner",
			output: "⣻ Agent to find the relevant files",
		},
		{
			name:   "Background task with spinner",
			output: "⢿ run_in_background task processing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateWaitingAgent {
				t.Errorf("Expected StateWaitingAgent, got %v (evidence: %s)", result.State, result.Evidence)
			}

			if result.Confidence != "medium" {
				t.Errorf("Expected medium confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_DetectState_Looping(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Loop command with spinner",
			output: "⣾ /loop 5m checking deploy status",
		},
		{
			name:   "Monitoring interval with spinner",
			output: "⣽ Monitoring every 30 seconds for changes",
		},
		{
			name:   "Polling interval with spinner",
			output: "⣻ Polling every 60 seconds",
		},
		{
			name:   "Running iteration with spinner",
			output: "⢿ Running iteration 5 of the check cycle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateLooping {
				t.Errorf("Expected StateLooping, got %v (evidence: %s)", result.State, result.Evidence)
			}

			if result.Confidence != "medium" {
				t.Errorf("Expected medium confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_WaitingAgentRequiresSpinner(t *testing.T) {
	detector := NewDetector()

	// Agent keywords without spinner should not match waiting_agent
	output := "I used the Agent tool earlier to find the file"
	result := detector.DetectState(output, time.Now())

	if result.State == StateWaitingAgent {
		t.Errorf("Expected NOT StateWaitingAgent without spinner, got %v", result.State)
	}
}

func TestDetector_LoopingRequiresSpinner(t *testing.T) {
	detector := NewDetector()

	// Loop keywords without spinner should NOT match looping
	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Loop command without spinner",
			output: "/loop 5m checking deploy status",
		},
		{
			name:   "Monitoring keyword without spinner",
			output: "Monitoring every 30 seconds for changes",
		},
		{
			name:   "Iteration keyword without spinner",
			output: "Running iteration 5 of the check cycle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())
			if result.State == StateLooping {
				t.Errorf("Expected NOT StateLooping without spinner, got %v", result.State)
			}
		})
	}
}

func TestDetector_NumberedListWithoutQuestionKeyword(t *testing.T) {
	detector := NewDetector()

	// Numbered lists in Claude's output should NOT trigger blocked_input
	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Plain numbered list",
			output: "Here are the steps:\n1. Install dependencies\n2. Run tests\n3. Deploy",
		},
		{
			name:   "Just options keyword alone",
			output: "Choose between the following options:",
		},
		{
			name:   "Just numbered items alone",
			output: "1. First item\n2. Second item\n3. Third item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())
			if result.State == StateBlockedInput {
				t.Errorf("Expected NOT StateBlockedInput for plain content, got %v", result.State)
			}
		})
	}
}

func TestState_IsWaiting(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateWaitingAgent, true},
		{StateThinking, false},
		{StateLooping, false},
		{StateReady, false},
		{StateUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if tt.state.IsWaiting() != tt.expected {
				t.Errorf("IsWaiting() for %s: expected %v, got %v",
					tt.state, tt.expected, tt.state.IsWaiting())
			}
		})
	}
}

func TestDetector_extractEvidence(t *testing.T) {
	detector := NewDetector()

	output := "This is a long line of text with y/N prompt in the middle and more text after"
	pattern := detector.blockedAuthPattern

	evidence := detector.extractEvidence(output, pattern, 20)

	if !strings.Contains(evidence, "y/N") {
		t.Errorf("Evidence should contain matched pattern, got: %s", evidence)
	}

	// Evidence should be truncated to context length
	if len(evidence) > 100 { // Pattern + 2*20 chars context + some buffer
		t.Errorf("Evidence too long: %d chars", len(evidence))
	}
}

func TestDetector_DetectState_BlockedPermission(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Standard permission prompt",
			output: "Do you want to proceed?\n❯ 1. Yes\n  2. No",
		},
		{
			name:   "Permission prompt with context above",
			output: "I'll modify the file now.\n\nDo you want to proceed?\n❯ 1. Yes\n  2. No",
		},
		{
			name:   "Permission prompt with Allow option",
			output: "Do you want to proceed?\n❯ 1. Allow\n  2. Deny",
		},
		{
			name:   "Just the question text",
			output: "Some output above\nDo you want to proceed?\nMore context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateBlockedPermission {
				t.Errorf("Expected StateBlockedPermission, got %v (evidence: %s)", result.State, result.Evidence)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_PermissionTakesPriorityOverReady(t *testing.T) {
	detector := NewDetector()

	// Permission prompt has ❯ as selector — should NOT be detected as ready
	output := "Do you want to proceed?\n❯ 1. Yes\n  2. No"
	result := detector.DetectState(output, time.Now())

	if result.State != StateBlockedPermission {
		t.Errorf("Expected StateBlockedPermission (not StateReady), got %v", result.State)
	}
}

func TestDetector_ReadyAfterPermissionDismissed(t *testing.T) {
	detector := NewDetector()

	// After permission is resolved, Claude shows normal prompt
	output := "I proceeded with the changes.\n❯ "
	result := detector.DetectState(output, time.Now())

	if result.State != StateReady {
		t.Errorf("Expected StateReady after permission resolved, got %v", result.State)
	}
}

func TestDetector_CheckCanReceive(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name     string
		output   string
		expected CanReceive
	}{
		{
			name:     "Ready prompt = YES",
			output:   "Previous output\n❯ ",
			expected: CanReceiveYes,
		},
		{
			name:     "Permission prompt = NO",
			output:   "Do you want to proceed?\n❯ 1. Yes\n  2. No",
			expected: CanReceiveNo,
		},
		{
			name:     "Spinner (thinking) = QUEUE",
			output:   "Processing ⣾ your request",
			expected: CanReceiveQueue,
		},
		{
			name:     "No prompt visible = QUEUE",
			output:   "Working on something...",
			expected: CanReceiveQueue,
		},
		{
			name:     "Empty output = QUEUE",
			output:   "",
			expected: CanReceiveQueue,
		},
		{
			name:     "Prompt with status bar below = YES",
			output:   "━━━━━━━━━━━━━━━━━━━━\n❯\u00a0\n━━━━━━━━━━━━━━━━━━━━\n\n\n",
			expected: CanReceiveYes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.CheckCanReceive(tt.output)
			if got != tt.expected {
				t.Errorf("CheckCanReceive() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestState_IsBlocked_IncludesPermission(t *testing.T) {
	if !StateBlockedPermission.IsBlocked() {
		t.Error("Expected StateBlockedPermission.IsBlocked() to return true")
	}
}

func TestDetector_DetectState_BackgroundTasksView(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Standard Background Tasks overlay",
			output: "Background tasks\nNo tasks currently running\n\u2191/\u2193 to select \u00b7 Enter to view \u00b7 \u2190/Esc to close",
		},
		{
			name:   "Background Tasks with content above",
			output: "Some previous output here\n\nBackground tasks\n\n  Task 1 - running\n  Task 2 - complete\n\n\u2191/\u2193 to select \u00b7 Enter to view \u00b7 \u2190/Esc to close",
		},
		{
			name:   "Background Tasks minimal",
			output: "Background tasks\n\u2191/\u2193 to select \u00b7 Enter to view \u00b7 \u2190/Esc to close",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State != StateBackgroundTasksView {
				t.Errorf("Expected StateBackgroundTasksView, got %v (evidence: %s)", result.State, result.Evidence)
			}

			if result.Confidence != "high" {
				t.Errorf("Expected high confidence, got %s", result.Confidence)
			}
		})
	}
}

func TestDetector_BackgroundTasksView_NotFalsePositive(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "Mention of Background tasks without overlay chrome",
			output: "I checked the Background tasks and found nothing relevant.\n\u276f ",
		},
		{
			name:   "Just the text Background tasks",
			output: "Background tasks are handled by the daemon.",
		},
		{
			name:   "Just navigation hints without Background tasks",
			output: "\u2191/\u2193 to select \u00b7 Enter to view \u00b7 \u2190/Esc to close",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.DetectState(tt.output, time.Now())

			if result.State == StateBackgroundTasksView {
				t.Errorf("Expected NOT StateBackgroundTasksView for non-overlay content, got %v", result.State)
			}
		})
	}
}

func TestDetector_BackgroundTasksTakesPriorityOverReady(t *testing.T) {
	detector := NewDetector()

	// The overlay may show while the prompt is visible underneath
	output := "Background tasks\nNo tasks currently running\n\u2191/\u2193 to select \u00b7 Enter to view \u00b7 \u2190/Esc to close\n\u276f "
	result := detector.DetectState(output, time.Now())

	if result.State != StateBackgroundTasksView {
		t.Errorf("Expected StateBackgroundTasksView (overlay takes priority over ready), got %v", result.State)
	}
}

func TestDetector_CheckCanReceive_Overlay(t *testing.T) {
	detector := NewDetector()

	tests := []struct {
		name     string
		output   string
		expected CanReceive
	}{
		{
			name:     "Background Tasks overlay = OVERLAY",
			output:   "Background tasks\nNo tasks currently running\n\u2191/\u2193 to select \u00b7 Enter to view \u00b7 \u2190/Esc to close",
			expected: CanReceiveOverlay,
		},
		{
			name:     "Background Tasks with prompt underneath = OVERLAY",
			output:   "Background tasks\n\u2191/\u2193 to select \u00b7 Enter to view \u00b7 \u2190/Esc to close\n\u276f ",
			expected: CanReceiveOverlay,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detector.CheckCanReceive(tt.output)
			if got != tt.expected {
				t.Errorf("CheckCanReceive() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestState_IsOverlay(t *testing.T) {
	tests := []struct {
		state    State
		expected bool
	}{
		{StateBackgroundTasksView, true},
		{StateReady, false},
		{StateThinking, false},
		{StateBlockedPermission, false},
		{StateBlockedAuth, false},
		{StateUnknown, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if tt.state.IsOverlay() != tt.expected {
				t.Errorf("IsOverlay() for %s: expected %v, got %v",
					tt.state, tt.expected, tt.state.IsOverlay())
			}
		})
	}
}
