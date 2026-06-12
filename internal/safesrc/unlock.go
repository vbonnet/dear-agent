package safesrc

// Stale-lock removal — the second thing safesrc is allowed to do to a golden
// ~/src/<repo> checkout, and the only file it will ever delete.
//
// # Why this exists
//
// The daily archive sync (a launchd job) rsyncs new session logs into
// ~/src/ai-conversation-logs and then `git commit`s them. If a previous git was
// killed mid-index-update, it leaves a zero-byte `.git/index.lock` behind. Every
// subsequent commit then fails with exit 128 ("Unable to create index.lock:
// File exists"), so the rsync keeps succeeding while the commit keeps failing —
// a silent, compounding backlog (14 days, in the incident that motivated this:
// see vbonnet-ai-auq).
//
// Clearing it means `rm ~/src/<repo>/.git/index.lock` — a raw write into ~/src,
// exactly what the deny-guards forbid. So an agent either can't fix it or routes
// around the guard. Unlock is the atomic-wrapper answer (CLAUDE.md principle 9):
// the one vetted path that removes a *provably stale* index.lock and nothing
// else.
//
// # Why it is safe by construction
//
// A real index.lock exists only for the milliseconds git takes to write the new
// index and rename it over `index`. Unlock removes a lock only when BOTH:
//
//  1. it is older than MinAge (default 60s) — far longer than any live index
//     update — so a lock a running git just created is never touched, and
//  2. no process currently holds it open (verified with `lsof`).
//
// It only ever targets `<gitdir>/index.lock`; the path is computed, never taken
// from an argument, so there is no file an agent can name for deletion.
import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultMinLockAge is how old a `.git/index.lock` must be before Unlock will
// treat it as stale debris. A live index update holds the lock for
// milliseconds; 60s is orders of magnitude beyond that, so this never races a
// running git while still clearing any lock a crashed/killed one left behind.
const DefaultMinLockAge = 60 * time.Second

// Unlocker removes a single stale git index.lock from a validated ~/src repo.
// Repo must already have passed ValidateRepo, so the boundary check is shared
// with recovery and cannot be skipped.
type Unlocker struct {
	Repo   string        // validated worktree root (from ValidateRepo)
	DryRun bool          // report what would happen; remove nothing
	MinAge time.Duration // min lock age to consider stale; DefaultMinLockAge if zero
	Now    time.Time     // injected clock (binary passes time.Now(); tests fix it)
	Log    io.Writer     // audit sink; the decision and outcome are written here
}

// UnlockResult records what Unlock decided, for the caller to report.
type UnlockResult struct {
	LockPath string        // <gitdir>/index.lock that was inspected
	Existed  bool          // whether a lock file was present at all
	Removed  bool          // whether it was removed (false in dry-run or when absent)
	Age      time.Duration // how old the lock was when inspected
}

// Unlock removes <repo>/.git/index.lock iff it is provably stale, and reports
// what it did.
//
// Outcomes:
//   - no lock present            → Existed=false, Removed=false, nil error (idempotent).
//   - lock younger than MinAge   → error (a live git may hold it; do not race it).
//   - a process holds it open    → error, naming the holding PIDs (a real git is mid-update).
//   - stale + DryRun             → Existed=true, Removed=false, nil error.
//   - stale                      → Existed=true, Removed=true, nil error.
func (u *Unlocker) Unlock() (UnlockResult, error) {
	if u.MinAge <= 0 {
		u.MinAge = DefaultMinLockAge
	}
	gitDir, err := gitDir(u.Repo)
	if err != nil {
		return UnlockResult{}, err
	}
	lockPath := filepath.Join(gitDir, "index.lock")
	res := UnlockResult{LockPath: lockPath}

	fmt.Fprintf(u.Log, "src-recovery unlock: repo=%s lock=%s dry_run=%v\n", u.Repo, lockPath, u.DryRun)

	info, err := os.Stat(lockPath)
	if os.IsNotExist(err) {
		fmt.Fprintf(u.Log, "    no index.lock present — nothing to do\n")
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("cannot stat %s: %w", lockPath, err)
	}
	res.Existed = true

	// (1) Age guard: refuse to touch a lock young enough to belong to a live git.
	age := u.Now.Sub(info.ModTime())
	res.Age = age
	if age < u.MinAge {
		return res, fmt.Errorf("refusing to remove %s: it is only %s old (< %s) — a running git may "+
			"hold it; wait for it to age out or confirm no git is in flight, then re-run",
			lockPath, age.Round(time.Second), u.MinAge)
	}

	// (2) Holder guard: refuse if any process has the lock open (git mid-update).
	if pids, checked := lockHolders(lockPath); checked && pids != "" {
		return res, fmt.Errorf("refusing to remove %s: process(es) %s currently hold it open — a git "+
			"operation is in flight; do not remove a held lock", lockPath, strings.ReplaceAll(pids, "\n", ", "))
	}

	if u.DryRun {
		fmt.Fprintf(u.Log, "    [dry-run] would remove stale lock (age %s)\n", age.Round(time.Second))
		return res, nil
	}

	if err := os.Remove(lockPath); err != nil {
		return res, fmt.Errorf("failed to remove stale lock %s: %w", lockPath, err)
	}
	res.Removed = true
	fmt.Fprintf(u.Log, "    removed stale index.lock (age %s)\n", age.Round(time.Second))
	fmt.Fprintf(u.Log, "src-recovery unlock: done\n")
	return res, nil
}

// gitDir resolves the git directory for a checkout. For a normal checkout `.git`
// is a directory; for a linked worktree it is a file containing "gitdir: <path>".
// index.lock lives in the per-worktree git dir in both cases. Resolved with pure
// filesystem reads so unlock never spawns git and stays independent of the
// recovery verb-allowlist.
func gitDir(repo string) (string, error) {
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
	return gd, nil
}

// lockHolders returns the PIDs (newline-separated) of processes holding lockPath
// open, and whether the check actually ran. `lsof -t -- <path>` prints one PID
// per holder and exits non-zero with empty output when there are none — that is
// the common, safe case. If lsof is unavailable (rare on macOS), checked is
// false and the caller falls back to the age guard alone.
func lockHolders(lockPath string) (pids string, checked bool) {
	cmd := exec.Command("lsof", "-t", "--", lockPath)
	out, err := cmd.Output()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		// An ExitError with empty output means "no process holds it" — a real,
		// checked result. Any other error (e.g. lsof not found) means we could
		// not verify; report not-checked so age remains the only guard.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return trimmed, true
		}
		return "", false
	}
	return trimmed, true
}
