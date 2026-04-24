package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/agm/internal/sentinel/config"
	"github.com/vbonnet/dear-agent/pkg/enforcement"
)

func TestNewSessionMonitor(t *testing.T) {
	// Create test config
	cfg := createTestConfig(t)

	// Create session monitor
	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)
	assert.NotNil(t, monitor)
	assert.NotNil(t, monitor.tmuxClient)
	assert.NotNil(t, monitor.detector)
	assert.NotNil(t, monitor.bashDetector)
	assert.NotNil(t, monitor.beadsDetector)
	assert.NotNil(t, monitor.gitDetector)
	assert.NotNil(t, monitor.incidentLogger)
	assert.NotNil(t, monitor.recoveryHistories)
	assert.False(t, monitor.IsRunning())
}

func TestIncidentLogger_LogIncident(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "incidents.jsonl")

	logger, err := NewIncidentLogger(incidentsFile)
	require.NoError(t, err)

	// Log first incident
	escapeMethod := "escape"
	trueVal := true
	dur150 := 0.15
	incident1 := &Incident{
		ID:                      "test-incident-1",
		Timestamp:               time.Now().Format(time.RFC3339),
		SessionName:             "test-session",
		SessionID:               "test-session",
		PatternID:               "cd-chaining",
		Severity:                "high",
		Command:                 "cd /tmp && git push",
		Symptom:                 "stuck_mustering",
		DurationMinutes:         10,
		DetectionHeuristic:      "mustering_timeout",
		CursorPosition:          "0,10",
		RecoveryAttempted:       true,
		RecoveryMethod:          &escapeMethod,
		RecoverySuccess:         &trueVal,
		RecoveryDurationSeconds: &dur150,
	}

	err = logger.LogIncident(incident1)
	require.NoError(t, err)

	// Log second incident
	ctrlcMethod := "ctrl_c"
	falseVal := false
	dur200 := 0.2
	incident2 := &Incident{
		ID:                      "test-incident-2",
		Timestamp:               time.Now().Format(time.RFC3339),
		SessionName:             "another-session",
		SessionID:               "another-session",
		Symptom:                 "stuck_waiting",
		DurationMinutes:         5,
		DetectionHeuristic:      "zero_token_galloping",
		CursorPosition:          "5,20",
		RecoveryAttempted:       true,
		RecoveryMethod:          &ctrlcMethod,
		RecoverySuccess:         &falseVal,
		RecoveryDurationSeconds: &dur200,
	}

	err = logger.LogIncident(incident2)
	require.NoError(t, err)

	// Read and verify incidents file
	data, err := os.ReadFile(incidentsFile)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Equal(t, 2, len(lines))

	// Parse first incident
	var parsedIncident1 Incident
	err = json.Unmarshal([]byte(lines[0]), &parsedIncident1)
	require.NoError(t, err)
	assert.Equal(t, "test-incident-1", parsedIncident1.ID)
	assert.Equal(t, "test-session", parsedIncident1.SessionName)
	assert.Equal(t, "test-session", parsedIncident1.SessionID)
	assert.Equal(t, "cd-chaining", parsedIncident1.PatternID)
	assert.Equal(t, "high", parsedIncident1.Severity)
	require.NotNil(t, parsedIncident1.RecoverySuccess)
	assert.True(t, *parsedIncident1.RecoverySuccess)

	// Parse second incident
	var parsedIncident2 Incident
	err = json.Unmarshal([]byte(lines[1]), &parsedIncident2)
	require.NoError(t, err)
	assert.Equal(t, "test-incident-2", parsedIncident2.ID)
	assert.Equal(t, "another-session", parsedIncident2.SessionID)
	require.NotNil(t, parsedIncident2.RecoverySuccess)
	assert.False(t, *parsedIncident2.RecoverySuccess)
}

