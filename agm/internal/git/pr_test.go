package git

import (
	"path/filepath"
	"testing"
)

// The reaper's safety rests on PRMergedState answering "unknown" (false,false)
// whenever it cannot positively prove a merge. These are the deterministic
// unknown paths that need no network or gh binary.

func TestPRMergedState_EmptyBranchIsUnknown(t *testing.T) {
	merged, known := PRMergedState("/anything", "")
	if merged || known {
		t.Fatalf("empty branch must be unknown, got merged=%v known=%v", merged, known)
	}
}

func TestPRMergedState_NotAGitRepoIsUnknown(t *testing.T) {
	dir := t.TempDir() // a bare temp dir, no .git anywhere up the tree
	merged, known := PRMergedState(filepath.Join(dir, "nope"), "claude/x")
	if merged || known {
		t.Fatalf("non-git path must be unknown, got merged=%v known=%v", merged, known)
	}
}

// The sweep's safety likewise rests on PRState answering unknown ("",false)
// whenever it cannot positively prove a PR's state.

func TestPRState_EmptyBranchIsUnknown(t *testing.T) {
	state, known := PRState("/anything", "")
	if state != "" || known {
		t.Fatalf("empty branch must be unknown, got state=%q known=%v", state, known)
	}
}

func TestPRState_NotAGitRepoIsUnknown(t *testing.T) {
	dir := t.TempDir()
	state, known := PRState(filepath.Join(dir, "nope"), "claude/x")
	if state != "" || known {
		t.Fatalf("non-git path must be unknown, got state=%q known=%v", state, known)
	}
}
