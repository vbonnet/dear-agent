package tmux

import (
	"testing"
)

// TestSendPromptLiteral_ConditionalESC is a REGRESSION TEST for Bug 2:
// ESC sent unconditionally, interrupting operations instead of queueing
//
// Bug History (2026-03-14):
// - Issue: SendPromptLiteral always sent ESC, interrupting Claude's thinking state
// - Root Cause: No conditional logic - ESC sent on lines 49-54 without checking context
// - Fix: Added shouldInterrupt parameter to make ESC sending conditional
//
// This test documents the conditional ESC behavior.
func TestSendPromptLiteral_ConditionalESC(t *testing.T) {
	t.Log("Conditional ESC Logic in SendPromptLiteral:")
	t.Log("")
	t.Log("OLD BEHAVIOR (Buggy):")
	t.Log("1. Always send ESC to interrupt thinking state")
	t.Log("2. Wait 500ms")
	t.Log("3. Send prompt text")
	t.Log("4. Wait 500ms")
	t.Log("5. Send Enter")
	t.Log("❌ Problem: ESC always sent, interrupts operations even in queue mode")
	t.Log("")
	t.Log("NEW BEHAVIOR (Fixed):")
	t.Log("Function signature: SendPromptLiteral(target, prompt string, shouldInterrupt bool)")
	t.Log("")
	t.Log("When shouldInterrupt=true:")
	t.Log("1. Send ESC to interrupt thinking state")
	t.Log("2. Wait 500ms")
	t.Log("3. Send prompt text")
	t.Log("4. Wait 500ms")
	t.Log("5. Send Enter")
	t.Log("✓ ESC sent - interrupts thinking (intended for --interrupt flag)")
	t.Log("")
	t.Log("When shouldInterrupt=false:")
	t.Log("1. (Skip ESC step)")
	t.Log("2. Send prompt text directly")
	t.Log("3. Wait 500ms")
	t.Log("4. Send Enter")
	t.Log("✓ No ESC sent - message queued instead of interrupting")
	t.Log("")
	t.Log("CALL SITES:")
	t.Log("- send.go:sendDirectly()        → shouldInterrupt=true (--interrupt flag)")
	t.Log("- new.go:--prompt handling      → shouldInterrupt=false (no interrupt)")
	t.Log("- select_option.go              → shouldInterrupt=true (UI interaction)")
	t.Log("")
	t.Log("VERIFICATION:")
	t.Log("Manual test - Queue mode (no ESC):")
	t.Log("  agm session new test-send")
	t.Log("  agm session send test-send --prompt='Test'")
	t.Log("  Expected: '⏳ Message queued' (no interruption)")
	t.Log("")
	t.Log("Manual test - Interrupt mode (sends ESC):")
	t.Log("  agm session send test-send --interrupt --prompt='Test'")
	t.Log("  Expected: '✓ Message sent' (interruption occurred)")
	t.Log("")
	t.Log("RELATED CHANGES:")
	t.Log("- SendMultiLinePromptSafe also updated with shouldInterrupt parameter")
	t.Log("- SendPromptFileSafe also updated with shouldInterrupt parameter")
	t.Log("- All call sites updated to pass correct boolean value")
}

// TestSendPromptLiteral_ParameterPropagation documents parameter flow
func TestSendPromptLiteral_ParameterPropagation(t *testing.T) {
	t.Log("Parameter Propagation Through Call Chain:")
	t.Log("")
	t.Log("HIGH-LEVEL FUNCTIONS:")
	t.Log("- SendMultiLinePromptSafe(session, prompt, shouldInterrupt)")
	t.Log("  └─> SendPromptLiteral(session, prompt, shouldInterrupt)")
	t.Log("")
	t.Log("- SendPromptFileSafe(session, file, shouldInterrupt)")
	t.Log("  └─> SendPromptLiteral(session, content, shouldInterrupt)")
	t.Log("")
	t.Log("COMMAND-LEVEL CALLERS:")
	t.Log("- cmd/agm/send.go:sendViaTmux()")
	t.Log("  - Interrupt mode: shouldInterrupt=true")
	t.Log("  - Queue mode: shouldInterrupt=false")
	t.Log("")
	t.Log("- cmd/agm/new.go:--prompt handling")
	t.Log("  - Always: shouldInterrupt=false (session init, no interrupt needed)")
	t.Log("")
	t.Log("PARAMETER FLOW EXAMPLE:")
	t.Log("agm session send → --interrupt flag checked")
	t.Log("  → sendDirectly()")
	t.Log("    → sendViaTmux(..., interrupt=true)")
	t.Log("      → SendMultiLinePromptSafe(..., true)")
	t.Log("        → SendPromptLiteral(..., true)")
	t.Log("          → ESC sent ✓")
}

