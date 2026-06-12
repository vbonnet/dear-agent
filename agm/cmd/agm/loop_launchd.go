package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const loopTickPlistLabel = "com.dear-agent.loop-tick"
const loopTickPlistFile = "schedules/com.dear-agent.loop-tick.plist"

// launchctlRun is the seam for testing — replaced in tests to avoid touching
// the real launchd.
var launchctlRun = func(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput() //#nosec G204 -- controlled fixed args
	if err != nil {
		return fmt.Errorf("launchctl %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return nil
}

var loopInstallLaunchdCmd = &cobra.Command{
	Use:   "install-launchd",
	Short: "Install the loop-tick launchd timer (fires agm loop tick every 300s)",
	Long: `Install a macOS LaunchAgent that fires 'agm loop tick' every 300 seconds.

Idempotent: running it twice converges (bootout + bootstrap). The plist is
written to ~/Library/LaunchAgents/ and activated immediately.

To trigger a tick manually outside the schedule:

  launchctl kickstart gui/$UID/com.dear-agent.loop-tick

To remove the timer, use 'agm loop uninstall-launchd'.`,
	Args: cobra.NoArgs,
	RunE: runLoopInstallLaunchd,
}

var loopUninstallLaunchdCmd = &cobra.Command{
	Use:   "uninstall-launchd",
	Short: "Remove the loop-tick launchd timer",
	Args:  cobra.NoArgs,
	RunE:  runLoopUninstallLaunchd,
}

func loopTickPlistPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", loopTickPlistLabel+".plist")
}

func runLoopInstallLaunchd(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	agmBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve agm binary path: %w", err)
	}

	tmpl, err := schedulesFS.ReadFile(loopTickPlistFile)
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

	logsDir := filepath.Join(homeDir, ".agm", "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}

	dest := loopTickPlistPath(homeDir)
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil { //#nosec G306 -- launchd plist is world-readable by convention
		return fmt.Errorf("write plist: %w", err)
	}
	fmt.Printf("✓ Installed: %s\n", dest)

	// Idempotent activate: bootout (best-effort — fails if not loaded), then bootstrap.
	if err := launchctlRun("bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), loopTickPlistLabel)); err != nil {
		// Not loaded yet is the expected case on first install; ignore.
		_ = err
	}
	if err := launchctlRun("bootstrap", fmt.Sprintf("gui/%d", os.Getuid()), dest); err != nil {
		fmt.Printf("⚠ launchctl bootstrap failed: %v\n", err)
		fmt.Println("  The plist was written; load it manually with:")
		fmt.Printf("  launchctl bootstrap gui/$UID %s\n", dest)
		return nil
	}
	fmt.Printf("✓ Loaded:    %s\n", loopTickPlistLabel)
	fmt.Println("\nTick fires every 300s. Trigger immediately with:")
	fmt.Printf("  launchctl kickstart gui/$UID/%s\n", loopTickPlistLabel)
	return nil
}

func runLoopUninstallLaunchd(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dest := loopTickPlistPath(homeDir)

	if err := launchctlRun("bootout", fmt.Sprintf("gui/%d/%s", os.Getuid(), loopTickPlistLabel)); err != nil {
		fmt.Printf("⚠ launchctl bootout: %v\n", err)
	} else {
		fmt.Printf("✓ Unloaded: %s\n", loopTickPlistLabel)
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

func init() {
	loopCmd.AddCommand(loopInstallLaunchdCmd)
	loopCmd.AddCommand(loopUninstallLaunchdCmd)
}
