package stophook

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// GitStatus checks for uncommitted changes in the given directory.
// Returns the list of dirty files (empty if clean).
func GitStatus(dir string) ([]string, error) {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain", ".")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	return parseLines(out), nil
}

// GitUnpushedCommits returns unpushed commit summaries.
func GitUnpushedCommits(dir string) ([]string, error) {
	cmd := exec.Command("git", "-C", dir, "log", "@{u}..HEAD", "--oneline")
	out, err := cmd.Output()
	if err != nil {
		// No upstream configured — not an error for this check
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			return nil, nil
		}
		return nil, fmt.Errorf("git log unpushed: %w", err)
	}
	return parseLines(out), nil
}

// GitExtraBranches returns branches other than main/master and the current branch.
func GitExtraBranches(dir string) ([]string, error) {
	cmd := exec.Command("git", "-C", dir, "branch", "--format=%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}

	currentCmd := exec.Command("git", "-C", dir, "branch", "--show-current")
	currentOut, _ := currentCmd.Output()
	current := strings.TrimSpace(string(currentOut))

	skip := map[string]bool{"main": true, "master": true}
	if current != "" {
		skip[current] = true
	}

	var extra []string
	for _, b := range parseLines(out) {
		if !skip[b] {
			extra = append(extra, b)
		}
	}
	return extra, nil
}

// WorktreeInfo holds parsed worktree data.
type WorktreeInfo struct {
	Path   string
	Branch string
	Bare   bool
}

// GitWorktrees returns all worktrees for the repo at dir.
func GitWorktrees(dir string) ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "-C", dir, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var worktrees []WorktreeInfo
	var current WorktreeInfo

	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			if current.Path != "" {
				worktrees = append(worktrees, current)
			}
			current = WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch "):
			current.Branch = strings.TrimPrefix(line, "branch ")
		case line == "bare":
			current.Bare = true
		}
	}
	if current.Path != "" {
		worktrees = append(worktrees, current)
	}

	return worktrees, nil
}

// IsGitRepo checks if the directory is inside a git repository.
func IsGitRepo(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	return cmd.Run() == nil
}

func parseLines(data []byte) []string {
	s := strings.TrimSpace(string(data))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
