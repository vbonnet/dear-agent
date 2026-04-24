// Package git provides utilities for extracting git repository state.
package git

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// State represents git repository state
type State struct {
	Branch        string
	WorkingDir    string
	ModifiedFiles []string
	CreatedFiles  []string
	DeletedFiles  []string
}

// ExtractState extracts git state from current directory
// Returns default State if not a git repository or git unavailable
func ExtractState() (State, error) {
	defaultState := State{
		Branch:        "unknown",
		WorkingDir:    ".",
		ModifiedFiles: []string{},
		CreatedFiles:  []string{},
		DeletedFiles:  []string{},
	}

	// Check if git is available
	if !isGitAvailable() {
		return defaultState, fmt.Errorf("git not available")
	}

	// Check if current directory is a git repository
	if !isGitRepo() {
		return defaultState, fmt.Errorf("not a git repository")
	}

	state := State{
		ModifiedFiles: []string{},
		CreatedFiles:  []string{},
		DeletedFiles:  []string{},
	}

	// Get branch name
	branch, err := getBranchName()
	if err != nil {
		state.Branch = "unknown"
	} else {
		state.Branch = branch
	}

	// Get working directory relative to repo root
	workingDir, err := getWorkingDir()
	if err != nil {
		state.WorkingDir = "."
	} else {
		state.WorkingDir = workingDir
	}

	// Get file changes via git status --porcelain
	modified, created, deleted := parseGitStatus()
	state.ModifiedFiles = modified
	state.CreatedFiles = created
	state.DeletedFiles = deleted

	return state, nil
}

// isGitAvailable checks if git command is in PATH
func isGitAvailable() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// isGitRepo checks if current directory is inside a git repository
func isGitRepo() bool {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--git-dir")
	err := cmd.Run()
	return err == nil
}

// getBranchName returns current git branch name
func getBranchName() (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getWorkingDir returns working directory relative to git repo root
func getWorkingDir() (string, error) {
	// Get repo root
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
	rootOutput, err := cmd.Output()
	if err != nil {
		return ".", err
	}
	repoRoot := strings.TrimSpace(string(rootOutput))

	// Get current directory
	cmd = exec.CommandContext(context.Background(), "pwd")
	pwdOutput, err := cmd.Output()
	if err != nil {
		return ".", err
	}
	currentDir := strings.TrimSpace(string(pwdOutput))

	// Calculate relative path
	relPath, err := filepath.Rel(repoRoot, currentDir)
	if err != nil {
		return ".", err
	}

	if relPath == "." || relPath == "" {
		return ".", nil
	}

	return relPath, nil
}

// parseGitStatus parses git status --porcelain output
// Returns three slices: modified, created, deleted files
func parseGitStatus() (modified, created, deleted []string) {
	cmd := exec.CommandContext(context.Background(), "git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, nil
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if len(line) < 4 {
			continue
		}

		status := line[0:2]
		filename := line[3:]

		// Handle renamed files (format: "R  old -> new")
		if status[0] == 'R' || status[1] == 'R' {
			parts := strings.Split(filename, " -> ")
			if len(parts) == 2 {
				deleted = append(deleted, parts[0])
				created = append(created, parts[1])
			}
			continue
		}

		modified, created, deleted = classifyIndexStatus(status[0], filename, modified, created, deleted)
		modified, created, deleted = classifyWorkTreeStatus(status[1], filename, modified, created, deleted)
	}

	return modified, created, deleted
}

// classifyIndexStatus categorizes a file based on the index (staged) status character.
func classifyIndexStatus(status byte, filename string, modified, created, deleted []string) ([]string, []string, []string) {
	switch status {
	case 'A':
		created = append(created, filename)
	case 'D':
		deleted = append(deleted, filename)
	case 'M':
		modified = append(modified, filename)
	}
	return modified, created, deleted
}

// classifyWorkTreeStatus categorizes a file based on the working tree status character.
func classifyWorkTreeStatus(status byte, filename string, modified, created, deleted []string) ([]string, []string, []string) {
	switch status {
	case 'M':
		if !contains(modified, filename) {
			modified = append(modified, filename)
		}
	case 'D':
		if !contains(deleted, filename) {
			deleted = append(deleted, filename)
		}
	case '?':
		if !contains(created, filename) {
			created = append(created, filename)
		}
	}
	return modified, created, deleted
}

// contains checks if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetBranchAbbrev returns last 5 characters of branch name
// If branch <= 5 chars, returns full name
func GetBranchAbbrev(branch string) string {
	if len(branch) <= 5 {
		return branch
	}
	return branch[len(branch)-5:]
}

// FormatFileList truncates file list to maxFiles and adds footer
// Returns formatted string with bullet points
func FormatFileList(files []string, maxFiles int) string {
	if len(files) == 0 {
		return "(none)"
	}

	var result strings.Builder
	count := len(files)
	displayed := count
	if count > maxFiles {
		displayed = maxFiles
	}

	for i := 0; i < displayed; i++ {
		result.WriteString(fmt.Sprintf("- %s\n", files[i]))
	}

	if count > maxFiles {
		result.WriteString(fmt.Sprintf("\n... and %d more file(s)", count-maxFiles))
	}

	return result.String()
}
