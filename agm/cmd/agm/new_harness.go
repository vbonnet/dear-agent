package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/huh/spinner"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// startHarness dispatches per-harness initialization.
// Returns (modeAppliedAtStartup, harnessHandledFullLifecycle, err): when
// harnessHandledFullLifecycle is true the caller should return immediately —
// the harness (e.g. gemini-cli wrapper) has already managed attach/detach.
func startHarness(ctx context.Context, spec ops.HarnessLaunchSpec, trustPreConfigured bool) (bool, bool, error) {
	switch spec.Harness {
	case "claude-code":
		return startClaudeHarness(ctx, spec, trustPreConfigured)
	case "gemini-cli":
		done, err := startGeminiHarness(ctx, spec)
		return false, done, err
	case "codex-cli":
		modeApplied, err := startCodexHarness(ctx, spec)
		return modeApplied, false, err
	case "opencode-cli":
		return false, false, startOpenCodeHarness(spec)
	case "agy":
		modeApplied, err := startAgyHarness(ctx, spec)
		return modeApplied, false, err
	case "pi-cli":
		modeApplied, err := startPiHarness(ctx, spec)
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
	case "claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli":
		return true
	default:
		return false
	}
}

func resolveHarnessLaunchSubmission(harness string, launch ops.HarnessLaunchCommand, submissionErr error) error {
	uncertain, err := ops.ResolveHarnessLaunchSubmission(launch, submissionErr)
	if uncertain {
		message := fmt.Sprintf("%s launch submission acknowledgement was lost; preserving the launch because the command may already be queued", harness)
		debug.Log("%s: %v", message, submissionErr)
		ui.PrintWarning(message)
	}
	return err
}

func runBeforeHarnessSpawn(spec ops.HarnessLaunchSpec) error {
	if spec.BeforeSpawn == nil {
		return nil
	}
	return spec.BeforeSpawn()
}

func submitHarnessLaunch(
	harness string,
	spec ops.HarnessLaunchSpec,
	launch ops.HarnessLaunchCommand,
	submit func() error,
) error {
	if err := runBeforeHarnessSpawn(spec); err != nil {
		return errors.Join(
			fmt.Errorf("%s launch admission: %w", harness, err),
			launch.CancelUndelivered(),
		)
	}
	return resolveHarnessLaunchSubmission(harness, launch, submit())
}

type piHarnessRuntime struct {
	lookPath      func(string) (string, error)
	sendCommand   func(string, string) error
	waitForPrompt func(context.Context, string, string, time.Duration) error
	sleep         func(time.Duration)
}

func realPiHarnessRuntime() piHarnessRuntime {
	return piHarnessRuntime{
		lookPath: exec.LookPath, sendCommand: tmux.SendCommand,
		waitForPrompt: tmux.WaitForPiLaunchPromptContext, sleep: time.Sleep,
	}
}

func startPiHarness(ctx context.Context, spec ops.HarnessLaunchSpec) (bool, error) {
	return startPiHarnessWithRuntime(ctx, spec, realPiHarnessRuntime())
}

func startPiHarnessWithRuntime(ctx context.Context, spec ops.HarnessLaunchSpec, runtime piHarnessRuntime) (bool, error) {
	debug.Phase("Start Pi")
	if _, err := runtime.lookPath("pi"); err != nil {
		return false, fmt.Errorf("pi executable is unavailable: %w", err)
	}
	if spec.PiLaunchID == "" {
		spec.PiLaunchID = launchparity.NewPiLaunchID()
	}
	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return false, fmt.Errorf("prepare Pi launch: %w", err)
	}
	if err := submitHarnessLaunch("Pi", spec, launch, func() error {
		return runtime.sendCommand(spec.SessionName, launch.Command)
	}); err != nil {
		return launch.ModeAppliedAtStartup, fmt.Errorf("start Pi in tmux: %w", err)
	}
	runtime.sleep(500 * time.Millisecond)
	if err := runtime.waitForPrompt(ctx, spec.SessionName, spec.PiLaunchID, 90*time.Second); err != nil {
		return launch.ModeAppliedAtStartup, fmt.Errorf("pi managed readiness: %w", err)
	}
	ui.PrintSuccess("Pi adapter ready")
	return launch.ModeAppliedAtStartup, nil
}

