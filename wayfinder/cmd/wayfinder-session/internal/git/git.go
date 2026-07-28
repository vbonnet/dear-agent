// Package git provides git-related functionality.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
)

// GitIntegrator handles git operations for wayfinder sessions
type GitIntegrator struct {
	projectDir string
}

// New creates a new GitIntegrator for the given project directory
func New(projectDir string) *GitIntegrator {
	return &GitIntegrator{
		projectDir: projectDir,
	}
}

// IsGitRepo checks if the project directory is within a git repository
// Works correctly even when project is a subdirectory of a git repo
func (g *GitIntegrator) IsGitRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = g.projectDir
	err := cmd.Run()
	return err == nil
}

// IsGitWorktree reports whether the project directory is inside a Git work
// tree. Unlike IsGitRepo, it rejects bare repositories, which cannot retain
// Wayfinder lifecycle artifacts as working-tree files.
func (g *GitIntegrator) IsGitWorktree() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = g.projectDir
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) == "true"
}

// CommitPhaseCompletion creates a scoped git commit for phase completion.
// It includes canonical marker files and the phase's Markdown artifacts while
// preserving unrelated staged or unstaged user changes.
func (g *GitIntegrator) CommitPhaseCompletion(phase, outcome, context string) error {
	if !g.IsGitRepo() {
		return fmt.Errorf("project directory is not a git repository")
	}

	files := []string{
		"WAYFINDER-STATUS.md",
		history.HistoryFilename,
		history.LegacyHistoryFilename,
	}
	artifacts, err := filepath.Glob(filepath.Join(g.projectDir, phase+"-*.md"))
	if err != nil {
		return fmt.Errorf("find %s phase artifacts: %w", phase, err)
	}
	for _, artifact := range artifacts {
		files = append(files, filepath.Base(artifact))
	}
	if phase == "DESIGN" {
		files = append(files, "ARCHITECTURE.md")
		adrs, err := filepath.Glob(filepath.Join(g.projectDir, "ADR-*.md"))
		if err != nil {
			return fmt.Errorf("find DESIGN ADR artifacts: %w", err)
		}
		for _, adr := range adrs {
			files = append(files, filepath.Base(adr))
		}
	}

	if err := g.commitScoped(g.formatCommitMessage(phase, outcome, context), files); err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}
	return nil
}

// CommitRewind commits the status and retrospective markers produced by a
// rewind so the documented next start-phase command sees a clean project.
func (g *GitIntegrator) CommitRewind(fromPhase, toPhase string) error {
	if !g.IsGitRepo() {
		return fmt.Errorf("project directory is not a git repository")
	}
	message := fmt.Sprintf("wayfinder: rewind %s to %s\n\nWayfinder-Event: rewind", fromPhase, toPhase)
	return g.commitScoped(message, []string{
		"WAYFINDER-STATUS.md",
		history.HistoryFilename,
		history.LegacyHistoryFilename,
		"RETRO-retrospective.md",
	})
}

