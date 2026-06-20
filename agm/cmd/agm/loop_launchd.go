package main

// loop_launchd.go — install / uninstall the host-side dispatch loop as a
// macOS LaunchAgent (ce-m3ya, Phase A of ce-cd14).
//
// The LaunchAgent fires `agm loop tick` every 60 seconds. Tick is idempotent
// and fast-exits when nothing is due, so a 1-minute fire interval gives
// sub-minute precision without busy-polling. AbandonProcessGroup=false prevents
// overlapping ticks on slow jobs.
//
// Installation is idempotent: re-running replaces the plist and reloads the
// agent. This mirrors the bumblebee and reap-orphans installers.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const loopTickPlistLabel = "com.dear-agent.loop-tick"
const loopTickPlistFile = "schedules/com.dear-agent.loop-tick.plist"

var loopInstallLaunchdCmd = &cobra.Command{
	Use:   "install-launchd",
	Short: "Install the host-side dispatch loop as a macOS LaunchAgent",
	Long: `Install a per-user macOS LaunchAgent that runs 'agm loop tick' every
60 seconds. This is the Phase-A host scheduler substrate (ce-m3ya / ce-cd14).

The agent fires 'agm loop tick', which runs any loops that are due and exits
immediately when nothing is due. One-minute granularity covers all practical
cadences without busy-polling.

The plist is written to ~/Library/LaunchAgents/ and loaded immediately.
It also runs once at load time (RunAtLoad=true) to drain overdue loops.

macOS only. On Linux, use a systemd --user timer or add a crontab entry:
  * * * * * agm loop tick

To remove the job, use 'agm loop uninstall-launchd'.
To trigger a tick outside the schedule:
  launchctl kickstart gui/$UID/com.dear-agent.loop-tick`,
	Args: cobra.NoArgs,
	RunE: runLoopInstallLaunchd,
}

var loopUninstallLaunchdCmd = &cobra.Command{
	Use:   "uninstall-launchd",
	Short: "Remove the host-side dispatch loop LaunchAgent",
	Args:  cobra.NoArgs,
	RunE:  runLoopUninstallLaunchd,
}

func loopTickPlistPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", loopTickPlistLabel+".plist")
}

func runLoopInstallLaunchd(_ *cobra.Command, _ []string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("LaunchAgent is macOS-only — on Linux add `* * * * * agm loop tick` to crontab")
	}

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

	content := strings.NewReplacer(
		"__USER_HOME__", homeDir,
		"__AGM_BINARY__", agmBin,
	).Replace(string(tmpl))

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}

	logsDir := filepath.Join(homeDir, ".agm", "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}

	dest := loopTickPlistPath(homeDir)
	// 0o644: plist must be readable by launchd (runs as the owning user).
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil { //#nosec G306 -- launchd plist, world-readable by convention
		return fmt.Errorf("write plist: %w", err)
	}
	fmt.Printf("✓ Installed: %s\n", dest)

	// Unload any prior version (ignore errors — agent may not be loaded yet).
	_ = exec.Command("launchctl", "unload", dest).Run() //#nosec G204

	if out, err := exec.Command("launchctl", "load", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl load failed (%v): %s\n", err, out)
		fmt.Println("  The plist was written; load it manually with:")
		fmt.Printf("  launchctl load %s\n", dest)
		return nil
	}
	fmt.Printf("✓ Loaded:    %s\n", loopTickPlistLabel)
	fmt.Println()
	fmt.Println("The host dispatch loop fires every 60 seconds.")
	fmt.Printf("Logs:    %s/.agm/logs/loop-tick.log\n", homeDir)
	fmt.Printf("Trigger: launchctl kickstart gui/$UID/%s\n", loopTickPlistLabel)
	fmt.Println("Remove:  agm loop uninstall-launchd")
	return nil
}

func runLoopUninstallLaunchd(_ *cobra.Command, _ []string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("LaunchAgent is macOS-only")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dest := loopTickPlistPath(homeDir)

	if out, err := exec.Command("launchctl", "unload", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl unload: %v: %s\n", err, out)
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
