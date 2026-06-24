package tmux

import "sync/atomic"

// forceDelivery is a process-global flag set by the agm CLI when an operator
// passes `agm send msg --force` (with a justification recorded to the override
// audit log). Like autonomousMode, it is process-global rather than threaded
// through every Send* signature because `agm` is a single-shot CLI: one
// invocation == one send, so the flag's lifetime is the whole process.
//
// Why this exists (ce-5sow): `--force` already bypasses safety.Check's
// human_typing guard in ensureRecipientReady (via guardOpts.SkipHumanTyping),
// but delivery then funnels through SendMultiLinePromptSafe, which runs a
// SEPARATE post-submit cooldown check that re-aborts on "input line has content
// after cooldown — human is typing". So a forced send to a looping supervisor
// still failed at the tmux-direct layer. Setting this flag makes that layer
// honor the same override: when true, the post-submit human-typing cooldown
// abort is skipped, mirroring guardOpts.SkipHumanTyping upstream.
//
// Unlike autonomousMode, force does NOT stash stale input with C-s — the
// operator has explicitly accepted the risk and just wants the message
// delivered. The override audit (the required --reason) is validated upstream
// in ensureRecipientReady before this flag is ever set, so the bypass is never
// silent.
var forceDelivery atomic.Bool

// SetForceDelivery marks the current process as performing an operator-forced
// send. Call this once from the CLI layer after the --force override has been
// audited (override.Require) and accepted.
func SetForceDelivery(on bool) { forceDelivery.Store(on) }

// ForceDelivery reports whether this process is performing an operator-forced send.
func ForceDelivery() bool { return forceDelivery.Load() }
