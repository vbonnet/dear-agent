package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLastTerminalToken(t *testing.T) {
	tests := []struct {
		name  string
		notes string
		want  string
	}{
		{"empty", "", ""},
		{"no token", "just some prose about progress", ""},
		{"done at end", "did some work\nDONE\n\nAll gates verified.", "DONE"},
		{"done with concerns", "shipped it\nDONE_WITH_CONCERNS\n\nsome risk noted", "DONE_WITH_CONCERNS"},
		{"failed", "hit a wall\nFAILED\n\nblocked by permission classifier", "FAILED"},
		{"token embedded mid-sentence does not match", "this task is DONE for today, more tomorrow", ""},
		{"picks the LAST standalone token across multiple notes", "attempt 1\nFAILED\n\nattempt 2\nDONE\n\nall gates verified", "DONE"},
		{"trailing whitespace on the token line still matches", "DONE   \n\nrest", "DONE"},
		{"real ce-2n5j-shaped notes", "Pushed commit adc0948b56 to PR #830; CI restarted.\nDONE\n\nAll three delivery gates verified:\n\n1. MERGED — ...", "DONE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastTerminalToken(tt.notes); got != tt.want {
				t.Errorf("lastTerminalToken(%q) = %q, want %q", tt.notes, got, tt.want)
			}
		})
	}
}

func TestReconcileBead(t *testing.T) {
	tests := []struct {
		name         string
		hasOpenPR    bool
		hasMergedPR  bool
		notes        string
		priorStrikes int
		wantOutcome  reconcileOutcome
	}{
		{"open PR in flight wins over everything", true, true, "FAILED", 5, reconcileNone},
		{"merged PR closes as done regardless of note", false, true, "FAILED", 0, reconcileCloseDone},
		{"merged PR closes as done with no note at all", false, true, "", 0, reconcileCloseDone},
		{"no PR, DONE note closes as no-op", false, false, "DONE\n\ndetails", 0, reconcileCloseNoOp},
		{"no PR, DONE_WITH_CONCERNS note closes as no-op", false, false, "DONE_WITH_CONCERNS\n\ndetails", 0, reconcileCloseNoOp},
		{"no PR, FAILED note blocks", false, false, "FAILED\n\nblocked by permission classifier", 0, reconcileBlockFailed},
		{"no PR, no note, first exit strikes", false, false, "", 0, reconcileStrike},
		{"no PR, no note, strike limit reached blocks", false, false, "", noProgressStrikeLimit - 1, reconcileBlockNoProgress},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reconcileBead(tt.hasOpenPR, tt.hasMergedPR, "merged: PR #1", tt.notes, tt.priorStrikes)
			if got.Outcome != tt.wantOutcome {
				t.Errorf("reconcileBead(...) outcome = %v, want %v (reason: %s)", got.Outcome, tt.wantOutcome, got.Reason)
			}
		})
	}
}

func TestFindMergedEvidence(t *testing.T) {
	prs := []pullRequest{
		{Number: 830, HeadRefName: "fix/ce-2n5j-canonical-supervisor-harness", Title: "fix(vroom): recover supervisors on canonical harnesses", MergedAt: "2026-07-04T06:46:04Z"},
	}
	evidence, ok := findMergedEvidence("ce-2n5j", prs)
	if !ok {
		t.Fatal("expected ce-2n5j to match PR #830 by branch name")
	}
	if !strings.Contains(evidence, "#830") || !strings.Contains(evidence, "2026-07-04T06:46:04Z") {
		t.Errorf("evidence should cite PR number and merge date, got %q", evidence)
	}
	if _, ok := findMergedEvidence("ce-nope", prs); ok {
		t.Error("ce-nope should not match any merged PR")
	}
}

