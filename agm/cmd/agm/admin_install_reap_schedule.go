package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const reapPlistLabel = "com.dear-agent.reap-orphans"
const reapPlistFile = "schedules/com.dear-agent.reap-orphans.plist"

var installReapScheduleCmd = &cobra.Command{
	Use:   "install-reap-schedule",
	Short: "Install the hourly orphan-process-reap launchd job",
	Long: `Install a macOS LaunchAgent that runs 'agm session reap-orphans'
every hour.

The Stop hook reaps orphaned gopls and agm-mcp-server processes on graceful
session end, but abrupt deaths (OOM, kill -9, Desktop crash) leave them
running. This launchd job is the backstop sweep that prevents orphan
accumulation to FD / swap exhaustion (root cause of the 2026-06-15 P0,
ce-710r). It complements the Stop hook layer rather than replacing it.

The plist is written to ~/Library/LaunchAgents/ and loaded immediately with
'launchctl load'. It also runs once at load time (RunAtLoad=true) to drain
any orphans that accumulated since the last session end.

To trigger an immediate sweep outside the hourly schedule:

  launchctl kickstart gui/$UID/com.dear-agent.reap-orphans

To remove the job, use 'agm admin uninstall-reap-schedule'.`,
	Args: cobra.NoArgs,
	RunE: runInstallReapSchedule,
}

var uninstallReapScheduleCmd = &cobra.Command{
	Use:   "uninstall-reap-schedule",
	Short: "Remove the hourly orphan-process-reap launchd job",
	Args:  cobra.NoArgs,
	RunE:  runUninstallReapSchedule,
}

func init() {
	adminCmd.AddCommand(installReapScheduleCmd)
	adminCmd.AddCommand(uninstallReapScheduleCmd)
}

func reapPlistPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", reapPlistLabel+".plist")
}

func runInstallReapSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	agmBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve agm binary path: %w", err)
	}

	tmpl, err := schedulesFS.ReadFile(reapPlistFile)
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

	dest := reapPlistPath(homeDir)
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
	fmt.Printf("✓ Loaded:    %s\n", reapPlistLabel)
	fmt.Println("\nThe reaper runs hourly and also runs immediately at load.")
	fmt.Println("Trigger an immediate sweep with:")
	fmt.Printf("  launchctl kickstart gui/$UID/%s\n", reapPlistLabel)
	return nil
}

func runUninstallReapSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dest := reapPlistPath(homeDir)

	if out, err := exec.Command("launchctl", "unload", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl unload: %v: %s\n", err, out)
	} else {
		fmt.Printf("✓ Unloaded: %s\n", reapPlistLabel)
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
