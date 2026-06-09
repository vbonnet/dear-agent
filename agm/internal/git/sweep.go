package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// IsAncestor reports whether ref's tip is reachable from base (i.e. ref's
// work is already on base via a fast-forward / merge-commit landing).
//
// This is the squash-SAFE half of the merge oracle the sweep needs. Unlike
// CommitsAhead it cannot be tricked into "ahead>0 ⇒ keep" by an unrelated
// tip — but it DOES return false for a *squash*-merged branch, because the
// squash commit on base is a different SHA than anything on the branch.
// Squash detection is delegated to PRState; the sweep treats a worktree as
// merged when EITHER signal fires.
//
// On any error establishing the relationship it returns (false, err):
// callers MUST treat unknown as "not an ancestor" so a probe failure can
// never green-light reaping a not-yet-merged worktree.
func IsAncestor(repoPath, ref, base string) (bool, error) {
	gitRoot, err := findGitRoot(repoPath)
	if err != nil {
		return false, fmt.Errorf("not a git repository: %w", err)
	}
	if ref == "" || base == "" {
		return false, fmt.Errorf("ref and base must both be set (ref=%q base=%q)", ref, base)
	}
	err = exec.Command("git", "-C", gitRoot,
		"merge-base", "--is-ancestor", ref, base).Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		// Exit 1 is git's well-defined "not an ancestor" — expected, not an error.
		return false, nil
	}
	return false, fmt.Errorf("merge-base --is-ancestor %s %s: %w", ref, base, err)
}

// LastCommitInfo returns the committer date and subject of the worktree's
// current HEAD commit. It powers the "last touched" column and the
// identical-subject dedup grouping in the sweep report.
//
// On any error it returns (zero, "", err); the sweep degrades that worktree
// to an indeterminate (kept) classification rather than guessing.
func LastCommitInfo(worktreePath string) (when time.Time, subject string, err error) {
	// %cI = strict-ISO-8601 committer date; NUL separates it from the
	// subject so a subject containing any byte cannot corrupt the split.
	out, e := exec.Command("git", "-C", worktreePath, "log", "-1",
		"--format=%cI%x00%s").Output()
	if e != nil {
		return time.Time{}, "", fmt.Errorf("git log -1 in %s: %w", worktreePath, e)
	}
	parts := strings.SplitN(strings.TrimRight(string(out), "\n"), "\x00", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("unexpected git log output %q", string(out))
	}
	ts, e := time.Parse(time.RFC3339, parts[0])
	if e != nil {
		return time.Time{}, "", fmt.Errorf("parse commit time %q: %w", parts[0], e)
	}
	return ts, parts[1], nil
}

// HasUnpushedCommits reports whether branch carries commits that exist on no
// remote-tracking ref at all (`git rev-list --count <branch> --not --remotes`
// is non-zero).
//
// This is NOT a merge signal: a squash-merged branch reports true here (its
// pre-squash commits live on no remote). It is consulted ONLY after the
// merge oracle has already said "not merged", to separate genuinely-unpushed
// work (real, unrecoverable data-loss risk — flag loudly, never reap) from
// pushed-but-unmerged work.
//
// On any error it returns (true, err): callers MUST treat unknown as "has
// unpushed work" so a probe failure can never mask lost commits.
func HasUnpushedCommits(worktreePath, branch string) (bool, error) {
	if branch == "" {
		return true, fmt.Errorf("branch must be set")
	}
	out, err := exec.Command("git", "-C", worktreePath, "rev-list",
		"--count", branch, "--not", "--remotes").Output()
	if err != nil {
		return true, fmt.Errorf("rev-list --count %s --not --remotes: %w", branch, err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return true, fmt.Errorf("unexpected rev-list output %q: %w", string(out), err)
	}
	return n > 0, nil
}

// MainWorktreePath returns the absolute path of the primary checkout that the
// linked worktree at worktreePath belongs to. `git worktree remove` must run
// from (or name) the main checkout — git refuses to remove a worktree from
// inside itself — so the sweep needs this to act on what it discovered by a
// filesystem walk.
//
// On any error it returns ("", err) and the sweep keeps the worktree
// (cannot safely act without knowing where to run the removal).
func MainWorktreePath(worktreePath string) (string, error) {
	wts, err := ListWorktrees(worktreePath)
	if err != nil {
		return "", fmt.Errorf("list worktrees from %s: %w", worktreePath, err)
	}
	for _, wt := range wts {
		if wt.IsMain {
			return wt.Path, nil
		}
	}
	return "", fmt.Errorf("no main worktree found for %s", worktreePath)
}
