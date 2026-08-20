package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// gitTimeout bounds every Git subprocess. A cleanup tool that hangs on a
// prompt or a wedged remote is worse than one that fails: the operator
// cannot tell a stalled probe from a slow one, and a stalled probe must
// never be mistaken for a "safe to delete" verdict.
const gitTimeout = 120 * time.Second

// ambientGitVars are the environment variables Git gives precedence over the
// `-C <repo>` argument. A cleanup run launched from a Git hook or a shell
// that exports them would otherwise enumerate and mutate a repository other
// than the one named on the command line. The post-merge wrapper already
// scrubs these; a tool that force-removes worktrees must do the same.
var ambientGitVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_COMMON_DIR",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_NAMESPACE",
	"GIT_CEILING_DIRECTORIES",
	"GIT_PREFIX",
}

// gitEnv returns the process environment with every ambient repository
// selector removed and interactive prompting disabled.
func gitEnv() []string {
	drop := make(map[string]bool, len(ambientGitVars))
	for _, name := range ambientGitVars {
		drop[name] = true
	}
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if drop[name] {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GIT_TERMINAL_PROMPT=0")
}

// git runs a Git command in repo and returns its stdout.
func git(ctx context.Context, repo string, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	all := append([]string{"-C", repo}, args...)
	cmd := exec.CommandContext(cctx, "git", all...)
	cmd.Env = gitEnv()
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg != "" {
			return out.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, msg)
		}
		return out.String(), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out.String(), nil
}

// gitOK reports whether a Git command succeeded, discarding its output.
func gitOK(ctx context.Context, repo string, args ...string) error {
	_, err := git(ctx, repo, args...)
	return err
}

// gitInt runs a Git command that prints a single integer.
//
// It returns an error rather than a zero value on failure. A swallowed
// failure here is the tool's worst possible bug: a transient `rev-list`
// error would read as "zero commits ahead" and authorize deleting unmerged
// work, and a failed `log` would read as a 1970 timestamp. Every caller must
// treat an error as UNKNOWN and keep the worktree.
func gitInt(ctx context.Context, repo string, args ...string) (int, error) {
	out, err := git(ctx, repo, args...)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("git %s: parsing integer output %q: %w", strings.Join(args, " "), strings.TrimSpace(out), err)
	}
	return n, nil
}

// runGitPassthrough runs a mutating Git command with its output attached to
// this process, so the operator sees exactly what Git did.
func runGitPassthrough(ctx context.Context, repo string, args ...string) error {
	cctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	all := append([]string{"-C", repo}, args...)
	cmd := exec.CommandContext(cctx, "git", all...)
	cmd.Env = gitEnv()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// topLevel resolves any path inside a checkout to that checkout's root.
func topLevel(ctx context.Context, path string) (string, error) {
	out, err := git(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", fmt.Errorf("git rev-parse --show-toplevel: empty output for %s", path)
	}
	return canonical(root), nil
}

// canonical returns an absolute, symlink-resolved, cleaned path. Comparisons
// between a caller-supplied path and Git's porcelain output are only sound
// once both sides have been through it: porcelain always reports absolute
// paths, while a caller may pass `.` or a subdirectory.
func canonical(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

// samePath reports whether two paths name the same location on disk.
func samePath(a, b string) bool { return canonical(a) == canonical(b) }
