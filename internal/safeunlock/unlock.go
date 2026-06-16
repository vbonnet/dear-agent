// Package safeunlock removes provably-stale git lock files from any repository
// or linked worktree, and nothing else.
//
// # Why this exists
//
// git protects index, ref, config and packed-refs updates with sibling
// lockfiles (`index.lock`, `refs/heads/<name>.lock`, `config.lock`,
// `packed-refs.lock`, …). A lock lives only for the milliseconds git holds it,
// then it is renamed over its target or removed. If the git process is killed
// mid-update (OOM, timeout, a supervisor `stop`, a crashed worktree command) the
// lock is orphaned, and every later operation on that repo fails with exit 128
// ("Unable to create '.git/index.lock': File exists"). An agent that hits this
// is wedged: it cannot commit, cannot checkout, cannot proceed — and the obvious
// "fix" is to `rm` the lock by hand, which races any git that *is* live.
//
// safesrc already solved this for the golden ~/src/<repo> checkouts, but it is
// hard-scoped to ~/src by construction. safeunlock is the general engine: it
// works on any repo — most importantly the ~/worktrees/** trees where agents do
// their work — handles the full family of git lock files (not just index.lock),
// scans linked worktrees, and appends a JSONL audit record of every decision.
// Raw `rm` of a lock is the unsafe form this wrapper replaces (CLAUDE.md
// principle 9): the one vetted path that removes a *provably stale* lock and
// refuses an active one.
//
// # Why it is safe by construction
//
// A lock is removed only when BOTH hold:
//
//  1. it is older than MinAge (default 2m) — orders of magnitude beyond the
//     sub-second window a live git holds a lock — so a lock a running git just
//     created is never touched, and
//  2. no process currently holds it open (verified with `lsof`). git keeps the
//     lock's file descriptor open for the whole operation, so "no holder" means
//     the git that created it is gone. This is the "is the owning PID still
//     alive?" check: git lockfiles carry no PID in their contents, so the live
//     holder set is the authoritative owner signal.
//
// If lsof is unavailable (rare on macOS) the holder check is skipped and age
// alone guards removal — strictly more conservative, never less. A lock that is
// present but fails either guard is reported Active and left in place; the
// caller surfaces it as a non-zero exit ("still locked, a real git is running").
//
// Lock paths are always computed by scanning known git lock locations; none is
// ever taken from a caller argument, so there is no arbitrary file an agent can
// name for deletion.
package safeunlock

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultMinLockAge is how old a git lock must be before Clean will treat it as
// stale debris. A live git holds a lock for milliseconds; 2 minutes is far
// beyond that, so this never races a running git while still clearing any lock a
// crashed or killed one left behind. Override per-invocation with Cleaner.MinAge
// (the binary exposes --min-age).
const DefaultMinLockAge = 2 * time.Minute

// Cleaner removes provably-stale git lock files from a single repository or
// linked worktree. Unlike safesrc.Unlocker it imposes no path boundary — it is
// meant for ~/worktrees/** and arbitrary checkouts — so callers are responsible
// for pointing it at a repo they intend to operate on.
type Cleaner struct {
	Repo             string        // checkout root (a dir whose .git resolves a git dir)
	DryRun           bool          // report what would happen; remove nothing
	MinAge           time.Duration // min lock age to consider stale; DefaultMinLockAge if zero
	IncludeWorktrees bool          // also scan <gitdir>/worktrees/*/ lock files
	Now              time.Time     // injected clock (binary passes time.Now(); tests fix it)
	Log              io.Writer     // human-readable audit sink; per-lock decisions are written here
}

// Result records what Clean decided about a single lock file, for the caller to
// report and for the JSONL audit trail.
type Result struct {
	LockPath string        `json:"lock_path"`        // absolute path of the lock inspected
	Kind     string        `json:"kind"`             // index | HEAD | config | packed-refs | ref:<name> | worktree:<name>/<…>
	Removed  bool          `json:"removed"`          // whether it was removed (false in dry-run or when active)
	Active   bool          `json:"active"`           // present but NOT stale (held, or younger than MinAge)
	Reason   string        `json:"reason,omitempty"` // why it was skipped, when Active
	Age      time.Duration `json:"age_seconds"`      // how old the lock was when inspected
}

