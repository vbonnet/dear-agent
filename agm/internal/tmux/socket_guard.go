package tmux

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Socket-deletion safety (ce-7ep9).
//
// Unlinking a Unix socket does NOT stop the server listening on it. A tmux
// server whose socket file has been removed keeps running with its listening
// fd open, but becomes permanently unreachable: every `tmux -S <path> ...`
// fails with "error connecting to <path> (No such file or directory)", and the
// server never recreates the file. Its sessions are orphaned and invisible,
// and the next `agm session new` silently starts a *second* server on the same
// path — so the earlier sessions can never be recovered.
//
// The original CleanStaleSocket treated a single failed net.Dial as proof the
// server was dead, and unlinked the socket. A dial can fail for reasons that
// have nothing to do with server death: a full listen backlog, a server busy
// under load, or a slow machine exceeding the timeout. That inverted the burden
// of proof — absence of evidence of life was taken as evidence of death — and
// so the "cleanup" itself created the outage it was meant to repair.
//
// The rule here is the inverse: only unlink after positively proving that no
// server is bound to the path. Every probe below can only ever make us *more*
// conservative about deleting.

// Liveness probe timeouts, kept short: these run on the session-creation path.
const (
	dialProbeTimeout    = 1 * time.Second
	commandProbeTimeout = 3 * time.Second
	pidProbeTimeout     = 5 * time.Second
)

// Probes are indirected through package vars so the decision ladder in
// CleanStaleSocket can be unit-tested without a real tmux server.
var (
	probeDialable    = socketDialable
	probeTmuxCommand = tmuxCommandWorks
	probeServerPIDs  = FindServerPIDs
)

// LiveServerBoundError reports that a socket is unreachable while a live tmux
// server process is still bound to it — the orphaned-server state. Removing the
// socket file cannot fix this and makes it permanent, so the caller must kill
// the bound server instead.
type LiveServerBoundError struct {
	SocketPath string
	PIDs       []int
}

func (e *LiveServerBoundError) Error() string {
	pids := make([]string, len(e.PIDs))
	for i, p := range e.PIDs {
		pids[i] = strconv.Itoa(p)
	}
	joined := strings.Join(pids, " ")
	return fmt.Sprintf(
		"tmux server is bound to %s but unreachable (orphaned): pid(s) %s\n\n"+
			"Removing the socket file will NOT recover these sessions and makes the\n"+
			"orphan permanent. Kill the bound server so a fresh socket can be created:\n"+
			"  kill %s\n"+
			"  agm session list    # verify recovery",
		e.SocketPath, joined, joined)
}

// FindServerPIDs returns the PIDs of live tmux processes bound to socketPath.
//
// This is the only probe that still works once the socket file is gone, which
// makes it the authoritative check for the orphaned-server state: a socket
// connect cannot distinguish "no server" from "server whose file was unlinked".
//
// Matching is on whole argv tokens (`-S` followed by exactly socketPath) rather
// than substring, so /tmp/a.sock does not match /tmp/a.sock.bak.
func FindServerPIDs(socketPath string) ([]int, error) {
	if socketPath == "" {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), pidProbeTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to scan process table: %w", err)
	}

	var pids []int
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		// Skip our own process so a scan can never indict its caller.
		if pid == os.Getpid() {
			continue
		}
		argv := fields[1:]
		if !isTmuxCommand(argv[0]) {
			continue
		}
		for i := 0; i < len(argv)-1; i++ {
			if argv[i] == "-S" && argv[i+1] == socketPath {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids, nil
}

// isTmuxCommand reports whether an argv[0] names the tmux binary, with or
// without a leading path (/opt/homebrew/bin/tmux, tmux).
func isTmuxCommand(arg0 string) bool {
	if idx := strings.LastIndex(arg0, "/"); idx >= 0 {
		arg0 = arg0[idx+1:]
	}
	return arg0 == "tmux"
}

// socketDialable reports whether a Unix socket connect to path succeeds.
func socketDialable(path string) bool {
	conn, err := net.DialTimeout("unix", path, dialProbeTimeout) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// tmuxCommandWorks reports whether tmux itself can talk to the server. A dial
// may fail while the server is merely busy, so spawning the real client is the
// more reliable (and more expensive) second opinion.
func tmuxCommandWorks(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), commandProbeTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "tmux", "-S", path, "list-sessions").Run() == nil
}

// OrphanedServer describes a tmux server that is running but unreachable
// because its socket file no longer exists.
type OrphanedServer struct {
	SocketPath string
	PIDs       []int
}

// DetectOrphanedServer reports a running-but-unreachable tmux server on the AGM
// socket, returning nil when the system is healthy.
//
// This is the monitoring counterpart to the CleanStaleSocket guard: the guard
// stops AGM from *creating* orphans, while this detects orphans created by
// anything else (an external `rm`, a cleanup script, an older AGM build).
// Without it the state is invisible — `agm doctor` sees a missing socket file
// and cheerfully reports that it "will be created on first use", which is
// precisely the wrong message while live sessions are stranded behind it.
func DetectOrphanedServer() (*OrphanedServer, error) {
	socketPath := GetSocketPath()

	// A reachable server is healthy regardless of what the process table says.
	if probeDialable(socketPath) || probeTmuxCommand(socketPath) {
		return nil, nil
	}

	pids, err := probeServerPIDs(socketPath)
	if err != nil {
		return nil, err
	}
	if len(pids) == 0 {
		return nil, nil // no server at all: not orphaned, just absent
	}
	return &OrphanedServer{SocketPath: socketPath, PIDs: pids}, nil
}
