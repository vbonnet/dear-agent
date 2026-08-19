package safegit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// RebaseConfig holds options for a safe rebase operation.
type RebaseConfig struct {
	RepoDir    string        // working directory; empty = cwd
	BaseBranch string        // branch to rebase onto; empty = "main"
	Auto       bool          // if true, push + preflight after clean rebase
	Timeout    time.Duration // timeout for network ops; zero = DefaultTimeout
	DryRun     bool          // report what would happen without doing it
}

// RebaseResult describes what happened.
type RebaseResult struct {
	Branch        string   // the feature branch that was rebased
	BaseBranch    string   // the branch it was rebased onto
	Conflicts     []string // conflict file paths (empty = clean rebase)
	CommitsBefore int      // commits ahead of base before rebase
	CommitsAfter  int      // commits ahead of base after rebase
	Pushed        bool     // whether the branch was force-pushed
	Preflight     bool     // whether preflight passed
}

// RebaseAuditEntry is written to the JSONL audit log.
type RebaseAuditEntry struct {
	Timestamp  string   `json:"timestamp"`
	Branch     string   `json:"branch"`
	BaseBranch string   `json:"base_branch"`
	Event      string   `json:"event"` // "rebase_clean", "rebase_conflict", "pushed", "preflight_pass", "preflight_fail", "error"
	Detail     string   `json:"detail,omitempty"`
	Conflicts  []string `json:"conflicts,omitempty"`
}

// SafeRebase fetches the latest base branch, rebases the current feature branch
// onto it, and optionally force-pushes + runs preflight (in --auto mode).
//
// Safety invariants:
//   - Refuses to operate on main/master (protected branches).
//   - On conflict: aborts the rebase, reports conflicts, and returns an error.
//   - Force-push is ONLY used on feature branches, never on protected branches.
//   - Network operations use GIT_TERMINAL_PROMPT=0 and a timeout.
func SafeRebase(cfg RebaseConfig) (*RebaseResult, error) {
	if cfg.BaseBranch == "" {
		cfg.BaseBranch = "main"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	dir := cfg.RepoDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("cannot determine working directory: %w", err)
		}
	}

	branch, err := currentBranch(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot determine current branch: %w", err)
	}
	if branch == "HEAD" {
		return nil, fmt.Errorf("refusing to rebase: repository is in a detached HEAD state")
	}
	if IsProtectedBranch(dir, branch) {
		return nil, fmt.Errorf("refusing to rebase %q — safe-rebase only operates on feature branches, "+
			"never on protected branches (main, master). Check out your feature branch first", branch)
	}

	result := &RebaseResult{
		Branch:     branch,
		BaseBranch: cfg.BaseBranch,
	}

	fmt.Fprintf(os.Stderr, "safe-rebase: branch %q onto %q\n", branch, cfg.BaseBranch)

	// Fetch latest base branch.
	fmt.Fprintf(os.Stderr, "safe-rebase: fetching origin/%s…\n", cfg.BaseBranch)
	if err := fetchBranch(dir, cfg.BaseBranch, cfg.Timeout); err != nil {
		appendRebaseAudit(branch, cfg.BaseBranch, "error", "fetch failed: "+err.Error(), nil)
		return nil, fmt.Errorf("fetch origin/%s failed: %w", cfg.BaseBranch, err)
	}

	// Count commits ahead before rebase.
	result.CommitsBefore = countAhead(dir, "origin/"+cfg.BaseBranch, branch)

	if cfg.DryRun {
		fmt.Fprintf(os.Stderr, "safe-rebase: dry-run — %d commits ahead of origin/%s; would rebase and %s\n",
			result.CommitsBefore, cfg.BaseBranch, autoDesc(cfg.Auto))
		return result, nil
	}

	// Attempt the rebase.
	fmt.Fprintf(os.Stderr, "safe-rebase: rebasing %d commit(s) onto origin/%s…\n",
		result.CommitsBefore, cfg.BaseBranch)
	conflicts, err := attemptRebase(dir, "origin/"+cfg.BaseBranch)
	if err != nil {
		result.Conflicts = conflicts
		appendRebaseAudit(branch, cfg.BaseBranch, "rebase_conflict",
			fmt.Sprintf("%d conflict(s)", len(conflicts)), conflicts)
		return result, fmt.Errorf("rebase failed with %d conflict(s) — the rebase has been aborted, "+
			"your branch is back to its pre-rebase state:\n  %s\n\n"+
			"To resolve: check out the branch, run `git rebase origin/%s` manually, "+
			"fix each conflict, then `git rebase --continue`",
			len(conflicts), strings.Join(conflicts, "\n  "), cfg.BaseBranch)
	}
	fmt.Fprintln(os.Stderr, "safe-rebase: ✓ rebase clean")
	appendRebaseAudit(branch, cfg.BaseBranch, "rebase_clean", "", nil)

	result.CommitsAfter = countAhead(dir, "origin/"+cfg.BaseBranch, branch)

	if !cfg.Auto {
		fmt.Fprintln(os.Stderr, "safe-rebase: rebase complete — run `safe-push -u origin "+branch+"` to push")
		return result, nil
	}

	// Auto mode: force-push + preflight.
	fmt.Fprintf(os.Stderr, "safe-rebase: auto mode — force-pushing %s…\n", branch)
	if err := forcePushFeatureBranch(dir, branch, cfg.Timeout); err != nil {
		appendRebaseAudit(branch, cfg.BaseBranch, "error", "push failed: "+err.Error(), nil)
		return result, fmt.Errorf("force-push failed: %w", err)
	}
	result.Pushed = true
	appendRebaseAudit(branch, cfg.BaseBranch, "pushed", "", nil)
	fmt.Fprintln(os.Stderr, "safe-rebase: ✓ pushed")

	fmt.Fprintln(os.Stderr, "safe-rebase: running make preflight…")
	if err := runPreflight(dir); err != nil {
		appendRebaseAudit(branch, cfg.BaseBranch, "preflight_fail", err.Error(), nil)
		return result, fmt.Errorf("preflight failed after rebase: %w", err)
	}
	result.Preflight = true
	appendRebaseAudit(branch, cfg.BaseBranch, "preflight_pass", "", nil)
	fmt.Fprintln(os.Stderr, "safe-rebase: ✓ preflight passed")

	return result, nil
}