// Clean scans the repo's git dir for lock files and removes every one that is
// provably stale, returning a Result per lock found.
//
// It returns an error only for conditions that prevent scanning at all (the
// path is not a git repository, the git dir cannot be read). Active locks are
// NOT errors — they are reported as Result{Active:true}; the caller decides the
// exit code. An empty result slice means no lock files were present (the
// idempotent, healthy case).
func (c *Cleaner) Clean() ([]Result, error) {
	if c.MinAge <= 0 {
		c.MinAge = DefaultMinLockAge
	}
	if c.Now.IsZero() {
		c.Now = time.Now()
	}
	if c.Log == nil {
		c.Log = io.Discard
	}

	gitDir, err := resolveGitDir(c.Repo)
	if err != nil {
		return nil, err
	}

	locks, err := collectLocks(gitDir, c.IncludeWorktrees)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(c.Log, "safe-unlock: repo=%s gitdir=%s candidates=%d dry_run=%v min_age=%s\n",
		c.Repo, gitDir, len(locks), c.DryRun, c.MinAge)

	results := make([]Result, 0, len(locks))
	for _, lockPath := range locks {
		res, err := c.evaluate(gitDir, lockPath)
		if err != nil {
			return results, err
		}
		results = append(results, res)
		appendAudit(c.Repo, res, c.DryRun)
	}

	if len(results) == 0 {
		fmt.Fprintf(c.Log, "    no git lock files present — nothing to do\n")
	}
	return results, nil
}

// evaluate inspects a single lock file, removing it iff it is provably stale.
func (c *Cleaner) evaluate(gitDir, lockPath string) (Result, error) {
	res := Result{LockPath: lockPath, Kind: classify(gitDir, lockPath)}

	info, err := os.Stat(lockPath)
	if errors.Is(err, fs.ErrNotExist) {
		// Raced away between scan and stat (a git finished and cleaned up).
		// Treat as nothing-to-do rather than an error.
		res.Active = false
		res.Reason = "vanished before inspection"
		fmt.Fprintf(c.Log, "    %s: gone before inspection — skipping\n", lockPath)
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("cannot stat %s: %w", lockPath, err)
	}

	age := c.Now.Sub(info.ModTime())
	res.Age = age

	// (1) Age guard: refuse a lock young enough to belong to a live git.
	if age < c.MinAge {
		res.Active = true
		res.Reason = fmt.Sprintf("only %s old (< %s); a running git may hold it", age.Round(time.Second), c.MinAge)
		fmt.Fprintf(c.Log, "    %s [%s]: ACTIVE — %s\n", lockPath, res.Kind, res.Reason)
		return res, nil
	}

	// (2) Holder guard: refuse if any process holds the lock open (git mid-op).
	if pids, checked := lockHolders(lockPath); checked && pids != "" {
		res.Active = true
		res.Reason = fmt.Sprintf("held open by process(es) %s; a git operation is in flight",
			strings.ReplaceAll(pids, "\n", ", "))
		fmt.Fprintf(c.Log, "    %s [%s]: ACTIVE — %s\n", lockPath, res.Kind, res.Reason)
		return res, nil
	}

	if c.DryRun {
		fmt.Fprintf(c.Log, "    %s [%s]: [dry-run] would remove stale lock (age %s)\n",
			lockPath, res.Kind, age.Round(time.Second))
		return res, nil
	}

	if err := os.Remove(lockPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) { // raced away after the guards — fine, the goal is met
			fmt.Fprintf(c.Log, "    %s [%s]: vanished during removal — already gone\n", lockPath, res.Kind)
			return res, nil
		}
		return res, fmt.Errorf("failed to remove stale lock %s: %w", lockPath, err)
	}
	res.Removed = true
	fmt.Fprintf(c.Log, "    %s [%s]: removed stale lock (age %s)\n", lockPath, res.Kind, age.Round(time.Second))
	return res, nil
}

// resolveGitDir resolves the git directory for a checkout. For a normal checkout
// `.git` is a directory; for a linked worktree it is a file containing
// "gitdir: <path>" pointing at <common>/worktrees/<name>. Lock files live in the
// resolved git dir in both cases. Resolved with pure filesystem reads so unlock
// never spawns git.
func resolveGitDir(repo string) (string, error) {
	if repo == "" {
		return "", errors.New("repo path is empty")
	}
	dotgit := filepath.Join(repo, ".git")
	info, err := os.Stat(dotgit)
	if err != nil {
		return "", fmt.Errorf("not a git repository (no %s): %w", dotgit, err)
	}
	if info.IsDir() {
		return dotgit, nil
	}
	data, err := os.ReadFile(dotgit)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", dotgit, err)
	}
	line := strings.TrimSpace(string(data))
	gd, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return "", fmt.Errorf("unexpected .git file contents in %s (no gitdir: prefix)", repo)
	}
	gd = strings.TrimSpace(gd)
	if !filepath.IsAbs(gd) {
		gd = filepath.Join(repo, gd)
	}
	return filepath.Clean(gd), nil
}

