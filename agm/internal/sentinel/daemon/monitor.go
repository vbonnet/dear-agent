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

	"github.com/vbonnet/dear-agent/agm/internal/sentinel/config"
	"github.com/vbonnet/dear-agent/agm/internal/sentinel/logging"
	"github.com/vbonnet/dear-agent/agm/internal/sentinel/tmux"
	"github.com/vbonnet/dear-agent/pkg/enforcement"
)

// MonitorConfig contains configuration for session monitoring.
type MonitorConfig struct {
	CheckInterval       time.Duration    // How often to check sessions
	StuckTimeout        time.Duration    // Minimum duration before considering stuck
	RecoveryStrategy    RecoveryStrategy // Default recovery strategy
	MaxRecoveryAttempts int              // Max recovery attempts per session
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
	recoveryHistories       map[string]*RecoveryHistory
	incidentLogger          *IncidentLogger
	dedup                   *IncidentDeduplicator
	escalation              *EscalationPipeline
	metrics                 *MetricsCollector
	accumulator             *PatternAccumulator
	streamWriter            *StreamErrorWriter
	frictionAcc             *PatternAccumulator
	frictionWriter          *StreamFrictionWriter
	heartbeatWriter         *HeartbeatWriter
	sessionHeartbeatWriter  *SessionHeartbeatWriter
	sessionHeartbeatMonitor *SessionHeartbeatMonitor
	loopMonitor             *LoopMonitor
	logger                  *slog.Logger // Structured logger
	running                 bool
	stopChan                chan struct{}
	mu                      sync.Mutex // Protects running field

	// CrossSessionThreshold is the number of distinct sessions that must
	// hit the same pattern before a work item is emitted. Default: 3.
	CrossSessionThreshold int
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

	// Create incident deduplicator
	dedupCooldown := 15 * time.Minute
	if cfg.Recovery.DedupCooldownDuration > 0 {
		dedupCooldown = cfg.Recovery.DedupCooldownDuration
	}

	// Create structured logger
	logger := logging.DefaultLogger()

	// Create escalation pipeline
	escalationExecutor := &ExecCommandExecutor{}
	escalationLogger := logging.DefaultLogger()
	agmBinary := "agm"
	maxPerHour := 5
	if cfg.Escalation.AgmBinary != "" {
		agmBinary = cfg.Escalation.AgmBinary
	}
	if cfg.Escalation.MaxAutoApprovalsHour > 0 {
		maxPerHour = cfg.Escalation.MaxAutoApprovalsHour
	}

	// Create metrics collector
	homeDir, _ := os.UserHomeDir()
	metricsFile := filepath.Join(homeDir, ".agm/logs/astrocyte/metrics.json")

	// Create pattern accumulator (30-minute sliding window)
	accumulator := NewPatternAccumulator(30 * time.Minute)

	// Create stream error writer
	streamWriter, err := NewStreamErrorWriter("")
	if err != nil {
		// Non-fatal: log and continue without stream error writing
		logger.Warn("Failed to create stream error writer, cross-session work items disabled", "error", err)
	}

	// Create friction accumulator and writer (30-minute sliding window)
	frictionAcc := NewPatternAccumulator(30 * time.Minute)
	frictionWriter, err := NewStreamFrictionWriter("")
	if err != nil {
		logger.Warn("Failed to create stream friction writer, friction work items disabled", "error", err)
	}

	// Create heartbeat writer
	heartbeatWriter, err := NewHeartbeatWriter("")
	if err != nil {
		logger.Warn("Failed to create heartbeat writer", "error", err)
	}

	// Create per-session heartbeat writer and monitor
	sessionHBWriter, err := NewSessionHeartbeatWriter("")
	if err != nil {
		logger.Warn("Failed to create session heartbeat writer", "error", err)
	}
	sessionHBMonitor, err := NewSessionHeartbeatMonitor("", logger)
	if err != nil {
		logger.Warn("Failed to create session heartbeat monitor", "error", err)
	}
	if sessionHBMonitor != nil {
		sessionHBMonitor.SetExemptSessions(cfg.Recovery.ExemptSessions)
	}

	// Create loop heartbeat monitor
	loopMon, err := NewLoopMonitor(logger)
	if err != nil {
		logger.Warn("Failed to create loop monitor", "error", err)
	}
	if loopMon != nil {
		loopMon.SetExemptSessions(cfg.Recovery.ExemptSessions)
		if cfg.LoopMonitoring.EscalationCommand != "" {
			loopMon.SetEscalationCommand(cfg.LoopMonitoring.EscalationCommand)
		}
	}

	escalationPipeline := NewEscalationPipeline(escalationExecutor, escalationLogger, agmBinary, maxPerHour)
	escalationPipeline.SetExemptSessions(cfg.Recovery.ExemptSessions)

	return &SessionMonitor{
		tmuxClient:              tmuxClient,
		detector:                detector,
		bashDetector:            bashDetector,
		beadsDetector:           beadsDetector,
		gitDetector:             gitDetector,
		config:                  cfg,
		recoveryHistories:       make(map[string]*RecoveryHistory),
		incidentLogger:          incidentLogger,
		dedup:                   NewIncidentDeduplicator(dedupCooldown),
		escalation:              escalationPipeline,
		metrics:                 NewMetricsCollector(metricsFile),
		accumulator:             accumulator,
		streamWriter:            streamWriter,
		frictionAcc:             frictionAcc,
		frictionWriter:          frictionWriter,
		heartbeatWriter:         heartbeatWriter,
		sessionHeartbeatWriter:  sessionHBWriter,
		sessionHeartbeatMonitor: sessionHBMonitor,
		loopMonitor:             loopMon,
		logger:                  logger,
		stopChan:                make(chan struct{}),
		CrossSessionThreshold:   3,
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

	m.logger.Info("Starting Astrocyte session monitor",
		"interval", m.config.Monitoring.IntervalDuration,
		"stuck_threshold", m.config.Monitoring.StuckThresholdDuration)

	ticker := time.NewTicker(m.config.Monitoring.IntervalDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check all sessions for stuck state
			if err := m.CheckAllSessions(); err != nil {
				m.logger.Error("Error checking sessions", "error", err)
			}

		case <-m.stopChan:
			m.logger.Info("Stopping session monitor")
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

// IsRunning returns whether the monitor is currently running (thread-safe).
func (m *SessionMonitor) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// CheckAllSessions scans all tmux sessions for stuck indicators.
// For each stuck session, attempts detection and recovery.
func (m *SessionMonitor) CheckAllSessions() error {
	// Clean up stale dedup entries
	m.dedup.Cleanup()

	// Clean up expired accumulator entries
	if m.accumulator != nil {
		m.accumulator.Cleanup()
	}
	if m.frictionAcc != nil {
		m.frictionAcc.Cleanup()
	}

	// List all tmux sessions
	sessions, err := m.tmuxClient.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	if m.config.Logging.Verbose {
		m.logger.Debug("Checking tmux sessions", "count", len(sessions))
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

		sessionErr := m.checkSession(sessionName)
		if sessionErr != nil {
			m.logger.Error("Error checking session", "session", sessionName, "error", sessionErr)
			// Continue to next session even if one fails
		}

		// Write per-session heartbeat.
		if m.sessionHeartbeatWriter != nil {
			if beatErr := m.sessionHeartbeatWriter.Beat(sessionName, sessionErr == nil); beatErr != nil {
				m.logger.Error("Failed to write session heartbeat", "session", sessionName, "error", beatErr)
			}
		}
	}

	// Check session heartbeat freshness and alert on stale sessions.
	if m.sessionHeartbeatMonitor != nil {
		m.sessionHeartbeatMonitor.CheckAll()
	}

	// Check loop heartbeat freshness and trigger wakes for stale loops.
	if m.loopMonitor != nil {
		m.loopMonitor.CheckAll()
	}

	if m.metrics != nil {
		if _, err := m.metrics.FlushIfDue(); err != nil {
			m.logger.Error("Failed to flush metrics", "error", err)
		}
	}

	// Write heartbeat after successful scan cycle.
	if m.heartbeatWriter != nil {
		scanOK := err == nil
		if beatErr := m.heartbeatWriter.Beat(len(sessions), scanOK); beatErr != nil {
			m.logger.Error("Failed to write heartbeat", "error", beatErr)
		}
	}

	// Notify systemd watchdog (no-op if not running under systemd).
	if notifyErr := NotifySystemdWatchdog(); notifyErr != nil {
		m.logger.Error("Failed to notify systemd watchdog", "error", notifyErr)
	}

	return nil
}

// checkSession checks a single session for stuck state and performs recovery if needed.
func (m *SessionMonitor) checkSession(sessionName string) error {
	// Exempt sessions (orchestrator, meta-orchestrator, human) skip full stuck detection
	// because their long inference periods cause false positives. However, we still
	// check meta-orchestrator sessions for permission prompts and auto-deny them to
	// prevent pipeline deadlock (the meta-orchestrator must never block on a prompt).
	if m.config.Recovery.IsSessionExempt(sessionName) {
		if isMetaOrchestrator(sessionName) {
			return m.autoDenyPermissionPrompt(sessionName)
		}
		if m.config.Logging.Verbose {
			m.logger.Debug("Skipping exempt session", "session", sessionName)
		}
		return nil
	}

	// Get pane information
	paneInfo, err := m.tmuxClient.GetPaneInfo(sessionName)
	if err != nil {
		return fmt.Errorf("failed to get pane info: %w", err)
	}

	// Parse hook errors from pane output and feed into accumulator
	m.processHookErrors(sessionName, paneInfo.Content)

	// Scan for friction signals in pane content
	m.processFrictionSignals(sessionName, paneInfo.Content)

	// Track cursor position for freeze detection
	m.detector.TrackSession(sessionName, paneInfo.CursorX, paneInfo.CursorY)

	// Track content length for token-consumption-based stuck detection
	m.detector.TrackContent(sessionName, len(paneInfo.Content))

	// Check if session is stuck
	stuckInfo := m.detector.DetectStuckSession(paneInfo)
	if stuckInfo == nil {
		// Session is not stuck, nothing to do
		return nil
	}

	if m.config.Logging.Verbose {
		m.logger.Debug("Stuck session detected", "details", stuckInfo.String())
	}

	// Attempt recovery
	return m.RecoverSession(sessionName, stuckInfo, paneInfo)
}

// isMetaOrchestrator returns true if the session name indicates a meta-orchestrator.
func isMetaOrchestrator(sessionName string) bool {
	return strings.HasPrefix(sessionName, "meta-orchestrator")
}

// autoDenyPermissionPrompt checks if a meta-orchestrator session has a permission
// prompt and auto-denies it. This prevents pipeline deadlock where the top-level
// supervisor gets stuck waiting for human approval it can never get.
//
// Recovery sequence:
//  1. Capture pane content
//  2. Check for permission prompt indicators
//  3. If found, auto-reject via agm send reject
//  4. Log the denied operation for investigation
func (m *SessionMonitor) autoDenyPermissionPrompt(sessionName string) error {
	paneInfo, err := m.tmuxClient.GetPaneInfo(sessionName)
	if err != nil {
		return nil //nolint:nilerr // Can't check — fail open (don't block monitoring loop)
	}

	// Check for permission prompt
	indicators := paneInfo.DetectStuckIndicators()
	if !indicators["permission_prompt"] {
		return nil // No permission prompt — nothing to do
	}

	// Extract the command that triggered the prompt for logging
	command := paneInfo.ExtractLastCommand()

	m.logger.Warn("[sentinel] auto-denying permission prompt on meta-orchestrator",
		"session", sessionName,
		"command", command)

	// Auto-reject via agm send reject
	reason := fmt.Sprintf("[sentinel] denied: %s. Logged for investigation. Use agm escape-ui or agm send msg instead of raw tmux commands.", command)
	rejectCmd := exec.Command("agm", "send", "reject", sessionName, "--reason", reason)
	output, rejectErr := rejectCmd.CombinedOutput()
	if rejectErr != nil {
		m.logger.Error("[sentinel] auto-deny failed",
			"session", sessionName,
			"error", rejectErr,
			"output", string(output))
		return nil // Don't block monitoring loop
	}

	m.logger.Info("[sentinel] auto-denied permission prompt on meta-orchestrator",
		"session", sessionName,
		"command", command)

	// Log incident for investigation
	if m.incidentLogger != nil {
		recoveryMethod := "sentinel_auto_deny"
		success := true
		incident := &Incident{
			ID:                fmt.Sprintf("sentinel-autodeny-%s", time.Now().Format("20060102-150405")),
			Timestamp:         time.Now().Format(time.RFC3339),
			SessionName:       sessionName,
			SessionID:         sessionName,
			Symptom:           "meta_orchestrator_permission_prompt",
			Command:           command,
			RecoveryAttempted: true,
			RecoveryMethod:    &recoveryMethod,
			RecoverySuccess:   &success,
		}
		if logErr := m.incidentLogger.LogIncident(incident); logErr != nil {
			m.logger.Error("Failed to log auto-deny incident", "error", logErr)
		}
	}

	return nil
}

// processHookErrors parses hook errors from pane content, records them in the
// accumulator, and emits work items when cross-session thresholds are met.
func (m *SessionMonitor) processHookErrors(sessionName, content string) {
	if m.accumulator == nil {
		return
	}

	hookErrors := ParseHookErrors(content)
	if len(hookErrors) == 0 {
		return
	}

	for _, he := range hookErrors {
		// Try to match against known violation patterns
		var patternID, severity string
		if p := he.MatchPattern(m.bashDetector); p != nil {
			patternID = p.ID
			severity = p.Severity
		} else if p := he.MatchPattern(m.beadsDetector); p != nil {
			patternID = p.ID
			severity = p.Severity
		} else if p := he.MatchPattern(m.gitDetector); p != nil {
			patternID = p.ID
			severity = p.Severity
		}

		// Fall back to hook name or reason as pattern ID
		if patternID == "" {
			if he.HookName != "" {
				patternID = "hook:" + he.HookName
			} else {
				patternID = "hook:" + he.Tool
			}
			severity = "medium"
		}

		m.accumulator.Record(sessionName, patternID, severity, he.Raw)
	}

	// Clean up expired entries
	m.accumulator.Cleanup()

	// Check for cross-session patterns
	m.checkCrossSessionThresholds()
}

// checkCrossSessionThresholds checks if any patterns have crossed the
// cross-session threshold and writes work items to stream-errors/pending.jsonl.
func (m *SessionMonitor) checkCrossSessionThresholds() {
	if m.streamWriter == nil || m.accumulator == nil {
		return
	}

	threshold := m.CrossSessionThreshold
	if threshold <= 0 {
		threshold = 3
	}

	crossPatterns := m.accumulator.GetCrossSessionPatterns(threshold)
	for _, patternID := range crossPatterns {
		item := BuildStreamErrorItem(m.accumulator, patternID)
		if err := m.streamWriter.WriteItem(item); err != nil {
			m.logger.Error("Failed to write stream error item",
				"pattern_id", patternID, "error", err)
		} else {
			m.logger.Info("Cross-session pattern detected, work item emitted",
				"pattern_id", patternID,
				"session_count", item.SessionCount)
		}
	}
}

// processFrictionSignals scans pane content for friction phrases, records them
// in the friction accumulator, and emits work items when thresholds are met.
func (m *SessionMonitor) processFrictionSignals(sessionName, content string) {
	if m.frictionAcc == nil {
		return
	}

	signals := DetectFriction(content)
	if len(signals) == 0 {
		return
	}

	for _, sig := range signals {
		m.frictionAcc.Record(sessionName, sig.PatternID, "medium", sig.Raw)
	}

	m.checkFrictionThresholds()
}

// checkFrictionThresholds checks if any friction patterns have crossed the
// cross-session threshold and writes work items to stream-friction/pending.jsonl.
func (m *SessionMonitor) checkFrictionThresholds() {
	if m.frictionWriter == nil || m.frictionAcc == nil {
		return
	}

	threshold := m.CrossSessionThreshold
	if threshold <= 0 {
		threshold = 3
	}

	crossPatterns := m.frictionAcc.GetCrossSessionPatterns(threshold)
	for _, patternID := range crossPatterns {
		// Look up description from friction patterns
		description := "Recurring friction detected across sessions"
		for _, fp := range frictionPatterns {
			if fp.patternID == patternID {
				description = fp.description
				break
			}
		}

		item := BuildStreamFrictionItem(m.frictionAcc, patternID, description)
		if err := m.frictionWriter.WriteItem(item); err != nil {
			m.logger.Error("Failed to write friction work item",
				"pattern_id", patternID, "error", err)
		} else {
			m.logger.Info("Cross-session friction detected, work item emitted",
				"pattern_id", patternID,
				"session_count", item.SessionCount,
				"occurrence_count", item.OccurrenceCount)
		}
	}
}

// RecoverSession handles recovery for a stuck session.
// Detects violation pattern, sends rejection message, logs incident, and files violation.
//
//nolint:gocyclo // recovery orchestration requires sequential branching
func (m *SessionMonitor) RecoverSession(sessionName string, stuckInfo *SessionStuckInfo, paneInfo *tmux.PaneInfo) error {
	// Extract last command from pane content (needed for both escalation and recovery)
	command := paneInfo.ExtractLastCommand()
	if command == "" {
		command = stuckInfo.LastCommand
	}

	// Check incident deduplication
	if !m.dedup.ShouldLog(sessionName, stuckInfo.Reason) {
		m.logger.Debug("Suppressing duplicate incident", "session", sessionName, "symptom", stuckInfo.Reason)
		if m.metrics != nil {
			m.metrics.RecordSuppression()
		}
		return nil
	}

	// Check recovery history (circuit breaker)
	history, exists := m.recoveryHistories[sessionName]
	if !exists {
		cooldown := time.Duration(m.config.Escalation.CooldownMinutes) * time.Minute
		history = NewRecoveryHistory(sessionName, m.config.Recovery.MaxAttempts, cooldown)
		m.recoveryHistories[sessionName] = history
	}

	if !history.CanAttemptRecovery() {
		m.logger.Warn("Circuit breaker triggered, escalating",
			"session", sessionName,
			"max_attempts", m.config.Recovery.MaxAttempts)
		if m.escalation != nil && m.config.Escalation.Enabled {
			result, err := m.escalation.Escalate(sessionName, stuckInfo.Reason, command)
			if err != nil {
				m.logger.Error("Escalation failed", "session", sessionName, "error", err)
			} else if m.metrics != nil {
				m.metrics.RecordEscalation(string(result.Action))
			}
		}
		return nil
	}

	// Safety check: don't inject messages or apply recovery if a human is present
	if isHumanPresent(sessionName) {
		m.logger.Info("Safety guard: human detected, skipping automated recovery",
			"session", sessionName, "symptom", stuckInfo.Reason)
		return nil
	}

	// Detect violation pattern
	pattern := m.detectViolationPattern(command, paneInfo.Content)

	var rejectionMessage string
	if pattern != nil {
		// Generate rejection message
		rejectionMessage = enforcement.GenerateRejectionMessage(pattern, command)

		// Send rejection message to session
		if err := SendRejectionMessage(sessionName, rejectionMessage, pattern); err != nil {
			m.logger.Error("Failed to send rejection message", "error", err)
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
			m.logger.Error("Failed to file violation", "error", err)
		}
	}

	// Select recovery strategy based on symptom type
	strategy := StrategyForSymptom(stuckInfo.Reason)
	m.logger.Info("Symptom-based recovery selected",
		"session", sessionName,
		"symptom", stuckInfo.Reason,
		"strategy", strategy.String())

	// Permission prompt escalation: if recovery has been tried and prompt persists
	// past the escalation threshold, force escalation to orchestrator/human.
	if stuckInfo.Reason == "stuck_permission_prompt_escalate" {
		m.logger.Warn("Permission prompt persisted past escalation threshold",
			"session", sessionName)
		if m.escalation != nil && m.config.Escalation.Enabled {
			result, err := m.escalation.Escalate(sessionName, stuckInfo.Reason, command)
			if err != nil {
				m.logger.Error("Permission prompt escalation failed", "session", sessionName, "error", err)
			} else if m.metrics != nil {
				m.metrics.RecordEscalation(string(result.Action))
			}
		}
	}

	// Apply recovery if enabled
	var recoveryResult *RecoveryResult
	if m.config.Recovery.Enabled {
		var err error
		recoveryResult, err = ApplyRecovery(sessionName, strategy, m.tmuxClient)
		if err != nil {
			m.logger.Error("Recovery failed for session", "session", sessionName, "error", err)
		}

		// Record recovery attempt
		history.RecordAttempt(strategy, recoveryResult != nil && recoveryResult.Success, stuckInfo.Reason)
	}

	// Log incident
	recoveryMethodStr := strategy.String()
	incident := &Incident{
		ID:                 fmt.Sprintf("astrocyte-%s", time.Now().Format("20060102-150405")),
		Timestamp:          time.Now().Format(time.RFC3339),
		SessionName:        sessionName,
		SessionID:          sessionName,
		Symptom:            stuckInfo.Reason,
		DurationMinutes:    int(m.config.Monitoring.StuckThresholdDuration.Minutes()),
		DetectionHeuristic: stuckInfo.Reason,
		PaneSnapshot:       truncateContent(paneInfo.Content, 500),
		CursorPosition:     fmt.Sprintf("%d,%d", paneInfo.CursorX, paneInfo.CursorY),
		RecoveryAttempted:  m.config.Recovery.Enabled,
		RecoveryMethod:     &recoveryMethodStr,
	}

	if pattern != nil {
		incident.PatternID = pattern.ID
		incident.Severity = pattern.Severity
		incident.Command = command
	}

	if recoveryResult != nil {
		success := recoveryResult.Success
		incident.RecoverySuccess = &success
		durationSec := float64(recoveryResult.DurationMs) / 1000.0
		incident.RecoveryDurationSeconds = &durationSec
	}

	if err := m.incidentLogger.LogIncident(incident); err != nil {
		m.logger.Error("Failed to log incident", "error", err)
	}

	if m.metrics != nil {
		m.metrics.RecordIncident(stuckInfo.Reason)
	}

	// Write diagnosis file if enabled
	if m.config.Logging.DiagnosesDir != "" {
		if err := m.writeDiagnosis(sessionName, incident, pattern, rejectionMessage); err != nil {
			m.logger.Error("Failed to write diagnosis", "error", err)
		}
	}

	return nil
}

// detectViolationPattern attempts to detect a violation pattern in command.
// Tries bash, beads, and git detectors in sequence.
// Only matches against the extracted command, NOT the full pane content,
// to prevent false positives from code snippets, discussions, or other
// sessions' output appearing in the pane.
func (m *SessionMonitor) detectViolationPattern(command, _ string) *enforcement.Pattern {
	if command == "" {
		return nil
	}

	// Try bash patterns first (most common)
	if pattern := m.bashDetector.Detect(command); pattern != nil {
		return pattern
	}

	// Try beads patterns
	if pattern := m.beadsDetector.Detect(command); pattern != nil {
		return pattern
	}

	// Try git patterns
	if pattern := m.gitDetector.Detect(command); pattern != nil {
		return pattern
	}

	return nil
}

// detectPatternType determines pattern type (bash/beads/git) from pattern.
func (m *SessionMonitor) detectPatternType(pattern *enforcement.Pattern) string {
	// Try to match against loaded pattern databases
	// This is a simple heuristic - in production, pattern should include type field
	if m.bashDetector != nil {
		if p := m.bashDetector.Detect(""); p != nil && p.ID == pattern.ID {
			return "bash"
		}
	}
	if m.beadsDetector != nil {
		if p := m.beadsDetector.Detect(""); p != nil && p.ID == pattern.ID {
			return "beads"
		}
	}
	if m.gitDetector != nil {
		if p := m.gitDetector.Detect(""); p != nil && p.ID == pattern.ID {
			return "git"
		}
	}
	return "bash" // Default to bash
}

// writeDiagnosis creates a diagnosis markdown file for the incident.
// File format matches Python Astrocyte output (YAML frontmatter + markdown body).
func (m *SessionMonitor) writeDiagnosis(sessionName string, incident *Incident, pattern *enforcement.Pattern, rejectionMessage string) error {
	// Create diagnoses directory if needed
	if err := os.MkdirAll(m.config.Logging.DiagnosesDir, 0750); err != nil {
		return fmt.Errorf("failed to create diagnoses directory: %w", err)
	}

	// Generate filename: diagnosis-{session_id}.md
	filename := fmt.Sprintf("diagnosis-%s.md", sessionName)
	filePath := filepath.Join(m.config.Logging.DiagnosesDir, filename)

	// Build diagnosis content
	recoveryMethod := ""
	if incident.RecoveryMethod != nil {
		recoveryMethod = *incident.RecoveryMethod
	}
	recoverySuccess := false
	if incident.RecoverySuccess != nil {
		recoverySuccess = *incident.RecoverySuccess
	}
	content := fmt.Sprintf(`---
session_id: %s
symptom: %s
timestamp: %s
cursor_position: %s
recovery_method: %s
recovery_success: %v
`, sessionName, incident.Symptom, incident.Timestamp, incident.CursorPosition,
		recoveryMethod, recoverySuccess)

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
	content += fmt.Sprintf("**Method**: %s\n", recoveryMethod)
	content += fmt.Sprintf("**Success**: %v\n", recoverySuccess)
	durationMs := int64(0)
	if incident.RecoveryDurationSeconds != nil {
		durationMs = int64(*incident.RecoveryDurationSeconds * 1000)
	}
	content += fmt.Sprintf("**Duration**: %d ms\n\n", durationMs)

	if rejectionMessage != "" {
		content += "## Rejection Message\n\n"
		content += fmt.Sprintf("```\n%s\n```\n", rejectionMessage)
	}

	// Write file
	return os.WriteFile(filePath, []byte(content), 0600)
}

// truncateContent truncates content to maxChars, preserving last N lines.
func truncateContent(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[len(content)-maxChars:]
}

// Incident represents a stuck session incident for logging.
// Field names are chosen for compatibility with the AGM watcher
// (agm/internal/astrocyte/watcher.go) which reads
// incidents.jsonl and publishes events to the eventbus.
type Incident struct {
	ID                      string   `json:"id"`
	Timestamp               string   `json:"timestamp"`
	SessionName             string   `json:"session_name"`
	SessionID               string   `json:"session_id"`
	PatternID               string   `json:"pattern_id,omitempty"`
	Severity                string   `json:"severity,omitempty"`
	Command                 string   `json:"command,omitempty"`
	Symptom                 string   `json:"symptom"`
	DurationMinutes         int      `json:"duration_minutes"`
	DetectionHeuristic      string   `json:"detection_heuristic"`
	PaneSnapshot            string   `json:"pane_snapshot,omitempty"`
	CursorPosition          string   `json:"cursor_position"`
	RecoveryAttempted       bool     `json:"recovery_attempted"`
	RecoveryMethod          *string  `json:"recovery_method"`
	RecoverySuccess         *bool    `json:"recovery_success"`
	RecoveryDurationSeconds *float64 `json:"recovery_duration_seconds"`
	DiagnosisFiled          bool     `json:"diagnosis_filed"`
	DiagnosisFile           *string  `json:"diagnosis_file,omitempty"`
}

// IncidentLogger logs incidents to incidents.jsonl.
type IncidentLogger struct {
	filePath string
	mu       sync.Mutex
}

// NewIncidentLogger creates a new incident logger.
func NewIncidentLogger(filePath string) (*IncidentLogger, error) {
	// Create directory if needed
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create incident log directory: %w", err)
	}

	return &IncidentLogger{filePath: filePath}, nil
}

// LogIncident appends an incident to the JSONL file.
func (l *IncidentLogger) LogIncident(incident *Incident) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Serialize to JSON
	data, err := json.Marshal(incident)
	if err != nil {
		return fmt.Errorf("failed to marshal incident: %w", err)
	}

	// Append to file
	f, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
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

	// Ensure written to disk (crash safety)
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to sync incidents file: %w", err)
	}

	return nil
}
