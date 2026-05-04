package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/daemon"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the AGM message delivery daemon",
	Long: `Control the background daemon that delivers queued messages.

The daemon polls the message queue every 30 seconds and delivers messages
when recipient sessions become READY.`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the AGM daemon",
	RunE:  runDaemonStart,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the AGM daemon",
	RunE:  runDaemonStop,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check daemon status",
	RunE:  runDaemonStatus,
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the AGM daemon",
	RunE:  runDaemonRestart,
}

var daemonHealthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check daemon health and metrics",
	RunE:  runDaemonHealth,
}

func init() {
	sessionCmd.AddCommand(daemonCmd)
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonRestartCmd)
	daemonCmd.AddCommand(daemonHealthCmd)
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	// Check if already running
	running, pid, err := isDaemonRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if running {
		fmt.Printf("Daemon already running (PID: %d)\n", pid)
		return nil
	}

	// Find agm-daemon binary
	daemonBinary, err := exec.LookPath("agm-daemon")
	if err != nil {
		return fmt.Errorf("agm-daemon binary not found in PATH: %w", err)
	}

	// Start daemon in background
	daemonCmd := exec.Command(daemonBinary)
	daemonCmd.Stdout = nil
	daemonCmd.Stderr = nil
	daemonCmd.Stdin = nil

	if err := daemonCmd.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Detach from parent process
	if err := daemonCmd.Process.Release(); err != nil {
		return fmt.Errorf("failed to release daemon process: %w", err)
	}

	fmt.Println("✓ AGM daemon started")
	fmt.Printf("  PID: %d\n", daemonCmd.Process.Pid)
	fmt.Printf("  Logs: ~/.agm/logs/daemon/daemon.log\n")

	return nil
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	running, pid, err := isDaemonRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if !running {
		fmt.Println("Daemon not running")
		return nil
	}

	// Send SIGTERM to daemon
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find daemon process: %w", err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}

	fmt.Printf("✓ Sent stop signal to daemon (PID: %d)\n", pid)
	return nil
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	running, pid, err := isDaemonRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if running {
		fmt.Printf("✓ Daemon running\n")
		fmt.Printf("  PID: %d\n", pid)
		fmt.Printf("  Logs: ~/.agm/logs/daemon/daemon.log\n")

		// Show recent log entries
		homeDir, _ := os.UserHomeDir()
		logPath := filepath.Join(homeDir, ".agm", "logs", "daemon", "daemon.log")
		if logBytes, err := exec.Command("tail", "-n", "5", logPath).Output(); err == nil {
			fmt.Printf("\nRecent log entries:\n%s", string(logBytes))
		}
	} else {
		fmt.Println("✗ Daemon not running")
	}

	return nil
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	// Stop if running
	running, _, _ := isDaemonRunning()
	if running {
		if err := runDaemonStop(cmd, args); err != nil {
			return err
		}
	}

	// Wait a moment for clean shutdown
	fmt.Println("Waiting for daemon to stop...")
	for i := 0; i < 10; i++ {
		running, _, _ := isDaemonRunning()
		if !running {
			break
		}
		exec.Command("sleep", "0.5").Run()
	}

	// Start daemon
	return runDaemonStart(cmd, args)
}

func runDaemonHealth(cmd *cobra.Command, args []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	pidFile := filepath.Join(homeDir, ".agm", "daemon.pid")

	// Open message queue to get stats
	queue, err := messages.NewMessageQueue()
	if err != nil {
		return fmt.Errorf("failed to open message queue: %w", err)
	}
	defer queue.Close()

	// Get health status
	health, err := daemon.GetHealthStatus(pidFile, queue)
	if err != nil {
		return fmt.Errorf("failed to get health status: %w", err)
	}

	// Display health information
	fmt.Printf("=== AGM Daemon Health Status ===\n\n")

	// Overall status
	statusSymbol := "✓"
	switch health.HealthStatusLevel {
	case "critical":
		statusSymbol = "✗"
	case "degraded":
		statusSymbol = "⚠"
	}

	fmt.Printf("%s Overall Status: %s\n", statusSymbol, strings.ToUpper(health.HealthStatusLevel))
	fmt.Printf("  Daemon Running: %v\n", health.Running)
	if health.Running {
		fmt.Printf("  PID: %d\n", health.PID)
	}

	// Queue statistics
	if health.QueueStats != nil {
		fmt.Printf("\n=== Queue Statistics ===\n")
		if queued, ok := health.QueueStats["queued"]; ok {
			fmt.Printf("  Queued Messages: %d\n", queued)
		}
		if delivered, ok := health.QueueStats["delivered"]; ok {
			fmt.Printf("  Delivered Messages: %d\n", delivered)
		}
		if failed, ok := health.QueueStats["failed"]; ok {
			fmt.Printf("  Failed Messages: %d\n", failed)
		}
	}

	// Recommendations
	fmt.Printf("\n=== Recommendations ===\n")
	switch {
	case !health.Running:
		fmt.Printf("  - Daemon is not running. Start it with: agm session daemon start\n")
	case health.HealthStatusLevel == "critical":
		fmt.Printf("  - Queue depth is critically high (>100). Check daemon logs.\n")
		fmt.Printf("  - Review failed messages with: agm queue dlq\n")
	case health.HealthStatusLevel == "degraded":
		fmt.Printf("  - Queue depth is elevated (>50). Monitor closely.\n")
	default:
		fmt.Printf("  - System is operating normally.\n")
	}

	fmt.Printf("\nLogs: %s\n", filepath.Join(homeDir, ".agm", "logs", "daemon", "daemon.log"))

	return nil
}

// isDaemonRunning checks if the daemon is currently running
func isDaemonRunning() (bool, int, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false, 0, err
	}

	pidFile := filepath.Join(homeDir, ".agm", "daemon.pid")
	pidBytes, err := os.ReadFile(pidFile)
	if os.IsNotExist(err) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}

	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(pidBytes)), "%d", &pid); err != nil {
		return false, 0, fmt.Errorf("invalid PID file: %w", err)
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, 0, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Send signal 0 to check if process is alive
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false, 0, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	return true, pid, nil
}