func TestIncidentLogger_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Use nested directory that doesn't exist
	incidentsFile := filepath.Join(tmpDir, "nested/dir/incidents.jsonl")

	logger, err := NewIncidentLogger(incidentsFile)
	require.NoError(t, err)

	// Verify directory was created
	dir := filepath.Dir(incidentsFile)
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())

	// Verify can log to file
	method := "escape"
	incident := &Incident{
		ID:              "test-1",
		Timestamp:       time.Now().Format(time.RFC3339),
		SessionName:     "session-1",
		SessionID:       "session-1",
		Symptom:         "test",
		DurationMinutes: 1,
		CursorPosition:  "0,0",
		RecoveryMethod:  &method,
	}

	err = logger.LogIncident(incident)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(incidentsFile)
	require.NoError(t, err)
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		maxChars int
		want     string
	}{
		{
			name:     "content shorter than limit",
			content:  "short content",
			maxChars: 100,
			want:     "short content",
		},
		{
			name:     "content longer than limit",
			content:  "This is a very long content that should be truncated to only the last N characters",
			maxChars: 20,
			want:     "he last N characters",
		},
		{
			name:     "exact length",
			content:  "exact",
			maxChars: 5,
			want:     "exact",
		},
		{
			name:     "empty content",
			content:  "",
			maxChars: 10,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateContent(tt.content, tt.maxChars)
			assert.Equal(t, tt.want, got)
			assert.LessOrEqual(t, len(got), tt.maxChars)
		})
	}
}

func TestSessionMonitor_WriteDiagnosis(t *testing.T) {
	cfg := createTestConfig(t)
	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)

	diagEscapeMethod := "escape"
	diagSuccess := true
	diagDuration := 0.15
	incident := &Incident{
		ID:                      "diag-test-1",
		Timestamp:               "2026-02-15T10:00:00Z",
		SessionName:             "test-session",
		SessionID:               "test-session",
		PatternID:               "cd-chaining",
		Severity:                "high",
		Command:                 "cd /tmp && git push",
		Symptom:                 "stuck_mustering",
		DurationMinutes:         10,
		CursorPosition:          "0,10",
		RecoveryAttempted:       true,
		RecoveryMethod:          &diagEscapeMethod,
		RecoverySuccess:         &diagSuccess,
		RecoveryDurationSeconds: &diagDuration,
	}

	// Create mock pattern (normally would come from detector)
	pattern := &enforcement.Pattern{
		ID:          "cd-chaining",
		Reason:      "Command chaining with cd",
		Alternative: "Use git -C /tmp push",
		Severity:    "high",
	}

	rejectionMessage := "Test rejection message"

	// Write diagnosis
	err = monitor.writeDiagnosis("test-session", incident, pattern, rejectionMessage)
	require.NoError(t, err)

	// Verify diagnosis file was created
	diagnosisPath := filepath.Join(cfg.Logging.DiagnosesDir, "diagnosis-test-session.md")
	_, err = os.Stat(diagnosisPath)
	require.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(diagnosisPath)
	require.NoError(t, err)

	contentStr := string(content)

	// Verify frontmatter
	assert.Contains(t, contentStr, "session_id: test-session")
	assert.Contains(t, contentStr, "symptom: stuck_mustering")
	assert.Contains(t, contentStr, "pattern_id: cd-chaining")
	assert.Contains(t, contentStr, "severity: high")

	// Verify markdown sections
	assert.Contains(t, contentStr, "# Diagnosis: test-session")
	assert.Contains(t, contentStr, "## Command")
	assert.Contains(t, contentStr, "cd /tmp && git push")
	assert.Contains(t, contentStr, "## Violation Pattern")
	assert.Contains(t, contentStr, "Command chaining with cd")
	assert.Contains(t, contentStr, "## Recovery")
	assert.Contains(t, contentStr, "**Method**: escape")
	assert.Contains(t, contentStr, "**Success**: true")
	assert.Contains(t, contentStr, "## Rejection Message")
	assert.Contains(t, contentStr, "Test rejection message")
}

func TestSessionMonitor_RecoveryHistoryTracking(t *testing.T) {
	cfg := createTestConfig(t)
	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)

	sessionName := "tracked-session"

	// Initially no history
	assert.Empty(t, monitor.recoveryHistories)

	// First recovery attempt should create history
	history := NewRecoveryHistory(sessionName, 3, 0)
	monitor.recoveryHistories[sessionName] = history

	// Verify history exists
	assert.Contains(t, monitor.recoveryHistories, sessionName)
	assert.Equal(t, sessionName, monitor.recoveryHistories[sessionName].SessionName)

	// Record some attempts
	history.RecordAttempt(RecoveryEscape, true, "stuck_1")
	history.RecordAttempt(RecoveryEscape, false, "stuck_2")

	assert.Equal(t, 2, len(monitor.recoveryHistories[sessionName].Attempts))
	assert.Equal(t, 2, monitor.recoveryHistories[sessionName].TotalAttempts)
}

