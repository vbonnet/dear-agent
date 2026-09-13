package safegit

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	freshnessHeadOID        = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	freshnessBaseOID        = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	freshnessLiveBaseOID    = "cccccccccccccccccccccccccccccccccccccccc"
	freshnessOldBaseOID     = "dddddddddddddddddddddddddddddddddddddddd"
	freshnessChangedHeadOID = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	freshnessOtherOID       = "ffffffffffffffffffffffffffffffffffffffff"
)

func TestLiveRefEndpointEscapesWholeRef(t *testing.T) {
	if got, want := liveRefEndpoint("owner/repo", "release/v1%beta?query#fragment"),
		"repos/owner/repo/git/ref/heads%2Frelease%2Fv1%25beta%3Fquery%23fragment"; got != want {
		t.Fatalf("liveRefEndpoint() = %q, want %q", got, want)
	}
}

func TestValidGitOID(t *testing.T) {
	valid64 := freshnessHeadOID + freshnessBaseOID[:24]
	for _, oid := range []string{freshnessHeadOID, valid64} {
		if !validGitOID(oid) {
			t.Errorf("validGitOID(%q) = false, want true", oid)
		}
	}
	for _, oid := range []string{"", "abc123", strings.ToUpper(freshnessHeadOID), freshnessHeadOID[:39], freshnessHeadOID + "g"} {
		if validGitOID(oid) {
			t.Errorf("validGitOID(%q) = true, want false", oid)
		}
	}
}

func TestEscapedRepoPathRejectsAmbiguousRepo(t *testing.T) {
	for _, repo := range []string{"owner", "owner/repo/extra", "/repo", "owner/", "../repo", "owner/..", "owner/white space"} {
		if _, err := escapedRepoPath(repo); err == nil {
			t.Errorf("escapedRepoPath(%q) succeeded; want error", repo)
		}
	}
}

func TestParseLiveRefOID(t *testing.T) {
	oid, err := parseLiveRefOID([]byte(`{"ref":"refs/heads/main","object":{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","type":"commit"}}`), "refs/heads/main")
	if err != nil {
		t.Fatalf("parseLiveRefOID() error = %v", err)
	}
	if oid != freshnessBaseOID {
		t.Fatalf("parseLiveRefOID() = %q, want %s", oid, freshnessBaseOID)
	}
}

func TestParseLiveRefOIDFailsClosed(t *testing.T) {
	for _, payload := range []string{
		`{}`,
		`{"ref":"refs/heads/other","object":{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","type":"commit"}}`,
		`{"ref":"refs/heads/main","object":{"sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","type":"tag"}}`,
		`{"ref":"refs/heads/main","object":{"sha":"abc123","type":"commit"}}`,
		`{"ref":"refs/heads/main","object":{"sha":"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB","type":"commit"}}`,
		`{not json`,
	} {
		if _, err := parseLiveRefOID([]byte(payload), "refs/heads/main"); err == nil {
			t.Errorf("parseLiveRefOID(%q) succeeded; want error", payload)
		}
	}
}

func TestParseMergeStateStatusFailsClosed(t *testing.T) {
	for _, payload := range []string{`{}`, `{"mergeStateStatus":""}`, `{not json`} {
		if _, err := parseMergeStateStatus([]byte(payload)); err == nil {
			t.Errorf("parseMergeStateStatus(%q) succeeded; want error", payload)
		}
	}
}

// The update is anchored to the SHA the gates ran against. Without an anchor a
// branch that moved mid-run would be advanced on stale evidence.
func TestUpdatePRBranchRequiresTOCTOUAnchor(t *testing.T) {
	err := updatePRBranch(t.Context(), 1, "owner/repo", "")
	if err == nil {
		t.Fatal("expected error when expectedHeadSHA is empty")
	}
	if !strings.Contains(err.Error(), "TOCTOU") {
		t.Errorf("error should name the missing anchor, got: %v", err)
	}
}

// ErrBranchUpdateRequested is a blocking outcome the caller must be able to tell apart
// from a gate failure — it means "retry", not "this PR is bad".
func TestErrBranchUpdateRequestedIsIdentifiable(t *testing.T) {
	wrapped := errors.Join(errors.New("context"), ErrBranchUpdateRequested)
	if !errors.Is(wrapped, ErrBranchUpdateRequested) {
		t.Error("ErrBranchUpdateRequested must survive wrapping so watch mode can retry")
	}
}

