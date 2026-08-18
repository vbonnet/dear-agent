package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const archiveUIPlistLabel = "com.dear-agent.archive-ui"
const archiveUIPlistFile = "schedules/com.dear-agent.archive-ui.plist"

var installArchiveUIScheduleCmd = &cobra.Command{
	Use:   "install-archive-ui-schedule",
	Short: "Install the daily idle Claude UI session-archival launchd job",
	Long: `Install a macOS LaunchAgent that runs 'agm session archive-ui
--older-than 7d --status idle --apply' every 24 hours.

The Claude desktop / claude.ai/code "Code" session list grows without bound:
every CLI session writes a local_<id>.json with isArchived=false, and the only
first-party way to clear them is the per-row UI archive action, so they
accumulate into the hundreds (ADR-026). 'agm session archive-ui' flips the
local isArchived flag for sessions idle past a threshold; this launchd job is
what makes that automatic instead of a manual chore. It is the scheduled caller
described by ADR-026 and closes the accumulation loop.

It is conservative by construction: it only archives sessions with no live
process owning them and last active more than 7 days ago. archive-ui never
deletes a file, never touches ~/.claude/projects/*.jsonl transcripts, never
calls a network API, and backs up every mutated file under
~/.agm/backups/claude-ui-sessions/<ts>/. Reverse a run with:

  agm session archive-ui --status all --unarchive --apply

The plist is written to ~/Library/LaunchAgents/ and loaded immediately with
'launchctl load'. RunAtLoad=true also drains the existing idle backlog once at
install time.

To trigger an immediate sweep outside the daily schedule:

  launchctl kickstart gui/$UID/com.dear-agent.archive-ui

To remove the job, use 'agm admin uninstall-archive-ui-schedule'.`,
	Args: cobra.NoArgs,
	RunE: runInstallArchiveUISchedule,
}

var uninstallArchiveUIScheduleCmd = &cobra.Command{
	Use:   "uninstall-archive-ui-schedule",
	Short: "Remove the daily Claude UI session-archival launchd job",
	Args:  cobra.NoArgs,
	RunE:  runUninstallArchiveUISchedule,
}

func init() {
	adminCmd.AddCommand(installArchiveUIScheduleCmd)
	adminCmd.AddCommand(uninstallArchiveUIScheduleCmd)
}

func archiveUIPlistPath(homeDir string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", archiveUIPlistLabel+".plist")
}

// renderArchiveUIPlist substitutes the template placeholders. Kept separate
// from the install side-effects so it can be unit-tested without launchctl.
func renderArchiveUIPlist(homeDir, agmBin string) (string, error) {
	tmpl, err := schedulesFS.ReadFile(archiveUIPlistFile)
	if err != nil {
		return "", fmt.Errorf("read embedded plist template: %w", err)
	}
	content := string(tmpl)
	content = strings.ReplaceAll(content, "__USER_HOME__", homeDir)
	content = strings.ReplaceAll(content, "__AGM_BINARY__", agmBin)
	return content, nil
}

func runInstallArchiveUISchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	agmBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve agm binary path: %w", err)
	}

	content, err := renderArchiveUIPlist(homeDir, agmBin)
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

	dest := archiveUIPlistPath(homeDir)
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
	fmt.Printf("✓ Loaded:    %s\n", archiveUIPlistLabel)
	fmt.Println("\nThe archiver runs daily and also runs immediately at load")
	fmt.Println("(archiving the existing idle backlog: --older-than 7d --status idle).")
	fmt.Println("Trigger an immediate sweep with:")
	fmt.Printf("  launchctl kickstart gui/$UID/%s\n", archiveUIPlistLabel)
	return nil
}

func runUninstallArchiveUISchedule(_ *cobra.Command, _ []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home dir: %w", err)
	}

	dest := archiveUIPlistPath(homeDir)

	if out, err := exec.Command("launchctl", "unload", dest).CombinedOutput(); err != nil { //#nosec G204
		fmt.Printf("⚠ launchctl unload: %v: %s\n", err, out)
	} else {
		fmt.Printf("✓ Unloaded: %s\n", archiveUIPlistLabel)
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
