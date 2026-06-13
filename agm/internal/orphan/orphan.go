// Package orphan reaps orphaned long-lived child processes — gopls language
// servers and agm-mcp-server instances — that outlive the Claude Code session
// that spawned them.
//
// Each Claude Code session (Desktop, Cowork, or CLI) starts its own gopls (via
// the gopls-lsp plugin) and its own agm-mcp-server. When a session dies
// abruptly (OOM, kill -9, Desktop crash) those children are NOT reaped by the
// normal Stop/SessionEnd hooks — they are reparented to launchd/init (PID 1)
// and linger, each holding hundreds of MB of RSS. Left unchecked they
// accumulate until the machine swaps (the 2026-06-12 break-glass retro observed
// 14+ gopls instances at ~7GB RSS with only 3 live sessions).
//
// The provable orphan signal this package keys on is PPID==1: a process whose
// parent has died is reparented to PID 1. A gopls/agm-mcp-server with a live
// parent keeps its real PPID, so this criterion never touches a process that
// belongs to a running session. That makes the reap safe by construction — no
// session-to-process mapping or worktree bookkeeping is required.
package orphan

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// DefaultTargets are the process command names this reaper considers. These are
// the per-session children that leak: the Go language server and the agm MCP
// server. Matching is done against the basename of the executable (ps `comm`),
// so a full path like /opt/homebrew/bin/gopls still matches "gopls".
var DefaultTargets = []string{"gopls", "agm-mcp-server"}

// orphanPPID is the parent PID a process is reparented to once its real parent
// dies. On both Linux and macOS this is PID 1 (init / launchd).
const orphanPPID = 1

// Proc is a single observed OS process.
type Proc struct {
	PID     int
	PPID    int
	Command string // executable basename (ps `comm`)
	CmdLine string // full argv (ps `args`), for logging/diagnostics
}

// IsOrphan reports whether p is an orphaned instance of one of targets: its
// command basename matches a target AND it has been reparented to PID 1.
func (p Proc) IsOrphan(targets []string) bool {
	if p.PPID != orphanPPID {
		return false
	}
	return slices.Contains(targets, basename(p.Command))
}

// FindOrphans filters procs down to the orphaned target processes, excluding
// selfPID (so the reaper never selects itself even if it were somehow a match).
// It is pure — no process is signalled — which makes it directly unit-testable.
func FindOrphans(procs []Proc, targets []string, selfPID int) []Proc {
	var out []Proc
	for _, p := range procs {
		if p.PID == selfPID {
			continue
		}
		if p.IsOrphan(targets) {
			out = append(out, p)
		}
	}
	return out
}

// ProcLister enumerates the OS process table.
type ProcLister interface {
	List() ([]Proc, error)
}

// ProcessKiller terminates a process by PID. It is satisfied by
// mcp.SignalKiller (SIGTERM, grace period, then SIGKILL) and by fakes in tests.
type ProcessKiller interface {
	Kill(pid int) error
}

// PSLister enumerates processes via `ps`. It works on macOS and Linux without a
// /proc dependency, which is what this host (darwin) needs.
type PSLister struct{}

// List runs `ps -ax -o pid=,ppid=,comm=,args=` and parses each row into a Proc.
func (PSLister) List() ([]Proc, error) {
	out, err := exec.Command("ps", "-ax", "-o", "pid=,ppid=,comm=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps failed: %w", err)
	}
	return parsePS(string(out)), nil
}

// parsePS parses the output of `ps -ax -o pid=,ppid=,comm=,args=`. Each line is
// "<pid> <ppid> <comm> <args...>" with leading whitespace-padded numeric
// columns. comm may itself contain spaces (rare), so we anchor on the two
// leading integer columns and treat the first whitespace-delimited token after
// them as comm and the remainder as args.
func parsePS(out string) []Proc {
	var procs []Proc
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// pid
		rest := line
		pidTok, rest := nextField(rest)
		pid, err := strconv.Atoi(pidTok)
		if err != nil {
			continue
		}
		// ppid
		ppidTok, rest := nextField(rest)
		ppid, err := strconv.Atoi(ppidTok)
		if err != nil {
			continue
		}
		// comm (first remaining token), args (everything from comm onward).
		commTok, _ := nextField(rest)
		if commTok == "" {
			continue
		}
		procs = append(procs, Proc{
			PID:     pid,
			PPID:    ppid,
			Command: commTok,
			CmdLine: rest,
		})
	}
	return procs
}

// nextField splits off the first whitespace-delimited token, returning it and
// the remainder with leading whitespace trimmed.
func nextField(s string) (token, rest string) {
	s = strings.TrimLeft(s, " \t")
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeft(s[i:], " \t")
}

func basename(s string) string {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// Result reports the outcome of a reap pass.
type Result struct {
	Targets   []string // process names considered
	Orphans   []Proc   // orphans found this pass
	Killed    []Proc   // orphans successfully signalled (empty when DryRun)
	Failed    []Proc   // orphans whose kill returned an error
	DryRun    bool
	KillError map[int]string // PID -> error message for Failed entries
}

// Reap finds orphaned target processes and kills them (unless dryRun). It is
// best-effort: a kill failure on one orphan is recorded and does not abort the
// pass. Errors from listing are returned because they make the result
// meaningless.
func Reap(lister ProcLister, killer ProcessKiller, targets []string, dryRun bool, logger *slog.Logger) (Result, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if len(targets) == 0 {
		targets = DefaultTargets
	}

	procs, err := lister.List()
	if err != nil {
		return Result{}, fmt.Errorf("listing processes: %w", err)
	}

	orphans := FindOrphans(procs, targets, os.Getpid())
	res := Result{
		Targets:   targets,
		Orphans:   orphans,
		DryRun:    dryRun,
		KillError: map[int]string{},
	}

	for _, o := range orphans {
		if dryRun {
			logger.Info("would reap orphan", "pid", o.PID, "command", o.Command, "cmdline", truncate(o.CmdLine, 120))
			continue
		}
		logger.Info("reaping orphan", "pid", o.PID, "command", o.Command, "cmdline", truncate(o.CmdLine, 120))
		if err := killer.Kill(o.PID); err != nil {
			res.Failed = append(res.Failed, o)
			res.KillError[o.PID] = err.Error()
			logger.Warn("failed to reap orphan", "pid", o.PID, "command", o.Command, "error", err)
			continue
		}
		res.Killed = append(res.Killed, o)
	}

	return res, nil
}

// truncate shortens s to at most maxLen runes (not bytes), so a process
// command line containing multi-byte UTF-8 (e.g. a localized path) is never
// sliced mid-character into invalid UTF-8 when logged.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