type freshnessFixture struct {
	baseBranch       string
	secondBaseBranch string
	firstBaseOID     string
	secondBaseOID    string
	firstHeadOID     string
	secondHeadOID    string
	compareState     string
	compareBaseOID   string
	mergeBaseOID     string
	mergeState       string
	refFailsAt       int
	compareFails     bool
	updateFails      bool
}

// fakeGhForFreshness records every provider read/mutation. It returns the
// first and second live-base resolutions independently so tests can exercise a
// base moving during the proof.
func fakeGhForFreshness(t *testing.T, fixture freshnessFixture) string {
	t.Helper()
	dir := t.TempDir()
	callLog := filepath.Join(dir, "calls")
	refCount := filepath.Join(dir, "ref-count")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_CALL_LOG"
case "$*" in
  *baseRefName*)
    head="$FAKE_FIRST_HEAD_OID"
    branch="$FAKE_FIRST_BASE_BRANCH"
    if [ -r "$FAKE_REF_COUNT" ]; then
      head="$FAKE_SECOND_HEAD_OID"
      branch="$FAKE_SECOND_BASE_BRANCH"
    fi
    printf '{"baseRefName":"%s","headRefOid":"%s"}\n' "$branch" "$head"
    ;;
  *git/ref/heads*)
    count=0
    if [ -r "$FAKE_REF_COUNT" ]; then
      IFS= read -r count < "$FAKE_REF_COUNT"
    fi
    count=$((count + 1))
    printf '%s\n' "$count" > "$FAKE_REF_COUNT"
    if [ "$FAKE_REF_FAILS_AT" = "$count" ]; then
      printf '%s\n' 'live ref unavailable' >&2
      exit 1
    fi
    oid="$FAKE_SECOND_BASE_OID"
    branch="$FAKE_SECOND_BASE_BRANCH"
    if [ "$count" -eq 1 ]; then
      oid="$FAKE_FIRST_BASE_OID"
      branch="$FAKE_FIRST_BASE_BRANCH"
    fi
    printf '{"ref":"refs/heads/%s","object":{"sha":"%s","type":"commit"}}\n' \
      "$branch" "$oid"
    ;;
  *compare/*)
    if [ "$FAKE_COMPARE_FAILS" = "1" ]; then
      printf '%s\n' 'compare unavailable' >&2
      exit 1
    fi
    printf '{"status":"%s","base_commit":{"sha":"%s"},"merge_base_commit":{"sha":"%s"}}\n' \
      "$FAKE_COMPARE_STATE" "$FAKE_COMPARE_BASE_OID" "$FAKE_MERGE_BASE_OID"
    ;;
  *mergeStateStatus*)
    printf '{"mergeStateStatus":"%s"}\n' "$FAKE_MERGE_STATE"
    ;;
  *update-branch*)
    if [ "$FAKE_UPDATE_FAILS" = "1" ]; then
      printf '%s\n' 'branch cannot be updated' >&2
      exit 1
    fi
    printf '%s\n' '{"message":"Updating pull request branch."}'
    ;;
  *)
    printf 'unexpected: %s\n' "$*" >&2
    exit 2
    ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	if fixture.baseBranch == "" {
		fixture.baseBranch = "main"
	}
	if fixture.secondBaseBranch == "" {
		fixture.secondBaseBranch = fixture.baseBranch
	}
	if fixture.mergeState == "" {
		fixture.mergeState = mergeStateBehind
	}
	if fixture.firstHeadOID == "" {
		fixture.firstHeadOID = freshnessHeadOID
	}
	if fixture.secondHeadOID == "" {
		fixture.secondHeadOID = fixture.firstHeadOID
	}
	if fixture.compareBaseOID == "" {
		fixture.compareBaseOID = fixture.firstBaseOID
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_CALL_LOG", callLog)
	t.Setenv("FAKE_REF_COUNT", refCount)
	t.Setenv("FAKE_FIRST_BASE_BRANCH", fixture.baseBranch)
	t.Setenv("FAKE_SECOND_BASE_BRANCH", fixture.secondBaseBranch)
	t.Setenv("FAKE_FIRST_BASE_OID", fixture.firstBaseOID)
	t.Setenv("FAKE_SECOND_BASE_OID", fixture.secondBaseOID)
	t.Setenv("FAKE_FIRST_HEAD_OID", fixture.firstHeadOID)
	t.Setenv("FAKE_SECOND_HEAD_OID", fixture.secondHeadOID)
	t.Setenv("FAKE_COMPARE_STATE", fixture.compareState)
	t.Setenv("FAKE_COMPARE_BASE_OID", fixture.compareBaseOID)
	t.Setenv("FAKE_MERGE_BASE_OID", fixture.mergeBaseOID)
	t.Setenv("FAKE_MERGE_STATE", fixture.mergeState)
	t.Setenv("FAKE_REF_FAILS_AT", strconv.Itoa(fixture.refFailsAt))
	if fixture.compareFails {
		t.Setenv("FAKE_COMPARE_FAILS", "1")
	} else {
		t.Setenv("FAKE_COMPARE_FAILS", "0")
	}
	if fixture.updateFails {
		t.Setenv("FAKE_UPDATE_FAILS", "1")
	} else {
		t.Setenv("FAKE_UPDATE_FAILS", "0")
	}
	return callLog
}

func readFreshnessCalls(t *testing.T, callLog string) string {
	t.Helper()
	calls, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("read call log: %v", err)
	}
	return string(calls)
}

func TestCheckBranchFreshnessPassesWhenLiveBaseIsAncestor(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		firstBaseOID: freshnessBaseOID, secondBaseOID: freshnessBaseOID,
		compareState: compareAhead, mergeBaseOID: freshnessBaseOID,
	})

	if _, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false); err != nil {
		t.Fatalf("current branch blocked: %v", err)
	}
	calls := readFreshnessCalls(t, callLog)
	if got := strings.Count(calls, "git/ref/heads%2Fmain"); got != 2 {
		t.Fatalf("live base resolved %d times, want 2; calls:\n%s", got, calls)
	}
	if !strings.Contains(calls, "compare/"+freshnessBaseOID+"..."+freshnessHeadOID) {
		t.Fatalf("missing explicit ancestry comparison; calls:\n%s", calls)
	}
	if strings.Contains(calls, "update-branch") {
		t.Fatalf("current branch was mutated; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessPassesIdenticalHeadAndBase(t *testing.T) {
	fakeGhForFreshness(t, freshnessFixture{
		firstBaseOID: freshnessHeadOID, secondBaseOID: freshnessHeadOID,
		compareState: compareIdentical, mergeBaseOID: freshnessHeadOID,
	})
	if _, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, true); err != nil {
		t.Fatalf("identical head and base blocked: %v", err)
	}
}

func TestCheckBranchFreshnessSupportsNestedBaseBranch(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		baseBranch: "release/v1", firstBaseOID: freshnessBaseOID, secondBaseOID: freshnessBaseOID,
		compareState: compareAhead, mergeBaseOID: freshnessBaseOID,
	})
	if _, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false); err != nil {
		t.Fatalf("nested base branch blocked: %v", err)
	}
	if calls := readFreshnessCalls(t, callLog); !strings.Contains(calls, "git/ref/heads%2Frelease%2Fv1") {
		t.Fatalf("nested ref was not escaped as one API path parameter; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessRejectsRetargetFromAttemptBase(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		baseBranch: "release/v1", firstBaseOID: freshnessBaseOID, secondBaseOID: freshnessBaseOID,
		compareState: compareAhead, mergeBaseOID: freshnessBaseOID,
	})

	_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "main", freshnessHeadOID, false)
	if err == nil || !strings.Contains(err.Error(), "base changed before freshness proof") {
		t.Fatalf("got %v, want retarget error", err)
	}
	calls := readFreshnessCalls(t, callLog)
	if strings.Contains(calls, "compare/") || strings.Contains(calls, "update-branch") {
		t.Fatalf("retargeted PR reached comparison or mutation; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessRejectsMalformedPRHeadOID(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		firstBaseOID: freshnessBaseOID, secondBaseOID: freshnessBaseOID,
		firstHeadOID: "main", secondHeadOID: "main",
		compareState: compareAhead, mergeBaseOID: freshnessBaseOID,
	})

	_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "main", freshnessHeadOID, false)
	if err == nil || !strings.Contains(err.Error(), "invalid headRefOid") {
		t.Fatalf("got %v, want invalid head OID error", err)
	}
	if calls := readFreshnessCalls(t, callLog); strings.Contains(calls, "git/ref/") || strings.Contains(calls, "compare/") {
		t.Fatalf("malformed PR head reached ref or compare reads; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessDryRunRejectsStaleHeadWithoutMutation(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		firstBaseOID: freshnessLiveBaseOID, secondBaseOID: freshnessLiveBaseOID,
		compareState: compareDiverged, mergeBaseOID: freshnessOldBaseOID, mergeState: mergeStateClean,
	})

	_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, true)
	if err == nil {
		t.Fatal("dry-run reported a stale head as current")
	}
	for _, want := range []string{"does not contain live base", "main@" + freshnessLiveBaseOID, "diverged"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if calls := readFreshnessCalls(t, callLog); strings.Contains(calls, "update-branch") {
		t.Fatalf("dry-run pushed to the branch; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessAdvancesStaleHead(t *testing.T) {
	for _, state := range []string{mergeStateBehind, mergeStateClean, mergeStateUnstable} {
		t.Run(state, func(t *testing.T) {
			callLog := fakeGhForFreshness(t, freshnessFixture{
				firstBaseOID: freshnessLiveBaseOID, secondBaseOID: freshnessLiveBaseOID,
				compareState: compareDiverged, mergeBaseOID: freshnessOldBaseOID, mergeState: state,
			})

			_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false)
			if !errors.Is(err, ErrBranchUpdateRequested) {
				t.Fatalf("got %v, want ErrBranchUpdateRequested", err)
			}
			calls := readFreshnessCalls(t, callLog)
			if !strings.Contains(calls, "update-branch") {
				t.Fatalf("gate did not request an update for stale branch; calls:\n%s", calls)
			}
			if !strings.Contains(calls, "expected_head_sha="+freshnessHeadOID) {
				t.Fatalf("update was not anchored to the gated head SHA; calls:\n%s", calls)
			}
		})
	}
}

func TestCheckBranchFreshnessFailsClosedWhenBaseMovesDuringProof(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		firstBaseOID: freshnessBaseOID, secondBaseOID: freshnessLiveBaseOID,
		compareState: compareAhead, mergeBaseOID: freshnessBaseOID,
	})

	_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false)
	if err == nil || !strings.Contains(err.Error(), "base or head changed during freshness proof") {
		t.Fatalf("got %v, want base-moved error", err)
	}
	if calls := readFreshnessCalls(t, callLog); strings.Contains(calls, "update-branch") {
		t.Fatalf("moving base triggered a mutation; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessFailsClosedWhenBaseIsRetargetedDuringProof(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		baseBranch: "main", secondBaseBranch: "develop",
		firstBaseOID: freshnessBaseOID, secondBaseOID: freshnessLiveBaseOID,
		compareState: compareAhead, mergeBaseOID: freshnessBaseOID,
	})

	_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "main", freshnessHeadOID, false)
	if err == nil || !strings.Contains(err.Error(), "base or head changed during freshness proof") {
		t.Fatalf("got %v, want base-retargeted error", err)
	}
	if calls := readFreshnessCalls(t, callLog); strings.Contains(calls, "update-branch") {
		t.Fatalf("retargeted base triggered a mutation; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessFailsClosedWhenHeadMovesDuringProof(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		firstBaseOID: freshnessBaseOID, secondBaseOID: freshnessBaseOID,
		firstHeadOID: freshnessHeadOID, secondHeadOID: freshnessChangedHeadOID,
		compareState: compareAhead, mergeBaseOID: freshnessBaseOID,
	})

	_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false)
	if err == nil || !strings.Contains(err.Error(), "base or head changed during freshness proof") {
		t.Fatalf("got %v, want head-moved error", err)
	}
	if calls := readFreshnessCalls(t, callLog); strings.Contains(calls, "update-branch") {
		t.Fatalf("moving head triggered a mutation; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessFailsClosedWhenLiveBaseCannotBeResolved(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		firstBaseOID: "", secondBaseOID: freshnessBaseOID,
		compareState: compareAhead, mergeBaseOID: freshnessBaseOID,
	})

	_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false)
	if err == nil || !strings.Contains(err.Error(), "invalid commit object.sha") {
		t.Fatalf("got %v, want fail-closed live-base error", err)
	}
	if calls := readFreshnessCalls(t, callLog); strings.Contains(calls, "compare/") {
		t.Fatalf("gate compared without a live base; calls:\n%s", calls)
	}
}

func TestCheckBranchFreshnessFailsClosedOnProviderReadFailures(t *testing.T) {
	tests := []struct {
		name         string
		refFailsAt   int
		compareFails bool
		want         string
	}{
		{name: "initial live ref", refFailsAt: 1, want: "cannot resolve live base before"},
		{name: "compare", compareFails: true, want: "cannot prove PR #42"},
		{name: "repeated live ref", refFailsAt: 2, want: "cannot re-resolve live base"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callLog := fakeGhForFreshness(t, freshnessFixture{
				firstBaseOID: freshnessBaseOID, secondBaseOID: freshnessBaseOID,
				compareState: compareAhead, mergeBaseOID: freshnessBaseOID,
				refFailsAt: tt.refFailsAt, compareFails: tt.compareFails,
			})
			_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
			if calls := readFreshnessCalls(t, callLog); strings.Contains(calls, "update-branch") {
				t.Fatalf("provider read failure caused mutation; calls:\n%s", calls)
			}
		})
	}
}

func TestCheckBranchFreshnessRejectsUnknownOrInconsistentCompareProof(t *testing.T) {
	tests := []struct {
		name        string
		state       string
		compareBase string
		mergeBase   string
		want        string
	}{
		{name: "unknown state", state: "mystery", mergeBase: freshnessBaseOID, want: "unknown status"},
		{name: "inconsistent witness", state: compareAhead, mergeBase: freshnessOtherOID, want: "inconsistent"},
		{name: "wrong resolved base", state: compareAhead, compareBase: freshnessOtherOID, mergeBase: freshnessBaseOID, want: "resolved base_commit.sha"},
		{name: "malformed merge base", state: compareAhead, mergeBase: "not-an-oid", want: "non-canonical"},
		{name: "missing merge base", state: compareAhead, want: "omitted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeGhForFreshness(t, freshnessFixture{
				firstBaseOID: freshnessBaseOID, secondBaseOID: freshnessBaseOID,
				compareState: tt.state, compareBaseOID: tt.compareBase, mergeBaseOID: tt.mergeBase,
			})
			_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestCheckBranchFreshnessReportsFailedBranchUpdate(t *testing.T) {
	fakeGhForFreshness(t, freshnessFixture{
		firstBaseOID: freshnessLiveBaseOID, secondBaseOID: freshnessLiveBaseOID,
		compareState: compareDiverged, mergeBaseOID: freshnessOldBaseOID, updateFails: true,
	})

	_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false)
	if err == nil || errors.Is(err, ErrBranchUpdateRequested) || !strings.Contains(err.Error(), "branch update failed") {
		t.Fatalf("got %v, want blocking update error", err)
	}
}

func TestCheckBranchFreshnessDoesNotUpdateUnapprovedMergeState(t *testing.T) {
	for _, state := range []string{"BLOCKED", "DRAFT", "HAS_HOOKS", "UNKNOWN"} {
		t.Run(state, func(t *testing.T) {
			callLog := fakeGhForFreshness(t, freshnessFixture{
				firstBaseOID: freshnessLiveBaseOID, secondBaseOID: freshnessLiveBaseOID,
				compareState: compareDiverged, mergeBaseOID: freshnessOldBaseOID, mergeState: state,
			})

			_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false)
			if err == nil || !strings.Contains(err.Error(), "does not authorize an automatic update") {
				t.Fatalf("got %v, want non-authorizing state error", err)
			}
			if calls := readFreshnessCalls(t, callLog); strings.Contains(calls, "update-branch") {
				t.Fatalf("state %s caused a branch mutation; calls:\n%s", state, calls)
			}
		})
	}
}

func TestCheckBranchFreshnessRefusesConflictWithoutUpdate(t *testing.T) {
	callLog := fakeGhForFreshness(t, freshnessFixture{
		firstBaseOID: freshnessLiveBaseOID, secondBaseOID: freshnessLiveBaseOID,
		compareState: compareDiverged, mergeBaseOID: freshnessOldBaseOID, mergeState: mergeStateDirty,
	})

	_, err := checkBranchFreshness(t.Context(), 42, "owner/repo", "", freshnessHeadOID, false)
	if err == nil || !strings.Contains(err.Error(), "conflicts with its base branch") {
		t.Fatalf("got %v, want conflict error", err)
	}
	if calls := readFreshnessCalls(t, callLog); strings.Contains(calls, "update-branch") {
		t.Fatalf("conflicting branch was mutated; calls:\n%s", calls)
	}
}
