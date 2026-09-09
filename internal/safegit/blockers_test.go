package safegit

import (
	"strings"
	"testing"
)

func openPR() PRState {
	return PRState{
		Number:           42,
		State:            "OPEN",
		Mergeable:        "MERGEABLE",
		MergeStateStatus: "CLEAN",
		BaseRefName:      "main",
	}
}

func blockerCodes(bs []Blocker) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Code
	}
	return out
}

func TestClassifyBlockers_CleanPRHasNoBlockers(t *testing.T) {
	bs := ClassifyBlockers(openPR(), "owner/repo", nil, nil)
	if len(bs) != 0 {
		t.Fatalf("clean PR should have no blockers, got %v", blockerCodes(bs))
	}
}

func TestClassifyBlockers_Draft(t *testing.T) {
	st := openPR()
	st.IsDraft = true
	bs := ClassifyBlockers(st, "owner/repo", nil, nil)
	if len(bs) != 1 || bs[0].Code != BlockDraft {
		t.Fatalf("want [DRAFT], got %v", blockerCodes(bs))
	}
	if !strings.Contains(bs[0].Fix, "gh pr ready 42") {
		t.Errorf("draft fix must name the exact command, got: %s", bs[0].Fix)
	}
}

func TestClassifyBlockers_Conflicts(t *testing.T) {
	st := openPR()
	st.Mergeable = "CONFLICTING"
	st.MergeStateStatus = "DIRTY"
	bs := ClassifyBlockers(st, "owner/repo", nil, nil)
	if len(bs) != 1 || bs[0].Code != BlockConflicts {
		t.Fatalf("want [CONFLICTS], got %v", blockerCodes(bs))
	}
}

func TestClassifyBlockers_BehindEmitsUpdateBranch(t *testing.T) {
	st := openPR()
	st.MergeStateStatus = "BEHIND"
	bs := ClassifyBlockers(st, "owner/repo", nil, nil)
	if len(bs) != 1 || bs[0].Code != BlockBehind {
		t.Fatalf("want [BEHIND], got %v", blockerCodes(bs))
	}
	if !strings.Contains(bs[0].Fix, "gh pr update-branch 42") {
		t.Errorf("BEHIND fix must be gh pr update-branch, got: %s", bs[0].Fix)
	}
}

func TestClassifyBlockers_OutdatedUnresolvedThreadBlocks(t *testing.T) {
	st := openPR()
	st.MergeStateStatus = "BLOCKED"
	threads := []ReviewThread{
		{IsResolved: true},
		{ID: "PRRT_abc123", IsResolved: false, IsOutdated: true, Author: "gemini-code-assist", Path: "cmd/x/main.go"},
	}
	bs := ClassifyBlockers(st, "owner/repo", threads, nil)
	if len(bs) != 1 || bs[0].Code != BlockThreads {
		t.Fatalf("want [UNRESOLVED_THREADS], got %v", blockerCodes(bs))
	}
	if !strings.Contains(bs[0].Detail, "outdated") {
		t.Errorf("outdated threads must be called out, got: %s", bs[0].Detail)
	}
	assertExternalTemporaryReplyFileLifecycle(t, bs[0].Fix)
	if !strings.Contains(bs[0].Fix, "resolve-review-threads resolve-all owner repo 42") {
		t.Errorf("thread fix must name resolve-review-threads, got: %s", bs[0].Fix)
	}
	// The prescribed reply-resolve command takes a thread ID, so the diagnosis
	// has to carry one; otherwise its output cannot be acted on directly.
	if !strings.Contains(bs[0].Detail, "PRRT_abc123") {
		t.Errorf("detail must carry the thread ID for reply-resolve, got: %s", bs[0].Detail)
	}
	if !strings.Contains(bs[0].Fix, "resolve-review-threads list owner repo 42") {
		t.Errorf("fix must offer a list command that yields thread IDs, got: %s", bs[0].Fix)
	}
}

