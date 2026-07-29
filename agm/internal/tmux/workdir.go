package tmux

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/shellquote"
)

// Workdir verification/repair tuning knobs. Package-level vars (not consts) so
// tests can shorten the deadlines without waiting out production timings.
var (
	// workDirVerifyDeadline bounds the whole verify+repair loop.
	workDirVerifyDeadline = 10 * time.Second
	// workDirRepairGrace is how long we give tmux to land the pane in the
	// requested directory on its own before sending a corrective `cd`.
	workDirRepairGrace = 1500 * time.Millisecond
	// workDirPollInterval is the poll cadence for pane_current_path.
	workDirPollInterval = 250 * time.Millisecond
)

// EnsureSessionWorkDir verifies that the (freshly created) session's active
// pane actually started in workDir, and repairs it with a `cd` when it did
// not. It returns an error when the pane cannot be steered into workDir
// before the deadline.
//
// Why this exists (ce-5zbg): tmux only honors `new-session -c <dir>` when the
// *server's own* working directory is still resolvable. spawn.c guards the
// pane chdir behind getcwd():
//
//	if (getcwd(path, sizeof path) != NULL) {
//	        if (chdir(new_wp->cwd) == 0) ...
//	}
//
// If the directory the tmux server was started from is later deleted (e.g. a
// reaped git worktree), getcwd() fails, the chdir block is skipped, and every
// new pane silently inherits the server's dead cwd — `-c` is ignored. A
// Bun-based CLI like Claude Code launched in a deleted cwd dies instantly
// ("ENOENT: Bun could not find a file"), so the ready prompt never appears
// and every spawn times out. The pane's shell, however, CAN chdir out of the
// dead directory, so sending `cd <workDir>` fully heals the pane without
// restarting the shared tmux server.
func EnsureSessionWorkDir(sessionName, workDir string) error {
	if workDir == "" {
		return nil
	}
	// Resolve relative workdirs against the client's cwd up front: the repair
	// `cd` below runs in the pane's shell, whose cwd is the (dead) server cwd,
	// so a relative path would resolve against the wrong base.
	if abs, err := filepath.Abs(workDir); err == nil {
		workDir = abs
	}
	deadline := time.Now().Add(workDirVerifyDeadline)
	graceEnd := time.Now().Add(workDirRepairGrace)
	repaired := false
	lastSeen := ""

	for time.Now().Before(deadline) {
		panePath, err := PaneCurrentPath(sessionName)
		if err == nil && panePath != "" {
			lastSeen = panePath
			if samePath(panePath, workDir) {
				if repaired {
					debug.Log("✓ Pane workdir repaired via cd: %s", workDir)
				}
				return nil
			}
		}

		// Give tmux a short grace period to settle before intervening: the
		// pane's shell may still be starting up.
		if !repaired && time.Now().After(graceEnd) {
			debug.Log("⚠️  Pane cwd %q does not match requested workdir %q — sending corrective cd (tmux server cwd likely deleted; ce-5zbg)", lastSeen, workDir)
			command, commandErr := correctiveWorkDirCommand(workDir)
			if commandErr != nil {
				return commandErr
			}
			if sendErr := SendCommand(sessionName, command); sendErr != nil {
				return fmt.Errorf("session %q started outside requested workdir %q (pane cwd %q) and corrective cd failed: %w",
					sessionName, workDir, lastSeen, sendErr)
			}
			repaired = true
		}

		time.Sleep(workDirPollInterval)
	}

	return fmt.Errorf("session %q pane is not in requested workdir %q (pane cwd: %q): "+
		"tmux ignored new-session -c, which happens when the tmux server's own working directory has been deleted; "+
		"restart the agm tmux server from a stable directory (ce-5zbg)",
		sessionName, workDir, lastSeen)
}

func correctiveWorkDirCommand(workDir string) (string, error) {
	if err := ValidatePastedText("workdir", workDir); err != nil {
		return "", fmt.Errorf("validate corrective workdir paste: %w", err)
	}
	return "cd " + shellquote.Quote(workDir), nil
}

// PaneCurrentPath returns the current working directory of the session's
// active pane, as reported by the tmux server.
func PaneCurrentPath(sessionName string) (string, error) {
	ctx := context.Background()
	socketPath := GetSocketPath()
	normalizedName := NormalizeTmuxSessionName(sessionName)
	out, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath,
		"display-message", "-p", "-t", normalizedName, "#{pane_current_path}")
	if err != nil {
		return "", fmt.Errorf("failed to read pane_current_path for %q: %w", sessionName, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// samePath reports whether two paths refer to the same directory, tolerating
// symlinks (e.g. sandbox `merged` symlinks resolve to their target in
// pane_current_path) and trailing slashes.
func samePath(a, b string) bool {
	return canonicalPath(a) == canonicalPath(b)
}

// canonicalPath makes the path absolute and resolves symlinks when possible.
// Relative workdirs (e.g. ".") are resolved against the current process cwd,
// matching how tmux resolves a relative `new-session -c` from the client.
func canonicalPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(p)
}
