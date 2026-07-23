package tmux

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// newCodexTestSession starts a detached tmux session for a readiness test and
// registers cleanup. When cmd is non-empty it runs that command (e.g. a fake
// codex script) instead of an interactive shell. Returns the socket path.
func newCodexTestSession(t *testing.T, sessionName string, cmd ...string) string {
	t.Helper()
	skipIfNoTmux(t)
	socketPath := GetSocketPath()
	exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()

	args := append([]string{"-S", socketPath, "new-session", "-d", "-s", sessionName}, cmd...)
	if err := exec.Command("tmux", args...).Run(); err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}
	t.Cleanup(func() {
		exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
	})
	return socketPath
}

func TestIsCodexComposerReady(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "structured initial composer",
			content:  "│ >_ OpenAI Codex (v0.141.0) │\n│ model: gpt-5.5 xhigh /model to change │\n╰──────────────────────────────╯\n›",
			expected: true,
		},
		{
			name:     "typed initial draft is not ready",
			content:  "│ >_ OpenAI Codex (v0.141.0) │\n│ model: gpt-5.5 xhigh /model to change │\n╰──────────────────────────────╯\n› Continue the task",
			expected: false,
		},
		{
			name:     "initial paste chip is not ready",
			content:  "│ >_ OpenAI Codex (v0.141.0) │\n│ model: gpt-5.5 xhigh /model to change │\n╰──────────────────────────────╯\n› [Pasted Content 2172 chars]",
			expected: false,
		},
		{
			name:     "header alone is incomplete",
			content:  "│ >_ OpenAI Codex (v0.141.0) │",
			expected: false,
		},
		{
			name:     "model status hint alone is incomplete",
			content:  "│ model:     gpt-5.5 xhigh   /model to change │",
			expected: false,
		},
		{
			name:     "post-turn cursor and footer",
			content:  "›\n\n  gpt-5.6 xhigh · ~/src/project",
			expected: true,
		},
		{
			name:     "typed post-turn draft is not ready",
			content:  "› Continue the task\n\n  gpt-5.6 xhigh · ~/src/project",
			expected: false,
		},
		{
			name:     "post-turn composer followed by shell prompt is stale",
			content:  "› Continue the task\n\n  gpt-5.6 xhigh · ~/src/project\nuser@host:~/src/project$",
			expected: false,
		},
		{
			name:     "new initial composer supersedes stale post-turn footer",
			content:  "› Previous turn\n  gpt-5.6 xhigh · ~/src/project\n│ >_ OpenAI Codex (v0.142.0) │\n│ model: gpt-5.6 xhigh /model to change │\n╰──────────────────────────────╯\n›",
			expected: true,
		},
		{
			name:     "working footer is not ready",
			content:  "• Working (3s • esc to interrupt)\n  gpt-5.6 xhigh · ~/src/project",
			expected: false,
		},
		{
			name:     "latest working footer overrides stale initial composer",
			content:  "│ >_ OpenAI Codex (v0.141.0) │\n│ /model to change │\n• Working (3s • esc to interrupt)\n  gpt-5.6 xhigh · ~/src/project",
			expected: false,
		},
		{
			name:     "initial composer followed by shell prompt is stale",
			content:  "│ >_ OpenAI Codex (v0.141.0) │\n│ /model to change │\n╰──────────────────────────────╯\nuser@host:~/src/project$",
			expected: false,
		},
		{
			name:     "unsubmitted paste before footer is not ready",
			content:  "› [Pasted Content 2172 chars]\n  gpt-5.6 xhigh · ~/src/project",
			expected: false,
		},
		{
			name:     "echoed launch model is not ready",
			content:  "user@host$ codex resume abc -m 'gpt-5.6'",
			expected: false,
		},
		{
			name:     "input box top border",
			content:  "╭───────────────────────────────────────╮",
			expected: false,
		},
		{
			name:     "input box bottom border",
			content:  "╰───────────────────────────────────────╯",
			expected: false,
		},
		{
			name:     "decorated shell prompt is not a Codex prompt",
			content:  "╭─ user@host ~/work\n╰─ %",
			expected: false,
		},
		{
			name:     "bash prompt is not a Codex prompt",
			content:  "user@host:~/work$",
			expected: false,
		},
		{
			name:     "claude prompt char is not a Codex prompt",
			content:  "❯ ",
			expected: false,
		},
		{
			name:     "codex input cursor alone is not a ready signal",
			content:  "› 1. Yes, continue", // also appears in the trust dialog
			expected: false,
		},
		{
			name:     "empty string",
			content:  "",
			expected: false,
		},
		{
			name:     "whitespace only",
			content:  "   \t  ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCodexComposerReady(tt.content); got != tt.expected {
				t.Errorf("IsCodexComposerReady(%q) = %v, expected %v", tt.content, got, tt.expected)
			}
		})
	}
}

func TestContainsCodexTrustPromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "exact first-run trust prompt (codex v0.141.0)",
			content:  "  Do you trust the contents of this directory? Working with untrusted contents",
			expected: true,
		},
		{
			name:     "trust phrasing fragment",
			content:  "trust the contents of this directory",
			expected: true,
		},
		{
			name:     "ready composer is not a trust prompt",
			content:  "│ >_ OpenAI Codex (v0.141.0)            │",
			expected: false,
		},
		{
			name:     "claude trust phrasing is not matched",
			content:  "Do you trust the files in this folder?",
			expected: false,
		},
		{
			name:     "empty string",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsCodexTrustPromptPattern(tt.content); got != tt.expected {
				t.Errorf("containsCodexTrustPromptPattern(%q) = %v, expected %v", tt.content, got, tt.expected)
			}
		})
	}
}

func TestContainsCodexModelUpgradePromptPattern(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "model upgrade choice prompt",
			content:  "Choose how you'd like Codex to proceed.\n\n› 1. Try new model\n  2. Use existing model",
			expected: true,
		},
		{
			name:     "ready composer is not a model upgrade prompt",
			content:  "│ >_ OpenAI Codex (v0.141.0)            │",
			expected: false,
		},
		{
			name:     "empty string",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsCodexModelUpgradePromptPattern(tt.content); got != tt.expected {
				t.Errorf("containsCodexModelUpgradePromptPattern(%q) = %v, expected %v", tt.content, got, tt.expected)
			}
		})
	}
}

func TestIsCodexHookReviewRequired(t *testing.T) {
	const activeReview = `Hooks need review

4 hooks are new or changed.

Hooks can run outside the sandbox after you trust them.

› 1. Review hooks
  2. Trust all and continue
  3. Continue without trusting (hooks won't run)

Press enter to confirm or esc to go back`

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Codex 0.145 structured hook review",
			content:  activeReview,
			expected: true,
		},
		{
			name:     "weak transcript text is not an active selector",
			content:  "The release notes say hooks need review before use.",
			expected: false,
		},
		{
			name: "newer composer supersedes retained selector",
			content: activeReview + `
review completed
│ >_ OpenAI Codex (v0.145.0) │
│ model: gpt-5.6 high /model to change │
╰──────────────────────────────╯
›`,
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
			if got := IsCodexHookReviewRequired(tt.content); got != tt.expected {
				t.Fatalf("IsCodexHookReviewRequired() = %t, want %t", got, tt.expected)
			}
		})
	}
}

// CodexPromptPatterns must include the stable composer header so detection does
// not rely solely on box-drawing corners (which Codex shares with Gemini).
func TestCodexPromptPatternsIncludeHeader(t *testing.T) {
	if !slices.Contains(CodexPromptPatterns, "OpenAI Codex") {
		t.Errorf("CodexPromptPatterns should include the %q header marker; got %v", "OpenAI Codex", CodexPromptPatterns)
	}
}

// containsAnyHarnessPromptPattern must also recognize Codex's composer so the
// harness-agnostic send path (SendPromptLiteral) detects Codex readiness.
func TestContainsAnyHarnessPromptPatternMatchesCodex(t *testing.T) {
	if !containsAnyHarnessPromptPattern("│ >_ OpenAI Codex (v0.141.0) │\n│ /model to change │\n›") {
		t.Error("containsAnyHarnessPromptPattern should match the Codex composer header")
	}
}

