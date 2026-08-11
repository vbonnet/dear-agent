package quota

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// DefaultSpawnGateMaxAge is how stale a published reading may be and
// still gate a spawn. Past this, the gate reports unknown and allows the
// spawn: an old reading is not evidence of a problem now.
const DefaultSpawnGateMaxAge = 90 * time.Minute

// SpawnGateOverrideEnvVar switches the guardrail off for one process.
// It exists because a guardrail with no escape hatch is one an operator
// disables permanently the first time it is wrong.
const SpawnGateOverrideEnvVar = "AGM_QUOTA_GUARDRAIL"

// SpawnDecision is the gate's answer for one spawn attempt.
type SpawnDecision struct {
	// Allowed is whether the spawn may proceed. False only ever follows
	// from a positive, fresh reading that the provider is out of room.
	Allowed bool

	// Family is the provider family the spawn would consume, or "" when
	// the model could not be mapped.
	Family string

	// State is the guardrail position behind the answer.
	State BreakerState

	// Reason explains the answer in one line, phrased for an operator
	// who is about to be told their spawn was refused.
	Reason string

	// ResetsAt is when the constraining window refills, so a refusal can
	// say when work resumes. Zero when unknown.
	ResetsAt time.Time
}

// SpawnGate decides whether a new agent session may start, based on the
// last published quota reading.
//
// It reads the state file rather than the meter directly, and that is the
// whole design: a spawn must not block for the seconds a live meter read
// takes, and a fleet of spawning processes must not each hammer the
// meter. One writer refreshes on a schedule; every gate does a local
// file read.
//
// The gate is fail-open by construction. No file, a stale file, a corrupt
// file, an unreadable provider, a model it cannot map — every one of them
// allows the spawn. It stops work only on a fresh reading that positively
// says the provider is out of room, because the cost of wrongly halting
// the fleet is far higher than the cost of one spawn too many.
type SpawnGate struct {
	// Path is the published state file. Empty resolves the default.
	Path string

	// Policy is the guardrail policy. The zero value uses the defaults.
	Policy BreakerPolicy

	// MaxAge is how old a reading may be and still gate. Zero uses
	// DefaultSpawnGateMaxAge; negative accepts any age.
	MaxAge time.Duration

	// FamilyForModel maps a spawn's model id to a provider family. Nil
	// uses ModelFamilyHeuristic.
	FamilyForModel func(model string) string

	// Now overrides the clock. Nil uses time.Now.
	Now func() time.Time

	// ReadState overrides the state loader. Nil reads Path.
	ReadState func(path string) (*State, error)
}

// ModelFamilyHeuristic maps a model id to a provider family by vendor
// prefix. It is intentionally the same shape as the agent package's
// mapping, duplicated here only so this package stays importable from
// anywhere without dragging in the harness layer.
func ModelFamilyHeuristic(model string) string {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "claude"), strings.Contains(lower, "anthropic"):
		return "anthropic"
	case strings.Contains(lower, "gpt"), strings.Contains(lower, "codex"), strings.Contains(lower, "openai"):
		return "openai"
	case strings.Contains(lower, "gemini"), strings.Contains(lower, "google"), strings.Contains(lower, "antigravity"):
		return "gemini"
	default:
		return ""
	}
}

// overrideEngaged reports whether the operator switched the guardrail
// off for this process.
func overrideEngaged() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(SpawnGateOverrideEnvVar))) {
	case "off", "0", "false", "disabled":
		return true
	default:
		return false
	}
}

func (g *SpawnGate) now() time.Time {
	if g == nil || g.Now == nil {
		return time.Now()
	}
	return g.Now()
}

func (g *SpawnGate) maxAge() time.Duration {
	if g.MaxAge == 0 {
		return DefaultSpawnGateMaxAge
	}
	return g.MaxAge
}

func (g *SpawnGate) family(model string) string {
	if g.FamilyForModel != nil {
		return g.FamilyForModel(model)
	}
	return ModelFamilyHeuristic(model)
}

// AllowSpawn decides whether a session using model may start.
//
// A nil gate allows everything, so callers may hold one unconditionally.
func (g *SpawnGate) AllowSpawn(model string) SpawnDecision {
	allow := func(reason string, family string) SpawnDecision {
		return SpawnDecision{Allowed: true, Family: family, State: BreakerClosed, Reason: reason}
	}
	if g == nil || overrideEngaged() {
		return allow("quota guardrail disabled", "")
	}

	family := g.family(model)
	if family == "" {
		return allow(fmt.Sprintf("model %q is not mapped to a metered provider", model), "")
	}

	state, err := g.loadState()
	switch {
	case err != nil:
		return allow(fmt.Sprintf("no usable quota reading (%v)", err), family)
	case state == nil:
		return allow("no usable quota reading", family)
	}

	if maxAge := g.maxAge(); maxAge > 0 {
		if age := state.Age(g.now()); age > maxAge {
			return allow(fmt.Sprintf("quota reading is %s old, past the %s gating limit",
				age.Round(time.Second), maxAge), family)
		}
	}

	provider, ok := state.Provider(family)
	if !ok {
		return allow(fmt.Sprintf("no quota reading for provider family %q", family), family)
	}
	if !provider.Readable {
		return allow(fmt.Sprintf("quota for %s is unreadable, not exhausted (%s)",
			family, provider.Availability), family)
	}
	if BreakerState(provider.BreakerState) != BreakerOpen {
		return SpawnDecision{
			Allowed:  true,
			Family:   family,
			State:    BreakerState(provider.BreakerState),
			Reason:   provider.Reason,
			ResetsAt: resetsAt(provider),
		}
	}

	return SpawnDecision{
		Allowed:  false,
		Family:   family,
		State:    BreakerOpen,
		Reason:   provider.Reason,
		ResetsAt: resetsAt(provider),
	}
}

func resetsAt(p ProviderState) time.Time {
	if p.ResetsAt == nil {
		return time.Time{}
	}
	return *p.ResetsAt
}

func (g *SpawnGate) loadState() (*State, error) {
	read := g.ReadState
	if read == nil {
		read = ReadStateFile
	}
	path := g.Path
	if path == "" {
		resolved, err := DefaultStateFilePath()
		if err != nil {
			return nil, err
		}
		path = resolved
	}
	return read(path)
}

// RefusalError renders a refused spawn as an operator-facing error. It
// names the constraint and, when known, when the provider recovers, so
// the message answers "what do I do now" rather than only "no".
func (d SpawnDecision) RefusalError() error {
	if d.Allowed {
		return nil
	}
	msg := fmt.Sprintf("quota guardrail: refusing to start a %s session — %s", d.Family, d.Reason)
	if !d.ResetsAt.IsZero() {
		msg += fmt.Sprintf("; the window resets at %s", d.ResetsAt.Format(time.RFC3339))
	}
	msg += fmt.Sprintf(". Route this work to another provider, or override with %s=off.", SpawnGateOverrideEnvVar)
	return fmt.Errorf("%s", msg)
}
