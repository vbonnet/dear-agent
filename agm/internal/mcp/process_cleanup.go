package mcp

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ProcessInfo holds metadata about a discovered OS process.
type ProcessInfo struct {
	PID     int
	CmdLine string
}

// ProcessFinder finds OS processes matching criteria.
type ProcessFinder interface {
	FindByCommandLine(substring string) ([]ProcessInfo, error)
}

// ProcessKiller kills an OS process.
type ProcessKiller interface {
	Kill(pid int) error
}

// ProcFSFinder scans /proc/*/cmdline to find processes by command line content.
type ProcFSFinder struct{}

// FindByCommandLine scans /proc for processes whose cmdline contains substring.
func (f *ProcFSFinder) FindByCommandLine(substring string) ([]ProcessInfo, error) {
	if substring == "" {
		return nil, nil
	}

	entries, err := filepath.Glob("/proc/[0-9]*/cmdline")
	if err != nil {
		return nil, fmt.Errorf("failed to glob /proc: %w", err)
	}

	var results []ProcessInfo
	for _, entry := range entries {
		data, err := os.ReadFile(entry)
		if err != nil {
			continue // process may have exited
		}
		cmdline := string(data)
		if !strings.Contains(cmdline, substring) {
			continue
		}

		// Extract PID from path: /proc/<pid>/cmdline
		parts := strings.Split(entry, "/")
		if len(parts) < 3 {
			continue
		}
		pid, err := strconv.Atoi(parts[2])
		if err != nil {
			continue
		}

		results = append(results, ProcessInfo{
			PID:     pid,
			CmdLine: strings.ReplaceAll(cmdline, "\x00", " "),
		})
	}

	return results, nil
}

// SignalKiller kills a process using SIGTERM then SIGKILL after a grace period.
type SignalKiller struct{}

// Kill sends SIGTERM, waits 5 seconds, then sends SIGKILL if still alive.
func (k *SignalKiller) Kill(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil // process doesn't exist
	}

	// Check if process is alive
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return nil // already dead
	}

	// Send SIGTERM
	if err := process.Signal(syscall.SIGTERM); err != nil {
		if strings.Contains(err.Error(), "process already finished") {
			return nil
		}
		return fmt.Errorf("SIGTERM failed for PID %d: %w", pid, err)
	}

	// Wait up to 5 seconds for graceful exit
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			if err := process.Signal(syscall.Signal(0)); err != nil {
				close(done)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		close(done)
	}()

	select {
	case <-done:
		// Check if actually dead
		if err := process.Signal(syscall.Signal(0)); err != nil {
			return nil // graceful exit
		}
	case <-time.After(5 * time.Second):
	}

	// Still alive — SIGKILL
	if err := process.Signal(syscall.SIGKILL); err != nil {
		if strings.Contains(err.Error(), "process already finished") {
			return nil
		}
		return fmt.Errorf("SIGKILL failed for PID %d: %w", pid, err)
	}

	return nil
}

// CleanupSessionMCPProcesses finds and kills MCP processes associated with a session.
// It searches by sandbox path (primary) or session ID (fallback).
// Returns the number of processes killed. Errors are logged but do not fail the operation.
func CleanupSessionMCPProcesses(finder ProcessFinder, killer ProcessKiller, sessionID, sandboxPath string) (int, error) {
	logger := slog.Default()

	searchTerm := sandboxPath
	if searchTerm == "" {
		searchTerm = sessionID
	}
	if searchTerm == "" {
		return 0, nil
	}

	procs, err := finder.FindByCommandLine(searchTerm)
	if err != nil {
		return 0, fmt.Errorf("failed to find MCP processes: %w", err)
	}

	if len(procs) == 0 {
		return 0, nil
	}

	selfPID := os.Getpid()
	killed := 0
	for _, p := range procs {
		if p.PID == selfPID {
			continue
		}
		logger.Info("Killing MCP process", "pid", p.PID, "cmdline_prefix", truncate(p.CmdLine, 120))
		if err := killer.Kill(p.PID); err != nil {
			logger.Warn("Failed to kill MCP process", "pid", p.PID, "error", err)
			continue
		}
		killed++
	}

	return killed, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
