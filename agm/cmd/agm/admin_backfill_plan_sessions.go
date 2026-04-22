package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var (
	backfillDryRun bool
	backfillApply  bool
)

var backfillPlanSessionsCmd = &cobra.Command{
	Use:   "backfill-plan-sessions",
	Short: "Find and fix orphaned execution sessions by linking them to planning parents",
	Long: `Find orphaned execution sessions and link them to their parent planning sessions.

This command identifies execution sessions created by "Clear Context and Execute Plan" that are
missing their parent_session_id link. It searches for planning sessions created 1-30 seconds
before each orphaned session, matching on CWD (Context.Project) and workspace.

The command operates in two modes:
  --dry-run: Preview changes without modifying the database (default if neither flag specified)
  --apply:   Execute changes and update the database

For each match found:
  - Sets child.ParentSessionID = parent.SessionID
  - Sets child.Name = parent.Name + "-exec"
  - Updates session in Dolt database`,
	Example: `  # Preview changes without applying
  agm admin backfill-plan-sessions --dry-run

  # Execute changes
  agm admin backfill-plan-sessions --apply`,
	RunE: runBackfillPlanSessions,
}

func init() {
	adminCmd.AddCommand(backfillPlanSessionsCmd)

	backfillPlanSessionsCmd.Flags().BoolVar(&backfillDryRun, "dry-run", false,
		"Preview changes without modifying database")
	backfillPlanSessionsCmd.Flags().BoolVar(&backfillApply, "apply", false,
		"Execute changes and update database")
}

func runBackfillPlanSessions(cmd *cobra.Command, args []string) error {
	// Validate flags: require exactly one of --dry-run or --apply
	if backfillDryRun && backfillApply {
		return fmt.Errorf("cannot use both --dry-run and --apply flags")
	}

	// Default to dry-run if neither flag specified
	if !backfillDryRun && !backfillApply {
		backfillDryRun = true
		fmt.Println(ui.Yellow("ℹ No mode specified, defaulting to --dry-run"))
		fmt.Println()
	}

	// Connect to Dolt storage
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	// Get all sessions
	allSessions, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	fmt.Printf("Found %d total sessions, scanning for orphaned executions...\n\n", len(allSessions))

	// Find orphaned execution sessions and their potential parents
	type parentChildPair struct {
		parent *struct {
			SessionID string
			Name      string
			CreatedAt time.Time
			CWD       string
		}
		child *struct {
			SessionID string
			Name      string
			CreatedAt time.Time
			CWD       string
		}
		timeDiff time.Duration
	}

	var pairs []parentChildPair

	// Identify orphaned children (no parent_session_id, regardless of name)
	for _, child := range allSessions {
		// Skip if already has parent
		if child.ParentSessionID != nil && *child.ParentSessionID != "" {
			continue
		}

		// Search for potential parent: created 1-30 seconds before, same CWD, has a name
		var bestParent *struct {
			SessionID string
			Name      string
			CreatedAt time.Time
			CWD       string
		}
		var bestTimeDiff time.Duration

		for _, parent := range allSessions {
			// Must have a name
			if parent.Name == "" || parent.Name == "Unknown" {
				continue
			}

			// Skip if it's the child itself
			if parent.SessionID == child.SessionID {
				continue
			}

			// Must match CWD
			if parent.Context.Project != child.Context.Project {
				continue
			}

			// Must be created before child
			timeDiff := child.CreatedAt.Sub(parent.CreatedAt)
			if timeDiff < 1*time.Second || timeDiff > 30*time.Second {
				continue
			}

			// Keep the most recent parent (closest in time)
			if bestParent == nil || timeDiff < bestTimeDiff {
				bestParent = &struct {
					SessionID string
					Name      string
					CreatedAt time.Time
					CWD       string
				}{
					SessionID: parent.SessionID,
					Name:      parent.Name,
					CreatedAt: parent.CreatedAt,
					CWD:       parent.Context.Project,
				}
				bestTimeDiff = timeDiff
			}
		}

		// If we found a parent, record the pair
		if bestParent != nil {
			pairs = append(pairs, parentChildPair{
				parent: bestParent,
				child: &struct {
					SessionID string
					Name      string
					CreatedAt time.Time
					CWD       string
				}{
					SessionID: child.SessionID,
					Name:      child.Name,
					CreatedAt: child.CreatedAt,
					CWD:       child.Context.Project,
				},
				timeDiff: bestTimeDiff,
			})
		}
	}

	// Display results
	if len(pairs) == 0 {
		ui.PrintSuccess("No orphaned execution sessions found")
		return nil
	}

	fmt.Printf("Found %d orphaned execution session(s) with parent candidates:\n\n", len(pairs))

	// Print table of pairs
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PARENT\tCHILD\tTIME GAP\tCWD")
	fmt.Fprintln(w, "------\t-----\t--------\t---")

	for _, pair := range pairs {
		parentID := pair.parent.SessionID[:8]
		childID := pair.child.SessionID[:8]
		parentName := pair.parent.Name
		if len(parentName) > 30 {
			parentName = parentName[:27] + "..."
		}
		cwd := pair.parent.CWD
		if len(cwd) > 40 {
			// Show end of path (most relevant)
			cwd = "..." + cwd[len(cwd)-37:]
		}

		fmt.Fprintf(w, "%s (%s)\t%s (Unknown)\t%.1fs\t%s\n",
			parentID, parentName, childID, pair.timeDiff.Seconds(), cwd)
	}
	w.Flush()
	fmt.Println()

	// If dry-run, stop here
	if backfillDryRun {
		fmt.Println(ui.Blue("ℹ Dry-run mode: No changes made"))
		fmt.Println(ui.Blue("  Run with --apply to execute these changes"))
		return nil
	}

	// Apply mode: update each child session
	fmt.Println(ui.Yellow("Applying changes..."))
	fmt.Println()

	successCount := 0
	failureCount := 0

	for i, pair := range pairs {
		fmt.Printf("[%d/%d] Linking %s → %s...\n",
			i+1, len(pairs), pair.child.SessionID[:8], pair.parent.SessionID[:8])

		// Fetch full child session
		child, err := adapter.GetSession(pair.child.SessionID)
		if err != nil {
			fmt.Printf("  %s Failed to load child session: %v\n", ui.Red("✗"), err)
			failureCount++
			continue
		}

		// Update child session
		child.ParentSessionID = &pair.parent.SessionID
		child.Name = pair.parent.Name + "-exec"

		if err := adapter.UpdateSession(child); err != nil {
			fmt.Printf("  %s Failed to update session: %v\n", ui.Red("✗"), err)
			failureCount++
			continue
		}

		fmt.Printf("  %s Set parent_session_id = %s\n", ui.Green("✓"), pair.parent.SessionID[:8])
		fmt.Printf("  %s Set name = '%s'\n", ui.Green("✓"), child.Name)
		successCount++
	}

	fmt.Println()

	// Print summary
	if successCount > 0 {
		ui.PrintSuccess(fmt.Sprintf("Successfully linked %d session(s)", successCount))
	}
	if failureCount > 0 {
		fmt.Printf("%s Failed to link %d session(s)\n", ui.Red("✗"), failureCount)
	}

	return nil
}
