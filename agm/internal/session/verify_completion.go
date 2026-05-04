package session

import (
	"fmt"
	"os/exec"
	"strings"
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
}

// Critical returns true if any blocking issue was found that should prevent
// archival without --force.
func (v *CompletionVerification) Critical() bool {
	return len(v.UncommittedFiles) > 0 || len(v.UnmergedCommits) > 0 || v.MissingTests
}

// CriticalErrors returns human-readable descriptions of each blocking issue.
func (v *CompletionVerification) CriticalErrors() []string {
	var errs []string
	if n := len(v.UncommittedFiles); n > 0 {
		errs = append(errs, fmt.Sprintf("uncommitted changes in %d file(s)", n))
	}
	if n := len(v.UnmergedCommits); n > 0 {
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

// collectUnmergedCommits populates result.UnmergedCommits from
// `git log main..HEAD --oneline`.
func collectUnmergedCommits(dir string, result *CompletionVerification) {
	logOut, err := exec.Command("git", "-C", dir, "log", "main..HEAD", "--oneline").Output()
	if err != nil {
		return
	}
	for _, c := range strings.Split(strings.TrimSpace(string(logOut)), "\n") {
		if cc := strings.TrimSpace(c); cc != "" {
			result.UnmergedCommits = append(result.UnmergedCommits, cc)
		}
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
}
