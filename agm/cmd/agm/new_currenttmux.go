package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// startClaudeInCurrentTmux starts a fresh Claude session in the current tmux session
func startClaudeInCurrentTmux(sessionName string) error {
	if !testMode {
		if dupErr := checkDuplicateSessionName(sessionName); dupErr != nil {
			return dupErr
		}
		if err := enforceCircuitBreakers(); err != nil {
			return err
		}
	}

	fmt.Printf("Starting new Claude session in current tmux: %s\n", sessionName)

	workDir, err := os.Getwd()
	if err != nil {
		ui.PrintError(err,
			"Failed to get current directory",
			"  • Check directory still exists: pwd\n"+
				"  • Verify directory permissions: ls -ld .\n"+
				"  • Try from a different directory")
		return err
	}

	sessionID := uuid.New().String()
	manifestDir := filepath.Join(getSessionsDir(), sessionName)
	runtime := &cliCreateSessionRuntime{
		launch: func(_ context.Context, spec ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
			if pwd := os.Getenv("PWD"); pwd != "" {
				spec.WorkDir = pwd
			}
			return ops.CreateSessionLaunchResult{}, startCurrentTmuxHarness(spec)
		},
		complete: func(_ context.Context, completion ops.CreateSessionCompletion) error {
			if completion.ManifestPath != "" {
				commitCurrentTmuxManifest(completion.ManifestPath, sessionName)
			}
			ui.PrintSuccess("Session metadata finalized")
			updateVSCodeTabTitle(sessionName)
			return nil
		},
	}
	opCtx := &ops.OpContext{
		Tmux:            session.NewRealTmux(),
		CreationRuntime: runtime,
		OpenSessionStorage: func(context.Context) (dolt.Storage, func(), error) {
			adapter, err := getStorage()
			if err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to connect to session storage: %v", err))
				return nil, nil, nil
			}
			return adapter, func() { _ = adapter.Close() }, nil
		},
	}
	_, err = ops.CreateSessionWithContext(context.Background(), opCtx, &ops.CreateSessionRequest{
		Cwd:                    workDir,
		Title:                  sessionName,
		Model:                  modelName,
		Harness:                harnessName,
		SessionID:              sessionID,
		Caller:                 ops.CreateSessionCaller{Surface: ops.CreateSurfaceCLI},
		PermissionMode:         modeFlagValue,
		DisableAutoMode:        noAutoMode,
		MaxBudgetUSD:           maxBudgetUsd,
		ForwardTelemetry:       true,
		ForwardClaudeOAuth:     true,
		AllowEmptyPrompt:       true,
		AllowUnsafeTitle:       true,
		ReuseExistingTmux:      true,
		RegistrationOptional:   true,
		ManifestDir:            manifestDir,
		ManifestDirOptional:    true,
		SkipCodexRemoteControl: true,
		Metadata: ops.CreateSessionMetadata{
			Workspace:        cfg.Workspace,
			ModelTier:        modelTierFlag,
			Tags:             buildSessionTags(roleName, sessionTags),
			PermissionPolicy: resolvedSessionPermissionPolicy,
			IsTest:           testMode,
			PermissionMode:   modeFlagValue,
			OpenCodeServer:   os.Getenv("OPENCODE_SERVER_URL"),
		},
	})
	if err != nil {
		return err
	}

	ui.PrintSuccess(fmt.Sprintf("%s session started in current tmux!", harnessName))
	return nil
}

func commitCurrentTmuxManifest(manifestPath, sessionName string) {
	if err := git.CommitManifest(manifestPath, "create", sessionName); err != nil {
		debug.Log("manifest commit skipped: %v", err)
	}
}

// currentTmuxHarnessRuntime is the narrow interactive boundary used by
// current-pane creation. Keeping the dispatcher parameterized makes active
// harness coverage and failure propagation deterministic without replacing the
// shared creation lifecycle or mutating package-global test hooks.
type currentTmuxHarnessRuntime struct {
	startClaude   func(ops.HarnessLaunchSpec) error
	startCodex    func(ops.HarnessLaunchSpec) (bool, error)
	startOpenCode func(ops.HarnessLaunchSpec) error
	startGemini   func(ops.HarnessLaunchSpec) error
	startAgy      func(ops.HarnessLaunchSpec) error
	validateCodex func() error
}

