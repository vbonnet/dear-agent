package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/migration"
)

// MigrateAllCmd migrates all projects in a workspace from V1 to V2
var MigrateAllCmd = &cobra.Command{
	Use:   "migrate-all [workspace-root]",
	Short: "Migrate all V1 projects to V2 in a workspace",
	Long: `Migrate all Wayfinder V1 projects to V2 schema in a workspace.

This command scans the workspace for all WAYFINDER-STATUS.md files,
identifies V1 projects, and migrates them to the V2 schema.

Features:
- Creates backup of V1 status files before migration
- Validates V2 schema after conversion
- Reports progress during migration
- Generates summary report with statistics
- Supports dry-run mode to preview changes
- Supports parallel migration for faster processing

The workspace root is typically ~/src/ws/{workspace}/wf/

Examples:
  # Migrate all projects in default workspace
  wayfinder-session migrate-all ~/src/ws/oss/wf

  # Dry-run to preview changes
  wayfinder-session migrate-all ~/src/ws/oss/wf --dry-run

  # Parallel migration with 8 workers
  wayfinder-session migrate-all ~/src/ws/oss/wf --parallel --workers 8

Flags:
  --dry-run         Preview changes without modifying files
  --parallel        Migrate projects in parallel (faster for many projects)
  --workers         Number of parallel workers (default: 4)

Exit codes:
  0 - All projects migrated successfully
  1 - One or more projects failed to migrate
  2 - No projects found to migrate`,
	Args: cobra.ExactArgs(1),
	RunE: runMigrateAll,
}

func init() {
	MigrateAllCmd.Flags().Bool("dry-run", false, "Preview changes without modifying files")
	MigrateAllCmd.Flags().Bool("parallel", false, "Migrate projects in parallel")
	MigrateAllCmd.Flags().Int("workers", 4, "Number of parallel workers (only with --parallel)")
}

func runMigrateAll(cmd *cobra.Command, args []string) error {
	workspaceRoot := args[0]

	// Get flags
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	parallel, _ := cmd.Flags().GetBool("parallel")
	workers, _ := cmd.Flags().GetInt("workers")

	// Validate workspace root exists
	if _, err := os.Stat(workspaceRoot); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("workspace root does not exist: %s", workspaceRoot)
		}
		return fmt.Errorf("failed to check workspace root: %w", err)
	}

	// Print header
	fmt.Println("Wayfinder V1 → V2 Batch Migration")
	fmt.Println("==================================")
	fmt.Printf("Workspace: %s\n", workspaceRoot)
	if dryRun {
		fmt.Println("Mode: DRY RUN (no files will be modified)")
	} else {
		fmt.Println("Mode: LIVE MIGRATION")
	}
	if parallel {
		fmt.Printf("Parallelism: %d workers\n", workers)
	} else {
		fmt.Println("Parallelism: Sequential")
	}
	fmt.Println()

	// Configure migration options
	options := migration.BatchMigrationOptions{
		WorkspaceRoot: workspaceRoot,
		DryRun:        dryRun,
		Parallel:      parallel,
		MaxWorkers:    workers,
	}

	// Run migration
	report, err := migration.MigrateAll(options)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Print summary report
	report.PrintReport()

	// Determine exit code
	if report.TotalProjects == 0 {
		fmt.Println("\nNo V1 projects found to migrate.")
		return fmt.Errorf("exit code 2: no projects found")
	}

	if report.FailureCount > 0 {
		fmt.Printf("\n⚠ Warning: %d projects failed to migrate\n", report.FailureCount)
		return fmt.Errorf("exit code 1: %d projects failed", report.FailureCount)
	}

	if dryRun {
		fmt.Printf("\n✓ Dry-run complete: %d projects would be migrated successfully\n", report.SuccessCount)
	} else {
		fmt.Printf("\n✓ Migration complete: %d projects migrated successfully\n", report.SuccessCount)
	}

	return nil
}
