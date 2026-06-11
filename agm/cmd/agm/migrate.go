package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var migrateDryRun bool

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate session directories to current formats",
}

var migrateClaudeToUnifiedCmd = &cobra.Command{
	Use:   "claude-to-unified",
	Short: "Rename old-format dirs ({name}-session) to unified format (session-{name})",
	Long: `Find session directories in the old {name}-session format and rename them to
the unified session-{name} format.

Use --dry-run to preview planned renames without modifying the filesystem.

Example:
  agm migrate claude-to-unified --dry-run   # preview
  agm migrate claude-to-unified             # apply`,
	RunE: runMigrateClaudeToUnified,
}

func init() {
	rootCmd.AddCommand(migrateCmd)
	migrateCmd.AddCommand(migrateClaudeToUnifiedCmd)
	migrateClaudeToUnifiedCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false,
		"Print planned renames without making any filesystem changes")
}

// oldFormatEntry describes a session directory in the old {name}-session format.
type oldFormatEntry struct {
	OldName string // e.g. "claude-1-session"
	NewName string // e.g. "session-claude-1"
	OldPath string
	NewPath string
}

// scanOldFormatDirs returns all directories under dir whose name ends with
// "-session" (the old format), sorted alphabetically.
func scanOldFormatDirs(dir string) ([]oldFormatEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var result []oldFormatEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if !strings.HasSuffix(name, "-session") {
			continue
		}
		sessionName := strings.TrimSuffix(name, "-session")
		newName := "session-" + sessionName
		result = append(result, oldFormatEntry{
			OldName: name,
			NewName: newName,
			OldPath: filepath.Join(dir, name),
			NewPath: filepath.Join(dir, newName),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].OldName < result[j].OldName
	})
	return result, nil
}

func runMigrateClaudeToUnified(cmd *cobra.Command, _ []string) error {
	sessionsDir := getDoctorSessionsDir()

	entries, err := scanOldFormatDirs(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(cmd.OutOrStdout(), "Sessions directory not found: %s\n", sessionsDir)
			return nil
		}
		return fmt.Errorf("scanning sessions directory: %w", err)
	}

	if len(entries) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No old-format session directories found in %s\n", sessionsDir)
		return nil
	}

	if migrateDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Dry run — no changes will be made.\n\n")
	}

	var migrated, skipped int
	for _, e := range entries {
		if _, err := os.Stat(e.NewPath); err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "  skip   %s  (target %s already exists)\n",
				e.OldName, e.NewName)
			skipped++
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  rename %s  ->  %s\n", e.OldName, e.NewName)
		if !migrateDryRun {
			if err := os.Rename(e.OldPath, e.NewPath); err != nil {
				return fmt.Errorf("renaming %s: %w", e.OldName, err)
			}
		}
		migrated++
	}

	fmt.Fprintln(cmd.OutOrStdout())
	if migrateDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Would rename %d, skip %d (remove --dry-run to apply)\n",
			migrated, skipped)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Renamed %d, skipped %d\n", migrated, skipped)
	}
	return nil
}