// TestWaitForCodexPromptPolling verifies WaitForCodexPrompt detects the Codex
// composer signal once it appears in the pane.
func TestWaitForCodexPromptPolling(t *testing.T) {
	sessionName := "test-codex-prompt-polling"
	socketPath := newCodexTestSession(t, sessionName)

	// Print the Codex composer header into the pane.
	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "printf 'OpenAI Codex (v0.141.0)\\n/model to change\\n›\\n'; sleep 30", "Enter")
	if err := sendCmd.Run(); err != nil {
		t.Fatalf("Failed to send composer signal: %v", err)
	}

	if err := WaitForCodexPrompt(sessionName, 10*time.Second); err != nil {
		t.Errorf("WaitForCodexPrompt failed to detect composer: %v", err)
	}
}

func TestWaitForCodexPromptRejectsComposerAboveNewerOutput(t *testing.T) {
	sessionName := "test-codex-stale-prompt-tail"
	script := "printf 'OpenAI Codex (v0.142.0)\\n/model to change\\nCodex exited\\nuser@host:~/work$\\n'; sleep 30"
	newCodexTestSession(t, sessionName, "sh", "-c", script)

	err := WaitForCodexPrompt(sessionName, time.Second)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("WaitForCodexPrompt() error = %v, want timeout for stale composer", err)
	}
}

func TestGetPaneCommandsIncludesStartCommand(t *testing.T) {
	sessionName := "test-codex-pane-commands"
	newCodexTestSession(t, sessionName, "sleep 30")

	commands, err := GetPaneCommands(sessionName)
	if err != nil {
		t.Fatalf("GetPaneCommands returned error: %v", err)
	}

	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "sleep 30") {
		t.Fatalf("GetPaneCommands() = %q, want pane_start_command", joined)
	}
}

// TestIsCodexIdleComposerVisible verifies IsCodexIdle reports true once the
// Codex composer header is showing in the pane.
func TestIsCodexIdleComposerVisible(t *testing.T) {
	sessionName := "test-codex-idle-composer"
	socketPath := newCodexTestSession(t, sessionName)

	// Render the Codex composer header into the pane.
	sendCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "printf 'OpenAI Codex (v0.141.0)\\n/model to change\\n›\\n'; sleep 30", "Enter")
	if err := sendCmd.Run(); err != nil {
		t.Fatalf("Failed to send composer signal: %v", err)
	}

	// Poll briefly: the keystroke takes a moment to render in the pane.
	deadline := time.Now().Add(5 * time.Second)
	var idle bool
	for time.Now().Before(deadline) {
		var err error
		idle, err = IsCodexIdle(sessionName)
		if err != nil {
			t.Fatalf("IsCodexIdle returned error: %v", err)
		}
		if idle {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !idle {
		t.Error("IsCodexIdle = false, want true when composer is visible")
	}
}

// TestIsCodexIdleWorking verifies IsCodexIdle reports false when the pane shows
// no composer (i.e. Codex is working / no TUI signals present).
func TestIsCodexIdleWorking(t *testing.T) {
	sessionName := "test-codex-idle-working"
	newCodexTestSession(t, sessionName, "sh", "-c", "printf '• Working (3s • esc to interrupt)\\n  gpt-5.6 xhigh · ~/src/project\\n'; sleep 30")

	idle, err := IsCodexIdle(sessionName)
	if err != nil {
		t.Fatalf("IsCodexIdle returned error: %v", err)
	}
	if idle {
		t.Error("IsCodexIdle = true, want false when composer is not visible")
	}
}

func TestWaitForCodexPromptRejectsEchoedLaunchModel(t *testing.T) {
	sessionName := "test-codex-echoed-model"
	newCodexTestSession(t, sessionName, "sh", "-c", "printf \"user@host$ codex resume abc -m 'gpt-5.6'\\n\"; sleep 30")

	err := WaitForCodexPrompt(sessionName, time.Second)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("WaitForCodexPrompt() error = %v, want timeout without a composer", err)
	}
}

// TestIsCodexIdleNoSession verifies IsCodexIdle surfaces an error when the tmux
// session does not exist (so callers can distinguish "can't capture" from
// "working").
func TestIsCodexIdleNoSession(t *testing.T) {
	skipIfNoTmux(t)
	idle, err := IsCodexIdle("test-codex-idle-nonexistent")
	if err == nil {
		t.Error("IsCodexIdle on a missing session: expected error, got nil")
	}
	if idle {
		t.Error("IsCodexIdle on a missing session: expected false")
	}
}

