package safemerge

import (
	"encoding/json"
	"strings"
	"testing"
)

// --- checkState ---

func TestCheckState_Open(t *testing.T) {
	pr := &prState{Number: 1, State: "OPEN", IsDraft: false}
	if err := checkState(pr); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckState_Closed(t *testing.T) {
	pr := &prState{Number: 1, State: "CLOSED"}
	if err := checkState(pr); err == nil {
		t.Error("expected error for CLOSED PR")
	}
}

func TestCheckState_Merged(t *testing.T) {
	pr := &prState{Number: 1, State: "MERGED"}
	if err := checkState(pr); err == nil {
		t.Error("expected error for MERGED PR")
	}
}

func TestCheckState_Draft(t *testing.T) {
	pr := &prState{Number: 1, State: "OPEN", IsDraft: true}
	err := checkState(pr)
	if err == nil {
		t.Error("expected error for draft PR")
	}
	if !strings.Contains(err.Error(), "draft") {
		t.Errorf("error should mention draft, got: %v", err)
	}
}

// --- checkMergeable ---

func TestCheckMergeable_Clean(t *testing.T) {
	pr := &prState{Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"}
	if err := checkMergeable(pr); err != nil {
		t.Errorf("expected nil for CLEAN, got %v", err)
	}
}

func TestCheckMergeable_Conflicting(t *testing.T) {
	pr := &prState{Number: 1, Mergeable: "CONFLICTING", BaseRefName: "main"}
	err := checkMergeable(pr)
	if err == nil {
		t.Error("expected error for CONFLICTING")
	}
	if !strings.Contains(err.Error(), "conflict") {
		t.Errorf("error should mention conflict, got: %v", err)
	}
}

func TestCheckMergeable_Dirty(t *testing.T) {
	pr := &prState{Number: 1, Mergeable: "MERGEABLE", MergeStateStatus: "DIRTY", BaseRefName: "main"}
	if err := checkMergeable(pr); err == nil {
		t.Error("expected error for DIRTY merge state")
	}
}

func TestCheckMergeable_Behind(t *testing.T) {
	pr := &prState{Number: 1, Mergeable: "MERGEABLE", MergeStateStatus: "BEHIND", BaseRefName: "main"}
	if err := checkMergeable(pr); err == nil {
		t.Error("expected error for BEHIND merge state")
	}
}

func TestCheckMergeable_Unknown(t *testing.T) {
	// UNKNOWN mergeable = GitHub hasn't computed it yet; not an error.
	pr := &prState{Number: 1, Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN"}
	if err := checkMergeable(pr); err != nil {
		t.Errorf("expected nil for UNKNOWN (pending computation), got %v", err)
	}
}

// --- checkChecks ---

func TestCheckChecks_AllSuccess(t *testing.T) {
	pr := &prState{Checks: []checkRun{
		{Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Name: "test", Status: "COMPLETED", Conclusion: "NEUTRAL"},
		{Name: "skip", Status: "COMPLETED", Conclusion: "SKIPPED"},
	}}
	pending, refused := checkChecks(pr)
	if pending || refused {
		t.Errorf("all passing: pending=%v refused=%v", pending, refused)
	}
}

func TestCheckChecks_Pending(t *testing.T) {
	pr := &prState{Checks: []checkRun{
		{Name: "build", Status: "IN_PROGRESS"},
	}}
	pending, refused := checkChecks(pr)
	if !pending || refused {
		t.Errorf("in-progress: pending=%v refused=%v", pending, refused)
	}
}

func TestCheckChecks_Failure(t *testing.T) {
	pr := &prState{Checks: []checkRun{
		{Name: "build", Status: "COMPLETED", Conclusion: "FAILURE"},
	}}
	_, refused := checkChecks(pr)
	if !refused {
		t.Errorf("expected refused=true")
	}
}

func TestCheckChecks_Empty(t *testing.T) {
	pr := &prState{}
	pending, refused := checkChecks(pr)
	if pending || refused {
		t.Errorf("no checks: pending=%v refused=%v", pending, refused)
	}
}

// --- checkThreads ---

func TestCheckThreads_NoUnresolved(t *testing.T) {
	pr := &prState{}
	if err := checkThreads(pr); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCheckThreads_AllResolved(t *testing.T) {
	pr := &prState{UnresolvedThreads: []reviewThread{}}
	if err := checkThreads(pr); err != nil {
		t.Errorf("expected nil for empty unresolved list, got %v", err)
	}
}

func TestCheckThreads_HasUnresolved(t *testing.T) {
	pr := &prState{
		Number: 42,
		UnresolvedThreads: []reviewThread{
			{ID: "a", Path: "foo.go", IsResolved: false, FirstBody: "please fix this"},
		},
	}
	err := checkThreads(pr)
	if err == nil {
		t.Error("expected error for unresolved thread")
	}
	if !strings.Contains(err.Error(), "foo.go") {
		t.Errorf("error should mention file path, got: %v", err)
	}
}

// --- findWorktreeForBranch ---

func TestFindWorktreeForBranch_Found(t *testing.T) {
	porcelain := strings.Join([]string{
		"worktree /Users/user/src/repo",
		"HEAD abc123",
		"branch refs/heads/main",
		"",
		"worktree /Users/user/worktrees/repo/feat-x",
		"HEAD def456",
		"branch refs/heads/feat-x",
		"",
	}, "\n")
	got := findWorktreeForBranch(porcelain, "feat-x")
	if got != "/Users/user/worktrees/repo/feat-x" {
		t.Errorf("expected feat-x worktree path, got %q", got)
	}
}

func TestFindWorktreeForBranch_NotFound(t *testing.T) {
	porcelain := "worktree /Users/user/src/repo\nHEAD abc123\nbranch refs/heads/main\n"
	got := findWorktreeForBranch(porcelain, "no-such-branch")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// --- parseCheckNode ---

func TestParseCheckNode_CheckRun_Success(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"__typename": "CheckRun",
		"name":       "build",
		"status":     "COMPLETED",
		"conclusion": "SUCCESS",
	})
	c := parseCheckNode(raw)
	if c.Name != "build" || c.Status != "COMPLETED" || c.Conclusion != "SUCCESS" {
		t.Errorf("unexpected: %+v", c)
	}
}

func TestParseCheckNode_StatusContext_Success(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"__typename": "StatusContext",
		"context":    "ci/lint",
		"state":      "SUCCESS",
	})
	c := parseCheckNode(raw)
	if c.Name != "ci/lint" || c.Status != "COMPLETED" || c.Conclusion != "SUCCESS" {
		t.Errorf("unexpected: %+v", c)
	}
}

func TestParseCheckNode_StatusContext_Pending(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"__typename": "StatusContext",
		"context":    "ci/test",
		"state":      "PENDING",
	})
	c := parseCheckNode(raw)
	if c.Status != "IN_PROGRESS" {
		t.Errorf("PENDING should map to IN_PROGRESS, got %q", c.Status)
	}
}

