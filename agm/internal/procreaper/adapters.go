package procreaper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// PSProcessLister enumerates the OS process table via `ps`, the same approach
// the orphan reaper uses (works on macOS and Linux without a /proc dependency).
// RSS is reported by ps in kibibytes. Idle time is not exposed by ps in a
// portable way, so IdleSecs is left 0 (unknown) and the RSS filter drives
// candidate selection; the Reaper treats unknown idle as not-idle, so this is
// safe.
type PSProcessLister struct{}

// List runs `ps -ax -o pid=,ppid=,rss=,comm=,args=` and parses each row.
func (PSProcessLister) List(ctx context.Context) ([]ProcessInfo, error) {
	out, err := exec.CommandContext(ctx, "ps", "-ax", "-o", "pid=,ppid=,rss=,comm=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("ps failed: %w", err)
	}
	return parsePS(string(out)), nil
}

// parsePS parses `ps -ax -o pid=,ppid=,rss=,comm=,args=`. Each row is
// "<pid> <ppid> <rss> <comm> <args...>". As in agm/internal/orphan, the command
// is taken from argv[0] (the args column) rather than comm, because macOS
// truncates comm to 16 chars when it is not the last column.
func parsePS(out string) []ProcessInfo {
	var procs []ProcessInfo
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pidTok, rest := nextField(line)
		pid, err := strconv.Atoi(pidTok)
		if err != nil {
			continue
		}
		ppidTok, rest := nextField(rest)
		ppid, err := strconv.Atoi(ppidTok)
		if err != nil {
			continue
		}
		rssTok, rest := nextField(rest)
		rss, err := strconv.ParseInt(rssTok, 10, 64)
		if err != nil {
			continue
		}
		commTok, args := nextField(rest)
		if commTok == "" {
			continue
		}
		argv0, _ := nextField(args)
		command, cmdline := argv0, args
		if command == "" {
			command, cmdline = commTok, rest
		}
		procs = append(procs, ProcessInfo{
			PID:     pid,
			PPID:    ppid,
			RSSKB:   rss,
			Command: command,
			CmdLine: cmdline,
		})
	}
	return procs
}

// nextField splits off the first whitespace-delimited token, returning it and
// the remainder (leading whitespace trimmed).
func nextField(s string) (field, rest string) {
	s = strings.TrimLeft(s, " \t")
	i := strings.IndexAny(s, " \t")
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeft(s[i:], " \t")
}

// SignalSignaler is the production Signaler. Term/Kill send SIGTERM/SIGKILL and
// Alive probes with signal 0. A nil os.Process is never constructed; we call
// syscall.Kill directly so liveness checks do not allocate.
type SignalSignaler struct{}

// Term sends SIGTERM to pid.
func (SignalSignaler) Term(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }

// Kill sends SIGKILL to pid.
func (SignalSignaler) Kill(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }

// Alive reports whether pid exists by sending signal 0. ESRCH means gone;
// EPERM means it exists but is owned by another user (still "alive").
func (SignalSignaler) Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err == syscall.EPERM
}

// AGMSessionSupervisor resolves the supervised root PIDs by listing active AGM
// sessions and mapping each to its tmux pane PID. It is fail-closed: if the
// session list cannot be obtained, SupervisedPIDs returns an error and the
// Reaper kills nothing. A session whose pane PID cannot be resolved is logged
// and skipped (it contributes no protection), which is the one residual risk
// this adapter documents — prefer a too-large protected set over a too-small
// one when extending it.
type AGMSessionSupervisor struct {
	// AGMBin is the agm binary (default "agm", resolved on PATH).
	AGMBin string
	// Timeout bounds the `agm session list` call. Zero uses 15s.
	Timeout time.Duration
	// Logger receives pane-resolution warnings. Nil uses slog.Default().
	Logger *slog.Logger

	// run executes `agm session list` and returns stdout; nil uses exec.
	run func(ctx context.Context, name string, args ...string) ([]byte, error)
	// panePID resolves a session name to its pane PID; nil uses tmux.GetPanePID.
	panePID func(sessionName string) (int, error)
}

// SupervisedPIDs implements SessionSupervisor.
func (s *AGMSessionSupervisor) SupervisedPIDs(ctx context.Context) (map[int]string, error) {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin := s.AGMBin
	if bin == "" {
		bin = "agm"
	}
	// `agm session list` defaults to active sessions (omit --all), and
	// --output json is the global persistent flag. This yields exactly the
	// supervised set we must protect.
	out, err := s.exec(ctx, bin, "session", "list", "--output", "json")
	if err != nil {
		return nil, fmt.Errorf("agm session list: %w", err)
	}

	var result ops.ListSessionsResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("agm session list: unparseable JSON: %w", err)
	}

	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}
	resolve := s.panePID
	if resolve == nil {
		resolve = tmux.GetPanePID
	}

	roots := make(map[int]string, len(result.Sessions))
	for _, sess := range result.Sessions {
		pid, perr := resolve(sess.Name)
		if perr != nil || pid <= 0 {
			logger.Warn("procreaper: could not resolve pane PID for active session",
				"session", sess.Name, "error", perr)
			continue
		}
		roots[pid] = sess.Name
	}
	return roots, nil
}

func (s *AGMSessionSupervisor) exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	if s.run != nil {
		return s.run(ctx, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...) //#nosec G204 -- fixed binary, constant flags
	return cmd.Output()
}

// ensure the production adapters satisfy the seams at compile time.
var (
	_ ProcessLister     = PSProcessLister{}
	_ Signaler          = SignalSignaler{}
	_ SessionSupervisor = (*AGMSessionSupervisor)(nil)
)

// SelfPID returns the current process id, the value callers pass to New so the
// reaper never signals itself.
func SelfPID() int { return os.Getpid() }
