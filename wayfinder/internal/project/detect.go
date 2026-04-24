// Package project provides shared utilities for Wayfinder project detection and validation.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectDir finds the wayfinder project directory.
// If projectPath is provided, it validates and returns it.
// Otherwise, it looks for WAYFINDER-STATUS.md in the current directory.
func DetectDir(projectPath string) (string, error) {
	if projectPath != "" {
		absPath, err := filepath.Abs(projectPath)
		if err != nil {
			return "", fmt.Errorf("invalid project path: %w", err)
		}
		return absPath, nil
	}

	// Detect from current directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	// Check if WAYFINDER-STATUS.md exists
	statusFile := filepath.Join(cwd, "WAYFINDER-STATUS.md")
	if _, err := os.Stat(statusFile); os.IsNotExist(err) {
		return "", fmt.Errorf(`no active Wayfinder project found

WAYFINDER-STATUS.md not found in current directory: %s

Usage:
  wayfinder <command>                  # Run in project directory
  wayfinder -C <path> <command>        # Run for specific project`, cwd)
	}

	return cwd, nil
}

// ValidatePath validates that a project path matches expected Wayfinder structure.
func ValidatePath(path, workspace string) bool {
	home := os.Getenv("HOME")
	expectedPrefix := filepath.Join(home, "src", "ws", workspace, "wf")
	return strings.HasPrefix(path, expectedPrefix)
}

// GenerateID creates a filesystem-safe project ID from a prompt.
func GenerateID(prompt string) string {
	// Convert to lowercase
	id := strings.ToLower(prompt)

	// Replace spaces and special characters with hyphens
	re := regexp.MustCompile(`[^a-z0-9]+`)
	id = re.ReplaceAllString(id, "-")

	// Remove leading/trailing hyphens
	id = strings.Trim(id, "-")

	// Truncate to 50 characters max
	if len(id) > 50 {
		id = id[:50]
		id = strings.TrimRight(id, "-")
	}

	return id
}

// DetermineWorkspace determines which workspace to use for project creation.
// Priority: $WORKSPACE env var → AGM session query → auto-detect from cwd → default "oss"
func DetermineWorkspace() (string, error) {
	// Priority 1: $WORKSPACE environment variable (AGM integration)
	workspace := os.Getenv("WORKSPACE")
	if workspace != "" {
		workspace = strings.ToLower(strings.TrimSpace(workspace))
		if isValidWorkspace(workspace) {
			return workspace, nil
		}
		return "", fmt.Errorf("$WORKSPACE must be a valid workspace name (got: %s)", workspace)
	}

	// Priority 2: Query AGM for current workspace
	agmWorkspace := queryAGMWorkspace()
	if agmWorkspace != "" {
		if isValidWorkspace(agmWorkspace) {
			return agmWorkspace, nil
		}
	}

	// Priority 3: Auto-detect from current directory
	cwdWorkspace := detectWorkspaceFromCwd()
	if cwdWorkspace != "" {
		return cwdWorkspace, nil
	}

	// Priority 4: Default to "oss"
	return "oss", nil
}

// isValidWorkspace checks if a workspace name is valid
func isValidWorkspace(name string) bool {
	if name == "" {
		return false
	}
	// Allow alphanumeric, hyphens, underscores
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// detectWorkspaceFromCwd detects workspace from current working directory
func detectWorkspaceFromCwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	realCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		realCwd = cwd
	}

	home := os.Getenv("HOME")

	// Look for /ws/{workspace}/ pattern
	prefix := filepath.Join(home, "src", "ws")
	if strings.HasPrefix(realCwd, prefix) {
		// Extract workspace name from path
		relPath := strings.TrimPrefix(realCwd, prefix+string(filepath.Separator))
		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) > 0 && isValidWorkspace(parts[0]) {
			return parts[0]
		}
	}

	return ""
}

// queryAGMWorkspace attempts to read workspace from AGM current session manifest
// Returns empty string if AGM session not found or workspace not set
func queryAGMWorkspace() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Try reading from AGM current session link
	currentSessionLink := filepath.Join(home, ".agm", "current-session")
	manifestPath, err := os.Readlink(currentSessionLink)
	if err != nil {
		// Symlink doesn't exist, try alternative paths
		currentSessionLink = filepath.Join(home, ".claude", "current-session")
		manifestPath, err = os.Readlink(currentSessionLink)
		if err != nil {
			return ""
		}
	}

	// Read manifest file
	manifestFile := filepath.Join(manifestPath, "manifest.yaml")
	data, err := os.ReadFile(manifestFile)
	if err != nil {
		// Try JSON format as fallback
		manifestFile = filepath.Join(manifestPath, "manifest.json")
		data, err = os.ReadFile(manifestFile)
		if err != nil {
			return ""
		}
	}

	// Parse manifest to extract workspace field
	var manifest struct {
		Workspace string `json:"workspace" yaml:"workspace"`
	}

	// Try JSON parsing (most AGM manifests are JSON)
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ""
	}

	return manifest.Workspace
}

// ExtractSessionID extracts the session ID from wayfinder-session output.
func ExtractSessionID(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Session ID:") {
			parts := strings.SplitN(line, "Session ID:", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}
