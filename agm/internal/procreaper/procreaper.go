// Package procreaper implements an AGM-aware process reaper. Under memory
// pressure it kills high-RSS / idle processes to reclaim RAM, but it consults
// the AGM session database first and NEVER signals a process that belongs to a
// supervised (live) session.
//
// This is the session-DB-aware counterpart to agm/internal/orphan, which only
// reaps processes reparented to PID 1 (provably orphaned). The orphan reaper is
// safe by construction but blind to memory hogs whose parent is still alive
// (e.g. a wedged gopls inside a stalled-but-not-dead session, or a runaway
// child of a session that should be drained, not preserved). This reaper closes
// that gap: it ranks processes by resident memory / idle time and reaps the
// worst offenders, with the supervised-session set as an absolute exclusion.
//
// Safety model (in priority order, all enforced before any signal is sent):
//
//  1. Fail-closed on an unknown supervised set. If the session DB cannot be
//     queried, Reap returns an error and kills nothing. We never guess.
//  2. Supervised subtree protection. The pane PID of every live session, and
//     every descendant of those PIDs, is protected. A session is a process
//     tree, not a single PID.
//  3. Name protection. Any process whose command matches a protected role
//     (orchestrator, overseer, meta-) is skipped regardless of RSS.
//  4. Self/init protection. The reaper never signals itself or PID 1.
//
// The kill itself is graduated: SIGTERM, then a grace window, then SIGKILL only
// if the process is still alive. The orchestration is built on seams
// (SessionSupervisor, ProcessLister, Signaler, clock) so the whole reap loop is
// unit-testable without touching the real OS or a live AGM database.
package procreaper

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
)

// ProcessInfo describes one observed OS process. RSSKB and IdleSecs drive the
// candidate filter; PID/PPID drive the supervised-subtree expansion.
type ProcessInfo struct {
	// PID is the process id.
	PID int
	// PPID is the parent process id, used to expand the supervised subtree.
	PPID int
	// RSSKB is resident set size in kibibytes.
	RSSKB int64
	// IdleSecs is seconds since the process last showed activity. Zero means
	// "unknown" and is treated as not-idle (we do not reap on an unknown idle).
	IdleSecs int64
	// Command is the executable basename (e.g. "gopls").
	Command string
	// CmdLine is the full argv, used for name-based protection matching.
	CmdLine string
	// SessionID is the AGM session this PID was matched to, when protected.
	// Populated by the reaper for processes placed in Protected.
	SessionID string
}

// ReapRequest configures one reap pass.
type ReapRequest struct {
	// MaxRSSMB: only processes using strictly more than this many MiB of RSS
	// are candidates. Zero means every process is a candidate by RSS (idle/
	// protection filters still apply).
	MaxRSSMB int64
	// MinIdle: when > 0, a candidate must also have been idle at least this
	// long. A process with unknown idle time (IdleSecs == 0) never satisfies a
	// positive MinIdle, so it is not reaped on idle grounds.
	MinIdle time.Duration
	// GracePeriod is the SIGTERM→SIGKILL window. Zero uses DefaultGracePeriod.
	GracePeriod time.Duration
	// DryRun computes the kill set and protections but sends no signals.
	DryRun bool
}

// ReapResult reports the outcome of one pass.
type ReapResult struct {
	// Candidates matched the RSS/idle filter (before protection).
	Candidates []ProcessInfo
	// Protected matched the filter but belong to a supervised session or a
	// protected role; they were skipped.
	Protected []ProcessInfo
	// Reapable is Candidates minus Protected — the processes eligible to kill.
	Reapable []ProcessInfo
	// Terminated were actually signalled (empty on a dry run).
	Terminated []ProcessInfo
	// Errors collects best-effort per-process failures; one failed kill never
	// aborts the rest of the pass.
	Errors []string
	// DryRun echoes the request flag.
	DryRun bool
}

// DefaultGracePeriod is the SIGTERM→SIGKILL window when ReapRequest leaves it
// zero. It mirrors the 10s grace used by the session reaper (REA-12).
const DefaultGracePeriod = 10 * time.Second

// gracePollSteps is how many liveness checks the grace window is split into.
// More steps means a process that exits early on SIGTERM is detected sooner
// (we skip the SIGKILL) at the cost of more polling.
const gracePollSteps = 20

// SessionSupervisor reports the root PIDs of supervised (live) AGM sessions:
// the pane PID of every session the AGM database considers active. The reaper
// expands these roots to their full process subtrees using the process table,
// so the supervisor only has to know the roots, not every descendant.
//
// Returning an error is a hard stop: the reaper refuses to kill anything when
// the supervised set is unknown (fail-closed). An empty map with a nil error
// legitimately means "no live sessions" and reaping proceeds.
type SessionSupervisor interface {
	SupervisedPIDs(ctx context.Context) (map[int]string, error)
}

