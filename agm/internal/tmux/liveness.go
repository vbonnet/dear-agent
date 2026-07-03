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

	verdict := PaneLiveness{SessionExists: true}
	var comms []string
	seen := make(map[int]bool)
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
		base := filepath.Base(p.Comm)
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
	const maxEvidence = 200
	verdict.Evidence = strings.Join(comms, ",")
	if len(verdict.Evidence) > maxEvidence {
		verdict.Evidence = verdict.Evidence[:maxEvidence] + "..."
	}
	return verdict
}

// listPanePIDs returns the pane root PIDs for sessionName on socketPath.
// A missing session returns (nil, nil) — absence is a verdict, not an error.
func listPanePIDs(ctx context.Context, sessionName, socketPath string) ([]int, error) {
	normalized := NormalizeTmuxSessionName(sessionName)
	has := exec.CommandContext(ctx, "tmux", "-S", socketPath, "has-session", "-t", FormatSessionTarget(normalized))
	if err := has.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("tmux has-session timed out: %w", ctx.Err())
		}
		return nil, nil // session does not exist
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
	ctx, cancel := context.WithTimeout(context.Background(), livenessScanTimeout)
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

// IsProcessInPaneTree reports whether a process named processName (exact comm
// or comm base-name match) is running anywhere in the descendant process tree
// of sessionName's panes. This is the generalized, full-tree successor of the
// direct-children-only scan that previously lived in internal/safety.
// Any failure (timeout, tmux error, missing session) reports false — callers
// use this as a "prove it is running" check.
func IsProcessInPaneTree(sessionName, socketPath, processName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), livenessScanTimeout)
	defer cancel()

	pids, err := listPanePIDs(ctx, sessionName, socketPath)
	if err != nil || len(pids) == 0 {
		return false
	}
	procs, err := readProcessTable(ctx)
	if err != nil {
		return false
	}
	verdict := ClassifyPaneLiveness(pids, procs, func(comm string) bool {
		return comm == processName || filepath.Base(comm) == processName
	})
	return verdict.HarnessAlive
}
