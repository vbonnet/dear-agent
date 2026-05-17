package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	archiveUIOlderThan string
	archiveUIStatus    string
	archiveUIApply     bool
	archiveUIUnarchive bool
	archiveUIBackup    bool
	archiveUIJSON      bool
)

var archiveUICmd = &cobra.Command{
	Use:   "archive-ui",
	Short: "Archive idle Claude desktop / claude.ai/code UI sessions",
	Long: `Declutter the Claude desktop "Code" / claude.ai/code session list by
flipping the local "isArchived" flag in the desktop session store.

This is a DISTINCT verb from "agm session archive", which archives AGM's own
Dolt session manifests. archive-ui operates only on the local desktop store at
~/Library/Application Support/Claude/claude-code-sessions/.

Safety guarantees (ADR-026):
  - Dry-run by default; --apply is required to modify anything.
  - Never deletes a file; never touches ~/.claude/projects/*.jsonl transcripts.
  - Never calls a network API; no credential access.
  - Skips any session with a live process (PID registry), regardless of --status.
  - Per-file verbatim backup before mutation (--backup, on by default).
  - Reversible: --unarchive flips the flag back; the edit is byte-minimal.
  - Refuses files whose schema it does not recognize (never rewrites them).

"idle" means: no live process owns it AND it has been inactive longer than
--older-than.

Examples:
  # Preview what would be archived (idle > 7d). Default — mutates nothing.
  agm session archive-ui --older-than 7d --status idle

  # Actually archive that set, with per-file backups.
  agm session archive-ui --older-than 7d --status idle --apply

  # Reverse: unarchive everything currently archived.
  agm session archive-ui --status all --unarchive --apply

  # Machine-readable output for a skill/cron.
  agm session archive-ui --json`,
	RunE: runArchiveUI,
}

func runArchiveUI(cmd *cobra.Command, args []string) error {
	olderThan, err := parseDuration(archiveUIOlderThan)
	if err != nil {
		return err
	}

	opCtx := newOpContext()
	opCtx.DryRun = !archiveUIApply

	result, err := ops.ArchiveUISessions(opCtx, &ops.ArchiveUISessionsRequest{
		OlderThan: olderThan,
		Status:    archiveUIStatus,
		Unarchive: archiveUIUnarchive,
		Apply:     archiveUIApply,
		Backup:    archiveUIBackup,
	})
	if err != nil {
		return handleError(err)
	}

	useJSON := archiveUIJSON || isJSONOutput()
	if useJSON {
		return printJSON(result)
	}

	printArchiveUITable(result)
	return nil
}

func printArchiveUITable(r *ops.ArchiveUISessionsResult) {
	if r.DryRun {
		fmt.Printf("DRY RUN — no files were modified (use --apply to %s)\n\n", r.Direction)
	}

	if len(r.Sessions) > 0 {
		tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "AGE\tLIVE\tARCHIVED\tACTION\tTITLE\tCWD")
		for _, s := range r.Sessions {
			action := s.Action
			if s.Reason != "" {
				action = fmt.Sprintf("%s:%s", s.Action, s.Reason)
			}
			fmt.Fprintf(tw, "%s\t%s\t%t\t%s\t%s\t%s\n",
				humanAge(s.AgeHours), yesNo(s.Live), s.IsArchived,
				action, truncate(s.Title, 40), truncate(s.Cwd, 48))
		}
		tw.Flush()
		fmt.Println()
	}

	if r.BackupDir != "" {
		fmt.Printf("Backups: %s\n", r.BackupDir)
	}

	summary := fmt.Sprintf("Store %s — scanned %d, %s %d, skipped %d",
		r.Store, r.Scanned, changedVerb(r.DryRun, r.Direction), r.Changed, r.Skipped)
	if r.Errors > 0 {
		summary += fmt.Sprintf(", errors %d", r.Errors)
	}

	switch {
	case r.Errors > 0:
		ui.PrintWarning(summary)
	case r.DryRun:
		ui.PrintSuccess("Dry run: " + summary)
	default:
		ui.PrintSuccess(summary)
	}
}

func changedVerb(dryRun bool, direction string) string {
	if dryRun {
		return "would-" + direction
	}
	return direction + "d"
}

func humanAge(hours float64) string {
	switch {
	case hours >= 24:
		return fmt.Sprintf("%.0fd", hours/24)
	case hours >= 1:
		return fmt.Sprintf("%.0fh", hours)
	default:
		return fmt.Sprintf("%.0fm", hours*60)
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func init() {
	archiveUICmd.Flags().StringVar(&archiveUIOlderThan, "older-than", "7d",
		"Only consider sessions idle for at least this long (e.g. 7d, 24h, 1w)")
	archiveUICmd.Flags().StringVar(&archiveUIStatus, "status", "idle",
		"Which sessions to consider: idle (default) | all")
	archiveUICmd.Flags().BoolVar(&archiveUIApply, "apply", false,
		"Perform the flip. Omitted (default) = dry-run")
	archiveUICmd.Flags().BoolVar(&archiveUIUnarchive, "unarchive", false,
		"Reverse: flip isArchived true -> false (same filters)")
	archiveUICmd.Flags().BoolVar(&archiveUIBackup, "backup", true,
		"Back up each file verbatim before mutating")
	archiveUICmd.Flags().BoolVar(&archiveUIJSON, "json", false,
		"Machine-readable JSON output (for skill/cron/MCP)")
	sessionCmd.AddCommand(archiveUICmd)
}
