// Package daemon provides background daemon monitoring.
package daemon

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SessionHeartbeat represents a per-session heartbeat.
// Written to ~/.agm/heartbeats/{session}.json on every scan cycle.
type SessionHeartbeat struct {
	Timestamp   time.Time `json:"timestamp"`
	SessionName string    `json:"session_name"`
	ScanOK      bool      `json:"scan_ok"`
}

// SessionHeartbeatWriter writes per-session heartbeat files.
type SessionHeartbeatWriter struct {
	mu  sync.Mutex
	dir string
}

// NewSessionHeartbeatWriter creates a writer for the given directory.
// If dir is empty, defaults to ~/.agm/heartbeats/.
func NewSessionHeartbeatWriter(dir string) (*SessionHeartbeatWriter, error) {
	if dir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dir = filepath.Join(homeDir, ".agm/heartbeats")
	}

	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create heartbeats directory: %w", err)
	}

	return &SessionHeartbeatWriter{dir: dir}, nil
}

// Beat writes a heartbeat for the given session.
func (w *SessionHeartbeatWriter) Beat(sessionName string, scanOK bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	hb := SessionHeartbeat{
		Timestamp:   time.Now(),
		SessionName: sessionName,
		ScanOK:      scanOK,
	}

	data, err := json.Marshal(hb)
	if err != nil {
		return fmt.Errorf("failed to marshal session heartbeat: %w", err)
	}

	path := filepath.Join(w.dir, sessionName+".json")
	return os.WriteFile(path, data, 0600)
}

// SessionStalenessResult reports the staleness level of a session heartbeat.
type SessionStalenessResult struct {
	SessionName string
	Age         time.Duration
	Level       string // "ok", "warn", "alert"
}

// AlertState is the persisted dedup/circuit-breaker state for heartbeat alerts.
// Saved to ~/.agm/heartbeat-alerts.json so it survives daemon restarts.
type AlertState struct {
	AlertedSessions map[string]time.Time `json:"alerted_sessions"`
	// AlertTimestamps tracks when each alert was sent (for circuit breaker window).
	AlertTimestamps []time.Time `json:"alert_timestamps"`
	// CircuitBreakerTrips counts how many times the circuit breaker has fired.
	CircuitBreakerTrips int `json:"circuit_breaker_trips"`
	// CircuitBreakerUntil is when the current circuit breaker cooldown expires.
	CircuitBreakerUntil time.Time `json:"circuit_breaker_until,omitempty"`
	// Disabled is set when the monitor has been permanently shut down.
	Disabled bool `json:"disabled"`
}

// SessionHeartbeatMonitor checks heartbeat freshness and sends alerts.
type SessionHeartbeatMonitor struct {
	dir                 string
	warnAge             time.Duration
	alertAge            time.Duration
	alertedSessions     map[string]time.Time
	agmBinary           string
	orchestratorSession string
	logger              *slog.Logger
	mu                  sync.Mutex

	// Rate limiting / circuit breaker state.
	exemptSessions      []string
	persistPath         string
	alertTimestamps     []time.Time
	circuitBreakerTrips int
	circuitBreakerUntil time.Time
	disabled            bool
	maxAlertsPerCycle   int
	alertCooldown       time.Duration // per-session cooldown (1 hour)
	cbWindowSize        time.Duration // circuit breaker window (5 min)
	cbThreshold         int           // alerts in window to trigger CB (10)
	cbCooldown          time.Duration // circuit breaker pause (30 min)
	cbMaxTrips          int           // trips before permanent disable (3)

	// For testing: injectable command runner, time source, and tmux lister.
	runCommand       func(name string, args ...string) ([]byte, error)
	nowFunc          func() time.Time
	listTmuxSessions func() ([]string, error)
}

