package sandbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProcessLimits configures resource limits for fork bomb detection.
type ProcessLimits struct {
	// MaxProcesses is the maximum number of child processes allowed.
	// 0 means use the default (500).
	MaxProcesses int

	// MaxProcessSpawnRate is the maximum processes spawned per second
	// before triggering a fork bomb alert. 0 means use default (50).
	MaxProcessSpawnRate int

	// PollInterval controls how often the monitor checks process counts.
	// 0 means use default (2s).
	PollInterval time.Duration
}

// DefaultProcessLimits returns sensible defaults for process monitoring.
func DefaultProcessLimits() ProcessLimits {
	return ProcessLimits{
		MaxProcesses:        500,
		MaxProcessSpawnRate: 50,
		PollInterval:        2 * time.Second,
	}
}

// ProcessMonitor watches for excessive process spawning inside a sandbox.
type ProcessMonitor struct {
	limits    ProcessLimits
	pid       int // root PID to monitor (sandbox entrypoint)
	cancel    context.CancelFunc
	mu        sync.Mutex
	running   bool
	lastCount int
	lastCheck time.Time
	onAlert   func(AlertType, string) // callback on alert
}

// AlertType classifies the kind of process alert.
type AlertType int

const (
	// AlertForkBomb indicates a likely fork bomb (rapid spawning).
	AlertForkBomb AlertType = iota
	// AlertProcessLimit indicates the process count exceeded the limit.
	AlertProcessLimit
)

func (a AlertType) String() string {
	switch a {
	case AlertForkBomb:
		return "fork_bomb"
	case AlertProcessLimit:
		return "process_limit"
	default:
		return "unknown"
	}
}

// NewProcessMonitor creates a monitor for the given root PID.
func NewProcessMonitor(pid int, limits ProcessLimits, onAlert func(AlertType, string)) *ProcessMonitor {
	if limits.MaxProcesses <= 0 {
		limits.MaxProcesses = DefaultProcessLimits().MaxProcesses
	}
	if limits.MaxProcessSpawnRate <= 0 {
		limits.MaxProcessSpawnRate = DefaultProcessLimits().MaxProcessSpawnRate
	}
	if limits.PollInterval <= 0 {
		limits.PollInterval = DefaultProcessLimits().PollInterval
	}
	return &ProcessMonitor{
		limits:  limits,
		pid:     pid,
		onAlert: onAlert,
	}
}

// Start begins monitoring in a background goroutine.
// Returns immediately. Call Stop() to terminate.
func (m *ProcessMonitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.lastCheck = time.Now()
	m.mu.Unlock()

	childCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	go m.run(childCtx)
}

// Stop terminates the monitor.
func (m *ProcessMonitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	m.running = false
}

// CountDescendants returns the number of descendant processes of the root PID.
// Works by walking /proc/<pid>/task/*/children recursively on Linux.
func (m *ProcessMonitor) CountDescendants() (int, error) {
	return countDescendants(m.pid)
}

func (m *ProcessMonitor) run(ctx context.Context) {
	ticker := time.NewTicker(m.limits.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.check()
		}
	}
}

func (m *ProcessMonitor) check() {
	count, err := countDescendants(m.pid)
	if err != nil {
		return // Process tree may have exited; not an error
	}

	now := time.Now()
	m.mu.Lock()
	prev := m.lastCount
	elapsed := now.Sub(m.lastCheck).Seconds()
	m.lastCount = count
	m.lastCheck = now
	m.mu.Unlock()

	// Check absolute limit
	if count > m.limits.MaxProcesses {
		if m.onAlert != nil {
			m.onAlert(AlertProcessLimit, fmt.Sprintf(
				"process count %d exceeds limit %d for PID %d",
				count, m.limits.MaxProcesses, m.pid))
		}
		// Attempt to kill the process tree
		killProcessTree(m.pid)
		return
	}

	// Check spawn rate (fork bomb detection)
	if elapsed > 0 && prev > 0 {
		delta := count - prev
		if delta > 0 {
			rate := float64(delta) / elapsed
			if rate > float64(m.limits.MaxProcessSpawnRate) {
				if m.onAlert != nil {
					m.onAlert(AlertForkBomb, fmt.Sprintf(
						"fork bomb detected: %.0f procs/sec (limit %d) for PID %d",
						rate, m.limits.MaxProcessSpawnRate, m.pid))
				}
				killProcessTree(m.pid)
			}
		}
	}
}

// countDescendants walks /proc to count all descendants of a PID.
func countDescendants(pid int) (int, error) {
	children, err := getChildPIDs(pid)
	if err != nil {
		return 0, err
	}
	total := len(children)
	for _, child := range children {
		sub, _ := countDescendants(child)
		total += sub
	}
	return total, nil
}

// getChildPIDs reads /proc/<pid>/task/*/children to find direct children.
func getChildPIDs(pid int) ([]int, error) {
	taskDir := fmt.Sprintf("/proc/%d/task", pid)
	tasks, err := os.ReadDir(taskDir)
	if err != nil {
		return nil, err
	}

	var children []int
	for _, task := range tasks {
		childFile := filepath.Join(taskDir, task.Name(), "children")
		data, err := os.ReadFile(childFile)
		if err != nil {
			continue
		}
		for _, field := range strings.Fields(string(data)) {
			if cpid, err := strconv.Atoi(field); err == nil {
				children = append(children, cpid)
			}
		}
	}
	return children, nil
}

// killProcessTree sends SIGKILL to a PID and all its descendants.
func killProcessTree(pid int) {
	children, _ := getChildPIDs(pid)
	for _, child := range children {
		killProcessTree(child)
	}
	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = proc.Kill()
	}
}
