package tmux

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestContainsPromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Claude cursor pattern",
			content:  "▌",
			expected: true,
		},
		{
			name:     "Claude cursor with text",
			content:  "some text ▌",
			expected: true,
		},
		{
			name:     "Common prompt",
			content:  "> ",
			expected: true,
		},
		{
			name:     "Shell prompt",
			content:  "$ ",
			expected: true,
		},
		{
			name:     "Root prompt",
			content:  "# ",
			expected: true,
		},
		{
			name:     "Prompt with path prefix",
			content:  "user@host:~/dir $ ",
			expected: true,
		},
		{
			name:     "Ends with >",
			content:  "user@host>",
			expected: true,
		},
		{
			name:     "Ends with $",
			content:  "bash-5.1$",
			expected: true,
		},
		{
			name:     "Ends with #",
			content:  "root@host#",
			expected: true,
		},
		{
			name:     "Empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "Whitespace only",
			content:  "   ",
			expected: false,
		},
		{
			name:     "Regular text",
			content:  "hello world",
			expected: false,
		},
		{
			name:     "Hash in middle of text",
			content:  "test #tag here",
			expected: false,
		},
		{
			name:     "Dollar in middle of text",
			content:  "costs $100 today",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsPromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsPromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.expected)
			}
		})
	}
}

func TestContainsClaudePromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		// Positive cases - should match Claude prompt
		{
			name:     "Exact Claude prompt",
			content:  "❯",
			expected: true,
		},
		{
			name:     "Claude prompt with whitespace",
			content:  "  ❯  ",
			expected: true,
		},
		{
			name:     "Claude prompt in context",
			content:  "user@host:~/dir ❯",
			expected: true,
		},
		{
			name:     "Multi-line with Claude prompt",
			content:  "some output\nmore output\n❯",
			expected: true,
		},
		// Negative cases - should NOT match bash prompts
		{
			name:     "Bash prompt $ (no space)",
			content:  "$",
			expected: false,
		},
		{
			name:     "Bash prompt > (no space)",
			content:  ">",
			expected: false,
		},
		{
			name:     "Bash prompt # (no space)",
			content:  "#",
			expected: false,
		},
		{
			name:     "Bash prompt $ with space",
			content:  "$ ",
			expected: false,
		},
		{
			name:     "Bash prompt > with space",
			content:  "> ",
			expected: false,
		},
		{
			name:     "Bash prompt # with space",
			content:  "# ",
			expected: false,
		},
		{
			name:     "Bash prompt with path",
			content:  "user@host:~/dir $ ",
			expected: false,
		},
		{
			name:     "Empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "Whitespace only",
			content:  "   ",
			expected: false,
		},
		{
			name:     "Regular text",
			content:  "hello world",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsClaudePromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsClaudePromptPattern(%q) = %v, expected %v",
					tt.content, result, tt.expected)
			}
		})
	}
}

func TestContainsTrustPromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		// Positive cases - should match trust prompt
		{
			name:     "Exact trust prompt",
			content:  "Do you trust the files in this folder?",
			expected: true,
		},
		{
			name:     "Trust prompt with whitespace",
			content:  "  Do you trust the files in this folder?  \n",
			expected: true,
		},
		{
			name:     "Trust prompt in multiline output",
			content:  "Some text\nDo you trust the files in this folder?\nMore text",
			expected: true,
		},
		{
			name:     "Trust prompt with surrounding text",
			content:  "Claude Code is asking: Do you trust the files in this folder? Please answer.",
			expected: true,
		},
		// Current Claude Code (2.x) wording (ce-wn4qe).
		{
			name:     "Current-wording trust question",
			content:  "Quick safety check: Is this a project you created or one you trust?",
			expected: true,
		},
		{
			name:     "Current-wording affirmative option only (question scrolled above tail)",
			content:  " ❯ 1. Yes, I trust this folder\n   2. No, exit\n\n Enter to confirm · Esc to cancel",
			expected: true,
		},
		{
			name: "Real captured new trust prompt tail",
			content: " ⚠ This folder pre-approves 20 tool permissions in .claude/settings.local.json:\n" +
				"   Bash(git status), Bash(git status *), Bash(git -C * status *)\n" +
				" These will apply without asking. Only proceed if you trust this configuration.\n\n" +
				" Security guide\n\n" +
				" ❯ 1. Yes, I trust this folder\n   2. No, exit\n\n Enter to confirm · Esc to cancel",
			expected: true,
		},
		// Negative cases - should NOT match
		{
			name:     "Empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "Whitespace only",
			content:  "   \n  ",
			expected: false,
		},
		{
			name:     "Claude ready prompt",
			content:  "❯ ",
			expected: false,
		},
		{
			name:     "Bash prompt",
			content:  "$ ",
			expected: false,
		},
		{
			name:     "Random text",
			content:  "Random output from command",
			expected: false,
		},
		{
			name:     "Similar but not exact trust text",
			content:  "Do you trust this folder?", // Missing "the files in"
			expected: false,
		},
		{
			name:     "Partial trust text",
			content:  "trust the files",
			expected: false,
		},
		{
			name:     "Trust word in different context",
			content:  "I trust you completely",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsTrustPromptPattern(tt.content)
			if result != tt.expected {
				t.Errorf("containsTrustPromptPattern(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestContainsClaudeTrustAffirmative(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{name: "legacy affirmative", content: "❯ 1. Yes, proceed\n  2. No, exit", expected: true},
		{name: "current affirmative", content: "❯ 1. Yes, I trust this folder\n  2. No, exit", expected: true},
		{name: "numbered option without cursor", content: " 1. Yes, I trust this folder", expected: true},
		{name: "question without selector rendered yet", content: "Quick safety check: Is this a project you created or one you trust?", expected: false},
		{name: "empty", content: "", expected: false},
		{name: "unrelated", content: "I trust you completely", expected: false},
		{name: "prose mentions the affirmative phrase", content: "The model replied: Yes, proceed with the migration.", expected: false},
		{name: "prose mentions trust phrase", content: "Answer: Yes, I trust this folder is safe to open.", expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsClaudeTrustAffirmative(tt.content); got != tt.expected {
				t.Errorf("ContainsClaudeTrustAffirmative(%q) = %v, want %v", tt.content, got, tt.expected)
			}
		})
	}
}

