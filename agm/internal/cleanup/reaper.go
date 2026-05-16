package cleanup

import (
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
)

// agentBranchPrefix marks a worktree branch as created by an agent session.
// Every worktree spawned by `agm new` checks out a `claude/<name>` branch;
// this prefix is the de-facto session marker (there is no per-worktree
// .session-id file). Restricting the reaper to this prefix is a deliberate
// allowlist: anything a human created by hand on a differently-named branch
// is never touched.
const agentBranchPrefix = "claude/"

// nestedWorktreeMarker identifies sandbox sub-worktrees that a running parent
// session created inside its own tree (…/<name>/.worktrees/<uuid>). These
// belong to a live session and must never be reaped from the outside.
const nestedWorktreeMarker = "/.worktrees/"

// ReaperGitOps abstracts the git queries and mutation the reaper needs, so
// the decision logic can be unit-tested without a real repository.
type ReaperGitOps interface {
	ListWorktrees(repoPath string) ([]gitpkg.Worktree, error)
	HasUncommittedChanges(worktreePath string) (bool, error)
	ResolveBaseRef(repoPath string) string
	CommitsAhead(repoPath, ref, base string) (int, error)
	RemoveWorktree(repoPath, worktreePath string, force bool) error
	// DeleteBranch removes the local branch left behind by a reaped worktree.
	// force selects `git branch -D` (needed for squash-merged branches git
	// still considers unmerged) over the safe `-d`.
	DeleteBranch(repoPath, branch string, force bool) error
	// PRMerged reports whether the pull request whose head is `branch` has
	// been merged. The second return ("known") is false when the answer
	// cannot be established (gh missing/unauthenticated, no PR, timeout,
	// network error); callers MUST treat unknown as "not safe to remove".
	PRMerged(repoPath, branch string) (merged bool, known bool)
}

// RealReaperGitOps implements ReaperGitOps against the real git CLI.
type RealReaperGitOps struct{}

// ListWorktrees lists the repository's worktrees via the gitpkg helper.
func (RealReaperGitOps) ListWorktrees(repoPath string) ([]gitpkg.Worktree, error) {
	return gitpkg.ListWorktrees(repoPath)
}

// HasUncommittedChanges reports whether a worktree is dirty via the gitpkg helper.
func (RealReaperGitOps) HasUncommittedChanges(worktreePath string) (bool, error) {
	return gitpkg.HasUncommittedChanges(worktreePath)
}

// ResolveBaseRef resolves the merge base ref via the gitpkg helper.
func (RealReaperGitOps) ResolveBaseRef(repoPath string) string {
	return gitpkg.ResolveBaseRef(repoPath)
}

// CommitsAhead counts commits on ref not in base via the gitpkg helper.
func (RealReaperGitOps) CommitsAhead(repoPath, ref, base string) (int, error) {
	return gitpkg.CommitsAhead(repoPath, ref, base)
}

// RemoveWorktree removes a git worktree via the gitpkg helper.
func (RealReaperGitOps) RemoveWorktree(repoPath, worktreePath string, force bool) error {
	return gitpkg.RemoveWorktree(repoPath, worktreePath, force)
}

// DeleteBranch deletes a local branch via the gitpkg helper.
func (RealReaperGitOps) DeleteBranch(repoPath, branch string, force bool) error {
	return gitpkg.DeleteBranch(repoPath, branch, force)
}

// PRMerged resolves the branch's PR merge state via the gitpkg helper (gh).
func (RealReaperGitOps) PRMerged(repoPath, branch string) (bool, bool) {
	return gitpkg.PRMergedState(repoPath, branch)
}

