package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// worktree is one record from `git worktree list --porcelain`.
type worktree struct {
	path     string
	branch   string
	head     string
	bare     bool
	detached bool
	// locked records an operator's explicit `git worktree lock`. The house
	// convention is to lock a checkout an agent is actively using, so a
	// locked worktree is never a removal candidate.
	locked bool
	// primary marks the repository's main checkout. Git always lists it
	// first, and it can never be removed.
	primary bool
}

// listWorktrees enumerates every worktree registered in repo.
//
// Records are requested NUL-delimited so a path containing a newline cannot
// be truncated into a prefix that happens to name a different registered
// worktree. Older Git releases predate `-z` on this subcommand, so a failure
// falls back to the newline form.
func listWorktrees(ctx context.Context, repo string) ([]worktree, error) {
	if out, err := git(ctx, repo, "worktree", "list", "--porcelain", "-z"); err == nil {
		return parseWorktrees(out, "\x00"), nil
	}
	out, err := git(ctx, repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(out, "\n"), nil
}

// parseWorktrees splits porcelain output into records. An empty field
// terminates the current record in both the NUL and newline encodings.
func parseWorktrees(out, sep string) []worktree {
	var result []worktree
	var cur worktree
	flush := func() {
		if cur.path != "" {
			cur.primary = len(result) == 0
			result = append(result, cur)
		}
		cur = worktree{}
	}
	for field := range strings.SplitSeq(out, sep) {
		if field == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(field, " ")
		switch key {
		case "worktree":
			flush()
			cur.path = val
		case "HEAD":
			cur.head = val
		case "branch":
			cur.branch = val
		case "bare":
			cur.bare = true
		case "detached":
			cur.detached = true
		case "locked":
			cur.locked = true
		}
	}
	flush()
	return result
}

// targetRef resolves the ref that merged work is measured against.
func targetRef(ctx context.Context, repo string) (string, error) {
	if gitOK(ctx, repo, "rev-parse", "--verify", "--quiet", "origin/HEAD") == nil {
		if out, err := git(ctx, repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil && strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out), nil
		}
	}
	if gitOK(ctx, repo, "rev-parse", "--verify", "--quiet", "origin/main") != nil {
		return "", errNoTargetRef
	}
	return "origin/main", nil
}

// shortBranch strips the refs/heads/ prefix Git's porcelain output carries.
// The dry-run preview and the delete call must agree on this form, or the
// audit output misstates the destructive command being approved.
func shortBranch(ref string) string { return strings.TrimPrefix(ref, "refs/heads/") }

// tmuxSockets are the servers an AGM or hand-run agent session can live on:
// the AGM socket and the user's default tmux server.
func tmuxSockets() [][]string {
	home, err := os.UserHomeDir()
	sockets := [][]string{{}}
	if err == nil {
		sockets = append([][]string{{"-S", filepath.Join(home, ".agm", "agm.sock")}}, sockets...)
	}
	return sockets
}

// activeSessions returns the set of live tmux session names across the AGM
// and default sockets, and whether that set could be positively established.
//
// The second return value is the fail-closed switch. A worktree owned by a
// live session has been reaped from under a running agent before; when the
// probe itself cannot answer, fix mode must decline to remove anything
// rather than assume the machine is idle. A missing tmux binary or a server
// that is simply not running are both "no sessions", not probe failures.
func activeSessions(ctx context.Context) (map[string]bool, bool) {
	names := map[string]bool{}
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return names, true
	}
	for _, socket := range tmuxSockets() {
		args := append(append([]string{}, socket...), "list-sessions", "-F", "#{session_name}")
		cmd := exec.CommandContext(ctx, tmuxPath, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			if noTmuxServer(string(out)) {
				continue
			}
			return names, false
		}
		for line := range strings.SplitSeq(string(out), "\n") {
			if name := strings.TrimSpace(line); name != "" {
				names[name] = true
			}
		}
	}
	return names, true
}

// noTmuxServer distinguishes "there is no server, so there are no sessions"
// from a genuine probe failure.
func noTmuxServer(message string) bool {
	return strings.Contains(message, "no server running") ||
		strings.Contains(message, "no current session") ||
		strings.Contains(message, "error connecting")
}