func TestContainsResumeFailurePattern(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		wantMatched string
		wantOK      bool
	}{
		{
			name:        "No conversation found",
			line:        "No conversation found with session ID: 61163f27-a35e-4370-be49-f8456401a872",
			wantMatched: "No conversation found",
			wantOK:      true,
		},
		{
			name:        "No messages returned",
			line:        "Error: No messages returned for this session",
			wantMatched: "No messages returned",
			wantOK:      true,
		},
		{
			name:   "Harness prompt is not a failure",
			line:   "❯ ",
			wantOK: false,
		},
		{
			name:   "Shell prompt is not a failure",
			line:   "vbonnet@mac install-skills %",
			wantOK: false,
		},
		{
			name:   "Empty line",
			line:   "",
			wantOK: false,
		},
		{
			name:   "Unrelated error",
			line:   "fatal: not a git repository",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, ok := containsResumeFailurePattern(tt.line)
			if ok != tt.wantOK {
				t.Errorf("containsResumeFailurePattern(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			}
			if ok && matched != tt.wantMatched {
				t.Errorf("containsResumeFailurePattern(%q) matched = %q, want %q", tt.line, matched, tt.wantMatched)
			}
		})
	}
}

func TestClaudePromptPatterns(t *testing.T) {
	// Verify that all expected patterns are defined
	expectedPatterns := map[string]bool{
		"❯":  false, // Claude Code primary prompt
		"▌":  false,
		"> ": false,
		"$ ": false,
		"# ": false,
	}

	if len(ClaudePromptPatterns) != len(expectedPatterns) {
		t.Errorf("Expected %d patterns, got %d", len(expectedPatterns), len(ClaudePromptPatterns))
	}

	for _, pattern := range ClaudePromptPatterns {
		if _, exists := expectedPatterns[pattern]; !exists {
			t.Errorf("Unexpected pattern: %q", pattern)
		}
		expectedPatterns[pattern] = true
	}

	for pattern, found := range expectedPatterns {
		if !found {
			t.Errorf("Missing expected pattern: %q", pattern)
		}
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "No ANSI codes",
			input:    "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "Color codes",
			input:    "\x1b[31mRed text\x1b[0m",
			expected: "Red text",
		},
		{
			name:     "Bracketed paste mode",
			input:    "\x1b[?2004h\x1b[?1004h",
			expected: "",
		},
		{
			name:     "Complex escape sequences from Claude",
			input:    "\x1b[?2004h\x1b[?1004hContent here",
			expected: "Content here",
		},
		{
			name:     "Multiple CSI sequences",
			input:    "\x1b[38;2;215;119;87m ▐\x1b[48;2;0;0;0m▛███▜\x1b[49m▌\x1b[39m   Claude Code",
			expected: " ▐▛███▜▌   Claude Code",
		},
		{
			name:     "OSC sequences",
			input:    "\x1b]0;Title\x07Normal text",
			expected: "Normal text",
		},
		{
			name:     "Mixed sequences",
			input:    "\x1b[?2026h\r\n\x1b[38;2;215;119;87m ▐\x1b[48;2;0;0;0m▛███▜\x1b[49m▌\x1b[39m   \x1b[1mClaude Code\x1b[22m",
			expected: "\r\n ▐▛███▜▌   Claude Code",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only escape sequences",
			input:    "\x1b[0m\x1b[31m\x1b[?2004h",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("stripANSI(%q) = %q, expected %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

// Integration Tests for WaitForClaudePrompt
// These tests verify the capture-pane polling approach works correctly

// TestWaitForClaudePromptPolling tests the capture-pane polling approach
func TestWaitForClaudePromptPolling(t *testing.T) {
	// Skip if tmux not available
	skipIfNoTmux(t)

	// Create a test session with Claude prompt
	sessionName := "test-prompt-detection-polling"
	socketPath := GetSocketPath()

	// Clean up any existing session
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Create session with fake Claude prompt
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	// Send Claude prompt character
	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "❯", "Space")
	if err := sendCmd.Run(); err != nil {
		t.Fatalf("Failed to send prompt: %v", err)
	}

	// Wait for prompt (should succeed quickly)
	if err := WaitForClaudePrompt(sessionName, 5*time.Second); err != nil {
		t.Errorf("WaitForClaudePrompt failed: %v", err)
	}
}

// TestWaitForClaudePromptTimeout tests timeout behavior
func TestWaitForClaudePromptTimeout(t *testing.T) {
	// Skip if tmux not available
	skipIfNoTmux(t)

	sessionName := "test-prompt-timeout"
	socketPath := GetSocketPath()

	// Clean up any existing session
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Create session WITHOUT Claude prompt
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	// Should timeout (no prompt)
	start := time.Now()
	err := WaitForClaudePrompt(sessionName, 2*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}

	// Should timeout after approximately 2 seconds
	if elapsed < 1800*time.Millisecond || elapsed > 3000*time.Millisecond {
		t.Errorf("Timeout took %v, expected ~2s", elapsed)
	}
}

// TestCapturePaneReadsHistoricalOutput verifies capture-pane can read output
// that was printed before we started monitoring (the core issue we're fixing).
// This is the REGRESSION TEST that prevents going back to control mode.
func TestCapturePaneReadsHistoricalOutput(t *testing.T) {
	// Skip if tmux not available
	skipIfNoTmux(t)

	sessionName := "test-historical-output"
	socketPath := GetSocketPath()

	// Clean up any existing session
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Create session
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	// Send output BEFORE we start monitoring
	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "echo 'Historical output' && echo '❯'", "Enter")
	if err := sendCmd.Run(); err != nil {
		t.Fatalf("Failed to send output: %v", err)
	}

	// Wait for command to execute (increased for CI stability)
	time.Sleep(2 * time.Second)

	// NOW try to detect the prompt that was printed earlier
	// This is the key test - control mode would fail here because it only sees NEW output
	if err := WaitForClaudePrompt(sessionName, 10*time.Second); err != nil {
		t.Errorf("Failed to detect historical prompt: %v", err)
	}

	// Verify we can actually read the historical output
	captureCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", sessionName, "-p")
	output, err := captureCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to capture pane: %v", err)
	}

	content := string(output)
	if !strings.Contains(content, "Historical output") {
		t.Errorf("capture-pane didn't read historical output: %q", content)
	}
	if !strings.Contains(content, "❯") {
		t.Errorf("capture-pane didn't read prompt: %q", content)
	}
}