// ReaperOptions configures a single reap pass.
type ReaperOptions struct {
	// RepoPath is the repository whose worktrees are enumerated. Any path
	// inside the repo (including a worktree) works.
	RepoPath string
	// WorktreesBase is the directory under which agent worktrees live
	// (e.g. ~/worktrees). Only worktrees physically located beneath
	// WorktreesBase are eligible — this is the location half of the
	// allowlist that pairs with the claude/ branch prefix.
	WorktreesBase string
	// SelfPath, when set, is the absolute path of the worktree the caller
	// is running inside (the session that is stopping). It is never reaped
	// even when it looks clean — a brand-new live session is clean and
	// zero-ahead.
	SelfPath string
	// DryRun reports what would be removed without removing anything.
	DryRun bool
	// CheckMergedPR enables the authoritative merged-PR escape: a clean
	// worktree that is commits-ahead of base (the squash-merge fingerprint)
	// is still reclaimed when its PR is provably MERGED on the remote.
	// When false the reaper keeps every commits-ahead worktree — the
	// historic conservative behavior. Off by the zero value so unit tests
	// of the core decision matrix are unaffected; the CLI defaults it on.
	CheckMergedPR bool
	// PRCheckBudget bounds the wall-clock spent on remote PR-state lookups
	// across the whole pass so the Stop hook's own timeout is never the
	// thing that kills the reaper. Past the budget, commits-ahead worktrees
	// are kept (unknown ⇒ conservative). Zero means unbounded (test default;
	// fakes answer instantly).
	PRCheckBudget time.Duration
}

// ReapResult records the outcome of a reap pass.
type ReapResult struct {
	Removed []string `json:"removed"`
	// Kept maps a worktree path to the reason it was preserved.
	Kept map[string]string `json:"kept"`
}

// ReapStaleWorktrees removes agent-created worktrees that are provably safe to
// drop. A worktree is reclaimed when it is on a claude/ branch, located under
// WorktreesBase, not the caller's own worktree, has a clean working tree, and
// EITHER carries zero commits ahead of the repo's base ref OR (when
// CheckMergedPR is set) its pull request is provably MERGED on the remote.
// On removal the backing local branch is deleted too. Every other worktree —
// and any worktree whose safety cannot be positively established — is kept and
// the reason is recorded.
//
// It is best-effort and non-blocking: it never returns an error and a failure
// on one worktree does not stop the others. Worktree removal uses non-force
// `git worktree remove`, so git itself is the final guard against deleting a
// dirty or locked tree even if a status probe raced; a failed local-branch
// delete is logged but never undoes the (already safe) worktree removal.
func ReapStaleWorktrees(opts ReaperOptions, git ReaperGitOps, logger *slog.Logger) *ReapResult {
	if logger == nil {
		logger = slog.Default()
	}
	res := &ReapResult{Kept: map[string]string{}}

	worktrees, err := git.ListWorktrees(opts.RepoPath)
	if err != nil {
		logger.Warn("reaper: could not list worktrees", "repo", opts.RepoPath, "error", err)
		return res
	}

	base := git.ResolveBaseRef(opts.RepoPath)
	if base == "" {
		// Without a base ref we cannot prove any tree is merged. Keep all.
		logger.Warn("reaper: could not resolve base ref; keeping all worktrees", "repo", opts.RepoPath)
		for _, wt := range worktrees {
			res.Kept[wt.Path] = "base-ref-unresolved"
		}
		return res
	}

	selfPath := cleanPath(opts.SelfPath)
	var prDeadline time.Time
	if opts.PRCheckBudget > 0 {
		prDeadline = time.Now().Add(opts.PRCheckBudget)
	}

	for _, wt := range worktrees {
		d := classifyWorktree(opts, git, wt, cleanPath(wt.Path), selfPath, base, prDeadline)

		if !d.remove {
			res.Kept[wt.Path] = d.reason
			if d.reason == "uncommitted-changes" {
				// Spec: a dirty worktree is left in place with a warning.
				logger.Warn("reaper: keep (uncommitted changes)", "path", wt.Path, "branch", wt.Branch)
			} else {
				logger.Debug("reaper: keep", "path", wt.Path, "branch", wt.Branch, "reason", d.reason)
			}
			continue
		}

		if opts.DryRun {
			res.Removed = append(res.Removed, wt.Path)
			logger.Info("reaper: would remove", "path", wt.Path, "branch", wt.Branch, "via", d.reason)
			continue
		}

		// Non-force on purpose: git refuses to remove a dirty or locked
		// worktree without --force, which is our last line of defense.
		if err := git.RemoveWorktree(opts.RepoPath, wt.Path, false); err != nil {
			res.Kept[wt.Path] = "remove-failed"
			logger.Warn("reaper: remove failed; keeping", "path", wt.Path, "error", err)
			continue
		}
		// Worktree (the sprawl artifact) is gone. Delete the now-orphaned
		// local branch best-effort: a leftover branch is cheap and a delete
		// failure must not undo or mask the successful worktree removal.
		if err := git.DeleteBranch(opts.RepoPath, wt.Branch, d.forceBranchDelete); err != nil {
			logger.Warn("reaper: worktree removed but local branch delete failed (left dangling)",
				"path", wt.Path, "branch", wt.Branch, "error", err)
		} else {
			logger.Info("reaper: deleted local branch", "branch", wt.Branch)
		}
		res.Removed = append(res.Removed, wt.Path)
		logger.Info("reaper: removed", "path", wt.Path, "branch", wt.Branch, "via", d.reason)
	}

	return res
}

