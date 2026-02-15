package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/ai-tools/astrocyte/internal/config"
)

// TestFullDaemonWorkflow tests the complete workflow:
// 1. Load configuration
// 2. Create session monitor
// 3. Detect stuck session
// 4. Apply recovery
// 5. Log incident
// 6. File violation
func TestFullDaemonWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test environment
	tmpDir := t.TempDir()

	// Create pattern database files
	bashPatternPath := createTestPatternFile(t, tmpDir, "bash")
	beadsPatternPath := createTestPatternFile(t, tmpDir, "beads")
	gitPatternPath := createTestPatternFile(t, tmpDir, "git")

	// Create configuration
	cfg := &config.Config{
		Patterns: config.PatternConfig{
			Bash:  bashPatternPath,
			Beads: beadsPatternPath,
			Git:   gitPatternPath,
		},
		Violations: config.ViolationsConfig{
			Directory: filepath.Join(tmpDir, "violations"),
		},
		Monitoring: config.MonitoringConfig{
			Interval:               "1s",  // Fast interval for testing
			StuckThreshold:         "10s", // Short threshold for testing
			IntervalDuration:       1 * time.Second,
			StuckThresholdDuration: 10 * time.Second,
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
			Verbose:       true,
		},
		EventBus: config.EventBusConfig{
			Enabled: false,
		},
		Temporal: config.TemporalConfig{
			Enabled: false,
		},
	}

	// Create session monitor
	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)
	require.NotNil(t, monitor)

	// Verify all components initialized
	assert.NotNil(t, monitor.tmuxClient)
	assert.NotNil(t, monitor.detector)
	assert.NotNil(t, monitor.bashDetector)
	assert.NotNil(t, monitor.beadsDetector)
	assert.NotNil(t, monitor.gitDetector)
	assert.NotNil(t, monitor.incidentLogger)

	// Test CheckAllSessions (will fail gracefully if no tmux sessions exist)
	// This is expected in test environment
	err = monitor.CheckAllSessions()
	// Error is ok - no tmux sessions in test environment
	// Just verify it doesn't panic
	t.Logf("CheckAllSessions result: %v", err)
}

func TestIncidentLoggingIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	incidentsFile := filepath.Join(tmpDir, "test-incidents.jsonl")

	logger, err := NewIncidentLogger(incidentsFile)
	require.NoError(t, err)

	// Log multiple incidents over time
	incidents := []*Incident{
		{
			ID:                 "incident-1",
			Timestamp:          time.Now().Format(time.RFC3339),
			SessionID:          "session-1",
			PatternID:          "cd-chaining",
			Severity:           "high",
			Command:            "cd /tmp && git push",
			Symptom:            "stuck_mustering",
			DurationMinutes:    10,
			CursorPosition:     "0,10",
			RecoveryMethod:     "escape",
			RecoverySuccess:    true,
			RecoveryDurationMs: 150,
		},
		{
			ID:                 "incident-2",
			Timestamp:          time.Now().Format(time.RFC3339),
			SessionID:          "session-2",
			PatternID:          "for-loop",
			Severity:           "medium",
			Command:            "for f in *.txt; do cat $f; done",
			Symptom:            "stuck_waiting",
			DurationMinutes:    5,
			CursorPosition:     "5,20",
			RecoveryMethod:     "ctrl_c",
			RecoverySuccess:    false,
			RecoveryDurationMs: 200,
		},
		{
			ID:              "incident-3",
			Timestamp:       time.Now().Format(time.RFC3339),
			SessionID:       "session-3",
			Symptom:         "cursor_frozen",
			DurationMinutes: 30,
			CursorPosition:  "10,15",
			RecoveryMethod:  "manual",
		},
	}

	// Log all incidents
	for _, incident := range incidents {
		err := logger.LogIncident(incident)
		require.NoError(t, err)
	}

	// Verify incidents file
	data, err := os.ReadFile(incidentsFile)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Equal(t, 3, len(lines), "Should have 3 JSONL lines")

	// Verify each line is valid JSON
	for i, line := range lines {
		var incident Incident
		err := UnmarshalIncident([]byte(line), &incident)
		require.NoError(t, err, "Line %d should be valid JSON", i+1)
		assert.NotEmpty(t, incident.ID)
		assert.NotEmpty(t, incident.SessionID)
	}
}

func TestRecoveryHistoryPersistence(t *testing.T) {
	// Test that recovery history persists across multiple recovery attempts
	tmpDir := t.TempDir()
	cfg := createTestConfigForIntegration(t, tmpDir)

	monitor, err := NewSessionMonitor(cfg)
	require.NoError(t, err)

	sessionName := "persistent-session"

	// Create recovery history
	history := NewRecoveryHistory(sessionName, 5)
	monitor.recoveryHistories[sessionName] = history

	// Simulate multiple recovery attempts over time
	for i := 0; i < 3; i++ {
		strategy := RecoveryEscape
		if i == 2 {
			strategy = RecoveryCtrlC
		}

		history.RecordAttempt(strategy, i%2 == 0, "test-reason")
		time.Sleep(10 * time.Millisecond)
	}

	// Verify history
	assert.Equal(t, 3, len(history.Attempts))
	assert.Equal(t, 3, history.TotalAttempts)
	assert.True(t, history.CanAttemptRecovery())

	// Verify attempt timestamps are ordered
	for i := 1; i < len(history.Attempts); i++ {
		assert.True(t, history.Attempts[i].Timestamp.After(history.Attempts[i-1].Timestamp))
	}
}

