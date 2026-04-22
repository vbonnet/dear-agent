package common

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// GetCurrentCommit returns the current git commit SHA.
// Returns an error if not in a git repository or git command fails.
func GetCurrentCommit() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get git commit: %w (output: %s)", err, out.String())
	}

	commit := strings.TrimSpace(out.String())
	if commit == "" {
		return "", fmt.Errorf("git commit SHA is empty")
	}

	return commit, nil
}

// GetCurrentBranch returns the current git branch name.
// Returns an error if not in a git repository or git command fails.
func GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get git branch: %w (output: %s)", err, out.String())
	}

	branch := strings.TrimSpace(out.String())
	if branch == "" {
		return "", fmt.Errorf("git branch name is empty")
	}

	return branch, nil
}