func TestSessionMonitor_StopMonitoring(t *testing.T) {
	cfg := createTestConfig(t)
	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)

	// Start monitoring in background
	go func() {
		_ = monitor.StartMonitoring()
	}()

	// Wait briefly for startup
	time.Sleep(100 * time.Millisecond)
	assert.True(t, monitor.IsRunning())

	// Stop monitoring
	monitor.StopMonitoring()

	// Wait for graceful shutdown
	time.Sleep(100 * time.Millisecond)
	assert.False(t, monitor.IsRunning())
}

// Helper functions

func createTestConfig(t *testing.T) *config.Config {
	tmpDir := t.TempDir()

	// Create test pattern files
	bashPatternPath := filepath.Join(tmpDir, "bash-patterns.yaml")
	beadsPatternPath := filepath.Join(tmpDir, "beads-patterns.yaml")
	gitPatternPath := filepath.Join(tmpDir, "git-patterns.yaml")

	// Write minimal pattern YAML
	minimalPattern := `version: "1.0"
updated: "2026-02-15"
purpose: "Test patterns"
used_by: ["test"]
patterns:
  - id: "cd-chaining"
    regex: "cd\\s+.*&&"
    reason: "Command chaining with cd"
    alternative: "Use -C flag"
    severity: "high"
    tier1_example: "BAD: cd /dir && git push"
    tier2_validation: true
    tier3_rejection: true
`

	require.NoError(t, os.WriteFile(bashPatternPath, []byte(minimalPattern), 0644))
	require.NoError(t, os.WriteFile(beadsPatternPath, []byte(minimalPattern), 0644))
	require.NoError(t, os.WriteFile(gitPatternPath, []byte(minimalPattern), 0644))

	return &config.Config{
		Patterns: config.PatternConfig{
			Bash:  bashPatternPath,
			Beads: beadsPatternPath,
			Git:   gitPatternPath,
		},
		Violations: config.ViolationsConfig{
			Directory: filepath.Join(tmpDir, "violations"),
		},
		Monitoring: config.MonitoringConfig{
			Interval:               "60s",
			StuckThreshold:         "10m",
			IntervalDuration:       60 * time.Second,
			StuckThresholdDuration: 10 * time.Minute,
		},
		Tmux: config.TmuxConfig{
			Socket: "",
		},
		Recovery: config.RecoveryConfig{
			Enabled:     true,
			Strategy:    "escape",
			MaxAttempts: 3,
		},
		Logging: config.LoggingConfig{
			IncidentsFile: filepath.Join(tmpDir, "incidents.jsonl"),
			DiagnosesDir:  filepath.Join(tmpDir, "diagnoses"),
			Verbose:       false,
		},
		Escalation: config.EscalationConfig{
			Enabled:              true,
			AutoApprove:          true,
			CooldownMinutes:      30,
			MaxAutoApprovalsHour: 5,
			AgmBinary:            "agm",
		},
	}
}

// (Removed mock pattern - using actual enforcement.Pattern struct instead)

// TestSessionMonitor_ErrorConditions tests error handling.
func TestSessionMonitor_ErrorConditions(t *testing.T) {
	t.Run("missing config file", func(t *testing.T) {
		cfg := &config.Config{
			Patterns: config.PatternConfig{
				Bash:  "/nonexistent/bash-patterns.yaml",
				Beads: "/nonexistent/beads-patterns.yaml",
				Git:   "/nonexistent/git-patterns.yaml",
			},
		}

		_, err := NewSessionMonitor(cfg)
		assert.Error(t, err, "should error with missing pattern files")
	})

	t.Run("corrupted pattern file", func(t *testing.T) {
		tmpDir := t.TempDir()
		corruptedPath := filepath.Join(tmpDir, "corrupted.yaml")

		// Write invalid YAML
		err := os.WriteFile(corruptedPath, []byte("invalid: yaml: content: [[["), 0644)
		require.NoError(t, err)

		cfg := &config.Config{
			Patterns: config.PatternConfig{
				Bash:  corruptedPath,
				Beads: corruptedPath,
				Git:   corruptedPath,
			},
		}

		_, err = NewSessionMonitor(cfg)
		assert.Error(t, err, "should error with corrupted pattern file")
	})
}

// TestSessionMonitor_EmptySessionList tests handling empty session list.
func TestSessionMonitor_EmptySessionList(t *testing.T) {
	cfg := createTestConfig(t)
	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)

	// CheckAllSessions should handle empty list gracefully
	// In real environment with no tmux sessions, this should not panic
	err = monitor.CheckAllSessions()
	// Error is acceptable (no tmux running), but shouldn't panic
	t.Logf("CheckAllSessions with no sessions: %v", err)
}