// Regression tests for queue timing bugs (2026-03-31)
// Bug 1: Queue drain too aggressive — message delivered in same cycle as human submission
// Bug 2: Agent message interrupts human typing on input line

func TestInputLineHasContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty prompt line",
			input:    "some output\n❯",
			expected: false,
		},
		{
			name:     "prompt with trailing space only",
			input:    "some output\n❯ ",
			expected: false,
		},
		{
			name:     "human typing after prompt",
			input:    "some output\n❯ please fix the b",
			expected: true,
		},
		{
			name:     "human typed full command",
			input:    "response text\n❯ /commit",
			expected: true,
		},
		{
			name:     "no prompt character at all",
			input:    "Processing...\nThinking...",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "prompt on earlier line, empty last line",
			input:    "❯ old command\nresponse\n",
			expected: true,
		},
		{
			name:     "multiple prompt lines — uses last one",
			input:    "❯ old command\nresponse\n❯",
			expected: false,
		},
		{
			name:     "multiple prompt lines — last has content",
			input:    "❯ old command\nresponse\n❯ new text",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InputLineHasContent(tt.input)
			if got != tt.expected {
				t.Errorf("InputLineHasContent(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestInputLineHasContent_ForceBypass(t *testing.T) {
	// Documents that --force (shouldInterrupt=true) bypasses InputLineHasContent checks
	// in SendPromptLiteral. The check is wrapped in `if !shouldInterrupt { ... }`.
	//
	// When shouldInterrupt=true:
	//   - hasQueuedInput check: SKIPPED
	//   - InputLineHasContent check: SKIPPED
	//   - ESC is sent to interrupt thinking
	//
	// When shouldInterrupt=false:
	//   - hasQueuedInput check: ACTIVE — aborts if pasted text detected
	//   - InputLineHasContent check: ACTIVE — aborts if human is typing
	//   - ESC is NOT sent

	// Verify the function itself works correctly (the bypass is in SendPromptLiteral)
	content := "❯ human is typing something"
	if !InputLineHasContent(content) {
		t.Error("InputLineHasContent should detect typed content")
	}
}

// Regression tests for spinner detection (2026-04-10)
// Bug: AI-generated output during active generation was falsely classified as
// human typing because pane content changed between captures.

func TestHasActiveSpinner(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "spinner char ⠋",
			input:    "⠋ Thinking...",
			expected: true,
		},
		{
			name:     "spinner char ⠙",
			input:    "⠙ Reading file.go",
			expected: true,
		},
		{
			name:     "spinner char ⠹",
			input:    "⠹ Running tests",
			expected: true,
		},
		{
			name:     "spinner char ⠸",
			input:    "⠸ Building",
			expected: true,
		},
		{
			name:     "spinner char ⠼",
			input:    "⠼ Processing",
			expected: true,
		},
		{
			name:     "spinner char ⠴",
			input:    "⠴ Searching",
			expected: true,
		},
		{
			name:     "spinner char ⠦",
			input:    "⠦ Writing",
			expected: true,
		},
		{
			name:     "spinner char ⠧",
			input:    "⠧ Analyzing",
			expected: true,
		},
		{
			name:     "spinner char ⠇",
			input:    "⠇ Editing",
			expected: true,
		},
		{
			name:     "spinner char ⠏",
			input:    "⠏ Compiling",
			expected: true,
		},
		{
			name:     "spinner in middle of output",
			input:    "some output\n⠋ Thinking...\n❯",
			expected: true,
		},
		{
			name:     "no spinner — clean prompt",
			input:    "Response text\n❯",
			expected: false,
		},
		{
			name:     "no spinner — empty",
			input:    "",
			expected: false,
		},
		{
			name:     "no spinner — normal text",
			input:    "Building project...\nTests passed.\n❯",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasActiveSpinner(tt.input)
			if got != tt.expected {
				t.Errorf("hasActiveSpinner(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSpinnerBypassesHumanInputDetection(t *testing.T) {
	// When a spinner is present, InputLineHasContent may return true because
	// AI-generated text appears after ❯. But hasActiveSpinner should cause the
	// caller to skip the InputLineHasContent check entirely.
	paneWithSpinnerAndContent := "⠋ Thinking...\n❯ some AI output text"

	// InputLineHasContent would flag this as human typing
	if !InputLineHasContent(paneWithSpinnerAndContent) {
		t.Fatal("expected InputLineHasContent to detect content (precondition)")
	}

	// But hasActiveSpinner detects AI generation, so caller should skip the check
	if !hasActiveSpinner(paneWithSpinnerAndContent) {
		t.Fatal("expected hasActiveSpinner to detect spinner")
	}

	// The combined logic: if spinner is active, do NOT treat content as human input
	// This is the fix — callers check hasActiveSpinner before InputLineHasContent
	isHumanTyping := !hasActiveSpinner(paneWithSpinnerAndContent) && InputLineHasContent(paneWithSpinnerAndContent)
	if isHumanTyping {
		t.Error("spinner present — should NOT classify as human typing")
	}
}

func TestSpinnerBypassesQueuedInputDetection(t *testing.T) {
	// Spinner should also bypass hasQueuedInput checks (e.g., "[Pasted text" can
	// appear transiently during AI generation).
	paneWithSpinnerAndQueued := "⠙ Running tests\n[Pasted text #1 +2 lines]\n❯"

	if !hasActiveSpinner(paneWithSpinnerAndQueued) {
		t.Fatal("expected hasActiveSpinner to detect spinner")
	}

	if !hasQueuedInput(paneWithSpinnerAndQueued) {
		t.Fatal("expected hasQueuedInput to detect queued input (precondition)")
	}

	// Combined logic: spinner active means skip both checks
	shouldBlock := !hasActiveSpinner(paneWithSpinnerAndQueued) && (hasQueuedInput(paneWithSpinnerAndQueued) || InputLineHasContent(paneWithSpinnerAndQueued))
	if shouldBlock {
		t.Error("spinner present — should NOT block delivery")
	}
}

// Regression tests for paste-buffer fix (commit e63b2ee)
// Bug: agm send msg pastes into input buffer instead of submitting when
// session starts inference between prompt detection and text delivery.

func TestHasQueuedInput_DetectsPastedText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "simple pasted text indicator",
			input:    "[Pasted text #1 +3 lines]",
			expected: true,
		},
		{
			name:     "pasted text with surrounding content",
			input:    "some output\n[Pasted text #2 +1 lines]\n❯",
			expected: true,
		},
		{
			name:     "pasted text partial match",
			input:    "[Pasted text",
			expected: true,
		},
		{
			name:     "clean pane output no paste",
			input:    "❯ hello world\nsome response text",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "prompt only",
			input:    "❯",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasQueuedInput(tt.input)
			if got != tt.expected {
				t.Errorf("hasQueuedInput(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestHasQueuedInput_DetectsQueuedMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "queued messages prompt",
			input:    "Press up to edit queued messages",
			expected: true,
		},
		{
			name:     "queued messages with context",
			input:    "output line\nPress up to edit queued messages\n❯",
			expected: true,
		},
		{
			name:     "no queued messages",
			input:    "normal output without queued text",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasQueuedInput(tt.input)
			if got != tt.expected {
				t.Errorf("hasQueuedInput(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestContainsClaudePromptPattern_ForPasteVerification(t *testing.T) {
	// Tests the condition used in the paste-buffer retry loop (prompt.go:95-126):
	// re-send Enter only when BOTH hasQueuedInput=true AND prompt is visible.
	tests := []struct {
		name            string
		content         string
		expectPrompt    bool
		expectQueued    bool
		shouldResendKey bool
	}{
		{
			name:            "queued input with prompt visible - should re-send Enter",
			content:         "[Pasted text #1 +2 lines]\n❯",
			expectPrompt:    true,
			expectQueued:    true,
			shouldResendKey: true,
		},
		{
			name:            "queued input without prompt - session still working",
			content:         "[Pasted text #1 +2 lines]\nProcessing...",
			expectPrompt:    false,
			expectQueued:    true,
			shouldResendKey: false,
		},
		{
			name:            "no queued input with prompt - message submitted normally",
			content:         "Response text\n❯",
			expectPrompt:    true,
			expectQueued:    false,
			shouldResendKey: false,
		},
		{
			name:            "empty content",
			content:         "",
			expectPrompt:    false,
			expectQueued:    false,
			shouldResendKey: false,
		},
		{
			name:            "bash prompt should not trigger re-send",
			content:         "[Pasted text #1 +2 lines]\n$ ",
			expectPrompt:    false,
			expectQueued:    true,
			shouldResendKey: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrompt := containsClaudePromptPattern(tt.content)
			gotQueued := hasQueuedInput(tt.content)
			resend := gotQueued
			if !gotPrompt {
				resend = false
			}

			if gotPrompt != tt.expectPrompt {
				t.Errorf("containsClaudePromptPattern(%q) = %v, want %v", tt.content, gotPrompt, tt.expectPrompt)
			}
			if gotQueued != tt.expectQueued {
				t.Errorf("hasQueuedInput(%q) = %v, want %v", tt.content, gotQueued, tt.expectQueued)
			}
			if resend != tt.shouldResendKey {
				t.Errorf("shouldResendEnter = %v, want %v (prompt=%v, queued=%v)",
					resend, tt.shouldResendKey, gotPrompt, gotQueued)
			}
		})
	}
}
