package tmux

import (
	"sync/atomic"
)

// autonomousMode is a process-global flag set by the agm CLI when a send is
// targeting an unattended session (e.g. `agm send msg --autonomous`, used by
// vroom-dispatch to drive the supervisory mesh). It is process-global rather
// than threaded through every Send* signature because `agm` is a single-shot
// CLI: one invocation == one send, so the flag's lifetime is the whole process.
//
// Historically (ce-v9in) this flag also gated the pre-send stash: only
// unattended sessions stashed their leftover input; attended sessions blocked
// on it. That distinction is gone — the human_typing guard no longer blocks for
// anyone, and every send stashes stale input via stashStaleInputLocked (see
// stash.go). autonomousMode now only gates the separate queued-paste / post-
// submit-cooldown aborts (see skipPostSubmitGuard and SendPromptLiteral), which
// remain protective for attended humans and are not part of human_typing.
var autonomousMode atomic.Bool

// SetAutonomousMode marks the current process as driving an unattended session.
// Call this once from the CLI layer when the operator passes --autonomous.
func SetAutonomousMode(on bool) { autonomousMode.Store(on) }

// AutonomousMode reports whether this process is driving an unattended session.
func AutonomousMode() bool { return autonomousMode.Load() }
