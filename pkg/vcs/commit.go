package vcs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// companionPath returns the companion file path for an engram file.
// .ai.md -> .why.md and vice versa
func companionPath(path string) string {
	if strings.HasSuffix(path, ".ai.md") {
		return strings.TrimSuffix(path, ".ai.md") + ".why.md"
	}
	if strings.HasSuffix(path, ".why.md") {
		return strings.TrimSuffix(path, ".why.md") + ".ai.md"
	}
	return ""
}

// isTrackedFileType returns true if the file is an engram or companion file
func isTrackedFileType(path string) bool {
	return strings.HasSuffix(path, ".ai.md") || strings.HasSuffix(path, ".why.md")
}

// TrackChange stages and commits a file change, including its companion.
// The path should be relative to the repo directory.
// Returns the commit hash or empty string if nothing to commit.
func (r *Repo) TrackChange(path, message string) (string, error) {
	absPath := path
	if !filepath.IsAbs(path) {
		absPath = filepath.Join(r.dir, path)
	}

	// Verify file exists
	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("file not found: %w", err)
	}

	// Make path relative to repo
	relPath, err := filepath.Rel(r.dir, absPath)
	if err != nil {
		return "", fmt.Errorf("relative path: %w", err)
	}

	// Collect files to stage
	filesToStage := []string{relPath}

	// Add companion file if it exists
	comp := companionPath(absPath)
	if comp != "" {
		if _, err := os.Stat(comp); err == nil {
			compRel, err := filepath.Rel(r.dir, comp)
			if err == nil {
				filesToStage = append(filesToStage, compRel)
			}
		}
	}

	// Stage files
	if err := r.StageFiles(filesToStage...); err != nil {
		return "", fmt.Errorf("stage: %w", err)
	}

	// Format message if not provided
	if message == "" {
		message = fmt.Sprintf("memory: update %s", filepath.Base(relPath))
	}

	// Commit
	return r.Commit(message)
}

// TrackDelete stages and commits a file deletion.
// Returns the commit hash or empty string if nothing to commit.
func (r *Repo) TrackDelete(path, message string) (string, error) {
	relPath := path
	if filepath.IsAbs(path) {
		var err error
		relPath, err = filepath.Rel(r.dir, path)
		if err != nil {
			return "", fmt.Errorf("relative path: %w", err)
		}
	}

	// Stage the deletion
	if err := r.run("git", "add", "--", relPath); err != nil {
		return "", fmt.Errorf("stage deletion: %w", err)
	}

	// Also stage companion deletion if it was removed
	comp := companionPath(filepath.Join(r.dir, relPath))
	if comp != "" {
		compRel, err := filepath.Rel(r.dir, comp)
		if err == nil {
			// Try to stage companion deletion (ignore errors if it still exists)
			_ = r.run("git", "add", "--", compRel)
		}
	}

	if message == "" {
		message = fmt.Sprintf("memory: delete %s", filepath.Base(relPath))
	}

	return r.Commit(message)
}
