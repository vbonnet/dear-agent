// Package deadlock provides deadlock functionality.
package deadlock

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ProcessInfo contains information about a Claude process
type ProcessInfo struct {
	PID         int
	CPU         float64
	RuntimeSec  int
	State       string
	WCHAN       string
	IsDeadlock  bool
	Command     string
	Connections int
}

// Deadlock detection thresholds (from ROADMAP-STAGE-1.md)
const (
	MinCPUPercent     = 25.0 // Processes using >25% CPU
	MinRuntimeMinutes = 5    // Running for >5 minutes
)

// DetectClaudeDeadlock detects if the Claude process in a tmux session is deadlocked
func DetectClaudeDeadlock(tmuxSessionName string) (*ProcessInfo, error) {
	// Step 1: Find Claude process PID from tmux session
	pid, err := getClaudePIDFromTmux(tmuxSessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to find Claude process: %w", err)
	}

	// Step 2: Get process information
	info, err := getProcessInfo(pid)
	if err != nil {
		return nil, fmt.Errorf("failed to get process info: %w", err)
	}

	// Step 3: Check deadlock criteria
	runtimeMinutes := info.RuntimeSec / 60
	info.IsDeadlock = (info.CPU >= MinCPUPercent) &&
		(runtimeMinutes >= MinRuntimeMinutes) &&
		(strings.HasPrefix(info.State, "R")) // Running/Runnable state

	return info, nil
}

// getClaudePIDFromTmux finds the Claude process PID running in a tmux session
func getClaudePIDFromTmux(tmuxSessionName string) (int, error) {
	// Get tmux pane PID
	cmd := exec.Command("tmux", "list-panes", "-t", tmuxSessionName, "-F", "#{pane_pid}")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("tmux list-panes failed: %w", err)
	}

	panePID, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		return 0, fmt.Errorf("invalid pane PID: %w", err)
	}

	// Find Claude process (child of pane)
	// Look for process named "claude" or "node" with "claude" in cmdline
	claudePID, err := findClaudeProcess(panePID)
	if err != nil {
		return 0, err
	}

	return claudePID, nil
}

// findClaudeProcess finds the Claude node process (child of tmux pane)
func findClaudeProcess(parentPID int) (int, error) {
	// ps -o pid,ppid,comm --no-headers | awk '$2 == <parentPID> && ($3 ~ /node/ || $3 ~ /claude/)'
	cmd := exec.Command("ps", "-o", "pid,ppid,comm", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ps command failed: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}

		if ppid == parentPID {
			comm := fields[2]
			if comm == "node" || comm == "claude" {
				pid, err := strconv.Atoi(fields[0])
				if err != nil {
					continue
				}
				return pid, nil
			}
		}
	}

	return 0, fmt.Errorf("no Claude process found under pane PID %d", parentPID)
}

// getProcessInfo retrieves detailed information about a process
func getProcessInfo(pid int) (*ProcessInfo, error) {
	info := &ProcessInfo{PID: pid}

	// Get CPU%, TIME, STATE from ps
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "pcpu,time,stat,wchan:30,cmd", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps command failed: %w", err)
	}

	fields := strings.Fields(string(output))
	if len(fields) < 4 {
		return nil, fmt.Errorf("unexpected ps output format")
	}

	// Parse CPU%
	cpuStr := fields[0]
	cpu, err := strconv.ParseFloat(cpuStr, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CPU: %w", err)
	}
	info.CPU = cpu

	// Parse TIME (format: MM:SS.ss or HH:MM:SS)
	timeStr := fields[1]
	runtimeSec, err := parseTime(timeStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse runtime: %w", err)
	}
	info.RuntimeSec = runtimeSec

	// Parse STATE
	info.State = fields[2]

	// Parse WCHAN
	info.WCHAN = fields[3]

	// Parse COMMAND (rest of fields)
	info.Command = strings.Join(fields[4:], " ")

	// Count network connections (lsof)
	connections, _ := countConnections(pid) // Ignore errors
	info.Connections = connections

	return info, nil
}

// parseTime converts ps TIME format (MM:SS.ss or HH:MM:SS) to seconds
func parseTime(timeStr string) (int, error) {
	parts := strings.Split(timeStr, ":")

	if len(parts) == 2 {
		// MM:SS.ss format
		minutes, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, err
		}

		// Handle seconds with decimal (e.g., "45.12")
		secondsStr := strings.Split(parts[1], ".")[0]
		seconds, err := strconv.Atoi(secondsStr)
		if err != nil {
			return 0, err
		}

		return minutes*60 + seconds, nil
	} else if len(parts) == 3 {
		// HH:MM:SS format
		hours, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, err
		}

		minutes, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, err
		}

		seconds, err := strconv.Atoi(parts[2])
		if err != nil {
			return 0, err
		}

		return hours*3600 + minutes*60 + seconds, nil
	}

	return 0, fmt.Errorf("unexpected time format: %s", timeStr)
}

