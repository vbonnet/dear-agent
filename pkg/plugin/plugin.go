package plugin

import (
	"github.com/vbonnet/dear-agent/pkg/audit"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// Plugin is the smallest possible contract every plugin satisfies. A
// plugin returns its Manifest and optionally implements one or more of
// the capability interfaces in this package (HookProvider, CheckProvider).
// A plugin that implements no capability is *valid*: it advertises
// presence and configuration without participating in execution. This
// is useful for two cases:
//
//   - A first-party plugin staged ahead of the capability it intends to
//     wire up (so the manifest can ship before the implementation).
//   - A configuration-only plugin whose only effect is to drive other
//     plugins through Manifest().Config.
//
// Capability detection is via Go type assertion at Registry.Register
// time and at fan-out time — no reflection on method names, no string
// tags. Adding a new capability in the future is purely additive: an
// older plugin that does not implement the new interface continues to
// satisfy Plugin and continues to work.
type Plugin interface {
	// Manifest returns the plugin's identity, capabilities, permissions,
	// and free-form config. It must be cheap and side-effect free; the
	// Registry calls it during Register and may call it again from
	// Plugins() snapshots. Implementations typically return a value
	// stored on the plugin struct rather than constructing a fresh one
	// each call.
	Manifest() Manifest
}

// HookProvider is implemented by plugins that participate in workflow
// lifecycle events. The returned workflow.Hooks value is composed with
// every other HookProvider's hooks via Registry.Hooks().
//
// Composition rules (see ADR-014 §D3):
//
//   - OnDefine: every provider's OnDefine runs in registration order.
//     The first non-nil error short-circuits and is returned to the
//     runner; later providers do not run for that call.
//   - OnEnforce: same as OnDefine. An error fails the node, matching
//     today's single-callback semantics.
//   - OnAudit: every provider's OnAudit runs unconditionally even if
//     earlier ones errored. Errors are accumulated with errors.Join
//     and returned; the runner logs but never blocks. This preserves
//     the substrate guarantee that audit emission is unconditional
//     (ADR-010 §D3).
//   - OnResolve: same accumulation as OnAudit — the run is already
//     failing, so reporting every plugin's resolve attempt is more
//     useful than short-circuiting.
//
// A HookProvider whose Hooks() returns a zero-value workflow.Hooks
// participates as a no-op. The Registry skips nil-callback fields so
// providers may implement only a subset of the four phases.
type HookProvider interface {
	Plugin

	// Hooks returns the per-phase callbacks this plugin contributes.
	// Any of the four fields may be nil; nil fields are skipped during
	// fan-out. Implementations should return a stable value (constructing
	// a fresh closure on each call is safe but wastes allocations).
	Hooks() workflow.Hooks
}

// CheckProvider is implemented by plugins that contribute audit
// checks. Registry.ApplyChecks(target) walks every CheckProvider and
// calls target.Register on each returned check, propagating the
// existing audit.Registry's idempotency and validation rules.
//
// A plugin's checks must satisfy audit.CheckMeta.Validate (non-empty
// ID, valid Cadence, valid SeverityCeiling) — the audit registry will
// reject malformed metadata. Two plugins returning checks with the
// same audit.CheckMeta.ID is an error; the second registration fails
// and ApplyChecks reports which plugin's check collided.
type CheckProvider interface {
	Plugin

	// Checks returns the audit.Check values this plugin contributes.
	// May return an empty slice. Each call should return logically
	// equivalent checks; ApplyChecks reads it once during registration.
	Checks() []audit.Check
}