// CommitPhaseStart creates a git commit for phase start.
// Adds WAYFINDER-STATUS.md and WAYFINDER-HISTORY.jsonl to staging and commits so
// the worktree is clean before any deliverable work begins. Without this, the
// next start-phase call finds uncommitted marker files and refuses (ce-fvkz).
func (g *GitIntegrator) CommitPhaseStart(phase string) error {
	if !g.IsGitRepo() {
		return fmt.Errorf("project directory is not a git repository")
	}

	markerFiles := []string{
		"WAYFINDER-STATUS.md",
		history.HistoryFilename,
		history.LegacyHistoryFilename,
	}

	// Track which files were successfully staged so we can scope the commit to
	// exactly those files. Using `git commit -- <files>` prevents accidentally
	// sweeping up any other staged changes the user may have queued separately.
	var staged []string
	for _, file := range markerFiles {
		present, err := g.pathExistsOrTracked(file)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		if err := g.gitAdd(file); err != nil {
			return fmt.Errorf("failed to add %s: %w", file, err)
		}
		staged = append(staged, file)
	}

	if len(staged) == 0 {
		return nil
	}

	commitMsg := fmt.Sprintf("wayfinder: start %s\n\nWayfinder-Phase: %s\nWayfinder-Event: started", phase, phase)
	args := append([]string{"commit", "-m", commitMsg, "--"}, staged...)
	cmd := exec.Command("git", args...)
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// CommitSessionInit commits WAYFINDER-STATUS.md after a new session is created
// so that the immediately following `start-phase CHARTER` sees a clean worktree
// and does not refuse with "uncommitted files detected" (ce-11fi bootstrap fix).
//
// Unlike CommitPhaseStart, this method is lenient: it returns nil when the
// directory is not inside a git repository (non-git workflows are valid).
func (g *GitIntegrator) CommitSessionInit(projectName string) error {
	if !g.IsGitRepo() {
		return nil // not an error — non-git workflows are allowed
	}

	statusFile := filepath.Join(g.projectDir, "WAYFINDER-STATUS.md")
	if _, err := os.Stat(statusFile); os.IsNotExist(err) {
		return nil // nothing to commit
	}

	if err := g.gitAdd("WAYFINDER-STATUS.md"); err != nil {
		return fmt.Errorf("failed to add WAYFINDER-STATUS.md: %w", err)
	}

	commitMsg := fmt.Sprintf("wayfinder: init session %s\n\nWayfinder-Event: session-started", projectName)
	cmd := exec.Command("git", "commit", "-m", commitMsg, "--", "WAYFINDER-STATUS.md")
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit failed: %w (output: %s)", err, string(output))
	}

	return nil
}

// formatCommitMessage creates a standardized commit message for phase completion
func (g *GitIntegrator) formatCommitMessage(phase, outcome, context string) string {
	var msg strings.Builder

	// Subject line
	fmt.Fprintf(&msg, "wayfinder: complete %s (%s)", phase, outcome)

	// Add context if provided
	if context != "" {
		msg.WriteString("\n\n")
		msg.WriteString(context)
	}

	// Add metadata footer
	msg.WriteString("\n\n")
	msg.WriteString("Wayfinder-Phase: " + phase + "\n")
	msg.WriteString("Wayfinder-Outcome: " + outcome)

	return msg.String()
}

// gitAdd runs git add for a file
func (g *GitIntegrator) gitAdd(file string) error {
	cmd := exec.Command("git", "add", "--", file)
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git add failed: %w (output: %s)", err, string(output))
	}
	return nil
}

func (g *GitIntegrator) commitScoped(message string, candidates []string) error {
	seen := make(map[string]bool, len(candidates))
	staged := make([]string, 0, len(candidates))
	for _, file := range candidates {
		if seen[file] {
			continue
		}
		seen[file] = true
		present, err := g.pathExistsOrTracked(file)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		if err := g.gitAdd(file); err != nil {
			return fmt.Errorf("failed to add %s: %w", file, err)
		}
		staged = append(staged, file)
	}
	if len(staged) == 0 {
		return nil
	}
	args := append([]string{"commit", "-m", message, "--"}, staged...)
	cmd := exec.Command("git", args...)
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("git commit failed: %w (output: %s)", err, string(output))
	}
	return nil
}

func (g *GitIntegrator) pathExistsOrTracked(file string) (bool, error) {
	if _, err := os.Stat(filepath.Join(g.projectDir, file)); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat %s: %w", file, err)
	}
	cmd := exec.Command("git", "ls-files", "--error-unmatch", "--", file)
	cmd.Dir = g.projectDir
	if err := cmd.Run(); err == nil {
		return true, nil
	} else if _, ok := err.(*exec.ExitError); ok {
		return false, nil
	} else {
		return false, fmt.Errorf("check tracked path %s: %w", file, err)
	}
}

// GetCommitHash returns the current HEAD commit hash
func (g *GitIntegrator) GetCommitHash() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// sourceCodeExtensions defines file extensions that are considered source code
var sourceCodeExtensions = map[string]bool{
	".go":   true, // Go
	".py":   true, // Python
	".js":   true, // JavaScript
	".ts":   true, // TypeScript
	".jsx":  true, // React JavaScript
	".tsx":  true, // React TypeScript
	".c":    true, // C
	".cpp":  true, // C++
	".java": true, // Java
	".rb":   true, // Ruby
	".rs":   true, // Rust
	".php":  true, // PHP
}