// TestWaitForCodexPromptTimeout verifies WaitForCodexPrompt times out when no
// Codex composer ever appears.
func TestWaitForCodexPromptTimeout(t *testing.T) {
	sessionName := "test-codex-prompt-timeout"
	// Run a bare sleep (not an interactive shell) so a fancy prompt theme with
	// box-drawing characters can't be mistaken for the Codex composer.
	newCodexTestSession(t, sessionName, "sh", "-c", "sleep 30")

	start := time.Now()
	err := WaitForCodexPrompt(sessionName, 2*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
	if elapsed < 1800*time.Millisecond || elapsed > 3500*time.Millisecond {
		t.Errorf("Timeout took %v, expected ~2s", elapsed)
	}
}

// TestWaitForCodexPromptAutoAcceptsTrust verifies that when a first-run trust
// prompt is showing, WaitForCodexPrompt auto-accepts it (sending Enter) and
// then proceeds once the composer renders.
func TestWaitForCodexPromptAutoAcceptsTrust(t *testing.T) {
	sessionName := "test-codex-trust-accept"
	// Run a tiny shell loop that first prints the trust prompt, then — on
	// receiving any input line (the auto-accepted Enter) — prints the composer
	// header. This exercises both the trust detection and the post-accept
	// readiness path.
	script := "printf 'Do you trust the contents of this directory?\\n'; read _; printf 'OpenAI Codex (v0.141.0)\\n/model to change\\n›\\n'; sleep 30"
	newCodexTestSession(t, sessionName, "sh", "-c", script)

	if err := WaitForCodexPrompt(sessionName, 10*time.Second); err != nil {
		t.Errorf("WaitForCodexPrompt failed after trust auto-accept: %v", err)
	}
}

// TestWaitForCodexPromptSelectsExistingModel verifies that the Codex
// model-upgrade prompt does not block startup. AGM must choose the explicitly
// requested existing model instead of accepting the highlighted upgrade option.
func TestWaitForCodexPromptSelectsExistingModel(t *testing.T) {
	sessionName := "test-codex-model-upgrade"
	script := "printf \"Choose how you'd like Codex to proceed\\n› 1. Try new model\\n  2. Use existing model\\n\"; read _; printf 'OpenAI Codex (v0.141.0)\\n/model to change\\n›\\n'; sleep 30"
	newCodexTestSession(t, sessionName, "sh", "-c", script)

	if err := WaitForCodexPrompt(sessionName, 10*time.Second); err != nil {
		t.Errorf("WaitForCodexPrompt failed after model upgrade selection: %v", err)
	}
}

func TestWaitForCodexPromptFailsFastForHookReviewWithoutInput(t *testing.T) {
	sessionName := "test-codex-hook-review"
	inputPath := fmt.Sprintf("%s/input-byte", t.TempDir())
	script := fmt.Sprintf(`stty -echo -icanon min 1 time 0
printf "Hooks need review\n\n4 hooks are new or changed.\n\nHooks can run outside the sandbox after you trust them.\n\n› 1. Review hooks\n  2. Trust all and continue\n  3. Continue without trusting (hooks won't run)\n\nPress enter to confirm or esc to go back\n"
dd bs=1 count=1 of=%q 2>/dev/null
sleep 30`, inputPath)
	newCodexTestSession(t, sessionName, "sh", "-c", script)

	start := time.Now()
	err := WaitForCodexPrompt(sessionName, 10*time.Second)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrCodexHookReviewRequired) {
		t.Fatalf("WaitForCodexPrompt() error = %v, want ErrCodexHookReviewRequired", err)
	}
	if !strings.Contains(err.Error(), "AGM will not trust executable hooks automatically") {
		t.Fatalf("WaitForCodexPrompt() error = %q, want actionable no-auto-trust guidance", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("hook review failure took %v, want prompt failure", elapsed)
	}

	// The fake TUI records the first input byte. Its empty output file proves
	// the readiness wait did not press Enter, move the selector, or otherwise
	// answer this security decision.
	time.Sleep(100 * time.Millisecond)
	info, statErr := os.Stat(inputPath)
	if errors.Is(statErr, os.ErrNotExist) {
		return
	}
	if statErr != nil {
		t.Fatalf("stat input recorder: %v", statErr)
	}
	if info.Size() != 0 {
		t.Fatalf("hook review received %d input bytes, want none", info.Size())
	}
}
