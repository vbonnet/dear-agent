package deadlock

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ProcessInfo contains information about a potentially deadlocked process
type ProcessInfo struct {
	PID         int
	Command     string
	State       string
	CPUPercent  float64
	Runtime     time.Duration
	RawStat     string
	IsDeadlock  bool
}

// DetectClaudeDeadlock finds the Claude process in a tmux session and checks if it's deadlocked
func DetectClaudeDeadlock(tmuxSessionName string) (*ProcessInfo, error) {
	// Step 1: Get the PID of the tmux pane
	panePID, err := getTmuxPanePID(tmuxSessionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get tmux pane PID: %w", err)
	}

	// Step 2: Find the Claude process (child of the pane shell)
	claudePID, err := findClaudeProcess(panePID)
	if err != nil {
		return nil, fmt.Errorf("failed to find Claude process: %w", err)
	}

	// Step 3: Get process information
	info, err := getProcessInfo(claudePID)
	if err != nil {
		return nil, fmt.Errorf("failed to get process info: %w", err)
	}

	// Step 4: Check for deadlock criteria
	info.IsDeadlock = isDeadlocked(info)

	return info, nil
}

// getTmuxPanePID gets the PID of the shell running in a tmux pane
func getTmuxPanePID(tmuxSessionName string) (int, error) {
	// Get socket path (default Claude socket)
	socketPath := os.Getenv("TMUX_SOCKET")
	if socketPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return 0, fmt.Errorf("failed to get home directory: %w", err)
		}
		socketPath = home + "/.claude/tmux.sock"
	}

	// Run: tmux list-panes -t <session> -F "#{pane_pid}"
	cmd := exec.Command("tmux", "-S", socketPath, "list-panes", "-t", tmuxSessionName, "-F", "#{pane_pid}")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("tmux list-panes failed: %w", err)
	}

	pidStr := strings.TrimSpace(string(output))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("invalid PID from tmux: %s", pidStr)
	}

	return pid, nil
}

// findClaudeProcess finds the Claude Code process (child of shell in tmux pane)
func findClaudeProcess(panePID int) (int, error) {
	// Use pgrep to find processes with "claude" in the command that are descendants
	// of the pane PID
	cmd := exec.Command("pgrep", "-P", strconv.Itoa(panePID), "-f", "claude")
	output, err := cmd.Output()
	if err != nil {
		// pgrep returns exit code 1 if no processes found
		return 0, fmt.Errorf("no Claude process found (pane PID %d may not have Claude running)", panePID)
	}

	// Parse PID (take first line if multiple)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return 0, fmt.Errorf("no Claude process found")
	}

	pid, err := strconv.Atoi(lines[0])
	if err != nil {
		return 0, fmt.Errorf("invalid PID from pgrep: %s", lines[0])
	}

	return pid, nil
}

// getProcessInfo retrieves detailed process information
func getProcessInfo(pid int) (*ProcessInfo, error) {
	info := &ProcessInfo{
		PID: pid,
	}

	// Read /proc/<pid>/stat for process state and CPU time
	statPath := fmt.Sprintf("/proc/%d/stat", pid)
	data, err := os.ReadFile(statPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", statPath, err)
	}

	info.RawStat = string(data)

	// Parse /proc/<pid>/stat
	// Format: pid (comm) state ppid ... utime stime ...
	// We need: state (field 3), utime (field 14), stime (field 15)
	fields := strings.Fields(string(data))
	if len(fields) < 15 {
		return nil, fmt.Errorf("invalid /proc/%d/stat format", pid)
	}

	// Extract state (field 3, after comm which is in parentheses)
	info.State = fields[2]

	// Extract command name (field 2, in parentheses)
	commStart := strings.Index(string(data), "(")
	commEnd := strings.LastIndex(string(data), ")")
	if commStart != -1 && commEnd != -1 && commEnd > commStart {
		info.Command = string(data[commStart+1 : commEnd])
	}

	// Calculate CPU percentage (simplified)
	// Read system uptime
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/uptime: %w", err)
	}
	uptimeFields := strings.Fields(string(uptimeData))
	if len(uptimeFields) < 1 {
		return nil, fmt.Errorf("invalid /proc/uptime format")
	}
	systemUptime, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse uptime: %w", err)
	}

	// Get process start time (field 22)
	if len(fields) >= 22 {
		starttime, err := strconv.ParseInt(fields[21], 10, 64)
		if err == nil {
			// starttime is in clock ticks since system boot
			clockTicks := int64(100) // Hz (usually 100 on Linux)
			processStartSeconds := float64(starttime) / float64(clockTicks)
			processRuntime := systemUptime - processStartSeconds
			info.Runtime = time.Duration(processRuntime * float64(time.Second))
		}
	}

	// Calculate CPU percentage using ps
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "%cpu", "--no-headers")
	output, err := cmd.Output()
	if err == nil {
		cpuStr := strings.TrimSpace(string(output))
		cpu, err := strconv.ParseFloat(cpuStr, 64)
		if err == nil {
			info.CPUPercent = cpu
		}
	}

	return info, nil
}

// isDeadlocked checks if a process meets deadlock criteria
func isDeadlocked(info *ProcessInfo) bool {
	// Deadlock criteria based on ROADMAP-STAGE-1.md:
	// - State: RNl+ (R = running, N = low priority, l = multi-threaded, + = foreground)
	// - CPU: > 25%
	// - Runtime: > 5 minutes

	// Check state for "R" (running/runnable)
	if !strings.HasPrefix(info.State, "R") {
		return false
	}

	// Check CPU percentage
	if info.CPUPercent <= 25.0 {
		return false
	}

	// Check runtime
	if info.Runtime < 5*time.Minute {
		return false
	}

	return true
}

// FormatProcessInfo returns a human-readable summary of process info
func FormatProcessInfo(info *ProcessInfo) string {
	return fmt.Sprintf(`Process Information:
  PID:        %d
  Command:    %s
  State:      %s
  CPU:        %.1f%%
  Runtime:    %s
  Deadlock:   %v`,
		info.PID,
		info.Command,
		info.State,
		info.CPUPercent,
		formatDuration(info.Runtime),
		info.IsDeadlock,
	)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm %.0fs", d.Minutes(), d.Seconds()-60*d.Minutes())
	}
	return fmt.Sprintf("%.0fh %.0fm", d.Hours(), d.Minutes()-60*d.Hours())
}

// LogDeadlockIncident appends a deadlock incident to ~/deadlock-log.txt
func LogDeadlockIncident(sessionName string, info *ProcessInfo) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	logPath := home + "/deadlock-log.txt"

	// Open file for appending (create if doesn't exist)
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)

	// Write log entry
	timestamp := time.Now().Format(time.RFC3339)
	_, err = writer.WriteString(fmt.Sprintf(`
================================================================================
Deadlock Incident: %s
================================================================================
Session:    %s
PID:        %d
Command:    %s
State:      %s
CPU:        %.1f%%
Runtime:    %s
Timestamp:  %s
Action:     SIGKILL sent
================================================================================

`, timestamp, sessionName, info.PID, info.Command, info.State, info.CPUPercent, formatDuration(info.Runtime), timestamp))
	if err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush log: %w", err)
	}

	return nil
}
