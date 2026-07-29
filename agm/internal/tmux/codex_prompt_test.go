package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	socketDir, err := newShortCodexTestSocketDir()
	if err != nil {
		t.Fatalf("create private tmux socket directory: %v", err)
	}
	socketPath := filepath.Join(socketDir, "agm.sock")
	t.Setenv("AGM_TMUX_SOCKET", socketPath)
	workDir := t.TempDir()

	args := append([]string{"-S", socketPath, "new-session", "-d", "-s", sessionName, "-c", workDir}, cmd...)
	if output, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		os.RemoveAll(socketDir)
		t.Fatalf("Failed to create test session: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		exec.Command("tmux", "-S", socketPath, "kill-server").Run()
		os.RemoveAll(socketDir)
	})
	return socketPath
}

// Keep the socket under the system temporary directory: macOS limits Unix
// socket paths, while testing.T.TempDir can include a long test-name prefix.
func newShortCodexTestSocketDir() (string, error) {
	return os.MkdirTemp("", "agm-codex-test")
}

func currentCodexWelcomeGhostScript() string {
	return "printf '\\033[2m│ >_ \\033[0;1mOpenAI Codex\\033[0;2m (v0.145.0) │\\033[0m\\n" +
		"\\033[2m│ model: \\033[0mgpt-5.6 high\\033[2m \\033[0m/model to change │\\n" +
		"To get started, describe a task or try /review\\n\\n" +
		"\\033[1m›\\033[0m \\033[2mRun /review on my current changes\\033[0m\\n\\n" +
		"gpt-5.6 high · ~/src/project\\n'; sleep 30"
}

