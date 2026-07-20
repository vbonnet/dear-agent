// Bead-closure reconciliation (ce-2n5j / ce-24f1 flywheel-stall defect).
//
// Before this file, nothing deterministically closed a bead when its worker
// finished. BeadsRoadmap (pkg/vroom/supervisor) commits to never triggering
// state transitions ("owned by the worker that actually claims and executes
// the bead"), and the worker prompt this package renders only asked the
// worker to leave a bead NOTE with a terminal status token — never to run
// `bd close`. A worker that verified its fix was already shipped, or that hit
// a permission block and stopped, left its bead `open` and `ready`; the next
// dispatch tick shelled `bd ready --json` again, saw the same bead, and
// redispatched a fresh worker to re-derive the same answer — forever.
//
// The fix is a deterministic reconcile pass, in Go, that runs every tick
// before candidate selection. It never depends on an LLM worker remembering
// to close its own bead:
//
//   - "work merged" is verified independently via `gh pr list --state
//     merged` — ground truth, not the worker's self-report.
//   - "verified already-shipped / no-op" is read from the mandatory
//     terminal-status note (DONE / DONE_WITH_CONCERNS) the worker prompt
//     already requires, but only when NO merged/open PR exists to explain a
//     real change — the note is corroborating evidence, not the sole trigger
//     for closing.
//   - "genuinely blocked" (an explicit FAILED note, or N consecutive silent
//     exits with no progress) moves the bead to bd's built-in `blocked`
//     status, which bd's `ready` query excludes, so it stops re-entering the
//     dispatch queue until a human clears it.
//
// Reconciliation only ever touches a bead this tool's own ledger shows it
// previously dispatched a worker for — a bead nobody has dispatched yet is
// left alone.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// noProgressStrikeLimit is how many consecutive worker exits with no merged
// PR, no open PR, and no recognized terminal-status note are tolerated before
// the bead is auto-blocked instead of redispatched again. This is the
// deterministic backstop for a worker that exits silently — killed, crashed,
// or simply never wrote the note the prompt asks for — without which a bead
// like that would cycle through workers indefinitely.
const noProgressStrikeLimit = 2

// ledgerEntry tracks one bead's dispatch history across vroom-dispatch-direct
// runs.
type ledgerEntry struct {
	SessionName       string    `json:"session_name"`
	DispatchedAt      time.Time `json:"dispatched_at"`
	NoProgressStrikes int       `json:"no_progress_strikes"`
}

// dispatchLedger is the on-disk record of every bead this tool has dispatched
// a worker for. The tool runs fresh each Orchestrator tick (~90s) with no
// persistent process, so this file is the only way a later run can tell "we
// dispatched a worker for this bead and it's now gone" apart from "this bead
// has never been touched" — bd ready alone cannot make that distinction, and
// making it wrongly would mean either reconciling (and possibly closing) a
// bead nobody ever worked, or never reconciling anything at all.
type dispatchLedger struct {
	Beads map[string]*ledgerEntry `json:"beads"`
}

// loadLedger reads the ledger from path. A missing file is a fresh, empty
// ledger — not an error — since the ledger does not exist until the first
// dispatch.
func loadLedger(path string) (*dispatchLedger, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &dispatchLedger{Beads: map[string]*ledgerEntry{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ledger %s: %w", path, err)
	}
	var l dispatchLedger
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parse ledger %s: %w", path, err)
	}
	if l.Beads == nil {
		l.Beads = map[string]*ledgerEntry{}
	}
	return &l, nil
}

// saveLedger persists the ledger atomically (write-to-temp + rename) so a
// crash mid-write cannot leave a truncated ledger file behind.
func saveLedger(path string, l *dispatchLedger) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ledger: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir ledger dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write ledger: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename ledger: %w", err)
	}
	return nil
}

// recordDispatch records (or refreshes) a successful dispatch for beadID.
// Any accumulated NoProgressStrikes are preserved — a redispatch after a
// strike must not reset the strike count, or the auto-block guard could
// never fire.
func (l *dispatchLedger) recordDispatch(beadID, sessionName string) {
	e, ok := l.Beads[beadID]
	if !ok {
		e = &ledgerEntry{}
		l.Beads[beadID] = e
	}
	e.SessionName = sessionName
	e.DispatchedAt = time.Now().UTC()
}

