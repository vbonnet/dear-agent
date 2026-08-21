package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

const quotaPlistLabel = "com.dear-agent.quota-refresh"
const quotaPlistFile = "schedules/com.dear-agent.quota-refresh.plist"

var installQuotaScheduleCmd = &cobra.Command{
	Use:   "install-quota-schedule",
	Short: "Install the half-hourly provider-quota refresh launchd job",
	Long: `Install a macOS LaunchAgent that runs 'agm quota-meter --refresh' every 30
minutes, publishing the reading every quota consumer reads.

This job is what makes the cost guardrail real. Reading the meter takes
seconds, so nothing on the spawn path may do it. Instead this job publishes
a state file, and the provider_quota circuit-breaker gate, 'agm quota-meter', and
the orchestrator all read that file in O(1).

The interval is half the guardrail's 90-minute staleness limit, so a single
missed run still leaves a usable reading. If this job stops, readings go
stale and the guardrail fails open — it stops halting spawns rather than
halting them on data it cannot trust. That is the safe direction, but it
does mean a dead job silently removes the guardrail; 'agm quota-meter' prints
STALE so the condition is visible.

The plist is written to ~/Library/LaunchAgents/ and loaded immediately with
'launchctl load'. RunAtLoad publishes a first reading right away.

To refresh immediately outside the schedule:

  launchctl kickstart gui/$UID/com.dear-agent.quota-refresh

To remove the job, use 'agm admin uninstall-quota-schedule'.`,
	Args: cobra.NoArgs,
	RunE: runInstallQuotaSchedule,
}

var uninstallQuotaScheduleCmd = &cobra.Command{
	Use:   "uninstall-quota-schedule",
	Short: "Remove the half-hourly provider-quota refresh launchd job",
	Args:  cobra.NoArgs,
	RunE:  runUninstallQuotaSchedule,
}

func init() {
	adminCmd.AddCommand(installQuotaScheduleCmd)
	adminCmd.AddCommand(uninstallQuotaScheduleCmd)
}

func quotaPlistPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", quotaPlistLabel+".plist")
}

// xmlEscapeText escapes s for placement inside an XML text node (a plist
// <string> element's content), so a value containing &, <, >, or a quote
// cannot break the surrounding markup. xml.EscapeText never returns an
// error for a bytes.Buffer target, so the error is intentionally ignored.
func xmlEscapeText(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

func runInstallQuotaSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	agmBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve agm binary path: %w", err)
	}

	// Resolve the state path here, in the installer's own environment,
	// and bake it into the job rather than letting quota-meter re-resolve
	// it at run time: launchd's EnvironmentVariables below carries only
	// PATH and HOME, not XDG_STATE_HOME, so a job left to its own default
	// would silently publish to ~/.local/state even for an operator whose
	// interactive shell (and every interactive reader) uses
	// $XDG_STATE_HOME instead (codex review on #1218).
	statePath, err := quota.DefaultStateFilePath()
	if err != nil {
		return fmt.Errorf("resolve quota state path: %w", err)
	}

	tmpl, err := schedulesFS.ReadFile(quotaPlistFile)
	if err != nil {
		return fmt.Errorf("read embedded plist template: %w", err)
	}

	// XML-escape every substituted value: each is a filesystem path that
	// can legitimately contain an XML metacharacter (a home directory
	// under "R&D", say). Substituting it raw produces a malformed plist —
	// launchctl load then fails, and this installer would otherwise still
	// report success while pointing an operator at the same invalid file
	// (codex review on #1218).
	content := string(tmpl)
	content = strings.ReplaceAll(content, "__USER_HOME__", xmlEscapeText(homeDir))
	content = strings.ReplaceAll(content, "__AGM_BINARY__", xmlEscapeText(agmBin))
	content = strings.ReplaceAll(content, "__STATE_FILE__", xmlEscapeText(statePath))

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0o755); err != nil {
		return fmt.Errorf("create LaunchAgents dir: %w", err)
	}
	logsDir := filepath.Join(homeDir, "Library", "Logs", "dear-agent")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}

	dest := quotaPlistPath(homeDir)
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil { //#nosec G306 -- launchd plist is world-readable by convention
		return fmt.Errorf("write plist: %w", err)
	}
	fmt.Printf("✓ Installed: %s\n", dest)

	// Load the job. Unload first in case an older version is running. Unload
	// failure is expected on first install (no older job loaded), so it is
	// reported but never fatal.
	if err := exec.Command("launchctl", "unload", dest).Run(); err != nil { //#nosec G204 -- controlled path
		fmt.Printf("  (no previous job unloaded: %v)\n", err)
	}
	if out, err := exec.Command("launchctl", "load", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl load failed (%v): %s\n", err, out)
		fmt.Println("  The plist was written; load it manually with:")
		fmt.Printf("  launchctl load %s\n", dest)
		return nil
	}
	fmt.Printf("✓ Loaded:    %s\n", quotaPlistLabel)

	fmt.Printf("\nReadings are published to:\n  %s\n", statePath)
	fmt.Println("\nThe refresh runs every 30 minutes and once immediately at load.")
	fmt.Println("Read the current reading with 'agm quota-meter' or 'agm quota-meter --json'.")
	return nil
}

func runUninstallQuotaSchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dest := quotaPlistPath(homeDir)

	if out, err := exec.Command("launchctl", "unload", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl unload: %v: %s\n", err, out)
	} else {
		fmt.Printf("✓ Unloaded: %s\n", quotaPlistLabel)
	}

	if err := os.Remove(dest); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("✓ Plist not found (already removed): %s\n", dest)
			return nil
		}
		return fmt.Errorf("remove plist: %w", err)
	}
	fmt.Printf("✓ Removed:  %s\n", dest)
	fmt.Println("\nReadings will go stale, and the quota guardrail will stop")
	fmt.Println("halting spawns rather than act on data it cannot trust.")
	return nil
}
