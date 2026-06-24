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

// TmuxWorkerCounter counts active sessions in the AGM tmux socket as a proxy
// for concurrent workers. The count is used only when MaxWorkers > 0 (the
// default is 0 = disabled). Fails open (returns 0) when tmux is unavailable.
type TmuxWorkerCounter struct {
	// Socket is the AGM tmux socket path. Defaults to ~/.agm/agm.sock.
	Socket string
}

// CountWorkers returns the number of tmux sessions in the AGM socket.
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

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	count := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			count++
		}
	}
	return count, nil
}
