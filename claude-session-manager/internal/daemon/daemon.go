package daemon

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/vbonnet/ai-tools/main/claude-session-manager/internal/api"
	"github.com/vbonnet/ai-tools/main/claude-session-manager/internal/state"
	"github.com/vbonnet/ai-tools/main/claude-session-manager/internal/tmux"
)

// Daemon runs AGM as a background service
type Daemon struct {
	port          int
	statusDir     string
	pollInterval  time.Duration
	server        *api.Server
	statusWriter  *api.StatusFileWriter
	detector      *state.Detector
	sessions      map[string]*SessionMonitor
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// SessionMonitor tracks a single session
type SessionMonitor struct {
	Name         string
	LastOutput   time.Time
	LastState    state.State
	PaneID       string
}

// NewDaemon creates a new AGM daemon
func NewDaemon(port int, statusDir string, pollInterval time.Duration) (*Daemon, error) {
	// Expand home directory
	if statusDir[:2] == "~/" {
		homeDir, _ := os.UserHomeDir()
		statusDir = filepath.Join(homeDir, statusDir[2:])
	}

	detector := state.NewDetector()
	statusWriter, err := api.NewStatusFileWriter(statusDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create status writer: %w", err)
	}

	server := api.NewServer(port, detector)

	ctx, cancel := context.WithCancel(context.Background())

	return &Daemon{
		port:         port,
		statusDir:    statusDir,
		pollInterval: pollInterval,
		server:       server,
		statusWriter: statusWriter,
		detector:     detector,
		sessions:     make(map[string]*SessionMonitor),
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start starts the daemon
func (d *Daemon) Start() error {
	log.Printf("Starting AGM daemon on port %d", d.port)
	log.Printf("Status directory: %s", d.statusDir)
	log.Printf("Poll interval: %v", d.pollInterval)

	// Start HTTP server in goroutine
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.server.Start(); err != nil {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start session monitoring loop
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.monitoringSessions()
	}()

	// Wait for shutdown signal
	d.waitForShutdown()

	return nil
}

// Stop gracefully shuts down the daemon
func (d *Daemon) Stop() error {
	log.Println("Shutting down AGM daemon...")

	// Cancel context (stops monitoring loop)
	d.cancel()

	// Stop HTTP server
	if err := d.server.Stop(); err != nil {
		return fmt.Errorf("failed to stop server: %w", err)
	}

	// Wait for goroutines to finish
	d.wg.Wait()

	log.Println("AGM daemon stopped")
	return nil
}

// monitoringSessions polls tmux sessions and updates states
func (d *Daemon) monitoringSessions() {
	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.pollSessions()
		}
	}
}

// pollSessions queries all active tmux sessions
func (d *Daemon) pollSessions() {
	// Get list of active tmux sessions
	sessions, err := tmux.ListSessions()
	if err != nil {
		// No tmux sessions or tmux not available
		return
	}

	// Monitor each session
	for _, sessionName := range sessions {
		d.monitorSession(sessionName)
	}

	// Clean up sessions that no longer exist
	d.cleanupStaleSessions(sessions)
}

// monitorSession checks state of a single session
func (d *Daemon) monitorSession(sessionName string) {
	// Capture pane output
	output, err := tmux.CapturePaneOutput(sessionName, 50)
	if err != nil {
		return // Session not accessible
	}

	// Get or create monitor
	d.mu.Lock()
	monitor, exists := d.sessions[sessionName]
	if !exists {
		monitor = &SessionMonitor{
			Name:       sessionName,
			LastOutput: time.Now(),
			LastState:  state.StateUnknown,
		}
		d.sessions[sessionName] = monitor
	}
	d.mu.Unlock()

	// Detect state
	result := d.detector.DetectState(output, monitor.LastOutput)

	// Update monitor
	d.mu.Lock()
	monitor.LastState = result.State
	if result.State == state.StateThinking || result.State == state.StateReady {
		monitor.LastOutput = time.Now()
	}
	d.mu.Unlock()

	// Update server cache
	d.server.UpdateSessionState(sessionName, result)

	// Write status file
	if err := d.statusWriter.WriteStatus(sessionName, result); err != nil {
		log.Printf("Failed to write status for %s: %v", sessionName, err)
	}
}

// cleanupStaleSessions removes monitors for sessions that no longer exist
func (d *Daemon) cleanupStaleSessions(activeSessions []string) {
	activeMap := make(map[string]bool)
	for _, name := range activeSessions {
		activeMap[name] = true
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for sessionName := range d.sessions {
		if !activeMap[sessionName] {
			delete(d.sessions, sessionName)
			d.statusWriter.DeleteStatus(sessionName)
		}
	}
}

// waitForShutdown blocks until shutdown signal received
func (d *Daemon) waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutdown signal received")

	d.Stop()
}
