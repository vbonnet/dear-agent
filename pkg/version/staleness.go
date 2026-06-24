package version

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

// IsStale returns true if the running binary's GitCommit is not an ancestor of
// origin/main in the repo at repoPath. It fails open: if git is unavailable,
// the repo path is invalid, or GitCommit is "unknown", it returns (false, nil)
// rather than a spurious staleness signal.
func IsStale(repoPath string) (bool, error) {
	commit := Short()
	if commit == "unknown" || commit == "" {
		return false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Verify the commit exists in this repo before running merge-base.
	if err := gitCmd(ctx, repoPath, "cat-file", "-e", commit+"^{commit}"); err != nil {
		// Commit not in this repo — treat as indeterminate (fail open).
		return false, nil
	}

	// git merge-base --is-ancestor exits 0 if ancestor, 1 if not, other on error.
	err := gitCmd(ctx, repoPath, "merge-base", "--is-ancestor", commit, "origin/main")
	if err == nil {
		return false, nil // on trunk
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return true, nil // not an ancestor
	}
	// git error (e.g. no remote tracking ref) — fail open.
	return false, err
}

func gitCmd(ctx context.Context, repoPath string, args ...string) error {
	full := append([]string{"-C", repoPath}, args...)
	return exec.CommandContext(ctx, "git", full...).Run()
}
