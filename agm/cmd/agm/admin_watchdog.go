package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var installWatchdogCmd = &cobra.Command{
	Use:   "install-watchdog",
	Short: "Install crontab watchdog for tmux crash recovery",
	Long: `Install a crontab entry that runs every minute to monitor the tmux server.

The watchdog detects tmux server crashes, restarts the server, and reports
missing sessions that may need recovery.

Components installed:
  • ~/.agm/bin/watchdog.sh  — monitoring script
  • Crontab entry           — runs the script every minute

Log output is written to ~/.agm/logs/watchdog.log.

To check status:  agm admin install-watchdog --status
To remove:        agm admin uninstall-watchdog`,
	RunE: runInstallWatchdog,
}

var uninstallWatchdogCmd = &cobra.Command{
	Use:   "uninstall-watchdog",
	Short: "Remove the tmux watchdog crontab entry and script",
	Long: `Remove the watchdog crontab entry and delete the monitoring script.

This does not remove log files. To clean those up:
  rm ~/.agm/logs/watchdog.log`,
	RunE: runUninstallWatchdog,
}

var watchdogStatusFlag bool

func init() {
	installWatchdogCmd.Flags().BoolVar(&watchdogStatusFlag, "status", false, "Show watchdog installation status without installing")
	adminCmd.AddCommand(installWatchdogCmd)
	adminCmd.AddCommand(uninstallWatchdogCmd)
}

func runInstallWatchdog(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	paths := ops.ResolveWatchdogPaths(homeDir)

	// Status-only mode
	if watchdogStatusFlag {
		if ops.IsWatchdogInstalled(paths) {
			ui.PrintSuccess("Watchdog is installed")
			fmt.Printf("  Script: %s\n", paths.ScriptPath)
			fmt.Printf("  Log:    %s\n", paths.LogPath)
		} else {
			ui.PrintWarning("Watchdog is not installed")
			fmt.Println("  Run: agm admin install-watchdog")
		}
		return nil
	}

	// Resolve agm binary path
	agmBin, err := exec.LookPath("agm")
	if err != nil {
		agmBin = "agm" // fallback — will work if agm is in PATH at cron runtime
	}

	socketPath := tmux.GetSocketPath()

	fmt.Println("Installing AGM watchdog...")
	if err := ops.InstallWatchdog(paths, socketPath, agmBin); err != nil {
		return fmt.Errorf("install watchdog: %w", err)
	}

	ui.PrintSuccess("Watchdog installed successfully")
	fmt.Printf("  Script:  %s\n", paths.ScriptPath)
	fmt.Printf("  Log:     %s\n", paths.LogPath)
	fmt.Printf("  Socket:  %s\n", socketPath)
	fmt.Printf("  Crontab: * * * * * (every minute)\n")
	return nil
}

func runUninstallWatchdog(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	paths := ops.ResolveWatchdogPaths(homeDir)

	if !ops.IsWatchdogInstalled(paths) {
		fmt.Println("Watchdog is not installed, nothing to remove.")
		return nil
	}

	fmt.Println("Removing AGM watchdog...")
	if err := ops.UninstallWatchdog(paths); err != nil {
		return fmt.Errorf("uninstall watchdog: %w", err)
	}

	ui.PrintSuccess("Watchdog uninstalled successfully")
	fmt.Println("  Log files were preserved at:", paths.LogPath)
	return nil
}
