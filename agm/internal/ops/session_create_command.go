package ops

import (
	"fmt"
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
}

// HarnessLaunchCommand is the command plus the startup-policy outcome needed
// by post-create handling.
type HarnessLaunchCommand struct {
	Command              string
	ModeAppliedAtStartup bool
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
	command := harnessexec.BuildClaudeCommand(harnessexec.ClaudeLaunch{
		SessionName:      spec.SessionName,
		SessionID:        spec.SessionID,
		Model:            resolvedModel,
		AddDirs:          addDirs,
		AutoMode:         !spec.DisableAutoMode,
		Permission:       permission,
		MaxBudgetUSD:     spec.MaxBudgetUSD,
		DisableOAuth:     spec.DisableOAuth,
		ForwardTelemetry: spec.ForwardTelemetry,
		Persistent:       spec.Persistent,
	})
	return HarnessLaunchCommand{Command: command, ModeAppliedAtStartup: modeApplied}
}

func buildCodexLaunchCommand(spec HarnessLaunchSpec, _ string) HarnessLaunchCommand {
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
	command := harnessexec.BuildCodexCommand(harnessexec.CodexLaunch{
		SessionName: spec.SessionName,
		Model:       resolvedModel,
		WorkDir:     spec.WorkDir,
		Sandbox:     sandboxMode,
		Approval:    approval,
		AddDirs:     spec.ExtraAddDirs,
		ResumeID:    resumeID,
		Remote:      resumeID != "",
		Persistent:  spec.Persistent,
	})
	return HarnessLaunchCommand{Command: command, ModeAppliedAtStartup: modeApplied}
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