// TestWaitForClaudePromptIgnoresBashPrompts verifies we don't false-positive on bash prompts
func TestWaitForClaudePromptIgnoresBashPrompts(t *testing.T) {
	// Skip if tmux not available
	skipIfNoTmux(t)

	sessionName := "test-bash-prompt"
	socketPath := GetSocketPath()

	// Clean up any existing session
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	// Create session with bash prompt (not Claude)
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	// Send bash-style prompts (should NOT match)
	bashPrompts := []string{"$", ">", "#", "user@host:~$ ", "root@host#"}
	for _, prompt := range bashPrompts {
		sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "echo '"+prompt+"'", "Enter")
		if err := sendCmd.Run(); err != nil {
			t.Fatalf("Failed to send bash prompt %q: %v", prompt, err)
		}
	}

	time.Sleep(500 * time.Millisecond)

	// Should timeout (bash prompts should NOT be detected as Claude prompts)
	err := WaitForClaudePrompt(sessionName, 2*time.Second)
	if err == nil {
		t.Error("Expected timeout on bash prompts, but detection succeeded")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

// BenchmarkWaitForClaudePrompt benchmarks the polling performance
func BenchmarkWaitForClaudePrompt(b *testing.B) {
	// Skip if tmux not available
	skipIfNoTmux(b)

	sessionName := "test-prompt-bench"
	socketPath := GetSocketPath()

	// Setup: Create session with prompt
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	cmd := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName)
	if err := cmd.Run(); err != nil {
		b.Fatalf("Failed to create test session: %v", err)
	}
	defer func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	}()

	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "❯", "Space")
	if err := sendCmd.Run(); err != nil {
		b.Fatalf("Failed to send prompt: %v", err)
	}

	time.Sleep(500 * time.Millisecond) // Ensure prompt is visible

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := WaitForClaudePrompt(sessionName, 5*time.Second); err != nil {
			b.Errorf("Iteration %d failed: %v", i, err)
		}
	}
}