// TestDispatchLedgerRoundTrip verifies load/save preserve entries, and that
// recordDispatch after a strike does not reset the strike count (a redispatch
// following a no-progress exit must not give the guard amnesia).
func TestDispatchLedgerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")

	l, err := loadLedger(path)
	if err != nil {
		t.Fatalf("loadLedger on missing file: %v", err)
	}
	if len(l.Beads) != 0 {
		t.Fatalf("expected empty ledger, got %+v", l.Beads)
	}

	l.recordDispatch("ce-test", "worker-ce-test")
	if n := l.recordStrike("ce-test"); n != 1 {
		t.Fatalf("recordStrike = %d, want 1", n)
	}
	l.recordDispatch("ce-test", "worker-ce-test") // redispatch after the strike
	if got := l.Beads["ce-test"].NoProgressStrikes; got != 1 {
		t.Errorf("redispatch must preserve strikes, got %d, want 1", got)
	}

	if err := saveLedger(path, l); err != nil {
		t.Fatalf("saveLedger: %v", err)
	}

	l2, err := loadLedger(path)
	if err != nil {
		t.Fatalf("loadLedger after save: %v", err)
	}
	e, ok := l2.Beads["ce-test"]
	if !ok {
		t.Fatal("expected ce-test to survive round trip")
	}
	if e.SessionName != "worker-ce-test" || e.NoProgressStrikes != 1 {
		t.Errorf("round-tripped entry = %+v, want session worker-ce-test strikes 1", e)
	}

	l2.recordProgress("ce-test")
	if l2.Beads["ce-test"].NoProgressStrikes != 0 {
		t.Error("recordProgress must reset strikes to 0")
	}

	l2.clear("ce-test")
	if _, ok := l2.Beads["ce-test"]; ok {
		t.Error("clear should remove the bead from the ledger")
	}
}

func TestLoadLedgerCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadLedger(path); err == nil {
		t.Error("expected an error loading a corrupt ledger file")
	}
}

// TestReconcile_NeverTouchesBeadWithNoLedgerEntry proves reconcile leaves a
// bead alone when this tool never dispatched a worker for it — a bead must
// not be closed or blocked just because it happens to appear in bd ready with
// no live worker (e.g. a freshly created bead nobody has picked up yet).
func TestReconcile_NeverTouchesBeadWithNoLedgerEntry(t *testing.T) {
	origClose, origBlock := closeBead, blockBead
	defer func() { closeBead, blockBead = origClose, origBlock }()
	closeBead = func(ctx context.Context, db, id, reason string) error {
		t.Fatalf("closeBead must not be called for %s (never dispatched)", id)
		return nil
	}
	blockBead = func(ctx context.Context, db, id, note string) error {
		t.Fatalf("blockBead must not be called for %s (never dispatched)", id)
		return nil
	}

	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{}}
	beads := []bead{{ID: "ce-fresh", Title: "never dispatched"}}
	var out, errOut bytes.Buffer
	resolved := reconcile(context.Background(), "db", beads, nil, nil, nil, ledger, false, &out, &errOut)
	if len(resolved) != 0 {
		t.Errorf("expected no beads resolved, got %v", resolved)
	}
}

// TestReconcile_LiveWorkerSkipped proves reconcile leaves a bead with an
// active worker session alone even if it has a ledger entry — it is still
// genuinely in flight.
func TestReconcile_LiveWorkerSkipped(t *testing.T) {
	origClose := closeBead
	defer func() { closeBead = origClose }()
	closeBead = func(ctx context.Context, db, id, reason string) error {
		t.Fatalf("closeBead must not be called for a live worker's bead")
		return nil
	}

	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{"ce-live": {SessionName: "worker-ce-live"}}}
	beads := []bead{{ID: "ce-live", Title: "still running"}}
	live := map[string]bool{"ce-live": true}
	var out, errOut bytes.Buffer
	resolved := reconcile(context.Background(), "db", beads, live, nil, nil, ledger, false, &out, &errOut)
	if len(resolved) != 0 {
		t.Errorf("expected no beads resolved for a live worker, got %v", resolved)
	}
}

