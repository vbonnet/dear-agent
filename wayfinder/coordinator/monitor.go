package coordinator

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Monitor provides hybrid monitoring (events + polling)
type Monitor struct {
	interval      time.Duration
	logDir        string
	statusPoller  *StatusPoller
	eventListener *EventListener
	logAggregator *LogAggregator
}

// StatusPoller polls WAYFINDER-STATUS.md files
type StatusPoller struct {
	interval time.Duration
	projects map[string]*ProjectStatus
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// EventListener listens for critical events
type EventListener struct {
	subscribers map[EventType][]EventHandler
	mu          sync.RWMutex
}

// LogAggregator aggregates logs from all projects
type LogAggregator struct {
	logDir string
	files  map[string]*os.File
	mu     sync.Mutex
}

// ProjectStatus represents current project state
type ProjectStatus struct {
	ProjectDir   string
	CurrentPhase string
	Progress     int // 0-100%
	LastUpdate   time.Time
	Message      string
}

// EventType defines critical events
type EventType string

// Recognized monitor EventType values.
const (
	EventProjectStarted   EventType = "project.started"
	EventProjectFailed    EventType = "project.failed"
	EventProjectCompleted EventType = "project.completed"
	EventPhaseChanged     EventType = "phase.changed"
)

// Event represents a monitoring event
type Event struct {
	Type       EventType
	ProjectDir string
	Timestamp  time.Time
	Data       map[string]any
	Error      error
}

// EventHandler handles monitoring events
type EventHandler func(Event)

// NewMonitor creates a new hybrid monitor
func NewMonitor(interval time.Duration, logDir string) *Monitor {
	if interval <= 0 {
		interval = 10 * time.Second
	}

	return &Monitor{
		interval: interval,
		logDir:   logDir,
		statusPoller: &StatusPoller{
			interval: interval,
			projects: make(map[string]*ProjectStatus),
		},
		eventListener: &EventListener{
			subscribers: make(map[EventType][]EventHandler),
		},
		logAggregator: &LogAggregator{
			logDir: logDir,
			files:  make(map[string]*os.File),
		},
	}
}

// Start begins monitoring
func (m *Monitor) Start(ctx context.Context, projectDirs []string) {
	// Ensure log directory exists
	_ = os.MkdirAll(m.logDir, 0o700)

	// Start status polling. The cancel is stored on the poller for a future
	// explicit Stop(); shutdown today is driven by the parent ctx.
	pollerCtx, pollerCancel := context.WithCancel(ctx) //nolint:gosec // TODO(stop): wire cancel through a Monitor.Stop()
	m.statusPoller.ctx = pollerCtx
	m.statusPoller.cancel = pollerCancel

	go m.statusPoller.Poll(projectDirs)
}

// Subscribe adds event handler
func (m *Monitor) Subscribe(eventType EventType, handler EventHandler) {
	m.eventListener.mu.Lock()
	defer m.eventListener.mu.Unlock()

	m.eventListener.subscribers[eventType] = append(m.eventListener.subscribers[eventType], handler)
}

// Emit sends event to handlers
func (m *Monitor) Emit(event Event) {
	m.eventListener.mu.RLock()
	defer m.eventListener.mu.RUnlock()

	handlers := m.eventListener.subscribers[event.Type]
	for _, handler := range handlers {
		// Run handler in goroutine to avoid blocking
		go handler(event)
	}
}

// GetStatus returns latest status for project
func (m *Monitor) GetStatus(projectDir string) (*ProjectStatus, error) {
	m.statusPoller.mu.RLock()
	defer m.statusPoller.mu.RUnlock()

	status, ok := m.statusPoller.projects[projectDir]
	if !ok {
		return nil, fmt.Errorf("no status for project: %s", projectDir)
	}

	// Return copy
	statusCopy := *status
	return &statusCopy, nil
}

// StreamLogs captures stdout/stderr to log file
func (m *Monitor) StreamLogs(projectID string, stdout, stderr io.Reader) {
	m.logAggregator.mu.Lock()

	// Create log file if not exists
	if _, ok := m.logAggregator.files[projectID]; !ok {
		logPath := filepath.Join(m.logAggregator.logDir, fmt.Sprintf("%s.log", projectID))
		f, err := os.Create(logPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log file for %s: %v\n", projectID, err)
			m.logAggregator.mu.Unlock()
			return
		}
		m.logAggregator.files[projectID] = f
	}

	logFile := m.logAggregator.files[projectID]
	m.logAggregator.mu.Unlock()

	// Stream stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			timestamp := time.Now().Format("15:04:05")
			fmt.Fprintf(logFile, "[%s] %s\n", timestamp, line)
		}
	}()

	// Stream stderr (with prefix)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			timestamp := time.Now().Format("15:04:05")
			fmt.Fprintf(logFile, "[%s] ERROR: %s\n", timestamp, line)
			// Also output to stderr for visibility
			fmt.Fprintf(os.Stderr, "[%s] %s\n", projectID, line)
		}
	}()
}

