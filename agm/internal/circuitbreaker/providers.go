// Package circuitbreaker provides session-aware capacity providers for AGM.
package circuitbreaker

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
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

// --- WorkerCounter: tmux sessions minus supervisors ---

// TmuxWorkerCounter counts active tmux sessions, excluding supervisors
// (orchestrator, meta-orchestrator, overseer).
type TmuxWorkerCounter struct{}

// CountWorkers returns the number of active non-supervisor tmux sessions.
func (TmuxWorkerCounter) CountWorkers() (int, error) {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return 0, fmt.Errorf("listing tmux sessions: %w", err)
	}

	count := 0
	for _, name := range sessions {
		if !tmux.IsSupervisorSession(name) {
			count++
		}
	}
	return count, nil
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

// --- AGMSessionCounter: role-tagged session counting ---

const workerTag = "role:worker"

// AGMSessionCounter counts active worker sessions by inspecting role tags.
// Only sessions with the "role:worker" tag are counted; human-created
// sessions (no role tag) and supervisor sessions are excluded.
type AGMSessionCounter struct{}

// CountWorkers returns the number of active sessions tagged with role:worker.
func (c *AGMSessionCounter) CountWorkers(sessions []ops.SessionSummary) int {
	count := 0
	for _, s := range sessions {
		if s.Status == "archived" {
			continue
		}
		if hasTag(s.Tags, workerTag) {
			count++
		}
	}
	return count
}

func hasTag(tags []string, target string) bool {
	for _, t := range tags {
		if t == target {
			return true
		}
	}
	return false
}
