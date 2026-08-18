package session

import (
	"fmt"
	"os/exec"
	"strings"

	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
)

// CompletionVerification holds the results of pre-archive verification checks.
// It inspects git history for signs of incomplete work.
type CompletionVerification struct {
	HasCodeChanges   bool     `json:"has_code_changes"`
	HasTestChanges   bool     `json:"has_test_changes"`
	DeferralWarnings []string `json:"deferral_warnings,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`

	// Blocking fields — these prevent archive without --force.
	UncommittedFiles []string `json:"uncommitted_files,omitempty"`
	UnmergedCommits  []string `json:"unmerged_commits,omitempty"`
	MissingTests     bool     `json:"missing_tests"`

	// HasOpenPR and OpenPRNumber report whether the branch that carries
	// UnmergedCommits has a confirmed OPEN pull request. This is
	// non-blocking evidence: an open PR means the unmerged work is tracked
	// and in flight, not abandoned, so it downgrades UnmergedCommits from a
	// hard block to a warning (see Critical). It is only populated when
	// UnmergedCommits is non-empty — a clean branch never needs a PR check.
	HasOpenPR    bool `json:"has_open_pr,omitempty"`
	OpenPRNumber int  `json:"open_pr_number,omitempty"`
}

// Critical returns true if any blocking issue was found that should prevent
// archival without --force.
//
// Unmerged commits are only critical when there is no confirmed open PR for
// the branch: "resolved" means merged, has an open PR, or is explicitly
// force-archived — not merely "git can't currently prove it's merged".
func (v *CompletionVerification) Critical() bool {
	return len(v.UncommittedFiles) > 0 || (len(v.UnmergedCommits) > 0 && !v.HasOpenPR) || v.MissingTests
}

// CriticalErrors returns human-readable descriptions of each blocking issue.
func (v *CompletionVerification) CriticalErrors() []string {
	var errs []string
	if n := len(v.UncommittedFiles); n > 0 {
		errs = append(errs, fmt.Sprintf("uncommitted changes in %d file(s)", n))
	}
	if n := len(v.UnmergedCommits); n > 0 && !v.HasOpenPR {
		errs = append(errs, fmt.Sprintf("branch has %d unmerged commit(s)", n))
	}
	if v.MissingTests {
		errs = append(errs, "code changes detected without corresponding test changes")
	}
	return errs
}

// deferralPatterns are case-insensitive substrings that suggest deferred work.
var deferralPatterns = []string{
	"todo",
	"wip",
	"fixme",
	"hack",
	"defer",
	"temporary",
	"placeholder",
}

// codeExtensions are file suffixes that indicate source code changes.
var codeExtensions = []string{
	".go", ".py", ".js", ".ts", ".tsx", ".jsx",
	".rs", ".java", ".rb", ".c", ".cpp", ".h",
	".cs", ".swift", ".kt", ".sh", ".sql",
}

// testPatterns are substrings/suffixes that indicate test file changes.
var testPatterns = []string{
	"_test.go",
	"test_",
	".test.js",
	".test.ts",
	".test.tsx",
	".spec.js",
	".spec.ts",
	".spec.tsx",
	"_test.py",
	"_test.rs",
	"Test.java",
	"_test.rb",
}

// VerifyCompletion checks a session's working directory for signs of incomplete work.
// It examines recent git commits for code/test changes and deferral language,
// and performs blocking checks for uncommitted changes and unmerged branches.
// Returns a zero-value result if the directory is not a git repo or git is unavailable.
func VerifyCompletion(dir string) *CompletionVerification {
	result := &CompletionVerification{}
	if dir == "" {
		return result
	}
	if !collectUncommittedFiles(dir, result) {
		return result
	}
	collectUnmergedCommits(dir, result)
	messages, ok := readRecentCommitMessages(dir)
	if !ok {
		return result
	}
	collectDeferralWarnings(messages, result)
	if !classifyCommittedFileTypes(dir, result) {
		return result
	}
	if result.HasCodeChanges && !result.HasTestChanges {
		result.MissingTests = true
	}
	buildVerifyWarnings(messages, result)
	return result
}

// collectUncommittedFiles populates result.UncommittedFiles from
// `git status --porcelain`. Returns false if git is unavailable / not a repo
// (caller should bail out with an empty result).
func collectUncommittedFiles(dir string, result *CompletionVerification) bool {
	statusOut, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(statusOut)), "\n") {
		if l := strings.TrimSpace(line); l != "" {
			result.UncommittedFiles = append(result.UncommittedFiles, l)
		}
	}
	return true
}

// openPRForBranch resolves the open PR (if any) for a branch. It is a
// package-level var — rather than a direct gitpkg.OpenPRForBranch call — so
// tests can stub it out and stay hermetic (no gh/network dependency).
var openPRForBranch = gitpkg.OpenPRForBranch