// countConnections counts network connections for a process
func countConnections(pid int) (int, error) {
	cmd := exec.Command("lsof", "-p", strconv.Itoa(pid), "-a", "-i")
	output, err := cmd.Output()
	if err != nil {
		// lsof may fail if no connections, that's OK
		return 0, nil
	}

	lines := strings.Split(string(output), "\n")
	// Subtract 1 for header line, count non-empty lines
	count := 0
	for i, line := range lines {
		if i == 0 || line == "" {
			continue
		}
		count++
	}

	return count, nil
}

// FormatProcessInfo formats process information for display
func FormatProcessInfo(info *ProcessInfo) string {
	var b strings.Builder

	runtimeMinutes := info.RuntimeSec / 60

	fmt.Fprintf(&b, "Process Information:\n")
	fmt.Fprintf(&b, "  PID:         %d\n", info.PID)
	fmt.Fprintf(&b, "  CPU:         %.1f%%\n", info.CPU)
	fmt.Fprintf(&b, "  Runtime:     %dm (%ds)\n", runtimeMinutes, info.RuntimeSec)
	fmt.Fprintf(&b, "  State:       %s\n", info.State)
	fmt.Fprintf(&b, "  WCHAN:       %s\n", info.WCHAN)
	fmt.Fprintf(&b, "  Connections: %d\n", info.Connections)
	fmt.Fprintf(&b, "  Command:     %s\n", info.Command)
	fmt.Fprintf(&b, "\n")

	if info.IsDeadlock {
		fmt.Fprintf(&b, "⚠️  DEADLOCK DETECTED\n")
		fmt.Fprintf(&b, "\nDeadlock criteria met:\n")
		fmt.Fprintf(&b, "  ✓ CPU > %d%% (%.1f%%)\n", int(MinCPUPercent), info.CPU)
		fmt.Fprintf(&b, "  ✓ Runtime > %dm (%dm)\n", MinRuntimeMinutes, runtimeMinutes)
		fmt.Fprintf(&b, "  ✓ State: R (running/runnable)\n")
	} else {
		fmt.Fprintf(&b, "ℹ️  No deadlock detected\n")
		fmt.Fprintf(&b, "\nDeadlock criteria:\n")

		if info.CPU >= MinCPUPercent {
			fmt.Fprintf(&b, "  ✓ CPU > %d%% (%.1f%%)\n", int(MinCPUPercent), info.CPU)
		} else {
			fmt.Fprintf(&b, "  ✗ CPU > %d%% (%.1f%% - below threshold)\n", int(MinCPUPercent), info.CPU)
		}

		if runtimeMinutes >= MinRuntimeMinutes {
			fmt.Fprintf(&b, "  ✓ Runtime > %dm (%dm)\n", MinRuntimeMinutes, runtimeMinutes)
		} else {
			fmt.Fprintf(&b, "  ✗ Runtime > %dm (%dm - below threshold)\n", MinRuntimeMinutes, runtimeMinutes)
		}

		if strings.HasPrefix(info.State, "R") {
			fmt.Fprintf(&b, "  ✓ State: R (running/runnable)\n")
		} else {
			fmt.Fprintf(&b, "  ✗ State: R (current: %s)\n", info.State)
		}
	}

	return b.String()
}

// LogDeadlockIncident logs a deadlock incident to ~/deadlock-log.txt
func LogDeadlockIncident(sessionName string, info *ProcessInfo) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	logPath := filepath.Join(homeDir, "deadlock-log.txt")

	// Open file in append mode, create if doesn't exist
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	// Format: timestamp | session | PID | CPU% | runtime | state | WCHAN
	timestamp := time.Now().Format(time.RFC3339)
	runtimeMinutes := info.RuntimeSec / 60

	logLine := fmt.Sprintf("%s | session=%s | pid=%d | cpu=%.1f%% | runtime=%dm | state=%s | wchan=%s\n",
		timestamp, sessionName, info.PID, info.CPU, runtimeMinutes, info.State, info.WCHAN)

	if _, err := f.WriteString(logLine); err != nil {
		return fmt.Errorf("failed to write log: %w", err)
	}

	return nil
}