func TestParseCheckNode_StatusContext_Failure(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"__typename": "StatusContext",
		"context":    "ci/test",
		"state":      "FAILURE",
	})
	c := parseCheckNode(raw)
	if c.Conclusion != "FAILURE" {
		t.Errorf("expected FAILURE, got %q", c.Conclusion)
	}
}

func TestParseCheckNode_InvalidJSON(t *testing.T) {
	c := parseCheckNode(json.RawMessage(`{not valid json`))
	if c.Conclusion != "FAILURE" {
		t.Errorf("invalid JSON should produce FAILURE conclusion, got %q", c.Conclusion)
	}
}

// --- splitRepo ---

func TestSplitRepo_Valid(t *testing.T) {
	owner, repo, err := splitRepo("vbonnet/dear-agent")
	if err != nil || owner != "vbonnet" || repo != "dear-agent" {
		t.Errorf("unexpected: owner=%q repo=%q err=%v", owner, repo, err)
	}
}

func TestSplitRepo_Invalid(t *testing.T) {
	if _, _, err := splitRepo("no-slash"); err == nil {
		t.Error("expected error for missing slash")
	}
	if _, _, err := splitRepo("/empty-owner"); err == nil {
		t.Error("expected error for empty owner")
	}
	if _, _, err := splitRepo("owner/"); err == nil {
		t.Error("expected error for empty repo")
	}
}

// --- allowedConclusion ---

func TestAllowedConclusion(t *testing.T) {
	for _, c := range []string{"SUCCESS", "NEUTRAL", "SKIPPED"} {
		if !allowedConclusion(c) {
			t.Errorf("expected %q to be allowed", c)
		}
	}
	for _, c := range []string{"FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", ""} {
		if allowedConclusion(c) {
			t.Errorf("expected %q to NOT be allowed", c)
		}
	}
}
