package supervisor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultWakePrompt is the prompt AGMRecovery asks a stale peer to resume.
// It mirrors cmd/agm send wake-loop's default so a woken supervisor re-enters
// its monitoring loop rather than sitting idle at the prompt.
const DefaultWakePrompt = "/loop 5m /orchestrate"

// AGMRecovery is the production PeerRecovery: the corrective-action half of
// the mutual-unblock invariant (ADR-002). When the Loop reports a peer as
// stale for RecoveryThreshold consecutive ticks, it calls Recover, which
// shells out to `agm send wake-loop <peer> --prompt <prompt>` to nudge the
// stale peer's monitoring loop back into motion.
//
// This is the in-loop analogue of the external sentinel LoopMonitor
// (agm/internal/sentinel/daemon/loop_monitor.go): the retro
// 2026-06-17-vroom-overnight-supervisor-cascade found the LoopMonitor
// insufficient because it lives outside the mesh. AGMRecovery wires the same
// `agm send wake-loop` action directly into every supervisor's loop, so the
// surviving supervisors actively unblock each other instead of merely
// recording peer_stale to the decision trail.
type AGMRecovery struct {
	// Binary is the agm executable name or path. Defaults to "agm".
	Binary string

	// Prompt is the wake prompt forwarded via `--prompt`. Defaults to
	// DefaultWakePrompt.
	Prompt string

	// SessionFor maps a peer Role to its AGM session name. Defaults to the
	// identity mapping (the Role string is the session name), which matches
	// the canonical mesh session names ("meta-orchestrator", "orchestrator",
	// "overseer"). Override for deployments that suffix session names.
	SessionFor func(Role) string

	// RunCommand runs the wake command and returns its combined output. It is
	// injectable so tests can observe the invocation without spawning a real
	// process. Defaults to exec.CommandContext + CombinedOutput.
	RunCommand func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Recover implements PeerRecovery. It sends `agm send wake-loop <session>
// --prompt <prompt>` to the stale peer. A non-nil error means the wake command
// itself failed to run (the Loop records the outcome either way). The agm
// command is deliberately state-aware: it no-ops if the peer is mid-inference
// or offline, so calling Recover on a transiently-busy peer is harmless.
func (r *AGMRecovery) Recover(ctx context.Context, peerRole Role, reason string) error {
	binary := r.Binary
	if binary == "" {
		binary = "agm"
	}
	prompt := r.Prompt
	if prompt == "" {
		prompt = DefaultWakePrompt
	}
	session := string(peerRole)
	if r.SessionFor != nil {
		session = r.SessionFor(peerRole)
	}
	if session == "" {
		return fmt.Errorf("supervisor: AGMRecovery has no session name for peer %q", peerRole)
	}

	run := r.RunCommand
	if run == nil {
		run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		}
	}

	args := []string{"send", "wake-loop", session, "--prompt", prompt}
	out, err := run(ctx, binary, args...)
	if err != nil {
		return fmt.Errorf("supervisor: wake-loop %q failed: %w (output: %s)",
			session, err, strings.TrimSpace(string(out)))
	}
	return nil
}
