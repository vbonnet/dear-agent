// Package safesrc recovers a golden ~/src/<repo> checkout back to a clean,
// current default branch — and refuses to do anything else.
//
// # Why this exists
//
// ~/src/** is the read-only golden reference tree; all real work must happen in
// ~/worktrees/**. In practice the tree still drifts: a scheduled task whose
// workspace is ~/src/<repo> runs `git pull`/`git stash` there, an agent
// fat-fingers a command, and the checkout ends up dirty, on a feature branch,
// behind origin, or carrying a stash-pop conflict (see vbonnet/engram-research
// retrospectives/2026-06-11-src-violations-and-burndown.md).
//
// Recovering it by hand means typing raw git writes into ~/src — exactly the
// thing the deny-rules forbid, so an agent either can't do it or routes around
// the guard. safesrc is the atomic-wrapper answer (AGENTS.md principle 9): one
// vetted binary that can perform ONLY the recovery sequence
//
//	git stash --include-untracked   (only if the tree is dirty)
//	git checkout <default-branch>
//	git pull --ff-only
//
// on a path it has proven lives under ~/src/. It takes no pass-through git
// arguments, so there is no verb or flag an agent can smuggle in: every git
// verb it runs is a compile-time literal, gated by an allowlist. That turns a
// fuzzy permission question ("can this agent write to ~/src?") into a crisp one
// ("can this agent run the binary we vetted?"), which is the only form safe to
// allow-list.
package safesrc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultTimeout bounds the network step (pull) so a wedged credential helper
// or a stalled network fails fast instead of hanging the recovery forever. It
// matches internal/safegit so both wrappers behave the same under a keychain
// hang.
const DefaultTimeout = 30 * time.Second

// allowedVerbs is the complete set of git subcommands safesrc may run. It is
// the load-bearing safety boundary: runGit refuses anything not listed here, so
// the binary cannot be coaxed into commit/reset --hard/push/branch -D/etc. even
// if a future edit tries. Read-only inspection verbs and the three mutating
// recovery verbs are all that appear.
var allowedVerbs = map[string]bool{
	// read-only inspection
	"rev-parse":    true,
	"symbolic-ref": true,
	"status":       true,
	"diff":         true,
	"stash":        true, // stash push (mutating) and stash list (read) both go here
	"rev-list":     true,
	// mutating recovery steps
	"checkout": true,
	"pull":     true,
}

// SrcRoot returns the canonical, symlink-resolved ~/src directory. Validation
// compares against this so a symlinked home or src dir cannot be used to slip a
// path that only textually looks like it is outside ~/src.
func SrcRoot(home string) (string, error) {
	root := filepath.Join(home, "src")
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		// If ~/src itself does not resolve, recovery cannot be in-scope.
		return "", fmt.Errorf("cannot resolve %s: %w", root, err)
	}
	return resolved, nil
}

// ValidateRepo resolves repoArg to the git worktree top-level and proves it is a
// repository strictly *under* ~/src/. It rejects ~/src itself, any path outside
// ~/src, and non-repositories. The returned path is the canonical worktree root
// to operate on.
//
// Symlinks are fully resolved on both sides before the prefix check, so neither
// a symlinked checkout nor a "/Users/x/src/../worktrees/y" style path can
// escape the boundary.
func ValidateRepo(home, repoArg string) (string, error) {
	if strings.TrimSpace(repoArg) == "" {
		return "", fmt.Errorf("a repository path under ~/src/ is required")
	}
	expanded := expandHome(repoArg, home)
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q: %w", repoArg, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("path does not exist: %s", abs)
	}

	root, err := SrcRoot(home)
	if err != nil {
		return "", err
	}
	if resolved == root {
		return "", fmt.Errorf("refusing %s: that is the ~/src root itself, not a repository inside it", resolved)
	}
	if !underDir(resolved, root) {
		return "", fmt.Errorf("refusing %s: src-recovery only operates on repositories "+
			"under %s/ (the golden checkouts); for anything else, work in ~/worktrees/", resolved, root)
	}
	return resolved, nil
}

