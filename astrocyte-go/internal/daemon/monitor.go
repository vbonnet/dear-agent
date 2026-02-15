package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vbonnet/ai-tools/astrocyte/internal/config"
	"github.com/vbonnet/ai-tools/astrocyte/internal/tmux"
	"github.com/vbonnet/ai-tools/astrocyte/pkg/enforcement"
)

// MonitorConfig contains configuration for session monitoring.
type MonitorConfig struct {
	CheckInterval       time.Duration     // How often to check sessions
	StuckTimeout        time.Duration     // Minimum duration before considering stuck
	RecoveryStrategy    RecoveryStrategy  // Default recovery strategy
	MaxRecoveryAttempts int               // Max recovery attempts per session
}

// SessionMonitor orchestrates session monitoring, detection, and recovery.
// Coordinates between tmux client, violation detector, and recovery system.
type SessionMonitor struct {
	// Dependencies
	tmuxClient    *tmux.Client
	detector      *StuckSessionDetector
	bashDetector  *enforcement.ViolationDetector
	beadsDetector *enforcement.ViolationDetector
	gitDetector   *enforcement.ViolationDetector

	// Configuration
	config *config.Config

	// State
	recoveryHistories map[string]*RecoveryHistory
	incidentLogger    *IncidentLogger
	running           bool
	stopChan          chan struct{}
	mu                sync.Mutex // Protects running field
}

// NewSessionMonitor creates a new session monitor with given configuration.
func NewSessionMonitor(cfg *config.Config) (*SessionMonitor, error) {
	// Create tmux client
	tmuxClient := tmux.NewClient()

	// Create stuck session detector
	detector := NewStuckSessionDetector()

	// Load pattern databases for violation detection
	bashPatterns, err := enforcement.LoadPatterns(cfg.Patterns.Bash)
	if err != nil {
		return nil, fmt.Errorf("failed to load bash patterns: %w", err)
	}

	beadsPatterns, err := enforcement.LoadPatterns(cfg.Patterns.Beads)
	if err != nil {
		return nil, fmt.Errorf("failed to load beads patterns: %w", err)
	}

	gitPatterns, err := enforcement.LoadPatterns(cfg.Patterns.Git)
	if err != nil {
		return nil, fmt.Errorf("failed to load git patterns: %w", err)
	}

	// Create violation detectors
	bashDetector, err := enforcement.NewDetector(bashPatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to create bash detector: %w", err)
	}

	beadsDetector, err := enforcement.NewDetector(beadsPatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to create beads detector: %w", err)
	}

	gitDetector, err := enforcement.NewDetector(gitPatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to create git detector: %w", err)
	}

	// Create incident logger
	incidentLogger, err := NewIncidentLogger(cfg.Logging.IncidentsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create incident logger: %w", err)
	}

	return &SessionMonitor{
		tmuxClient:        tmuxClient,
		detector:          detector,
		bashDetector:      bashDetector,
		beadsDetector:     beadsDetector,
		gitDetector:       gitDetector,
		config:            cfg,
		recoveryHistories: make(map[string]*RecoveryHistory),
		incidentLogger:    incidentLogger,
		stopChan:          make(chan struct{}),
	}, nil
}

// StartMonitoring begins the main daemon monitoring loop.
// Checks all tmux sessions at configured interval for stuck indicators.
// Blocks until StopMonitoring is called.
func (m *SessionMonitor) StartMonitoring() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("monitor is already running")
	}
	m.running = true
	m.mu.Unlock()

	log.Printf("Starting Astrocyte session monitor (interval: %v, stuck threshold: %v)",
		m.config.Monitoring.IntervalDuration,
		m.config.Monitoring.StuckThresholdDuration)

	ticker := time.NewTicker(m.config.Monitoring.IntervalDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check all sessions for stuck state
			if err := m.CheckAllSessions(); err != nil {
				log.Printf("Error checking sessions: %v", err)
			}

		case <-m.stopChan:
			log.Printf("Stopping session monitor")
			m.mu.Lock()
			m.running = false
			m.mu.Unlock()
			return nil
		}
	}
}