// reapDecision is the verdict for one worktree. When remove is false, reason
// is the recorded Kept reason; when true, reason is the removal tag
// ("clean-zero-ahead" or "pr-merged") and forceBranchDelete selects `-D`.
type reapDecision struct {
	remove            bool
	reason            string
	forceBranchDelete bool
}

func keep(reason string) reapDecision { return reapDecision{reason: reason} }
func remove(reason string, force bool) reapDecision {
	return reapDecision{remove: true, reason: reason, forceBranchDelete: force}
}

// classifyWorktree decides a single worktree's fate without mutating
// anything. A worktree is removable only when it passes the allowlist, is
// clean, and is either zero commits ahead of base or has a provably MERGED
// PR. Every uncertain or unprovable state resolves to keep (fail-safe).
func classifyWorktree(opts ReaperOptions, git ReaperGitOps, wt gitpkg.Worktree,
	path, selfPath, base string, prDeadline time.Time) reapDecision {

	if ineligible, reason := ineligibleReason(wt, path, selfPath, opts.WorktreesBase); ineligible {
		return keep(reason)
	}

	dirty, err := git.HasUncommittedChanges(wt.Path)
	if err != nil {
		return keep("status-check-failed")
	}
	if dirty {
		// Uncommitted work always wins, even over a merged PR: a clean
		// reap must never destroy edits the agent never committed.
		return keep("uncommitted-changes")
	}

	ahead, err := git.CommitsAhead(opts.RepoPath, wt.Branch, base)
	if err != nil {
		return keep("ahead-check-failed")
	}
	if ahead == 0 {
		// No unmerged work; safe `-d` branch delete.
		return remove("clean-zero-ahead", false)
	}

	// ahead > 0: either genuine unmerged work or a squash-merge artifact.
	// Only an authoritative MERGED PR distinguishes the latter.
	if !opts.CheckMergedPR {
		return keep("commits-ahead")
	}
	if !prDeadline.IsZero() && !time.Now().Before(prDeadline) {
		return keep("commits-ahead") // PR-check budget exhausted ⇒ keep
	}
	merged, known := git.PRMerged(opts.RepoPath, wt.Branch)
	if !known || !merged {
		return keep("commits-ahead")
	}
	// PR provably MERGED: ahead>0 is the squash artifact. git still sees the
	// branch as unmerged, so the local branch needs force delete (`-D`).
	return remove("pr-merged", true)
}

// ineligibleReason returns (true, reason) when a worktree must not even be
// considered for removal, independent of its clean/ahead status.
func ineligibleReason(wt gitpkg.Worktree, path, selfPath, worktreesBase string) (bool, string) {
	switch {
	case wt.IsMain:
		return true, "main-checkout"
	case wt.Branch == "":
		return true, "detached-head"
	case selfPath != "" && path == selfPath:
		return true, "self-worktree"
	case strings.Contains(path, nestedWorktreeMarker):
		return true, "nested-sandbox-worktree"
	case !strings.HasPrefix(wt.Branch, agentBranchPrefix):
		return true, "non-agent-branch"
	case !underBase(path, worktreesBase):
		return true, "outside-worktrees-base"
	}
	return false, ""
}

// underBase reports whether path is located beneath base. An empty base
// disables the location constraint (eligibility then rests on the other
// allowlist checks).
func underBase(path, base string) bool {
	base = cleanPath(base)
	if base == "" {
		return true
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func cleanPath(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}
