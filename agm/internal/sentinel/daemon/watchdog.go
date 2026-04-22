// Package daemon provides background daemon monitoring.
package daemon

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// WatchdogConfig configures the self-watchdog process.
type WatchdogConfig struct {
	// HeartbeatPath is the path to the heartbeat JSON file to monitor.
	HeartbeatPath string
	// StaleThreshold is how old the heartbeat can be before considering it stale.
	// Default: 5 minutes.
	StaleThreshold time.Duration
	// CheckInterval is how often the watchdog checks the heartbeat.
	// Default: 30 seconds.
	CheckInterval time.Duration
	// IncidentsFile is the path to the incidents JSONL file for logging failures.
	IncidentsFile string
	// RestartCommand is the command used to restart the daemon.
	// Default: ["systemctl", "--user", "restart", "astrocyte"]
	RestartCommand []string
	// MaxRestartAttempts limits restart attempts before giving up.
	// Default: 3.
	MaxRestartAttempts int
	// Logger for structured logging.
	Logger *slog.Logger
}

// Watchdog monitors the astrocyte daemon's own heartbeat.
// If the heartbeat becomes stale, it attempts to restart the daemon
// and logs incidents on failure.
type Watchdog struct {
	config   WatchdogConfig
	stopChan chan struct{}
	mu       sync.Mutex
	running  bool

	// restartAttempts tracks consecutive restart failures.
	restartAttempts int

	// For testing: injectable command runner and time source.
	runCommand func(name string, args ...string) error
	nowFunc    func() time.Time
}

// NewWatchdog creates a watchdog with the given configuration.
func NewWatchdog(cfg WatchdogConfig) *Watchdog {
	if cfg.StaleThreshold == 0 {
		cfg.StaleThreshold = 5 * time.Minute
	}
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 30 * time.Second
	}
	if cfg.MaxRestartAttempts == 0 {
		cfg.MaxRestartAttempts = 3
	}
	if cfg.HeartbeatPath == "" {
		cfg.HeartbeatPath = DefaultHeartbeatPath()
	}
	if cfg.IncidentsFile == "" {
		homeDir, _ := os.UserHomeDir()
		cfg.IncidentsFile = filepath.Join(homeDir, ".agm/logs/astrocyte/incidents.jsonl")
	}
	if len(cfg.RestartCommand) == 0 {
		cfg.RestartCommand = []string{"systemctl", "--user", "restart", "astrocyte"}
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Watchdog{
		config:   cfg,
		stopChan: make(chan struct{}),
		runCommand: func(name string, args ...string) error {
			return exec.Command(name, args...).Run()
		},
		nowFunc: time.Now,
	}
}

// Start begins the watchdog monitoring loop. Blocks until Stop is called.
func (w *Watchdog) Start() error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return fmt.Errorf("watchdog is already running")
	}
	w.running = true
	w.mu.Unlock()

	w.config.Logger.Info("Watchdog started",
		"heartbeat_path", w.config.HeartbeatPath,
		"stale_threshold", w.config.StaleThreshold,
		"check_interval", w.config.CheckInterval)

	ticker := time.NewTicker(w.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.check()
		case <-w.stopChan:
			w.config.Logger.Info("Watchdog stopped")
			w.mu.Lock()
			w.running = false
			w.mu.Unlock()
			return nil
		}
	}
}

// Stop halts the watchdog loop.
func (w *Watchdog) Stop() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		close(w.stopChan)
	} else {
		w.mu.Unlock()
	}
}

