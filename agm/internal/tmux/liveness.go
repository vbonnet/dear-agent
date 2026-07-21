package tmux

// Harness-process liveness for tmux panes (ce-axsr).
//
// `tmux has-session` and a fresh heartbeat file are both false-green signals:
// the tmux session keeps existing after the harness (claude/codex/agy/…)
// exits and the pane falls back to a bare shell, and an orphaned writer
// process can keep a heartbeat file fresh long after the harness died
// (ce-qkf7: an orphaned agm child wrote meta-o's heartbeat for ~3h while the
// only pane processes were zsh + that agm child).
//
// Real liveness is "a harness process is actually running in the pane's
// process tree". This file owns that scan: resolve the session's pane PIDs,
// read the process table once, walk the descendant tree, and classify. The
// classification core is pure (no tmux, no ps) so it is table-testable with
// fakes.

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// livenessScanTimeout bounds the tmux + ps round-trips for one scan.
const livenessScanTimeout = 2 * time.Second

// ProcEntry is one row of a process table: a PID, its parent, and the
// command name (comm). Comm may be a bare name or a full path.
type ProcEntry struct {
	PID  int
	PPID int
	Comm string
}

// PaneLiveness is the verdict of a harness-process liveness scan for one
// tmux session.
type PaneLiveness struct {
	// SessionExists reports whether the tmux session (and at least one pane)
	// was found at all. When false, every other field is zero-valued: the
	// scan can prove nothing about a session it cannot see.
	SessionExists bool
	// HarnessAlive reports whether a harness process (claude, codex, agy,
	// node, …) is running anywhere in a pane's descendant process tree.
	// This is the only signal that proves the session is genuinely alive.
	HarnessAlive bool
	// ZombieWriter reports the ce-qkf7 failure mode: no harness process is
	// alive, but an agm process is still running in the pane tree — the
	// likely orphaned writer keeping a heartbeat file falsely fresh.
	ZombieWriter bool
	// RestartableShell reports that the session has exactly one pane and every
	// process in its descendant tree is a plain interactive shell. Callers may
	// safely deliver a cold-resume command only when this positive proof is true.
	RestartableShell bool
	// Evidence is a human-readable summary of the pane's descendant process
	// names, so callers can say WHY a session was classified dead.
	Evidence string
}

// harnessComms is the set of process names that count as a live harness
// foreground. "node" is included because Node-based CLIs (Codex, some Claude
// installs) report as node.
var harnessComms = map[string]bool{
	"claude":   true,
	"codex":    true,
	"agy":      true,
	"node":     true,
	"gemini":   true,
	"opencode": true,
}

// IsHarnessComm reports whether a process comm value names a known harness
// binary. Comm may be a full path; only the base name is matched. Claude
// Code's semver process names (e.g. "2.1.50", macOS "2_1_195") also count.
func IsHarnessComm(comm string) bool {
	base := filepath.Base(strings.TrimSpace(comm))
	if harnessComms[base] {
		return true
	}
	return isClaudeProcess(base)
}

// ParsePSTable parses `ps -axo pid=,ppid=,comm=` output into ProcEntry rows.
// Comm is split at the first whitespace after the two numeric columns only,
// so command paths containing spaces survive intact. Malformed lines are
// skipped.
func ParsePSTable(out string) []ProcEntry {
	var entries []ProcEntry
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// pid
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:idx])
		if err != nil {
			continue
		}
		rest := strings.TrimSpace(line[idx:])
		// ppid
		idx = strings.IndexAny(rest, " \t")
		if idx < 0 {
			continue
		}
		ppid, err := strconv.Atoi(rest[:idx])
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(rest[idx:])
		if comm == "" {
			continue
		}
		entries = append(entries, ProcEntry{PID: pid, PPID: ppid, Comm: comm})
	}
	return entries
}