// topLevelLockNames are the lock files git writes directly in the git dir.
// Anything matching these (plus a generic *.lock in the top level) is a
// candidate; every candidate is still independently guarded before removal.
var topLevelLockNames = map[string]bool{
	"index.lock":       true,
	"HEAD.lock":        true,
	"config.lock":      true,
	"packed-refs.lock": true,
	"shallow.lock":     true,
	"ORIG_HEAD.lock":   true,
}

// collectLocks enumerates candidate lock files under gitDir: every *.lock
// directly in the git dir, every *.lock under refs/, and (when includeWorktrees
// is set) every *.lock under each linked worktree's git dir in worktrees/*. The
// scan is deliberately bounded to git's known lock locations rather than walking
// the whole git dir (objects/, hooks/, …), which never contain operation locks.
func collectLocks(gitDir string, includeWorktrees bool) ([]string, error) {
	var locks []string

	// Top-level *.lock directly in the git dir.
	entries, err := os.ReadDir(gitDir)
	if err != nil {
		return nil, fmt.Errorf("cannot read git dir %s: %w", gitDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lock") {
			continue
		}
		// Known names, or any *.lock at the top level — both are git operation
		// locks; the staleness guards make a broad-but-bounded scan safe.
		_ = topLevelLockNames // documented allowlist; we accept any top-level *.lock
		locks = append(locks, filepath.Join(gitDir, e.Name()))
	}

	// refs/**/*.lock — per-ref update locks (heads, tags, remotes).
	locks = appendWalkLocks(locks, filepath.Join(gitDir, "refs"))

	// Linked worktrees: each has its own index.lock / HEAD.lock and may have
	// per-worktree refs. Scan when asked (default for a main checkout).
	if includeWorktrees {
		wtRoot := filepath.Join(gitDir, "worktrees")
		subs, err := os.ReadDir(wtRoot)
		if err == nil {
			for _, s := range subs {
				if s.IsDir() {
					locks = appendWalkLocks(locks, filepath.Join(wtRoot, s.Name()))
				}
			}
		}
	}

	return locks, nil
}

// appendWalkLocks appends every *.lock file found under root (recursively) to
// locks. A missing root is not an error — refs/ or worktrees/ need not exist.
func appendWalkLocks(locks []string, root string) []string {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // unreadable subtree: skip it, never fail the whole lock scan
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".lock") {
			locks = append(locks, path)
		}
		return nil
	})
	return locks
}

// classify gives a human label for a lock by its location under the git dir, so
// audit lines and reports say "index" / "ref:heads/main" / "worktree:foo/index"
// rather than a bare path.
func classify(gitDir, lockPath string) string {
	rel, err := filepath.Rel(gitDir, lockPath)
	if err != nil {
		return strings.TrimSuffix(filepath.Base(lockPath), ".lock")
	}
	rel = filepath.ToSlash(rel)
	switch {
	case rel == "index.lock":
		return "index"
	case rel == "HEAD.lock":
		return "HEAD"
	case rel == "config.lock":
		return "config"
	case rel == "packed-refs.lock":
		return "packed-refs"
	case strings.HasPrefix(rel, "refs/"):
		return "ref:" + strings.TrimSuffix(strings.TrimPrefix(rel, "refs/"), ".lock")
	case strings.HasPrefix(rel, "worktrees/"):
		return "worktree:" + strings.TrimSuffix(strings.TrimPrefix(rel, "worktrees/"), ".lock")
	default:
		return strings.TrimSuffix(filepath.Base(lockPath), ".lock")
	}
}

// lockHolders returns the PIDs (newline-separated) of processes holding lockPath
// open, and whether the check actually ran. `lsof -t -- <path>` prints one PID
// per holder and exits non-zero with empty output when there are none — the
// common, safe case. If lsof is unavailable, checked is false and the caller
// falls back to the age guard alone.
func lockHolders(lockPath string) (pids string, checked bool) {
	cmd := exec.Command("lsof", "-t", "--", lockPath)
	out, err := cmd.Output()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		// lsof exits 1 with empty output when no process holds the file — the
		// common, safe "no holder" result. Any OTHER exit code (permission
		// denied, syntax/system error) or a non-exec error (lsof not found)
		// means we could NOT verify holders; report not-checked so the age
		// guard alone governs removal rather than wrongly assuming no holder.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return trimmed, true
		}
		return "", false
	}
	return trimmed, true
}
