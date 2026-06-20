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
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// startHarness dispatches per-harness initialization (Claude/Gemini/Codex/OpenCode).
// Returns (modeAppliedAtStartup, harnessHandledFullLifecycle, err): when
// harnessHandledFullLifecycle is true the caller should return immediately —
// the harness (e.g. gemini-cli wrapper) has already managed attach/detach.
func startHarness(sessionName, workDir string, exists bool, extraAddDirs []string, trustPreConfigured bool) (bool, bool, error) {
	switch harnessName {
	case "claude-code":
		return startClaudeHarness(sessionName, workDir, exists, extraAddDirs, trustPreConfigured)
	case "gemini-cli":
		done, err := startGeminiHarness(sessionName, exists)
		return false, done, err
	case "codex-cli":
		return false, false, startCodexHarness()
	case "opencode-cli":
		return false, false, startOpenCodeHarness(sessionName, exists)
	default:
		debug.Phase("Skip CLI Startup")
		debug.Log("Skipping CLI startup for harness: %s (no CLI configured)", harnessName)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s harness", sessionName))
		return false, false, nil
	}
}

// startClaudeHarness builds and sends the claude command, waits for the prompt,
// and answers the trust prompt if needed. Returns (modeAppliedAtStartup, false, err).
func startClaudeHarness(sessionName, workDir string, exists bool, extraAddDirs []string, trustPreConfigured bool) (bool, bool, error) {
	claudeReady := tmux.NewClaudeReadyFile(sessionName)
	if err := claudeReady.Cleanup(); err != nil {
		debug.Log("Warning: failed to cleanup ready-files: %v", err)
	}

	debug.Phase("Start Claude")
	claudeCmd, modeAppliedAtStartup := buildClaudeCommand(sessionName, workDir, extraAddDirs)
	debug.Log("Sending command: %s", claudeCmd)
	if err := tmux.SendCommand(sessionName, claudeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Claude in tmux session",
			"  • Verify Claude is installed: which claude\n"+
				"  • Test Claude manually: claude --version\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+sessionName)
		if !exists {
			_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
		}
		return false, false, err
	}
	debug.Log("Claude command sent successfully")
	ui.PrintSuccess("Started Claude CLI in tmux session")

	debug.Log("Initial sleep (500ms) before polling")
	time.Sleep(500 * time.Millisecond)

	if err := waitForClaudeReady(sessionName, claudeReady); err != nil {
		return modeAppliedAtStartup, false, err
	}

	if trustPreConfigured {
		debug.Phase("Skip Trust Prompt Monitoring")
		debug.Log("Skipping trust prompt monitoring since directory was pre-configured")
	} else {
		debug.Phase("Monitor for Trust Prompt")
		debug.Log("Starting control mode to monitor for trust prompt")
		if err := monitorAndAnswerTrustPrompt(sessionName, 10*time.Second); err != nil {
			debug.Log("Trust prompt handling: %v", err)
		}
	}

	debug.Phase("Skip Explicit SessionStart Hook Wait")
	debug.Log("SessionStart hooks confirmed complete (ready-file signal received)")
	return modeAppliedAtStartup, false, nil
}

// spawnSessionID holds the manifest UUID for the session being created. Set
// before harness startup so OTel env vars reference the correct session.
var spawnSessionID string

// otelEnvArgs returns additional env var assignments to inject into the
// spawned worker's shell command. Forwards OTEL_EXPORTER_OTLP_ENDPOINT from
// the orchestrator environment (so workers inherit tracing config), and sets
// ENGRAM_SESSION_ID to the session manifest UUID (so each worker's JSONL
// exporter writes to its own trace file).
func otelEnvArgs() string {
	var args strings.Builder
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		fmt.Fprintf(&args, " OTEL_EXPORTER_OTLP_ENDPOINT=%s", endpoint)
	}
	if spawnSessionID != "" {
		fmt.Fprintf(&args, " ENGRAM_SESSION_ID=%s", spawnSessionID)
	}
	return args.String()
}

// oauthEnvArg returns the CLAUDE_CODE_OAUTH_TOKEN env assignment to inject into
// a spawned worker session. It resolves the token via auth.ResolveOAuthToken,
// which prefers the live token from ~/.claude/.credentials.json over the
// CLAUDE_CODE_OAUTH_TOKEN env var. This propagates Max-plan OAuth auth from the
// orchestrator into spawned workers (ce-dzhz): the env var goes stale once
// Claude Code auto-refreshes the credentials file, so a worker that inherited
// the env token 401s on every turn — reading the file first keeps spawns fresh.
func oauthEnvArg() string {
	if token := auth.ResolveOAuthToken(); token != "" {
		// Single-quote the token (defense in depth; matches
		// ops.buildHarnessCommand) so an unexpected shell metacharacter can't
		// break the command line the token is concatenated into.
		escaped := strings.ReplaceAll(token, "'", `'\''`)
		return " CLAUDE_CODE_OAUTH_TOKEN='" + escaped + "'"
	}
	return ""
}

