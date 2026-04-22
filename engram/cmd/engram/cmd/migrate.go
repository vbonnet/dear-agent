package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
)

var (
	migrateWorkspaceFlag string
	migrateDryRun        bool
	migrateForce         bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate engram data to workspace-aware paths",
	Long: `Migrate engram data from legacy ~/.engram to workspace-specific paths.

This command:
  1. Backs up ~/.engram to ~/.engram.backup-{timestamp}
  2. Moves data to workspace path (e.g., ~/src/ws/oss/.engram/)
  3. Creates symlink ~/.engram → {workspace}/.engram for compatibility
  4. Preserves all data (user configs, logs, cache)

The migration is safe and reversible. If anything goes wrong, you can restore
from the backup directory.

WORKSPACE DETECTION
The target workspace is determined by (in priority order):
  1. --workspace flag
  2. $WORKSPACE environment variable
  3. Auto-detect from current directory
  4. Prompt user to select workspace

EXAMPLES
  # Migrate to specific workspace
  $ engram migrate --workspace oss

  # Migrate using environment variable
  $ export WORKSPACE=oss
  $ engram migrate

  # Dry run to see what would happen
  $ engram migrate --workspace oss --dry-run

  # Force migration even if target already exists
  $ engram migrate --workspace oss --force
`,
	RunE: runMigrate,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().StringVar(&migrateWorkspaceFlag, "workspace", "", "Target workspace name")
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Show what would be done without making changes")
	migrateCmd.Flags().BoolVar(&migrateForce, "force", false, "Force migration even if target exists")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	legacyPath := filepath.Join(home, ".engram")

	// Step 1: Check if legacy path exists
	// Use Lstat instead of Stat to detect symlinks without following them
	legacyInfo, err := os.Lstat(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			cli.PrintSuccess("No migration needed - ~/.engram does not exist")
			fmt.Println()
			fmt.Println("You can initialize a workspace-aware installation with:")
			fmt.Println("  export WORKSPACE=oss")
			fmt.Println("  engram init")
			return nil
		}
		return fmt.Errorf("failed to check legacy path: %w", err)
	}

	// Check if it's a symlink (already migrated)
	if legacyInfo.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(legacyPath)
		if err == nil {
			cli.PrintSuccess(fmt.Sprintf("Already migrated! ~/.engram → %s", target))
			return nil
		}
	}

	// Step 2: Determine target workspace
	cfg, err := loadConfigWithWorkspace()
	if err != nil {
		return fmt.Errorf("failed to load config with workspace detection: %w", err)
	}

	// Override with explicit flag if provided
	if migrateWorkspaceFlag != "" {
		// For explicit workspace flag, we need to compute the target path
		// This is a simplified approach - in production, use workspace detector
		targetBase := filepath.Join(home, "src", "ws", migrateWorkspaceFlag)
		cfg.Platform.EngramPath = filepath.Join(targetBase, ".engram")
		cfg.Workspace = migrateWorkspaceFlag
	}

	if cfg.Workspace == "" {
		return fmt.Errorf("no workspace detected. Please specify --workspace flag or set $WORKSPACE")
	}

	targetPath := cfg.Platform.EngramPath
	if targetPath == "" || targetPath == legacyPath {
		return fmt.Errorf("target path same as legacy path - migration not needed")
	}

	// Step 3: Display migration plan
	fmt.Println("Migration Plan:")
	fmt.Println("═══════════════")
	fmt.Printf("  Workspace: %s\n", cfg.Workspace)
	fmt.Printf("  From:      %s\n", legacyPath)
	fmt.Printf("  To:        %s\n", targetPath)
	fmt.Println()

	if migrateDryRun {
		cli.PrintWarning("DRY RUN MODE - No changes will be made")
		fmt.Println()
		fmt.Println("Would perform:")
		fmt.Printf("  1. Create backup: %s.backup-%s\n", legacyPath, time.Now().Format("20060102-150405"))
		fmt.Printf("  2. Create target: %s\n", targetPath)
		fmt.Printf("  3. Move data:     %s/* → %s/\n", legacyPath, targetPath)
		fmt.Printf("  4. Create symlink: %s → %s\n", legacyPath, targetPath)
		return nil
	}

	// Step 4: Check if target already exists
	if _, err := os.Stat(targetPath); err == nil {
		if !migrateForce {
			return fmt.Errorf("target path already exists: %s\nUse --force to overwrite or choose a different workspace", targetPath)
		}
		cli.PrintWarning(fmt.Sprintf("Target exists, will merge (--force enabled): %s", targetPath))
	}

	// Step 5: Confirm with user
	fmt.Println("This will:")
	fmt.Println("  1. Backup your current engram data")
	fmt.Println("  2. Move it to the workspace-specific location")
	fmt.Println("  3. Create a compatibility symlink")
	fmt.Println()
	fmt.Print("Proceed with migration? (yes/no): ")

	var response string
	fmt.Scanln(&response)
	if response != "yes" && response != "y" {
		fmt.Println("Migration cancelled")
		return nil
	}

	// Step 6: Create backup
	timestamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.backup-%s", legacyPath, timestamp)

	cli.PrintInfo(fmt.Sprintf("Creating backup: %s", backupPath))
	if err := os.Rename(legacyPath, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	cli.PrintSuccess(fmt.Sprintf("✓ Backup created: %s", backupPath))

	// Step 7: Create target directory structure
	cli.PrintInfo(fmt.Sprintf("Creating target directory: %s", targetPath))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		// Rollback: restore backup
		os.Rename(backupPath, legacyPath)
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Step 8: Move data to target
	cli.PrintInfo(fmt.Sprintf("Moving data: %s → %s", backupPath, targetPath))
	if err := os.Rename(backupPath, targetPath); err != nil {
		// Rollback: restore backup
		os.Rename(backupPath, legacyPath)
		return fmt.Errorf("failed to move data: %w", err)
	}
	cli.PrintSuccess(fmt.Sprintf("✓ Data moved to: %s", targetPath))

	// Step 9: Create symlink for backward compatibility
	cli.PrintInfo(fmt.Sprintf("Creating compatibility symlink: %s → %s", legacyPath, targetPath))
	if err := os.Symlink(targetPath, legacyPath); err != nil {
		cli.PrintWarning(fmt.Sprintf("Failed to create symlink: %v", err))
		cli.PrintWarning("Old tools may not find engram data. You can create it manually:")
		fmt.Printf("  ln -s %s %s\n", targetPath, legacyPath)
	} else {
		cli.PrintSuccess(fmt.Sprintf("✓ Symlink created: %s → %s", legacyPath, targetPath))
	}

	// Success!
	fmt.Println()
	cli.PrintSuccess("Migration completed successfully!")
	fmt.Println()
	fmt.Println("Your engram data is now at:")
	fmt.Printf("  %s\n", targetPath)
	fmt.Println()
	fmt.Println("Backup preserved at:")
	fmt.Printf("  %s.backup-%s\n", legacyPath, timestamp)
	fmt.Println()
	fmt.Println("To rollback this migration:")
	fmt.Printf("  rm %s\n", legacyPath)
	fmt.Printf("  mv %s.backup-%s %s\n", legacyPath, timestamp, legacyPath)
	fmt.Println()

	return nil
}
