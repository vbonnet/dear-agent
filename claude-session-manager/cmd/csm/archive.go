package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/discovery"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	forceArchive bool
)

var archiveCmd = &cobra.Command{
	Use:   "archive <session-name>",
	Short: "Archive a Claude session",
	Long: `Archive a Claude session by marking it as archived.

Archived sessions:
  • Hidden from 'csm list' (use --all flag to see them)
  • Files are NOT deleted (only metadata updated)
  • Cannot be resumed until restored
  • Automatic backup created before archiving

This command will:
  1. Find the session by name, tmux name, or session ID
  2. Check if session is currently active in tmux
  3. Prompt for confirmation (unless --force is used)
  4. Update the manifest Lifecycle field to "archived"
  5. Create automatic backup of the manifest

To restore an archived session:
  1. Run: csm list --all
  2. Find session ID
  3. Edit: ~/sessions/session-<ID>/manifest.yaml
  4. Change: lifecycle: "archived" to lifecycle: ""
  5. Save and session will appear in csm list

Examples:
  # Archive with confirmation prompt
  csm archive my-old-session

  # Archive without confirmation (automation/scripts)
  csm archive my-old-session --force

  # List all sessions including archived
  csm list --all

  # Archive by tmux session name
  csm archive claude-5

  # Archive by session ID
  csm archive session-abc123`,
	Args: cobra.ExactArgs(1),
	RunE: archiveSession,
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Only complete first argument
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}

		// List all manifests
		manifests, err := manifest.List(cfg.SessionsDir)
		if err != nil {
			return []string{}, cobra.ShellCompDirectiveNoFileComp
		}

		// Filter out archived sessions (can't archive what's already archived)
		filtered := make([]*manifest.Manifest, 0, len(manifests))
		for _, m := range manifests {
			if m.Lifecycle != manifest.LifecycleArchived {
				filtered = append(filtered, m)
			}
		}
		manifests = filtered

		// Get tmux mapping
		tmuxMapping, _ := discovery.GetTmuxMapping(cfg.SessionsDir)

		// Build suggestions from non-archived sessions
		var suggestions []string
		for _, m := range manifests {
			// Add tmux name
			if tmuxName := tmuxMapping[m.SessionID]; tmuxName != "" {
				suggestions = append(suggestions, tmuxName)
			}

			// Add manifest name (if different)
			if m.Name != "" && m.Name != tmuxMapping[m.SessionID] {
				suggestions = append(suggestions, m.Name)
			}
		}

		return suggestions, cobra.ShellCompDirectiveNoFileComp
	},
}

func archiveSession(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	sessionsDir := cfg.SessionsDir

	// Resolve session identifier to manifest
	m, manifestPath, err := session.ResolveIdentifier(sessionName, sessionsDir)
	if err != nil {
		ui.PrintError(err, "Session not found",
			fmt.Sprintf("  • Check session name with: csm list\n"+
				"  • Available sessions are in: %s", sessionsDir))
		return err
	}

	// Check if already archived
	if m.Lifecycle == manifest.LifecycleArchived {
		msg := fmt.Sprintf("Session '%s' is already archived", sessionName)
		ui.PrintWarning(msg)
		fmt.Printf("\nManifest: %s\n", manifestPath)
		fmt.Println("\nTo restore this session:")
		fmt.Println("  1. Edit the manifest file above")
		fmt.Println("  2. Change lifecycle: \"archived\" to lifecycle: \"\"")
		fmt.Println("  3. Session will appear in csm list")
		return nil
	}

	// Check if session is active (unless --force)
	if !forceArchive {
		tmux := session.NewRealTmux()
		isActive, err := tmux.HasSession(m.Tmux.SessionName)
		if err != nil {
			// Ignore error - if we can't check, assume not active
			isActive = false
		}
		if isActive {
			ui.PrintError(
				fmt.Errorf("session is active"),
				fmt.Sprintf("Cannot archive active session '%s'", sessionName),
				fmt.Sprintf("The session is currently running in tmux.\n\n"+
					"To archive this session:\n"+
					"  1. Stop the tmux session first:\n"+
					"     tmux kill-session -t %s\n\n"+
					"  2. Then archive:\n"+
					"     csm archive %s\n\n"+
					"Or use --force to archive anyway:\n"+
					"  csm archive %s --force",
					m.Tmux.SessionName, sessionName, sessionName))
			return fmt.Errorf("cannot archive active session")
		}
	}

	// Show confirmation prompt (unless --force)
	if !forceArchive {
		fmt.Printf("Archive session: %s\n", ui.Bold(m.Name))
		fmt.Printf("  Location: %s\n", manifestPath)
		if m.Context.Project != "" {
			fmt.Printf("  Project: %s\n", m.Context.Project)
		}

		// Show status
		tmux := session.NewRealTmux()
		status := "stopped"
		isActive, err := tmux.HasSession(m.Tmux.SessionName)
		if err == nil && isActive {
			status = "active"
		}
		fmt.Printf("  Status: %s\n", status)

		fmt.Println("\nThis will mark the session as archived.")
		fmt.Println("Files will NOT be deleted.")
		fmt.Println()

		confirmed, err := ui.Confirm("Archive this session?")
		if err != nil {
			ui.PrintError(err, "Failed to read confirmation", "")
			return err
		}

		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Update lifecycle field
	m.Lifecycle = manifest.LifecycleArchived

	// Write manifest (automatic backup + UpdatedAt)
	if err := manifest.Write(manifestPath, m); err != nil {
		ui.PrintError(err, "Failed to write manifest",
			"  • Check file permissions\n"+
				"  • Verify disk space")
		return err
	}

	// Move session directory to .archive-old-format/
	sessionDir := filepath.Dir(manifestPath)
	archiveBaseDir := filepath.Join(sessionsDir, ".archive-old-format")
	archiveTargetDir := filepath.Join(archiveBaseDir, filepath.Base(sessionDir))

	// Create archive directory if it doesn't exist
	if err := os.MkdirAll(archiveBaseDir, 0700); err != nil {
		ui.PrintError(err, "Failed to create archive directory",
			fmt.Sprintf("  • Directory: %s\n"+
				"  • Check permissions and disk space", archiveBaseDir))
		return err
	}

	// Check for conflict and auto-rename if needed
	originalTargetName := filepath.Base(archiveTargetDir)
	if _, err := os.Stat(archiveTargetDir); err == nil {
		// Conflict detected: target already exists
		timestamp := time.Now().Format("20060102T150405Z")
		archiveTargetDir = archiveTargetDir + "-" + timestamp

		ui.PrintWarning(fmt.Sprintf("Archive '%s' already exists", originalTargetName))
		fmt.Printf("Renaming to: %s\n", filepath.Base(archiveTargetDir))
	}

	// Move session directory to archive
	if err := os.Rename(sessionDir, archiveTargetDir); err != nil {
		ui.PrintError(err, "Failed to move session to archive",
			fmt.Sprintf("  • From: %s\n"+
				"  • To: %s\n"+
				"  • Check permissions and ensure target doesn't exist", sessionDir, archiveTargetDir))
		return err
	}

	// Report success
	ui.PrintSuccess(fmt.Sprintf("Archived session: %s", sessionName))
	fmt.Printf("\nSession moved to: %s\n", archiveTargetDir)
	fmt.Printf("\nThe session is now hidden from 'csm list'.\n")
	fmt.Printf("Use 'csm list --all' to see archived sessions.\n")

	return nil
}

func init() {
	archiveCmd.Flags().BoolVarP(&forceArchive, "force", "f", false,
		"Skip confirmation prompt and active session check")
	rootCmd.AddCommand(archiveCmd)
}
