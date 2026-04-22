package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/monitoring"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var heartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Manage loop heartbeats",
	Long: `Write, check, list, and query loop heartbeat status.

Loop heartbeats are written by monitoring loops (e.g., /loop 5m /orchestrate)
to signal liveness. The daemon checks these for staleness and wakes loops
that have stopped.

Available Commands:
  write   Write a heartbeat for the current loop cycle
  check   Check freshness of a session's loop heartbeat
  list    List all loop heartbeats
  status  Show overall heartbeat status`,
}

var heartbeatIntervalSecs int
var heartbeatCycleNumber int
var heartbeatMaxAge time.Duration
var heartbeatRestartCmd string

var heartbeatWriteCmd = &cobra.Command{
	Use:   "write <session>",
	Short: "Write a loop heartbeat",
	Long: `Write a heartbeat file for the given session's monitoring loop.
Called at the start of each loop cycle.

Examples:
  agm heartbeat write orchestrator-v2 --interval 300
  agm heartbeat write meta-orchestrator --interval 300 --cycle 42`,
	Args: cobra.ExactArgs(1),
	RunE: runHeartbeatWrite,
}

var heartbeatCheckCmd = &cobra.Command{
	Use:   "check <session>",
	Short: "Check a loop heartbeat's freshness",
	Long: `Check whether a session's loop heartbeat is fresh, warning, or stale.

If --max-age is set, uses that duration as the staleness threshold.
Otherwise falls back to the interval-based threshold (interval + 60s).

Exit codes:
  0 = ok (heartbeat is fresh)
  1 = stale (heartbeat expired, when --max-age is set)
  2 = stale (heartbeat expired, interval-based)

Examples:
  agm heartbeat check orchestrator-v2
  agm heartbeat check orchestrator-v2 --max-age 20m`,
	Args: cobra.ExactArgs(1),
	RunE: runHeartbeatCheck,
}

var heartbeatWatchdogCmd = &cobra.Command{
	Use:   "watchdog <session>",
	Short: "Monitor heartbeat and auto-restart if stale",
	Long: `Run a watchdog that polls a session's heartbeat and executes a restart
command when the heartbeat becomes stale.

The watchdog checks every 30 seconds (or max-age/4, whichever is smaller).
If the heartbeat is older than --max-age, it runs --restart-cmd via sh -c.

Examples:
  agm heartbeat watchdog orchestrator-v2 --max-age 20m --restart-cmd "agm session resume orchestrator-v2"`,
	Args: cobra.ExactArgs(1),
	RunE: runHeartbeatWatchdog,
}

var heartbeatListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all loop heartbeats",
	Long: `Show all loop heartbeat files with their status.

Examples:
  agm heartbeat list`,
	RunE: runHeartbeatList,
}

var heartbeatStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show overall heartbeat status",
	Long: `Show a summary of all loop heartbeats and their health.

Examples:
  agm heartbeat status`,
	RunE: runHeartbeatStatus,
}

func init() {
	rootCmd.AddCommand(heartbeatCmd)
	heartbeatCmd.AddCommand(heartbeatWriteCmd)
	heartbeatCmd.AddCommand(heartbeatCheckCmd)
	heartbeatCmd.AddCommand(heartbeatListCmd)
	heartbeatCmd.AddCommand(heartbeatStatusCmd)
	heartbeatCmd.AddCommand(heartbeatWatchdogCmd)

	heartbeatWriteCmd.Flags().IntVar(&heartbeatIntervalSecs, "interval", 300, "Loop interval in seconds")
	heartbeatWriteCmd.Flags().IntVar(&heartbeatCycleNumber, "cycle", 0, "Current cycle number")

	heartbeatCheckCmd.Flags().DurationVar(&heartbeatMaxAge, "max-age", 0, "Maximum age before heartbeat is considered stale (e.g., 20m)")

	heartbeatWatchdogCmd.Flags().DurationVar(&heartbeatMaxAge, "max-age", 20*time.Minute, "Maximum age before heartbeat is considered stale")
	heartbeatWatchdogCmd.Flags().StringVar(&heartbeatRestartCmd, "restart-cmd", "", "Command to execute when heartbeat is stale")
	_ = heartbeatWatchdogCmd.MarkFlagRequired("restart-cmd")
}

func runHeartbeatWrite(_ *cobra.Command, args []string) error {
	session := args[0]

	writer, err := monitoring.NewHeartbeatWriter("")
	if err != nil {
		return fmt.Errorf("failed to create heartbeat writer: %w", err)
	}

	if err := writer.Write(session, heartbeatIntervalSecs, heartbeatCycleNumber, true); err != nil {
		return fmt.Errorf("failed to write heartbeat: %w", err)
	}

	fmt.Printf("Heartbeat written for %s (interval=%ds, cycle=%d)\n", session, heartbeatIntervalSecs, heartbeatCycleNumber)
	return nil
}

