package tmux

import (
	"os/exec"
	"sync/atomic"
	"time"
)

// autonomousMode is a process-global flag set by the agm CLI when a send is
// targeting an unattended session (e.g. `agm send msg --autonomous`, used by
// vroom-dispatch to drive the supervisory mesh). It is process-global rather
// than threaded through every Send* signature because `agm` is a single-shot
// CLI: one invocation == one send, so the flag's lifetime is the whole process.
//
// Why this exists (ce-v9in — the #1 cause of VROOM mesh deadlocks):
// When a paste-buffer delivery's Enter (C-m) fails to register, the pasted text
// is left sitting in the Claude Code input box (e.g. "merge PR 527",
// "pkill -x gopls"). On the next tick the human_typing checks see that leftover
// text and refuse to send — mistaking AGM's own un-submitted text for a human
// mid-keystroke. The whole mesh then freezes until a human manually clears the
// line with C-u.
//
// In an unattended session there is, by definition, no human at the keyboard,
// so any non-ghost leftover text is provably AGM's own stale send. We therefore
// clear it with C-u (the documented manual workaround) before delivering, and
// skip the human-typing aborts. Attended sessions are unaffected: without
// --autonomous the flag is false and the original human-protecting behavior
// stands.
var autonomousMode atomic.Bool

// SetAutonomousMode marks the current process as driving an unattended session.
// Call this once from the CLI layer when the operator passes --autonomous.
func SetAutonomousMode(on bool) { autonomousMode.Store(on) }

// AutonomousMode reports whether this process is driving an unattended session.
func AutonomousMode() bool { return autonomousMode.Load() }

// shouldClearStaleInput reports whether the input box holds clearable stale
// text: real (non-ghost) content after the ❯ prompt. Ghost text (Claude Code's
// dim placeholder, \x1b[2m) is excluded — C-u cannot clear it and it never
// blocks submission. Pure decision helper (no tmux I/O) for testability.
func shouldClearStaleInput(ansiContent string) bool {
	return InputLineHasContent(stripANSI(ansiContent)) && !HasGhostTextInANSI(ansiContent)
}

// clearStaleInputLocked clears any leftover non-ghost text from the input box
// before a send, so AGM's own un-submitted text is neither concatenated with
// the new send nor mistaken for human typing. It is a no-op unless autonomous
// mode is active.
//
// MUST be called while already holding the tmux server lock (it issues raw
// send-keys via exec rather than re-entering withTmuxLock, which is not
// reentrant). socketPath and normalizedTarget must be pre-resolved.
//
// Ghost text (Claude Code's dim placeholder, \x1b[2m) is left alone: C-u cannot
// clear it and Claude ignores it on submit, so it never blocks delivery.
func clearStaleInputLocked(socketPath, normalizedTarget, ansiContent string) bool {
	if !autonomousMode.Load() || !shouldClearStaleInput(ansiContent) {
		return false
	}
	// C-u clears the readline-style input buffer to the start of the line —
	// the same keystroke a human uses to recover a stuck pane by hand.
	cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedTarget, "C-u")
	if err := cmd.Run(); err != nil {
		return false
	}
	// Give Claude Code a moment to re-render the now-empty input line before the
	// caller captures/pastes.
	time.Sleep(50 * time.Millisecond)
	return true
}
