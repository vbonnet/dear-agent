package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const watchStalledPlistLabel = "com.dear-agent.watch-stalled"
const watchStalledPlistFile = "schedules/com.dear-agent.watch-stalled.plist"

// defaultStallOrchestrator is the --orchestrator value the generated
// schedule carries. It is empty on purpose: routing discovers a live
// Dispatch/orchestrator/supervisor session at run time and queues durably
// when none is reachable, so baking a name in at install time would pin
// the schedule to a session that may since have died.
//
// There is deliberately no named default anywhere for this: under runtime
// discovery, "who receives an alert" is only knowable at delivery time, so
// a constant naming one session would be a confident wrong answer on any
// host where that session is not the live supervisor.
const defaultStallOrchestrator = ""

var (
	watchStalledOrchestrator string
	watchStalledWorkspace    string
)

var installWatchStalledScheduleCmd = &cobra.Command{
	Use:   "install-watch-stalled-schedule",
	Short: "Install the host stall-monitor launchd daemon",
	Long: `Install a macOS LaunchAgent that runs 'agm watch-stalled' as a
persistent daemon and routes stall alerts into the VROOM mesh.

Unlike the reap-orphans and worktree-sweep jobs (one-shot, run on an interval),
'agm watch-stalled' is a long-running monitor: it polls active AGM sessions and
emits a JSON stall event whenever a session is stuck in a permission prompt,
makes no commits for too long, or loops on the same error. The plist therefore
uses KeepAlive (restart on crash) rather than StartInterval, with RunAtLoad so
the monitor comes up at login and after a reboot.

Completion watching is on by default, so the installed daemon is not stall-only.
It also emits one {"event_type":"completion"} object per session that finishes a
unit of work, delivers each to the notify dispatchers (~/.agm/notify.yaml, or
the stderr log dispatcher when that file is absent), and relays the result tail
to a live supervisor. Sessions whose names contain orchestrator, overseer, or
meta- are excluded by default, as is whichever session routing has currently
selected, so a supervisor's own completion is never relayed back into itself.

Alert routing: by default the daemon leaves --orchestrator empty, so every
recovery action (permission-prompt alerts, no-commit nudges, error-loop
diagnostics, and max-retry escalations) plus every surfaced completion
discovers a live Dispatch/orchestrator/supervisor session at run time and is
tried against each in preference order. Passing --orchestrator names a
preferred first candidate rather than pinning routing to it. An alert no live
session accepts is written durably to ~/.agm/alerts/queue.jsonl with status
"queued" and re-attempted on later scans, so a supervisor outage delays a
stall alert rather than losing it ('agm alerts list --status queued').

The plist is written to ~/Library/LaunchAgents/ and loaded immediately with
'launchctl load'. Output (one JSON object per event) is logged to
~/Library/Logs/dear-agent/watch-stalled.log.

To restart the monitor outside the normal lifecycle:

  launchctl kickstart -k gui/$UID/com.dear-agent.watch-stalled

To remove the job, use 'agm admin uninstall-watch-stalled-schedule'.`,
	Args: cobra.NoArgs,
	RunE: runInstallWatchStalledSchedule,
}

var uninstallWatchStalledScheduleCmd = &cobra.Command{
	Use:   "uninstall-watch-stalled-schedule",
	Short: "Remove the host stall-monitor launchd daemon",
	Args:  cobra.NoArgs,
	RunE:  runUninstallWatchStalledSchedule,
}

func init() {
	installWatchStalledScheduleCmd.Flags().StringVar(&watchStalledOrchestrator, "orchestrator",
		defaultStallOrchestrator, "VROOM mesh session to route stall alerts to")
	installWatchStalledScheduleCmd.Flags().StringVar(&watchStalledWorkspace, "workspace",
		os.Getenv("WORKSPACE"), "agm/Dolt workspace whose sessions are monitored (defaults to $WORKSPACE)")
	adminCmd.AddCommand(installWatchStalledScheduleCmd)
	adminCmd.AddCommand(uninstallWatchStalledScheduleCmd)
}

func watchStalledPlistPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", watchStalledPlistLabel+".plist")
}

// renderWatchStalledPlist substitutes the template placeholders. Kept separate
// from the install side-effects so it can be unit-tested without launchctl.
func renderWatchStalledPlist(homeDir, agmBin, orchestrator, workspace string) (string, error) {
	tmpl, err := schedulesFS.ReadFile(watchStalledPlistFile)
	if err != nil {
		return "", fmt.Errorf("read embedded plist template: %w", err)
	}
	// Normalize line endings before any newline-sensitive replacement below:
	// a CRLF checkout (e.g. git autocrlf on Windows) would otherwise make
	// the --orchestrator removal below match nothing and silently leave the
	// placeholder in the installed plist.
	content := strings.ReplaceAll(string(tmpl), "\r\n", "\n")
	content = strings.ReplaceAll(content, "__USER_HOME__", homeDir)
	content = strings.ReplaceAll(content, "__AGM_BINARY__", agmBin)
	// The default orchestrator is empty, which is what enables run-time
	// discovery. Emitting "--orchestrator" followed by an empty <string>
	// leaves a flag whose value is a blank argument: it parses today, but
	// any plist round-trip that drops empty strings would leave the flag
	// dangling and make it swallow the next argument instead. Drop the
	// whole pair when there is no orchestrator to name.
	if strings.TrimSpace(orchestrator) == "" {
		content = strings.ReplaceAll(content,
			"        <string>--orchestrator</string>\n        <string>__ORCHESTRATOR__</string>\n", "")
	}
	// Run unconditionally afterwards: the placeholder also appears in the
	// template's header comment, which must be substituted whether or not
	// the argument pair survived.
	content = strings.ReplaceAll(content, "__ORCHESTRATOR__", orchestratorDescription(orchestrator))
	content = strings.ReplaceAll(content, "__WORKSPACE__", workspace)
	return content, nil
}

func runInstallWatchStalledSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	agmBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve agm binary path: %w", err)
	}

	if watchStalledWorkspace == "" {
		fmt.Println("⚠ No workspace set (--workspace empty and $WORKSPACE unset).")
		fmt.Println("  launchd runs in a minimal environment; without WORKSPACE the watcher")
		fmt.Println("  will connect to the default Dolt database and may exit immediately.")
		fmt.Println("  Re-run with --workspace <name> if the monitor fails to start.")
	}

	content, err := renderWatchStalledPlist(homeDir, agmBin, watchStalledOrchestrator, watchStalledWorkspace)
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

	dest := watchStalledPlistPath(homeDir)
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
	fmt.Printf("✓ Loaded:    %s\n", watchStalledPlistLabel)
	fmt.Printf("\nThe monitor runs continuously over workspace %q and routes alerts to %q.\n",
		watchStalledWorkspace, watchStalledOrchestrator)
	fmt.Println("Tail stall events with:")
	fmt.Println("  tail -f ~/Library/Logs/dear-agent/watch-stalled.log | jq .")
	fmt.Println("Restart the monitor with:")
	fmt.Printf("  launchctl kickstart -k gui/$UID/%s\n", watchStalledPlistLabel)
	return nil
}

func runUninstallWatchStalledSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dest := watchStalledPlistPath(homeDir)

	if out, err := exec.Command("launchctl", "unload", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl unload: %v: %s\n", err, out)
	} else {
		fmt.Printf("✓ Unloaded: %s\n", watchStalledPlistLabel)
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

// orchestratorDescription names the routing target for the generated
// plist's header comment. An empty orchestrator is not "nothing": it is
// what selects run-time discovery, so the comment says so rather than
// leaving a blank where a session name is expected.
func orchestratorDescription(orchestrator string) string {
	if strings.TrimSpace(orchestrator) == "" {
		return "(discovery: no pinned orchestrator)"
	}
	return orchestrator
}