// claudeEnvUnsetFlags returns the `env -u` flag list for the spawned claude
// shell. CLAUDECODE is always unset; ANTHROPIC_API_KEY is additionally unset
// whenever an OAuth (Max-plan) token is being injected, so a stray metered key
// inherited from the tmux server's long-lived environment cannot shadow the
// OAuth token and silently route the session through the metered API instead of
// the Max plan (ce-84l2 — the "Claude API" symptom).
func claudeEnvUnsetFlags(haveOAuth bool) string {
	if haveOAuth {
		return "-u CLAUDECODE -u ANTHROPIC_API_KEY"
	}
	return "-u CLAUDECODE"
}

// buildClaudeCommand assembles the env+claude shell command line, applying
// flags for model, --add-dir, --permission-mode, and --max-budget-usd.
// Returns the command string and whether --permission-mode was applied.
func buildClaudeCommand(sessionName, workDir string, extraAddDirs []string) (string, bool) {
	resolvedModel := agent.ResolveModelFullName("claude-code", modelName)
	autoModeFlag := " --enable-auto-mode"
	if noAutoMode {
		autoModeFlag = ""
		debug.Log("Auto mode disabled by flag/env var")
	}
	oauthArg := oauthEnvArg()
	claudeCmd := fmt.Sprintf("env %s AGM_SESSION_NAME=%s%s%s claude --model %s --add-dir %s%s && exit", claudeEnvUnsetFlags(oauthArg != ""), shellQuote(sessionName), otelEnvArgs(), oauthArg, shellQuote(resolvedModel), shellQuote(workDir), autoModeFlag)
	for _, dir := range extraAddDirs {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --add-dir %s && exit", shellQuote(dir)), 1)
	}
	modeAppliedAtStartup := false
	if modeFlagValue == "auto" || modeFlagValue == "plan" || modeFlagValue == "default" {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --permission-mode %s && exit", modeFlagValue), 1)
		modeAppliedAtStartup = true
	}
	if maxBudgetUsd > 0 {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --max-budget-usd %.2f && exit", maxBudgetUsd), 1)
	}
	return claudeCmd, modeAppliedAtStartup
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
		socketPath := tmux.GetSocketPath()
		killCmd := exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName)
		if err := killCmd.Run(); err != nil {
			debug.Log("Failed to clean up session: %v", err)
		}
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
func startGeminiHarness(sessionName string, exists bool) (bool, error) {
	debug.Phase("Start Gemini")
	wrapperPath, err := exec.LookPath("agm-agent-wrapper")
	if err != nil {
		return false, startGeminiDirect(sessionName, exists)
	}
	debug.Log("Found agm-agent-wrapper at: %s", wrapperPath)
	debug.Log("Executing wrapper directly (not via tmux): %s --agent=gemini-cli %s", wrapperPath, sessionName)
	cmd := exec.Command(wrapperPath, "--agent=gemini-cli", sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		ui.PrintError(err,
			"Failed to run agm-agent-wrapper",
			"  • Check wrapper installed: which agm-agent-wrapper\n"+
				"  • Try direct mode by temporarily renaming wrapper\n"+
				"  • Attach and check: tmux attach -t "+sessionName)
		if !exists {
			_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
		}
		return false, err
	}
	ui.PrintSuccess("Gemini session ended")
	return true, nil
}

// startGeminiDirect runs gemini directly in the tmux session and handles the
// optional first-run trust prompt by sending "1<Enter>" if detected.
func startGeminiDirect(sessionName string, exists bool) error {
	debug.Log("agm-agent-wrapper not found, falling back to direct gemini")
	resolvedModel := agent.ResolveModelFullName("gemini-cli", modelName)
	geminiCmd := fmt.Sprintf("gemini -m %s && exit", shellQuote(resolvedModel))
	debug.Log("Sending command: %s", geminiCmd)
	if err := tmux.SendCommand(sessionName, geminiCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Gemini in tmux session",
			"  • Verify Gemini is installed: which gemini\n"+
				"  • Test Gemini manually: gemini --version\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+sessionName)
		if !exists {
			_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
		}
		return err
	}
	debug.Log("Gemini command sent successfully (direct mode)")
	ui.PrintSuccess("Started Gemini CLI in tmux session")

	debug.Log("Checking for Gemini trust prompt (3s window)...")
	time.Sleep(2 * time.Second)
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
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

// startCodexHarness verifies Codex credentials are configured.
func startCodexHarness() error {
	debug.Phase("Start Codex")
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
	ui.PrintSuccess("Codex adapter ready")
	return nil
}

// startOpenCodeHarness sends the `opencode attach` command into the tmux
// session and surfaces SSE-based readiness.
func startOpenCodeHarness(sessionName string, exists bool) error {
	debug.Phase("Start OpenCode")
	debug.Log("OpenCode server validated (health check passed)")
	opencodeCmd := "opencode attach && exit"
	debug.Log("Sending command: %s", opencodeCmd)
	if err := tmux.SendCommand(sessionName, opencodeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start OpenCode in tmux session",
			"  • Verify OpenCode server is running: curl http://localhost:4096/health\n"+
				"  • Start server if needed: opencode serve --port 4096\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+sessionName)
		if !exists {
			_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
		}
		return err
	}
	debug.Log("OpenCode attach command sent successfully")
	ui.PrintSuccess("Started OpenCode in tmux session")
	debug.Log("OpenCode session ready (SSE monitoring active)")
	ui.PrintSuccess("OpenCode is ready! (state tracked via SSE)")
	return nil
}