func realCurrentTmuxHarnessRuntime() currentTmuxHarnessRuntime {
	return currentTmuxHarnessRuntime{
		startClaude:   startCurrentTmuxClaude,
		startCodex:    queueCurrentTmuxCodex,
		startOpenCode: startCurrentTmuxOpenCode,
		startGemini:   startCurrentTmuxGemini,
		startAgy:      startCurrentTmuxAgy,
		validateCodex: validateCodexCredentials,
	}
}

type currentTmuxCodexQueueRuntime struct {
	sendCommand func(sessionName, command string) error
}

func realCurrentTmuxCodexQueueRuntime() currentTmuxCodexQueueRuntime {
	return currentTmuxCodexQueueRuntime{sendCommand: tmux.SendCommand}
}

// queueCurrentTmuxCodex queues Codex behind the AGM process currently owning
// the pane. It deliberately does not wait for composer readiness: the shell
// cannot consume the queued command until AGM finishes metadata registration
// and returns control of the pane.
func queueCurrentTmuxCodex(spec ops.HarnessLaunchSpec) (bool, error) {
	return queueCurrentTmuxCodexWithRuntime(spec, realCurrentTmuxCodexQueueRuntime())
}

func queueCurrentTmuxCodexWithRuntime(spec ops.HarnessLaunchSpec, runtime currentTmuxCodexQueueRuntime) (bool, error) {
	launch := ops.BuildHarnessLaunchCommand(spec)
	if err := runtime.sendCommand(spec.SessionName, launch.Command); err != nil {
		ui.PrintError(err,
			"Failed to queue Codex in current tmux pane",
			"  • Verify Codex is installed: which codex\n"+
				"  • Test Codex manually: codex --version\n"+
				"  • Check you're in tmux: echo $TMUX")
		return false, err
	}
	debug.Log("Codex command queued; metadata will finalize before the current shell launches it")
	ui.PrintSuccess("Queued Codex CLI in current tmux session")
	return launch.ModeAppliedAtStartup, nil
}

// startCurrentTmuxHarness dispatches the per-harness startup flow for the
// in-place (current tmux pane) active harnesses and deprecated Gemini
// compatibility path.
func startCurrentTmuxHarness(spec ops.HarnessLaunchSpec) error {
	return startCurrentTmuxHarnessWithRuntime(spec, realCurrentTmuxHarnessRuntime())
}

func startCurrentTmuxHarnessWithRuntime(spec ops.HarnessLaunchSpec, runtime currentTmuxHarnessRuntime) error {
	switch spec.Harness {
	case "claude-code":
		return runtime.startClaude(spec)
	case "codex-cli":
		if err := runtime.validateCodex(); err != nil {
			return err
		}
		_, err := runtime.startCodex(spec)
		return err
	case "opencode-cli":
		return runtime.startOpenCode(spec)
	case "gemini-cli":
		return runtime.startGemini(spec)
	case "agy":
		return runtime.startAgy(spec)
	default:
		debug.Log("Skipping CLI startup for harness: %s (no CLI configured)", spec.Harness)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s harness", spec.Harness))
		return nil
	}
}