// TestSessionMonitor_MultipleSessions tests monitoring multiple sessions.
func TestSessionMonitor_MultipleSessions(t *testing.T) {
	cfg := createTestConfig(t)
	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)

	// Track multiple sessions in recovery history
	sessions := []string{"session-1", "session-2", "session-3"}
	for _, session := range sessions {
		history := NewRecoveryHistory(session, 3, 0)
		monitor.recoveryHistories[session] = history
		history.RecordAttempt(RecoveryEscape, true, "test")
	}

	assert.Equal(t, 3, len(monitor.recoveryHistories))
	for _, session := range sessions {
		assert.Contains(t, monitor.recoveryHistories, session)
		assert.Equal(t, 1, monitor.recoveryHistories[session].TotalAttempts)
	}
}

// TestSessionMonitor_SessionDisappearsCheck tests handling session disappearance.
func TestSessionMonitor_SessionDisappearsCheck(t *testing.T) {
	cfg := createTestConfig(t)
	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)

	// Create history for session that no longer exists
	monitor.recoveryHistories["ghost-session"] = NewRecoveryHistory("ghost-session", 3, 0)

	// Should not panic when checking non-existent session
	err = monitor.CheckAllSessions()
	// Error expected (session doesn't exist), but no panic
	t.Logf("Check on disappeared session: %v", err)
}

// TestIncidentLogger_ConcurrentWrites tests thread safety.
func TestIncidentLogger_ConcurrentWrites(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "concurrent-incidents.jsonl")

	logger, err := NewIncidentLogger(incidentsFile)
	require.NoError(t, err)

	// Write incidents concurrently
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(id int) {
			m := "escape"
			incident := &Incident{
				ID:              fmt.Sprintf("concurrent-%d", id),
				Timestamp:       time.Now().Format(time.RFC3339),
				SessionName:     fmt.Sprintf("session-%d", id),
				SessionID:       fmt.Sprintf("session-%d", id),
				Symptom:         "test",
				DurationMinutes: 1,
				CursorPosition:  "0,0",
				RecoveryMethod:  &m,
			}
			_ = logger.LogIncident(incident)
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify file integrity
	data, err := os.ReadFile(incidentsFile)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Equal(t, 5, len(lines), "should have 5 incident lines")
}

// TestSessionMonitor_CircuitBreakerBehavior tests circuit breaker.
func TestSessionMonitor_CircuitBreakerBehavior(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.Recovery.MaxAttempts = 2

	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)

	sessionName := "circuit-test"
	history := NewRecoveryHistory(sessionName, 2, 0)
	monitor.recoveryHistories[sessionName] = history

	// First attempt - allowed
	assert.True(t, history.CanAttemptRecovery())
	history.RecordAttempt(RecoveryEscape, false, "stuck_1")

	// Second attempt - allowed
	assert.True(t, history.CanAttemptRecovery())
	history.RecordAttempt(RecoveryEscape, false, "stuck_2")

	// Third attempt - blocked by circuit breaker
	assert.False(t, history.CanAttemptRecovery(),
		"circuit breaker should prevent further attempts")
	assert.Equal(t, 2, history.TotalAttempts)
}

// TestIncidentLogger_WritePermissionError tests handling write errors.
func TestIncidentLogger_WritePermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}

	tmpDir := t.TempDir()
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	require.NoError(t, os.Mkdir(readOnlyDir, 0555)) // read-only directory

	incidentsFile := filepath.Join(readOnlyDir, "incidents.jsonl")

	logger, err := NewIncidentLogger(incidentsFile)
	// Might error on creation if directory check fails
	if err != nil {
		return // Expected
	}

	permMethod := "escape"
	incident := &Incident{
		ID:              "test-1",
		Timestamp:       time.Now().Format(time.RFC3339),
		SessionName:     "test",
		SessionID:       "test",
		Symptom:         "test",
		DurationMinutes: 1,
		CursorPosition:  "0,0",
		RecoveryMethod:  &permMethod,
	}

	err = logger.LogIncident(incident)
	assert.Error(t, err, "should error when writing to read-only directory")
}