// TestReconcile_MergedPRClosesRegardlessOfNotes mirrors the real ce-2n5j
// evidence: a merged PR mentioning the bead closes it even though nothing
// about the note is consulted for that decision (ground truth wins).
func TestReconcile_MergedPRClosesRegardlessOfNotes(t *testing.T) {
	origClose, origNotes := closeBead, queryBeadNotes
	defer func() { closeBead, queryBeadNotes = origClose, origNotes }()

	var closedID, closedReason string
	closeBead = func(ctx context.Context, db, id, reason string) error {
		closedID, closedReason = id, reason
		return nil
	}
	queryBeadNotes = func(ctx context.Context, db, id string) (string, error) {
		t.Fatal("queryBeadNotes must not be called when a merged PR already settles the outcome")
		return "", nil
	}

	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{"ce-2n5j": {SessionName: "worker-ce-2n5j"}}}
	beads := []bead{{ID: "ce-2n5j", Title: "canonical harness fix"}}
	merged := []pullRequest{{Number: 830, HeadRefName: "fix/ce-2n5j-canonical-supervisor-harness", Title: "fix(vroom)", MergedAt: "2026-07-04T06:46:04Z"}}

	var out, errOut bytes.Buffer
	resolved := reconcile(context.Background(), "db", beads, nil, nil, merged, ledger, false, &out, &errOut)

	if !resolved["ce-2n5j"] {
		t.Fatalf("expected ce-2n5j to be resolved, got %v", resolved)
	}
	if closedID != "ce-2n5j" {
		t.Errorf("closeBead called with id %q, want ce-2n5j", closedID)
	}
	if !strings.Contains(closedReason, "#830") {
		t.Errorf("close reason should cite PR #830, got %q", closedReason)
	}
	if _, stillTracked := ledger.Beads["ce-2n5j"]; stillTracked {
		t.Error("ledger entry should be cleared after closing")
	}
}

// TestReconcile_NoOpDoneNoteClosesWithoutPR mirrors "verified already
// shipped": no PR was ever opened, but the worker's terminal note reports
// DONE — the bead must close instead of being redispatched forever.
func TestReconcile_NoOpDoneNoteClosesWithoutPR(t *testing.T) {
	origClose, origNotes := closeBead, queryBeadNotes
	defer func() { closeBead, queryBeadNotes = origClose, origNotes }()

	var closedID string
	closeBead = func(ctx context.Context, db, id, reason string) error {
		closedID = id
		return nil
	}
	queryBeadNotes = func(ctx context.Context, db, id string) (string, error) {
		return "DONE\n\nverified the fix already shipped in a prior PR, nothing to do", nil
	}

	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{"ce-noop": {}}}
	beads := []bead{{ID: "ce-noop", Title: "already satisfied"}}
	var out, errOut bytes.Buffer
	resolved := reconcile(context.Background(), "db", beads, nil, nil, nil, ledger, false, &out, &errOut)

	if !resolved["ce-noop"] || closedID != "ce-noop" {
		t.Fatalf("expected ce-noop to close as no-op, resolved=%v closedID=%q", resolved, closedID)
	}
}

// TestReconcile_FailedNoteBlocksNotCloses mirrors the real ce-24f1 evidence:
// the worker hit a permission block and reported FAILED — the bead must move
// to blocked, not close, and must not be redispatched.
func TestReconcile_FailedNoteBlocksNotCloses(t *testing.T) {
	origClose, origBlock, origNotes := closeBead, blockBead, queryBeadNotes
	defer func() { closeBead, blockBead, queryBeadNotes = origClose, origBlock, origNotes }()

	closeBead = func(ctx context.Context, db, id, reason string) error {
		t.Fatal("closeBead must not be called for a FAILED outcome")
		return nil
	}
	var blockedID, blockedNote string
	blockBead = func(ctx context.Context, db, id, note string) error {
		blockedID, blockedNote = id, note
		return nil
	}
	queryBeadNotes = func(ctx context.Context, db, id string) (string, error) {
		return "FAILED\n\nblocked by the permission classifier, needs a human", nil
	}

	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{"ce-24f1": {}}}
	beads := []bead{{ID: "ce-24f1", Title: "gobin wipe guard"}}
	var out, errOut bytes.Buffer
	resolved := reconcile(context.Background(), "db", beads, nil, nil, nil, ledger, false, &out, &errOut)

	if !resolved["ce-24f1"] || blockedID != "ce-24f1" {
		t.Fatalf("expected ce-24f1 to be blocked, resolved=%v blockedID=%q", resolved, blockedID)
	}
	if !strings.Contains(blockedNote, "FAILED") {
		t.Errorf("block note should mention the FAILED report, got %q", blockedNote)
	}
}

