// Package coordinator manages concurrent execution of wayfinder projects
package coordinator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Coordinator manages concurrent execution of wayfinder projects
type Coordinator struct {
	maxConcurrent int
	semaphore     chan struct{}
	wg            sync.WaitGroup
	projects      map[string]*ProjectExecution
	mu            sync.RWMutex
	monitor       *Monitor
	sandboxMgr    SandboxManager
}

// SandboxManager interface for sandbox operations (from wp11)
type SandboxManager interface {
	CreateSandbox(name string) (*Sandbox, error)
	ListSandboxes() ([]*Sandbox, error)
	CleanupSandbox(nameOrID string) error
}

// Sandbox represents an isolated wayfinder project environment
type Sandbox struct {
	ID            string
	Name          string
	WorktreePath  string
	GitRepository string
}

// ProjectExecution tracks a single project's execution state
type ProjectExecution struct {
	ProjectDir  string
	SandboxID   string
	Status      ExecutionStatus
	StartedAt   time.Time
	CompletedAt time.Time
	Error       error
	Process     *exec.Cmd
	mu          sync.RWMutex
}

// ExecutionStatus represents project execution states
type ExecutionStatus string

const (
	StatusQueued    ExecutionStatus = "queued"
	StatusRunning   ExecutionStatus = "running"
	StatusCompleted ExecutionStatus = "completed"
	StatusFailed    ExecutionStatus = "failed"
	StatusCancelled ExecutionStatus = "cancelled"
)

// Config holds coordinator configuration
type Config struct {
	MaxConcurrent   int
	SandboxDir      string
	MonitorInterval time.Duration
	NoSandbox       bool
}

// DefaultConfig returns default coordinator configuration
func DefaultConfig() Config {
	return Config{
		MaxConcurrent:   4,
		SandboxDir:      filepath.Join(os.Getenv("HOME"), ".wayfinder", "sandboxes"),
		MonitorInterval: 10 * time.Second,
		NoSandbox:       false,
	}
}

// NewCoordinator creates a new coordinator with config
func NewCoordinator(cfg Config, sandboxMgr SandboxManager) *Coordinator {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 4
	}
	if cfg.MonitorInterval <= 0 {
		cfg.MonitorInterval = 10 * time.Second
	}

	return &Coordinator{
		maxConcurrent: cfg.MaxConcurrent,
		semaphore:     make(chan struct{}, cfg.MaxConcurrent),
		projects:      make(map[string]*ProjectExecution),
		monitor:       NewMonitor(cfg.MonitorInterval, filepath.Join(cfg.SandboxDir, "logs")),
		sandboxMgr:    sandboxMgr,
	}
}

// Start executes multiple projects concurrently
func (c *Coordinator) Start(ctx context.Context, projectDirs []string) error {
	if len(projectDirs) == 0 {
		return fmt.Errorf("no project directories specified")
	}

	// Initialize projects
	for _, dir := range projectDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return fmt.Errorf("invalid project directory %s: %w", dir, err)
		}

		c.mu.Lock()
		c.projects[absDir] = &ProjectExecution{
			ProjectDir: absDir,
			Status:     StatusQueued,
		}
		c.mu.Unlock()
	}

	// Start monitor
	c.monitor.Start(ctx, projectDirs)

	// Start all projects (goroutines will queue via semaphore)
	for _, dir := range projectDirs {
		c.wg.Add(1)
		go c.runProject(ctx, dir)
	}

	// Wait for all projects to complete
	c.wg.Wait()

	// Check for errors
	var errors []error
	c.mu.RLock()
	for _, proj := range c.projects {
		if proj.Error != nil {
			errors = append(errors, fmt.Errorf("%s: %w", proj.ProjectDir, proj.Error))
		}
	}
	c.mu.RUnlock()

	if len(errors) > 0 {
		return fmt.Errorf("some projects failed: %v", errors)
	}

	return nil
}

// Status returns current status of all projects
func (c *Coordinator) Status() map[string]*ProjectExecution {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return copy to prevent race conditions
	status := make(map[string]*ProjectExecution)
	for k, v := range c.projects {
		v.mu.RLock()
		projCopy := &ProjectExecution{
			ProjectDir:  v.ProjectDir,
			SandboxID:   v.SandboxID,
			Status:      v.Status,
			StartedAt:   v.StartedAt,
			CompletedAt: v.CompletedAt,
			Error:       v.Error,
		}
		v.mu.RUnlock()
		status[k] = projCopy
	}

	return status
}

