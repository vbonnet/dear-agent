// Package override is the shared contract for dangerous overrides — the
// escape hatches that let an operator switch off a safety control.
//
// Two exist today: the Codex hook-trust bypass, which runs repo hooks without
// per-path review, and the admission-brake override, which admits spawns while
// a watchdog hold is engaged. Both were introduced for the same reason: the
// control they disable had no bounded way out, so the alternative was an
// outage. Both carry the same failure mode: an unattended agent flips one to
// get past a blocker, nobody notices, and the control is silently dead.
//
// Every override therefore travels the same four gates:
//
//  1. Reason — the caller states WHY, in its own words. Missing, trivial, or
//     boilerplate reasons are refused. The reason is what a later audit reads.
//  2. Grant — a human approved this override kind, recently, with an expiry.
//     Grants are minted only through an interactive terminal, so an unattended
//     agent cannot mint one for itself.
//  3. Ledger — every authorized use is appended durably: kind, reason, actor,
//     session, timestamp.
//  4. Audit — the ledger is reviewed on a schedule, and an alert fires when
//     use climbs. A rising count is the signal that the override has become a
//     workaround for an unfixed root cause.
//
// The gates fail closed. An unreadable grant, an unreadable ledger, or an
// unwritable ledger refuses the override: an override nobody can audit is
// exactly the one that must not proceed.
package override

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Kind identifies a dangerous override. Adding one means adopting all four
// gates; there is deliberately no way to register a kind that skips them.
type Kind string

const (
	// KindCodexHookTrust covers launching Codex with
	// --dangerously-bypass-hook-trust, which runs every enabled hook without
	// the per-path trust review.
	KindCodexHookTrust Kind = "codex-hook-trust"

	// KindAdmissionBrake covers admitting a spawn while an admission brake is
	// engaged by a watchdog or an operator.
	KindAdmissionBrake Kind = "admission-brake"
)

// Kinds returns every known override kind, in stable order.
func Kinds() []Kind { return []Kind{KindCodexHookTrust, KindAdmissionBrake} }

// Valid reports whether k is a known override kind.
func (k Kind) Valid() bool { return slices.Contains(Kinds(), k) }

// MinReasonRunes is the shortest accepted reason. It is long enough to make
// "because" and "fix" fail while staying short enough for a real one-liner.
const MinReasonRunes = 16

// Sentinel errors so callers can distinguish "you may not" from "you did it
// wrong" and print the right remediation.
var (
	ErrUnknownKind     = errors.New("unknown override kind")
	ErrReasonMissing   = errors.New("override requires a reason")
	ErrReasonTooShort  = errors.New("override reason is too short to audit")
	ErrReasonBoilerpl8 = errors.New("override reason is boilerplate")
	ErrNoGrant         = errors.New("no human approval on file for this override")
	ErrGrantExpired    = errors.New("human approval for this override has expired")
	ErrGrantKind       = errors.New("human approval is for a different override kind")
)

// boilerplateReasons are refused outright. A reason that survives this list is
// not necessarily good, but the audit gate is what judges quality — this list
// only stops the reflex answers that carry no information at all.
var boilerplateReasons = []string{
	"because", "because i need to", "need to", "needed", "n/a", "na", "none",
	"test", "testing", "just testing", "temporary", "temp", "tmp", "quick fix",
	"it is blocked", "blocked", "unblock", "to unblock", "fix", "fixing",
	"required", "necessary", "workaround", "bypass", "override", "please",
	"idk", "unknown", "asdf", "todo", "tbd",
}

// ValidateReason normalizes and checks a caller-supplied reason, returning the
// normalized form. Whitespace is collapsed so a reason cannot pad its way past
// the length gate.
func ValidateReason(reason string) (string, error) {
	normalized := strings.Join(strings.Fields(reason), " ")
	if normalized == "" {
		return "", ErrReasonMissing
	}
	if len([]rune(normalized)) < MinReasonRunes {
		return "", fmt.Errorf("%w: %q is %d characters, need %d",
			ErrReasonTooShort, normalized, len([]rune(normalized)), MinReasonRunes)
	}
	folded := strings.ToLower(strings.TrimRight(normalized, ".!"))
	if slices.Contains(boilerplateReasons, folded) {
		return "", fmt.Errorf("%w: %q says nothing an audit could act on", ErrReasonBoilerpl8, normalized)
	}
	return normalized, nil
}

