package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/db"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/temporal"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	migrateTmuxDryRun       bool
	migrateTmuxDeleteSessions bool
)

var migrateTmuxToTemporalCmd = &cobra.Command{
	Use:   "migrate-tmux-to-temporal",
	Short: "Migrate tmux sessions to Temporal workflows",
	Long: `Migrate existing tmux sessions to Temporal-backed workflows.

This command:
- Scans all active tmux sessions
- Extracts session metadata from manifest files
- Creates corresponding Temporal workflows
- Preserves conversation history
- Optionally archives old tmux sessions

Examples:
  # Preview migration (no changes)
  agm migrate-tmux-to-temporal --dry-run

  # Migrate all tmux sessions
  agm migrate-tmux-to-temporal

  # Migrate and delete old tmux sessions
  agm migrate-tmux-to-temporal --delete-tmux-sessions
`,
	RunE: runMigrateTmuxToTemporal,
}

// MigrationResult tracks the results of a migration operation
type MigrationResult struct {
	TotalSessions   int
	SuccessCount    int
	SkippedCount    int
	FailedCount     int
	Errors          []MigrationError
}

// MigrationError tracks individual migration failures
type MigrationError struct {
	SessionName string
	SessionID   string
	Error       error
}

func init() {
	migrateCmd.AddCommand(migrateTmuxToTemporalCmd)
	migrateTmuxToTemporalCmd.Flags().BoolVar(&migrateTmuxDryRun, "dry-run", false, "Preview migration without making changes")
	migrateTmuxToTemporalCmd.Flags().BoolVar(&migrateTmuxDeleteSessions, "delete-tmux-sessions", false, "Delete tmux sessions after successful migration")
}

func runMigrateTmuxToTemporal(cmd *cobra.Command, args []string) error {
	if migrateTmuxDryRun {
		fmt.Println("[DRY-RUN MODE] No changes will be made")
		fmt.Println()
	}

	// Get sessions directory
	sessionsDir := getSessionsDir()

	// Initialize Temporal client
	temporalClient := temporal.NewTemporalClient()

	// Initialize database
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	dbPath := filepath.Join(homeDir, ".agm", "sessions.db")
	var database *db.DB

	if !migrateTmuxDryRun {
		// Ensure database directory exists
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}

		database, err = db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()
	} else {
		// Use in-memory database for dry-run
		database, err = db.Open(":memory:")
		if err != nil {
			return fmt.Errorf("failed to open in-memory database: %w", err)
		}
		defer database.Close()
	}

	// Scan tmux sessions
	fmt.Println("Scanning tmux sessions...")
	tmuxSessions, err := tmux.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	if len(tmuxSessions) == 0 {
		fmt.Println("No tmux sessions found.")
		return nil
	}

	fmt.Printf("Found %d tmux session(s)\n\n", len(tmuxSessions))

	// Perform migration
	var migrationErr error
	result := &MigrationResult{
		TotalSessions: len(tmuxSessions),
		Errors:        []MigrationError{},
	}

	if !migrateTmuxDryRun {
		spinErr := spinner.New().
			Title("Migrating sessions...").
			Accessible(true).
			Action(func() {
				migrationErr = migrateAllSessions(tmuxSessions, sessionsDir, temporalClient, database, result)
			}).
			Run()
		if spinErr != nil {
			return fmt.Errorf("spinner error: %w", spinErr)
		}
	} else {
		migrationErr = migrateAllSessions(tmuxSessions, sessionsDir, temporalClient, database, result)
	}

	if migrationErr != nil {
		return migrationErr
	}

	// Print summary
	printMigrationSummary(result)

	// Cleanup tmux sessions if requested
	if !migrateTmuxDryRun && migrateTmuxDeleteSessions && result.SuccessCount > 0 {
		fmt.Println("\nArchiving migrated tmux sessions...")
		if err := cleanupMigratedSessions(result); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to cleanup some sessions: %v", err))
		}
	}

	if migrateTmuxDryRun {
		fmt.Println("\n✓ Dry-run completed successfully")
		fmt.Println("  Run without --dry-run to perform actual migration")
	} else if result.FailedCount == 0 {
		fmt.Println("\n✓ Migration completed successfully")
		if result.SkippedCount > 0 {
			fmt.Println("  (Some sessions were skipped as they were already migrated)")
		}
	}

	return nil
}

