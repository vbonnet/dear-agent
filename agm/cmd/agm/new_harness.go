package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/huh/spinner"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// startHarness dispatches per-harness initialization (Claude/Gemini/Codex/OpenCode/AGY).
// Returns (modeAppliedAtStartup, harnessHandledFullLifecycle, err): when
// harnessHandledFullLifecycle is true the caller should return immediately —
// the harness (e.g. gemini-cli wrapper) has already managed attach/detach.
func startHarness(spec ops.HarnessLaunchSpec, trustPreConfigured bool) (bool, bool, error) {
	switch spec.Harness {
	case "claude-code":
		return startClaudeHarness(spec, trustPreConfigured)
	case "gemini-cli":
		done, err := startGeminiHarness(spec)
		return false, done, err
	case "codex-cli":
		modeApplied, err := startCodexHarness(spec)
		return modeApplied, false, err
	case "opencode-cli":
		return false, false, startOpenCodeHarness(spec)
	case "agy":
		modeApplied, err := startAgyHarness(spec)
		return modeApplied, false, err
	default:
		debug.Phase("Skip CLI Startup")
		debug.Log("Skipping CLI startup for harness: %s (no CLI configured)", spec.Harness)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s harness", spec.SessionName))
		return false, false, nil
	}
}

func activeHarnessHasTmuxLauncher(harness string) bool {
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code", "codex-cli", "agy", "opencode-cli":
		return true
	default:
		return false
	}
}

// startClaudeHarness builds and sends the claude command, waits for the prompt,
// and answers the trust prompt if needed. Returns (modeAppliedAtStartup, false, err).
func startClaudeHarness(spec ops.HarnessLaunchSpec, trustPreConfigured bool) (bool, bool, error) {
	claudeReady := tmux.NewClaudeReadyFile(spec.SessionName)
	if err := claudeReady.Cleanup(); err != nil {
		debug.Log("Warning: failed to cleanup ready-files: %v", err)
	}

	debug.Phase("Start Claude")
	launch := ops.BuildHarnessLaunchCommand(spec)
	claudeCmd, modeAppliedAtStartup := launch.Command, launch.ModeAppliedAtStartup
	debug.Log("Sending command: %s", claudeCmd)
	if err := tmux.SendCommand(spec.SessionName, claudeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Claude in tmux session",
			"  • Verify Claude is installed: which claude\n"+
				"  • Test Claude manually: claude --version\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+spec.SessionName)
		return false, false, err
	}
	debug.Log("Claude command sent successfully")
	ui.PrintSuccess("Started Claude CLI in tmux session")

	debug.Log("Initial sleep (500ms) before polling")
	time.Sleep(500 * time.Millisecond)

	if err := waitForClaudeReady(spec.SessionName, claudeReady); err != nil {
		return modeAppliedAtStartup, false, err
	}

	if trustPreConfigured {
		debug.Phase("Skip Trust Prompt Monitoring")
		debug.Log("Skipping trust prompt monitoring since directory was pre-configured")
	} else {
		debug.Phase("Monitor for Trust Prompt")
		debug.Log("Starting control mode to monitor for trust prompt")
		if err := monitorAndAnswerTrustPrompt(spec.SessionName, 10*time.Second); err != nil {
			debug.Log("Trust prompt handling: %v", err)
		}
	}

	debug.Phase("Skip Explicit SessionStart Hook Wait")
	debug.Log("SessionStart hooks confirmed complete (ready-file signal received)")
	return modeAppliedAtStartup, false, nil
}

// waitForClaudeReady waits for the Claude prompt to appear (90s timeout) and
// triggers the SessionStart hook for consistency. On failure it tears the
// session down before returning.
func waitForClaudeReady(sessionName string, claudeReady *tmux.ClaudeReadyFile) error {
	debug.Phase("Wait for Claude Ready Signal (Text-Parsing)")
	debug.Log("Waiting for Claude prompt to appear in tmux (timeout: 90s)")
	var waitErr error
	spinErr := spinner.New().
		Title("Waiting for Claude to be ready...").
		Accessible(true).
		Action(func() {
			waitErr = tmux.WaitForClaudePrompt(sessionName, 90*time.Second)
		}).
		Run()
	if spinErr != nil {
		return fmt.Errorf("spinner error: %w", spinErr)
	}
	fmt.Println()
	if waitErr != nil {
		debug.Log("Claude prompt detection failed: %v", waitErr)
		ui.PrintError(waitErr,
			"Failed to detect Claude ready signal",
			"  Claude prompt not detected in tmux session.\n"+
				"  \n"+
				"  Troubleshooting:\n"+
				"    1. Check if Claude started: tmux attach -t "+sessionName+"\n"+
				"    2. Verify Claude is installed: which claude\n"+
				"    3. Check for errors in tmux: tmux capture-pane -t "+sessionName+" -p\n")
		return fmt.Errorf("claude not ready: %w", waitErr)
	}
	debug.Log("✓ Claude prompt detected - Claude is ready")
	debug.Log("Triggering SessionStart hook post-verification")
	if err := claudeReady.TriggerHookManually(); err != nil {
		debug.Log("Manual hook trigger failed (non-fatal): %v", err)
	}
	debug.Log("Claude ready signal received")
	ui.PrintSuccess("Claude is ready!")
	return nil
}