// recordProgress clears a bead's strike count. Evidence of real progress (an
// open PR) resets the no-progress clock, so a slow-but-working bead is never
// auto-blocked just because earlier attempts stalled.
func (l *dispatchLedger) recordProgress(beadID string) {
	if e, ok := l.Beads[beadID]; ok {
		e.NoProgressStrikes = 0
	}
}

// recordStrike increments a bead's no-progress counter and returns the new
// count.
func (l *dispatchLedger) recordStrike(beadID string) int {
	e, ok := l.Beads[beadID]
	if !ok {
		e = &ledgerEntry{}
		l.Beads[beadID] = e
	}
	e.NoProgressStrikes++
	return e.NoProgressStrikes
}

// clear removes a bead from the ledger — used once reconcile has resolved it
// (closed or blocked), so a bead that reopens later starts with a clean
// dispatch history rather than an inherited strike count.
func (l *dispatchLedger) clear(beadID string) {
	delete(l.Beads, beadID)
}

// terminalStatusTokens are the exactly-one-outcome values the worker prompt
// (renderPrompt) mandates as the first token of a worker's final bead note.
var terminalStatusTokens = map[string]bool{
	"DONE":               true,
	"DONE_WITH_CONCERNS": true,
	"FAILED":             true,
}

// lastTerminalToken scans a bead's notes text from the end and returns the
// most recent standalone line that exactly matches one of the mandated
// terminal-status tokens, or "" if none is present.
//
// bd's notes field has no per-entry boundary marker — `bd note` /
// `bd update --append-notes` just appends text, so notes from different
// workers across different dispatches are one unstructured, newline-joined
// blob. "The last standalone line matching the token vocabulary" is the
// closest deterministic proxy available for "the most recent worker's
// reported outcome" without requiring the worker to run anything beyond the
// note the prompt already asks it to leave.
func lastTerminalToken(notes string) string {
	lines := strings.Split(notes, "\n")
	for _, line := range slices.Backward(lines) {
		token := strings.TrimSpace(line)
		if terminalStatusTokens[token] {
			return token
		}
	}
	return ""
}

// beadDetail mirrors the subset of `bd show <id> --json` fields the
// reconciler consumes.
type beadDetail struct {
	ID    string `json:"id"`
	Notes string `json:"notes"`
}

// queryBeadNotes runs `bd --db <db> show <id> --json` and returns the bead's
// accumulated notes text. Package var for test stubbing.
var queryBeadNotes = func(ctx context.Context, db, id string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "--db", db, "show", id, "--json")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("bd show %s: %w", id, err)
	}
	var details []beadDetail
	if err := json.Unmarshal(out, &details); err != nil {
		return "", fmt.Errorf("parse bd show %s: %w", id, err)
	}
	if len(details) == 0 {
		return "", fmt.Errorf("bd show %s: no such issue", id)
	}
	return details[0].Notes, nil
}