// check performs a single heartbeat health check and takes action if stale.
func (w *Watchdog) check() {
	hb, err := CheckHealth(w.config.HeartbeatPath, w.config.StaleThreshold)
	if err == nil {
		// Healthy — reset restart counter.
		if w.restartAttempts > 0 {
			w.config.Logger.Info("Daemon heartbeat recovered", "pid", hb.PID)
			w.restartAttempts = 0
		}
		return
	}

	w.config.Logger.Warn("Daemon heartbeat unhealthy", "error", err)

	if w.restartAttempts >= w.config.MaxRestartAttempts {
		w.config.Logger.Error("Max restart attempts exhausted, logging incident",
			"attempts", w.restartAttempts)
		w.logIncident(hb, err, "max_restarts_exhausted")
		return
	}

	// Attempt restart.
	w.restartAttempts++
	w.config.Logger.Info("Attempting daemon restart",
		"attempt", w.restartAttempts,
		"max", w.config.MaxRestartAttempts)

	restartErr := w.runCommand(w.config.RestartCommand[0], w.config.RestartCommand[1:]...)
	if restartErr != nil {
		w.config.Logger.Error("Restart failed", "error", restartErr,
			"attempt", w.restartAttempts)
		w.logIncident(hb, restartErr, "restart_failed")
	} else {
		w.config.Logger.Info("Restart command succeeded", "attempt", w.restartAttempts)
	}
}

// WatchdogIncident records a watchdog-detected failure.
type WatchdogIncident struct {
	ID              string  `json:"id"`
	Timestamp       string  `json:"timestamp"`
	Symptom         string  `json:"symptom"`
	Reason          string  `json:"reason"`
	RestartAttempts int     `json:"restart_attempts"`
	MaxAttempts     int     `json:"max_attempts"`
	DaemonPID       int     `json:"daemon_pid,omitempty"`
	LastHeartbeat   *string `json:"last_heartbeat,omitempty"`
	Error           string  `json:"error"`
}

// logIncident writes a watchdog incident to the incidents JSONL file.
func (w *Watchdog) logIncident(hb *Heartbeat, checkErr error, reason string) {
	now := w.nowFunc()
	incident := WatchdogIncident{
		ID:              fmt.Sprintf("watchdog-%s", now.Format("20060102-150405")),
		Timestamp:       now.Format(time.RFC3339),
		Symptom:         "daemon_unhealthy",
		Reason:          reason,
		RestartAttempts: w.restartAttempts,
		MaxAttempts:     w.config.MaxRestartAttempts,
		Error:           checkErr.Error(),
	}

	if hb != nil {
		incident.DaemonPID = hb.PID
		ts := hb.Timestamp.Format(time.RFC3339)
		incident.LastHeartbeat = &ts
	}

	data, err := json.Marshal(incident)
	if err != nil {
		w.config.Logger.Error("Failed to marshal watchdog incident", "error", err)
		return
	}

	dir := filepath.Dir(w.config.IncidentsFile)
	if mkErr := os.MkdirAll(dir, 0750); mkErr != nil {
		w.config.Logger.Error("Failed to create incidents directory", "error", mkErr)
		return
	}

	f, err := os.OpenFile(w.config.IncidentsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		w.config.Logger.Error("Failed to open incidents file", "error", err)
		return
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		w.config.Logger.Error("Failed to write watchdog incident", "error", err)
		return
	}
	if _, err := f.WriteString("\n"); err != nil {
		w.config.Logger.Error("Failed to write newline", "error", err)
		return
	}
	_ = f.Sync()
}

// NotifySystemdWatchdog sends sd_notify WATCHDOG=1 to the systemd watchdog socket.
// This should be called periodically from the main daemon loop to indicate liveness.
// Returns nil if NOTIFY_SOCKET is not set (not running under systemd).
func NotifySystemdWatchdog() error {
	socketAddr := os.Getenv("NOTIFY_SOCKET")
	if socketAddr == "" {
		return nil
	}

	conn, err := net.Dial("unixgram", socketAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to notify socket: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("WATCHDOG=1"))
	if err != nil {
		return fmt.Errorf("failed to send watchdog notification: %w", err)
	}
	return nil
}

// NotifySystemdReady sends sd_notify READY=1 to indicate the daemon is ready.
// Returns nil if NOTIFY_SOCKET is not set.
func NotifySystemdReady() error {
	socketAddr := os.Getenv("NOTIFY_SOCKET")
	if socketAddr == "" {
		return nil
	}

	conn, err := net.Dial("unixgram", socketAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to notify socket: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("READY=1"))
	if err != nil {
		return fmt.Errorf("failed to send ready notification: %w", err)
	}
	return nil
}
