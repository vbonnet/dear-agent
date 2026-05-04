package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/orphan"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	findOrphansWorkspace  string
	findOrphansAutoImport bool
)

var findOrphansCmd = &cobra.Command{
	Use:   "find-orphans",
	Short: "Detect orphaned conversations across all workspaces",
	Long: `Scan for conversations that exist in Claude history but have no AGM manifest.

Orphaned sessions occur when:
- AGM crashes during session creation
- Workspace refactoring removes manifest files
- Manual deletion of session directories

This command scans ~/.claude/history.jsonl and compares UUIDs against
AGM manifests in the sessions directory. Orphaned sessions can be imported
to restore full AGM tracking.

Examples:
  agm admin find-orphans                    # Detect all orphans
  agm admin find-orphans --workspace oss    # Detect orphans in oss workspace
  agm admin find-orphans --auto-import      # Prompt to import each orphan

Output:
  UUID       | Project Path           | Last Modified        | Workspace  | Status
  -----------|------------------------|---------------------|------------|----------
  abc123...  | ~/src/project-x        | 2026-02-17 14:32    | oss        | orphaned`,
	RunE: runFindOrphans,
}

func init() {
	adminCmd.AddCommand(findOrphansCmd)

	findOrphansCmd.Flags().StringVar(&findOrphansWorkspace, "workspace", "",
		"Filter orphans to specific workspace")
	findOrphansCmd.Flags().BoolVar(&findOrphansAutoImport, "auto-import", false,
		"Prompt to import each orphaned session")
}

func runFindOrphans(cmd *cobra.Command, args []string) error {
	fmt.Println(ui.Blue("=== Orphaned Session Detection ===\n"))

	// Get sessions directory from config
	sessionsDir := cfg.SessionsDir
	if sessionsDir == "" {
		homeDir, _ := os.UserHomeDir()
		sessionsDir = homeDir + "/sessions"
	}

	fmt.Printf("Scanning sessions directory: %s\n", sessionsDir)
	if findOrphansWorkspace != "" {
		fmt.Printf("Workspace filter: %s\n", findOrphansWorkspace)
	}
	fmt.Println()

	// Get Dolt adapter for orphan detection
	adapter, _ := getStorage()
	if adapter != nil {
		defer adapter.Close()
	}

	// Run orphan detection
	report, err := orphan.DetectOrphans(sessionsDir, findOrphansWorkspace, adapter)
	if err != nil {
		return fmt.Errorf("failed to detect orphans: %w", err)
	}

	// Display scan statistics
	scanDuration := report.ScanCompleted.Sub(report.ScanStarted)
	fmt.Printf("Scan completed in %v\n", scanDuration.Round(time.Millisecond))
	fmt.Printf("History entries scanned: %d\n", report.HistoryEntries)
	fmt.Printf("Manifests found: %d\n", report.ManifestsFound)
	fmt.Printf("Orphaned sessions: %s\n\n", ui.Yellow(fmt.Sprintf("%d", report.TotalOrphans)))

	// Display errors if any
	if len(report.Errors) > 0 {
		fmt.Println(ui.Yellow("Warnings during scan:"))
		for _, e := range report.Errors {
			fmt.Printf("  • %s: %v\n", e.Path, e.Error)
		}
		fmt.Println()
	}

	// If no orphans found, exit early
	if report.TotalOrphans == 0 {
		ui.PrintSuccess("No orphaned sessions found")
		return nil
	}

	// Display orphans table
	fmt.Println(ui.Blue("Orphaned Sessions:"))
	fmt.Println()
	displayOrphansTable(report.Orphans)

	// Display workspace summary
	if len(report.ByWorkspace) > 1 || findOrphansWorkspace == "" {
		fmt.Println()
		fmt.Println(ui.Blue("By Workspace:"))
		for workspace, count := range report.ByWorkspace {
			wsName := workspace
			if wsName == "" {
				wsName = "(no workspace)"
			}
			fmt.Printf("  • %s: %d orphan(s)\n", wsName, count)
		}
	}

	// Auto-import mode
	if findOrphansAutoImport {
		fmt.Println()
		runAutoImport(report.Orphans)
		return nil
	}

	// Suggest remediation
	fmt.Println()
	fmt.Println(ui.Blue("Next Steps:"))
	fmt.Println("  • Run 'agm admin find-orphans --auto-import' to import orphans")
	fmt.Println("  • Run 'agm session import <uuid>' to import a specific orphan")

	return nil
}

func displayOrphansTable(orphans []*orphan.OrphanedSession) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header
	fmt.Fprintln(w, "UUID\tProject Path\tLast Modified\tWorkspace\tStatus")
	fmt.Fprintln(w, "----\t------------\t-------------\t---------\t------")

	// Rows
	for _, o := range orphans {
		uuid := o.UUID
		if len(uuid) > 8 {
			uuid = uuid[:8] + "..."
		}

		projectPath := o.ProjectPath
		if projectPath == "" {
			projectPath = "(unknown)"
		}
		// Shorten project path for display
		if len(projectPath) > 30 {
			projectPath = "..." + projectPath[len(projectPath)-27:]
		}

		lastModified := o.LastModified.Format("2006-01-02 15:04")
		if o.LastModified.IsZero() {
			lastModified = "(unknown)"
		}

		workspace := o.Workspace
		if workspace == "" {
			workspace = "-"
		}

		status := o.Status

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			uuid, projectPath, lastModified, workspace, status)
	}
}

func runAutoImport(orphans []*orphan.OrphanedSession) {
	fmt.Println(ui.Blue("=== Auto-Import Mode ==="))
	fmt.Printf("Found %d orphaned session(s)\n\n", len(orphans))

	// TODO: Implement auto-import logic
	// This will be implemented in Task 1.2 (agm session import)
	// For now, show a message that it's not yet implemented

	fmt.Println(ui.Yellow("Auto-import not yet implemented"))
	fmt.Println("This feature will be available in Task 1.2 (agm session import)")
	fmt.Println()
	fmt.Println("For now, manually import orphans using:")
	for i, o := range orphans {
		if i >= 3 {
			fmt.Printf("  ... and %d more\n", len(orphans)-3)
			break
		}
		fmt.Printf("  agm session import %s\n", o.UUID)
	}
}
