package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/gclog"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var logSandboxGCEntry = logSandboxGCEntryDefault

// printSandboxGCText renders the human-readable sweep report.
func printSandboxGCText(result *ops.SandboxGCResult) {
	if result.DryRun {
		fmt.Println("DRY RUN — no sandboxes were removed (use --reap to delete)")
	}
	for _, warning := range result.Warnings {
		ui.PrintWarning(warning)
	}
	for _, e := range result.Entries {
		switch e.Action {
		case "reaped":
			fmt.Printf("  [reaped]     %s\n", e.Name)
		case "would-reap":
			fmt.Printf("  [would reap] %s\n", e.Name)
		case "kept":
			fmt.Printf("  [kept]       %s (%s)\n", e.Name, e.Reason)
		case "error":
			fmt.Printf("  [error]      %s: %s\n", e.Name, e.Reason)
		}
	}
	fmt.Println()

	summary := fmt.Sprintf("Scanned %d, reaped %d, kept %d", result.Scanned, result.Reaped, result.Kept)
	if result.Errors > 0 {
		summary += fmt.Sprintf(", errors %d", result.Errors)
	}
	if result.ProbeFailures > 0 {
		summary += fmt.Sprintf(", probe failures %d", result.ProbeFailures)
	}
	switch {
	case result.DryRun:
		ui.PrintSuccess(fmt.Sprintf("Dry run: %s", summary))
	case result.Reaped > 0:
		ui.PrintSuccess(summary)
	default:
		fmt.Println(summary)
	}
	if result.Errors > 0 {
		ui.PrintWarning(fmt.Sprintf("%d sandbox(es) failed to remove — check ~/.agm/logs/gc.jsonl", result.Errors))
	}
	if result.ProbeFailures > 0 {
		ui.PrintWarning(fmt.Sprintf("%d sandbox(es) could not be evaluated (lsof/mount/session-store probe failed) — check ~/.agm/logs/gc.jsonl", result.ProbeFailures))
	}
}

// effectiveSandboxGCReap decides whether a requested reap may actually delete.
//
// Never reap on partial knowledge. A skipped workspace contributes none of its
// live session IDs to the inventory, so every sandbox owned by that workspace
// looks unowned and can pass the remaining gates — deleting a live session's
// sandbox because we could not read the store that proves it is live. A
// partial inventory downgrades the run to a scan and returns the notice
// explaining why; a complete inventory reaps as requested. A dry run is
// already non-destructive and needs no notice.
func effectiveSandboxGCReap(requested bool, warnings []string) (bool, string) {
	if !requested || len(warnings) == 0 {
		return requested, ""
	}
	return false, fmt.Sprintf(
		"refusing to reap: live-session inventory is partial (%d workspace store(s) skipped); "+
			"scanning only. Fix the workspace Dolt endpoints, then re-run.", len(warnings))
}

// sandboxGCSourceEnv names the environment variable a scheduler sets to declare
// itself in every record this sweep writes.
//
// It is an environment variable rather than a flag deliberately: an older `agm`
// on the host ignores an unknown variable, whereas an unknown flag would turn
// an automated caller's remediation into a hard "unknown flag" failure the
// moment the caller was upgraded first.
const sandboxGCSourceEnv = "AGM_GC_SOURCE"

// logSandboxGCEntryTagged stamps the declared runner onto an entry before it is
// written. Readers use it to tell a scheduled sweep from one that some other
// component triggered on its own behalf; see gclog.Entry.Source.
func logSandboxGCEntryTagged(entry gclog.Entry) {
	if entry.Source == "" {
		entry.Source = strings.TrimSpace(os.Getenv(sandboxGCSourceEnv))
	}
	logSandboxGCEntry(entry)
}

// logSandboxGCEntryDefault appends one record to the shared GC log.
//
// A write failure is reported on stderr rather than swallowed. The completion
// record is the watchdog's only proof of life: if it never lands, the command
// still exits zero while disk-watchdog raises a dead-reaper alarm six hours
// later, and nothing anywhere says the observation channel itself broke.
func logSandboxGCEntryDefault(entry gclog.Entry) {
	logger, err := gclog.NewDefault()
	if err != nil {
		fmt.Fprintf(os.Stderr, "agm sandbox gc: WARNING: cannot open the GC log to record %q: %v\n", entry.Operation, err)
		return
	}
	if err := logger.Log(entry); err != nil {
		fmt.Fprintf(os.Stderr, "agm sandbox gc: WARNING: cannot record %q in the GC log: %v\n", entry.Operation, err)
	}
}
