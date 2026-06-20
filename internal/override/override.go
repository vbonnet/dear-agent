// Package override enforces that any bypass of a curated safety, confirmation,
// or overwrite gate carries a recorded justification — and, for high-risk
// bypasses, is vetted by an independent Judge before it runs.
//
// # Why this exists
//
// A safety gate with a one-keystroke bypass (--force / --skip-*) is not a gate.
// An LLM reaches for the bypass impulsively the moment the gate says "no",
// which defeats the gate's entire purpose. The fix is policy, enforced in code:
//
//  1. Every override flag MUST be accompanied by a mandatory --reason "text".
//     No reason → the *bypass* is refused (the underlying op is never touched
//     by this package; Guard only decides whether the bypass is permitted).
//  2. The reason + caller is appended to an audit log so a human can later see
//     who bypassed what, when, and why.
//  3. For P0 overrides an independent [Judge] verifies the reason *before* the
//     bypass executes — a separate decision, not the calling agent's own
//     judgment. The agent invoking --force is the party that wants the bypass;
//     it cannot also be the judge of whether the bypass is warranted.
//
// This is the standard pattern for curated override flags — any tool in this
// repo that needs a legitimate bypass must route through this package, not
// add a bare flag. Bypass flags without override.Require are prohibited
// (see ADR-031 and .claude/hooks/pretool-bypass-guard).
//
// Typical call site:
//
//	if force {
//	    if err := override.Require(ctx, override.Guard{
//	        Tool: "agm send-compact",
//	        Flag: "--force",
//	        Gate: "anti-loop guard (compaction cooldown and max-compactions cap)",
//	        Risk: override.RiskP0,
//	    }, reason); err != nil {
//	        return err // bypass refused; do NOT proceed with --force
//	    }
//	}
package override

import (
	"context"
	"fmt"
	"strings"
)

// Risk classifies how dangerous a bypass is. It drives whether the default
// [Judge] is sufficient (P2/P3: reason + audit) or an independent classifier
// should vet the reason (P0/P1).
type Risk string

const (
	// RiskP0 — bypasses a real safety/data-integrity gate (anti-loop, oauth,
	// overwrite of live state). These should route through an independent Judge.
	RiskP0 Risk = "P0"
	// RiskP1 — bypasses an important guard whose misuse is recoverable but costly.
	RiskP1 Risk = "P1"
	// RiskP2 — skips an interactive confirmation ("are you sure?").
	RiskP2 Risk = "P2"
	// RiskP3 — cosmetic / low-impact.
	RiskP3 Risk = "P3"
)

// NeedsIndependentJudge reports whether a bypass at this risk level warrants an
// independent classifier (not just deterministic reason-quality checks).
func (r Risk) NeedsIndependentJudge() bool { return r == RiskP0 || r == RiskP1 }

// Guard describes a single override surface: which tool, which flag, what gate
// it bypasses, and how risky that is. A zero Judge means [DefaultJudge].
type Guard struct {
	Tool  string // e.g. "agm send-compact"
	Flag  string // e.g. "--force"
	Gate  string // what is bypassed, in human terms (used in the audit + judge)
	Risk  Risk
	Judge Judge // nil → DefaultJudge

	// auditSink is an override for the audit destination, used by tests.
	// nil → the default JSONL file under the state dir.
	auditSink func(auditEntry)
}

// DeniedError is returned when an override is refused. It carries positive
// guidance (per the repo's enforcement principle: explain the right way and the
// why, never a bare "you can't").
type DeniedError struct {
	Tool        string
	Flag        string
	Gate        string
	Reason      string
	Explanation string // why the reason was insufficient, from the Judge
}

func (e *DeniedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "override refused: %s %s bypasses %s.\n", e.Tool, e.Flag, e.Gate)
	if strings.TrimSpace(e.Reason) == "" {
		fmt.Fprintf(&b, "This bypass requires a justification. The right way:\n")
		fmt.Fprintf(&b, "  %s %s --reason \"<why this bypass is warranted right now>\"\n", e.Tool, e.Flag)
	} else {
		fmt.Fprintf(&b, "The justification you gave was rejected: %s\n", e.Explanation)
		fmt.Fprintf(&b, "Provide a specific, honest reason (what is unusual about this case), e.g.:\n")
		fmt.Fprintf(&b, "  %s %s --reason \"<concrete situation forcing the bypass>\"\n", e.Tool, e.Flag)
	}
	fmt.Fprintf(&b, "Because: %s. If the gate is wrong, fix the gate — do not route around it.", e.Gate)
	return b.String()
}

// Require is the one-line call site: build a Guard's verdict for `reason`,
// record it to the audit log, and return a non-nil error if the bypass is
// refused. On nil error the caller may proceed with the override.
//
// It never executes the underlying operation — the caller does that only if
// Require returns nil.
func Require(ctx context.Context, g Guard, reason string) error {
	judge := g.Judge
	if judge == nil {
		judge = DefaultJudge{}
	}

	verdict, jerr := judge.Evaluate(ctx, JudgeRequest{
		Tool:   g.Tool,
		Flag:   g.Flag,
		Gate:   g.Gate,
		Risk:   g.Risk,
		Reason: reason,
	})

	entry := auditEntry{
		Tool:    g.Tool,
		Flag:    g.Flag,
		Gate:    g.Gate,
		Risk:    string(g.Risk),
		Reason:  strings.TrimSpace(reason),
		Judge:   judge.Name(),
		Allowed: jerr == nil && verdict.Allow,
		Verdict: verdict.Explanation,
	}
	if jerr != nil {
		entry.Verdict = "judge error: " + jerr.Error()
	}
	g.audit(entry)

	if jerr != nil {
		// Fail closed: if the judge could not render a verdict, refuse the bypass.
		return &DeniedError{
			Tool: g.Tool, Flag: g.Flag, Gate: g.Gate, Reason: reason,
			Explanation: "the override judge could not evaluate the reason: " + jerr.Error(),
		}
	}
	if !verdict.Allow {
		return &DeniedError{
			Tool: g.Tool, Flag: g.Flag, Gate: g.Gate, Reason: reason,
			Explanation: verdict.Explanation,
		}
	}
	return nil
}

func (g Guard) audit(e auditEntry) {
	if g.auditSink != nil {
		g.auditSink(e)
		return
	}
	appendAudit(e)
}