// startCurrentTmuxClaude runs Claude in the current tmux pane, waits for
// readiness, and runs the rename/agm-assoc init sequence.
func startCurrentTmuxClaude(spec ops.HarnessLaunchSpec) error {
	claudeReady := tmux.NewClaudeReadyFile(spec.SessionName)
	if err := claudeReady.Cleanup(); err != nil {
		debug.Log("Warning: failed to cleanup ready-files: %v", err)
	}
	fmt.Println("Starting Claude CLI...")
	claudeCmd := ops.BuildHarnessLaunchCommand(spec).Command
	if err := tmux.SendCommand(spec.SessionName, claudeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Claude in current tmux pane",
			"  • Verify Claude is installed: which claude\n"+
				"  • Test Claude manually: claude --version\n"+
				"  • Check you're in tmux: echo $TMUX\n"+
				"  • Exit tmux and try: agmnew "+spec.SessionName)
		return err
	}
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Waiting for Claude to initialize...")
	if err := tmux.WaitForClaudePrompt(spec.SessionName, 30*time.Second); err != nil {
		ui.PrintWarning("Claude ready signal not detected")
		fmt.Printf("💡 Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("Claude is ready!")
	}
	debug.Log("Triggering SessionStart hook post-verification")
	if err := claudeReady.TriggerHookManually(); err != nil {
		debug.Log("Manual hook trigger failed (non-fatal): %v", err)
	}
	if os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "" {
		debug.Log("Skipping association in test environment")
		ui.PrintSuccess("Test session ready (association skipped)")
		return nil
	}
	// Associate and signal readiness deterministically from Go — no dependency
	// on the /agm:agm-assoc plugin slash command loading in the pane (ce-o1sg).
	associateSpawnedClaudeSession(spec.SessionName)
	ui.PrintSuccess("Claude is ready and session associated!")
	return nil
}

// startCurrentTmuxOpenCode starts OpenCode in the current tmux pane and waits
// for prompt readiness.
func startCurrentTmuxOpenCode(spec ops.HarnessLaunchSpec) error {
	fmt.Println("Starting OpenCode...")
	opencodeCmd := ops.BuildHarnessLaunchCommand(spec).Command
	if err := tmux.SendCommand(spec.SessionName, opencodeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start OpenCode in current tmux pane",
			"  • Verify OpenCode server is running: curl http://localhost:4096/health\n"+
				"  • Start server if needed: opencode serve --port 4096\n"+
				"  • Check you're in tmux: echo $TMUX\n"+
				"  • Exit tmux and try: agm new "+spec.SessionName+" --harness opencode-cli")
		return err
	}
	fmt.Println("Waiting for OpenCode to initialize...")
	if err := tmux.WaitForPromptSimple(spec.SessionName, 30*time.Second); err != nil {
		ui.PrintWarning("OpenCode ready signal not detected")
		fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("OpenCode is ready!")
	}
	return nil
}

// startCurrentTmuxGemini starts Gemini in the current tmux pane and handles
// the optional first-run trust prompt.
func startCurrentTmuxGemini(spec ops.HarnessLaunchSpec) error {
	fmt.Println("Starting Gemini CLI...")
	geminiCmd := ops.BuildHarnessLaunchCommand(spec).Command
	debug.Log("Sending command: %s", geminiCmd)
	if err := tmux.SendCommand(spec.SessionName, geminiCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Gemini in current tmux pane",
			"  • Verify Gemini is installed: which gemini\n"+
				"  • Test Gemini manually: gemini --version\n"+
				"  • Check you're in tmux: echo $TMUX")
		return err
	}
	autoAcceptGeminiTrustPrompt(spec.SessionName)
	fmt.Println("Waiting for Gemini to initialize...")
	if err := tmux.WaitForPromptSimple(spec.SessionName, 30*time.Second); err != nil {
		ui.PrintWarning("Gemini ready signal not detected")
		fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("Gemini is ready!")
	}
	return nil
}

func startCurrentTmuxAgy(spec ops.HarnessLaunchSpec) error {
	fmt.Println("Starting AGY...")
	agyCmd := ops.BuildHarnessLaunchCommand(spec).Command
	if err := tmux.SendCommand(spec.SessionName, agyCmd); err != nil {
		ui.PrintError(err,
			"Failed to start AGY in current tmux pane",
			"  • Verify AGY is installed: which agy\n"+
				"  • Test AGY manually: agy --help\n"+
				"  • Check you're in tmux: echo $TMUX")
		return err
	}
	fmt.Println("Waiting for AGY to initialize...")
	if err := tmux.WaitForAgyPrompt(spec.SessionName, 30*time.Second); err != nil {
		ui.PrintWarning("AGY ready signal not detected")
		fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("AGY is ready!")
	}
	associateSpawnedAgySession(spec.SessionName)
	return nil
}