// isSourceCodeFile returns true if file extension matches source code allowlist
func isSourceCodeFile(filePath string) bool {
	ext := filepath.Ext(filePath)
	return sourceCodeExtensions[ext]
}

// isInProjectDir returns true if file is under project directory
func isInProjectDir(filePath, projectDir string) bool {
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return false // Conservative: if can't resolve, assume NOT in project-dir
	}

	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return false
	}

	// Add separator to avoid false matches (e.g., ~/project-dir-2 matching ~/project-dir)
	return strings.HasPrefix(absFilePath, absProjectDir+string(filepath.Separator))
}

// GetUncommittedFilesInProjectDir returns list of uncommitted/unstaged files
// within the project directory, excluding .wayfinder/ internal metadata.
//
// Algorithm:
//  1. Run: git status --porcelain . (from projectDir context)
//  2. Parse output for ANY modified/unstaged files
//  3. Filter out .wayfinder/ directory paths
//  4. Return remaining files
//
// Returns empty list if:
//   - No git repo found (not an error)
//   - No uncommitted files in project directory
//   - Only .wayfinder/ files are uncommitted
func (g *GitIntegrator) GetUncommittedFilesInProjectDir() ([]string, error) {
	// Check if git repo exists
	if !g.IsGitRepo() {
		return []string{}, nil // Not an error - return empty list
	}

	// Execute git status --porcelain . to check only project directory
	cmd := exec.Command("git", "status", "--porcelain", ".")
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w (output: %s)", err, string(output))
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	var uncommittedFiles []string

	for _, line := range lines {
		if len(line) < 3 {
			continue // Skip empty lines
		}

		// All non-empty status lines indicate uncommitted/unstaged files
		statusCode := line[0:2]
		filePath := strings.TrimSpace(line[3:])

		// Skip ignored files
		if statusCode == "!!" {
			continue
		}

		// Skip .wayfinder/ internal directory
		if strings.HasPrefix(filePath, ".wayfinder/") || strings.Contains(filePath, "/.wayfinder/") {
			continue
		}

		uncommittedFiles = append(uncommittedFiles, filePath)
	}

	return uncommittedFiles, nil
}

// GetModifiedSourceFiles returns list of modified/added source code files
// in the target repository, excluding project-dir and /tmp/ files.
//
// Algorithm:
//  1. Run: git status --porcelain (from projectDir context)
//  2. Parse output for modified (M) and added (A) files
//  3. Filter out files in project-dir (research code allowed)
//  4. Filter out files in /tmp/ (temporary files allowed)
//  5. Filter by source code extensions (.go, .py, .js, .ts, etc.)
//  6. Return remaining files as violations
//
// Returns empty list if:
//   - No git repo found (not an error, just skip validation)
//   - All changes are in project-dir or /tmp/
//   - All changes are non-code files (.md, .yaml, .sh)
func (g *GitIntegrator) GetModifiedSourceFiles(projectDir string) ([]string, error) {
	// Check if git repo exists
	if !g.IsGitRepo() {
		return []string{}, nil // Not an error - return empty list to skip validation
	}

	// Execute git status --porcelain
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git status failed: %w (output: %s)", err, string(output))
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	var violations []string

	for _, line := range lines {
		if len(line) < 3 {
			continue // Skip empty lines
		}

		statusCode := line[0:2]
		filePath := strings.TrimSpace(line[3:])

		// Skip unmodified and ignored files
		// Note: Untracked files (??) are included as potential violations
		if statusCode == "  " || statusCode == "!!" {
			continue
		}

		// Skip files in project-dir (research code allowed)
		if isInProjectDir(filePath, projectDir) {
			continue
		}

		// Skip files in /tmp/ (temporary files allowed)
		if strings.HasPrefix(filePath, "/tmp/") {
			continue
		}

		// Check if file is source code
		if isSourceCodeFile(filePath) {
			violations = append(violations, filePath)
		}
	}

	return violations, nil
}