// ProcessLister enumerates the full OS process table (not just candidates):
// the reaper needs every PID/PPID to expand supervised subtrees correctly.
type ProcessLister interface {
	List(ctx context.Context) ([]ProcessInfo, error)
}

// Signaler sends signals and checks liveness. Term/Kill must be non-blocking;
// the reaper does its own grace-period waiting between them via the clock seam.
type Signaler interface {
	// Term sends SIGTERM to pid.
	Term(pid int) error
	// Kill sends SIGKILL to pid.
	Kill(pid int) error
	// Alive reports whether pid is still running.
	Alive(pid int) bool
}

// DefaultProtectedRoles are command/argv substrings that are never reaped,
// matching the operator rule "NEVER kill sessions matching: orchestrator,
// overseer, meta-". Matching is case-insensitive substring against both the
// command basename and the full argv.
var DefaultProtectedRoles = []string{"orchestrator", "overseer", "meta-"}

// Reaper is the AGM-aware process reaper. Construct it with New and drive it
// with Reap. All external effects go through the seams, so the reap loop is
// deterministic under test.
type Reaper struct {
	sessions       SessionSupervisor
	procs          ProcessLister
	signaler       Signaler
	logger         *slog.Logger
	sleep          func(time.Duration)
	selfPID        int
	protectedRoles []string
}

// Option configures a Reaper.
type Option func(*Reaper)

// WithLogger sets the logger (default: slog.Default()).
func WithLogger(l *slog.Logger) Option {
	return func(r *Reaper) {
		if l != nil {
			r.logger = l
		}
	}
}

// WithProtectedRoles overrides the protected role substrings.
func WithProtectedRoles(roles []string) Option {
	return func(r *Reaper) { r.protectedRoles = roles }
}

// WithSelfPID overrides the self PID that is always protected (default
// os.Getpid via New). Exposed for tests.
func WithSelfPID(pid int) Option {
	return func(r *Reaper) { r.selfPID = pid }
}

// withClock overrides the sleep seam (tests inject a no-op to fast-forward the
// grace window). Unexported: production always sleeps for real.
func withClock(sleep func(time.Duration)) Option {
	return func(r *Reaper) {
		if sleep != nil {
			r.sleep = sleep
		}
	}
}

// New constructs a Reaper. sessions, procs, and signaler are required.
func New(sessions SessionSupervisor, procs ProcessLister, signaler Signaler, selfPID int, opts ...Option) (*Reaper, error) {
	if sessions == nil {
		return nil, fmt.Errorf("procreaper: SessionSupervisor is required")
	}
	if procs == nil {
		return nil, fmt.Errorf("procreaper: ProcessLister is required")
	}
	if signaler == nil {
		return nil, fmt.Errorf("procreaper: Signaler is required")
	}
	r := &Reaper{
		sessions:       sessions,
		procs:          procs,
		signaler:       signaler,
		logger:         slog.Default(),
		sleep:          time.Sleep,
		selfPID:        selfPID,
		protectedRoles: DefaultProtectedRoles,
	}
	for _, o := range opts {
		o(r)
	}
	return r, nil
}

