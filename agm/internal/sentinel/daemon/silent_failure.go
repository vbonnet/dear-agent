// Package daemon provides background daemon monitoring.
package daemon

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Alert represents a silent failure detection alert.
type Alert struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`     // "worker_health", "fork_bomb", "stale_session"
	Severity  string            `json:"severity"` // "warning", "critical"
	Message   string            `json:"message"`
	Timestamp time.Time         `json:"timestamp"`
	Details   map[string]string `json:"details,omitempty"`
}

// AlertFunc is a callback invoked when a silent failure is detected.
type AlertFunc func(Alert)

// SilentFailureConfig configures the silent failure detector.
type SilentFailureConfig struct {
	// HeartbeatPath is the path to the daemon heartbeat file.
	HeartbeatPath string
	// MaxHeartbeatAge is the maximum heartbeat age before alerting.
	// Default: 5 minutes.
	MaxHeartbeatAge time.Duration
	// MaxChildProcesses is the threshold for fork bomb detection.
	// Default: 50.
	MaxChildProcesses int
	// ForkBombCheckInterval is how often fork bomb checks run.
	// Default: 30 seconds (ensures detection within 60s).
	ForkBombCheckInterval time.Duration
	// StaleScanThreshold is the number of consecutive stale scans before alerting.
	// Default: 2.
	StaleScanThreshold int
	// AlertFunc is called when a silent failure is detected.
	AlertFunc AlertFunc
	// IncidentsFile is the path for logging silent failure incidents.
	IncidentsFile string
	// Logger for structured logging.
	Logger *slog.Logger
}

// SilentFailureDetector detects and alerts on silent worker failures.
// It monitors worker heartbeats, detects fork bombs, and identifies
// stale sessions that have stopped making progress.
type SilentFailureDetector struct {
	config SilentFailureConfig

	// Stale session tracking: maps session name to consecutive stale scan count.
	sessionScanCounts map[string]int
	// Last known session content lengths for staleness detection.
	sessionLastLengths map[string]int

	mu       sync.Mutex
	stopChan chan struct{}
	running  bool

	// For testing: injectable process lister, command runner, and time source.
	listChildPIDs func(pid int) ([]int, error)
	killProcess   func(pid int) error
	nowFunc       func() time.Time
}

// NewSilentFailureDetector creates a detector with the given configuration.
func NewSilentFailureDetector(cfg SilentFailureConfig) *SilentFailureDetector {
	if cfg.MaxHeartbeatAge == 0 {
		cfg.MaxHeartbeatAge = 5 * time.Minute
	}
	if cfg.MaxChildProcesses == 0 {
		cfg.MaxChildProcesses = 50
	}
	if cfg.ForkBombCheckInterval == 0 {
		cfg.ForkBombCheckInterval = 30 * time.Second
	}
	if cfg.StaleScanThreshold == 0 {
		cfg.StaleScanThreshold = 2
	}
	if cfg.HeartbeatPath == "" {
		cfg.HeartbeatPath = DefaultHeartbeatPath()
	}
	if cfg.IncidentsFile == "" {
		homeDir, _ := os.UserHomeDir()
		cfg.IncidentsFile = filepath.Join(homeDir, ".agm/logs/astrocyte/silent-failure-incidents.jsonl")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &SilentFailureDetector{
		config:             cfg,
		sessionScanCounts:  make(map[string]int),
		sessionLastLengths: make(map[string]int),
		stopChan:           make(chan struct{}),
		listChildPIDs:      defaultListChildPIDs,
		killProcess:        defaultKillProcess,
		nowFunc:            time.Now,
	}
}

// Start begins the silent failure detection loop. Blocks until Stop is called.
func (d *SilentFailureDetector) Start() error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return fmt.Errorf("silent failure detector is already running")
	}
	d.running = true
	d.mu.Unlock()

	d.config.Logger.Info("Silent failure detector started",
		"heartbeat_path", d.config.HeartbeatPath,
		"max_heartbeat_age", d.config.MaxHeartbeatAge,
		"fork_bomb_interval", d.config.ForkBombCheckInterval,
		"stale_scan_threshold", d.config.StaleScanThreshold)

	ticker := time.NewTicker(d.config.ForkBombCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.CheckWorkerHealth()
			d.CheckForkBomb(0) // 0 = current process
		case <-d.stopChan:
			d.config.Logger.Info("Silent failure detector stopped")
			d.mu.Lock()
			d.running = false
			d.mu.Unlock()
			return nil
		}
	}
}

// Stop halts the detection loop.
func (d *SilentFailureDetector) Stop() {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		close(d.stopChan)
	} else {
		d.mu.Unlock()
	}
}

// CheckWorkerHealth checks the worker heartbeat and alerts if stale.
// Returns nil if healthy, error describing the failure otherwise.
func (d *SilentFailureDetector) CheckWorkerHealth() error {
	hb, err := CheckHealth(d.config.HeartbeatPath, d.config.MaxHeartbeatAge)
	if err == nil {
		return nil
	}

	now := d.nowFunc()
	alert := Alert{
		ID:        fmt.Sprintf("worker-health-%s", now.Format("20060102-150405")),
		Type:      "worker_health",
		Severity:  "critical",
		Message:   fmt.Sprintf("Worker heartbeat unhealthy: %s", err),
		Timestamp: now,
		Details:   make(map[string]string),
	}

	if hb != nil {
		alert.Details["pid"] = strconv.Itoa(hb.PID)
		alert.Details["last_heartbeat"] = hb.Timestamp.Format(time.RFC3339)
		alert.Details["session_count"] = strconv.Itoa(hb.SessionCount)
	}

	d.config.Logger.Warn("Worker health check failed",
		"error", err,
		"heartbeat_path", d.config.HeartbeatPath)

	d.emitAlert(alert)
	d.logIncident(alert)

	return err
}

// CheckForkBomb checks if a process has spawned an excessive number of children,
// indicating a fork bomb. If detected, kills the offending child processes.
// Pass pid=0 to check the current process.
func (d *SilentFailureDetector) CheckForkBomb(pid int) (bool, error) {
	if pid == 0 {
		pid = os.Getpid()
	}

	children, err := d.listChildPIDs(pid)
	if err != nil {
		return false, fmt.Errorf("failed to list child processes for pid %d: %w", pid, err)
	}

	if len(children) <= d.config.MaxChildProcesses {
		return false, nil
	}

	now := d.nowFunc()
	alert := Alert{
		ID:        fmt.Sprintf("fork-bomb-%s", now.Format("20060102-150405")),
		Type:      "fork_bomb",
		Severity:  "critical",
		Message:   fmt.Sprintf("Fork bomb detected: pid %d has %d children (threshold %d)", pid, len(children), d.config.MaxChildProcesses),
		Timestamp: now,
		Details: map[string]string{
			"parent_pid":   strconv.Itoa(pid),
			"child_count":  strconv.Itoa(len(children)),
			"threshold":    strconv.Itoa(d.config.MaxChildProcesses),
			"killed_count": "0",
		},
	}

	d.config.Logger.Error("Fork bomb detected",
		"parent_pid", pid,
		"child_count", len(children),
		"threshold", d.config.MaxChildProcesses)

	// Kill excess child processes (newest first).
	killed := 0
	for i := len(children) - 1; i >= 0; i-- {
		if err := d.killProcess(children[i]); err != nil {
			d.config.Logger.Error("Failed to kill fork bomb child",
				"child_pid", children[i], "error", err)
		} else {
			killed++
		}
	}

	alert.Details["killed_count"] = strconv.Itoa(killed)
	d.emitAlert(alert)
	d.logIncident(alert)

	return true, nil
}

// CheckStaleSessions checks sessions for staleness based on content length changes.
// A session is considered stale if its content length hasn't changed for
// StaleScanThreshold consecutive scans.
// Returns the list of sessions detected as stale.
func (d *SilentFailureDetector) CheckStaleSessions(sessions map[string]int) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.nowFunc()
	var stale []string

	for name, contentLen := range sessions {
		lastLen, exists := d.sessionLastLengths[name]

		if !exists || contentLen != lastLen {
			// Session is making progress — reset counter.
			d.sessionScanCounts[name] = 0
			d.sessionLastLengths[name] = contentLen
			continue
		}

		// Content unchanged — increment stale counter.
		d.sessionScanCounts[name]++
		d.config.Logger.Debug("Session content unchanged",
			"session", name,
			"stale_scans", d.sessionScanCounts[name],
			"threshold", d.config.StaleScanThreshold)

		if d.sessionScanCounts[name] >= d.config.StaleScanThreshold {
			stale = append(stale, name)

			alert := Alert{
				ID:        fmt.Sprintf("stale-session-%s-%s", name, now.Format("20060102-150405")),
				Type:      "stale_session",
				Severity:  "warning",
				Message:   fmt.Sprintf("Session %s stale: no progress for %d consecutive scans", name, d.sessionScanCounts[name]),
				Timestamp: now,
				Details: map[string]string{
					"session":        name,
					"stale_scans":    strconv.Itoa(d.sessionScanCounts[name]),
					"threshold":      strconv.Itoa(d.config.StaleScanThreshold),
					"content_length": strconv.Itoa(contentLen),
				},
			}

			d.config.Logger.Warn("Stale session detected",
				"session", name,
				"stale_scans", d.sessionScanCounts[name],
				"content_length", contentLen)

			d.emitAlert(alert)
			d.logIncident(alert)

			// Reset after alerting to avoid repeated alerts every cycle.
			d.sessionScanCounts[name] = 0
		}
	}

	// Clean up sessions that no longer exist.
	for name := range d.sessionLastLengths {
		if _, exists := sessions[name]; !exists {
			delete(d.sessionLastLengths, name)
			delete(d.sessionScanCounts, name)
		}
	}

	return stale
}

// ClearSession removes tracking state for a session.
func (d *SilentFailureDetector) ClearSession(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.sessionScanCounts, name)
	delete(d.sessionLastLengths, name)
}

// emitAlert calls the configured alert function if set.
func (d *SilentFailureDetector) emitAlert(alert Alert) {
	if d.config.AlertFunc != nil {
		d.config.AlertFunc(alert)
	}
}

// logIncident writes an alert to the incidents JSONL file.
func (d *SilentFailureDetector) logIncident(alert Alert) {
	data, err := json.Marshal(alert)
	if err != nil {
		d.config.Logger.Error("Failed to marshal silent failure incident", "error", err)
		return
	}

	dir := filepath.Dir(d.config.IncidentsFile)
	if mkErr := os.MkdirAll(dir, 0750); mkErr != nil {
		d.config.Logger.Error("Failed to create incidents directory", "error", mkErr)
		return
	}

	f, err := os.OpenFile(d.config.IncidentsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		d.config.Logger.Error("Failed to open incidents file", "error", err)
		return
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		d.config.Logger.Error("Failed to write silent failure incident", "error", err)
		return
	}
	if _, err := f.WriteString("\n"); err != nil {
		d.config.Logger.Error("Failed to write newline", "error", err)
		return
	}
	_ = f.Sync()
}

// defaultListChildPIDs reads /proc to find child PIDs of the given parent.
func defaultListChildPIDs(parentPID int) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc: %w", err)
	}

	var children []int
	parentStr := strconv.Itoa(parentPID)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // Not a PID directory.
		}

		statusPath := filepath.Join("/proc", entry.Name(), "status")
		data, err := os.ReadFile(statusPath)
		if err != nil {
			continue // Process may have exited.
		}

		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PPid:\t") {
				ppid := strings.TrimPrefix(line, "PPid:\t")
				ppid = strings.TrimSpace(ppid)
				if ppid == parentStr {
					children = append(children, pid)
				}
				break
			}
		}
	}

	return children, nil
}

// defaultKillProcess sends SIGKILL to the given PID.
func defaultKillProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}
	return proc.Kill()
}
