package freshness

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// gitT runs git in dir and fails the test on error.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gittest.Output(t, dir, args...)
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(out)
}

// newTrunkRepo builds a repo with a local "origin/main" tracking ref and returns
// the repo path. Layout:
//
//	C1 ── C2 (main, origin/main)   <- on-trunk history
//	  \── R1 (rolled-back/divergent, NOT on origin/main)
//
// It returns the repo path and the two commit shas (onTrunk = C2, offTrunk = R1).
func newTrunkRepo(t *testing.T) (repo, onTrunk, offTrunk string) {
	t.Helper()
	repo = t.TempDir()
	gitT(t, repo, "init", "-q", "-b", "main")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "c1")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "c2")
	onTrunk = gitT(t, repo, "rev-parse", "HEAD")

	// Create a divergent commit off c1 that never reaches main.
	gitT(t, repo, "checkout", "-q", "-b", "rollback", "HEAD~1")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "rolled-back work")
	offTrunk = gitT(t, repo, "rev-parse", "HEAD")
	gitT(t, repo, "checkout", "-q", "main")

	// Simulate the local origin/main tracking ref pointing at trunk tip, without
	// any network: write the remote-tracking ref directly.
	gitT(t, repo, "update-ref", "refs/remotes/origin/main", onTrunk)
	return repo, onTrunk, offTrunk
}

func TestVerifyAncestry_Verified(t *testing.T) {
	repo, onTrunk, _ := newTrunkRepo(t)
	r := VerifyAncestry(repo, onTrunk, "origin/main")
	if r.Status != AncestryVerified || !r.OK() {
		t.Fatalf("expected Verified, got %v (%s)", r.Status, r.Reason())
	}
	if r.FailLoud() {
		t.Error("verified result must not FailLoud")
	}
}

func TestVerifyAncestry_NotAncestor(t *testing.T) {
	repo, _, offTrunk := newTrunkRepo(t)
	r := VerifyAncestry(repo, offTrunk, "origin/main")
	if r.Status != AncestryNotAncestor {
		t.Fatalf("expected NotAncestor, got %v (%s)", r.Status, r.Reason())
	}
	if !r.FailLoud() {
		t.Error("a rolled-back commit must FailLoud")
	}
}

func TestVerifyAncestry_CommitMissing(t *testing.T) {
	repo, _, _ := newTrunkRepo(t)
	// A syntactically valid sha that is not an object in the repo.
	r := VerifyAncestry(repo, "0fd2ee230fd2ee230fd2ee230fd2ee230fd2ee23", "origin/main")
	if r.Status != AncestryCommitMissing {
		t.Fatalf("expected CommitMissing, got %v (%s)", r.Status, r.Reason())
	}
	if !r.FailLoud() {
		t.Error("a commit absent from the repo must FailLoud")
	}
}

func TestVerifyAncestry_UnknownCommit(t *testing.T) {
	for _, c := range []string{"", "unknown"} {
		r := VerifyAncestry(t.TempDir(), c, "origin/main")
		if r.Status != AncestryUnknownCommit {
			t.Errorf("commit %q: expected UnknownCommit, got %v", c, r.Status)
		}
		if !r.FailLoud() {
			t.Errorf("commit %q: must FailLoud", c)
		}
	}
}

func TestVerifyAncestry_NoRepo_FailsOpen(t *testing.T) {
	r := VerifyAncestry("/tmp/nonexistent-path-xyz-123", "abc1234", "origin/main")
	if r.Status != AncestryIndeterminate {
		t.Fatalf("expected Indeterminate, got %v (%s)", r.Status, r.Reason())
	}
	if r.FailLoud() || r.OK() {
		t.Error("indeterminate must neither FailLoud nor be OK (fail open)")
	}
	if r.Err == nil {
		t.Error("expected an error on indeterminate result")
	}
}

func TestVerifyAncestry_NoTrunkRef_FailsOpen(t *testing.T) {
	// Repo with commits but no origin/main tracking ref at all.
	repo := t.TempDir()
	gitT(t, repo, "init", "-q", "-b", "main")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "c1")
	head := gitT(t, repo, "rev-parse", "HEAD")

	r := VerifyAncestry(repo, head, "origin/main")
	if r.Status != AncestryIndeterminate {
		t.Fatalf("expected Indeterminate when trunk ref absent, got %v (%s)", r.Status, r.Reason())
	}
}

func TestVerifyAncestry_DirtySuffixStripped(t *testing.T) {
	repo, onTrunk, _ := newTrunkRepo(t)
	r := VerifyAncestry(repo, onTrunk+"-dirty", "origin/main")
	if r.Status != AncestryVerified {
		t.Fatalf("expected Verified for dirty suffix of on-trunk commit, got %v (%s)", r.Status, r.Reason())
	}
	if strings.HasSuffix(r.BinaryCommit, "-dirty") {
		t.Error("BinaryCommit should have -dirty stripped")
	}
}

func TestVerifyAncestry_DefaultTrunkRef(t *testing.T) {
	repo, onTrunk, _ := newTrunkRepo(t)
	// Empty trunkRef should resolve (falls back to origin/main here).
	r := VerifyAncestry(repo, onTrunk, "")
	if r.TrunkRef == "" {
		t.Error("expected trunk ref to be resolved")
	}
	if r.Status != AncestryVerified {
		t.Fatalf("expected Verified with resolved trunk ref, got %v (%s)", r.Status, r.Reason())
	}
}
