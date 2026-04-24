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

	// Blocking check 1: uncommitted changes (git status --porcelain)
	statusOut, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		// Not a git repo or git unavailable — return empty result
		return result
	}
	statusLines := strings.Split(strings.TrimSpace(string(statusOut)), "\n")
	for _, line := range statusLines {
		line = strings.TrimSpace(line)
		if line != "" {
			result.UncommittedFiles = append(result.UncommittedFiles, line)
		}
	}

	// Blocking check 2: unmerged commits (git log main..HEAD)
	logOut, err := exec.Command("git", "-C", dir, "log", "main..HEAD", "--oneline").Output()
	if err == nil {
		commits := strings.Split(strings.TrimSpace(string(logOut)), "\n")
		for _, c := range commits {
			c = strings.TrimSpace(c)
			if c != "" {
				result.UnmergedCommits = append(result.UnmergedCommits, c)
			}
		}
	}

	// Get recent commit messages
	msgOut, err := exec.Command("git", "-C", dir, "log", "--oneline", "-50", "--format=%s").Output()
	if err != nil {
		return result
	}

	messages := strings.Split(strings.TrimSpace(string(msgOut)), "\n")
	if len(messages) == 1 && messages[0] == "" {
		messages = nil
	}

	// Scan for deferral language
	for _, msg := range messages {
		lower := strings.ToLower(msg)
		for _, pattern := range deferralPatterns {
			if strings.Contains(lower, pattern) {
				result.DeferralWarnings = append(result.DeferralWarnings, msg)
				break
			}
		}
	}

	// Get changed file paths from recent commits
	filesOut, err := exec.Command("git", "-C", dir, "log", "--oneline", "-50", "--name-only", "--format=").Output()
	if err != nil {
		return result
	}

	files := strings.Split(strings.TrimSpace(string(filesOut)), "\n")
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		lower := strings.ToLower(f)

		// Check test patterns first (tests are also code)
		if !result.HasTestChanges {
			for _, tp := range testPatterns {
				if strings.Contains(lower, tp) {
					result.HasTestChanges = true
					break
				}
			}
		}

		// Check code extensions
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

	// Blocking check 3: code changes without test changes
	if result.HasCodeChanges && !result.HasTestChanges {
		result.MissingTests = true
	}

	// Build warnings
	if len(messages) > 0 && !result.HasCodeChanges {
		result.Warnings = append(result.Warnings, "no code changes detected in recent commits")
	}
	if len(messages) > 0 && !result.HasTestChanges {
		result.Warnings = append(result.Warnings, "no test changes detected in recent commits")
	}
	if len(result.DeferralWarnings) > 0 {
		result.Warnings = append(result.Warnings, "deferral language detected in commit messages")
	}

	return result
}
