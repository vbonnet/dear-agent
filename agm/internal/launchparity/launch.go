// Package launchparity defines harness-neutral startup command contracts.
package launchparity

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// Contract describes the native startup tokens required by one active harness.
type Contract struct {
	Harness          string
	InteractiveToken string
	ModeToken        string
	ExitSuffix       string
}

// AgyCommandSpec is the complete native AGY launch/resume contract. Callers
// resolve model aliases before crossing this boundary; command ordering,
// permission mapping, quoting, and persistence stay centralized here.
type AgyCommandSpec struct {
	WorkDir        string
	ResolvedModel  string
	PermissionMode string
	ConversationID string
	ExtraAddDirs   []string
	Persistent     bool
}

// AgyCommand is the shell command plus the permission-policy outcome needed
// by lifecycle callers.
type AgyCommand struct {
	Command              string
	ModeAppliedAtStartup bool
}

// PiCommandSpec is the complete native Pi create/resume command contract.
type PiCommandSpec struct {
	WorkDir              string
	ResolvedModel        string
	SessionName          string
	SessionID            string
	LaunchID             string
	SessionDir           string
	CodingAgentDir       string
	PermissionMode       string
	PermissionExtension  string
	PermissionPolicyFile string
	Persistent           bool
}

// PiCommand is the Pi command plus its startup mode outcome.
type PiCommand struct {
	Command              string
	ModeAppliedAtStartup bool
}

// NewPiLaunchID returns a compact per-process identity that remains visible in
// Pi's shared terminal footer while retaining 64 bits of random uniqueness.
func NewPiLaunchID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
}

// BuildPiCommand constructs one canonical Pi command for create and resume.
func BuildPiCommand(spec PiCommandSpec) PiCommand {
	var b strings.Builder
	fmt.Fprintf(&b, "cd %s && env -u CLAUDECODE -u PI_CODING_AGENT_DIR AGM_SESSION_NAME=%s PI_SESSION_ID=%s AGM_PI_LAUNCH_ID=%s AGM_PI_PROJECT_DIR=%s", ShellQuote(spec.WorkDir), ShellQuote(spec.SessionName), ShellQuote(spec.SessionID), ShellQuote(spec.LaunchID), ShellQuote(spec.WorkDir))
	fmt.Fprintf(&b, " AGM_PI_PERMISSION_MODE=%s AGM_PI_PERMISSION_POLICY_FILE=%s", ShellQuote(defaultPiMode(spec.PermissionMode)), ShellQuote(spec.PermissionPolicyFile))
	if spec.CodingAgentDir != "" {
		fmt.Fprintf(&b, " PI_CODING_AGENT_DIR=%s", ShellQuote(spec.CodingAgentDir))
	}
	b.WriteString(" pi")
	fmt.Fprintf(&b, " --session-id %s --session-dir %s --name %s", ShellQuote(spec.SessionID), ShellQuote(spec.SessionDir), ShellQuote(spec.SessionName))
	if spec.ResolvedModel != "" {
		fmt.Fprintf(&b, " --model %s", ShellQuote(spec.ResolvedModel))
	}
	if spec.PermissionExtension != "" {
		fmt.Fprintf(&b, " --extension %s", ShellQuote(spec.PermissionExtension))
	}
	fmt.Fprintf(&b, " --approve --tools %s", ShellQuote(PiToolsForMode(spec.PermissionMode)))
	b.WriteString(ExitSuffix(spec.Persistent))
	return PiCommand{Command: b.String(), ModeAppliedAtStartup: true}
}

func defaultPiMode(mode string) string {
	if mode == "" {
		return "default"
	}
	return mode
}

// PiToolsForMode removes mutating tools completely in plan mode.
func PiToolsForMode(mode string) string {
	if mode == "plan" {
		return "read,grep,find,ls"
	}
	return "read,bash,edit,write,grep,find,ls"
}

// BuildAgyCommand constructs one canonical AGY interactive command for both
// fresh launches and cold resumes.
func BuildAgyCommand(spec AgyCommandSpec) AgyCommand {
	var b strings.Builder
	fmt.Fprintf(&b, "cd %s && agy", ShellQuote(spec.WorkDir))
	if spec.ResolvedModel != "" {
		fmt.Fprintf(&b, " --model %s", ShellQuote(spec.ResolvedModel))
	}
	modeFlag := AgyPermissionModeFlag(spec.PermissionMode)
	if modeFlag != "" {
		b.WriteString(" " + modeFlag)
	}
	if spec.ConversationID != "" {
		fmt.Fprintf(&b, " --conversation %s", ShellQuote(spec.ConversationID))
	}
	for _, dir := range spec.ExtraAddDirs {
		fmt.Fprintf(&b, " --add-dir %s", ShellQuote(dir))
	}
	b.WriteString(ExitSuffix(spec.Persistent))
	return AgyCommand{Command: b.String(), ModeAppliedAtStartup: modeFlag != ""}
}

// ShellQuote wraps a value for safe interpolation into a POSIX shell command.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

// Resolve returns the startup contract for an active harness.
func Resolve(harness, mode string, persistent bool) (Contract, error) {
	contract := Contract{Harness: harness, ExitSuffix: ExitSuffix(persistent)}
	switch harness {
	case "claude-code":
		contract.InteractiveToken = "claude"
		if mode != "" {
			contract.ModeToken = "--permission-mode " + mode
		}
	case "codex-cli":
		contract.InteractiveToken = "codex"
		contract.ModeToken = CodexPermissionModeFlag(mode)
	case "agy":
		// AGY's --prompt-interactive flag is string-valued and requires an
		// initial prompt. AGM starts a bare interactive process and delivers
		// detached startup prompts only after native readiness is observed.
		contract.InteractiveToken = "agy"
		contract.ModeToken = AgyPermissionModeFlag(mode)
	case "opencode-cli":
		contract.InteractiveToken = "opencode attach"
	case "pi-cli":
		contract.InteractiveToken = "pi"
		contract.ModeToken = "--tools " + PiToolsForMode(mode)
	default:
		return Contract{}, fmt.Errorf("unsupported active harness %q", harness)
	}
	return contract, nil
}

// ExitSuffix keeps the pane shell alive for persistent sessions.
func ExitSuffix(persistent bool) string {
	if persistent {
		return ""
	}
	return " && exit"
}

// CodexPermissionModeFlag maps shared permission modes to Codex startup flags.
func CodexPermissionModeFlag(mode string) string {
	switch mode {
	case "auto":
		return "-a never"
	case "plan":
		return "-a untrusted"
	default:
		return ""
	}
}

// CodexSandboxMode maps shared plan mode to a read-only Codex sandbox. Other
// modes retain AGM's workspace-write default.
func CodexSandboxMode(mode string) string {
	if mode == "plan" {
		return "read-only"
	}
	return "workspace-write"
}

// ValidateFinalLiveness requires both the tmux session and a harness process
// after post-create work completes.
func ValidateFinalLiveness(verdict tmux.PaneLiveness, err error) error {
	if err != nil {
		return fmt.Errorf("final harness liveness check failed: %w", err)
	}
	if !verdict.SessionExists {
		return fmt.Errorf("harness startup failed: tmux session disappeared before registration completed")
	}
	if !verdict.HarnessAlive {
		return fmt.Errorf("harness startup failed: no active harness process after post-create (%s)", verdict.Evidence)
	}
	return nil
}

// AgyPermissionModeFlag maps shared automatic mode to AGY's startup flag.
func AgyPermissionModeFlag(mode string) string {
	if mode == "auto" || mode == "dangerously-skip-permissions" {
		return "--dangerously-skip-permissions"
	}
	return ""
}
