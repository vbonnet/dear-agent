package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Batch cleanup sessions (archive/delete)",
	Long: `Interactive cleanup tool for batch operations on sessions.

Shows multi-select lists of:
  • Stopped sessions >30 days (suggested for archival)
  • Archived sessions >90 days (suggested for deletion)

Thresholds can be customized in ~/.config.agm/config.yaml

Examples:
  agm admin clean                 # Interactive cleanup with smart suggestions
  agm admin clean --dry-run       # Preview what would be cleaned`,
	RunE: func(cmd *cobra.Command, args []string) error {
		uiCfg := ui.LoadConfig()

		// Get Dolt storage adapter
		adapter, err := getStorage()
		if err != nil {
			return fmt.Errorf("failed to connect to Dolt storage: %w", err)
		}
		defer adapter.Close()

		// List all sessions from Dolt
		manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
		if err != nil {
			return fmt.Errorf("failed to list sessions: %w", err)
		}

		// Convert to UI sessions with status
		uiSessions := make([]*ui.Session, len(manifests))
		statuses := session.ComputeStatusBatch(manifests, tmuxClient)

		for i, m := range manifests {
			uiSessions[i] = &ui.Session{
				Manifest:  m,
				Status:    statuses[m.Name],
				UpdatedAt: m.UpdatedAt,
			}
		}

		// Show multi-select cleanup UI
		result, err := ui.CleanupMultiSelect(uiSessions, uiCfg)
		if err != nil {
			return fmt.Errorf("cleanup cancelled: %w", err)
		}

		if len(result.ToArchive) == 0 && len(result.ToDelete) == 0 {
			fmt.Println("No sessions selected for cleanup.")
			return nil
		}

		// Build confirmation message
		var toArchiveNames, toDeleteNames []string
		for _, s := range result.ToArchive {
			toArchiveNames = append(toArchiveNames, s.Name)
		}
		for _, s := range result.ToDelete {
			toDeleteNames = append(toDeleteNames, s.Name)
		}

		// Confirm cleanup
		confirmed, err := ui.ConfirmCleanup(toArchiveNames, toDeleteNames, uiCfg)
		if err != nil {
			return err
		}

		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}

		// Perform cleanup operations
		archived := 0
		deleted := 0

		// Archive stopped sessions
		for _, s := range result.ToArchive {
			if err := archiveSessionManifest(adapter, s.Manifest); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to archive %s: %v", s.Name, err))
			} else {
				archived++
				fmt.Printf("📦 Archived: %s\n", s.Name)
			}
		}

		// Delete archived sessions
		for _, s := range result.ToDelete {
			if err := deleteSessionManifest(s.Manifest); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to delete %s: %v", s.Name, err))
			} else {
				deleted++
				fmt.Printf("🗑️  Deleted: %s\n", s.Name)
			}
		}

		// Summary
		fmt.Println()
		ui.PrintSuccess(fmt.Sprintf("Cleanup complete: %d archived, %d deleted", archived, deleted))
		return nil
	},
}

func archiveSessionManifest(adapter *dolt.Adapter, m *manifest.Manifest) error {
	m.Lifecycle = manifest.LifecycleArchived
	if err := adapter.UpdateSession(m); err != nil {
		return err
	}

	// Auto-commit manifest change if in git repo
	manifestPath := getManifestPath(m.SessionID)
	_ = git.CommitManifest(manifestPath, "archive", m.Name) // Errors logged internally

	return nil
}

func deleteSessionManifest(m *manifest.Manifest) error {
	manifestDir := getSessionDir(m.SessionID)
	return os.RemoveAll(manifestDir)
}

func getManifestPath(sessionID string) string {
	return filepath.Join(cfg.SessionsDir, sessionID, "manifest.yaml")
}

func getSessionDir(sessionID string) string {
	return filepath.Join(cfg.SessionsDir, sessionID)
}

func init() {
	adminCmd.AddCommand(cleanCmd)
}
