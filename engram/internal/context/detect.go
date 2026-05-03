// Package context provides context-related functionality.
package context

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectContext auto-detects a query from the current environment
// Priority: git repo name > directory name > generic fallback
func DetectContext() (string, error) {
	// Try git repo name first
	if repoName, err := getGitRepoName(); err == nil && repoName != "" {
		return cleanName(repoName) + " patterns", nil
	}

	// Try directory name
	cwd, err := os.Getwd()
	if err == nil {
		dirName := filepath.Base(cwd)
		if dirName != "" && dirName != "/" && dirName != "." {
			return cleanName(dirName) + " patterns", nil
		}
	}

	// Final fallback
	return "coding best practices", nil
}

// getGitRepoName extracts repository name from git remote URL
func getGitRepoName() (string, error) {
	cmd := exec.Command("git", "config", "--get", "remote.origin.url")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	url := strings.TrimSpace(string(output))
	if url == "" {
		return "", nil
	}

	// Parse git@github.com:user/repo.git → repo
	// Or https://github.com/user/repo → repo
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		repo := parts[len(parts)-1]
		repo = strings.TrimSuffix(repo, ".git")
		return repo, nil
	}

	return "", nil
}

// cleanName sanitizes a name for use as a query
// Replaces separators with spaces, removes non-alphanumeric, lowercases
func cleanName(name string) string {
	// Replace separators with spaces
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// Remove non-alphanumeric except spaces
	re := regexp.MustCompile(`[^a-zA-Z0-9 ]+`)
	name = re.ReplaceAllString(name, "")

	// Collapse multiple spaces
	name = strings.Join(strings.Fields(name), " ")

	return strings.ToLower(name)
}