// TestReconcile_SilentExitStrikesThenBlocks proves the anti-re-dispatch
// guard: a bead with no PR and no recognizable note strikes on early exits
// and only blocks once it reaches the strike limit — this is what stops a
// bead from cycling through workers indefinitely when nothing about its
// outcome is ever legible.
func TestReconcile_SilentExitStrikesThenBlocks(t *testing.T) {
	origBlock, origNotes := blockBead, queryBeadNotes
	defer func() { blockBead, queryBeadNotes = origBlock, origNotes }()

	blockCalls := 0
	blockBead = func(ctx context.Context, db, id, note string) error {
		blockCalls++
		return nil
	}
	queryBeadNotes = func(ctx context.Context, db, id string) (string, error) {
		return "some prose with no terminal token", nil
	}

	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{"ce-silent": {}}}
	beads := []bead{{ID: "ce-silent", Title: "silent exit"}}

	// First reconcile: below the strike limit, must NOT block yet.
	var out1, errOut1 bytes.Buffer
	resolved1 := reconcile(context.Background(), "db", beads, nil, nil, nil, ledger, false, &out1, &errOut1)
	if len(resolved1) != 0 {
		t.Fatalf("first strike should not resolve the bead yet, got %v", resolved1)
	}
	if blockCalls != 0 {
		t.Fatalf("blockBead must not be called before the strike limit, called %d times", blockCalls)
	}
	if got := ledger.Beads["ce-silent"].NoProgressStrikes; got != 1 {
		t.Fatalf("expected 1 strike recorded, got %d", got)
	}

	// Second reconcile (simulating a redispatch + another silent exit) hits
	// the strike limit and blocks.
	var out2, errOut2 bytes.Buffer
	resolved2 := reconcile(context.Background(), "db", beads, nil, nil, nil, ledger, false, &out2, &errOut2)
	if !resolved2["ce-silent"] {
		t.Fatalf("expected ce-silent to be blocked at the strike limit, got %v", resolved2)
	}
	if blockCalls != 1 {
		t.Errorf("expected exactly 1 blockBead call, got %d", blockCalls)
	}
}

// TestReconcile_OpenPRResetsStrikesAndSkips proves an open PR is treated as
// real progress: it must not be reconciled away, and it must reset any
// accumulated no-progress strikes.
func TestReconcile_OpenPRResetsStrikesAndSkips(t *testing.T) {
	origClose := closeBead
	defer func() { closeBead = origClose }()
	closeBead = func(ctx context.Context, db, id, reason string) error {
		t.Fatal("closeBead must not be called while a PR is still open")
		return nil
	}

	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{"ce-inflight": {NoProgressStrikes: 1}}}
	beads := []bead{{ID: "ce-inflight", Title: "still cooking"}}
	open := []pullRequest{{Number: 5, HeadRefName: "feat/ce-inflight-thing", Title: "wip"}}

	var out, errOut bytes.Buffer
	resolved := reconcile(context.Background(), "db", beads, nil, open, nil, ledger, false, &out, &errOut)
	if len(resolved) != 0 {
		t.Errorf("open-PR bead must not be resolved, got %v", resolved)
	}
	if ledger.Beads["ce-inflight"].NoProgressStrikes != 0 {
		t.Error("an open PR must reset the strike counter")
	}
}

