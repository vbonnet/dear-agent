package freshness

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// AncestryStatus enumerates the outcome of a deployment-ancestry check: whether
// the running binary's embedded vcs.revision is contained in the trunk
// (origin/main) history.
type AncestryStatus int

const (
	// AncestryVerified means the binary commit is an ancestor of the trunk ref
	// (it was built from code that is on origin/main). This is the healthy case.
	AncestryVerified AncestryStatus = iota
	// AncestryNotAncestor means the binary commit exists in the repo but is NOT
	// an ancestor of the trunk ref — it was built from a rolled-back or divergent
	// checkout. This is the exact failure that deadlocked the VROOM mesh
	// (rev 0fd2ee23 ∉ origin/main). FAIL LOUD.
	AncestryNotAncestor
	// AncestryCommitMissing means the binary commit object is not present in the
	// repo at all — the binary was built from something not even in this clone.
	// FAIL LOUD.
	AncestryCommitMissing
	// AncestryUnknownCommit means the binary was built without embedded vcs info,
	// so its provenance cannot be verified. FAIL LOUD.
	AncestryUnknownCommit
	// AncestryIndeterminate means the check could not be performed due to genuine
	// infrastructure indeterminacy (no source repo, git unavailable, no trunk
	// tracking ref, or timeout). FAIL OPEN — warn but do not block.
	AncestryIndeterminate
)

// AncestryResult holds the outcome of a VerifyAncestry call.
type AncestryResult struct {
	Status       AncestryStatus
	BinaryCommit string // the (de-dirtied) commit the binary was built from
	TrunkRef     string // the ref verified against, e.g. "origin/main"
	TrunkCommit  string // short sha the trunk ref resolves to, when resolvable
	RepoPath     string
	Err          error
}

// OK reports whether the binary is verified to be on trunk.
func (r AncestryResult) OK() bool { return r.Status == AncestryVerified }

// FailLoud reports whether this is a definitive deployment-verification failure
// that must be surfaced loudly (non-zero exit / unhealthy doctor), as opposed to
// infra indeterminacy which fails open.
func (r AncestryResult) FailLoud() bool {
	switch r.Status {
	case AncestryNotAncestor, AncestryCommitMissing, AncestryUnknownCommit:
		return true
	case AncestryVerified, AncestryIndeterminate:
		return false
	default:
		return false
	}
}

// Reason returns a short human-readable explanation of the status.
func (r AncestryResult) Reason() string {
	switch r.Status {
	case AncestryVerified:
		return fmt.Sprintf("binary commit %s is on trunk (%s = %s)", r.BinaryCommit, r.TrunkRef, r.TrunkCommit)
	case AncestryNotAncestor:
		return fmt.Sprintf("binary commit %s is NOT an ancestor of %s (%s) — built from a rolled-back or divergent checkout", r.BinaryCommit, r.TrunkRef, r.TrunkCommit)
	case AncestryCommitMissing:
		return fmt.Sprintf("binary commit %s is not present in the source repo at all", r.BinaryCommit)
	case AncestryUnknownCommit:
		return "binary has no embedded vcs.revision — provenance cannot be verified"
	case AncestryIndeterminate:
		if r.Err != nil {
			return fmt.Sprintf("could not verify deployment ancestry: %v", r.Err)
		}
		return "could not verify deployment ancestry"
	default:
		return "unknown ancestry status"
	}
}

// gitTimeout bounds every git invocation so a wedged git can never hang doctor
// or a git hook.
const gitTimeout = 2 * time.Second

// ResolveTrunkRef determines the trunk ref to verify against. It prefers the
// remote default branch (origin/HEAD) and falls back to "origin/main".
func ResolveTrunkRef(repoPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	// origin/HEAD -> e.g. "refs/remotes/origin/main"; --short -> "origin/main".
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
	if out, err := cmd.Output(); err == nil {
		if ref := strings.TrimSpace(string(out)); ref != "" {
			return ref
		}
	}
	return "origin/main"
}

// VerifyAncestry checks that binaryCommit (the running binary's embedded
// vcs.revision) is an ancestor of trunkRef in the repo at repoPath. If trunkRef
// is empty it is resolved via ResolveTrunkRef.
//
// It fails LOUD (FailLoud()==true) for definitive violations — the commit is off
// trunk, missing, or absent — and fails OPEN (AncestryIndeterminate) for infra
// problems it cannot distinguish from a healthy-but-offline machine. No network
// fetch is performed: it verifies against the local origin/main tracking ref.
func VerifyAncestry(repoPath, binaryCommit, trunkRef string) AncestryResult {
	r := AncestryResult{
		BinaryCommit: binaryCommit,
		TrunkRef:     trunkRef,
		RepoPath:     repoPath,
	}

	// No embedded vcs info — provenance is unverifiable.
	if binaryCommit == "" || binaryCommit == "unknown" {
		r.Status = AncestryUnknownCommit
		return r
	}

	// A dirty build still has a meaningful base commit; verify that.
	commit := strings.TrimSuffix(binaryCommit, "-dirty")
	r.BinaryCommit = commit

	if r.TrunkRef == "" {
		r.TrunkRef = ResolveTrunkRef(repoPath)
	}

	// Resolve the trunk ref. If there is no local origin/main tracking ref (no
	// remote, fresh clone, not a repo), we cannot verify — fail open.
	sha, err := runGit(repoPath, "rev-parse", "--short", "--verify", r.TrunkRef+"^{commit}")
	if err != nil {
		r.Status = AncestryIndeterminate
		r.Err = fmt.Errorf("cannot resolve trunk ref %q in %s: %w", r.TrunkRef, repoPath, err)
		return r
	}
	r.TrunkCommit = sha

	// Is the binary commit even an object in this repo? `cat-file -e` exits
	// non-zero (and our runGit returns err) when the object is absent.
	if _, err := runGit(repoPath, "cat-file", "-e", commit+"^{commit}"); err != nil {
		r.Status = AncestryCommitMissing
		return r
	}

	// The decisive check: is the binary commit an ancestor of trunk?
	//   exit 0  -> ancestor (verified)
	//   exit 1  -> not an ancestor (rolled-back / divergent)
	//   other   -> error (indeterminate)
	_, ancErr := runGit(repoPath, "merge-base", "--is-ancestor", commit, r.TrunkRef)
	if ancErr == nil {
		r.Status = AncestryVerified
		return r
	}
	if ec, ok := exitCode(ancErr); ok && ec == 1 {
		r.Status = AncestryNotAncestor
		return r
	}
	r.Status = AncestryIndeterminate
	r.Err = fmt.Errorf("git merge-base --is-ancestor failed: %w", ancErr)
	return r
}

// runGit runs a time-bounded git command in repoPath and returns trimmed stdout.
func runGit(repoPath string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	full := append([]string{"-C", repoPath}, args...)
	out, err := exec.CommandContext(ctx, "git", full...).Output()
	return strings.TrimSpace(string(out)), err
}

// exitCode extracts a process exit code from an *exec.ExitError.
func exitCode(err error) (int, bool) {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), true
	}
	return 0, false
}
