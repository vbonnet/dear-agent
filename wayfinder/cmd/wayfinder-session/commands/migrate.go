package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/migrate"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

var (
	migrateProjectName     string
	migrateProjectType     string
	migrateRiskLevel       string
	migrateDryRun          bool
	migratePreserveSession bool
	migrateBackup          bool
	migrateVerbose         bool
)

// MigrateCmd converts V1 WAYFINDER-STATUS.md to V2 schema
var MigrateCmd = &cobra.Command{
	Use:   "migrate [path]",
	Short: "Migrate V1 WAYFINDER-STATUS.md to V2 schema",
	Long: `Migrates a V1 WAYFINDER-STATUS.md file to the V2 schema.

The migration process:
  1. Reads and validates the V1 status file
  2. Maps 13 V1 phases to 9 V2 phases
  3. Merges phases (S4→D4, S5→S6, S8/S9/S10→S8)
  4. Preserves all data (timestamps, outcomes, etc.)
  5. Writes the V2 status file

Phase Mapping:
  W0        → W0        (unchanged)
  D1-D4     → D1-D4     (unchanged)
  S4        → D4        (merged - stakeholder approval)
  S5        → S6        (merged - research notes)
  S6        → S6        (unchanged)
  S7        → S7        (unchanged)
  S8        → S8        (BUILD phase)
  S9        → S8        (merged - validation status)
  S10       → S8        (merged - deployment status)
  S11       → S11       (unchanged)

Examples:
  # Migrate with dry-run (preview changes)
  wayfinder-session migrate /path/to/project --dry-run

  # Migrate with custom metadata
  wayfinder-session migrate . --project-name "My Project" --risk-level L

  # Migrate with backup
  wayfinder-session migrate . --backup

  # Preserve V1 session ID as tag
  wayfinder-session migrate . --preserve-session-id
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runMigrate,
}

func init() {
	MigrateCmd.Flags().StringVar(&migrateProjectName, "project-name", "", "Override project name (default: derived from path)")
	MigrateCmd.Flags().StringVar(&migrateProjectType, "project-type", "", "Set project type (feature, bugfix, refactor, infrastructure)")
	MigrateCmd.Flags().StringVar(&migrateRiskLevel, "risk-level", "", "Set risk level (XS, S, M, L, XL)")
	MigrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Preview changes without modifying files")
	MigrateCmd.Flags().BoolVar(&migratePreserveSession, "preserve-session-id", false, "Preserve V1 session ID as a tag")
	MigrateCmd.Flags().BoolVar(&migrateBackup, "backup", true, "Create backup of V1 file before migration")
	MigrateCmd.Flags().BoolVar(&migrateVerbose, "verbose", false, "Show detailed migration report")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	// Determine project path
	projectPath := "."
	if len(args) > 0 {
		projectPath = args[0]
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	statusFile := filepath.Join(absPath, status.StatusFilename)

	// Check if status file exists
	if _, err := os.Stat(statusFile); os.IsNotExist(err) {
		return fmt.Errorf("WAYFINDER-STATUS.md not found in %s", absPath)
	}

	// Detect schema version
	version, err := status.DetectSchemaVersion(statusFile)
	if err != nil {
		return fmt.Errorf("failed to detect schema version: %w", err)
	}

	// Check if already V2
	if version == status.SchemaVersionV2 {
		fmt.Println("✓ Already using V2 schema, no migration needed")
		return nil
	}

	// Read V1 status
	fmt.Printf("Reading V1 status from %s...\n", absPath)
	v1Status, err := status.ReadFrom(absPath)
	if err != nil {
		return fmt.Errorf("failed to read V1 status: %w", err)
	}

	// Prepare conversion options
	opts := &migrate.ConvertOptions{
		DryRun:            migrateDryRun,
		ProjectName:       migrateProjectName,
		ProjectType:       migrateProjectType,
		RiskLevel:         migrateRiskLevel,
		PreserveSessionID: migratePreserveSession,
	}

	// Validate options
	if err := validateMigrateOptions(opts); err != nil {
		return err
	}

	// Convert to V2
	fmt.Println("Converting to V2 schema...")
	v2Status, err := migrate.ConvertV1ToV2(v1Status, opts)
	if err != nil {
		return fmt.Errorf("failed to convert to V2: %w", err)
	}

	// Generate detailed report
	report := migrate.GenerateReport(v1Status, v2Status, opts)

	// Display migration summary or full report
	if migrateDryRun || migrateVerbose {
		fmt.Println(report.String())
		if migrateDryRun {
			fmt.Println("\n✓ Dry-run complete (no files modified)")
			return nil
		}
	} else {
		printMigrationSummary(v1Status, v2Status)
	}

	// Create backup if requested
	if migrateBackup {
		if err := createBackup(statusFile); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
		fmt.Println("✓ Created backup: WAYFINDER-STATUS.md.v1.backup")
	}

	// Write V2 status
	if !migrateVerbose {
		fmt.Println("\nWriting V2 status file...")
	}
	if err := status.WriteV2ToDir(v2Status, absPath); err != nil {
		return fmt.Errorf("failed to write V2 status: %w", err)
	}

	if !migrateVerbose {
		fmt.Println("✓ Migration complete!")
		fmt.Printf("\nNext steps:\n")
		fmt.Printf("  1. Review the migrated WAYFINDER-STATUS.md\n")
		fmt.Printf("  2. Verify all data was preserved correctly\n")
		fmt.Printf("  3. Remove backup file if satisfied: rm WAYFINDER-STATUS.md.v1.backup\n")
	}

	return nil
}

// validateMigrateOptions validates migration options
func validateMigrateOptions(opts *migrate.ConvertOptions) error {
	// Validate project type if provided
	if opts.ProjectType != "" {
		valid := false
		for _, pt := range status.ValidProjectTypes() {
			if opts.ProjectType == pt {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid project type %q, must be one of: %v",
				opts.ProjectType, status.ValidProjectTypes())
		}
	}

	// Validate risk level if provided
	if opts.RiskLevel != "" {
		valid := false
		for _, rl := range status.ValidRiskLevels() {
			if opts.RiskLevel == rl {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid risk level %q, must be one of: %v",
				opts.RiskLevel, status.ValidRiskLevels())
		}
	}

	return nil
}

// printMigrationSummary displays a summary of the migration
func printMigrationSummary(v1 *status.Status, v2 *status.StatusV2) {
	fmt.Println("\nMigration Summary:")
	fmt.Println("═══════════════════════════════════════")
	fmt.Printf("Project Name:     %s\n", v2.ProjectName)
	fmt.Printf("Project Type:     %s\n", v2.ProjectType)
	fmt.Printf("Risk Level:       %s\n", v2.RiskLevel)
	fmt.Printf("Schema:           %s → %s\n", v1.SchemaVersion, v2.SchemaVersion)
	fmt.Printf("Current Phase:    %s → %s\n", v1.CurrentPhase, v2.CurrentWaypoint)
	fmt.Printf("Status:           %s → %s\n", v1.Status, v2.Status)
	fmt.Println()
	fmt.Printf("V1 Phases:        %d\n", len(v1.Phases))
	fmt.Printf("V2 Waypoint History: %d\n", len(v2.WaypointHistory))
	fmt.Println()

	// Show phase mapping
	fmt.Println("Phase Mapping:")
	for _, v1Phase := range v1.Phases {
		v2Phase := migrate.V1ToV2PhaseMap[v1Phase.Name]
		merged := ""
		if v1Phase.Name != v2Phase {
			merged = " (merged)"
		}
		fmt.Printf("  %s → %s%s\n", v1Phase.Name, v2Phase, merged)
	}
}

// createBackup creates a backup of the V1 status file
func createBackup(statusFile string) error {
	backupFile := statusFile + ".v1.backup"

	// Read original file
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return fmt.Errorf("failed to read status file: %w", err)
	}

	// Write backup
	if err := os.WriteFile(backupFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write backup: %w", err)
	}

	return nil
}