// TestReconcile_DryRunMakesNoMutations proves dry-run reports what would
// happen without calling closeBead/blockBead or persisting a strike.
func TestReconcile_DryRunMakesNoMutations(t *testing.T) {
	origClose, origBlock, origNotes := closeBead, blockBead, queryBeadNotes
	defer func() { closeBead, blockBead, queryBeadNotes = origClose, origBlock, origNotes }()

	closeBead = func(ctx context.Context, db, id, reason string) error {
		t.Fatal("dry-run must not call closeBead")
		return nil
	}
	blockBead = func(ctx context.Context, db, id, note string) error {
		t.Fatal("dry-run must not call blockBead")
		return nil
	}
	queryBeadNotes = func(ctx context.Context, db, id string) (string, error) {
		return "DONE\n\nalready shipped", nil
	}

	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{"ce-dry": {}}}
	beads := []bead{{ID: "ce-dry", Title: "dry run check"}}
	var out, errOut bytes.Buffer
	resolved := reconcile(context.Background(), "db", beads, nil, nil, nil, ledger, true, &out, &errOut)

	if !resolved["ce-dry"] {
		t.Fatalf("dry-run should still report the bead as would-resolve, got %v", resolved)
	}
	if _, stillTracked := ledger.Beads["ce-dry"]; !stillTracked {
		t.Error("dry-run must not mutate the ledger (entry should still be present)")
	}
	if !strings.Contains(out.String(), "would close") {
		t.Errorf("dry-run output should say 'would close', got %q", out.String())
	}
}

func TestExcludeReconciled(t *testing.T) {
	beads := []bead{{ID: "ce-1"}, {ID: "ce-2"}, {ID: "ce-3"}}
	resolved := map[string]bool{"ce-2": true}
	got := excludeReconciled(beads, resolved)
	if len(got) != 2 {
		t.Fatalf("expected 2 beads after exclusion, got %d: %+v", len(got), got)
	}
	for _, b := range got {
		if b.ID == "ce-2" {
			t.Error("ce-2 should have been excluded")
		}
	}
	// No resolutions: must return the input unchanged (same beads, any order).
	if got := excludeReconciled(beads, nil); len(got) != 3 {
		t.Errorf("expected all 3 beads with no resolutions, got %d", len(got))
	}
}

// TestDispatchCandidates_RecordsLedgerOnSuccess proves a successful dispatch
// is recorded in the ledger — without this, reconcile could never later tell
// "we dispatched a worker for this bead" apart from "nobody ever touched it".
func TestDispatchCandidates_RecordsLedgerOnSuccess(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error { return nil }
	sendPrompt = func(ctx context.Context, name, prompt string) error { return nil }

	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{}}
	candidates := []bead{{ID: "ce-new", Title: "fresh work", Priority: 1}}
	var out, errOut bytes.Buffer
	got := dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", false, &out, &errOut, ledger)

	if got != 1 {
		t.Fatalf("dispatched = %d, want 1", got)
	}
	e, ok := ledger.Beads["ce-new"]
	if !ok {
		t.Fatal("expected ce-new to be recorded in the ledger after a successful dispatch")
	}
	if e.SessionName != "worker-ce-new" {
		t.Errorf("ledger session name = %q, want worker-ce-new", e.SessionName)
	}
}

// TestDispatchCandidates_DryRunDoesNotTouchLedger proves dry-run dispatch
// never writes ledger entries — nothing was actually spawned.
func TestDispatchCandidates_DryRunDoesNotTouchLedger(t *testing.T) {
	ledger := &dispatchLedger{Beads: map[string]*ledgerEntry{}}
	candidates := []bead{{ID: "ce-new", Title: "fresh work", Priority: 1}}
	var out, errOut bytes.Buffer
	dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", true, &out, &errOut, ledger)

	if len(ledger.Beads) != 0 {
		t.Errorf("dry-run must not write ledger entries, got %+v", ledger.Beads)
	}
}

// TestDispatchCandidates_NilLedgerIsSafe proves a nil ledger (the default for
// existing call sites / tests that don't care about reconciliation) never
// panics.
func TestDispatchCandidates_NilLedgerIsSafe(t *testing.T) {
	origSpawn, origSend := spawnSession, sendPrompt
	defer func() { spawnSession, sendPrompt = origSpawn, origSend }()
	spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error { return nil }
	sendPrompt = func(ctx context.Context, name, prompt string) error { return nil }

	candidates := []bead{{ID: "ce-new", Title: "fresh work", Priority: 1}}
	var out, errOut bytes.Buffer
	got := dispatchCandidates(context.Background(), candidates, testWorkerLaunchConfig(), "/repo", false, &out, &errOut, nil)
	if got != 1 {
		t.Fatalf("dispatched = %d, want 1", got)
	}
}