// StopMonitoring stops the monitoring loop gracefully.
func (m *SessionMonitor) StopMonitoring() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		close(m.stopChan)
	} else {
		m.mu.Unlock()
	}
}

// CheckAllSessions scans all tmux sessions for stuck indicators.
// For each stuck session, attempts detection and recovery.
func (m *SessionMonitor) CheckAllSessions() error {
	// List all tmux sessions
	sessions, err := m.tmuxClient.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	if m.config.Logging.Verbose {
		log.Printf("Checking %d tmux sessions", len(sessions))
	}

	// Check each session
	for _, sessionName := range sessions {
		// Check if we should stop (non-blocking check)
		select {
		case <-m.stopChan:
			// Stop signal received, abort checking remaining sessions
			return nil
		default:
			// Continue processing
		}

		if err := m.checkSession(sessionName); err != nil {
			log.Printf("Error checking session %s: %v", sessionName, err)
			// Continue to next session even if one fails
		}
	}

	return nil
}

// checkSession checks a single session for stuck state and performs recovery if needed.
func (m *SessionMonitor) checkSession(sessionName string) error {
	// Get pane information
	paneInfo, err := m.tmuxClient.GetPaneInfo(sessionName)
	if err != nil {
		return fmt.Errorf("failed to get pane info: %w", err)
	}

	// Track cursor position for freeze detection
	m.detector.TrackSession(sessionName, paneInfo.CursorX, paneInfo.CursorY)

	// Check if session is stuck
	stuckInfo := m.detector.DetectStuckSession(paneInfo)
	if stuckInfo == nil {
		// Session is not stuck, nothing to do
		return nil
	}

	if m.config.Logging.Verbose {
		log.Printf("Stuck session detected: %s", stuckInfo.String())
	}

	// Attempt recovery
	return m.RecoverSession(sessionName, stuckInfo, paneInfo)
}

// RecoverSession handles recovery for a stuck session.
// Detects violation pattern, sends rejection message, logs incident, and files violation.
func (m *SessionMonitor) RecoverSession(sessionName string, stuckInfo *SessionStuckInfo, paneInfo *tmux.PaneInfo) error {
	// Check recovery history (circuit breaker)
	history, exists := m.recoveryHistories[sessionName]
	if !exists {
		history = NewRecoveryHistory(sessionName, m.config.Recovery.MaxAttempts)
		m.recoveryHistories[sessionName] = history
	}

	if !history.CanAttemptRecovery() {
		log.Printf("Session %s: max recovery attempts reached (%d), skipping recovery",
			sessionName, m.config.Recovery.MaxAttempts)
		return nil
	}

	// Extract last command from pane content
	command := paneInfo.ExtractLastCommand()
	if command == "" {
		command = stuckInfo.LastCommand
	}

	// Detect violation pattern
	pattern, err := m.detectViolationPattern(command, paneInfo.Content)
	if err != nil {
		log.Printf("Failed to detect violation pattern: %v", err)
		// Continue with recovery even if pattern detection fails
	}

	var rejectionMessage string
	if pattern != nil {
		// Generate rejection message
		rejectionMessage = enforcement.GenerateRejectionMessage(pattern, command)

		// Send rejection message to session
		if err := SendRejectionMessage(sessionName, rejectionMessage, pattern); err != nil {
			log.Printf("Failed to send rejection message: %v", err)
		}

		// File violation
		violationData := enforcement.ViolationData{
			PatternID:   pattern.ID,
			PatternType: m.detectPatternType(pattern),
			Command:     command,
			SessionID:   sessionName,
			AgentType:   "general-purpose",
			Timestamp:   time.Now(),
		}

		if _, err := enforcement.FileViolation(violationData, m.config.Violations.Directory, pattern); err != nil {
			log.Printf("Failed to file violation: %v", err)
		}
	}

	// Parse recovery strategy
	strategy, err := ParseStrategy(m.config.Recovery.Strategy)
	if err != nil {
		log.Printf("Invalid recovery strategy, using escape: %v", err)
		strategy = RecoveryEscape
	}

	// Apply recovery if enabled
	var recoveryResult *RecoveryResult
	if m.config.Recovery.Enabled {
		recoveryResult, err = ApplyRecovery(sessionName, strategy, m.tmuxClient)
		if err != nil {
			log.Printf("Recovery failed for session %s: %v", sessionName, err)
		}

		// Record recovery attempt
		history.RecordAttempt(strategy, recoveryResult != nil && recoveryResult.Success, stuckInfo.Reason)
	}

	// Log incident
	incident := &Incident{
		ID:              fmt.Sprintf("astrocyte-%s", time.Now().Format("20060102-150405")),
		Timestamp:       time.Now().Format(time.RFC3339),
		SessionID:       sessionName,
		Symptom:         stuckInfo.Reason,
		DurationMinutes: int(m.config.Monitoring.StuckThresholdDuration.Minutes()),
		PaneSnapshot:    truncateContent(paneInfo.Content, 500),
		CursorPosition:  fmt.Sprintf("%d,%d", paneInfo.CursorX, paneInfo.CursorY),
		RecoveryMethod:  strategy.String(),
	}

	if pattern != nil {
		incident.PatternID = pattern.ID
		incident.Severity = pattern.Severity
		incident.Command = command
	}

	if recoveryResult != nil {
		incident.RecoverySuccess = recoveryResult.Success
		incident.RecoveryDurationMs = recoveryResult.DurationMs
	}

	if err := m.incidentLogger.LogIncident(incident); err != nil {
		log.Printf("Failed to log incident: %v", err)
	}

	// Write diagnosis file if enabled
	if m.config.Logging.DiagnosesDir != "" {
		if err := m.writeDiagnosis(sessionName, incident, pattern, rejectionMessage); err != nil {
			log.Printf("Failed to write diagnosis: %v", err)
		}
	}

	return nil
}

