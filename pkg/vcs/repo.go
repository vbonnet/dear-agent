package vcs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo manages a git repository for memory storage
type Repo struct {
	dir string
}

// NewRepo creates a Repo for the given directory
func NewRepo(dir string) *Repo {
	return &Repo{dir: dir}
}

// Dir returns the repository directory
func (r *Repo) Dir() string {
	return r.dir
}

// IsRepo checks if the directory is a git repository
func (r *Repo) IsRepo() bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = r.dir
	return cmd.Run() == nil
}

// EnsureRepo initializes a git repo if it doesn't already exist.
// Creates the directory, runs git init, sets up .gitignore, and
// optionally configures a remote.
func EnsureRepo(dir, remoteName, remoteURL, branch string) (*Repo, error) {
	// Create directory if needed
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	r := &Repo{dir: dir}

	if r.IsRepo() {
		return r, nil
	}

	// Initialize git repo
	if err := r.run("git", "init"); err != nil {
		return nil, fmt.Errorf("git init: %w", err)
	}

	// Set default branch
	if branch == "" {
		branch = "main"
	}
	// Rename default branch to configured name
	if err := r.run("git", "branch", "-M", branch); err != nil {
		// Non-fatal: branch rename may fail if no commits yet
		_ = err
	}

	// Create .gitignore that defaults to tracking .ai.md and .why.md
	gitignore := "# Track .ai.md and .why.md by default (opt-out)\n# Everything else is ignored unless explicitly added\n*\n!.gitignore\n!**/*.ai.md\n!**/*.why.md\n!**/\n"
	gitignorePath := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(gitignore), 0o600); err != nil {
		return nil, fmt.Errorf("write .gitignore: %w", err)
	}

	// Stage and create initial commit
	if err := r.run("git", "add", ".gitignore"); err != nil {
		return nil, fmt.Errorf("stage .gitignore: %w", err)
	}
	if err := r.run("git", "commit", "-m", "vcs: initialize memory repository"); err != nil {
		return nil, fmt.Errorf("initial commit: %w", err)
	}

	// Configure remote if provided
	if remoteURL != "" {
		if remoteName == "" {
			remoteName = "origin"
		}
		if err := r.run("git", "remote", "add", remoteName, remoteURL); err != nil {
			return nil, fmt.Errorf("add remote: %w", err)
		}
	}

	return r, nil
}

// StageFiles adds files to the git staging area
func (r *Repo) StageFiles(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"add", "--"}, paths...)
	return r.run("git", args...)
}

// StageDeleted stages deleted files
func (r *Repo) StageDeleted(paths ...string) error {
	if len(paths) == 0 {
		return nil
	}
	args := append([]string{"rm", "--cached", "--"}, paths...)
	return r.run("git", args...)
}

// Commit creates a git commit with the given message.
// Returns the commit hash or empty string if nothing to commit.
func (r *Repo) Commit(message string) (string, error) {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = r.dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			return "", nil
		}
		return "", fmt.Errorf("git commit: %w (output: %s)", err, string(output))
	}

	return r.HeadHash()
}

// HeadHash returns the current HEAD commit hash
func (r *Repo) HeadHash() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = r.dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// Log returns commit history for a file (or all files if path is empty)
func (r *Repo) Log(path string, limit int) ([]CommitEntry, error) {
	args := []string{"log", "--format=%H|%aI|%s"}
	if limit > 0 {
		args = append(args, fmt.Sprintf("-%d", limit))
	}
	if path != "" {
		args = append(args, "--", path)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var entries []CommitEntry
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) == 3 {
			entries = append(entries, CommitEntry{
				Hash:    parts[0],
				Date:    parts[1],
				Message: parts[2],
			})
		}
	}
	return entries, nil
}

// Diff returns the diff between two refs for a file
func (r *Repo) Diff(path, fromRef, toRef string) (string, error) {
	args := []string{"diff", fromRef}
	if toRef != "" {
		args = append(args, toRef)
	}
	if path != "" {
		args = append(args, "--", path)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff: %w", err)
	}
	return string(output), nil
}

// Restore reverts a file to a specific commit
func (r *Repo) Restore(path, commitHash string) error {
	if err := r.run("git", "checkout", commitHash, "--", path); err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}
	return nil
}

// Status returns the git status output
func (r *Repo) Status() (string, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = r.dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git status: %w", err)
	}
	return string(output), nil
}

// Push pushes to the configured remote
func (r *Repo) Push(remoteName, branch string) error {
	if remoteName == "" {
		remoteName = "origin"
	}
	if branch == "" {
		branch = "main"
	}
	return r.run("git", "push", remoteName, branch)
}

// run executes a command in the repo directory
func (r *Repo) run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = r.dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w (output: %s)", name, strings.Join(args, " "), err, string(output))
	}
	return nil
}

// CommitEntry represents a single commit in the log
type CommitEntry struct {
	Hash    string
	Date    string
	Message string
}
