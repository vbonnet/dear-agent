package escalation

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ReadEvents parses an append-only JSONL event log into a slice, in file order.
// Blank lines are skipped; a malformed line is an error (the log is machine
// written, so a parse failure means corruption worth surfacing, not skipping).
func ReadEvents(r io.Reader) ([]EscalationEvent, error) {
	var out []EscalationEvent
	sc := bufio.NewScanner(r)
	// Escalation questions/answers can be long; raise the line cap from 64KiB.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	line := 0
	for sc.Scan() {
		line++
		b := sc.Bytes()
		if len(trimSpaceBytes(b)) == 0 {
			continue
		}
		var ev EscalationEvent
		if err := json.Unmarshal(b, &ev); err != nil {
			return nil, fmt.Errorf("escalation: parse event log line %d: %w", line, err)
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("escalation: read event log: %w", err)
	}
	return out, nil
}

// trimSpaceBytes reports the byte slice with leading/trailing ASCII space
// trimmed, avoiding a string allocation just to test for emptiness.
func trimSpaceBytes(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && asciiSpace(b[i]) {
		i++
	}
	for j > i && asciiSpace(b[j-1]) {
		j--
	}
	return b[i:j]
}

func asciiSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\r' || c == '\n' }

// WriteEvents writes events as JSONL (one object per line) in order.
func WriteEvents(w io.Writer, events []EscalationEvent) error {
	bw := bufio.NewWriter(w)
	for _, ev := range events {
		b, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("escalation: marshal event %s: %w", ev.EventID, err)
		}
		b = append(b, '\n')
		if _, err := bw.Write(b); err != nil {
			return fmt.Errorf("escalation: write event: %w", err)
		}
	}
	return bw.Flush()
}

// BackfillResult summarises a backfill pass.
type BackfillResult struct {
	// Total is how many events were in the log.
	Total int
	// Candidates is how many answered events were eligible for adjudication.
	Candidates int
	// Updated is how many events got a (newly) non-empty outcome written.
	Updated int
	// Skipped is candidates the adjudicator declined to score (empty outcome) or
	// that errored — left unchanged for a later pass.
	Skipped int
	// ByOutcome counts the outcomes that were written, keyed by Outcome.
	ByOutcome map[Outcome]int
}

// isAdjudicationCandidate reports whether an event is an answered escalation the
// adjudicator should score: a terminal answer with answer text. Auto-resolved
// events are excluded — their answer is the deterministic classifier's own
// canned confirmation, not a judgment call worth a model pass.
func isAdjudicationCandidate(ev EscalationEvent) bool {
	return ev.Phase == PhaseAnswered && ev.Answer != ""
}

// Backfill scores every answered event lacking an Outcome (or all answered
// events when force is true) with adj, returning a new slice with the
// outcome/misalignment columns filled in. Non-candidate events pass through
// untouched. An adjudicator that returns an empty Outcome or an error leaves
// that event unchanged (counted in Skipped) — Backfill never invents a verdict.
//
// The input slice is not mutated; the returned slice is a fresh copy safe to
// write back over the log.
func Backfill(ctx context.Context, events []EscalationEvent, adj Adjudicator, force bool) ([]EscalationEvent, BackfillResult, error) {
	res := BackfillResult{Total: len(events), ByOutcome: map[Outcome]int{}}
	out := make([]EscalationEvent, len(events))
	copy(out, events)

	for i := range out {
		if err := ctx.Err(); err != nil {
			return out, res, err
		}
		ev := &out[i]
		if !isAdjudicationCandidate(*ev) {
			continue
		}
		if ev.Outcome != "" && !force {
			continue // already adjudicated; idempotent unless --force
		}
		res.Candidates++

		verdict, err := adj.Adjudicate(ctx, AdjudicationRequest{
			Kind:           ev.Kind,
			Question:       ev.Question,
			Context:        "", // event log does not retain raise-time context
			Answer:         ev.Answer,
			AnsweredByRole: ev.AnsweredByRole,
			Topic:          ev.Topic,
		})
		if err != nil || verdict.Outcome == "" {
			res.Skipped++
			continue
		}
		ev.Outcome = string(verdict.Outcome)
		ev.Misalignment = verdict.Misalignment
		res.Updated++
		res.ByOutcome[verdict.Outcome]++
	}
	return out, res, nil
}

// BackfillFile reads the JSONL log at path, runs Backfill, and atomically
// rewrites the file (temp + rename) so a concurrent reader never sees a
// half-written log. It is an offline maintenance pass: unlike the live append
// path it rewrites the whole file, so it should not run while escalations are
// actively being logged. A path that does not exist yet is treated as an empty
// log (nothing to do).
func BackfillFile(ctx context.Context, path string, adj Adjudicator, force bool) (BackfillResult, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return BackfillResult{ByOutcome: map[Outcome]int{}}, nil
	}
	if err != nil {
		return BackfillResult{}, fmt.Errorf("escalation: open log %q: %w", path, err)
	}
	events, rerr := ReadEvents(f)
	f.Close()
	if rerr != nil {
		return BackfillResult{}, rerr
	}

	updated, res, berr := Backfill(ctx, events, adj, force)
	if berr != nil {
		return res, berr
	}
	if res.Updated == 0 {
		return res, nil // nothing changed; do not churn the file
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return res, fmt.Errorf("escalation: temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if werr := WriteEvents(tmp, updated); werr != nil {
		tmp.Close()
		return res, werr
	}
	if err := tmp.Close(); err != nil {
		return res, fmt.Errorf("escalation: close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return res, fmt.Errorf("escalation: rename: %w", err)
	}
	return res, nil
}