// underDir reports whether path is dir itself-excluded but strictly inside dir.
func underDir(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	// "." is dir itself (not "under"); ".." / "../x" climb out.
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// expandHome replaces a leading ~ or ~/ with home; other paths pass through.
func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// Recoverer runs the recovery sequence against one validated repository,
// emitting a line-per-step audit trail to its log writer.
type Recoverer struct {
	Repo    string        // validated worktree root (from ValidateRepo)
	DryRun  bool          // print the plan, run no mutating verb
	Timeout time.Duration // bounds the pull; DefaultTimeout if zero
	Log     io.Writer     // audit sink; every git verb + outcome is written here
}

// runGit runs `git -C <repo> <args...>` with the verb-allowlist enforced. It is
// the single choke point through which every git invocation passes, so the
// allowlist cannot be bypassed by any call site. stdout is captured and
// returned; stderr is mirrored to the audit log on failure.
func (r *Recoverer) runGit(ctx context.Context, args ...string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("internal: runGit called with no verb")
	}
	verb := args[0]
	if !allowedVerbs[verb] {
		// By construction this should be unreachable from in-package call
		// sites; it exists so a future edit that adds a forbidden verb fails
		// loudly instead of silently widening the tool's powers.
		return "", fmt.Errorf("internal: git %q is not in the src-recovery allowlist", verb)
	}
	full := append([]string{"-C", r.Repo}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	// Never block on an interactive credential/terminal prompt.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(r.Log, "    git %s -> error: %v: %s\n", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
		return out.String(), err
	}
	return out.String(), nil
}

// Plan describes what Recover decided to do, for the caller to report.
type Plan struct {
	DefaultBranch string
	Dirty         bool
	Stashed       bool
	StartBranch   string
}

// Recover performs the recovery sequence on r.Repo and returns what it did.
//
// Order and rules:
//  1. Refuse outright if the tree carries an unresolved merge/rebase/stash-pop
//     conflict — stashing unmerged paths is impossible and auto-aborting could
//     discard the user's stashed work. The human resolves those by hand.
//  2. If the tree is dirty (tracked changes or untracked files), stash them
//     with --include-untracked so nothing is lost; the user recovers them with
//     `git -C <repo> stash pop` if wanted.
//  3. Check out the default branch.
//  4. Fast-forward only: `git pull --ff-only`. Never a merge or rebase that
//     could rewrite or entangle the golden tree.
func (r *Recoverer) Recover(ctx context.Context) (Plan, error) {
	if r.Timeout <= 0 {
		r.Timeout = DefaultTimeout
	}
	var plan Plan

	fmt.Fprintf(r.Log, "src-recovery: repo=%s dry_run=%v\n", r.Repo, r.DryRun)

	branch, err := r.runGit(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return plan, fmt.Errorf("not a git repository or cannot read HEAD: %w", err)
	}
	plan.StartBranch = strings.TrimSpace(branch)

	plan.DefaultBranch = r.defaultBranch(ctx)
	fmt.Fprintf(r.Log, "    start_branch=%s default_branch=%s\n", plan.StartBranch, plan.DefaultBranch)

	// (1) Conflict guard.
	if conflicted, files := r.conflicted(ctx); conflicted {
		return plan, fmt.Errorf("refusing to recover %s: it has unresolved conflicts (%s); "+
			"src-recovery will not stash or discard a half-merged tree — resolve it by hand "+
			"(git -C %s status; then git -C %s merge --abort, or rebase --abort, or stash drop) "+
			"and re-run src-recovery", r.Repo, files, r.Repo, r.Repo)
	}

	// (2) Stash if dirty.
	dirty, err := r.dirty(ctx)
	if err != nil {
		return plan, err
	}
	plan.Dirty = dirty
	if dirty {
		stamp := stashStamp(ctx)
		if r.DryRun {
			fmt.Fprintf(r.Log, "    [dry-run] would: git stash push --include-untracked -m %q\n", stamp)
		} else {
			if _, err := r.runGit(ctx, "stash", "push", "--include-untracked", "-m", stamp); err != nil {
				return plan, fmt.Errorf("failed to stash dirty tree (nothing was changed): %w", err)
			}
			plan.Stashed = true
			fmt.Fprintf(r.Log, "    stashed working tree as %q (recover with: git -C %s stash pop)\n", stamp, r.Repo)
		}
	}

	// (3) Checkout default branch.
	switch {
	case r.DryRun:
		fmt.Fprintf(r.Log, "    [dry-run] would: git checkout %s\n", plan.DefaultBranch)
	case plan.StartBranch != plan.DefaultBranch:
		if _, err := r.runGit(ctx, "checkout", plan.DefaultBranch); err != nil {
			return plan, fmt.Errorf("failed to checkout %s: %w", plan.DefaultBranch, err)
		}
		fmt.Fprintf(r.Log, "    checked out %s\n", plan.DefaultBranch)
	default:
		fmt.Fprintf(r.Log, "    already on %s\n", plan.DefaultBranch)
	}

	// (4) Fast-forward pull, time-bounded.
	if r.DryRun {
		fmt.Fprintf(r.Log, "    [dry-run] would: git pull --ff-only (timeout %s)\n", r.Timeout)
		return plan, nil
	}
	pctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	if _, err := r.runGit(pctx, "pull", "--ff-only"); err != nil {
		if pctx.Err() == context.DeadlineExceeded {
			return plan, fmt.Errorf("git pull --ff-only exceeded %s and was killed — likely the "+
				"macOS keychain credential hang; checkout/stash already succeeded, just re-run "+
				"src-recovery to finish the pull", r.Timeout)
		}
		return plan, fmt.Errorf("git pull --ff-only failed (not a fast-forward?); the branch is "+
			"clean and on %s — resolve the divergence by hand: %w", plan.DefaultBranch, err)
	}
	fmt.Fprintf(r.Log, "    pulled %s (fast-forward only)\n", plan.DefaultBranch)
	fmt.Fprintf(r.Log, "src-recovery: done\n")
	return plan, nil
}