// ClassifyPaneLiveness is the pure classification core: given the session's
// pane PIDs and a process table, it walks the full descendant tree of each
// pane (not just direct children — a harness that crashed and was resumed
// runs as a grandchild under a shell) and classifies the session.
//
// isHarness decides which comm values count as a live harness; pass
// IsHarnessComm for the standard set. The pane processes themselves (the
// shells tmux spawned) are included in the walk, so a pane whose root
// process IS the harness classifies as alive.
func ClassifyPaneLiveness(panePIDs []int, procs []ProcEntry, isHarness func(comm string) bool) PaneLiveness {
	if len(panePIDs) == 0 {
		return PaneLiveness{SessionExists: false}
	}
	children := make(map[int][]ProcEntry, len(procs))
	byPID := make(map[int]ProcEntry, len(procs))
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p)
		byPID[p.PID] = p
	}

	verdict := PaneLiveness{SessionExists: true, RestartableShell: len(panePIDs) == 1}
	var comms []string
	seen := make(map[int]bool)
	processSeen := false
	queue := make([]int, 0, len(panePIDs))
	for _, pid := range panePIDs {
		if p, ok := byPID[pid]; ok {
			queue = append(queue, pid)
			comms = append(comms, filepath.Base(p.Comm))
		}
	}
	for i := 0; i < len(queue); i++ {
		pid := queue[i]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		p, ok := byPID[pid]
		if !ok {
			continue
		}
		processSeen = true
		base := filepath.Base(p.Comm)
		if !IsShellCommand(p.Comm) {
			verdict.RestartableShell = false
		}
		if isHarness(p.Comm) {
			verdict.HarnessAlive = true
		} else if base == "agm" {
			verdict.ZombieWriter = true
		}
		for _, c := range children[pid] {
			if !seen[c.PID] {
				queue = append(queue, c.PID)
				comms = append(comms, filepath.Base(c.Comm))
			}
		}
	}
	// A zombie writer only matters as a verdict when no harness is alive:
	// with a live harness, an agm process in the tree is just normal tooling.
	if verdict.HarnessAlive {
		verdict.ZombieWriter = false
	}
	if !processSeen {
		verdict.RestartableShell = false
	}
	const maxEvidence = 200
	verdict.Evidence = strings.Join(comms, ",")
	// Truncate on a rune boundary: comm values may be paths containing
	// multi-byte UTF-8, and slicing by byte index could produce invalid UTF-8.
	if runes := []rune(verdict.Evidence); len(runes) > maxEvidence {
		verdict.Evidence = string(runes[:maxEvidence]) + "..."
	}
	return verdict
}