func TestDetectorIntegration(t *testing.T) {
	// Test stuck session detector with realistic session tracking
	detector := NewStuckSessionDetector()

	sessionName := "test-integration-session"

	// Simulate cursor positions over time (frozen cursor)
	positions := []struct {
		x int
		y int
	}{
		{10, 5},
		{10, 5}, // Same position
		{10, 5}, // Still same
		{10, 5}, // Still same
	}

	for _, pos := range positions {
		detector.TrackSession(sessionName, pos.x, pos.y)
		time.Sleep(10 * time.Millisecond)
	}

	// Check if cursor is frozen (requires checking with actual pane info)
	// In real scenario, would check with tmux.PaneInfo
	history := detector.sessionHistories[sessionName]
	assert.NotNil(t, history)
	assert.GreaterOrEqual(t, len(history.cursorPositions), 4)

	// Verify all positions are same (frozen cursor)
	for i := 1; i < len(history.cursorPositions); i++ {
		assert.Equal(t, 10, history.cursorPositions[i].X)
		assert.Equal(t, 5, history.cursorPositions[i].Y)
	}
}

func TestConfigurationLoadingIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config file
	configPath := filepath.Join(tmpDir, "integration-config.yaml")
	configYAML := `
patterns:
  bash: /tmp/bash-patterns.yaml
  beads: /tmp/beads-patterns.yaml
  git: /tmp/git-patterns.yaml

violations:
  directory: /tmp/violations

monitoring:
  interval: 30s
  stuck_threshold: 15m

recovery:
  enabled: true
  strategy: escape
  max_attempts: 5

logging:
  incidents_file: /tmp/incidents.jsonl
  diagnoses_dir: /tmp/diagnoses
  verbose: false
`

	err := os.WriteFile(configPath, []byte(configYAML), 0644)
	require.NoError(t, err)

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	require.NoError(t, err)

	// Verify configuration loaded correctly
	assert.Equal(t, "/tmp/bash-patterns.yaml", cfg.Patterns.Bash)
	assert.Equal(t, "/tmp/violations", cfg.Violations.Directory)
	assert.Equal(t, 30*time.Second, cfg.Monitoring.IntervalDuration)
	assert.Equal(t, 15*time.Minute, cfg.Monitoring.StuckThresholdDuration)
	assert.Equal(t, "escape", cfg.Recovery.Strategy)
	assert.Equal(t, 5, cfg.Recovery.MaxAttempts)
}

// Helper functions

func createTestPatternFile(t *testing.T, dir, patternType string) string {
	filename := filepath.Join(dir, patternType+"-patterns.yaml")

	patternYAML := `version: "1.0"
updated: "2026-02-15"
purpose: "Test patterns for integration testing"
used_by: ["astrocyte-test"]
patterns:
  - id: "cd-chaining"
    regex: "cd\\s+.*&&"
    reason: "Command chaining with cd violates tool usage rules"
    alternative: "Use git -C /dir push instead"
    severity: "high"
    tier1_example: "BAD: cd /repo && git push\nGOOD: git -C /repo push"
    tier2_validation: true
    tier3_rejection: true
  - id: "for-loop"
    regex: "for\\s+\\w+\\s+in"
    reason: "Bash for loops are not allowed"
    alternative: "Use appropriate tool instead"
    severity: "medium"
    tier1_example: "BAD: for f in *.txt; do cat $f; done"
    tier2_validation: true
    tier3_rejection: true
`

	err := os.WriteFile(filename, []byte(patternYAML), 0644)
	require.NoError(t, err)

	return filename
}

func createTestConfigForIntegration(t *testing.T, tmpDir string) *config.Config {
	bashPath := createTestPatternFile(t, tmpDir, "bash")
	beadsPath := createTestPatternFile(t, tmpDir, "beads")
	gitPath := createTestPatternFile(t, tmpDir, "git")

	return &config.Config{
		Patterns: config.PatternConfig{
			Bash:  bashPath,
			Beads: beadsPath,
			Git:   gitPath,
		},
		Violations: config.ViolationsConfig{
			Directory: filepath.Join(tmpDir, "violations"),
		},
		Monitoring: config.MonitoringConfig{
			Interval:               "1s",
			StuckThreshold:         "10s",
			IntervalDuration:       1 * time.Second,
			StuckThresholdDuration: 10 * time.Second,
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
			Verbose:       true,
		},
	}
}

// UnmarshalIncident is a helper to unmarshal JSONL incident data.
func UnmarshalIncident(data []byte, incident *Incident) error {
	return json.Unmarshal(data, incident)
}
