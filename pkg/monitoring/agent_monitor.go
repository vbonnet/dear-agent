package monitoring

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// AgentMonitor coordinates monitoring for a single sub-agent
type AgentMonitor struct {
	AgentID      string
	WorkDir      string
	EventLogPath string
	Config       ValidationConfig

	fileWatcher  *FileWatcher
	gitHooks     *GitHookManager
	outputParser *OutputParser
	validator    *Validator
	eventBus     *eventbus.LocalBus

	started   bool
	startTime time.Time
	stopTime  time.Time
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewAgentMonitor creates a new agent monitor
func NewAgentMonitor(agentID, workDir string) (*AgentMonitor, error) {
	// Generate event log path
	eventLogPath := filepath.Join(os.TempDir(), fmt.Sprintf("sub-agent-monitor-%s.jsonl", agentID))

	// Create EventBus instance (caller can provide custom bus if needed)
	bus := eventbus.NewBus(nil)

	ctx, cancel := context.WithCancel(context.Background())

	am := &AgentMonitor{
		AgentID:      agentID,
		WorkDir:      workDir,
		EventLogPath: eventLogPath,
		Config:       DefaultValidationConfig,
		eventBus:     bus,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Initialize components
	var err error

	am.fileWatcher, err = NewFileWatcher(agentID, workDir, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	am.gitHooks, err = NewGitHookManager(agentID, workDir, eventLogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create git hook manager: %w", err)
	}

	am.outputParser = NewOutputParser(agentID, bus)

	am.validator = NewValidator(agentID, workDir, eventLogPath, am.Config)

	return am, nil
}

// SetValidationConfig updates validation configuration
func (am *AgentMonitor) SetValidationConfig(config ValidationConfig) {
	am.Config = config
	am.validator = NewValidator(am.AgentID, am.WorkDir, am.EventLogPath, config)
}

// Start begins monitoring
func (am *AgentMonitor) Start() error {
	if am.started {
		return fmt.Errorf("monitor already started")
	}

	am.startTime = time.Now()
	am.started = true

	// Publish agent started event
	am.eventBus.Publish(context.Background(), &eventbus.Event{
		Type:      EventAgentStarted,
		Source:    "agent-monitor",
		Data: map[string]interface{}{
			"agent_id":  am.AgentID,
			"workdir":   am.WorkDir,
			"timestamp": am.startTime.Format(time.RFC3339),
		},
	})

	// Start file watcher
	if err := am.fileWatcher.Start(); err != nil {
		// Log warning but continue (monitoring is best-effort)
		slog.Warn("file watcher failed to start", "error", err)
	}

	// Install git hooks (if .git exists)
	if _, err := os.Stat(filepath.Join(am.WorkDir, ".git")); err == nil {
		if err := am.gitHooks.InstallHooks(); err != nil {
			// Log warning but continue
			slog.Warn("git hooks failed to install", "error", err)
		}
	}

	return nil
}

// Stop stops monitoring and cleans up
func (am *AgentMonitor) Stop() error {
	if !am.started {
		return nil
	}

	am.stopTime = time.Now()

	// Stop file watcher
	if am.fileWatcher != nil {
		am.fileWatcher.Stop()
	}

	// Uninstall git hooks
	if am.gitHooks != nil {
		am.gitHooks.UninstallHooks()
	}

	// Publish agent completed event
	duration := am.stopTime.Sub(am.startTime)
	am.eventBus.Publish(context.Background(), &eventbus.Event{
		Type:      EventAgentDone,
		Source:    "agent-monitor",
		Data: map[string]interface{}{
			"agent_id":     am.AgentID,
			"duration_sec": duration.Seconds(),
			"timestamp":    am.stopTime.Format(time.RFC3339),
		},
	})

	// Cancel context
	am.cancel()

	return nil
}

// ParseOutput processes sub-agent output line by line
func (am *AgentMonitor) ParseOutput(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		am.outputParser.ParseLine(line)
	}
	return scanner.Err()
}

// Validate runs validation and returns result
func (am *AgentMonitor) Validate() (*ValidationResult, error) {
	return am.validator.Validate()
}

// GetStats returns aggregated monitoring statistics
func (am *AgentMonitor) GetStats() MonitorStats {
	duration := time.Duration(0)
	if am.started {
		if am.stopTime.IsZero() {
			duration = time.Since(am.startTime)
		} else {
			duration = am.stopTime.Sub(am.startTime)
		}
	}

	// Count events from log
	filesCreated := am.countEventsByType(EventFileCreated)
	filesModified := am.countEventsByType(EventFileModified)
	commits := am.countEventsByType(EventGitCommit)
	tests := am.countEventsByType(EventTestStarted)

	return MonitorStats{
		AgentID:         am.AgentID,
		StartTime:       am.startTime,
		Duration:        duration,
		FilesCreated:    filesCreated,
		FilesModified:   filesModified,
		CommitsDetected: commits,
		TestRuns:        tests,
	}
}

// countEventsByType counts events of specific type in log
func (am *AgentMonitor) countEventsByType(eventType string) int {
	file, err := os.Open(am.EventLogPath)
	if err != nil {
		return 0
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Simple check (could parse JSON for more accuracy)
		line := scanner.Text()
		if contains(line, eventType) && contains(line, am.AgentID) {
			count++
		}
	}
	return count
}

// contains checks if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetEventLog returns the path to the event log file
func (am *AgentMonitor) GetEventLog() string {
	return am.EventLogPath
}