// closeBead deterministically closes a bead. Package var for test stubbing.
var closeBead = func(ctx context.Context, db, id, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "--db", db, "close", id, "--reason", reason)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd close %s: %w\n%s", id, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// blockBead deterministically moves a bead into bd's built-in `blocked`
// status (category "wip", excluded from `bd ready`) so it stops re-entering
// dispatch until a human clears it. Package var for test stubbing.
var blockBead = func(ctx context.Context, db, id, note string) error {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "--db", db, "update", id, "--status", "blocked", "--append-notes", note)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("bd update %s --status blocked: %w\n%s", id, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// reconcileOutcome is the deterministic disposition reconcileBead computes
// for one bead.
type reconcileOutcome int

const (
	// reconcileNone leaves the bead untouched (still genuinely in flight).
	reconcileNone reconcileOutcome = iota
	// reconcileCloseDone closes the bead: a merged PR proves the work shipped.
	reconcileCloseDone
	// reconcileCloseNoOp closes the bead: the worker's terminal note reports
	// DONE/DONE_WITH_CONCERNS with no merged PR to point to (e.g. "already
	// satisfied, nothing to ship").
	reconcileCloseNoOp
	// reconcileBlockFailed moves the bead to blocked: the worker explicitly
	// reported FAILED.
	reconcileBlockFailed
	// reconcileBlockNoProgress moves the bead to blocked: repeated silent
	// exits with no evidence of progress hit the strike limit.
	reconcileBlockNoProgress
	// reconcileStrike leaves the bead open but records a no-progress strike.
	reconcileStrike
)

// reconcileAction is the decision reconcileBead returns for one bead.
type reconcileAction struct {
	Outcome reconcileOutcome
	Reason  string
}

// reconcileBead decides the deterministic terminal outcome for one ready bead
// whose ledger entry shows we previously dispatched a worker and which
// currently has no live worker session. A merged PR is ground truth and
// always wins over the note; the note is only consulted once no PR (open or
// merged) explains the bead's state.
func reconcileBead(hasOpenPR, hasMergedPR bool, mergedEvidence, notes string, priorStrikes int) reconcileAction {
	if hasOpenPR {
		return reconcileAction{Outcome: reconcileNone, Reason: "progress: open PR in flight"}
	}
	if hasMergedPR {
		return reconcileAction{Outcome: reconcileCloseDone, Reason: mergedEvidence}
	}

	token := lastTerminalToken(notes)
	switch token {
	case "DONE", "DONE_WITH_CONCERNS":
		return reconcileAction{
			Outcome: reconcileCloseNoOp,
			Reason:  fmt.Sprintf("no-op: worker reported %s with no merged PR (already satisfied)", token),
		}
	case "FAILED":
		return reconcileAction{
			Outcome: reconcileBlockFailed,
			Reason:  "worker reported FAILED — needs human review before further dispatch",
		}
	default:
		strikes := priorStrikes + 1
		if strikes >= noProgressStrikeLimit {
			return reconcileAction{
				Outcome: reconcileBlockNoProgress,
				Reason: fmt.Sprintf("auto-blocked: %d consecutive worker exits with no merged PR, no open PR, "+
					"and no recognized terminal-status note — needs human review", strikes),
			}
		}
		return reconcileAction{Outcome: reconcileStrike, Reason: "worker exited with no evidence of progress"}
	}
}

// findMergedEvidence reports whether a merged PR mentions id (by branch name
// or title) and, if so, a human-readable citation for the close reason.
func findMergedEvidence(id string, mergedPRs []pullRequest) (evidence string, ok bool) {
	for _, pr := range mergedPRs {
		if mentionsID(pr.HeadRefName, id) || mentionsID(pr.Title, id) {
			evidence = fmt.Sprintf("merged: PR #%d %q", pr.Number, pr.Title)
			if pr.MergedAt != "" {
				evidence += " (merged " + pr.MergedAt + ")"
			}
			return evidence, true
		}
	}
	return "", false
}

// reconcile scans every ready bead the ledger shows was previously dispatched
// to a worker. For each one with no live worker session, it applies
// reconcileBead's decision: closing done/no-op beads, blocking failed/
// no-progress beads, or recording a strike. It returns the set of bead IDs it
// resolved (closed or blocked) this run, so the caller can exclude them from
// this tick's candidate selection — bd ready was queried before reconcile
// ran, so its in-memory snapshot is stale for anything reconcile just closed
// or blocked.
//
// In dry-run mode no bd mutation is made (no close, no block, no strike
// persisted), but the same beads are still reported as "would resolve" so
// dry-run output reflects what a real run would do.
func reconcile(ctx context.Context, db string, beads []bead, live map[string]bool, openPRs, mergedPRs []pullRequest, ledger *dispatchLedger, dryRun bool, out, errOut io.Writer) map[string]bool {
	resolved := map[string]bool{}
	for _, b := range beads {
		if b.ID == "" || live[normalizeSessionID(b.ID)] {
			continue
		}
		entry, dispatchedBefore := ledger.Beads[b.ID]
		if !dispatchedBefore {
			continue // never dispatched by this tool — nothing to reconcile
		}
		if reconcileOneBead(ctx, db, b.ID, entry, openPRs, mergedPRs, ledger, dryRun, out, errOut) {
			resolved[b.ID] = true
		}
	}
	return resolved
}

// reconcileOneBead determines and applies the outcome for a single bead,
// returning true if the bead was resolved (closed or blocked) this run.
func reconcileOneBead(ctx context.Context, db, id string, entry *ledgerEntry, openPRs, mergedPRs []pullRequest, ledger *dispatchLedger, dryRun bool, out, errOut io.Writer) bool {
	hasOpenPR := inFlightInPR(id, openPRs)
	if hasOpenPR {
		ledger.recordProgress(id)
		return false
	}

	mergedEvidence, hasMergedPR := findMergedEvidence(id, mergedPRs)

	var notes string
	if !hasMergedPR {
		var err error
		notes, err = queryBeadNotes(ctx, db, id)
		if err != nil {
			fmt.Fprintf(errOut, "vroom-dispatch-direct: reconcile %s: read notes: %v\n", id, err)
			return false // fail closed: do not guess an outcome without notes
		}
	}

	action := reconcileBead(hasOpenPR, hasMergedPR, mergedEvidence, notes, entry.NoProgressStrikes)
	return applyReconcileAction(ctx, db, id, action, ledger, dryRun, out, errOut)
}

// applyReconcileAction executes the decision reconcileBead computed: close,
// block, record a strike, or (reconcileNone) do nothing. Returns true if the
// bead was resolved (closed or blocked, or would be under dry-run).
func applyReconcileAction(ctx context.Context, db, id string, action reconcileAction, ledger *dispatchLedger, dryRun bool, out, errOut io.Writer) bool {
	switch action.Outcome {
	case reconcileCloseDone, reconcileCloseNoOp:
		return applyCloseOrBlock(ctx, db, id, action.Reason, "close", closeBead, ledger, dryRun, out, errOut)
	case reconcileBlockFailed, reconcileBlockNoProgress:
		return applyCloseOrBlock(ctx, db, id, action.Reason, "block", blockBead, ledger, dryRun, out, errOut)
	case reconcileStrike:
		if dryRun {
			fmt.Fprintf(out, "reconciled %s: would strike (%s)\n", id, action.Reason)
			return false
		}
		n := ledger.recordStrike(id)
		fmt.Fprintf(out, "reconciled %s: strike %d/%d (%s)\n", id, n, noProgressStrikeLimit, action.Reason)
		return false
	case reconcileNone:
		// Progress observed elsewhere (open PR) — nothing to do here.
		return false
	}
	return false
}

// applyCloseOrBlock runs the given bd mutation (closeBead or blockBead) for
// id unless dryRun is set, reports the outcome, and on success clears the
// bead's ledger entry so a later reopen starts with a clean dispatch history.
func applyCloseOrBlock(ctx context.Context, db, id, reason, verb string, mutate func(context.Context, string, string, string) error, ledger *dispatchLedger, dryRun bool, out, errOut io.Writer) bool {
	if dryRun {
		fmt.Fprintf(out, "reconciled %s: would %s (%s)\n", id, verb, reason)
		return true
	}
	if err := mutate(ctx, db, id, reason); err != nil {
		fmt.Fprintf(errOut, "vroom-dispatch-direct: reconcile %s %s: %v\n", verb, id, err)
		return false
	}
	ledger.clear(id)
	pastTense := verb + "d"
	if verb == "block" {
		pastTense = "blocked"
	}
	fmt.Fprintf(out, "reconciled %s: %s (%s)\n", id, pastTense, reason)
	return true
}

// excludeReconciled filters beads down to those reconcile did not just
// resolve (close or block) this run, so a bead cannot be reconciled and
// redispatched in the same tick from a stale in-memory bd-ready snapshot.
func excludeReconciled(beads []bead, resolved map[string]bool) []bead {
	if len(resolved) == 0 {
		return beads
	}
	out := make([]bead, 0, len(beads))
	for _, b := range beads {
		if !resolved[b.ID] {
			out = append(out, b)
		}
	}
	return out
}
