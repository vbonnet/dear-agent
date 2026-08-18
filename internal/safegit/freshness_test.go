package safegit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyFreshnessAdvancesOnlyBehindBranches(t *testing.T) {
	cases := map[string]freshnessAction{
		"BEHIND":   freshnessUpdate,
		"DIRTY":    freshnessConflict,
		"CLEAN":    freshnessProceed,
		"BLOCKED":  freshnessProceed,
		"UNSTABLE": freshnessProceed,
		"UNKNOWN":  freshnessProceed,
		"":         freshnessProceed,
	}
	for state, want := range cases {
		if got := classifyFreshness(state); got != want {
			t.Errorf("classifyFreshness(%q) = %v, want %v", state, got, want)
		}
	}
}

// A future provider state must not be read as "advance the branch". Only the
// one state that means "trails the base tip" earns a push.
func TestClassifyFreshnessTreatsUnrecognizedStatesAsNoOp(t *testing.T) {
	for _, state := range []string{"HAS_HOOKS", "DRAFT", "behind", "Behind"} {
		if got := classifyFreshness(state); got != freshnessProceed {
			t.Errorf("classifyFreshness(%q) = %v, want freshnessProceed", state, got)
		}
	}
}

func TestParseMergeStateStatus(t *testing.T) {
	state, err := parseMergeStateStatus([]byte(`{"mergeStateStatus":"BEHIND"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state != mergeStateBehind {
		t.Errorf("got %q, want %q", state, mergeStateBehind)
	}
}

// An absent verdict must be an error, never an empty string: classifyFreshness
// reads "" as proceed, so a silently-empty parse would let a behind branch
// through the gate it exists to enforce.
func TestParseMergeStateStatusRejectsAbsentVerdict(t *testing.T) {
	for _, payload := range []string{`{}`, `{"mergeStateStatus":""}`} {
		if _, err := parseMergeStateStatus([]byte(payload)); err == nil {
			t.Errorf("parseMergeStateStatus(%s) succeeded; want error", payload)
		}
	}
}

func TestParseMergeStateStatusRejectsMalformedJSON(t *testing.T) {
	if _, err := parseMergeStateStatus([]byte(`{not json`)); err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

// The update is anchored to the SHA the gates ran against. Without an anchor a
// branch that moved mid-run would be advanced on stale evidence.
func TestUpdatePRBranchRequiresTOCTOUAnchor(t *testing.T) {
	err := updatePRBranch(1, "owner/repo", "")
	if err == nil {
		t.Fatal("expected error when expectedHeadSHA is empty")
	}
	if !strings.Contains(err.Error(), "TOCTOU") {
		t.Errorf("error should name the missing anchor, got: %v", err)
	}
}

// ErrBranchUpdated is a blocking outcome the caller must be able to tell apart
// from a gate failure — it means "retry", not "this PR is bad".
func TestErrBranchUpdatedIsIdentifiable(t *testing.T) {
	wrapped := errors.Join(errors.New("context"), ErrBranchUpdated)
	if !errors.Is(wrapped, ErrBranchUpdated) {
		t.Error("ErrBranchUpdated must survive wrapping so watch mode can retry")
	}
}

// fakeGhForFreshness puts a gh on PATH that answers the two calls this gate
// makes and records the update-branch invocation.
func fakeGhForFreshness(t *testing.T, mergeState string) (callLog string) {
	t.Helper()
	dir := t.TempDir()
	callLog = filepath.Join(dir, "calls")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> \"" + callLog + "\"\n" +
		"case \"$*\" in\n" +
		"  *mergeStateStatus*) printf '%s\\n' '{\"mergeStateStatus\":\"" + mergeState + "\"}' ;;\n" +
		"  *update-branch*) printf '%s\\n' '{\"message\":\"Updating pull request branch.\"}' ;;\n" +
		"  *) printf 'unexpected: %s\\n' \"$*\" >&2; exit 2 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return callLog
}

func TestCheckBranchFreshnessAdvancesBehindBranch(t *testing.T) {
	callLog := fakeGhForFreshness(t, "BEHIND")

	err := checkBranchFreshness(42, "owner/repo", "abc123", false)
	if !errors.Is(err, ErrBranchUpdated) {
		t.Fatalf("got %v, want ErrBranchUpdated", err)
	}

	calls, readErr := os.ReadFile(callLog)
	if readErr != nil {
		t.Fatalf("read call log: %v", readErr)
	}
	if !strings.Contains(string(calls), "update-branch") {
		t.Errorf("gate did not advance the branch; calls:\n%s", calls)
	}
	if !strings.Contains(string(calls), "expected_head_sha=abc123") {
		t.Errorf("update was not anchored to the gated head SHA; calls:\n%s", calls)
	}
}

// A behind branch must block this attempt. Returning nil here would let the
// merge run against checks that the update is about to invalidate.
func TestCheckBranchFreshnessBlocksRatherThanMerging(t *testing.T) {
	fakeGhForFreshness(t, "BEHIND")
	if err := checkBranchFreshness(42, "owner/repo", "abc123", false); err == nil {
		t.Fatal("gate returned nil for a behind branch; the merge would proceed on stale checks")
	}
}

// Dry-run must report without pushing: a --dry-run that mutates the branch is
// not a dry run.
func TestCheckBranchFreshnessDryRunDoesNotPush(t *testing.T) {
	callLog := fakeGhForFreshness(t, "BEHIND")

	err := checkBranchFreshness(42, "owner/repo", "abc123", true)
	if err == nil {
		t.Fatal("dry-run reported a behind branch as mergeable")
	}
	if errors.Is(err, ErrBranchUpdated) {
		t.Fatal("dry-run claimed it updated the branch")
	}
	calls, _ := os.ReadFile(callLog)
	if strings.Contains(string(calls), "update-branch") {
		t.Errorf("dry-run pushed to the branch; calls:\n%s", calls)
	}
}

// Conflicts are content decisions. The gate must refuse them outright rather
// than attempt an update that would either fail or resolve them blindly.
func TestCheckBranchFreshnessRefusesConflicts(t *testing.T) {
	callLog := fakeGhForFreshness(t, "DIRTY")

	err := checkBranchFreshness(42, "owner/repo", "abc123", false)
	if err == nil || errors.Is(err, ErrBranchUpdated) {
		t.Fatalf("got %v, want a blocking conflict error", err)
	}
	calls, _ := os.ReadFile(callLog)
	if strings.Contains(string(calls), "update-branch") {
		t.Errorf("gate tried to update a conflicting branch; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessPassesUpToDateBranch(t *testing.T) {
	fakeGhForFreshness(t, "CLEAN")
	if err := checkBranchFreshness(42, "owner/repo", "abc123", false); err != nil {
		t.Fatalf("up-to-date branch blocked by freshness gate: %v", err)
	}
}