// autoAcceptGeminiTrustPrompt scans the tmux pane for the Gemini trust prompt
// and answers "1<Enter>" if found.
func autoAcceptGeminiTrustPrompt(sessionName string) {
	debug.Log("Checking for Gemini trust prompt (3s window)...")
	time.Sleep(2 * time.Second)
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
	trustCheckCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-20")
	trustOutput, err := trustCheckCmd.CombinedOutput()
	if err != nil {
		return
	}
	content := string(trustOutput)
	if !strings.Contains(content, "Do you trust") && !strings.Contains(content, "trust the files") {
		debug.Log("No trust prompt detected (directory already trusted)")
		return
	}
	debug.Log("Gemini trust prompt detected, auto-accepting with '1' + Enter")
	selectCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "1")
	_ = selectCmd.Run()
	time.Sleep(300 * time.Millisecond)
	enterCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "-H", "0d")
	_ = enterCmd.Run()
	debug.Log("Trust prompt auto-accepted")
	ui.PrintSuccess("Auto-accepted Gemini trust prompt")
}

// monitorAndAnswerTrustPrompt monitors tmux output via control mode and answers trust prompt if detected
// Returns nil if no prompt appears (success), error if prompt appears but we can't answer it
func monitorAndAnswerTrustPrompt(sessionName string, timeout time.Duration) error {
	// Start control mode
	ctrl, err := tmux.StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer func() { _ = ctrl.Close() }()

	// Create output watcher
	watcher := tmux.NewOutputWatcher(ctrl.Stdout)

	deadline := time.Now().Add(timeout)
	trustPromptDetected := false

	for time.Now().Before(deadline) {
		// Read next line with short timeout
		line, err := watcher.GetRawLine(1 * time.Second)
		if err != nil {
			// Timeout reading - no more output
			// If we haven't seen trust prompt, assume it won't appear
			if !trustPromptDetected {
				debug.Log("No trust prompt detected (good - additionalDirectories likely worked)")
				return nil
			}
			continue
		}

		// Parse %output events
		if !strings.HasPrefix(line, "%output") {
			continue
		}

		content := tmux.ExtractOutputContent(line)

		// Check for trust prompt
		if strings.Contains(content, "Do you trust the files in this folder?") {
			trustPromptDetected = true
			debug.Log("Trust prompt detected!")
			fmt.Println("📋 Trust prompt appeared - answering automatically...")
		}

		// If we detected the prompt, look for the selection UI
		if trustPromptDetected && strings.Contains(content, "Yes, proceed") {
			debug.Log("Sending Enter to select 'Yes, proceed'")

			// Close control mode before sending keys (mixing control + send-keys doesn't work well)
			_ = ctrl.Close()

			// Send Enter key via regular tmux
			if err := tmux.SendCommand(sessionName, "C-m"); err != nil {
				return fmt.Errorf("failed to answer trust prompt: %w", err)
			}

			debug.Log("Trust prompt answered successfully")
			ui.PrintSuccess("Trust prompt answered")
			return nil
		}
	}

	if trustPromptDetected {
		return fmt.Errorf("trust prompt detected but couldn't find 'Yes, proceed' option")
	}

	// No trust prompt seen - this is success
	return nil
}

// addToAdditionalDirectories was removed — it wrote sandbox paths to the global
// ~/.claude/settings.json, breaking sandbox isolation. Trust is now handled
// exclusively via --add-dir CLI flags passed per-session to Claude.

// Permission resolution, parent permission reading, and project settings
// configuration have been moved to agm/internal/rbac package.
// Use rbac.ResolvePermissions, rbac.ReadParentPermissions, and
// rbac.ConfigureProjectPermissions respectively.