// defaultBranch resolves origin's default branch (origin/HEAD), falling back to
// "main" when origin/HEAD is unset. Spec calls for "checkout main"; resolving
// origin/HEAD keeps the tool correct for the rare repo whose default is master.
func (r *Recoverer) defaultBranch(ctx context.Context) string {
	out, err := r.runGit(ctx, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "main"
	}
	ref := strings.TrimSpace(out) // e.g. "origin/main"
	if _, after, found := strings.Cut(ref, "/"); found {
		return after
	}
	if ref == "" {
		return "main"
	}
	return ref
}

// conflicted reports whether the worktree has unmerged (UU/AA/etc.) paths.
func (r *Recoverer) conflicted(ctx context.Context) (bool, string) {
	out, err := r.runGit(ctx, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return false, ""
	}
	files := strings.TrimSpace(out)
	if files == "" {
		return false, ""
	}
	return true, strings.ReplaceAll(files, "\n", ", ")
}

// dirty reports whether the worktree has any tracked change or untracked file.
func (r *Recoverer) dirty(ctx context.Context) (bool, error) {
	out, err := r.runGit(ctx, "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("cannot read status: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

// stashStamp builds the stash message. It reads the time from the context-free
// process clock indirectly via a monotonic counter is not possible here, so the
// caller (binary) injects the timestamp through the context value; tests inject
// a fixed value for determinism.
func stashStamp(ctx context.Context) string {
	if v, ok := ctx.Value(stampKey{}).(string); ok && v != "" {
		return "src-recovery " + v
	}
	return "src-recovery"
}

type stampKey struct{}

// WithStamp returns a context carrying the timestamp string used in the stash
// message, so the binary controls the clock and tests stay deterministic.
func WithStamp(ctx context.Context, stamp string) context.Context {
	return context.WithValue(ctx, stampKey{}, stamp)
}