// detectViolationPattern attempts to detect a violation pattern in command/content.
// Tries bash, beads, and git detectors in sequence.
func (m *SessionMonitor) detectViolationPattern(command, content string) (*enforcement.Pattern, error) {
	// Try bash patterns first (most common)
	if pattern, err := m.bashDetector.Detect(command); err == nil && pattern != nil {
		return pattern, nil
	}

	// Try beads patterns
	if pattern, err := m.beadsDetector.Detect(command); err == nil && pattern != nil {
		return pattern, nil
	}

	// Try git patterns
	if pattern, err := m.gitDetector.Detect(command); err == nil && pattern != nil {
		return pattern, nil
	}

	// Also check content for patterns (some violations appear in output, not command)
	if pattern, err := m.bashDetector.Detect(content); err == nil && pattern != nil {
		return pattern, nil
	}

	return nil, nil
}

// detectPatternType determines pattern type (bash/beads/git) from pattern.
func (m *SessionMonitor) detectPatternType(pattern *enforcement.Pattern) string {
	// Try to match against loaded pattern databases
	// This is a simple heuristic - in production, pattern should include type field
	if m.bashDetector != nil {
		if p, _ := m.bashDetector.Detect(""); p != nil && p.ID == pattern.ID {
			return "bash"
		}
	}
	if m.beadsDetector != nil {
		if p, _ := m.beadsDetector.Detect(""); p != nil && p.ID == pattern.ID {
			return "beads"
		}
	}
	if m.gitDetector != nil {
		if p, _ := m.gitDetector.Detect(""); p != nil && p.ID == pattern.ID {
			return "git"
		}
	}
	return "bash" // Default to bash
}

