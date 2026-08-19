package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
func startClaudeInCurrentTmux(ctx context.Context, sessionName string) error {
	var admission *circuitBreakerAdmission
	if !testMode {
		if dupErr := checkDuplicateSessionName(sessionName); dupErr != nil {
			return dupErr
		}
		var err error
		admission, err = enforceCircuitBreakers(sessionName, harnessName, modelName)
		if err != nil {
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
		launch: func(ctx context.Context, spec ops.HarnessLaunchSpec) (ops.CreateSessionLaunchResult, error) {
			if pwd := os.Getenv("PWD"); pwd != "" {
				spec.WorkDir = pwd
			}
			if admission != nil {
				spec.BeforeSpawn = admission.beforeSpawn
				spec.AfterAuthorization = admission.afterAuthorization
			}
			if err := startCurrentTmuxHarness(ctx, spec); err != nil {
				return ops.CreateSessionLaunchResult{}, err
			}
			return currentTmuxLaunchResult(spec.Harness), nil
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
				return nil, nil, err
			}
			return adapter, func() { _ = adapter.Close() }, nil
		},
	}
	_, err = ops.CreateSessionWithContext(ctx, opCtx, &ops.CreateSessionRequest{
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
		RequireStorage:         true,
		RegisterBeforeLaunch:   true,
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

func currentTmuxLaunchResult(harness string) ops.CreateSessionLaunchResult {
	result := ops.CreateSessionLaunchResult{}
	switch harness {
	case "claude-code", "codex-cli", "opencode-cli", "pi-cli", "gemini-cli":
		result.Readiness = ops.CreateSessionReadinessDeferredUntilCallerExit
	}
	return result
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
	startClaude   func(context.Context, ops.HarnessLaunchSpec) error
	startCodex    func(ops.HarnessLaunchSpec) (bool, error)
	startPi       func(ops.HarnessLaunchSpec) (bool, error)
	startOpenCode func(context.Context, ops.HarnessLaunchSpec) error
	startGemini   func(context.Context, ops.HarnessLaunchSpec) error
	validateCodex func() error
}

func realCurrentTmuxHarnessRuntime() currentTmuxHarnessRuntime {
	return currentTmuxHarnessRuntime{
		startClaude:   startCurrentTmuxClaude,
		startCodex:    queueCurrentTmuxCodex,
		startPi:       queueCurrentTmuxPi,
		startOpenCode: startCurrentTmuxOpenCode,
		startGemini:   startCurrentTmuxGemini,
		validateCodex: validateCodexCredentials,
	}
}

type currentTmuxPiQueueRuntime struct {
	sendCommand func(sessionName, command string) error
	lookPath    func(file string) (string, error)
}

func queueCurrentTmuxPi(spec ops.HarnessLaunchSpec) (bool, error) {
	return queueCurrentTmuxPiWithRuntime(spec, currentTmuxPiQueueRuntime{
		sendCommand: tmux.SendCommand,
		lookPath:    exec.LookPath,
	})
}

// queueCurrentTmuxPi mirrors Codex's in-pane lifecycle: AGM must finish
// registering metadata before the shell can consume the queued foreground Pi
// command. Detached creation performs the managed-ready wait instead.
func queueCurrentTmuxPiWithRuntime(spec ops.HarnessLaunchSpec, runtime currentTmuxPiQueueRuntime) (bool, error) {
	if _, err := runtime.lookPath("pi"); err != nil {
		return false, fmt.Errorf("pi executable is unavailable: %w", err)
	}
	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return false, fmt.Errorf("prepare Pi launch: %w", err)
	}
	if err := submitHarnessLaunch("Pi", spec, launch, func() error {
		return runtime.sendCommand(spec.SessionName, launch.Command)
	}); err != nil {
		ui.PrintError(err,
			"Failed to queue Pi in current tmux pane",
			"  • Verify Pi is installed: which pi\n"+
				"  • Test Pi manually: pi --version\n"+
				"  • Check you're in tmux: echo $TMUX")
		return false, err
	}
	debug.Log("Pi command queued; metadata will finalize before the current shell launches it")
	ui.PrintSuccess("Queued Pi in current tmux session")
	return launch.ModeAppliedAtStartup, nil
}

type currentTmuxCodexQueueRuntime struct {
	sendCommand func(sessionName, command string) error
	lookPath    func(file string) (string, error)
}

func realCurrentTmuxCodexQueueRuntime() currentTmuxCodexQueueRuntime {
	return currentTmuxCodexQueueRuntime{sendCommand: tmux.SendCommand, lookPath: exec.LookPath}
}

// queueCurrentTmuxCodex queues Codex behind the AGM process currently owning
// the pane. It deliberately does not wait for composer readiness: the shell
// cannot consume the queued command until AGM finishes metadata registration
// and returns control of the pane.
func queueCurrentTmuxCodex(spec ops.HarnessLaunchSpec) (bool, error) {
	return queueCurrentTmuxCodexWithRuntime(spec, realCurrentTmuxCodexQueueRuntime())
}

func queueCurrentTmuxCodexWithRuntime(spec ops.HarnessLaunchSpec, runtime currentTmuxCodexQueueRuntime) (bool, error) {
	if _, err := runtime.lookPath("codex"); err != nil {
		return false, fmt.Errorf("codex executable is unavailable: %w", err)
	}
	spec.DeferredUntilCallerExit = true
	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return false, fmt.Errorf("prepare Codex launch: %w", err)
	}
	if err := submitHarnessLaunch("Codex", spec, launch, func() error {
		return runtime.sendCommand(spec.SessionName, launch.Command)
	}); err != nil {
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

// startCurrentTmuxHarness dispatches the supported in-place (current tmux
// pane) harnesses and the deprecated Gemini compatibility path. AGY is
// deliberately excluded because its native identity cannot be correlated
// until the foreground AGM command returns; callers reject that route before
// invoking this dispatcher.
func startCurrentTmuxHarness(ctx context.Context, spec ops.HarnessLaunchSpec) error {
	return startCurrentTmuxHarnessWithRuntime(ctx, spec, realCurrentTmuxHarnessRuntime())
}

func currentTmuxAgyUnsupportedError() error {
	return fmt.Errorf("AGY cannot start in the current tmux pane because its provider-native identity cannot be correlated until the invoking AGM process exits; use --detached with a different session name")
}

func startCurrentTmuxHarnessWithRuntime(ctx context.Context, spec ops.HarnessLaunchSpec, runtime currentTmuxHarnessRuntime) error {
	switch spec.Harness {
	case "claude-code":
		return runtime.startClaude(ctx, spec)
	case "codex-cli":
		if err := runtime.validateCodex(); err != nil {
			return err
		}
		_, err := runtime.startCodex(spec)
		return err
	case "pi-cli":
		_, err := runtime.startPi(spec)
		return err
	case "opencode-cli":
		return runtime.startOpenCode(ctx, spec)
	case "gemini-cli":
		return runtime.startGemini(ctx, spec)
	case "agy":
		return currentTmuxAgyUnsupportedError()
	default:
		debug.Log("Skipping CLI startup for harness: %s (no CLI configured)", spec.Harness)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s harness", spec.Harness))
		return nil
	}
}

type currentTmuxCommandSender func(sessionName, command string) error

type currentTmuxCommandQueueRuntime struct {
	sendCommand currentTmuxCommandSender
	lookPath    func(file string) (string, error)
}

func realCurrentTmuxCommandQueueRuntime() currentTmuxCommandQueueRuntime {
	return currentTmuxCommandQueueRuntime{sendCommand: tmux.SendCommand, lookPath: exec.LookPath}
}

func currentTmuxHarnessExecutable(harness string) (string, error) {
	switch harness {
	case "claude-code":
		return "claude", nil
	case "opencode-cli":
		return "opencode", nil
	case "gemini-cli":
		return "gemini", nil
	default:
		return "", fmt.Errorf("current-tmux command queue does not support harness %q", harness)
	}
}

func queueCurrentTmuxHarnessCommand(ctx context.Context, spec ops.HarnessLaunchSpec, runtime currentTmuxCommandQueueRuntime) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	executable, err := currentTmuxHarnessExecutable(spec.Harness)
	if err != nil {
		return err
	}
	if _, err := runtime.lookPath(executable); err != nil {
		return fmt.Errorf("%s executable is unavailable: %w", executable, err)
	}
	spec.DeferredUntilCallerExit = true
	launch, err := ops.PrepareHarnessLaunchCommand(spec)
	if err != nil {
		return fmt.Errorf("prepare %s launch: %w", spec.Harness, err)
	}
	if err := submitHarnessLaunch(spec.Harness, spec, launch, func() error {
		return runtime.sendCommand(spec.SessionName, launch.Command)
	}); err != nil {
		return err
	}
	return nil
}

// startCurrentTmuxClaude queues Claude behind the foreground AGM process. The
// command cannot execute until AGM returns, so readiness is explicitly deferred.
func startCurrentTmuxClaude(ctx context.Context, spec ops.HarnessLaunchSpec) error {
	if err := queueCurrentTmuxHarnessCommand(ctx, spec, realCurrentTmuxCommandQueueRuntime()); err != nil {
		ui.PrintError(err,
			"Failed to queue Claude in current tmux pane",
			"  • Verify Claude is installed: which claude\n"+
				"  • Test Claude manually: claude --version\n"+
				"  • Check you're in tmux: echo $TMUX\n"+
				"  • Exit tmux and try: agmnew "+spec.SessionName)
		return err
	}
	debug.Log("Claude command queued; the SessionStart hook will associate its UUID after AGM releases the pane")
	ui.PrintSuccess("Queued Claude CLI in current tmux session")
	return nil
}

// startCurrentTmuxOpenCode queues OpenCode behind the foreground AGM process.
func startCurrentTmuxOpenCode(ctx context.Context, spec ops.HarnessLaunchSpec) error {
	if err := queueCurrentTmuxHarnessCommand(ctx, spec, realCurrentTmuxCommandQueueRuntime()); err != nil {
		ui.PrintError(err,
			"Failed to queue OpenCode in current tmux pane",
			"  • Verify OpenCode server is running: curl http://localhost:4096/health\n"+
				"  • Start server if needed: opencode serve --port 4096\n"+
				"  • Check you're in tmux: echo $TMUX\n"+
				"  • Exit tmux and try: agm new "+spec.SessionName+" --harness opencode-cli")
		return err
	}
	debug.Log("OpenCode command queued; readiness is deferred until AGM releases the pane")
	ui.PrintSuccess("Queued OpenCode in current tmux session")
	return nil
}

// startCurrentTmuxGemini queues deprecated Gemini compatibility behind the
// foreground AGM process.
func startCurrentTmuxGemini(ctx context.Context, spec ops.HarnessLaunchSpec) error {
	if err := queueCurrentTmuxHarnessCommand(ctx, spec, realCurrentTmuxCommandQueueRuntime()); err != nil {
		ui.PrintError(err,
			"Failed to queue Gemini in current tmux pane",
			"  • Verify Gemini is installed: which gemini\n"+
				"  • Test Gemini manually: gemini --version\n"+
				"  • Check you're in tmux: echo $TMUX")
		return err
	}
	debug.Log("Gemini command queued; readiness is deferred until AGM releases the pane")
	ui.PrintSuccess("Queued Gemini CLI in current tmux session")
	return nil
}

// monitorAndAnswerTrustPrompt polls the current exact pane and answers a live
// affirmative trust prompt. Control-mode fragments are not authority evidence.
func monitorAndAnswerTrustPrompt(ctx context.Context, sessionName string, timeout time.Duration) error {
	return monitorAndAnswerTrustPromptWithProbe(ctx, sessionName, timeout, 100*time.Millisecond, tmux.ProbeClaudeInputContext)
}

func monitorAndAnswerTrustPromptWithProbe(
	parent context.Context,
	sessionName string,
	timeout, pollInterval time.Duration,
	probeInput func(context.Context, string, bool) (tmux.ClaudeInputProbe, error),
) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	if pollInterval <= 0 {
		pollInterval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	trustPromptDetected := false

	for {
		if err := ctx.Err(); err != nil {
			if err == context.DeadlineExceeded {
				if trustPromptDetected {
					return fmt.Errorf("trust prompt detected but couldn't find its affirmative option")
				}
				return nil
			}
			return err
		}
		probe, probeErr := probeInput(ctx, sessionName, true)
		if probeErr != nil {
			return fmt.Errorf("probe current trust dialog: %w", probeErr)
		}
		if probe.DialogOwnsInput && !trustPromptDetected {
			trustPromptDetected = true
			debug.Log("Trust prompt detected from current exact pane")
			fmt.Println("📋 Trust prompt appeared - answering automatically...")
		}
		if probe.TrustAnswered {
			debug.Log("Trust prompt answered successfully")
			ui.PrintSuccess("Trust prompt answered")
			return nil
		}
		if probe.ComposerOwnsInput {
			debug.Log("No live trust prompt remains; Claude composer owns input")
			return nil
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}

// addToAdditionalDirectories was removed — it wrote sandbox paths to the global
// ~/.claude/settings.json, breaking sandbox isolation. Trust is now handled
// exclusively via --add-dir CLI flags passed per-session to Claude.

// Permission resolution, parent permission reading, and project settings
// configuration have been moved to agm/internal/rbac package.
// Use rbac.ResolvePermissions, rbac.ReadParentPermissions, and
// rbac.ConfigureProjectPermissions respectively.