// Reap performs one pass: determine the supervised set (fail-closed on error),
// enumerate processes, filter to candidates by RSS/idle, subtract the protected
// subtree and protected roles, then SIGTERM→grace→SIGKILL the remainder.
func (r *Reaper) Reap(ctx context.Context, req ReapRequest) (ReapResult, error) {
	res := ReapResult{DryRun: req.DryRun}

	// (1) Fail-closed: never kill when the supervised set is unknown.
	roots, err := r.sessions.SupervisedPIDs(ctx)
	if err != nil {
		return ReapResult{}, fmt.Errorf("procreaper: cannot determine supervised PIDs, refusing to kill: %w", err)
	}

	procs, err := r.procs.List(ctx)
	if err != nil {
		return ReapResult{}, fmt.Errorf("procreaper: list processes: %w", err)
	}

	// (2) Expand supervised roots to their full subtrees. A session is a
	// process tree; protecting only the pane PID would leave its children
	// (gopls, MCP server, shells) exposed.
	protected := expandSubtree(roots, procs)

	grace := req.GracePeriod
	if grace <= 0 {
		grace = DefaultGracePeriod
	}

	for _, p := range procs {
		if !r.isCandidate(p, req) {
			continue
		}
		res.Candidates = append(res.Candidates, p)

		// (3) Protection gates — checked in order, all before any signal.
		if sid, ok := protected[p.PID]; ok {
			p.SessionID = sid
			res.Protected = append(res.Protected, p)
			continue
		}
		if r.protectedByRole(p) {
			p.SessionID = "protected-role"
			res.Protected = append(res.Protected, p)
			continue
		}
		if p.PID == r.selfPID || p.PID == 1 {
			p.SessionID = "self-or-init"
			res.Protected = append(res.Protected, p)
			continue
		}

		res.Reapable = append(res.Reapable, p)
	}

	if req.DryRun {
		r.logger.Info("procreaper dry-run",
			"candidates", len(res.Candidates),
			"protected", len(res.Protected),
			"reapable", len(res.Reapable))
		return res, nil
	}

	for _, p := range res.Reapable {
		if err := r.terminate(p.PID, grace); err != nil {
			res.Errors = append(res.Errors, err.Error())
			r.logger.Warn("procreaper kill failed", "pid", p.PID, "command", p.Command, "error", err)
			continue
		}
		res.Terminated = append(res.Terminated, p)
		r.logger.Info("procreaper reaped", "pid", p.PID, "command", p.Command, "rss_kb", p.RSSKB)
	}

	return res, nil
}

// isCandidate applies the RSS and idle filters.
func (r *Reaper) isCandidate(p ProcessInfo, req ReapRequest) bool {
	if req.MaxRSSMB > 0 && p.RSSKB <= req.MaxRSSMB*1024 {
		return false
	}
	if req.MinIdle > 0 {
		// Unknown idle (0) never satisfies a positive MinIdle.
		if p.IdleSecs <= 0 || time.Duration(p.IdleSecs)*time.Second < req.MinIdle {
			return false
		}
	}
	return true
}

// protectedByRole reports whether p matches a protected role substring on
// either its command basename or full argv (case-insensitive).
func (r *Reaper) protectedByRole(p ProcessInfo) bool {
	hay := strings.ToLower(basename(p.Command) + " " + p.CmdLine)
	for _, role := range r.protectedRoles {
		if role == "" {
			continue
		}
		if strings.Contains(hay, strings.ToLower(role)) {
			return true
		}
	}
	return false
}

// terminate implements the graduated escalation: SIGTERM, poll for exit across
// the grace window, then SIGKILL only if the process is still alive.
func (r *Reaper) terminate(pid int, grace time.Duration) error {
	if err := r.signaler.Term(pid); err != nil {
		return fmt.Errorf("SIGTERM pid %d: %w", pid, err)
	}

	step := grace / gracePollSteps
	if step <= 0 {
		step = time.Millisecond
	}
	for range gracePollSteps {
		if !r.signaler.Alive(pid) {
			return nil // exited gracefully on SIGTERM — no SIGKILL needed
		}
		r.sleep(step)
	}

	if !r.signaler.Alive(pid) {
		return nil
	}
	if err := r.signaler.Kill(pid); err != nil {
		return fmt.Errorf("SIGKILL pid %d: %w", pid, err)
	}
	return nil
}

// expandSubtree returns the set of all PIDs that are roots or transitive
// descendants of roots, mapped to the owning session id. Roots whose PID is not
// in the process table are still protected (the root itself), even if we cannot
// see its children. The walk is bounded by len(procs) iterations so a cyclic or
// self-parented table cannot loop forever.
func expandSubtree(roots map[int]string, procs []ProcessInfo) map[int]string {
	protected := make(map[int]string, len(roots))
	for pid, sid := range roots {
		if pid > 0 {
			protected[pid] = sid
		}
	}
	if len(protected) == 0 {
		return protected
	}

	// Index children by parent for an efficient BFS.
	children := make(map[int][]ProcessInfo, len(procs))
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p)
	}

	queue := make([]int, 0, len(protected))
	for pid := range protected {
		queue = append(queue, pid)
	}
	// Bound the walk: each process is enqueued at most once because we only
	// enqueue PIDs not already in `protected`.
	for len(queue) > 0 {
		parent := queue[0]
		queue = queue[1:]
		sid := protected[parent]
		for _, child := range children[parent] {
			if child.PID == parent {
				continue // self-parented row — would loop
			}
			if _, seen := protected[child.PID]; seen {
				continue
			}
			protected[child.PID] = sid
			queue = append(queue, child.PID)
		}
	}
	return protected
}

// basename returns the file basename of a possibly-pathed command name.
func basename(cmd string) string {
	if cmd == "" {
		return cmd
	}
	return filepath.Base(cmd)
}