// writeDiagnosis creates a diagnosis markdown file for the incident.
// File format matches Python Astrocyte output (YAML frontmatter + markdown body).
func (m *SessionMonitor) writeDiagnosis(sessionName string, incident *Incident, pattern *enforcement.Pattern, rejectionMessage string) error {
	// Create diagnoses directory if needed
	if err := os.MkdirAll(m.config.Logging.DiagnosesDir, 0755); err != nil {
		return fmt.Errorf("failed to create diagnoses directory: %w", err)
	}

	// Generate filename: diagnosis-{session_id}.md
	filename := fmt.Sprintf("diagnosis-%s.md", sessionName)
	filePath := filepath.Join(m.config.Logging.DiagnosesDir, filename)

	// Build diagnosis content
	content := fmt.Sprintf(`---
session_id: %s
symptom: %s
timestamp: %s
cursor_position: %s
recovery_method: %s
recovery_success: %v
`, sessionName, incident.Symptom, incident.Timestamp, incident.CursorPosition,
		incident.RecoveryMethod, incident.RecoverySuccess)

	if pattern != nil {
		content += fmt.Sprintf(`pattern_id: %s
severity: %s
`, pattern.ID, pattern.Severity)
	}

	content += "---\n\n"

	// Markdown body
	content += fmt.Sprintf("# Diagnosis: %s\n\n", sessionName)
	content += fmt.Sprintf("**Detected**: %s\n", incident.Timestamp)
	content += fmt.Sprintf("**Symptom**: %s\n", incident.Symptom)
	content += fmt.Sprintf("**Cursor**: %s\n\n", incident.CursorPosition)

	if incident.Command != "" {
		content += "## Command\n\n"
		content += fmt.Sprintf("```\n%s\n```\n\n", incident.Command)
	}

	if pattern != nil {
		content += "## Violation Pattern\n\n"
		content += fmt.Sprintf("**Pattern ID**: %s\n", pattern.ID)
		content += fmt.Sprintf("**Reason**: %s\n", pattern.Reason)
		content += fmt.Sprintf("**Alternative**: %s\n\n", pattern.Alternative)
	}

	content += "## Recovery\n\n"
	content += fmt.Sprintf("**Method**: %s\n", incident.RecoveryMethod)
	content += fmt.Sprintf("**Success**: %v\n", incident.RecoverySuccess)
	content += fmt.Sprintf("**Duration**: %d ms\n\n", incident.RecoveryDurationMs)

	if rejectionMessage != "" {
		content += "## Rejection Message\n\n"
		content += fmt.Sprintf("```\n%s\n```\n", rejectionMessage)
	}

	// Write file
	return os.WriteFile(filePath, []byte(content), 0644)
}

// truncateContent truncates content to maxChars, preserving last N lines.
func truncateContent(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[len(content)-maxChars:]
}

// Incident represents a stuck session incident for logging.
type Incident struct {
	ID                 string  `json:"id"`
	Timestamp          string  `json:"timestamp"`
	SessionID          string  `json:"session_id"`
	PatternID          string  `json:"pattern_id,omitempty"`
	Severity           string  `json:"severity,omitempty"`
	Command            string  `json:"command,omitempty"`
	Symptom            string  `json:"symptom"`
	DurationMinutes    int     `json:"duration_minutes"`
	PaneSnapshot       string  `json:"pane_snapshot,omitempty"`
	CursorPosition     string  `json:"cursor_position"`
	RecoveryMethod     string  `json:"recovery_method"`
	RecoverySuccess    bool    `json:"recovery_success"`
	RecoveryDurationMs int64   `json:"recovery_duration_ms"`
}

// IncidentLogger logs incidents to incidents.jsonl.
type IncidentLogger struct {
	filePath string
}

// NewIncidentLogger creates a new incident logger.
func NewIncidentLogger(filePath string) (*IncidentLogger, error) {
	// Create directory if needed
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create incident log directory: %w", err)
	}

	return &IncidentLogger{filePath: filePath}, nil
}

// LogIncident appends an incident to the JSONL file.
func (l *IncidentLogger) LogIncident(incident *Incident) error {
	// Serialize to JSON
	data, err := json.Marshal(incident)
	if err != nil {
		return fmt.Errorf("failed to marshal incident: %w", err)
	}

	// Append to file
	f, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open incidents file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write incident: %w", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return nil
}