// collectUnmergedCommits populates result.UnmergedCommits by comparing HEAD
// against the resolved base branch (origin/HEAD → origin/main → main, via
// gitpkg.ResolveBaseRef — the same resolution the worktree sweep and
// archive-ui safety checks already use), instead of a hard-coded "main".
//
// It fails CLOSED: if there are no commits yet (nothing could possibly be
// unmerged) it is a no-op, but any other failure to positively establish
// "no unmerged commits" — an unresolvable base ref, or a git error counting
// commits — populates UnmergedCommits with a synthetic entry describing the
// failure. This mirrors gitpkg.CommitsAhead's documented contract ("on any
// error, return -1 = has unmerged work"): a verification failure must never
// silently look like a clean branch.
//
// When UnmergedCommits ends up non-empty (real commits or a fail-closed
// synthetic entry), it also checks for a confirmed open PR on the current
// branch and records it in HasOpenPR/OpenPRNumber. Critical() downgrades
// unmerged-with-an-open-PR to non-blocking — the retro's "resolved := merged
// | has-open-PR | explicitly-abandoned" rule — while unmerged-with-no-PR
// still blocks. The PR check is itself fail-closed (OpenPRForBranch reports
// "known=false" on any gh/network/auth error), so a flaky or missing gh
// never manufactures a bypass.
func collectUnmergedCommits(dir string, result *CompletionVerification) {
	// No commits yet (freshly initialized repo/worktree): nothing could be
	// unmerged. This is the one legitimate reason to skip the check — every
	// other failure below fails closed.
	if err := exec.Command("git", "-C", dir, "rev-parse", "--verify", "--quiet", "HEAD").Run(); err != nil {
		return
	}

	base := gitpkg.ResolveBaseRef(dir)
	if base == "" {
		result.UnmergedCommits = append(result.UnmergedCommits,
			"unable to resolve base branch (origin/HEAD, origin/main, main all unavailable) — treating branch as unmerged")
		checkOpenPR(dir, result)
		return
	}

	n, err := gitpkg.CommitsAhead(dir, "HEAD", base)
	if err != nil || n < 0 {
		result.UnmergedCommits = append(result.UnmergedCommits,
			fmt.Sprintf("unable to verify commits against %s: %v — treating branch as unmerged", base, err))
		checkOpenPR(dir, result)
		return
	}
	if n == 0 {
		return
	}

	logOut, logErr := exec.Command("git", "-C", dir, "log", base+"..HEAD", "--oneline").Output()
	if logErr != nil {
		// CommitsAhead just proved n>0 commits exist; a failure listing them
		// here is transient/inconsistent, not evidence of a clean branch —
		// fail closed with the count we already have.
		result.UnmergedCommits = append(result.UnmergedCommits,
			fmt.Sprintf("%d commit(s) ahead of %s (unable to list details: %v)", n, base, logErr))
	} else {
		for _, c := range strings.Split(strings.TrimSpace(string(logOut)), "\n") {
			if cc := strings.TrimSpace(c); cc != "" {
				result.UnmergedCommits = append(result.UnmergedCommits, cc)
			}
		}
	}
	checkOpenPR(dir, result)
}

// checkOpenPR resolves the current branch in dir and, if a confirmed open PR
// exists for it, records HasOpenPR/OpenPRNumber on result. Any failure to
// resolve the branch or the PR state leaves HasOpenPR false (fail closed —
// unmerged commits stay blocking unless a PR is positively confirmed open).
func checkOpenPR(dir string, result *CompletionVerification) {
	branchOut, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return
	}
	branch := strings.TrimSpace(string(branchOut))
	if branch == "" || branch == "HEAD" {
		return // detached HEAD — no branch to look up a PR for
	}
	if num, known := openPRForBranch(dir, branch); known {
		result.HasOpenPR = true
		result.OpenPRNumber = num
	}
}

// readRecentCommitMessages returns the last 50 commit subject lines.
func readRecentCommitMessages(dir string) ([]string, bool) {
	msgOut, err := exec.Command("git", "-C", dir, "log", "--oneline", "-50", "--format=%s").Output()
	if err != nil {
		return nil, false
	}
	messages := strings.Split(strings.TrimSpace(string(msgOut)), "\n")
	if len(messages) == 1 && messages[0] == "" {
		messages = nil
	}
	return messages, true
}

// collectDeferralWarnings appends commit messages containing deferral language
// to result.DeferralWarnings.
func collectDeferralWarnings(messages []string, result *CompletionVerification) {
	for _, msg := range messages {
		lower := strings.ToLower(msg)
		for _, pattern := range deferralPatterns {
			if strings.Contains(lower, pattern) {
				result.DeferralWarnings = append(result.DeferralWarnings, msg)
				break
			}
		}
	}
}

// classifyCommittedFileTypes inspects recent commit file paths to set
// result.HasCodeChanges and result.HasTestChanges.
func classifyCommittedFileTypes(dir string, result *CompletionVerification) bool {
	filesOut, err := exec.Command("git", "-C", dir, "log", "--oneline", "-50", "--name-only", "--format=").Output()
	if err != nil {
		return false
	}
	for _, f := range strings.Split(strings.TrimSpace(string(filesOut)), "\n") {
		ff := strings.TrimSpace(f)
		if ff == "" {
			continue
		}
		lower := strings.ToLower(ff)
		if !result.HasTestChanges {
			for _, tp := range testPatterns {
				if strings.Contains(lower, tp) {
					result.HasTestChanges = true
					break
				}
			}
		}
		if !result.HasCodeChanges {
			for _, ext := range codeExtensions {
				if strings.HasSuffix(lower, ext) {
					result.HasCodeChanges = true
					break
				}
			}
		}
		if result.HasCodeChanges && result.HasTestChanges {
			break
		}
	}
	return true
}

// buildVerifyWarnings appends summary warnings to result.Warnings based on
// what was/wasn't found in the commit history.
func buildVerifyWarnings(messages []string, result *CompletionVerification) {
	if len(messages) > 0 && !result.HasCodeChanges {
		result.Warnings = append(result.Warnings, "no code changes detected in recent commits")
	}
	if len(messages) > 0 && !result.HasTestChanges {
		result.Warnings = append(result.Warnings, "no test changes detected in recent commits")
	}
	if len(result.DeferralWarnings) > 0 {
		result.Warnings = append(result.Warnings, "deferral language detected in commit messages")
	}
	if len(result.UnmergedCommits) > 0 && result.HasOpenPR {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"branch has %d unmerged commit(s) but open PR #%d covers them (not blocking)",
			len(result.UnmergedCommits), result.OpenPRNumber))
	}
}