func resizeCodexTestWindow(t *testing.T, socketPath, sessionName string) {
	t.Helper()
	if output, err := exec.Command("tmux", "-S", socketPath, "resize-window", "-t", sessionName, "-x", "28", "-y", "20").CombinedOutput(); err != nil {
		t.Fatalf("resize Codex fixture to narrow pane: %v\n%s", err, output)
	}
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
			name: "Codex 0.145 welcome composer with styled suggestion",
			content: "\x1b[2m╭────────────────────────────────────────────╮\x1b[0m\n" +
				"\x1b[2m│ >_ \x1b[0;1mOpenAI Codex\x1b[0;2m (v0.145.0)                 │\x1b[0m\n" +
				"\x1b[2m│ model:     \x1b[0mgpt-5.5 high\x1b[2m   \x1b[0m/model to change │\n" +
				"\x1b[2m╰────────────────────────────────────────────╯\x1b[0m\n" +
				"  To get started, describe a task or try /review\n\n" +
				"\x1b[1m›\x1b[0m \x1b[2mRun /review on my current changes\x1b[0m\n\n" +
				"  gpt-5.5 high · ~/.agm/sandboxes/example/merged/repo0",
			expected: true,
		},
		{
			name: "Codex 0.145 welcome composer with human draft",
			content: "\x1b[2m│ >_ \x1b[0;1mOpenAI Codex\x1b[0;2m (v0.145.0) │\x1b[0m\n" +
				"\x1b[2m│ model: \x1b[0mgpt-5.5 high\x1b[2m \x1b[0m/model to change │\n" +
				"\x1b[1m›\x1b[0m Run /review on my current changes\n\n" +
				"  gpt-5.5 high · ~/src/project",
			expected: false,
		},
		{
			name: "human draft with later dim token is not ready",
			content: "\x1b[2m│ >_ \x1b[0;1mOpenAI Codex\x1b[0;2m (v0.145.0) │\x1b[0m\n" +
				"\x1b[2m│ model: \x1b[0mgpt-5.5 high\x1b[2m \x1b[0m/model to change │\n" +
				"\x1b[1m›\x1b[0m Review \x1b[2mthis\x1b[0m change\n\n" +
				"  gpt-5.5 high · ~/src/project",
			expected: false,
		},
		{
			name: "styled initial paste chip is not ready",
			content: "\x1b[2m│ >_ \x1b[0;1mOpenAI Codex\x1b[0;2m (v0.145.0) │\x1b[0m\n" +
				"\x1b[2m│ model: \x1b[0mgpt-5.5 high\x1b[2m \x1b[0m/model to change │\n" +
				"\x1b[1m›\x1b[0m \x1b[2m[Pasted Content 2172 chars]\x1b[0m\n\n" +
				"  gpt-5.5 high · ~/src/project",
			expected: false,
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
			name:     "active trust selector numbered choice is not a composer",
			content:  "Do you trust the contents of this directory?\n› 1. Yes, continue\n  2. No, quit",
			expected: true,
		},
		{
			name:     "ready composer is not a trust prompt",
			content:  "│ >_ OpenAI Codex (v0.141.0)            │",
			expected: false,
		},
		{
			name: "stale trust prompt above current composer",
			content: "Do you trust the contents of this directory?\n" +
				"› 1. Yes, continue\n  2. No, quit\n\nPress enter to continue\n" +
				"OpenAI Codex (v0.141.0)\n/model to change\n›",
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
			name: "stale model selector above current composer",
			content: "Choose how you'd like Codex to proceed.\n\n› 1. Try new model\n  2. Use existing model\n" +
				"OpenAI Codex (v0.141.0)\n/model to change\n›",
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
	const activeSelector = `Hooks need review

4 hooks are new or changed.

Hooks can run outside the sandbox after you trust them.

› 1. Review hooks
  2. Trust all and continue
  3. Continue without trusting (hooks won't run)

Press enter to confirm or esc to go back`
	const hookDashboard = `Hooks
Lifecycle hooks from config and enabled plugins.

⚠ 11 hooks need review before they can run.

Event                 Installed   Active      Review      Description
PreToolUse            5           0           5           Before a tool exec
SessionStart          1           0           1           When a new session

Press t to trust all; enter to review hooks; esc to close`
	styledHookDashboardControls := strings.Replace(
		hookDashboard,
		"Press t to trust all",
		"Press \x1b[1mt\x1b[0m to trust all",
		1,
	)
	const composer = `│ >_ OpenAI Codex (v0.145.0) │
│ model: gpt-5.6 high /model to change │
╰──────────────────────────────╯
›

gpt-5.6 high · ~/src/project`
	const styledGhostComposer = "\x1b[2m│ >_ \x1b[0;1mOpenAI Codex\x1b[0;2m (v0.145.0) │\x1b[0m\n" +
		"\x1b[2m│ model: \x1b[0mgpt-5.6 high\x1b[2m \x1b[0m/model to change │\n" +
		"To get started, describe a task or try /review\n\n" +
		"\x1b[1m›\x1b[0m \x1b[2mRun /review on my current changes\x1b[0m\n\n" +
		"gpt-5.6 high · ~/src/project"

	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Codex 0.145 structured hook review",
			content:  activeSelector,
			expected: true,
		},
		{
			name:     "numbered selector above blank terminal rows",
			content:  activeSelector + strings.Repeat("\n", 18),
			expected: true,
		},
		{
			name:     "active dashboard redraw below retained composer",
			content:  hookDashboard + "\n" + composer + "\n" + hookDashboard,
			expected: true,
		},
		{
			name:     "styled dashboard control remains review-required",
			content:  styledHookDashboardControls,
			expected: true,
		},
		{
			name:     "weak transcript text is not an active selector",
			content:  "The release notes say hooks need review before use.",
			expected: false,
		},
		{
			name:     "incomplete dashboard prose is not active",
			content:  "Hooks\nLifecycle hooks from config and enabled plugins.\n11 hooks need review before they can run.",
			expected: false,
		},
		{
			name: "newer composer supersedes retained selector",
			content: activeSelector + `
review completed
│ >_ OpenAI Codex (v0.145.0) │
│ model: gpt-5.6 high /model to change │
╰──────────────────────────────╯
›`,
			expected: false,
		},
		{
			name:     "newer composer supersedes retained dashboard",
			content:  hookDashboard + "\n" + composer,
			expected: false,
		},
		{
			name:     "newer styled ghost composer supersedes retained dashboard",
			content:  hookDashboard + "\n" + styledGhostComposer,
			expected: false,
		},
		{
			name: "newer occupied composer supersedes retained dashboard",
			content: hookDashboard + `

› preserve this human draft

gpt-5.6 high · ~/src/project`,
			expected: false,
		},
		{
			name: "newer working turn supersedes retained dashboard",
			content: hookDashboard + `

› inspect the release

• Working (3s • esc to interrupt)

gpt-5.6 high · ~/src/project`,
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
	styledGhost := "\x1b[2m│ >_ \x1b[0;1mOpenAI Codex\x1b[0;2m (v0.145.0) │\x1b[0m\n" +
		"\x1b[2m│ model: \x1b[0mgpt-5.6 high\x1b[2m \x1b[0m/model to change │\n" +
		"\x1b[1m›\x1b[0m \x1b[2mRun /review on my current changes\x1b[0m\n\n" +
		"gpt-5.6 high · ~/src/project"
	if !containsAnyHarnessPromptPattern(styledGhost) {
		t.Error("containsAnyHarnessPromptPattern should preserve the styled Codex welcome suggestion")
	}
	if containsAnyHarnessPromptPattern(stripANSI(styledGhost)) {
		t.Error("containsAnyHarnessPromptPattern should treat identical unstyled Codex text as a human draft")
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

// TestIsCodexIdlePreservesCurrentWelcomeGhostStyle verifies the live capture
// keeps the style that distinguishes Codex's suggestion from a human draft.
func TestIsCodexIdlePreservesCurrentWelcomeGhostStyle(t *testing.T) {
	sessionName := "test-codex-idle-composer"
	socketPath := newCodexTestSession(t, sessionName, "sh", "-c", currentCodexWelcomeGhostScript())
	resizeCodexTestWindow(t, socketPath, sessionName)

	// Poll briefly: the process takes a moment to render in the pane.
	deadline := time.Now().Add(5 * time.Second)
	var idle bool
	for time.Now().Before(deadline) {
		raw, captureErr := CapturePaneANSIOutput(sessionName, 30)
		if captureErr == nil && strings.Contains(stripANSI(raw), "OpenAI Codex") && IsCodexComposerReady(raw) {
			t.Fatal("physical-row capture unexpectedly classified a wrapped Codex composer as ready")
		}
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

func TestWaitForPromptSimplePreservesCurrentCodexWelcomeGhostStyle(t *testing.T) {
	sessionName := "test-codex-simple-ghost"
	socketPath := newCodexTestSession(t, sessionName, "sh", "-c", currentCodexWelcomeGhostScript())
	resizeCodexTestWindow(t, socketPath, sessionName)

	if err := WaitForPromptSimpleContext(t.Context(), sessionName, 3*time.Second); err != nil {
		t.Fatalf("WaitForPromptSimpleContext() error = %v, want styled ghost composer readiness", err)
	}
}

func TestWaitForPromptOrResumeFailurePreservesCurrentCodexWelcomeGhostStyle(t *testing.T) {
	sessionName := "test-codex-resume-ghost"
	socketPath := newCodexTestSession(t, sessionName, "sh", "-c", currentCodexWelcomeGhostScript())
	resizeCodexTestWindow(t, socketPath, sessionName)

	if err := WaitForPromptOrResumeFailureContext(t.Context(), sessionName, 3*time.Second); err != nil {
		t.Fatalf("WaitForPromptOrResumeFailureContext() error = %v, want styled ghost composer readiness", err)
	}
}

func TestSendMultiLinePromptSafePreservesCurrentCodexWelcomeGhostStyle(t *testing.T) {
	sessionName := "test-codex-send-ghost"
	const marker = "AGM-CODEX-GHOST-DELIVERY"
	script := strings.TrimSuffix(currentCodexWelcomeGhostScript(), "sleep 30") +
		"IFS= read -r line; printf '\\nreceived:%s\\n' \"$line\"; sleep 30"
	socketPath := newCodexTestSession(t, sessionName, "sh", "-c", script)
	resizeCodexTestWindow(t, socketPath, sessionName)

	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	if err := SendMultiLinePromptSafeContext(ctx, sessionName, marker, false); err != nil {
		t.Fatalf("SendMultiLinePromptSafeContext() error = %v, want styled composer delivery", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		content, err := CapturePaneLogicalANSIOutput(sessionName, 30)
		if err == nil && strings.Contains(stripANSI(content), "received:"+marker) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("delivered marker did not reach fixture; capture error = %v; pane:\n%s", err, content)
		}
		time.Sleep(50 * time.Millisecond)
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

func TestWaitForCodexPromptAcceptsCurrentWelcomeGhostComposer(t *testing.T) {
	sessionName := "test-codex-current-welcome"
	script := `printf '\033[2m│ >_ \033[0;1mOpenAI Codex\033[0;2m (v0.145.0) │\033[0m\n'
printf '\033[2m│ model: \033[0mgpt-5.5 high\033[2m \033[0m/model to change │\n'
printf 'To get started, describe a task or try /review\n\n'
printf '\033[1m›\033[0m \033[2mRun /review on my current changes\033[0m\n\n'
printf 'gpt-5.5 high · ~/.agm/sandboxes/example/merged/repo0\n'
sleep 30`
	socketPath := newCodexTestSession(t, sessionName, "sh", "-c", script)
	resizeCodexTestWindow(t, socketPath, sessionName)

	if err := WaitForCodexPrompt(sessionName, 10*time.Second); err != nil {
		t.Errorf("WaitForCodexPrompt failed to detect current styled welcome composer: %v", err)
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

func TestWaitForCodexPromptFailsFastForHookDashboardWithoutInput(t *testing.T) {
	sessionName := "test-codex-hook-dashboard"
	inputPath := fmt.Sprintf("%s/input-byte", t.TempDir())
	script := styledCodexHookDashboardScript(inputPath)
	newCodexTestSession(t, sessionName, "sh", "-c", script)

	start := time.Now()
	err := WaitForCodexPrompt(sessionName, 10*time.Second)
	elapsed := time.Since(start)
	if !errors.Is(err, ErrCodexHookReviewRequired) {
		t.Fatalf("WaitForCodexPrompt() error = %v, want ErrCodexHookReviewRequired", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("hook dashboard failure took %v, want prompt failure", elapsed)
	}
	assertCodexFixtureReceivedNoInput(t, inputPath)
}

func TestGenericPromptWaitsFailFastForStyledCodexHookDashboard(t *testing.T) {
	tests := []struct {
		name string
		wait func(context.Context, string, time.Duration) error
	}{
		{
			name: "simple",
			wait: func(ctx context.Context, sessionName string, timeout time.Duration) error {
				return WaitForPromptSimpleForHarnessContext(ctx, sessionName, timeout, "codex-cli")
			},
		},
		{
			name: "resume-aware",
			wait: func(ctx context.Context, sessionName string, timeout time.Duration) error {
				return WaitForPromptOrResumeFailureForHarnessContext(ctx, sessionName, timeout, "codex-cli")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionName := "test-codex-generic-hook-" + tt.name
			inputPath := fmt.Sprintf("%s/input-byte", t.TempDir())
			newCodexTestSession(t, sessionName, "sh", "-c", styledCodexHookDashboardScript(inputPath))

			start := time.Now()
			err := tt.wait(t.Context(), sessionName, 10*time.Second)
			if !errors.Is(err, ErrCodexHookReviewRequired) {
				t.Fatalf("generic wait error = %v, want ErrCodexHookReviewRequired", err)
			}
			if elapsed := time.Since(start); elapsed > 3*time.Second {
				t.Fatalf("hook dashboard failure took %v, want prompt failure", elapsed)
			}
			assertCodexFixtureReceivedNoInput(t, inputPath)
		})
	}
}

func TestGenericPromptWaitsIgnoreCopiedCodexHookDashboardForOtherHarnesses(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		prompt  string
		wait    func(context.Context, string, time.Duration, string) error
	}{
		{
			name:    "simple Claude",
			harness: "claude-code",
			prompt:  "❯",
			wait:    WaitForPromptSimpleForHarnessContext,
		},
		{
			name:    "simple Gemini",
			harness: "gemini-cli",
			prompt:  ">   Type your message",
			wait:    WaitForPromptSimpleForHarnessContext,
		},
		{
			name:    "resume-aware Claude",
			harness: "claude-code",
			prompt:  "❯",
			wait:    WaitForPromptOrResumeFailureForHarnessContext,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionName := "test-copied-hook-" + strings.ReplaceAll(tt.name, " ", "-")
			script := fmt.Sprintf("printf 'Hooks\\nLifecycle hooks from config and enabled plugins.\\n⚠ 11 hooks need review before they can run.\\nPress t to trust all; enter to review hooks; esc to close\\n%s\\n'; sleep 30", tt.prompt)
			newCodexTestSession(t, sessionName, "sh", "-c", script)

			if err := tt.wait(t.Context(), sessionName, 3*time.Second, tt.harness); err != nil {
				t.Fatalf("generic %s wait rejected copied Codex dashboard: %v", tt.harness, err)
			}
		})
	}
}

func styledCodexHookDashboardScript(inputPath string) string {
	return fmt.Sprintf(`stty -echo -icanon min 1 time 0
printf "OpenAI Codex (v0.145.0)\n/model to change\n›\ngpt-5.6 high · ~/src/project\n"
printf "Hooks\nLifecycle hooks from config and enabled plugins.\n\n⚠ 11 hooks need review before they can run.\n\nEvent Installed Active Review Description\nPreToolUse 5 0 5 Before a tool exec\n\nPress \033[1mt\033[0m to trust all; enter to review hooks; esc to close\n"
dd bs=1 count=1 of=%q 2>/dev/null
sleep 30`, inputPath)
}

func assertCodexFixtureReceivedNoInput(t *testing.T, inputPath string) {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	info, statErr := os.Stat(inputPath)
	if errors.Is(statErr, os.ErrNotExist) {
		return
	}
	if statErr != nil {
		t.Fatalf("stat input recorder: %v", statErr)
	}
	if info.Size() != 0 {
		t.Fatalf("hook dashboard received %d input bytes, want none", info.Size())
	}
}