// Grant records that a human approved an override kind for a bounded window.
// It is the only gate an unattended agent cannot satisfy on its own: grants are
// minted exclusively through an interactive terminal.
type Grant struct {
	Kind          Kind      `json:"kind"`
	ApprovedBy    string    `json:"approved_by"`
	ApprovedAtUTC time.Time `json:"approved_at_utc"`
	ExpiresUTC    time.Time `json:"expires_utc"`
	Note          string    `json:"note,omitempty"`
}

// Active reports whether g authorizes kind at now.
func (g *Grant) Active(kind Kind, now time.Time) error {
	if g == nil {
		return ErrNoGrant
	}
	if g.Kind != kind {
		return fmt.Errorf("%w: approval covers %q, requested %q", ErrGrantKind, g.Kind, kind)
	}
	if !now.Before(g.ExpiresUTC) {
		return fmt.Errorf("%w: expired at %s", ErrGrantExpired, g.ExpiresUTC.UTC().Format(time.RFC3339))
	}
	return nil
}

// Use is one authorized override, as written to the ledger.
type Use struct {
	Kind    Kind      `json:"kind"`
	Reason  string    `json:"reason"`
	Actor   string    `json:"actor"`
	Session string    `json:"session,omitempty"`
	AtUTC   time.Time `json:"at_utc"`
}

// Request is one attempt to use a dangerous override.
type Request struct {
	Kind    Kind
	Reason  string
	Actor   string
	Session string
	Now     time.Time
}

// Dir is the root for override state. AGM_CONFIG_DIR keeps it beside the rest
// of AGM's state so a relocated config directory does not silently split the
// grant from the ledger that audits it.
func Dir() string {
	if dir := os.Getenv("AGM_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "overrides")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".agm", "overrides")
	}
	return filepath.Join(home, ".agm", "overrides")
}

// GrantPath is where the human approval for kind lives.
func GrantPath(kind Kind) string {
	return filepath.Join(Dir(), "grants", string(kind)+".json")
}

// LedgerPath is the append-only record of authorized overrides.
func LedgerPath() string { return filepath.Join(Dir(), "ledger.jsonl") }

// LoadGrant reads the approval for kind. A missing grant is (nil, nil) so the
// caller can report "not approved" distinctly from "could not tell".
func LoadGrant(kind Kind) (*Grant, error) {
	data, err := os.ReadFile(GrantPath(kind))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read override grant: %w", err)
	}
	var grant Grant
	if err := json.Unmarshal(data, &grant); err != nil {
		return nil, fmt.Errorf("parse override grant %s: %w", GrantPath(kind), err)
	}
	return &grant, nil
}