func assertExternalTemporaryReplyFileLifecycle(t *testing.T, got string) {
	t.Helper()
	normalized := strings.ToLower(got)
	for _, want := range []string{
		`reply_file="$(mktemp /tmp/resolve-review-thread.XXXXXX)"`,
		`--body-file "$reply_file"`,
		`rm -f -- "$reply_file"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("reply-file guidance missing %q:\n%s", want, got)
		}
	}
	for _, want := range []string{
		"for each thread",
		"exact-body retry remains valid",
		"file unchanged",
		"terminal resolution is confirmed",
		"only then",
		"safe-merge",
	} {
		if !strings.Contains(normalized, want) {
			t.Errorf("reply-file guidance missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{
		"reply.md",
		`reply-resolve <threadId> "`,
		`--body-file $reply_file`,
		"mktemp -t",
		"mktemp -u",
		"trap ",
	} {
		if strings.Contains(got, forbidden) {
			t.Errorf("reply-file guidance contains unsafe form %q:\n%s", forbidden, got)
		}
	}
	create := strings.Index(got, `reply_file="$(mktemp /tmp/resolve-review-thread.XXXXXX)"`)
	bodyFile := strings.Index(got, `--body-file "$reply_file"`)
	retention := strings.Index(normalized, "file unchanged")
	confirmation := strings.Index(normalized, "terminal resolution is confirmed")
	cleanup := strings.Index(got, `rm -f -- "$reply_file"`)
	nextThread := strings.Index(normalized, "next thread")
	merge := strings.LastIndex(normalized, "safe-merge")
	if create >= bodyFile || bodyFile >= retention || retention >= confirmation || confirmation >= cleanup || cleanup >= nextThread || cleanup >= merge {
		t.Errorf("reply-file guidance lifecycle is out of order (create=%d body=%d retain=%d confirm=%d cleanup=%d next=%d merge=%d):\n%s",
			create, bodyFile, retention, confirmation, cleanup, nextThread, merge, got)
	}
	if sweep := strings.Index(got, "resolve-review-threads resolve-all"); sweep >= 0 && cleanup >= sweep {
		t.Errorf("reply-file cleanup must precede resolve-all (cleanup=%d sweep=%d):\n%s", cleanup, sweep, got)
	}
}

func TestClassifyBlockers_ChecksSplitFailingAndPending(t *testing.T) {
	checks := []RequiredCheck{
		{Name: "Build & Test (ubuntu-latest)", Status: RequiredCheckFailing},
		{Name: "govulncheck", Status: RequiredCheckPending},
		{Name: "Vulnerability Scan", Status: RequiredCheckPassing},
	}
	bs := ClassifyBlockers(openPR(), "owner/repo", nil, checks)
	got := blockerCodes(bs)
	if len(got) != 2 || got[0] != BlockFailingCheck || got[1] != BlockPendingCheck {
		t.Fatalf("want [FAILING_REQUIRED_CHECK PENDING_REQUIRED_CHECK], got %v", got)
	}
	if !strings.Contains(bs[0].Detail, "Build & Test (ubuntu-latest)") {
		t.Errorf("failing check must be named, got: %s", bs[0].Detail)
	}
}

func TestClassifyBlockers_BehindOrderedLast(t *testing.T) {
	st := openPR()
	st.MergeStateStatus = "BEHIND"
	threads := []ReviewThread{{IsResolved: false, Author: "vbonnet", Path: "a.go"}}
	bs := ClassifyBlockers(st, "owner/repo", threads, nil)
	got := blockerCodes(bs)
	if len(got) != 2 || got[0] != BlockThreads || got[1] != BlockBehind {
		t.Fatalf("BEHIND must come after content blockers, got %v", got)
	}
}

func TestClassifyBlockers_ReviewDecision(t *testing.T) {
	st := openPR()
	st.ReviewDecision = "CHANGES_REQUESTED"
	bs := ClassifyBlockers(st, "owner/repo", nil, nil)
	if len(bs) != 1 || bs[0].Code != BlockChangesReq {
		t.Fatalf("want [CHANGES_REQUESTED], got %v", blockerCodes(bs))
	}
}

func TestClassifyBlockers_BlockedWithNoCauseIsExplicit(t *testing.T) {
	st := openPR()
	st.MergeStateStatus = "BLOCKED"
	bs := ClassifyBlockers(st, "owner/repo", nil, nil)
	if len(bs) != 1 || bs[0].Code != BlockUnknown {
		t.Fatalf("want [UNKNOWN_BLOCK], got %v", blockerCodes(bs))
	}
	if !strings.Contains(bs[0].Fix, "Do NOT hunt for code problems") {
		t.Errorf("UNKNOWN_BLOCK must forbid guessing, got: %s", bs[0].Fix)
	}
}

func TestStateBlockers_CleanIsEmpty(t *testing.T) {
	if bs := StateBlockers(openPR(), "owner/repo"); len(bs) != 0 {
		t.Fatalf("clean state should have no blockers, got %v", blockerCodes(bs))
	}
}
