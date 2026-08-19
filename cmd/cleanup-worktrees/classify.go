package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// classification is the verdict for one worktree. Exactly one is assigned,
// and only classMerged is ever removed. This allowlist stance mirrors
// agm/internal/ops/worktree_sweep.go: delete only what positively proves
// reapable, because a denylist sweep eventually eats unmerged work.
type classification string

const (
	// classProtected covers the checkouts that are structurally never
	// candidates: the primary checkout, the caller's own worktree, a
	// `git worktree lock`ed checkout, an explicit --preserve name, and any
	// worktree holding the repository's target branch.
	classProtected classification = "PROTECTED"
	// classActive means a live tmux/AGM session owns this checkout.
	classActive classification = "ACTIVE"
	// classDirty means uncommitted or untracked changes are present. Never
	// removed: a reap must never destroy work that was never committed.
	classDirty classification = "DIRTY"
	// classMerged means provably landed: the branch carries no commits
	// beyond the target ref. The only removable class.
	classMerged classification = "MERGED"
	// classOrphaned means clean and not active, but carrying commits that
	// are not on the target ref. Reported, never removed. Age alone is not
	// evidence that work landed.
	classOrphaned classification = "ORPHANED"
	// classUnknown means a probe failed, so safety could not be positively
	// established. Kept, fail-safe.
	classUnknown classification = "UNKNOWN"
)

// removable reports whether fix mode may remove this class.
func (c classification) removable() bool { return c == classMerged }

// scanEnv is the immutable per-run context every classification consults.
type scanEnv struct {
	// target is the resolved remote ref merged work is measured against.
	target string
	// targetBranch is target's short branch name, e.g. "main". A worktree
	// holding it is protected: deleting that branch would take out the
	// repository's default branch.
	targetBranch string
	now          time.Time
	// active is the set of live session names; activeKnown is false when
	// the probe failed, which disables removal entirely.
	active      map[string]bool
	activeKnown bool
	// protected holds canonical paths that must never be removed: the
	// primary checkout, the supplied repo path, and the caller's cwd.
	protected map[string]bool
}

// verdict is the classification of one worktree plus what fix mode did.
type verdict struct {
	class   classification
	reason  string
	removed bool
	failed  bool
}

// structuralVerdict covers the checkouts that never reach the merge oracle:
// the ones protected by what they are rather than by what they contain. It
// returns false when the worktree is an ordinary removal candidate.
//
// Order matters. Every rule here outranks the merge proof, so a locked or
// session-owned checkout that happens to be fully merged is still kept.
func structuralVerdict(cfg config, env scanEnv, wt worktree) (verdict, bool) {
	name := filepath.Base(wt.path)
	branch := shortBranch(wt.branch)
	switch {
	case wt.primary || env.protected[canonical(wt.path)]:
		return verdict{class: classProtected, reason: "primary checkout or invoking worktree"}, true
	case wt.bare:
		return verdict{class: classProtected, reason: "bare repository"}, true
	case wt.locked:
		return verdict{class: classProtected, reason: "git worktree lock held"}, true
	case cfg.preserve[name]:
		return verdict{class: classProtected, reason: "--preserve " + name}, true
	case branch != "" && branch == env.targetBranch:
		return verdict{class: classProtected, reason: "holds the target branch " + branch}, true
	case wt.detached:
		return verdict{class: classUnknown, reason: "detached HEAD: no branch to prove merged"}, true
	case env.active[name] || (branch != "" && env.active[branch]):
		return verdict{class: classActive, reason: "live session owns this checkout"}, true
	}
	return verdict{}, false
}

