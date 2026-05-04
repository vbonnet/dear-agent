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

	"github.com/vbonnet/dear-agent/agm/internal/monitoring"
)

// LoopMonitorState is the persisted state for loop heartbeat wake attempts.
type LoopMonitorState struct {
	WakeAttempts map[string]*WakeAttemptState `json:"wake_attempts"` // keyed by session name
}

// WakeAttemptState tracks wake attempts for a single session.
type WakeAttemptState struct {
	Attempts      int       `json:"attempts"`
	LastAttempt   time.Time `json:"last_attempt"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	Escalated     bool      `json:"escalated"`
}

// LoopMonitor checks loop heartbeat freshness and triggers wakes.
type LoopMonitor struct {
	heartbeatDir string
	persistPath  string
	agmBinary    string
	logger       *slog.Logger
	mu           sync.Mutex

	// State
	state LoopMonitorState

	// Configuration
	maxAttempts int           // Max wake attempts before escalation (default: 3)
	cooldown    time.Duration // Cooldown between wake attempts (default: 2min)

	// Escalation
	escalationCommand string // Command to run after circuit breaker trips

	// Protection
	exemptSessions []string // session name prefixes to skip

	// For testing
	runCommand func(name string, args ...string) ([]byte, error)
	nowFunc    func() time.Time
}

// NewLoopMonitor creates a new loop heartbeat monitor.
func NewLoopMonitor(logger *slog.Logger) (*LoopMonitor, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	if logger == nil {
		logger = slog.Default()
	}

	m := &LoopMonitor{
		heartbeatDir: filepath.Join(homeDir, ".agm", "heartbeats"),
		persistPath:  filepath.Join(homeDir, ".agm", "loop-monitor-state.json"),
		agmBinary:    "agm",
		logger:       logger,
		state: LoopMonitorState{
			WakeAttempts: make(map[string]*WakeAttemptState),
		},
		maxAttempts: 3,
		cooldown:    2 * time.Minute,
		runCommand: func(name string, args ...string) ([]byte, error) {
			return exec.Command(name, args...).CombinedOutput()
		},
		nowFunc: time.Now,
	}

	m.loadState()
	return m, nil
}

// SetEscalationCommand sets the command to run when circuit breaker trips.
func (m *LoopMonitor) SetEscalationCommand(cmd string) {
	m.escalationCommand = cmd
}

// SetExemptSessions sets session name prefixes to skip during monitoring.
func (m *LoopMonitor) SetExemptSessions(prefixes []string) {
	m.exemptSessions = prefixes
}

// isExempt checks if a session name matches any exempt prefix.
func (m *LoopMonitor) isExempt(sessionName string) bool {
	for _, prefix := range m.exemptSessions {
		if strings.HasPrefix(sessionName, prefix) {
			return true
		}
	}
	return false
}

// CheckAll scans all loop heartbeat files and triggers wakes for stale ones.
func (m *LoopMonitor) CheckAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.nowFunc()

	heartbeats, err := monitoring.ListHeartbeats(m.heartbeatDir)
	if err != nil {
		m.logger.Error("Failed to list loop heartbeats", "error", err)
		return
	}

	for _, hb := range heartbeats {
		// Skip exempt sessions — these must never be woken or escalated.
		if m.isExempt(hb.Session) {
			continue
		}
		status := monitoring.CheckStaleness(hb)
		if status != "stale" {
			// Reset wake attempts if heartbeat is fresh (loop recovered)
			if ws, exists := m.state.WakeAttempts[hb.Session]; exists && ws.Attempts > 0 {
				m.logger.Info("Loop heartbeat recovered, resetting wake attempts",
					"session", hb.Session)
				delete(m.state.WakeAttempts, hb.Session)
			}
			continue
		}

		// Stale heartbeat — check wake state
		ws := m.state.WakeAttempts[hb.Session]
		if ws == nil {
			ws = &WakeAttemptState{
				LastHeartbeat: hb.Timestamp,
			}
			m.state.WakeAttempts[hb.Session] = ws
		}

		// Check cooldown
		if !ws.LastAttempt.IsZero() && now.Sub(ws.LastAttempt) < m.cooldown {
			m.logger.Debug("Wake cooldown active, skipping",
				"session", hb.Session,
				"last_attempt", ws.LastAttempt,
				"cooldown", m.cooldown)
			continue
		}

		// Check circuit breaker
		if ws.Attempts >= m.maxAttempts {
			if !ws.Escalated {
				m.logger.Warn("Circuit breaker tripped for loop monitor",
					"session", hb.Session,
					"attempts", ws.Attempts)
				m.escalate(hb.Session, ws)
				ws.Escalated = true
			}
			continue
		}

		// Trigger wake
		m.logger.Info("Loop heartbeat stale, sending wake",
			"session", hb.Session,
			"age", now.Sub(hb.Timestamp).Round(time.Second),
			"attempt", ws.Attempts+1)

		output, cmdErr := m.runCommand(m.agmBinary, "send", "wake-loop", hb.Session)
		if cmdErr != nil {
			m.logger.Error("Failed to send wake-loop",
				"session", hb.Session, "error", cmdErr, "output", string(output))
		}

		ws.Attempts++
		ws.LastAttempt = now
	}

	m.saveState()
}

// escalate runs the escalation command after circuit breaker trips.
func (m *LoopMonitor) escalate(session string, ws *WakeAttemptState) {
	// Log incident
	m.logIncident(session, ws)

	if m.escalationCommand == "" {
		m.logger.Warn("No escalation command configured",
			"session", session,
			"attempts", ws.Attempts)
		return
	}

	// Prepare JSON payload for stdin
	payload := map[string]interface{}{
		"session":        session,
		"attempts":       ws.Attempts,
		"last_heartbeat": ws.LastHeartbeat,
		"age":            time.Since(ws.LastHeartbeat).String(),
	}
	payloadJSON, _ := json.Marshal(payload)

	cmd := exec.Command("sh", "-c", m.escalationCommand)
	cmd.Stdin = bytesNewReader(payloadJSON)
	output, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Error("Escalation command failed",
			"session", session,
			"command", m.escalationCommand,
			"error", err,
			"output", string(output))
	} else {
		m.logger.Info("Escalation command executed",
			"session", session,
			"command", m.escalationCommand)
	}
}

// logIncident appends an incident to ~/.agm/astrocyte/incidents.jsonl.
func (m *LoopMonitor) logIncident(session string, ws *WakeAttemptState) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	incidentDir := filepath.Join(homeDir, ".agm", "astrocyte")
	if err := os.MkdirAll(incidentDir, 0750); err != nil {
		return
	}

	incident := map[string]interface{}{
		"type":           "loop_monitor_circuit_breaker",
		"session":        session,
		"attempts":       ws.Attempts,
		"last_heartbeat": ws.LastHeartbeat,
		"timestamp":      time.Now(),
	}

	data, err := json.Marshal(incident)
	if err != nil {
		return
	}

	f, err := os.OpenFile(
		filepath.Join(incidentDir, "incidents.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600,
	)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(data, '\n'))
}

func (m *LoopMonitor) loadState() {
	data, err := os.ReadFile(m.persistPath)
	if err != nil {
		return
	}

	var state LoopMonitorState
	if err := json.Unmarshal(data, &state); err != nil {
		m.logger.Warn("Failed to parse loop monitor state, starting fresh", "error", err)
		return
	}

	if state.WakeAttempts != nil {
		m.state = state
	}
}

func (m *LoopMonitor) saveState() {
	data, err := json.Marshal(m.state)
	if err != nil {
		m.logger.Error("Failed to marshal loop monitor state", "error", err)
		return
	}

	dir := filepath.Dir(m.persistPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		m.logger.Error("Failed to create loop monitor state directory", "error", err)
		return
	}

	if err := os.WriteFile(m.persistPath, data, 0600); err != nil {
		m.logger.Error("Failed to write loop monitor state", "error", err)
	}
}

// bytesNewReader is a helper to avoid importing bytes package name collision.
// Returns an *os.File-like reader for use with exec.Command.Stdin.
func bytesNewReader(data []byte) *os.File {
	r, w, err := os.Pipe()
	if err != nil {
		return nil
	}
	go func() {
		w.Write(data)
		w.Close()
	}()
	return r
}