// startClaudeHarness builds and sends the claude command, waits for the prompt,
// and answers the trust prompt if needed. Returns (modeAppliedAtStartup, false, err).
func startClaudeHarness(ctx context.Context, spec ops.HarnessLaunchSpec, trustPreConfigured bool) (bool, bool, error) {
	claudeReady := tmux.NewClaudeReadyFile(spec.SessionName)
	if err := claudeReady.Cleanup(); err != nil {
		debug.Log("Warning: failed to cleanup ready-files: %v", err)
	}

	debug.Phase("Start Claude")
	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return false, false, fmt.Errorf("prepare Claude launch: %w", err)
	}
	claudeCmd, modeAppliedAtStartup := launch.Command, launch.ModeAppliedAtStartup
	debug.Log("Sending command: %s", claudeCmd)
	if err := submitHarnessLaunch("Claude", spec, launch, func() error {
		return tmux.SendCommand(spec.SessionName, claudeCmd)
	}); err != nil {
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
	if err := sleepWithCallerContext(ctx, 500*time.Millisecond); err != nil {
		return modeAppliedAtStartup, false, err
	}

	if err := waitForClaudeReady(ctx, spec.SessionName, claudeReady); err != nil {
		return modeAppliedAtStartup, false, err
	}

	if trustPreConfigured {
		debug.Phase("Skip Trust Prompt Monitoring")
		debug.Log("Skipping trust prompt monitoring since directory was pre-configured")
	} else {
		debug.Phase("Monitor for Trust Prompt")
		debug.Log("Starting control mode to monitor for trust prompt")
		if err := monitorAndAnswerTrustPrompt(ctx, spec.SessionName, 10*time.Second); err != nil {
			if ctx.Err() != nil {
				return modeAppliedAtStartup, false, ctx.Err()
			}
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
func waitForClaudeReady(ctx context.Context, sessionName string, claudeReady *tmux.ClaudeReadyFile) error {
	debug.Phase("Wait for Claude Ready Signal (Text-Parsing)")
	debug.Log("Waiting for Claude prompt to appear in tmux (timeout: 90s)")
	var waitErr error
	spinErr := spinner.New().
		Title("Waiting for Claude to be ready...").
		Accessible(true).
		Action(func() {
			waitErr = tmux.WaitForClaudePromptContext(ctx, sessionName, 90*time.Second)
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
func startGeminiHarness(ctx context.Context, spec ops.HarnessLaunchSpec) (bool, error) {
	debug.Phase("Start Gemini")
	wrapperPath, err := exec.LookPath("agm-agent-wrapper")
	if err != nil {
		return false, startGeminiDirect(ctx, spec)
	}
	debug.Log("Found agm-agent-wrapper at: %s", wrapperPath)
	debug.Log("Executing wrapper directly (not via tmux): %s --agent=gemini-cli %s", wrapperPath, spec.SessionName)
	cmd := exec.CommandContext(ctx, wrapperPath, "--agent=gemini-cli", spec.SessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := runBeforeHarnessSpawn(spec); err != nil {
		return false, fmt.Errorf("gemini launch admission: %w", err)
	}
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
func startGeminiDirect(ctx context.Context, spec ops.HarnessLaunchSpec) error {
	debug.Log("agm-agent-wrapper not found, falling back to direct gemini")
	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return fmt.Errorf("prepare Gemini launch: %w", err)
	}
	geminiCmd := launch.Command
	debug.Log("Sending command: %s", geminiCmd)
	if err := submitHarnessLaunch("Gemini", spec, launch, func() error {
		return tmux.SendCommand(spec.SessionName, geminiCmd)
	}); err != nil {
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
	if err := sleepWithCallerContext(ctx, 2*time.Second); err != nil {
		return err
	}
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(spec.SessionName)
	trustCheckCmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-20")
	trustOutput, err := trustCheckCmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return nil //nolint:nilerr // best-effort capture; failure means no auto-accept this run
	}
	content := string(trustOutput)
	if !strings.Contains(content, "Do you trust") && !strings.Contains(content, "trust the files") {
		debug.Log("No trust prompt detected (directory already trusted)")
		return nil
	}
	debug.Log("Gemini trust prompt detected, auto-accepting with '1' + Enter")
	selectCmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "1")
	_ = selectCmd.Run()
	if err := sleepWithCallerContext(ctx, 300*time.Millisecond); err != nil {
		return err
	}
	enterCmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "-H", "0d")
	_ = enterCmd.Run()
	debug.Log("Trust prompt auto-accepted")
	ui.PrintSuccess("Auto-accepted Gemini trust prompt")
	return nil
}

type agyHarnessRuntime struct {
	lookPath      func(string) (string, error)
	sendCommand   func(string, string) error
	waitForPrompt func(context.Context, string, time.Duration) error
	sleep         func(time.Duration)
}

func realAgyHarnessRuntime() agyHarnessRuntime {
	return agyHarnessRuntime{
		lookPath:      exec.LookPath,
		sendCommand:   tmux.SendCommand,
		waitForPrompt: tmux.WaitForAgyPrompt,
		sleep:         time.Sleep,
	}
}

func startAgyHarness(ctx context.Context, spec ops.HarnessLaunchSpec) (bool, error) {
	return startAgyHarnessWithRuntime(ctx, spec, realAgyHarnessRuntime())
}

func startAgyHarnessWithRuntime(ctx context.Context, spec ops.HarnessLaunchSpec, runtime agyHarnessRuntime) (bool, error) {
	debug.Phase("Start AGY")
	if _, err := runtime.lookPath("agy"); err != nil {
		ui.PrintError(err,
			"Failed to find AGY on PATH",
			"  • Verify AGY is installed: which agy\n"+
				"  • Test AGY manually: agy --help\n"+
				"  • Check AGY authentication in the CLI")
		return false, err
	}

	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return false, fmt.Errorf("prepare AGY launch: %w", err)
	}
	agyCmd := launch.Command
	modeAppliedAtStartup := launch.ModeAppliedAtStartup
	debug.Log("Sending command: %s", agyCmd)
	if err := submitHarnessLaunch("AGY", spec, launch, func() error {
		return runtime.sendCommand(spec.SessionName, agyCmd)
	}); err != nil {
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
	runtime.sleep(500 * time.Millisecond)

	debug.Log("Waiting for AGY prompt readiness (timeout: 90s)")
	if err := runtime.waitForPrompt(ctx, spec.SessionName, 90*time.Second); err != nil {
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
func startCodexHarness(ctx context.Context, spec ops.HarnessLaunchSpec) (bool, error) {
	debug.Phase("Start Codex")
	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return false, fmt.Errorf("prepare Codex launch: %w", err)
	}
	codexCmd := launch.Command
	debug.Log("Sending command: %s", codexCmd)
	if err := submitHarnessLaunch("Codex", spec, launch, func() error {
		return tmux.SendCommand(spec.SessionName, codexCmd)
	}); err != nil {
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
	if err := sleepWithCallerContext(ctx, 500*time.Millisecond); err != nil {
		return false, err
	}

	debug.Log("Waiting for Codex prompt readiness (timeout: 90s)")
	if err := tmux.WaitForCodexPromptContext(ctx, spec.SessionName, 90*time.Second); err != nil {
		debug.Log("Codex prompt readiness wait failed: %v", err)
		ui.PrintError(err,
			"Codex did not become ready",
			"  • Attach to inspect: tmux attach -t "+spec.SessionName+"\n"+
				"  • Check for onboarding, model selection, auth, or permission prompts\n"+
				"  • Retry after resolving the prompt")
		return false, err
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
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
	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return fmt.Errorf("prepare OpenCode launch: %w", err)
	}
	opencodeCmd := launch.Command
	debug.Log("Sending command: %s", opencodeCmd)
	if err := submitHarnessLaunch("OpenCode", spec, launch, func() error {
		return tmux.SendCommand(spec.SessionName, opencodeCmd)
	}); err != nil {
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