// SaveGrant persists a human approval. Callers must confirm interactively
// first; this function does not and cannot verify that.
func SaveGrant(grant Grant) error {
	if !grant.Kind.Valid() {
		return fmt.Errorf("%w: %q", ErrUnknownKind, grant.Kind)
	}
	path := GrantPath(grant.Kind)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create override grant dir: %w", err)
	}
	data, err := json.MarshalIndent(grant, "", "  ")
	if err != nil {
		return fmt.Errorf("encode override grant: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// RevokeGrant removes the approval for kind. Removing an absent grant is not
// an error: the caller wanted no grant, and there is none.
func RevokeGrant(kind Kind) error {
	err := os.Remove(GrantPath(kind))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("revoke override grant: %w", err)
	}
	return nil
}

// Record appends a use to the ledger. A use that cannot be recorded is an
// error the caller must surface: the ledger is what the audit gate reads, and
// an unrecorded override is an invisible one.
func Record(use Use) error {
	path := LedgerPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create override ledger dir: %w", err)
	}
	line, err := json.Marshal(use)
	if err != nil {
		return fmt.Errorf("encode override use: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open override ledger: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("append override ledger: %w", err)
	}
	return nil
}

// LoadUses reads recorded uses at or after since. A malformed line is skipped
// rather than fatal so one bad record cannot blind the whole audit.
func LoadUses(since time.Time) ([]Use, error) {
	data, err := os.ReadFile(LedgerPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read override ledger: %w", err)
	}
	var uses []Use
	for line := range strings.SplitSeq(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var use Use
		if err := json.Unmarshal([]byte(line), &use); err != nil {
			continue
		}
		if use.AtUTC.Before(since) {
			continue
		}
		uses = append(uses, use)
	}
	return uses, nil
}

// Authorize runs every gate and records the use. The returned error is safe to
// show a caller: it names the gate that refused and how to satisfy it.
//
// Authorize is the only sanctioned entry point. Callers must not consult
// ValidateReason or LoadGrant directly and decide for themselves — that is how
// one path ends up skipping the ledger.
func Authorize(req Request) (Use, error) {
	if !req.Kind.Valid() {
		return Use{}, fmt.Errorf("%w: %q", ErrUnknownKind, req.Kind)
	}
	reason, err := ValidateReason(req.Reason)
	if err != nil {
		return Use{}, err
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	grant, err := LoadGrant(req.Kind)
	if err != nil {
		// Fail closed: an unreadable grant is not an absent one.
		return Use{}, err
	}
	if err := grant.Active(req.Kind, now); err != nil {
		return Use{}, err
	}
	actor := req.Actor
	if actor == "" {
		actor = defaultActor()
	}
	use := Use{
		Kind:    req.Kind,
		Reason:  reason,
		Actor:   actor,
		Session: req.Session,
		AtUTC:   now.UTC(),
	}
	if err := Record(use); err != nil {
		return Use{}, err
	}
	return use, nil
}

func defaultActor() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return "unknown"
}

// AuditReport summarizes override use over a window.
type AuditReport struct {
	Window    time.Duration `json:"window"`
	Since     time.Time     `json:"since"`
	Total     int           `json:"total"`
	Threshold int           `json:"threshold"`
	Breached  bool          `json:"breached"`
	ByKind    map[Kind]int  `json:"by_kind"`
	ByReason  []ReasonTally `json:"by_reason"`
	Breaches  []KindBreach  `json:"breaches,omitempty"`
}

// ReasonTally counts how often one reason recurs. A reason that repeats is the
// audit's primary signal: the override has become load-bearing for a root
// cause nobody fixed.
type ReasonTally struct {
	Kind   Kind   `json:"kind"`
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// KindBreach records a kind whose use exceeded the threshold.
type KindBreach struct {
	Kind  Kind `json:"kind"`
	Count int  `json:"count"`
}

// Audit aggregates uses into a report. threshold is per kind: overrides are
// not interchangeable, so ten hook-trust bypasses are not offset by zero brake
// overrides. A threshold of zero disables the alert.
func Audit(uses []Use, window time.Duration, threshold int, now time.Time) AuditReport {
	report := AuditReport{
		Window:    window,
		Since:     now.Add(-window).UTC(),
		Threshold: threshold,
		ByKind:    map[Kind]int{},
		Total:     len(uses),
	}
	reasons := map[Kind]map[string]int{}
	for _, use := range uses {
		report.ByKind[use.Kind]++
		if reasons[use.Kind] == nil {
			reasons[use.Kind] = map[string]int{}
		}
		reasons[use.Kind][use.Reason]++
	}
	for kind, byReason := range reasons {
		for reason, count := range byReason {
			report.ByReason = append(report.ByReason, ReasonTally{Kind: kind, Reason: reason, Count: count})
		}
	}
	sort.Slice(report.ByReason, func(i, j int) bool {
		if report.ByReason[i].Count != report.ByReason[j].Count {
			return report.ByReason[i].Count > report.ByReason[j].Count
		}
		if report.ByReason[i].Kind != report.ByReason[j].Kind {
			return report.ByReason[i].Kind < report.ByReason[j].Kind
		}
		return report.ByReason[i].Reason < report.ByReason[j].Reason
	})
	if threshold > 0 {
		for _, kind := range Kinds() {
			if count := report.ByKind[kind]; count >= threshold {
				report.Breached = true
				report.Breaches = append(report.Breaches, KindBreach{Kind: kind, Count: count})
			}
		}
	}
	return report
}