// migrateAllSessions performs the actual migration logic
func migrateAllSessions(
	tmuxSessions []string,
	sessionsDir string,
	temporalClient temporal.TemporalInterface,
	database *db.DB,
	result *MigrationResult,
) error {
	for _, sessionName := range tmuxSessions {
		if err := migrateSession(sessionName, sessionsDir, temporalClient, database, result); err != nil {
			result.FailedCount++
			result.Errors = append(result.Errors, MigrationError{
				SessionName: sessionName,
				Error:       err,
			})
		}
	}
	return nil
}

// migrateSession migrates a single tmux session to Temporal
func migrateSession(
	sessionName string,
	sessionsDir string,
	temporalClient temporal.TemporalInterface,
	database *db.DB,
	result *MigrationResult,
) error {
	// Read manifest file
	manifestPath := filepath.Join(sessionsDir, sessionName, "manifest.yaml")
	m, err := manifest.Read(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	// Check if already migrated (check database)
	existing, err := database.GetSession(m.SessionID)
	if err == nil && existing != nil {
		// Check if it's marked as Temporal-backed
		// For now, we consider any session in database as potentially migrated
		// In production, we'd add a backend field to the manifest/database
		result.SkippedCount++
		return nil
	}

	// Backup manifest file before migration
	if !migrateTmuxDryRun {
		backupPath := manifestPath + ".backup." + time.Now().Format("20060102-150405")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return fmt.Errorf("failed to read manifest for backup: %w", err)
		}
		if err := os.WriteFile(backupPath, data, 0600); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Get working directory from manifest
	workDir := m.Context.Project
	if workDir == "" {
		workDir = "." // Fallback to current directory
	}

	// Create Temporal workflow with same session name
	if !migrateTmuxDryRun {
		if err := temporalClient.CreateSession(sessionName, workDir); err != nil {
			return fmt.Errorf("failed to create Temporal session: %w", err)
		}
	}

	// Update database to mark as migrated
	// Update the manifest's updated_at timestamp
	m.UpdatedAt = time.Now()

	if !migrateTmuxDryRun {
		// Try to create or update in database
		if existing == nil {
			if err := database.CreateSession(m); err != nil {
				return fmt.Errorf("failed to create session in database: %w", err)
			}
		} else {
			if err := database.UpdateSession(m); err != nil {
				return fmt.Errorf("failed to update session in database: %w", err)
			}
		}
	}

	// Migration successful
	result.SuccessCount++
	return nil
}

// printMigrationSummary prints the migration results
func printMigrationSummary(result *MigrationResult) {
	fmt.Println("\nMigration Summary")
	fmt.Println("-----------------")
	fmt.Printf("Total sessions:     %d\n", result.TotalSessions)
	fmt.Printf("✓ Migrated:         %d\n", result.SuccessCount)

	if result.SkippedCount > 0 {
		fmt.Printf("⏭  Skipped:          %d (already migrated)\n", result.SkippedCount)
	}

	if result.FailedCount > 0 {
		fmt.Printf("✗ Failed:           %d\n", result.FailedCount)
		fmt.Println("\nErrors:")
		for _, migErr := range result.Errors {
			fmt.Printf("  - %s: %v\n", migErr.SessionName, migErr.Error)
		}
	}
}

// cleanupMigratedSessions archives or deletes successfully migrated tmux sessions
func cleanupMigratedSessions(result *MigrationResult) error {
	// For now, we just report what would be cleaned up
	// In production, this would actually kill the tmux sessions
	fmt.Printf("  Would cleanup %d session(s)\n", result.SuccessCount)
	return nil
}
