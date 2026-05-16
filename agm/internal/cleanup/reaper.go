package cleanup

import (
	"log/slog"
	"path/filepath"
	"strings"

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
}

// ReapResult records the outcome of a reap pass.
type ReapResult struct {
	Removed []string `json:"removed"`
	// Kept maps a worktree path to the reason it was preserved.
	Kept map[string]string `json:"kept"`
}

// ReapStaleWorktrees removes agent-created worktrees that are provably safe to
// drop: clean working tree AND zero commits ahead of the repo's base ref AND
// on a claude/ branch AND located under WorktreesBase. Every other worktree —
// and any worktree whose safety cannot be positively established — is kept and
// the reason is recorded.
//
// It is best-effort and non-blocking: it never returns an error and a failure
// on one worktree does not stop the others. Removal uses non-force
// `git worktree remove`, so git itself is the final guard against deleting a
// dirty or locked tree even if a status probe raced.
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

	for _, wt := range worktrees {
		path := cleanPath(wt.Path)

		if keep, reason := ineligibleReason(wt, path, selfPath, opts.WorktreesBase); keep {
			res.Kept[wt.Path] = reason
			logger.Debug("reaper: keep", "path", wt.Path, "reason", reason)
			continue
		}

		dirty, err := git.HasUncommittedChanges(wt.Path)
		if err != nil {
			res.Kept[wt.Path] = "status-check-failed"
			logger.Warn("reaper: status check failed; keeping", "path", wt.Path, "error", err)
			continue
		}
		if dirty {
			res.Kept[wt.Path] = "uncommitted-changes"
			logger.Info("reaper: keep (dirty)", "path", wt.Path)
			continue
		}

		ahead, err := git.CommitsAhead(opts.RepoPath, wt.Branch, base)
		if err != nil {
			res.Kept[wt.Path] = "ahead-check-failed"
			logger.Warn("reaper: ahead check failed; keeping", "path", wt.Path, "error", err)
			continue
		}
		if ahead != 0 {
			res.Kept[wt.Path] = "commits-ahead"
			logger.Info("reaper: keep (unmerged commits)", "path", wt.Path, "branch", wt.Branch, "ahead", ahead)
			continue
		}

		if opts.DryRun {
			res.Removed = append(res.Removed, wt.Path)
			logger.Info("reaper: would remove", "path", wt.Path, "branch", wt.Branch)
			continue
		}

		// Non-force on purpose: git refuses to remove a dirty or locked
		// worktree without --force, which is our last line of defense.
		if err := git.RemoveWorktree(opts.RepoPath, wt.Path, false); err != nil {
			res.Kept[wt.Path] = "remove-failed"
			logger.Warn("reaper: remove failed; keeping", "path", wt.Path, "error", err)
			continue
		}
		res.Removed = append(res.Removed, wt.Path)
		logger.Info("reaper: removed", "path", wt.Path, "branch", wt.Branch)
	}

	return res
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
