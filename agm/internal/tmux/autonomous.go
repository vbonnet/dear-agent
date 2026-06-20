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
// mid-keystroke. The whole mesh then freezes.
//
// In an unattended session there is, by definition, no human at the keyboard,
// so any non-ghost leftover text is provably AGM's own stale send. We therefore
// stash it with C-s before delivering, then skip the human-typing aborts.
// Claude Code's C-s stashes the current input and auto-unstashes once the next
// message is submitted — so the stale text never blocks delivery, yet (unlike a
// destructive C-u clear) any input that genuinely belonged to a human is
// preserved and reinstated after AGM's message goes through. Attended sessions
// are unaffected: without --autonomous the flag is false and the original
// human-protecting behavior stands.
var autonomousMode atomic.Bool

// SetAutonomousMode marks the current process as driving an unattended session.
// Call this once from the CLI layer when the operator passes --autonomous.
func SetAutonomousMode(on bool) { autonomousMode.Store(on) }

// AutonomousMode reports whether this process is driving an unattended session.
func AutonomousMode() bool { return autonomousMode.Load() }

// shouldClearStaleInput reports whether the input box holds stashable stale
// text: real (non-ghost) content after the ❯ prompt. Ghost text (Claude Code's
// dim placeholder, \x1b[2m) is excluded — there is nothing real to stash and it
// never blocks submission. Pure decision helper (no tmux I/O) for testability.
func shouldClearStaleInput(ansiContent string) bool {
	return InputLineHasContent(stripANSI(ansiContent)) && !HasGhostTextInANSI(ansiContent)
}

// clearStaleInputLocked stashes any leftover non-ghost text from the input box
// before a send, so AGM's own un-submitted text is neither concatenated with
// the new send nor mistaken for human typing. It is a no-op unless autonomous
// mode is active.
//
// It uses Claude Code's C-s stash (not a destructive C-u clear): the current
// input is set aside and auto-restored once the next message is submitted. If
// the leftover text was actually a human's in-progress message, it is preserved
// and reinstated after AGM's message goes through, rather than destroyed.
//
// MUST be called while already holding the tmux server lock (it issues raw
// send-keys via exec rather than re-entering withTmuxLock, which is not
// reentrant). socketPath and normalizedTarget must be pre-resolved.
//
// Ghost text (Claude Code's dim placeholder, \x1b[2m) is left alone: there is
// nothing real to stash and Claude ignores it on submit, so it never blocks
// delivery.
func clearStaleInputLocked(socketPath, normalizedTarget, ansiContent string) bool {
	if !autonomousMode.Load() || !shouldClearStaleInput(ansiContent) {
		return false
	}
	// C-s stashes the current input buffer; Claude Code auto-unstashes it once
	// the next message is submitted, preserving any genuine human input.
	cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedTarget, "C-s")
	if err := cmd.Run(); err != nil {
		return false
	}
	// Give Claude Code a moment to re-render the now-empty input line before the
	// caller captures/pastes.
	time.Sleep(50 * time.Millisecond)
	return true
}