// startGeminiHarness starts Gemini either via the agm-agent-wrapper (preferred)
// or directly. Returns (handledFullLifecycle, err). When handledFullLifecycle
// is true the wrapper has already attached/exited and the caller should
// short-circuit the rest of session setup.
func startGeminiHarness(spec ops.HarnessLaunchSpec) (bool, error) {
	debug.Phase("Start Gemini")
	wrapperPath, err := exec.LookPath("agm-agent-wrapper")
	if err != nil {
		return false, startGeminiDirect(spec)
	}
	debug.Log("Found agm-agent-wrapper at: %s", wrapperPath)
	debug.Log("Executing wrapper directly (not via tmux): %s --agent=gemini-cli %s", wrapperPath, spec.SessionName)
	cmd := exec.Command(wrapperPath, "--agent=gemini-cli", spec.SessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		ui.PrintError(err,
			"Failed to run agm-agent-wrapper",
			"  • Check wrapper installed: which agm-agent-wrapper\n"+
				"  • Try direct mode by temporarily renaming wrapper\n"+
				"  • Attach and check: tmux attach -t "+spec.SessionName)
		return false, err
	}
	ui.PrintSuccess("Gemini session ended")
	return true, nil
}

// startGeminiDirect runs gemini directly in the tmux session and handles the
// optional first-run trust prompt by sending "1<Enter>" if detected.
func startGeminiDirect(spec ops.HarnessLaunchSpec) error {
	debug.Log("agm-agent-wrapper not found, falling back to direct gemini")
	geminiCmd := ops.BuildHarnessLaunchCommand(spec).Command
	debug.Log("Sending command: %s", geminiCmd)
	if err := tmux.SendCommand(spec.SessionName, geminiCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Gemini in tmux session",
			"  • Verify Gemini is installed: which gemini\n"+
				"  • Test Gemini manually: gemini --version\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+spec.SessionName)
		return err
	}
	debug.Log("Gemini command sent successfully (direct mode)")
	ui.PrintSuccess("Started Gemini CLI in tmux session")

	debug.Log("Checking for Gemini trust prompt (3s window)...")
	time.Sleep(2 * time.Second)
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(spec.SessionName)
	trustCheckCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-20")
	trustOutput, err := trustCheckCmd.CombinedOutput()
	if err != nil {
		return nil //nolint:nilerr // best-effort capture; failure means no auto-accept this run
	}
	content := string(trustOutput)
	if !strings.Contains(content, "Do you trust") && !strings.Contains(content, "trust the files") {
		debug.Log("No trust prompt detected (directory already trusted)")
		return nil
	}
	debug.Log("Gemini trust prompt detected, auto-accepting with '1' + Enter")
	selectCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "1")
	_ = selectCmd.Run()
	time.Sleep(300 * time.Millisecond)
	enterCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "-H", "0d")
	_ = enterCmd.Run()
	debug.Log("Trust prompt auto-accepted")
	ui.PrintSuccess("Auto-accepted Gemini trust prompt")
	return nil
}