// listPanePIDs returns the pane root PIDs for sessionName on socketPath.
// A missing session returns (nil, nil) — absence is a verdict, not an error.
// Only a clean non-zero exit from tmux ("no such session") counts as absence:
// a timeout, a missing tmux binary, or any other execution failure returns an
// error so callers fail safe instead of misreading "could not check" as
// "session is dead".
func listPanePIDs(ctx context.Context, sessionName, socketPath string) ([]int, error) {
	normalized := NormalizeTmuxSessionName(sessionName)
	has := exec.CommandContext(ctx, "tmux", "-S", socketPath, "has-session", "-t", FormatSessionTarget(normalized))
	if err := has.Run(); err != nil {
		// Check the context FIRST: a context-kill also surfaces as an
		// *exec.ExitError (signal: killed), which must not be mistaken for
		// "session does not exist".
		if ctx.Err() != nil {
			return nil, fmt.Errorf("tmux has-session timed out: %w", ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, nil // tmux ran and said: session does not exist
		}
		return nil, fmt.Errorf("tmux has-session failed: %w", err)
	}
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "list-panes", "-s", "-t", FormatSessionTarget(normalized), "-F", "#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	var pids []int
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, convErr := strconv.Atoi(line)
		if convErr != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// readProcessTable runs `ps -axo pid=,ppid=,comm=` and parses the result.
func readProcessTable(ctx context.Context) ([]ProcEntry, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,comm=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	return ParsePSTable(string(out)), nil
}

// CheckPaneLiveness runs the real harness-liveness scan for sessionName on
// socketPath: pane PIDs via tmux, one ps snapshot, then the pure classifier
// with the standard harness set. A missing session returns
// SessionExists=false with a nil error — that IS the verdict.
func CheckPaneLiveness(sessionName, socketPath string) (PaneLiveness, error) {
	return CheckPaneLivenessContext(context.Background(), sessionName, socketPath)
}

// CheckPaneLivenessContext runs the liveness scan under the caller's lifetime
// while retaining the package timeout as an upper bound.
func CheckPaneLivenessContext(parent context.Context, sessionName, socketPath string) (PaneLiveness, error) {
	ctx, cancel := context.WithTimeout(parent, livenessScanTimeout)
	defer cancel()

	pids, err := listPanePIDs(ctx, sessionName, socketPath)
	if err != nil {
		return PaneLiveness{}, err
	}
	if len(pids) == 0 {
		return PaneLiveness{SessionExists: false}, nil
	}
	procs, err := readProcessTable(ctx)
	if err != nil {
		return PaneLiveness{}, err
	}
	return ClassifyPaneLiveness(pids, procs, IsHarnessComm), nil
}

// CheckPaneLivenessBatch scans many sessions with a constant number of
// subprocesses: ONE `tmux list-panes -a` (all panes on the server, tagged
// with their session name) and ONE `ps` snapshot, then the pure classifier
// per requested session. The result map is keyed by the caller's original
// session names; a requested session with no panes on the server reports
// SessionExists=false. Use this instead of per-session CheckPaneLiveness in
// list paths, where N sessions would otherwise mean 3N subprocesses.
func CheckPaneLivenessBatch(sessionNames []string, socketPath string) (map[string]PaneLiveness, error) {
	ctx, cancel := context.WithTimeout(context.Background(), livenessScanTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "list-panes", "-a", "-F", "#{session_name}\t#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("tmux list-panes -a timed out: %w", ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// tmux ran but failed — most commonly "no server running", which
			// means no session exists at all. That IS a verdict for every
			// requested name.
			results := make(map[string]PaneLiveness, len(sessionNames))
			for _, name := range sessionNames {
				results[name] = PaneLiveness{SessionExists: false}
			}
			return results, nil
		}
		return nil, fmt.Errorf("tmux list-panes -a failed: %w", err)
	}

	pidsBySession := make(map[string][]int)
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, "\t")
		if idx < 0 {
			continue
		}
		sessionName := line[:idx]
		pid, convErr := strconv.Atoi(strings.TrimSpace(line[idx+1:]))
		if convErr != nil {
			continue
		}
		pidsBySession[sessionName] = append(pidsBySession[sessionName], pid)
	}

	procs, err := readProcessTable(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]PaneLiveness, len(sessionNames))
	for _, name := range sessionNames {
		pids := pidsBySession[NormalizeTmuxSessionName(name)]
		if len(pids) == 0 {
			results[name] = PaneLiveness{SessionExists: false}
			continue
		}
		results[name] = ClassifyPaneLiveness(pids, procs, IsHarnessComm)
	}
	return results, nil
}

// CheckProcessInPaneTree reports whether a process named processName (exact
// comm or comm base-name match) is running anywhere in the descendant process
// tree of sessionName's panes. It preserves scan errors so lifecycle callers
// can fail safe instead of injecting a command when liveness is unknown.
func CheckProcessInPaneTree(sessionName, socketPath, processName string) (bool, error) {
	return IsProcessInPaneTreeContext(context.Background(), sessionName, socketPath, processName)
}

// IsProcessInPaneTree reports whether a process named processName (exact comm
// or comm base-name match) is running anywhere in the descendant process tree
// of sessionName's panes. Any failure reports false for compatibility with
// existing best-effort liveness callers.
func IsProcessInPaneTree(sessionName, socketPath, processName string) bool {
	running, err := IsProcessInPaneTreeContext(context.Background(), sessionName, socketPath, processName)
	if err != nil {
		return false
	}
	return running
}

// IsProcessInPaneTreeContext is the cancellation-aware process-tree scan used
// by command transactions that must not outlive their caller.
func IsProcessInPaneTreeContext(parent context.Context, sessionName, socketPath, processName string) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, livenessScanTimeout)
	defer cancel()

	pids, err := listPanePIDs(ctx, sessionName, socketPath)
	if err != nil || len(pids) == 0 {
		return false, err
	}
	procs, err := readProcessTable(ctx)
	if err != nil {
		return false, err
	}
	verdict := ClassifyPaneLiveness(pids, procs, func(comm string) bool {
		return comm == processName || filepath.Base(comm) == processName
	})
	return verdict.HarnessAlive, nil
}
