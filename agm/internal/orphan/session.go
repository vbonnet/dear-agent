package orphan

// session.go adds a *session-scoped* reap that complements the PPID==1 reaper in
// orphan.go. The PPID==1 signal only fires once a child has already been
// reparented to init — i.e. after its session has fully exited. That misses the
// graceful session-end window: when the SessionEnd hook runs, the ending
// session's gopls/agm-mcp-server are still parented to the (about-to-exit)
// session process, so their PPID is not yet 1 and the orphan reaper skips them.
// On a quiet machine (no other session generating Stop events) they then linger
// until the next hourly launchd sweep.
//
// ReapSession closes that window. It resolves the session root — the nearest
// ancestor of the reaper whose command basename is one of rootNames (default
// "claude") — and reaps the target processes in that root's subtree. Because we
// only ever descend from an ancestor of the reaper itself, the subtree is
// exactly "this session and its descendants": a sibling session is structurally
// unreachable, which preserves the same safety guarantee the PPID==1 reaper has.

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
)

// DefaultRootNames are the command basenames treated as a session root. Each
// Claude Code session (Desktop, Cowork, CLI) is rooted at a `claude` process,
// and that session's gopls/agm-mcp-server are its descendants.
var DefaultRootNames = []string{"claude"}

// FindAncestor walks the parent chain upward from startPID (inclusive) and
// returns the PID of the first process whose command basename is in rootNames.
// ok is false when no such ancestor exists, the chain is broken (a PPID missing
// from procs), or a cycle is detected. It is pure and signal-free.
func FindAncestor(procs []Proc, startPID int, rootNames []string) (rootPID int, ok bool) {
	byPID := make(map[int]Proc, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}

	seen := make(map[int]bool)
	cur := startPID
	for cur > 1 {
		if seen[cur] {
			return 0, false // cycle guard
		}
		seen[cur] = true

		p, found := byPID[cur]
		if !found {
			return 0, false // broken chain
		}
		if slices.Contains(rootNames, basename(p.Command)) {
			return p.PID, true
		}
		cur = p.PPID
	}
	return 0, false
}

// FindDescendants returns the target processes within the subtree rooted at
// rootPID, excluding rootPID itself and selfPID. Targets are matched on command
// basename. It is pure and signal-free, so it is directly unit-testable.
func FindDescendants(procs []Proc, rootPID int, targets []string, selfPID int) []Proc {
	children := make(map[int][]Proc)
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p)
	}

	var out []Proc
	seen := make(map[int]bool)
	queue := []int{rootPID}
	seen[rootPID] = true
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		for _, child := range children[parent] {
			if seen[child.PID] {
				continue // cycle guard
			}
			seen[child.PID] = true
			queue = append(queue, child.PID)
			if child.PID == selfPID || child.PID == rootPID {
				continue
			}
			if isTarget(child.Command, targets) {
				out = append(out, child)
			}
		}
	}
	return out
}

func isTarget(command string, targets []string) bool {
	return slices.Contains(targets, basename(command))
}

// SessionResult reports the outcome of a session-scoped reap pass.
type SessionResult struct {
	Targets   []string // process names considered
	RootNames []string // ancestor command names treated as a session root
	StartPID  int      // PID the ancestor walk started from
	RootPID   int      // resolved session-root PID (0 when none found)
	RootFound bool     // whether a session root was resolved
	Found     []Proc   // session-descendant targets found this pass
	Killed    []Proc   // successfully signalled (empty when DryRun)
	Failed    []Proc   // targets whose kill returned an error
	DryRun    bool
	KillError map[int]string // PID -> error message for Failed entries
}

// ReapSession finds and kills the ending session's own gopls/agm-mcp-server
// descendants. It resolves the session root by walking up from startPID (the
// reaper's parent — typically the SessionEnd hook shell), unless rootPIDOverride
// is > 0, in which case that PID is used directly as the root.
//
// When no session root can be resolved it returns a safe empty result (no kills,
// no error): better to reap nothing than to guess at a root and risk a sibling
// session. It is best-effort — a kill failure on one process is recorded and does
// not abort the pass. Only a listing failure returns an error.
func ReapSession(lister ProcLister, killer ProcessKiller, startPID, rootPIDOverride int, rootNames, targets []string, dryRun bool, logger *slog.Logger) (SessionResult, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if len(targets) == 0 {
		targets = DefaultTargets
	}
	if len(rootNames) == 0 {
		rootNames = DefaultRootNames
	}

	procs, err := lister.List()
	if err != nil {
		return SessionResult{}, fmt.Errorf("listing processes: %w", err)
	}

	res := SessionResult{
		Targets:   targets,
		RootNames: rootNames,
		StartPID:  startPID,
		DryRun:    dryRun,
		KillError: map[int]string{},
	}

	if rootPIDOverride > 0 {
		res.RootPID, res.RootFound = rootPIDOverride, true
	} else {
		res.RootPID, res.RootFound = FindAncestor(procs, startPID, rootNames)
	}
	if !res.RootFound {
		logger.Info("no session root resolved; nothing to reap", "start_pid", startPID, "root_names", rootNames)
		return res, nil
	}

	res.Found = FindDescendants(procs, res.RootPID, targets, os.Getpid())

	for _, o := range res.Found {
		if dryRun {
			logger.Info("would reap session process", "pid", o.PID, "command", o.Command, "root_pid", res.RootPID)
			continue
		}
		logger.Info("reaping session process", "pid", o.PID, "command", o.Command, "root_pid", res.RootPID, "cmdline", truncate(o.CmdLine, 120))
		if err := killer.Kill(o.PID); err != nil {
			res.Failed = append(res.Failed, o)
			res.KillError[o.PID] = err.Error()
			logger.Warn("failed to reap session process", "pid", o.PID, "command", o.Command, "error", err)
			continue
		}
		res.Killed = append(res.Killed, o)
	}

	return res, nil
}