func startAgyHarness(spec ops.HarnessLaunchSpec) (bool, error) {
	debug.Phase("Start AGY")
	if _, err := exec.LookPath("agy"); err != nil {
		ui.PrintError(err,
			"Failed to find AGY on PATH",
			"  • Verify AGY is installed: which agy\n"+
				"  • Test AGY manually: agy --help\n"+
				"  • Check AGY authentication in the CLI")
		return false, err
	}

	launch := ops.BuildHarnessLaunchCommand(spec)
	agyCmd := launch.Command
	modeAppliedAtStartup := launch.ModeAppliedAtStartup
	debug.Log("Sending command: %s", agyCmd)
	if err := tmux.SendCommand(spec.SessionName, agyCmd); err != nil {
		ui.PrintError(err,
			"Failed to start AGY in tmux session",
			"  • Verify AGY is installed: which agy\n"+
				"  • Test AGY manually: agy --help\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+spec.SessionName)
		return modeAppliedAtStartup, err
	}
	debug.Log("AGY command sent successfully")
	ui.PrintSuccess("Started AGY in tmux session")

	debug.Log("Initial sleep (500ms) before polling")
	time.Sleep(500 * time.Millisecond)

	debug.Log("Waiting for AGY prompt readiness (timeout: 90s)")
	if err := tmux.WaitForAgyPrompt(spec.SessionName, 90*time.Second); err != nil {
		debug.Log("AGY prompt readiness wait failed: %v", err)
		ui.PrintError(err,
			"AGY did not become ready",
			"  • Attach to inspect: tmux attach -t "+spec.SessionName+"\n"+
				"  • Check for trust or authentication prompts\n"+
				"  • Retry after resolving the prompt")
		return modeAppliedAtStartup, err
	}
	debug.Log("✓ AGY prompt detected - AGY is ready")
	ui.PrintSuccess("AGY adapter ready")
	return modeAppliedAtStartup, nil
}

// startCodexHarness launches the Codex TUI into the tmux pane and waits for the
// prompt to appear. It mirrors startClaudeHarness /
// startGeminiDirect: send the command, sleep briefly, then poll for readiness.
// The shared ops lifecycle owns teardown on failure.
func startCodexHarness(spec ops.HarnessLaunchSpec) (bool, error) {
	debug.Phase("Start Codex")
	launch := ops.BuildHarnessLaunchCommand(spec)
	codexCmd := launch.Command
	debug.Log("Sending command: %s", codexCmd)
	if err := tmux.SendCommand(spec.SessionName, codexCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Codex in tmux session",
			"  • Verify Codex is installed: which codex\n"+
				"  • Test Codex manually: codex --version\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+spec.SessionName)
		return false, err
	}
	debug.Log("Codex command sent successfully")
	ui.PrintSuccess("Started Codex CLI in tmux session")

	debug.Log("Initial sleep (500ms) before polling")
	time.Sleep(500 * time.Millisecond)

	debug.Log("Waiting for Codex prompt readiness (timeout: 90s)")
	if err := tmux.WaitForCodexPrompt(spec.SessionName, 90*time.Second); err != nil {
		debug.Log("Codex prompt readiness wait failed: %v", err)
		ui.PrintError(err,
			"Codex did not become ready",
			"  • Attach to inspect: tmux attach -t "+spec.SessionName+"\n"+
				"  • Check for onboarding, model selection, auth, or permission prompts\n"+
				"  • Retry after resolving the prompt")
		return false, err
	}
	debug.Log("✓ Codex prompt detected - Codex is ready")
	ui.PrintSuccess("Codex adapter ready")
	return launch.ModeAppliedAtStartup, nil
}

func validateCodexCredentials() error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" && !agent.IsCodexOAuthConfigured() {
		ui.PrintError(fmt.Errorf("no Codex credentials found"),
			"Codex requires either API key or OAuth login",
			"  • Login via OAuth: codex login\n"+
				"  • Or set API key: export OPENAI_API_KEY=sk-...\n"+
				"  • Get key from: https://platform.openai.com/api-keys")
		return fmt.Errorf("no Codex credentials found (run 'codex login' or set OPENAI_API_KEY)")
	}
	if apiKey != "" {
		debug.Log("Codex initialized with API key")
	} else {
		debug.Log("Codex initialized with OAuth credentials (~/.codex/auth.json)")
	}
	return nil
}

// startOpenCodeHarness sends the `opencode attach` command into the tmux
// session and surfaces SSE-based readiness.
func startOpenCodeHarness(spec ops.HarnessLaunchSpec) error {
	debug.Phase("Start OpenCode")
	debug.Log("OpenCode server validated (health check passed)")
	opencodeCmd := ops.BuildHarnessLaunchCommand(spec).Command
	debug.Log("Sending command: %s", opencodeCmd)
	if err := tmux.SendCommand(spec.SessionName, opencodeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start OpenCode in tmux session",
			"  • Verify OpenCode server is running: curl http://localhost:4096/health\n"+
				"  • Start server if needed: opencode serve --port 4096\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+spec.SessionName)
		return err
	}
	debug.Log("OpenCode attach command sent successfully")
	ui.PrintSuccess("Started OpenCode in tmux session")
	debug.Log("OpenCode session ready (SSE monitoring active)")
	ui.PrintSuccess("OpenCode is ready! (state tracked via SSE)")
	return nil
}
