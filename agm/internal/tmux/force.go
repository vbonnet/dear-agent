package tmux

import "sync/atomic"

// forceDelivery is a process-global flag set by the agm CLI when an operator
// passes `agm send msg --force`. Like autonomousMode, it is process-global rather than threaded
// through every Send* signature because `agm` is a single-shot CLI: one
// invocation == one send, so the flag's lifetime is the whole process.
//
// Why this exists (ce-5sow): structured legacy delivery can pass through
// SendMultiLinePromptSafe's separate post-submit composer-stability check.
// Setting this flag makes that path honor the operator's delivery intent too.
// Shared message delivery receives the same intent explicitly through
// InputDeliveryOptions and may override only a verified QUEUE state.
//
// Unlike autonomousMode, force does NOT stash stale input with C-s — the
// operator has explicitly accepted the risk and just wants the message
// delivered. Permission and wrong-harness states remain protected.
var forceDelivery atomic.Bool

// SetForceDelivery marks the current process as performing an operator-forced
// send. Call this once from the CLI layer after --force has been accepted.
func SetForceDelivery(on bool) { forceDelivery.Store(on) }

// ForceDelivery reports whether this process is performing an operator-forced send.
func ForceDelivery() bool { return forceDelivery.Load() }