// Poll periodically polls project status
func (sp *StatusPoller) Poll(projectDirs []string) {
	ticker := time.NewTicker(sp.interval)
	defer ticker.Stop()

	for {
		select {
		case <-sp.ctx.Done():
			return
		case <-ticker.C:
			sp.pollOnce(projectDirs)
		}
	}
}

// pollOnce performs a single poll of all projects
func (sp *StatusPoller) pollOnce(projectDirs []string) {
	for _, dir := range projectDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			continue
		}

		status := sp.readStatus(absDir)
		if status != nil {
			sp.mu.Lock()
			sp.projects[absDir] = status
			sp.mu.Unlock()
		}
	}
}

// readStatus reads WAYFINDER-STATUS.md file
func (sp *StatusPoller) readStatus(projectDir string) *ProjectStatus {
	statusFile := filepath.Join(projectDir, "WAYFINDER-STATUS.md")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		// File doesn't exist yet, return default status
		return &ProjectStatus{
			ProjectDir:   projectDir,
			CurrentPhase: "unknown",
			Progress:     0,
			LastUpdate:   time.Now(),
			Message:      "Waiting to start...",
		}
	}

	return parseWayfinderStatus(projectDir, string(data))
}

// parseWayfinderStatus parses WAYFINDER-STATUS.md content
func parseWayfinderStatus(projectDir, content string) *ProjectStatus {
	status := &ProjectStatus{
		ProjectDir: projectDir,
		LastUpdate: time.Now(),
	}

	// Parse current phase: Look for "**Current Phase**: D2 - Research" pattern
	phaseRe := regexp.MustCompile(`\*\*Current Phase\*\*:\s*([A-Z]\d+)`)
	if match := phaseRe.FindStringSubmatch(content); len(match) > 1 {
		status.CurrentPhase = match[1]
	}

	// Parse progress: Look for percentage like "50%" or "Phase Progress: 50%"
	progressRe := regexp.MustCompile(`(\d+)%`)
	if match := progressRe.FindStringSubmatch(content); len(match) > 1 {
		progress, _ := strconv.Atoi(match[1])
		status.Progress = progress
	}

	// Parse status message: Look for "**Status**: ..." pattern
	messageRe := regexp.MustCompile(`\*\*Status\*\*:\s*(.+)`)
	if match := messageRe.FindStringSubmatch(content); len(match) > 1 {
		status.Message = strings.TrimSpace(match[1])
	}

	// Fallback defaults
	if status.CurrentPhase == "" {
		status.CurrentPhase = "unknown"
	}
	if status.Message == "" {
		status.Message = "In progress..."
	}

	return status
}

// Close closes all open log files
func (la *LogAggregator) Close() error {
	la.mu.Lock()
	defer la.mu.Unlock()

	var errors []error
	for projectID, f := range la.files {
		if err := f.Close(); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", projectID, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to close log files: %v", errors)
	}

	return nil
}
