package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
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

	createCurrentTmuxManifest(sessionName, workDir)

	if err := startCurrentTmuxHarness(sessionName, workDir); err != nil {
		return err
	}

	ui.PrintSuccess(fmt.Sprintf("%s session started in current tmux!", harnessName))
	updateVSCodeTabTitle(sessionName)
	return nil
}

// createCurrentTmuxManifest writes the manifest dir and registers a v2 session
// in Dolt for the in-place tmux pane Claude case. Failures are non-fatal —
// they only emit warnings, since the user already has a usable tmux pane.
func createCurrentTmuxManifest(sessionName, workDir string) {
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, sessionName)
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
		return
	}
	generatedUUID := uuid.New().String()
	spawnSessionID = generatedUUID // Expose to otelEnvArgs() for OTel injection
	debug.Log("Generated SessionID: %s", generatedUUID)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     generatedUUID,
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Workspace:     cfg.Workspace,
		Context:       manifest.Context{Project: workDir},
		Tmux:          manifest.Tmux{SessionName: sessionName},
		Harness:       harnessName,
		Model:         modelName,
	}
	if testMode {
		m.IsTest = true
		debug.Log("Marking session as test (is_test=true)")
	}
	adapter, err := getStorage()
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to connect to Dolt: %v", err))
		return
	}
	defer func() { _ = adapter.Close() }()
	if err := adapter.CreateSession(m); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create session in Dolt: %v", err))
		return
	}
	if testMode {
		ui.PrintSuccess(fmt.Sprintf("Test session registered in database: %s (hidden from default list)", m.SessionID))
	} else {
		ui.PrintSuccess(fmt.Sprintf("Session registered in database: %s", m.SessionID))
	}
	_ = git.CommitManifest(manifestPath, "create", sessionName)
}

// startCurrentTmuxHarness dispatches the per-harness startup flow for the
// in-place (current tmux pane) Claude/Gemini/OpenCode/AGY cases.
func startCurrentTmuxHarness(sessionName, workDir string) error {
	switch harnessName {
	case "claude-code":
		return startCurrentTmuxClaude(sessionName, workDir)
	case "opencode-cli":
		return startCurrentTmuxOpenCode(sessionName)
	case "gemini-cli":
		return startCurrentTmuxGemini(sessionName)
	case "agy":
		return startCurrentTmuxAgy(sessionName, workDir)
	default:
		debug.Log("Skipping CLI startup for harness: %s (no CLI configured)", harnessName)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s harness", harnessName))
		return nil
	}
}

// startCurrentTmuxClaude runs Claude in the current tmux pane, waits for
// readiness, and runs the rename/agm-assoc init sequence.
func startCurrentTmuxClaude(sessionName, workDir string) error {
	claudeReady := tmux.NewClaudeReadyFile(sessionName)
	if err := claudeReady.Cleanup(); err != nil {
		debug.Log("Warning: failed to cleanup ready-files: %v", err)
	}
	fmt.Println("Starting Claude CLI...")
	workDirForClaude := os.Getenv("PWD")
	if workDirForClaude == "" {
		workDirForClaude = workDir
	}
	resolvedModel := agent.ResolveModelFullName("claude-code", modelName)
	autoModeFlag := " --enable-auto-mode"
	if noAutoMode {
		autoModeFlag = ""
		debug.Log("Auto mode disabled by flag/env var")
	}
	claudeCmd := fmt.Sprintf("AGM_SESSION_NAME=%s%s%s claude --model %s --add-dir %s%s && exit", shellQuote(sessionName), otelEnvArgs(), oauthEnvArg(), shellQuote(resolvedModel), shellQuote(workDirForClaude), autoModeFlag)
	if modeFlagValue == "auto" || modeFlagValue == "plan" || modeFlagValue == "default" {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --permission-mode %s && exit", modeFlagValue), 1)
	}
	if maxBudgetUsd > 0 {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --max-budget-usd %.2f && exit", maxBudgetUsd), 1)
	}
	if err := tmux.SendCommand(sessionName, claudeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Claude in current tmux pane",
			"  • Verify Claude is installed: which claude\n"+
				"  • Test Claude manually: claude --version\n"+
				"  • Check you're in tmux: echo $TMUX\n"+
				"  • Exit tmux and try: agmnew "+sessionName)
		return err
	}
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Waiting for Claude to initialize...")
	if err := tmux.WaitForClaudePrompt(sessionName, 30*time.Second); err != nil {
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
	associateSpawnedClaudeSession(sessionName)
	ui.PrintSuccess("Claude is ready and session associated!")
	return nil
}

// startCurrentTmuxOpenCode starts OpenCode in the current tmux pane and waits
// for prompt readiness.
func startCurrentTmuxOpenCode(sessionName string) error {
	fmt.Println("Starting OpenCode...")
	opencodeCmd := "opencode attach && exit"
	if err := tmux.SendCommand(sessionName, opencodeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start OpenCode in current tmux pane",
			"  • Verify OpenCode server is running: curl http://localhost:4096/health\n"+
				"  • Start server if needed: opencode serve --port 4096\n"+
				"  • Check you're in tmux: echo $TMUX\n"+
				"  • Exit tmux and try: agm new "+sessionName+" --harness opencode-cli")
		return err
	}
	fmt.Println("Waiting for OpenCode to initialize...")
	if err := tmux.WaitForPromptSimple(sessionName, 30*time.Second); err != nil {
		ui.PrintWarning("OpenCode ready signal not detected")
		fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("OpenCode is ready!")
	}
	return nil
}

// startCurrentTmuxGemini starts Gemini in the current tmux pane and handles
// the optional first-run trust prompt.
func startCurrentTmuxGemini(sessionName string) error {
	fmt.Println("Starting Gemini CLI...")
	resolvedModel := agent.ResolveModelFullName("gemini-cli", modelName)
	geminiCmd := fmt.Sprintf("gemini -m %s && exit", shellQuote(resolvedModel))
	debug.Log("Sending command: %s", geminiCmd)
	if err := tmux.SendCommand(sessionName, geminiCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Gemini in current tmux pane",
			"  • Verify Gemini is installed: which gemini\n"+
				"  • Test Gemini manually: gemini --version\n"+
				"  • Check you're in tmux: echo $TMUX")
		return err
	}
	autoAcceptGeminiTrustPrompt(sessionName)
	fmt.Println("Waiting for Gemini to initialize...")
	if err := tmux.WaitForPromptSimple(sessionName, 30*time.Second); err != nil {
		ui.PrintWarning("Gemini ready signal not detected")
		fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("Gemini is ready!")
	}
	return nil
}

func startCurrentTmuxAgy(sessionName, workDir string) error {
	fmt.Println("Starting AGY...")
	agyCmd := buildAgyCommand(workDir, nil, modeFlagValue)
	if err := tmux.SendCommand(sessionName, agyCmd); err != nil {
		ui.PrintError(err,
			"Failed to start AGY in current tmux pane",
			"  • Verify AGY is installed: which agy\n"+
				"  • Test AGY manually: agy --help\n"+
				"  • Check you're in tmux: echo $TMUX")
		return err
	}
	fmt.Println("Waiting for AGY to initialize...")
	if err := tmux.WaitForAgyPrompt(sessionName, 30*time.Second); err != nil {
		ui.PrintWarning("AGY ready signal not detected")
		fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("AGY is ready!")
	}
	associateSpawnedAgySession(sessionName)
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
