package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const supervisorUnblockPlistLabel = "com.dear-agent.supervisor-unblock"
const supervisorUnblockPlistFile = "schedules/com.dear-agent.supervisor-unblock.plist"

var supervisorUnblockWorkspace string

var installSupervisorUnblockScheduleCmd = &cobra.Command{
	Use:   "install-supervisor-unblock-schedule",
	Short: "Install the external supervisor permission-prompt watchdog launchd daemon",
	Long: `Install a macOS LaunchAgent that runs 'agm scan --loop --cross-check' as a
persistent daemon, auto-approving permission prompts on stuck VROOM supervisors.

WHY. ADR-002 mutual-unblock has each supervisor run 'agm send approve' on a
stuck peer — but that assumes at least one supervisor is alive to do the
approving. When the whole trio blocks or is uninitialized at once, no peer can
approve and the mesh deadlocks. 'agm watch-stalled' does not cover this: its
permission-prompt recovery only alerts the orchestrator, so the alert lands on a
dead session. This watchdog is the supervisor-INDEPENDENT backstop — the cross-
check captures each supervisor pane via tmux and auto-approves RBAC-safe prompts,
whether or not any supervisor is alive.

Unlike the reap-orphans and worktree-sweep jobs (one-shot, run on an interval),
this is a long-running loop, so the plist uses KeepAlive (restart on crash) with
RunAtLoad. '--interval 2m' bounds how long a stuck supervisor waits for auto-
approval.

The plist is written to ~/Library/LaunchAgents/ and loaded immediately with
'launchctl load'. Output (one JSON scan report per cycle) is logged to
~/Library/Logs/dear-agent/supervisor-unblock.log.

To restart the watchdog outside the normal lifecycle:

  launchctl kickstart -k gui/$UID/com.dear-agent.supervisor-unblock

To remove the job, use 'agm admin uninstall-supervisor-unblock-schedule'.`,
	Args: cobra.NoArgs,
	RunE: runInstallSupervisorUnblockSchedule,
}

var uninstallSupervisorUnblockScheduleCmd = &cobra.Command{
	Use:   "uninstall-supervisor-unblock-schedule",
	Short: "Remove the external supervisor permission-prompt watchdog launchd daemon",
	Args:  cobra.NoArgs,
	RunE:  runUninstallSupervisorUnblockSchedule,
}

func init() {
	installSupervisorUnblockScheduleCmd.Flags().StringVar(&supervisorUnblockWorkspace, "workspace",
		os.Getenv("WORKSPACE"), "agm/Dolt workspace whose supervisor sessions are scanned (defaults to $WORKSPACE)")
	adminCmd.AddCommand(installSupervisorUnblockScheduleCmd)
	adminCmd.AddCommand(uninstallSupervisorUnblockScheduleCmd)
}

func supervisorUnblockPlistPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", supervisorUnblockPlistLabel+".plist")
}

// renderSupervisorUnblockPlist substitutes the template placeholders. Kept
// separate from the install side-effects so it can be unit-tested without
// launchctl.
func renderSupervisorUnblockPlist(homeDir, agmBin, workspace string) (string, error) {
	tmpl, err := schedulesFS.ReadFile(supervisorUnblockPlistFile)
	if err != nil {
		return "", fmt.Errorf("read embedded plist template: %w", err)
	}
	content := string(tmpl)
	content = strings.ReplaceAll(content, "__USER_HOME__", homeDir)
	content = strings.ReplaceAll(content, "__AGM_BINARY__", agmBin)
	content = strings.ReplaceAll(content, "__WORKSPACE__", workspace)
	return content, nil
}

func runInstallSupervisorUnblockSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	agmBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve agm binary path: %w", err)
	}

	if supervisorUnblockWorkspace == "" {
		fmt.Println("⚠ No workspace set (--workspace empty and $WORKSPACE unset).")
		fmt.Println("  launchd runs in a minimal environment; without WORKSPACE the scan")
		fmt.Println("  connects to the default Dolt database and finds no supervisor sessions.")
		fmt.Println("  Re-run with --workspace <name> if the watchdog approves nothing.")
	}

	content, err := renderSupervisorUnblockPlist(homeDir, agmBin, supervisorUnblockWorkspace)
	if err != nil {
		return err
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}

	logsDir := filepath.Join(homeDir, "Library", "Logs", "dear-agent")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}

	dest := supervisorUnblockPlistPath(homeDir)
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
	fmt.Printf("✓ Loaded:    %s\n", supervisorUnblockPlistLabel)
	fmt.Printf("\nThe watchdog scans workspace %q every 2m and auto-approves stuck supervisors.\n",
		supervisorUnblockWorkspace)
	fmt.Println("Tail scan reports with:")
	fmt.Println("  tail -f ~/Library/Logs/dear-agent/supervisor-unblock.log | jq .")
	fmt.Println("Restart the watchdog with:")
	fmt.Printf("  launchctl kickstart -k gui/$UID/%s\n", supervisorUnblockPlistLabel)
	return nil
}

func runUninstallSupervisorUnblockSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dest := supervisorUnblockPlistPath(homeDir)

	if out, err := exec.Command("launchctl", "unload", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl unload: %v: %s\n", err, out)
	} else {
		fmt.Printf("✓ Unloaded: %s\n", supervisorUnblockPlistLabel)
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
