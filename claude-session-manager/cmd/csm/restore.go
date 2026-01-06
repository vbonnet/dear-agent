package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var restoreCmd = &cobra.Command{
	Use:   "restore <session-name>",
	Short: "Restore an archived Claude session (DEPRECATED: use 'unarchive')",
	Long: `DEPRECATED: This command is deprecated. Use 'csm unarchive' instead.

Restore an archived Claude session by changing its lifecycle back to active.

Restored sessions:
  • Visible in 'csm list' (no longer need --all flag)
  • Can be resumed with tmux attach
  • Moved from .archive-old-format/ back to active sessions directory

This command will:
  1. Find the archived session by name, tmux name, or session ID
  2. Validate session is archived (lifecycle: "archived")
  3. Update manifest Lifecycle field to "" (active)
  4. Move directory back to active sessions directory
  5. Session becomes visible in csm list

Examples:
  # Restore by session name
  csm restore my-old-session

  # Restore by session ID
  csm restore session-abc123

  # List archived sessions first
  csm list --all`,
	Args: cobra.ExactArgs(1),
	RunE: restoreSession,
	ValidArgsFunction: restoreCompletion,
}

func restoreSession(cmd *cobra.Command, args []string) error {
	// Show deprecation warning
	ui.PrintWarning("DEPRECATION: 'csm restore' is deprecated. Use 'csm unarchive' instead.")
	fmt.Printf("\nThe 'csm restore' command will be removed in a future version.\n")
	fmt.Printf("Please use: csm unarchive %s\n\n", args[0])

	sessionName := args[0]
	archiveDir := filepath.Join(cfg.SessionsDir, ".archive-old-format")

	// Try resolving in ARCHIVE directory first (most likely location for archived sessions)
	// This prevents conflicts with leftover non-archived sessions in main directory
	m, manifestPath, err := session.ResolveIdentifier(sessionName, archiveDir)
	if err != nil {
		// If not found in archive directory, try main directory (for in-place archived sessions)
		m, manifestPath, err = session.ResolveIdentifier(sessionName, cfg.SessionsDir)
		if err != nil {
			ui.PrintError(err, "Archived session not found",
				fmt.Sprintf("  • Check archived sessions with: csm list --all\n"+
					"  • Tried: %s and %s", archiveDir, cfg.SessionsDir))
			return err
		}
	}

	// Validate session is archived
	if m.Lifecycle != manifest.LifecycleArchived {
		msg := fmt.Sprintf("Session '%s' is not archived", sessionName)
		ui.PrintWarning(msg)
		fmt.Printf("\nSession is already active.\n")
		fmt.Printf("Use 'csm list' to see active sessions.\n")
		return nil
	}

	// Update lifecycle to active
	m.Lifecycle = ""

	// Write manifest (automatic backup + UpdatedAt)
	if err := manifest.Write(manifestPath, m); err != nil {
		ui.PrintError(err, "Failed to write manifest",
			"  • Check file permissions\n"+
				"  • Verify disk space")
		return err
	}

	// Check if session is in archive directory (old format) and needs to be moved
	sessionDir := filepath.Dir(manifestPath)
	inArchiveDir := filepath.Dir(sessionDir) == archiveDir

	if inArchiveDir {
		// Move directory back to active sessions
		activeDir := filepath.Join(cfg.SessionsDir, filepath.Base(sessionDir))

		// Check for conflict and auto-rename if needed
		originalTargetName := filepath.Base(activeDir)
		if _, err := os.Stat(activeDir); err == nil {
			// Conflict detected: target already exists
			timestamp := time.Now().Format("20060102T150405Z")
			activeDir = activeDir + "-" + timestamp

			ui.PrintWarning(fmt.Sprintf("Active session '%s' already exists", originalTargetName))
			fmt.Printf("Renaming restored session to: %s\n", filepath.Base(activeDir))
		}

		if err := os.Rename(sessionDir, activeDir); err != nil {
			ui.PrintError(err, "Failed to move session to active directory",
				fmt.Sprintf("  • From: %s\n"+
					"  • To: %s\n"+
					"  • Check permissions", sessionDir, activeDir))
			return err
		}

		// Report success with move
		ui.PrintSuccess(fmt.Sprintf("Restored session: %s", m.Name))
		fmt.Printf("\nSession moved to: %s\n", activeDir)
		fmt.Printf("\nThe session is now visible in 'csm list'.\n")
	} else {
		// In-place restore (session was in main directory, just had lifecycle=archived)
		ui.PrintSuccess(fmt.Sprintf("Restored session: %s", m.Name))
		fmt.Printf("\nLifecycle updated to active in: %s\n", sessionDir)
		fmt.Printf("\nThe session is now visible in 'csm list' as active/stopped.\n")
	}

	return nil
}

func restoreCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Only complete first argument
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	archiveDir := filepath.Join(cfg.SessionsDir, ".archive-old-format")
	var allManifests []*manifest.Manifest

	// List archived manifests from main directory
	if manifests, err := manifest.List(cfg.SessionsDir); err == nil {
		for _, m := range manifests {
			if m.Lifecycle == manifest.LifecycleArchived {
				allManifests = append(allManifests, m)
			}
		}
	}

	// List archived manifests from old archive directory
	if manifests, err := manifest.List(archiveDir); err == nil {
		for _, m := range manifests {
			if m.Lifecycle == manifest.LifecycleArchived {
				allManifests = append(allManifests, m)
			}
		}
	}

	// Build suggestions
	var suggestions []string
	for _, m := range allManifests {
		// Add manifest name
		if m.Name != "" {
			suggestions = append(suggestions, m.Name)
		}
		// Add session ID
		suggestions = append(suggestions, m.SessionID)
	}

	return suggestions, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	rootCmd.AddCommand(restoreCmd)
}