// classify assigns a verdict without mutating anything. Every probe failure
// resolves to classUnknown, so no failed probe can become a stale verdict.
func classify(ctx context.Context, cfg config, env scanEnv, wt worktree) verdict {
	if v, done := structuralVerdict(cfg, env, wt); done {
		return v
	}

	dirty, err := isDirty(ctx, wt.path)
	if err != nil {
		return verdict{class: classUnknown, reason: fmt.Sprintf("dirty probe failed: %v", err)}
	}
	if dirty {
		return verdict{class: classDirty, reason: "uncommitted or untracked changes"}
	}

	ahead, err := gitInt(ctx, cfg.repo, "rev-list", "--count", env.target+".."+wt.head)
	if err != nil {
		return verdict{class: classUnknown, reason: fmt.Sprintf("commits-ahead probe failed: %v", err)}
	}
	if ahead > 0 {
		return verdict{class: classOrphaned, reason: idleReason(ctx, cfg, env, wt, ahead)}
	}
	return verdict{class: classMerged, reason: fmt.Sprintf("no commits beyond %s", env.target)}
}

// idleReason annotates an orphaned worktree with its age. Age is reported
// only. An old branch with unique commits is abandoned work, not landed
// work, and deleting it is unrecoverable.
func idleReason(ctx context.Context, cfg config, env scanEnv, wt worktree, ahead int) string {
	base := fmt.Sprintf("%d commit(s) not on %s", ahead, env.target)
	last, err := gitInt(ctx, cfg.repo, "log", "-1", "--format=%ct", wt.head)
	if err != nil {
		return base + " (age unknown)"
	}
	days := int(env.now.Sub(time.Unix(int64(last), 0)).Hours() / 24)
	if days >= cfg.maxAge {
		return fmt.Sprintf("%s, idle %d days: review by hand", base, days)
	}
	return fmt.Sprintf("%s, last commit %d days ago", base, days)
}

// isDirty reports whether the checkout has uncommitted or untracked changes.
func isDirty(ctx context.Context, worktreePath string) (bool, error) {
	out, err := git(ctx, worktreePath, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// inspect classifies one worktree and, in fix mode, removes it when the
// verdict allows.
func inspect(ctx context.Context, cfg config, env scanEnv, wt worktree) verdict {
	v := classify(ctx, cfg, env, wt)
	name := filepath.Base(wt.path)
	if !v.class.removable() {
		logf("%-9s %s [%s] - %s", v.class, name, shortBranch(wt.branch), v.reason)
		return v
	}
	if !env.activeKnown {
		v.class = classUnknown
		v.reason = "active-session probe failed: refusing to remove anything this run"
		logf("%-9s %s [%s] - %s", v.class, name, shortBranch(wt.branch), v.reason)
		return v
	}

	branch := shortBranch(wt.branch)
	logf("%-9s %s [%s] - %s", v.class, name, branch, v.reason)
	if !cfg.fix {
		logf("  would: git -C %s worktree remove %s", cfg.repo, wt.path)
		if branch != "" {
			logf("  would: git -C %s branch -d %s", cfg.repo, branch)
		}
		return v
	}
	return remove(ctx, cfg, wt, branch, v)
}

// remove performs the reap. Removal is deliberately not forced: Git's own
// refusal to drop a dirty or locked checkout is a second, independent guard
// against a race between the dirty probe and this call. The remote branch is
// never touched, matching the canonical sweep's contract.
func remove(ctx context.Context, cfg config, wt worktree, branch string, v verdict) verdict {
	if err := runGitPassthrough(ctx, cfg.repo, "worktree", "remove", wt.path); err != nil {
		logf("  FAILED: worktree remove %s: %v", wt.path, err)
		logf("  kept branch %s because the checkout is still present", branch)
		v.failed = true
		return v
	}
	v.removed = true
	if branch == "" {
		return v
	}
	if gitOK(ctx, cfg.repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch) != nil {
		return v
	}
	// -d, not -D: the branch was classified as carrying no commits beyond
	// the target, so a safe delete must succeed. If Git disagrees, its
	// refusal outranks our classification and the branch stays.
	if err := runGitPassthrough(ctx, cfg.repo, "branch", "-d", branch); err != nil {
		logf("  note: kept local branch %s (git declined the safe delete): %v", branch, err)
	}
	return v
}