// IsProtectedBranch returns true for branches that must never be force-pushed or rebased.
func IsProtectedBranch(repoDir, name string) bool {
	return ProtectedBranches(repoDir)[name]
}

func currentBranch(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func fetchBranch(dir, branch string, timeout time.Duration) error {
	cmd := exec.Command("gtimeout", fmt.Sprintf("%.0f", timeout.Seconds()),
		"git", "-C", dir, "fetch", "origin", branch)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func countAhead(dir, base, branch string) int {
	cmd := exec.Command("git", "-C", dir, "rev-list", "--count", base+".."+branch)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var n int
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &n)
	return n
}

// attemptRebase tries to rebase onto base. On conflict, it aborts and returns
// the list of conflicted files. On success, it returns nil.
func attemptRebase(dir, base string) (conflicts []string, err error) {
	cmd := exec.Command("git", "-C", dir, "rebase", base)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Parse conflicts from the status output.
		conflicts = parseConflicts(dir)

		// Abort the rebase to restore the branch.
		abort := exec.Command("git", "-C", dir, "rebase", "--abort")
		abort.Stderr = os.Stderr
		_ = abort.Run()

		if len(conflicts) == 0 {
			return nil, fmt.Errorf("rebase failed: %s", strings.TrimSpace(stderr.String()))
		}
		return conflicts, fmt.Errorf("%d conflict(s)", len(conflicts))
	}
	return nil, nil
}

func parseConflicts(dir string) []string {
	cmd := exec.Command("git", "-C", dir, "diff", "--name-only", "--diff-filter=U")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var files []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func forcePushFeatureBranch(dir, branch string, timeout time.Duration) error {
	if IsProtectedBranch(dir, branch) {
		return fmt.Errorf("refusing to force-push protected branch %q", branch)
	}

	cmd := exec.Command("gtimeout", fmt.Sprintf("%.0f", timeout.Seconds()),
		"git", "-C", dir,
		"-c", "credential.helper=",
		"-c", "credential.helper=!gh auth git-credential",
		"push", "--force-with-lease", "-u", "origin", branch)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runPreflight(dir string) error {
	cmd := exec.Command("make", "-C", dir, "preflight")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func autoDesc(auto bool) string {
	if auto {
		return "force-push + run preflight"
	}
	return "stop (manual push required)"
}

func appendRebaseAudit(branch, baseBranch, event, detail string, conflicts []string) {
	auditDir := auditLogDir()
	if err := os.MkdirAll(auditDir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(auditDir, "safe-rebase-audit.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	// Closing a writable handle can surface deferred write errors (flush
	// failures, full disk). The audit log is non-fatal — never block a
	// rebase on it — but a dropped close error means a silently corrupt log,
	// so log it via slog rather than discarding it.
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("safe-rebase: failed to close audit log", "error", cerr)
		}
	}()
	entry := RebaseAuditEntry{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		Branch:     branch,
		BaseBranch: baseBranch,
		Event:      event,
		Detail:     detail,
		Conflicts:  conflicts,
	}
	b, _ := json.Marshal(entry)
	if _, werr := fmt.Fprintln(f, string(b)); werr != nil {
		slog.Warn("safe-rebase: failed to write audit log entry", "error", werr)
	}
}
