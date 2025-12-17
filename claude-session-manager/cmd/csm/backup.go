package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/backup"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage manifest backups",
	Long: `Manage manifest backups with list and restore operations.

Examples:
  csm backup list <identifier>       # List backups for a session
  csm backup restore <identifier> <num>  # Restore backup number`,
}

var backupListCmd = &cobra.Command{
	Use:   "list <identifier>",
	Short: "List backups for a session manifest",
	Long: `List all available backups for a session manifest.

The identifier can be:
- Session UUID (full or partial)
- Tmux session name
- Project path pattern

Examples:
  csm backup list c4eb298c              # By UUID prefix
  csm backup list claude-1              # By tmux name
  csm backup list workspace-design      # By project path`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]

		// Resolve identifier to manifest path
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		sessionsDir := filepath.Join(homeDir, "sessions")

		m, manifestPath, err := session.ResolveIdentifier(identifier, sessionsDir)
		if err != nil {
			ui.PrintError(err, "Failed to resolve session identifier",
				"  • Try: csm list --all to see available sessions")
			return err
		}

		// List backups
		backups, err := backup.ListBackups(manifestPath)
		if err != nil {
			ui.PrintError(err, "Failed to list backups", "")
			return err
		}

		if len(backups) == 0 {
			ui.PrintWarning(fmt.Sprintf("No backups found for session %s", m.SessionID[:8]))
			fmt.Println("\nBackups are created automatically when you modify a manifest.")
			return nil
		}

		// Display backups
		fmt.Printf("Backups for session %s:\n\n", m.SessionID[:8])
		fmt.Printf("%-8s %s\n", "NUMBER", "PATH")
		fmt.Println("────────────────────────────────────────────────")

		for _, num := range backups {
			backupPath := fmt.Sprintf("%s.%d", manifestPath, num)
			fmt.Printf("%-8d %s\n", num, backupPath)
		}

		fmt.Printf("\nTotal: %d backup(s)\n", len(backups))
		fmt.Printf("\nRestore with: csm backup restore %s <number>\n", m.SessionID[:8])

		return nil
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore <identifier> <backup-number>",
	Short: "Restore a manifest from a backup",
	Long: `Restore a session manifest from a specific backup.

The current manifest will be backed up before restoration.

The identifier can be:
- Session UUID (full or partial)
- Tmux session name
- Project path pattern

Examples:
  csm backup restore c4eb298c 3         # Restore backup #3 by UUID
  csm backup restore claude-1 2         # Restore backup #2 by tmux name`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		identifier := args[0]
		backupNum := 0
		if _, err := fmt.Sscanf(args[1], "%d", &backupNum); err != nil {
			return fmt.Errorf("invalid backup number %q: must be an integer", args[1])
		}

		// Resolve identifier to manifest path
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		sessionsDir := filepath.Join(homeDir, "sessions")

		m, manifestPath, err := session.ResolveIdentifier(identifier, sessionsDir)
		if err != nil {
			ui.PrintError(err, "Failed to resolve session identifier",
				"  • Try: csm list --all to see available sessions")
			return err
		}

		// Confirm restoration
		fmt.Printf("Restore session %s from backup #%d?\n", m.SessionID[:8], backupNum)
		fmt.Println("\nWarning: The current manifest will be backed up before restoration.")

		confirm, err := ui.Confirm("Continue?")
		if err != nil || !confirm {
			fmt.Println("Restoration cancelled.")
			return nil
		}

		// Restore backup
		if err := backup.RestoreBackup(manifestPath, backupNum); err != nil {
			ui.PrintError(err, "Failed to restore backup", "")
			return err
		}

		ui.PrintSuccess(fmt.Sprintf("Restored session %s from backup #%d", m.SessionID[:8], backupNum))

		// Read and display restored manifest
		restoredManifest, err := manifest.Read(manifestPath)
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Restored but failed to read manifest: %v", err))
			return nil
		}

		fmt.Println("\nRestored manifest:")
		fmt.Printf("  Name:    %s\n", restoredManifest.Name)
		fmt.Printf("  Project: %s\n", restoredManifest.Context.Project)
		fmt.Printf("  Tmux:    %s\n", restoredManifest.Tmux.SessionName)

		return nil
	},
}

func init() {
	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	rootCmd.AddCommand(backupCmd)
}
