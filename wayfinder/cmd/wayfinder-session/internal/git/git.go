// Package git provides git-related functionality.
package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/archive"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/retrospective"
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

// CheckGitRepo returns (true, nil) if projectDir is inside a git repository,
// (false, nil) if confirmed NOT a git repository (exit 128),
// or (false, error) if git probe failed due to an error (e.g., missing git, unsafe repo, permissions).
func (g *GitIntegrator) CheckGitRepo() (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
		outStr := strings.ToLower(string(output))
		if strings.Contains(outStr, "not a git repository") || strings.Contains(outStr, "must be run in a work tree") {
			return false, nil
		}
	}
	return false, fmt.Errorf("git repository check failed: %w (output: %s)", err, strings.TrimSpace(string(output)))
}

// IsGitRepo checks if the project directory is within a git repository
// Works correctly even when project is a subdirectory of a git repo
func (g *GitIntegrator) IsGitRepo() bool {
	isRepo, _ := g.CheckGitRepo()
	return isRepo
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
// It reports whether a commit was created; a repository that ignores every
// candidate artifact yields (false, nil) so the caller does not announce a
// commit that does not exist.
func (g *GitIntegrator) CommitPhaseCompletion(phase, outcome, context string) (bool, error) {
	if !g.IsGitRepo() {
		return false, fmt.Errorf("project directory is not a git repository")
	}

	files := []string{
		"WAYFINDER-STATUS.md",
		history.HistoryFilename,
		history.LegacyHistoryFilename,
	}
	artifacts, err := filepath.Glob(filepath.Join(g.projectDir, phase+"-*.md"))
	if err != nil {
		return false, fmt.Errorf("find %s phase artifacts: %w", phase, err)
	}
	for _, artifact := range artifacts {
		files = append(files, filepath.Base(artifact))
	}
	if phase == "DESIGN" {
		files = append(files, "ARCHITECTURE.md")
		adrs, err := filepath.Glob(filepath.Join(g.projectDir, "ADR-*.md"))
		if err != nil {
			return false, fmt.Errorf("find DESIGN ADR artifacts: %w", err)
		}
		for _, adr := range adrs {
			files = append(files, filepath.Base(adr))
		}
	}

	committed, err := g.commitScoped(g.formatCommitMessage(phase, outcome, context), files)
	if err != nil {
		return false, fmt.Errorf("failed to create commit: %w", err)
	}
	return committed, nil
}

// CommitRewind commits the status and retrospective markers produced by a
// rewind so the documented next start-phase command sees a clean project.
// It reports whether a commit was created.
func (g *GitIntegrator) CommitRewind(fromPhase, toPhase string, archiveRef archive.ArchiveRef) (bool, error) {
	if !g.IsGitRepo() {
		return false, fmt.Errorf("project directory is not a git repository")
	}
	archivePath := archiveRef.RelativePath()
	cleanArchivePath := filepath.ToSlash(filepath.Clean(archivePath))
	if archivePath == "" || filepath.IsAbs(archivePath) || archivePath != cleanArchivePath || !strings.HasPrefix(cleanArchivePath, ".wayfinder/archives/") {
		return false, fmt.Errorf("invalid rewind archive reference %q", archivePath)
	}
	message := fmt.Sprintf("wayfinder: rewind %s to %s\n\nWayfinder-Event: rewind", fromPhase, toPhase)
	return g.commitScoped(message, []string{
		"WAYFINDER-STATUS.md",
		history.HistoryFilename,
		history.LegacyHistoryFilename,
		retrospective.RetroFilename,
		archivePath,
	})
}

// CommitPhaseStart creates a git commit for phase start.
// Adds WAYFINDER-STATUS.md and WAYFINDER-HISTORY.jsonl to staging and commits so
// the worktree is clean before any deliverable work begins. Without this, the
// next start-phase call finds uncommitted marker files and refuses (ce-fvkz).
// It reports whether a commit was created.
func (g *GitIntegrator) CommitPhaseStart(phase string) (bool, error) {
	if !g.IsGitRepo() {
		return false, fmt.Errorf("project directory is not a git repository")
	}

	message := fmt.Sprintf("wayfinder: start %s\n\nWayfinder-Phase: %s\nWayfinder-Event: started", phase, phase)
	return g.commitScoped(message, []string{
		"WAYFINDER-STATUS.md",
		history.HistoryFilename,
		history.LegacyHistoryFilename,
	})
}

// CommitSessionInit commits WAYFINDER-STATUS.md after a new session is created
// so that the immediately following `start-phase CHARTER` sees a clean worktree
// and does not refuse with "uncommitted files detected" (ce-11fi bootstrap fix).
//
// Unlike CommitPhaseStart, this method is lenient: it reports (false, nil) when
// the directory is not inside a git repository (non-git workflows are valid).
// It reports whether a commit was created.
func (g *GitIntegrator) CommitSessionInit(projectName string) (bool, error) {
	if !g.IsGitRepo() {
		return false, nil // not an error — non-git workflows are allowed
	}

	statusFile := filepath.Join(g.projectDir, "WAYFINDER-STATUS.md")
	if _, err := os.Stat(statusFile); os.IsNotExist(err) {
		return false, nil // nothing to commit
	}

	message := fmt.Sprintf("wayfinder: init session %s\n\nWayfinder-Event: session-started", projectName)
	return g.commitScoped(message, []string{"WAYFINDER-STATUS.md"})
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

// stageArtifact stages one scoped lifecycle artifact and reports whether it was
// staged. The repository's ignore policy is authoritative: a path the
// repository ignores and does not already track is deliberately temporal, so it
// is skipped rather than force-added.
//
// Force-adding ignored markers made the mandated PR path block itself
// (ce-2sgej): `wayfinder session start` committed WAYFINDER-STATUS.md and
// WAYFINDER-HISTORY.jsonl, routing-guard's temporal-debt gate then rejected
// them as tracked temporal artifacts, that gate runs inside `preflight-full`,
// and `safe-pr` refuses to open a PR when preflight fails. Skipping costs
// nothing: Git does not report ignored files, so the worktree the next
// start-phase inspects is still clean.
//
// A marker the repository already tracks keeps receiving its updates. Ignore
// rules do not apply to tracked paths, so a plain `git add` is correct there
// and staging it prevents silent drift.
func (g *GitIntegrator) stageArtifact(file string) (bool, error) {
	tracked, err := g.isTracked(file)
	if err != nil {
		return false, err
	}
	if !tracked {
		ignored, err := g.isIgnored(file)
		if err != nil {
			return false, err
		}
		if ignored {
			return false, nil
		}
	}

	cmd := exec.Command("git", "add", "--", file)
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git add failed: %w (output: %s)", err, string(output))
	}
	return true, nil
}

// isIgnored reports whether the repository's ignore rules exclude the path.
// `git check-ignore -q` exits 0 when the path is ignored and 1 when it is not;
// any other exit status is a real failure and must not read as "not ignored".
func (g *GitIntegrator) isIgnored(file string) (bool, error) {
	cmd := exec.Command("git", "check-ignore", "-q", "--", file)
	cmd.Dir = g.projectDir
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("check ignore status of %s: %w", file, err)
}

// isTracked reports whether the path is in the index.
func (g *GitIntegrator) isTracked(file string) (bool, error) {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", "--", file)
	cmd.Dir = g.projectDir
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// `git ls-files --error-unmatch` exits 1 for a path that is not in the
	// index, which is an answer rather than a failure. Any other exit code is
	// a real error and must not be reported as "untracked". Binding the typed
	// error instead of discarding it also keeps errcheck and modernize from
	// contradicting each other on this line.
	if exitErr, isExit := errors.AsType[*exec.ExitError](err); isExit && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("check tracked path %s: %w", file, err)
}

// commitScoped stages the candidate artifacts the repository accepts and
// commits exactly those. It reports whether a commit was actually created so
// callers do not announce lifecycle commits that never happened.
func (g *GitIntegrator) commitScoped(message string, candidates []string) (bool, error) {
	seen := make(map[string]bool, len(candidates))
	staged := make([]string, 0, len(candidates))
	for _, file := range candidates {
		if seen[file] {
			continue
		}
		seen[file] = true
		present, err := g.pathExistsOrTracked(file)
		if err != nil {
			return false, err
		}
		if !present {
			continue
		}
		added, err := g.stageArtifact(file)
		if err != nil {
			return false, fmt.Errorf("failed to add %s: %w", file, err)
		}
		if !added {
			continue
		}
		staged = append(staged, file)
	}
	if len(staged) == 0 {
		return false, nil
	}
	args := append([]string{"commit", "-m", message, "--"}, staged...)
	cmd := exec.Command("git", args...)
	cmd.Dir = g.projectDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			return false, nil
		}
		return false, fmt.Errorf("git commit failed: %w (output: %s)", err, string(output))
	}
	return true, nil
}

func (g *GitIntegrator) pathExistsOrTracked(file string) (bool, error) {
	if _, err := os.Stat(filepath.Join(g.projectDir, file)); err == nil {
		return true, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat %s: %w", file, err)
	}
	return g.isTracked(file)
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
