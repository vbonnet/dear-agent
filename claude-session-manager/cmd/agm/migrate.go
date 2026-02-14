package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/db"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/migrate"
)

var (
	dryRunFlag     bool
	forceFlag      bool
	workspaceFlag  string
	manifestDirFlag string
	dbPathFlag     string
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

var migrateYAMLToSQLiteCmd = &cobra.Command{
	Use:   "migrate-yaml-to-sqlite",
	Short: "Migrate YAML manifests to SQLite database",
	Long: `Migrate existing YAML session manifests to SQLite database.

This command:
- Scans manifest directory for *.yaml files
- Imports each manifest into SQLite database
- Validates all fields migrated correctly
- Reports migration statistics

Examples:
  # Migrate with default paths
  agm migrate migrate-yaml-to-sqlite

  # Specify custom paths
  agm migrate migrate-yaml-to-sqlite --manifest-dir ~/.agm/sessions --db-path ~/.agm/sessions.db

  # Preview migration (no changes)
  agm migrate migrate-yaml-to-sqlite --dry-run

  # View progress during migration
  agm migrate migrate-yaml-to-sqlite --manifest-dir ~/sessions
`,
	RunE: runMigrateYAMLToSQLite,
}

func init() {
	migrateCmd.AddCommand(migrateToUnifiedStorageCmd)
	migrateCmd.AddCommand(migrateYAMLToSQLiteCmd)

	// Flags for unified storage migration
	migrateToUnifiedStorageCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Preview changes without modifying files")
	migrateToUnifiedStorageCmd.Flags().BoolVar(&forceFlag, "force", false, "Overwrite existing destinations")
	migrateToUnifiedStorageCmd.Flags().StringVar(&workspaceFlag, "workspace", "", "Migrate only specified workspace (e.g., 'oss')")

	// Flags for YAML to SQLite migration
	home, _ := os.UserHomeDir()
	defaultManifestDir := filepath.Join(home, ".agm", "sessions")
	defaultDBPath := filepath.Join(home, ".agm", "sessions.db")

	migrateYAMLToSQLiteCmd.Flags().StringVar(&manifestDirFlag, "manifest-dir", defaultManifestDir, "Directory containing YAML manifests")
	migrateYAMLToSQLiteCmd.Flags().StringVar(&dbPathFlag, "db-path", defaultDBPath, "SQLite database path")
	migrateYAMLToSQLiteCmd.Flags().BoolVar(&dryRunFlag, "dry-run", false, "Preview migration without writing to database")

	// Add to root command
	rootCmd.AddCommand(migrateCmd)
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

func runMigrateYAMLToSQLite(cmd *cobra.Command, args []string) error {
	fmt.Println("YAML to SQLite Migration")
	fmt.Println("========================")
	fmt.Println()

	if dryRunFlag {
		fmt.Println("[DRY-RUN MODE] No database changes will be made")
		fmt.Println()
	}

	// Display configuration
	fmt.Printf("Manifest directory: %s\n", manifestDirFlag)
	fmt.Printf("Database path:      %s\n", dbPathFlag)
	fmt.Println()

	// Read YAML manifests
	fmt.Println("Scanning for YAML manifests...")
	manifests, err := migrate.ReadYAMLManifests(manifestDirFlag)
	if err != nil {
		return fmt.Errorf("failed to read manifests: %w", err)
	}

	if len(manifests) == 0 {
		fmt.Println("No YAML manifests found.")
		fmt.Printf("(Searched in: %s)\n", manifestDirFlag)
		return nil
	}

	fmt.Printf("Found %d manifest(s)\n\n", len(manifests))

	// Open or create SQLite database
	var database *db.DB
	if !dryRunFlag {
		// Ensure parent directory exists
		dbDir := filepath.Dir(dbPathFlag)
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}

		database, err = db.Open(dbPathFlag)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()
	} else {
		// For dry-run, use in-memory database for validation
		database, err = db.Open(":memory:")
		if err != nil {
			return fmt.Errorf("failed to open in-memory database: %w", err)
		}
		defer database.Close()
	}

	// Migrate manifests
	fmt.Println("Migrating manifests...")
	result, err := migrate.MigrateAll(database, manifests, dryRunFlag)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	// Progress reporting
	for i, m := range manifests {
		status := "✓"
		if result.ErrorCount > 0 {
			// Check if this specific manifest had an error
			for _, migErr := range result.Errors {
				if migErr.FilePath == m.SessionID {
					status = "✗"
					break
				}
			}
		}
		fmt.Printf("  %s Migrating %d of %d: %s (%s)\n",
			status, i+1, len(manifests), m.Name, m.SessionID)
	}
	fmt.Println()

	// Summary
	fmt.Println("Migration Summary")
	fmt.Println("-----------------")
	fmt.Printf("Total manifests:    %d\n", result.TotalFiles)
	fmt.Printf("✓ Migrated:         %d\n", result.SuccessCount)
	if result.SkippedCount > 0 {
		fmt.Printf("⏭  Skipped:          %d (already exist)\n", result.SkippedCount)
	}
	if result.ErrorCount > 0 {
		fmt.Printf("✗ Failed:           %d\n", result.ErrorCount)
	}
	fmt.Println()

	// Display errors if any
	if result.ErrorCount > 0 {
		fmt.Println("Errors:")
		for _, migErr := range result.Errors {
			fmt.Printf("  - %s: %v\n", migErr.FilePath, migErr.Error)
		}
		fmt.Println()
		return fmt.Errorf("migration completed with %d error(s)", result.ErrorCount)
	}

	// Success message
	if dryRunFlag {
		fmt.Println("✓ Dry-run completed successfully")
		fmt.Println("  Run without --dry-run to perform actual migration")
	} else {
		fmt.Println("✓ Migration completed successfully")
		fmt.Printf("  Database: %s\n", dbPathFlag)
	}

	return nil
}