// TestSessionMonitor_MaxRecoveryAttempts tests max attempts enforcement.
func TestSessionMonitor_MaxRecoveryAttempts(t *testing.T) {
	cfg := createTestConfig(t)
	cfg.Recovery.MaxAttempts = 3

	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)

	sessionName := "max-attempts-test"
	history := NewRecoveryHistory(sessionName, cfg.Recovery.MaxAttempts, 0)
	monitor.recoveryHistories[sessionName] = history

	// Exhaust all attempts
	for i := 0; i < cfg.Recovery.MaxAttempts; i++ {
		assert.True(t, history.CanAttemptRecovery(),
			"attempt %d should be allowed", i+1)
		history.RecordAttempt(RecoveryEscape, false, fmt.Sprintf("attempt-%d", i+1))
	}

	// No more attempts allowed
	assert.False(t, history.CanAttemptRecovery(),
		"should block after max attempts")
	assert.Equal(t, cfg.Recovery.MaxAttempts, history.TotalAttempts)
}

// TestIncidentLogger_WatcherCompatibility verifies the incident JSON format
// is compatible with the AGM watcher (agm/internal/astrocyte/watcher.go)
// which reads incidents.jsonl and publishes events to the eventbus.
func TestIncidentLogger_WatcherCompatibility(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "incidents.jsonl")

	logger, err := NewIncidentLogger(incidentsFile)
	require.NoError(t, err)

	method := "escape"
	success := true
	duration := 5.031
	incident := &Incident{
		ID:                      "astrocyte-20260328-130000",
		Timestamp:               "2026-03-28T13:00:00Z",
		SessionName:             "my-session",
		SessionID:               "my-session",
		Symptom:                 "stuck_mustering",
		DurationMinutes:         10,
		DetectionHeuristic:      "mustering_timeout",
		PaneSnapshot:            "some pane content",
		CursorPosition:          "0,38",
		RecoveryAttempted:       true,
		RecoveryMethod:          &method,
		RecoverySuccess:         &success,
		RecoveryDurationSeconds: &duration,
		DiagnosisFiled:          false,
	}

	err = logger.LogIncident(incident)
	require.NoError(t, err)

	// Read back and parse as the watcher would (AstrocyteIncident struct)
	data, err := os.ReadFile(incidentsFile)
	require.NoError(t, err)

	// Parse using a struct matching the watcher's AstrocyteIncident
	type WatcherIncident struct {
		Timestamp               string   `json:"timestamp"`
		SessionName             string   `json:"session_name"`
		SessionID               string   `json:"session_id"`
		Symptom                 string   `json:"symptom"`
		DurationMinutes         int      `json:"duration_minutes"`
		DetectionHeuristic      string   `json:"detection_heuristic"`
		PaneSnapshot            string   `json:"pane_snapshot"`
		CursorPosition          string   `json:"cursor_position"`
		RecoveryAttempted       bool     `json:"recovery_attempted"`
		RecoveryMethod          *string  `json:"recovery_method"`
		RecoverySuccess         *bool    `json:"recovery_success"`
		RecoveryDurationSeconds *float64 `json:"recovery_duration_seconds"`
		DiagnosisFiled          bool     `json:"diagnosis_filed"`
		DiagnosisFile           *string  `json:"diagnosis_file"`
	}

	var parsed WatcherIncident
	err = json.Unmarshal([]byte(strings.TrimSpace(string(data))), &parsed)
	require.NoError(t, err, "incident must be parseable by watcher format")

	// Verify all fields the watcher depends on
	assert.Equal(t, "2026-03-28T13:00:00Z", parsed.Timestamp)
	assert.Equal(t, "my-session", parsed.SessionName, "session_name required by watcher")
	assert.Equal(t, "my-session", parsed.SessionID, "session_id required by watcher for event routing")
	assert.Equal(t, "stuck_mustering", parsed.Symptom)
	assert.Equal(t, 10, parsed.DurationMinutes)
	assert.Equal(t, "mustering_timeout", parsed.DetectionHeuristic, "detection_heuristic required by watcher")
	assert.Equal(t, "some pane content", parsed.PaneSnapshot)
	assert.Equal(t, "0,38", parsed.CursorPosition)
	assert.True(t, parsed.RecoveryAttempted)
	require.NotNil(t, parsed.RecoveryMethod)
	assert.Equal(t, "escape", *parsed.RecoveryMethod)
	require.NotNil(t, parsed.RecoverySuccess)
	assert.True(t, *parsed.RecoverySuccess)
	require.NotNil(t, parsed.RecoveryDurationSeconds)
	assert.InDelta(t, 5.031, *parsed.RecoveryDurationSeconds, 0.001)
	assert.False(t, parsed.DiagnosisFiled)
}
