package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// readPace fetches burn-rate readings and folds them onto the snapshot.
//
// Pace lives on CodexBar's `usage` surface, not on `dashboard`, so it takes
// a second invocation. The split is worth it: remaining percent alone
// cannot tell a healthy 50% from a 50% that will be gone in an hour, and
// that difference is the whole point of a runaway-cost guardrail.
//
// The read is best-effort. A failure leaves the snapshot's windows intact
// and simply omits pace, because a missing burn-rate reading must never
// downgrade a usable quota reading.
func (r CodexBarReader) readPace(ctx context.Context, snapshot *Snapshot, command string, runner CommandRunner, timeout time.Duration) {
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// No --provider argument: CodexBar then honors the operator's own
	// enabled-provider toggles instead of probing every vendor it knows.
	out, err := runner.Output(callCtx, command, "usage", "--format", "json", "--no-credits")
	if err != nil {
		return
	}
	byID, err := parseCodexBarPace(out)
	if err != nil || len(byID) == 0 {
		return
	}
	for i := range snapshot.Providers {
		if pace, ok := byID[strings.ToLower(snapshot.Providers[i].SourceID)]; ok {
			snapshot.Providers[i].Pace = pace
		}
	}
}

// codexBarUsageEntry is a deliberately narrow projection of CodexBar's
// usage payload.
//
// The usage surface has no --identity redacted mode: its JSON carries
// account emails. Declaring only these fields means encoding/json drops
// the identity while decoding, so an account address is never
// materialized into this process's memory rather than merely being
// ignored after the fact.
type codexBarUsageEntry struct {
	Provider string `json:"provider"`
	Pace     map[string]struct {
		Stage               string  `json:"stage"`
		DeltaPercent        float64 `json:"deltaPercent"`
		ExpectedUsedPercent float64 `json:"expectedUsedPercent"`
		WillLastToReset     bool    `json:"willLastToReset"`
		ETASeconds          float64 `json:"etaSeconds"`
		Summary             string  `json:"summary"`
	} `json:"pace"`
}

// ParseCodexBarPace extracts burn-rate readings keyed by CodexBar provider
// id. A provider reporting pace for more than one window is reduced to its
// most alarming rung, because the guardrail cares about the budget that
// runs out first.
func ParseCodexBarPace(data []byte) (map[string]*Pace, error) {
	return parseCodexBarPace(data)
}

func parseCodexBarPace(data []byte) (map[string]*Pace, error) {
	var entries []codexBarUsageEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("quota: parse codexbar usage json: %w", err)
	}
	out := make(map[string]*Pace, len(entries))
	for _, entry := range entries {
		if entry.Provider == "" || len(entry.Pace) == 0 {
			continue
		}
		var worst *Pace
		for _, rung := range entry.Pace {
			candidate := &Pace{
				Stage:               rung.Stage,
				DeltaPercent:        rung.DeltaPercent,
				ExpectedUsedPercent: rung.ExpectedUsedPercent,
				WillLastToReset:     rung.WillLastToReset,
				ExhaustsIn:          time.Duration(rung.ETASeconds * float64(time.Second)),
				Summary:             rung.Summary,
			}
			worst = morePressing(worst, candidate)
		}
		if worst != nil {
			out[strings.ToLower(entry.Provider)] = worst
		}
	}
	return out, nil
}

// morePressing picks the pace reading a guardrail should act on: a window
// that will not survive to its reset outranks one that will, and among
// equals the larger overspend wins.
func morePressing(current, candidate *Pace) *Pace {
	if current == nil {
		return candidate
	}
	if candidate == nil {
		return current
	}
	switch {
	case current.Overspending() != candidate.Overspending():
		if candidate.Overspending() {
			return candidate
		}
		return current
	case candidate.DeltaPercent > current.DeltaPercent:
		return candidate
	default:
		return current
	}
}
