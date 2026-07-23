package ops

import (
	"fmt"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// HarnessLaunchSpec is the harness-neutral launch contract used by every
// creation surface. Surface adapters may still own interactive readiness and
// presentation, but they must not assemble a second launch command.
type HarnessLaunchSpec struct {
	Harness          string
	Model            string
	SessionName      string
	SessionID        string
	ResumeID         string
	WorkDir          string
	Persistent       bool
	PermissionMode   string
	DisableAutoMode  bool
	DisableOAuth     bool
	MaxBudgetUSD     float64
	ExtraAddDirs     []string
	ForwardTelemetry bool
	Codex            *manifest.Codex
	Pi               *manifest.Pi
	PiLaunchID       string
	PiExtension      string
	PiPolicyJSON     string
	PiPolicyFile     string
	// DeferredUntilCallerExit is set only by current-pane launchers whose
	// queued command cannot run until the producing AGM process releases tmux.
	DeferredUntilCallerExit bool
}

// HarnessLaunchCommand is the command plus the startup-policy outcome needed
// by post-create handling.
type HarnessLaunchCommand struct {
	Command              string
	ModeAppliedAtStartup bool
	Cancel               func() error
}

// CancelUndelivered removes a private handoff when its pane command could not
// be queued. Once delivery succeeds, the executor owns one-shot consumption.
func (c HarnessLaunchCommand) CancelUndelivered() error {
	if c.Cancel == nil {
		return nil
	}
	return c.Cancel()
}

// BuildHarnessLaunchCommand builds the one canonical shell command for a
// supported AGM harness.
func BuildHarnessLaunchCommand(spec HarnessLaunchSpec) HarnessLaunchCommand {
	exitSuffix := launchparity.ExitSuffix(spec.Persistent)
	switch agent.NormalizeHarnessName(spec.Harness) {
	case "claude-code":
		return buildClaudeLaunchCommand(spec, exitSuffix)
	case "codex-cli":
		return buildCodexLaunchCommand(spec, exitSuffix)
	case "agy":
		return buildAgyLaunchCommand(spec, exitSuffix)
	case "pi-cli":
		return buildPiLaunchCommand(spec)
	case "opencode-cli":
		return HarnessLaunchCommand{Command: fmt.Sprintf("cd %s && opencode attach%s", launchparity.ShellQuote(spec.WorkDir), exitSuffix)}
	case "gemini-cli":
		resolvedModel := agent.ResolveModelFullName("gemini-cli", spec.Model)
		return HarnessLaunchCommand{Command: fmt.Sprintf("gemini -m %s%s", launchparity.ShellQuote(resolvedModel), exitSuffix)}
	default:
		return HarnessLaunchCommand{Command: fmt.Sprintf("echo %s && exit 1", launchparity.ShellQuote("Unknown harness: "+spec.Harness))}
	}
}

// PrepareHarnessLaunchCommand stages caller-only credentials and telemetry for
// private Codex and Claude execution. The returned command contains only the
// absolute current AGM path, validated non-secret metadata, and an opaque
// owner-only handoff path. Call Cancel only when the command was not delivered.
func PrepareHarnessLaunchCommand(spec HarnessLaunchSpec) (HarnessLaunchCommand, error) {
	switch agent.NormalizeHarnessName(spec.Harness) {
	case "claude-code":
		launch, modeApplied := claudeLaunch(spec)
		prepared, err := harnessexec.PrepareClaudeCommand(launch, os.Environ())
		if err != nil {
			return HarnessLaunchCommand{}, err
		}
		return HarnessLaunchCommand{
			Command: prepared.Command, ModeAppliedAtStartup: modeApplied, Cancel: prepared.Cancel,
		}, nil
	case "codex-cli":
		launch, modeApplied := codexLaunch(spec)
		prepared, err := harnessexec.PrepareCodexCommand(launch, os.Environ())
		if err != nil {
			return HarnessLaunchCommand{}, err
		}
		return HarnessLaunchCommand{
			Command: prepared.Command, ModeAppliedAtStartup: modeApplied, Cancel: prepared.Cancel,
		}, nil
	default:
		return BuildHarnessLaunchCommand(spec), nil
	}
}

func buildPiLaunchCommand(spec HarnessLaunchSpec) HarnessLaunchCommand {
	nativeID := spec.SessionID
	sessionDir := ""
	codingAgentDir := ""
	if spec.Pi != nil {
		if spec.Pi.SessionID != "" {
			nativeID = spec.Pi.SessionID
		}
		sessionDir = spec.Pi.SessionDir
		codingAgentDir = spec.Pi.CodingAgentDir
	}
	launchID := spec.PiLaunchID
	if launchID == "" {
		launchID = launchparity.NewPiLaunchID()
	}
	command := launchparity.BuildPiCommand(launchparity.PiCommandSpec{
		WorkDir: spec.WorkDir, ResolvedModel: agent.ResolveModelFullName("pi-cli", spec.Model),
		SessionName: spec.SessionName, SessionID: nativeID, LaunchID: launchID, SessionDir: sessionDir,
		CodingAgentDir: codingAgentDir,
		PermissionMode: spec.PermissionMode, PermissionExtension: spec.PiExtension,
		PermissionPolicyFile: spec.PiPolicyFile, Persistent: spec.Persistent,
	})
	return HarnessLaunchCommand{Command: command.Command, ModeAppliedAtStartup: command.ModeAppliedAtStartup}
}

func buildClaudeLaunchCommand(spec HarnessLaunchSpec, _ string) HarnessLaunchCommand {
	launch, modeApplied := claudeLaunch(spec)
	return HarnessLaunchCommand{Command: harnessexec.BuildClaudeCommand(launch), ModeAppliedAtStartup: modeApplied}
}

func claudeLaunch(spec HarnessLaunchSpec) (harnessexec.ClaudeLaunch, bool) {
	resolvedModel := agent.ResolveModelFullName("claude-code", spec.Model)
	addDirs := make([]string, 0, len(spec.ExtraAddDirs)+1)
	addDirs = append(addDirs, spec.WorkDir)
	addDirs = append(addDirs, spec.ExtraAddDirs...)
	permission := ""
	modeApplied := false
	if spec.PermissionMode == "auto" || spec.PermissionMode == "plan" || spec.PermissionMode == "default" {
		permission = spec.PermissionMode
		modeApplied = true
	}
	launch := harnessexec.ClaudeLaunch{
		SessionName:            spec.SessionName,
		SessionID:              spec.SessionID,
		ResumeID:               spec.ResumeID,
		WorkDir:                spec.WorkDir,
		Model:                  resolvedModel,
		AddDirs:                addDirs,
		AutoMode:               !spec.DisableAutoMode,
		Permission:             permission,
		MaxBudgetUSD:           spec.MaxBudgetUSD,
		DisableOAuth:           spec.DisableOAuth,
		ForwardTelemetry:       spec.ForwardTelemetry,
		Persistent:             spec.Persistent,
		DeferUntilProducerExit: spec.DeferredUntilCallerExit,
	}
	return launch, modeApplied
}

func buildCodexLaunchCommand(spec HarnessLaunchSpec, _ string) HarnessLaunchCommand {
	launch, modeApplied := codexLaunch(spec)
	return HarnessLaunchCommand{Command: harnessexec.BuildCodexCommand(launch), ModeAppliedAtStartup: modeApplied}
}

func codexLaunch(spec HarnessLaunchSpec) (harnessexec.CodexLaunch, bool) {
	resolvedModel := agent.ResolveModelFullName("codex-cli", spec.Model)
	sandboxMode := launchparity.CodexSandboxMode(spec.PermissionMode)
	resumeID := ""
	if spec.Codex != nil {
		resumeID = spec.Codex.SessionID
	}
	modeApplied := false
	approval := ""
	if flag := launchparity.CodexPermissionModeFlag(spec.PermissionMode); flag != "" {
		approval = strings.TrimPrefix(flag, "-a ")
		modeApplied = true
	}
	launch := harnessexec.CodexLaunch{
		SessionName:            spec.SessionName,
		Model:                  resolvedModel,
		WorkDir:                spec.WorkDir,
		Sandbox:                sandboxMode,
		Approval:               approval,
		AddDirs:                spec.ExtraAddDirs,
		ResumeID:               resumeID,
		Remote:                 resumeID != "",
		Persistent:             spec.Persistent,
		DeferUntilProducerExit: spec.DeferredUntilCallerExit,
	}
	return launch, modeApplied
}

func buildAgyLaunchCommand(spec HarnessLaunchSpec, _ string) HarnessLaunchCommand {
	return buildAgyCommand(spec, "")
}

// BuildAgyResumeCommand builds AGY's native cold-resume command from the same
// model, permission, directory, quoting, and persistence policy used at create.
func BuildAgyResumeCommand(spec HarnessLaunchSpec, conversationID string) HarnessLaunchCommand {
	return buildAgyCommand(spec, conversationID)
}

func buildAgyCommand(spec HarnessLaunchSpec, conversationID string) HarnessLaunchCommand {
	resolvedModel := agent.ResolveModelFullName("agy", spec.Model)
	command := launchparity.BuildAgyCommand(launchparity.AgyCommandSpec{
		WorkDir:        spec.WorkDir,
		ResolvedModel:  resolvedModel,
		PermissionMode: spec.PermissionMode,
		ConversationID: conversationID,
		ExtraAddDirs:   spec.ExtraAddDirs,
		Persistent:     spec.Persistent,
	})
	return HarnessLaunchCommand{Command: command.Command, ModeAppliedAtStartup: command.ModeAppliedAtStartup}
}