func runHeartbeatCheck(_ *cobra.Command, args []string) error {
	session := args[0]

	hb, err := monitoring.ReadHeartbeat("", session)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("No heartbeat found for %s\n", session)
			if heartbeatMaxAge > 0 {
				os.Exit(1)
			}
			os.Exit(2)
		}
		return fmt.Errorf("failed to read heartbeat: %w", err)
	}

	age := time.Since(hb.Timestamp)
	var status string
	if heartbeatMaxAge > 0 {
		status = monitoring.CheckStalenessWithMaxAge(hb, heartbeatMaxAge)
	} else {
		status = monitoring.CheckStaleness(hb)
	}

	fmt.Printf("Session:  %s\n", ui.Bold(hb.Session))
	fmt.Printf("Age:      %s\n", age.Round(time.Second))
	fmt.Printf("Interval: %ds\n", hb.IntervalSecs)
	fmt.Printf("Cycle:    %d\n", hb.CycleNumber)
	fmt.Printf("Status:   %s\n", formatStatus(status))

	if status == "stale" {
		if heartbeatMaxAge > 0 {
			os.Exit(1)
		}
		os.Exit(2)
	}
	return nil
}

func runHeartbeatList(_ *cobra.Command, _ []string) error {
	heartbeats, err := monitoring.ListHeartbeats("")
	if err != nil {
		return fmt.Errorf("failed to list heartbeats: %w", err)
	}

	if len(heartbeats) == 0 {
		fmt.Println("No loop heartbeats found")
		return nil
	}

	fmt.Printf("%-25s %-10s %-12s %-8s %s\n", "SESSION", "STATUS", "AGE", "CYCLE", "INTERVAL")
	for _, hb := range heartbeats {
		age := time.Since(hb.Timestamp)
		status := monitoring.CheckStaleness(hb)
		fmt.Printf("%-25s %-10s %-12s %-8d %ds\n",
			hb.Session,
			formatStatus(status),
			age.Round(time.Second),
			hb.CycleNumber,
			hb.IntervalSecs,
		)
	}
	return nil
}

func runHeartbeatStatus(_ *cobra.Command, _ []string) error {
	heartbeats, err := monitoring.ListHeartbeats("")
	if err != nil {
		return fmt.Errorf("failed to list heartbeats: %w", err)
	}

	if len(heartbeats) == 0 {
		fmt.Println("No loop heartbeats found")
		return nil
	}

	okCount, warnCount, staleCount := 0, 0, 0
	for _, hb := range heartbeats {
		switch monitoring.CheckStaleness(hb) {
		case "ok":
			okCount++
		case "warn":
			warnCount++
		case "stale":
			staleCount++
		}
	}

	fmt.Printf("Loop Heartbeats: %d total\n", len(heartbeats))
	fmt.Printf("  OK:    %d\n", okCount)
	fmt.Printf("  Warn:  %d\n", warnCount)
	fmt.Printf("  Stale: %d\n", staleCount)

	if staleCount > 0 {
		fmt.Println("\nStale loops:")
		for _, hb := range heartbeats {
			if monitoring.CheckStaleness(hb) == "stale" {
				age := time.Since(hb.Timestamp)
				fmt.Printf("  - %s (age: %s)\n", hb.Session, age.Round(time.Second))
			}
		}
	}

	return nil
}

func runHeartbeatWatchdog(_ *cobra.Command, args []string) error {
	session := args[0]

	pollInterval := heartbeatMaxAge / 4
	if pollInterval > 30*time.Second {
		pollInterval = 30 * time.Second
	}
	if pollInterval < time.Second {
		pollInterval = time.Second
	}

	fmt.Printf("Watchdog started for %s (max-age=%s, poll=%s)\n", session, heartbeatMaxAge, pollInterval)
	fmt.Printf("Restart command: %s\n", heartbeatRestartCmd)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\nWatchdog stopped")
			return nil
		case <-ticker.C:
			hb, err := monitoring.ReadHeartbeat("", session)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Printf("[%s] No heartbeat found for %s, executing restart\n",
						time.Now().Format(time.RFC3339), session)
					if err := executeRestart(heartbeatRestartCmd); err != nil {
						fmt.Fprintf(os.Stderr, "Restart failed: %v\n", err)
					}
					continue
				}
				fmt.Fprintf(os.Stderr, "Error reading heartbeat: %v\n", err)
				continue
			}

			status := monitoring.CheckStalenessWithMaxAge(hb, heartbeatMaxAge)
			if status == "stale" {
				age := time.Since(hb.Timestamp)
				fmt.Printf("[%s] Heartbeat stale for %s (age=%s > max-age=%s), executing restart\n",
					time.Now().Format(time.RFC3339), session, age.Round(time.Second), heartbeatMaxAge)
				if err := executeRestart(heartbeatRestartCmd); err != nil {
					fmt.Fprintf(os.Stderr, "Restart failed: %v\n", err)
				}
			}
		}
	}
}

func executeRestart(cmd string) error {
	c := exec.Command("sh", "-c", cmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func formatStatus(status string) string {
	switch status {
	case "ok":
		return "OK"
	case "warn":
		return "WARN"
	case "stale":
		return "STALE"
	default:
		return status
	}
}
