// Package circuitbreaker provides session-aware capacity providers for AGM.
package circuitbreaker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// --- LoadReader: /proc/loadavg ---

// ProcLoadReader reads the 5-minute load average from /proc/loadavg.
type ProcLoadReader struct{}

// Load5 reads the 5-minute load average from /proc/loadavg.
func (ProcLoadReader) Load5() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, fmt.Errorf("reading /proc/loadavg: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected /proc/loadavg format: %q", string(data))
	}
	return strconv.ParseFloat(fields[1], 64)
}

// --- SpawnTimer: ~/.agm/last-spawn.txt ---

const lastSpawnFile = "last-spawn.txt"

// FileSpawnTimer persists the last spawn timestamp in ~/.agm/last-spawn.txt.
type FileSpawnTimer struct {
	Dir string // directory to store last-spawn.txt (default: ~/.agm)
}

// NewFileSpawnTimer returns a FileSpawnTimer using the default AGM directory.
func NewFileSpawnTimer() FileSpawnTimer {
	dir := os.Getenv("AGM_CONFIG_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".agm")
	}
	return FileSpawnTimer{Dir: dir}
}

func (f FileSpawnTimer) path() string {
	return filepath.Join(f.Dir, lastSpawnFile)
}

// LastSpawnTime reads the last spawn timestamp from the file.
func (f FileSpawnTimer) LastSpawnTime() (time.Time, error) {
	data, err := os.ReadFile(f.path())
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
}

// RecordSpawn persists the spawn timestamp to the file.
func (f FileSpawnTimer) RecordSpawn(t time.Time) error {
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(f.path(), []byte(t.Format(time.RFC3339)+"\n"), 0o600)
}

// --- WorkerCounter: AGM tmux socket ---

// WorkerSessionPrefix is the tmux session-name prefix AGM gives to dispatched
// worker sessions (e.g. "worker-ce-2mib"). It is the cheap, DB-free signal for
// classifying a session on the shared socket as a worker.
const WorkerSessionPrefix = "worker-"

// TmuxWorkerCounter counts *worker* sessions in the AGM tmux socket.
//
// The socket is shared: test fixtures, supervisor panes (orchestrator,
// overseer, meta-orchestrator) and orphan sessions all live there alongside
// workers. Counting every session against the worker cap made the cap
// self-deadlocking — a single `go test` fixture session was enough to refuse
// every dispatch. A session is therefore counted only when it is recognisably
// a worker: its name carries WorkerSessionPrefix, or KnownWorkers reports it
// as tagged role:worker in the AGM session DB.
//
// The count is used only when MaxWorkers > 0 (the default is 0 = disabled).
// Fails open (returns 0) when tmux is unavailable.
type TmuxWorkerCounter struct {
	// Socket is the AGM tmux socket path. Defaults to ~/.agm/agm.sock.
	Socket string

	// KnownWorkers optionally resolves the set of session names AGM records as
	// active workers (status active, tagged role:worker). It is consulted only
	// for live sessions whose names lack WorkerSessionPrefix, so a fleet named
	// by convention costs no DB access. A nil resolver, one that errors, or one
	// that exceeds KnownWorkersTimeout leaves classification to the name prefix
	// alone.
	//
	// The set must be keyed by *tmux* session name, since that is what the
	// counter matches against. A session whose record name differs from its
	// tmux session name should appear under both.
	KnownWorkers func() (map[string]bool, error)

	// KnownWorkersTimeout bounds the KnownWorkers lookup. Zero selects
	// DefaultKnownWorkersTimeout. The resolver reads a database, and this
	// counter runs on the session-admission path, so a locked or overloaded
	// store must not be able to hang a spawn (see CBRK-02 for the same rule
	// applied to the memory probe).
	KnownWorkersTimeout time.Duration
}

// DefaultKnownWorkersTimeout bounds the session-DB lookup used to classify
// sessions whose names lack WorkerSessionPrefix.
const DefaultKnownWorkersTimeout = 5 * time.Second

// CountWorkers returns the number of worker sessions in the AGM socket.
func (t TmuxWorkerCounter) CountWorkers() (int, error) {
	sock := t.Socket
	if sock == "" {
		home, _ := os.UserHomeDir() // fail open: empty home → no socket path
		if home == "" {
			return 0, nil
		}
		sock = filepath.Join(home, ".agm", "agm.sock")
	}

	// If the socket doesn't exist, no workers are running.
	if _, err := os.Stat(sock); os.IsNotExist(err) {
		return 0, nil
	}

	// tmux exits non-zero when there are no sessions; treat any error as 0 workers.
	out, _ := exec.Command("tmux", "-S", sock, "list-sessions", "-F", "#S").Output()

	return countWorkerSessions(splitSessionNames(string(out)), t.boundedKnownWorkers()), nil
}

// boundedKnownWorkers wraps KnownWorkers so a slow session DB cannot hang
// session admission. On timeout it reports an error, which countWorkerSessions
// treats like any other resolver failure: prefix-only classification.
func (t TmuxWorkerCounter) boundedKnownWorkers() func() (map[string]bool, error) {
	if t.KnownWorkers == nil {
		return nil
	}
	timeout := t.KnownWorkersTimeout
	if timeout <= 0 {
		timeout = DefaultKnownWorkersTimeout
	}

	return func() (map[string]bool, error) {
		type result struct {
			workers map[string]bool
			err     error
		}
		// Buffered so the goroutine can always publish and exit, even after
		// this function has already given up on it.
		done := make(chan result, 1)
		go func() {
			w, err := t.KnownWorkers()
			done <- result{w, err}
		}()

		select {
		case r := <-done:
			return r.workers, r.err
		case <-time.After(timeout):
			return nil, fmt.Errorf("known-worker lookup exceeded %s", timeout)
		}
	}
}

// splitSessionNames turns `tmux list-sessions -F '#S'` output into session
// names, dropping blank lines.
func splitSessionNames(out string) []string {
	var names []string
	for l := range strings.SplitSeq(out, "\n") {
		if n := strings.TrimSpace(l); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// countWorkerSessions counts the names that denote worker sessions. Names
// carrying WorkerSessionPrefix count outright; the rest are checked against
// the AGM session DB via knownWorkers, which is resolved lazily and only when
// at least one unprefixed name needs adjudicating.
func countWorkerSessions(names []string, knownWorkers func() (map[string]bool, error)) int {
	count := 0
	var known map[string]bool
	resolved := false

	for _, name := range names {
		if strings.HasPrefix(name, WorkerSessionPrefix) {
			count++
			continue
		}
		if knownWorkers == nil {
			continue
		}
		if !resolved {
			// Best-effort: an unreadable session DB falls back to prefix-only
			// classification rather than resurrecting the count-everything bug.
			w, err := knownWorkers()
			if err == nil {
				known = w
			}
			resolved = true
		}
		if known[name] {
			count++
		}
	}
	return count
}