// NewSessionHeartbeatMonitor creates a monitor for the given heartbeats directory.
// If dir is empty, defaults to ~/.agm/heartbeats/.
func NewSessionHeartbeatMonitor(dir string, logger *slog.Logger) (*SessionHeartbeatMonitor, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	if dir == "" {
		dir = filepath.Join(homeDir, ".agm/heartbeats")
	}

	if logger == nil {
		logger = slog.Default()
	}

	persistPath := filepath.Join(homeDir, ".agm/heartbeat-alerts.json")

	m := &SessionHeartbeatMonitor{
		dir:                 dir,
		warnAge:             10 * time.Minute,
		alertAge:            30 * time.Minute,
		alertedSessions:     make(map[string]time.Time),
		agmBinary:           "agm",
		orchestratorSession: "orchestrator",
		logger:              logger,
		persistPath:         persistPath,
		maxAlertsPerCycle:   5,
		alertCooldown:       1 * time.Hour,
		cbWindowSize:        5 * time.Minute,
		cbThreshold:         10,
		cbCooldown:          30 * time.Minute,
		cbMaxTrips:          3,
		runCommand: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		},
		nowFunc: time.Now,
		listTmuxSessions: func() ([]string, error) {
			out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}").CombinedOutput()
			if err != nil {
				return nil, err
			}
			var sessions []string
			for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if line != "" {
					sessions = append(sessions, line)
				}
			}
			return sessions, nil
		},
	}

	// Load persisted state.
	m.loadAlertState()

	return m, nil
}

// SetExemptSessions sets session name prefixes to skip during monitoring.
func (m *SessionHeartbeatMonitor) SetExemptSessions(prefixes []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exemptSessions = prefixes
}

// isExempt checks if a session name matches any exempt prefix.
func (m *SessionHeartbeatMonitor) isExempt(sessionName string) bool {
	for _, prefix := range m.exemptSessions {
		if strings.HasPrefix(sessionName, prefix) {
			return true
		}
	}
	return false
}

// loadAlertState loads persisted alert state from disk.
func (m *SessionHeartbeatMonitor) loadAlertState() {
	data, err := os.ReadFile(m.persistPath)
	if err != nil {
		return // File doesn't exist yet — fresh start.
	}

	var state AlertState
	if err := json.Unmarshal(data, &state); err != nil {
		m.logger.Warn("Failed to parse heartbeat alert state, starting fresh", "error", err)
		return
	}

	if state.AlertedSessions != nil {
		m.alertedSessions = state.AlertedSessions
	}
	m.alertTimestamps = state.AlertTimestamps
	m.circuitBreakerTrips = state.CircuitBreakerTrips
	m.circuitBreakerUntil = state.CircuitBreakerUntil
	m.disabled = state.Disabled
}

// saveAlertState persists alert state to disk.
func (m *SessionHeartbeatMonitor) saveAlertState() {
	state := AlertState{
		AlertedSessions:     m.alertedSessions,
		AlertTimestamps:     m.alertTimestamps,
		CircuitBreakerTrips: m.circuitBreakerTrips,
		CircuitBreakerUntil: m.circuitBreakerUntil,
		Disabled:            m.disabled,
	}

	data, err := json.Marshal(state)
	if err != nil {
		m.logger.Error("Failed to marshal heartbeat alert state", "error", err)
		return
	}

	dir := filepath.Dir(m.persistPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		m.logger.Error("Failed to create alert state directory", "error", err)
		return
	}

	if err := os.WriteFile(m.persistPath, data, 0600); err != nil {
		m.logger.Error("Failed to write heartbeat alert state", "error", err)
	}
}