// Stop gracefully stops all running projects
func (c *Coordinator) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Send SIGTERM to all running processes
	for _, proj := range c.projects {
		proj.mu.Lock()
		if proj.Process != nil && proj.Status == StatusRunning {
			if proj.Process.Process != nil {
				proj.Process.Process.Signal(syscall.SIGTERM)
			}
			proj.Status = StatusCancelled
		}
		proj.mu.Unlock()
	}

	// Wait for graceful shutdown (up to 10s)
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-shutdownCtx.Done():
		// Force kill remaining processes
		for _, proj := range c.projects {
			proj.mu.Lock()
			if proj.Process != nil && proj.Status == StatusRunning {
				if proj.Process.Process != nil {
					proj.Process.Process.Kill()
				}
			}
			proj.mu.Unlock()
		}
		return fmt.Errorf("forced shutdown after timeout")
	}
}

// runProject executes a single project (internal)
func (c *Coordinator) runProject(ctx context.Context, projectDir string) error {
	defer c.wg.Done()

	// Acquire semaphore (blocks until slot available)
	c.semaphore <- struct{}{}
	defer func() { <-c.semaphore }()

	absDir, _ := filepath.Abs(projectDir)

	// Update status to running
	c.updateProjectStatus(absDir, StatusRunning, nil)
	c.monitor.Emit(Event{
		Type:       EventProjectStarted,
		ProjectDir: absDir,
		Timestamp:  time.Now(),
	})

	// Get or create sandbox (hybrid approach)
	var sb *Sandbox
	var err error
	if c.sandboxMgr != nil {
		sb, err = c.getOrCreateSandbox(absDir)
		if err != nil {
			// Fallback: continue without sandbox
			fmt.Fprintf(os.Stderr, "Warning: sandbox creation failed for %s, running without isolation: %v\n", absDir, err)
			sb = nil
		}
	}

	// Update sandbox ID
	if sb != nil {
		c.mu.Lock()
		if proj, ok := c.projects[absDir]; ok {
			proj.mu.Lock()
			proj.SandboxID = sb.ID
			proj.mu.Unlock()
		}
		c.mu.Unlock()
	}

	// Spawn wayfinder-session process
	cmd := exec.CommandContext(ctx, "wayfinder-session", "start", absDir)

	// Set environment variables if sandboxed
	if sb != nil {
		sandboxPath := filepath.Join(os.Getenv("HOME"), ".wayfinder", "sandboxes", sb.ID)
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("WAYFINDER_SANDBOX=%s", sb.ID),
			fmt.Sprintf("WAYFINDER_SANDBOX_PATH=%s", sandboxPath),
		)
		if sb.WorktreePath != "" {
			cmd.Env = append(cmd.Env,
				fmt.Sprintf("WAYFINDER_WORKTREE_PATH=%s", sb.WorktreePath),
			)
			cmd.Dir = sb.WorktreePath // Run in worktree directory
		}
	}

	// Capture stdout/stderr
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	// Start log streaming
	projectID := filepath.Base(absDir)
	if sb != nil {
		projectID = sb.ID
	}
	go c.monitor.StreamLogs(projectID, stdout, stderr)

	// Store process reference
	c.mu.Lock()
	if proj, ok := c.projects[absDir]; ok {
		proj.mu.Lock()
		proj.Process = cmd
		proj.StartedAt = time.Now()
		proj.mu.Unlock()
	}
	c.mu.Unlock()

	// Run process
	err = cmd.Run()

	// Update completion status
	if err != nil {
		c.updateProjectStatus(absDir, StatusFailed, err)
		c.monitor.Emit(Event{
			Type:       EventProjectFailed,
			ProjectDir: absDir,
			Timestamp:  time.Now(),
			Error:      err,
		})
		return err
	}

	c.updateProjectStatus(absDir, StatusCompleted, nil)
	c.monitor.Emit(Event{
		Type:       EventProjectCompleted,
		ProjectDir: absDir,
		Timestamp:  time.Now(),
	})

	return nil
}

// getOrCreateSandbox hybrid sandbox integration (internal)
func (c *Coordinator) getOrCreateSandbox(projectDir string) (*Sandbox, error) {
	projectName := filepath.Base(projectDir)

	// Check if sandbox exists
	sandboxes, err := c.sandboxMgr.ListSandboxes()
	if err != nil {
		return nil, fmt.Errorf("failed to list sandboxes: %w", err)
	}

	for _, sb := range sandboxes {
		if sb.Name == projectName {
			// Reuse existing sandbox
			return sb, nil
		}
	}

	// Create new sandbox
	sb, err := c.sandboxMgr.CreateSandbox(projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}

	return sb, nil
}

// updateProjectStatus updates project status thread-safely (internal)
func (c *Coordinator) updateProjectStatus(projectDir string, status ExecutionStatus, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if proj, ok := c.projects[projectDir]; ok {
		proj.mu.Lock()
		proj.Status = status
		proj.Error = err
		if status == StatusCompleted || status == StatusFailed || status == StatusCancelled {
			proj.CompletedAt = time.Now()
		}
		proj.mu.Unlock()
	}
}

// GetMonitor returns the monitor instance for event subscription
func (c *Coordinator) GetMonitor() *Monitor {
	return c.monitor
}
