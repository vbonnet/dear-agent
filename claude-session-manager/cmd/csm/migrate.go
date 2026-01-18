package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

var (
	dryRunFlag    bool
	forceFlag     bool
	workspaceFlag string
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate sessions to unified storage",
	Long: `Migrate sessions from fragmented workspace locations to unified storage.

This command:
- Discovers sessions across all workspaces (~/src/ws/*/sessions/)
- Moves manifests and conversations to ~/src/sessions/{session-name}/
- Converts conversation formats (HTML → JSONL)
- Creates audit log of all operations

Examples:
  # Preview migration (no changes)
  agm migrate --to-unified-storage --dry-run

  # Migrate all sessions
  agm migrate --to-unified-storage

  # Migrate only 'oss' workspace sessions
  agm migrate --to-unified-storage --workspace=oss

  # Force overwrite of existing destinations
  agm migrate --to-unified-storage --force
`,
	RunE: runMigrate,
}

var migrateToUnifiedStorageCmd = &cobra.Command{
	Use:   "--to-unified-storage",
	Short: "Migrate to ~/src/sessions/{session-name}/ layout",
	RunE:  runMigrate,
}

func init() {
	migrateCmd.AddCommand(migrateToUnifiedStorageCmd)

	// Flags for migration
	migrateToUnifiedStorageCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Preview changes without modifying files")
	migrateToUnifiedStorageCmd.Flags().BoolVar(&forceFlag, "force", false, "Overwrite existing destinations")
	migrateToUnifiedStorageCmd.Flags().StringVar(&workspaceFlag, "workspace", "", "Migrate only specified workspace (e.g., 'oss')")

	// Add to root command (assumes rootCmd exists in main.go)
	// rootCmd.AddCommand(migrateCmd)
}

func runMigrate(cmd *cobra.Command, args []string) error {
	fmt.Println("Discovering sessions across workspaces...")

	// Find all sessions
	locations, err := discovery.FindSessionsAcrossWorkspaces()
	if err != nil {
		return fmt.Errorf("session discovery failed: %w", err)
	}

	if len(locations) == 0 {
		fmt.Println("No sessions found to migrate.")
		return nil
	}

	fmt.Printf("Found %d sessions\n", len(locations))

	// Configure migration
	opts := manifest.MigrationOptions{
		DryRun:    dryRunFlag,
		Force:     forceFlag,
		Workspace: workspaceFlag,
	}

	if opts.DryRun {
		fmt.Println("[DRY-RUN MODE] No files will be modified")
	}

	// Execute migration
	fmt.Println("Migrating sessions...")
	report, err := manifest.MigrateToUnifiedStorage(convertLocations(locations), opts)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Print summary
	fmt.Println("\nMigration complete:")
	fmt.Printf("  ✅ Succeeded: %d\n", report.Succeeded)
	if report.Skipped > 0 {
		fmt.Printf("  ⏭  Skipped:   %d (already migrated, use --force to overwrite)\n", report.Skipped)
	}
	if report.Failed > 0 {
		fmt.Printf("  ❌ Failed:    %d\n", report.Failed)
		fmt.Println("\nErrors:")
		for _, migErr := range report.Errors {
			fmt.Printf("  - Session %s (%s): %s\n", migErr.SessionName, migErr.SessionID, migErr.Error)
		}
		os.Exit(1)
	}

	if !opts.DryRun {
		fmt.Println("\n✅ Migration successful!")
		fmt.Println("\nOld session directories preserved for 30 days (rollback safety).")
		fmt.Println("AGM will automatically read from new locations.")
	}

	return nil
}

// convertLocations converts discovery.SessionLocation to manifest.SessionLocation
// (In production, these would share a common type to avoid this conversion)
func convertLocations(locations []discovery.SessionLocation) []manifest.SessionLocation {
	result := make([]manifest.SessionLocation, len(locations))
	for i, loc := range locations {
		result[i] = manifest.SessionLocation{
			Workspace:       loc.Workspace,
			SessionID:       loc.SessionID,
			Name:            loc.Name,
			ManifestPath:    loc.ManifestPath,
			ConversationDir: loc.ConversationDir,
		}
	}
	return result
}