// CheckAll scans all heartbeat files and returns staleness results.
// Logs warnings for stale heartbeats (>10m) and sends alerts via agm for
// very stale heartbeats (>30m). Applies rate limiting, exempt session filtering,
// tmux existence checks, and circuit breaker logic.
func (m *SessionHeartbeatMonitor) CheckAll() []SessionStalenessResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.nowFunc()

	// Check if permanently disabled.
	if m.disabled {
		return nil
	}

	// Check circuit breaker cooldown.
	if !m.circuitBreakerUntil.IsZero() && now.Before(m.circuitBreakerUntil) {
		m.logger.Debug("Circuit breaker active, skipping heartbeat alerts",
			"until", m.circuitBreakerUntil)
		return nil
	}

	entries, err := filepath.Glob(filepath.Join(m.dir, "*.json"))
	if err != nil {
		m.logger.Error("Failed to glob heartbeat files", "error", err)
		return nil
	}

	// Build set of active tmux sessions for filtering.
	tmuxSet := m.activeTmuxSessions()

	var results []SessionStalenessResult
	alertsSentThisCycle := 0

	for _, path := range entries {
		data, err := os.ReadFile(path)
		if err != nil {
			m.logger.Error("Failed to read heartbeat file", "path", path, "error", err)
			continue
		}

		var hb SessionHeartbeat
		if err := json.Unmarshal(data, &hb); err != nil {
			m.logger.Error("Failed to parse heartbeat file", "path", path, "error", err)
			continue
		}

		age := now.Sub(hb.Timestamp)
		sessionName := hb.SessionName
		if sessionName == "" {
			// Derive from filename if not in JSON.
			base := filepath.Base(path)
			sessionName = strings.TrimSuffix(base, ".json")
		}

		result := SessionStalenessResult{
			SessionName: sessionName,
			Age:         age,
			Level:       "ok",
		}

		if age > m.alertAge {
			result.Level = "alert"

			// Skip exempt sessions.
			if m.isExempt(sessionName) {
				results = append(results, result)
				continue
			}

			// Skip sessions not present in tmux (stale heartbeat for deleted session).
			if tmuxSet != nil {
				if _, exists := tmuxSet[sessionName]; !exists {
					m.logger.Debug("Skipping alert for session not in tmux",
						"session", sessionName)
					results = append(results, result)
					continue
				}
			}

			// Rate limit: max alerts per cycle.
			if alertsSentThisCycle >= m.maxAlertsPerCycle {
				m.logger.Debug("Per-cycle alert limit reached, skipping",
					"session", sessionName, "limit", m.maxAlertsPerCycle)
				results = append(results, result)
				continue
			}

			// Rate limit: max 1 alert per session per hour.
			lastAlert, alerted := m.alertedSessions[sessionName]
			if alerted && now.Sub(lastAlert) < m.alertCooldown {
				m.logger.Debug("Per-session cooldown active, skipping alert",
					"session", sessionName,
					"last_alert", lastAlert,
					"cooldown", m.alertCooldown)
				results = append(results, result)
				continue
			}

			// Send alert.
			prompt := fmt.Sprintf("Session %s heartbeat stale: %s (threshold %s)",
				sessionName, age.Round(time.Second), m.alertAge)

			output, cmdErr := m.runCommand(m.agmBinary, "send", "msg",
				m.orchestratorSession,
				"--sender", "astrocyte",
				"--priority", "urgent",
				"--prompt", prompt)

			if cmdErr != nil {
				m.logger.Error("Failed to send staleness alert",
					"session", sessionName, "error", cmdErr, "output", string(output))
			} else {
				m.logger.Info("Sent staleness alert",
					"session", sessionName, "age", age.Round(time.Second))
			}

			m.alertedSessions[sessionName] = now
			m.alertTimestamps = append(m.alertTimestamps, now)
			alertsSentThisCycle++

			// Check circuit breaker after each alert.
			m.checkCircuitBreaker(now)
			if !m.circuitBreakerUntil.IsZero() && now.Before(m.circuitBreakerUntil) {
				break
			}
		} else if age > m.warnAge {
			result.Level = "warn"
			m.logger.Warn("Session heartbeat stale",
				"session", sessionName,
				"age", age.Round(time.Second),
				"threshold", m.warnAge)
		}

		results = append(results, result)
	}

	// Persist state after each scan.
	m.saveAlertState()

	return results
}

// activeTmuxSessions returns a set of active tmux session names, or nil if
// tmux is unavailable (in which case the filter is skipped).
func (m *SessionHeartbeatMonitor) activeTmuxSessions() map[string]struct{} {
	sessions, err := m.listTmuxSessions()
	if err != nil {
		m.logger.Debug("Could not list tmux sessions, skipping tmux filter", "error", err)
		return nil
	}
	set := make(map[string]struct{}, len(sessions))
	for _, s := range sessions {
		set[s] = struct{}{}
	}
	return set
}

// checkCircuitBreaker evaluates the circuit breaker and triggers if needed.
func (m *SessionHeartbeatMonitor) checkCircuitBreaker(now time.Time) {
	// Prune alert timestamps outside the window.
	cutoff := now.Add(-m.cbWindowSize)
	pruned := m.alertTimestamps[:0]
	for _, ts := range m.alertTimestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	m.alertTimestamps = pruned

	if len(m.alertTimestamps) > m.cbThreshold {
		m.circuitBreakerTrips++
		m.circuitBreakerUntil = now.Add(m.cbCooldown)
		m.logger.Warn("Circuit breaker triggered: excessive heartbeat alerts",
			"alerts_in_window", len(m.alertTimestamps),
			"trip_count", m.circuitBreakerTrips)

		if m.circuitBreakerTrips >= m.cbMaxTrips {
			m.disabled = true
			m.logger.Error("Heartbeat monitor disabled — manual restart required",
				"trip_count", m.circuitBreakerTrips)
		}

		m.saveAlertState()
	}
}

// IsDisabled returns whether the monitor has been permanently disabled by the circuit breaker.
func (m *SessionHeartbeatMonitor) IsDisabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disabled
}
