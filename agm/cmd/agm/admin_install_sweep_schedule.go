package main

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed schedules/*
var schedulesFS embed.FS

const sweepPlistLabel = "com.dear-agent.worktree-sweep"
const sweepPlistFile = "schedules/com.dear-agent.worktree-sweep.plist"

var installSweepScheduleCmd = &cobra.Command{
	Use:   "install-sweep-schedule",
	Short: "Install the daily worktree-sweep launchd job",
	Long: `Install a macOS LaunchAgent that runs 'agm worktree sweep --execute'
daily at 03:00 local time.

The job complements the on-Stop hook Layer 3 (fire-and-forget sweep after each
session close): the launchd schedule ensures a full sweep runs even on days with
no Claude Code sessions — e.g. after a weekend or a machine restart.

The plist is written to ~/Library/LaunchAgents/ and loaded immediately with
'launchctl load'. To trigger a sweep manually outside the schedule:

  launchctl kickstart gui/$UID/com.dear-agent.worktree-sweep

To remove the job, use 'agm admin uninstall-sweep-schedule'.`,
	Args: cobra.NoArgs,
	RunE: runInstallSweepSchedule,
}

var uninstallSweepScheduleCmd = &cobra.Command{
	Use:   "uninstall-sweep-schedule",
	Short: "Remove the daily worktree-sweep launchd job",
	Args:  cobra.NoArgs,
	RunE:  runUninstallSweepSchedule,
}

func init() {
	adminCmd.AddCommand(installSweepScheduleCmd)
	adminCmd.AddCommand(uninstallSweepScheduleCmd)
}

func sweepPlistPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", sweepPlistLabel+".plist")
}

func runInstallSweepSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	agmBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve agm binary path: %w", err)
	}

	tmpl, err := schedulesFS.ReadFile(sweepPlistFile)
	if err != nil {
		return fmt.Errorf("read embedded plist template: %w", err)
	}

	content := string(tmpl)
	content = strings.ReplaceAll(content, "__USER_HOME__", homeDir)
	content = strings.ReplaceAll(content, "__AGM_BINARY__", agmBin)

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}

	logsDir := filepath.Join(homeDir, "Library", "Logs", "dear-agent")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}

	dest := sweepPlistPath(homeDir)
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil { //#nosec G306 -- launchd plist is world-readable by convention
		return fmt.Errorf("write plist: %w", err)
	}
	fmt.Printf("✓ Installed: %s\n", dest)

	// Load the job. Unload first in case an older version is running.
	_ = exec.Command("launchctl", "unload", dest).Run()                                   //#nosec G204 -- controlled path
	if out, err := exec.Command("launchctl", "load", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl load failed (%v): %s\n", err, out)
		fmt.Println("  The plist was written; load it manually with:")
		fmt.Printf("  launchctl load %s\n", dest)
		return nil
	}
	fmt.Printf("✓ Loaded:    %s\n", sweepPlistLabel)
	fmt.Println("\nThe sweep will run daily at 03:00. Trigger immediately with:")
	fmt.Printf("  launchctl kickstart gui/$UID/%s\n", sweepPlistLabel)
	return nil
}

func runUninstallSweepSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dest := sweepPlistPath(homeDir)

	if out, err := exec.Command("launchctl", "unload", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl unload: %v: %s\n", err, out)
	} else {
		fmt.Printf("✓ Unloaded: %s\n", sweepPlistLabel)
	}

	if err := os.Remove(dest); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("✓ Plist not found (already removed): %s\n", dest)
			return nil
		}
		return fmt.Errorf("remove plist: %w", err)
	}
	fmt.Printf("✓ Removed:  %s\n", dest)
	return nil
}
