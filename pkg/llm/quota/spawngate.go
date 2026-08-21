package quota

import (
	"fmt"
	"os"
	"strings"
	"sync"
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

	// Evaluated reports whether a quota reading produced this answer.
	//
	// False means the guardrail could not evaluate the spawn: no published
	// reading, a stale or corrupt one, no entry for the family, an
	// unreadable provider, an operator override, or a model that mapped to
	// no metered provider. Those spawns are allowed by design, but they are
	// allowed without evidence. A caller that cannot tell them apart from a
	// measured pass cannot tell a working guardrail from an inert one — and
	// an inert guardrail that reports success is the failure mode this
	// field exists to make visible.
	Evaluated bool

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
//
// That fail-open must stay exceptional. A gate that fails open on the
// normal path is not a lenient guardrail, it is an absent one, and it
// reports success either way. So every fail-open marks its decision
// Evaluated=false and announces itself through Warn, and FamilyForModel
// must be wired to a resolver that understands the caller's model aliases:
// the bare tier aliases every harness defaults to ("sonnet", "5.5",
// "3.5-flash", "sonnet-200k") carry no vendor token, so a substring
// heuristic maps none of them and silently skips the guardrail for the
// default launch of every provider.
type SpawnGate struct {
	// Path is the published state file. Empty resolves the default.
	Path string

	// Policy is the guardrail policy. The zero value uses the defaults.
	Policy BreakerPolicy

	// MaxAge is how old a reading may be and still gate. Zero uses
	// DefaultSpawnGateMaxAge; negative accepts any age.
	MaxAge time.Duration

	// FamilyForModel maps a spawn's model id to a provider family. Nil
	// falls back to ModelFamilyHeuristic, which recognises only full model
	// identifiers — callers that spawn by alias must wire a resolver.
	FamilyForModel func(model string) string

	// Warn receives one line for every spawn the guardrail allowed without
	// being able to evaluate it. Nil writes to stderr.
	//
	// This path is noisy on purpose. A guardrail that quietly fails open is
	// indistinguishable from one that is working, which is how an inert
	// guardrail survives review; saying so out loud is what makes the
	// exception narrow rather than permanent.
	Warn func(msg string)

	// Now overrides the clock. Nil uses time.Now.
	Now func() time.Time

	// ReadState overrides the state loader. Nil reads Path.
	ReadState func(path string) (*State, error)

	// resolveDefaultPath caches the one-time DefaultStateFilePath()
	// lookup. Admission checks every spawn twice, and the default path
	// never changes for the life of a gate, so re-resolving it on every
	// check — os.UserHomeDir() and an env lookup — is pure repeated cost
	// on the spawn hot path (gemini review on #1325).
	resolveDefaultPath sync.Once
	defaultPath        string
	defaultPathErr     error
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
	// failOpen allows a spawn the guardrail could not evaluate, and says so
	// on both channels: Evaluated=false for callers, Warn for operators.
	// cause is a small fixed category used only to dedupe the warning;
	// reason is the full, detail-bearing message that gets printed — they
	// diverge on purpose so a reading that keeps getting staler by the
	// second doesn't mint a fresh warnedCauses entry every call (gemini
	// review on #1325).
	failOpen := func(cause string, reason string, family string) SpawnDecision {
		g.warn(cause+"|"+family, fmt.Sprintf(
			"quota guardrail: allowing this spawn without checking %s quota — %s",
			familyLabel(family), reason))
		return SpawnDecision{
			Allowed:   true,
			Family:    family,
			State:     BreakerClosed,
			Reason:    reason,
			Evaluated: false,
		}
	}
	if g == nil {
		return failOpen("no-gate", "no quota guardrail is wired on this spawn path", "")
	}
	if overrideEngaged() {
		return failOpen("override", fmt.Sprintf("%s disables the guardrail for this process",
			SpawnGateOverrideEnvVar), "")
	}

	family := g.family(model)
	if family == "" {
		return failOpen("unmapped-model", fmt.Sprintf(
			"model %q is not mapped to a metered provider; if this model does draw"+
				" down a subscription budget, its family resolver is missing an alias", model), "")
	}

	state, err := g.loadState()
	switch {
	case err != nil:
		return failOpen("read-error", fmt.Sprintf("no usable quota reading (%v)", err), family)
	case state == nil:
		return failOpen("no-state", "no usable quota reading", family)
	}

	if maxAge := g.maxAge(); maxAge > 0 {
		// A missing/unparseable generatedAt is an unknown age, not a
		// fresh one: State.Age reports a zero-GeneratedAt state as age 0
		// by design (so staleness alone never discards it — see Age's
		// doc comment), but that makes an undated reading look younger
		// than any real one and pass the check below unconditionally,
		// evaluating a breaker verdict this gate cannot actually vouch
		// for the freshness of. policy.go's Evaluate already treats
		// this the same way, gated the same way on maxAge > 0 — an
		// operator who has explicitly disabled the freshness check
		// (negative MaxAge) still gets that (codex review on #1218,
		// fourth pass).
		if state.GeneratedAt.IsZero() {
			return failOpen("no-generation-time", "quota reading has no generation time, cannot confirm freshness", family)
		}
		if age := state.Age(g.now()); age > maxAge {
			return failOpen("stale-reading", fmt.Sprintf("quota reading is %s old, past the %s gating limit",
				age.Round(time.Second), maxAge), family)
		}
	}

	provider, ok := state.Provider(family)
	if !ok {
		return failOpen("unmapped-family", fmt.Sprintf("no quota reading for provider family %q", family), family)
	}
	if !provider.Readable {
		return failOpen("unreadable", fmt.Sprintf("quota for %s is unreadable, not exhausted (%s)",
			family, provider.Availability), family)
	}
	if BreakerState(provider.BreakerState) != BreakerOpen {
		return SpawnDecision{
			Allowed:   true,
			Family:    family,
			State:     BreakerState(provider.BreakerState),
			Reason:    provider.Reason,
			ResetsAt:  resetsAt(provider),
			Evaluated: true,
		}
	}

	return SpawnDecision{
		Allowed:   false,
		Family:    family,
		State:     BreakerOpen,
		Reason:    provider.Reason,
		ResetsAt:  resetsAt(provider),
		Evaluated: true,
	}
}

// familyLabel names the provider in a warning, for the case where the gate
// failed open before it could work out which provider was at stake.
func familyLabel(family string) string {
	if family == "" {
		return "provider"
	}
	return family
}

// warnedCauses remembers which fail-open causes this process has already
// announced on the default channel.
//
// Admission checks each spawn twice (preflight, then again at launch), and a
// host that has not installed the refresh schedule fails open on every spawn.
// Printing that on every check would train an operator to filter the warning
// out, which costs more than the repetition buys — the warning has to survive
// to be worth emitting. One line per distinct cause per process keeps it
// loud without making it wallpaper. A caller that wants every occurrence sets
// Warn and does its own accounting.
var warnedCauses sync.Map

// warn announces a fail-open, deduped by cause rather than by the full
// message text so a detail that changes on every call (age, error text)
// can't turn a "once per cause" warning into one entry per call. A nil
// gate still warns: "no gate wired" is precisely the condition an
// operator needs to hear about.
func (g *SpawnGate) warn(cause, msg string) {
	if g != nil && g.Warn != nil {
		g.Warn(msg)
		return
	}
	if _, seen := warnedCauses.LoadOrStore(cause, struct{}{}); seen {
		return
	}
	fmt.Fprintln(os.Stderr, msg)
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
		g.resolveDefaultPath.Do(func() {
			g.defaultPath, g.defaultPathErr = DefaultStateFilePath()
		})
		if g.defaultPathErr != nil {
			return nil, g.defaultPathErr
		}
		path = g.defaultPath
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
