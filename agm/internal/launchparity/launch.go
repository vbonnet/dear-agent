// Package launchparity defines harness-neutral startup command contracts.
package launchparity

import (
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// Contract describes the native startup tokens required by one active harness.
type Contract struct {
	Harness          string
	InteractiveToken string
	ModeToken        string
	ExitSuffix       string
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
