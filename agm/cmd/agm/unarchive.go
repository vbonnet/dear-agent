package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var unarchiveCmd = &cobra.Command{
	Use:   "unarchive <pattern>",
	Short: "Restore archived sessions using glob patterns",
	Long: `Restore archived Claude sessions by pattern.

Supports glob patterns: *, ?, [abc]

The command will:
  1. Find archived sessions matching the pattern
  2. If multiple matches, show interactive selection menu
  3. Prompt for confirmation
  4. Restore the selected session (set lifecycle to active)
  5. Move from .archive-old-format/ if needed

Examples:
  # Exact match
  agm session unarchive my-session

  # Any session with "acme" in the name
  agm session unarchive *acme*

  # Wildcard year pattern
  agm session unarchive session-202?-*

  # All archived sessions (interactive selection)
  agm session unarchive "*"`,
	Args:              cobra.ExactArgs(1),
	RunE:              runUnarchive,
	ValidArgsFunction: unarchiveCompletion,
}

func runUnarchive(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	// Validate glob pattern
	if _, err := filepath.Match(pattern, ""); err != nil {
		return fmt.Errorf("invalid glob pattern: %w\nExamples:\n  *session*    Match any session with 'session'\n  my-?-work    ? matches one character\n  [abc]*       Match starting with a, b, or c", err)
	}

	// Get Dolt storage adapter
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	// Find matching archived sessions
	matches, err := session.FindArchived(cfg.SessionsDir, pattern, adapter)
	if err != nil {
		ui.PrintError(err, "Failed to search for archived sessions",
			"  • Check sessions directory exists\n"+
				"  • Verify permissions")
		return err
	}

	// Handle results based on count
	switch len(matches) {
	case 0:
		fmt.Printf("No archived sessions match pattern: %s\n", pattern)
		fmt.Printf("\nSuggestions:\n")
		fmt.Printf("  • List all archived sessions: agm session list --all\n")
		fmt.Printf("  • Try a broader pattern: agm session unarchive '*'\n")
		fmt.Printf("  • Use search for semantic matching: agm session search \"<query>\"\n")
		return nil

	case 1:
		// Auto-restore single match
		matched := matches[0]
		fmt.Printf("Found 1 match: %s\n", matched.Name)
		return restoreArchivedSession(adapter, matched)

	default:
		// Multiple matches - show interactive selection
		fmt.Printf("Found %d matches\n", len(matches))

		// Convert to ui.ArchivedSessionInfo
		var sessionInfos []ui.ArchivedSessionInfo
		for _, s := range matches {
			sessionInfos = append(sessionInfos, ui.ArchivedSessionInfo{
				SessionID:  s.SessionID,
				Name:       s.Name,
				ArchivedAt: s.ArchivedAt,
				Tags:       s.Tags,
				Project:    s.Project,
			})
		}

		// Show picker
		selectedID, err := ui.ArchivedSessionPicker(sessionInfos)
		if err != nil {
			// User cancelled
			fmt.Printf("\nRestore cancelled.\n")
			return nil
		}

		// Find the selected session
		var selected *session.ArchivedSession
		for _, s := range matches {
			if s.SessionID == selectedID {
				selected = s
				break
			}
		}

		if selected == nil {
			return fmt.Errorf("selected session not found (internal error)")
		}

		return restoreArchivedSession(adapter, selected)
	}
}

// restoreArchivedSession restores a single archived session
func restoreArchivedSession(adapter *dolt.Adapter, archived *session.ArchivedSession) error {
	// Read manifest from Dolt
	m, err := adapter.GetSession(archived.SessionID)
	if err != nil {
		ui.PrintError(err, "Failed to read session from Dolt",
			fmt.Sprintf("  • SessionID: %s\n"+
				"  • Check database connection", archived.SessionID))
		return err
	}

	// Validate it's still archived (double-check)
	if m.Lifecycle != manifest.LifecycleArchived {
		ui.PrintWarning(fmt.Sprintf("Session '%s' is not archived", archived.Name))
		fmt.Printf("\nSession is already active.\n")
		return nil
	}

	// Show session info
	fmt.Printf("Restore session: %s\n", ui.Bold(m.Name))
	fmt.Printf("  Location: %s\n", archived.ManifestPath)
	if m.Context.Project != "" {
		fmt.Printf("  Project: %s\n", m.Context.Project)
	}
	fmt.Printf("  Archived: %s\n", archived.ArchivedAt)
	fmt.Println("\nThis will restore the session and make it visible in 'agm session list'.")
	fmt.Println()

	if !unarchiveForce {
		var confirmed bool
		err = huh.NewConfirm().
			Title("Restore this session?").
			Affirmative("Yes").
			Negative("No").
			Value(&confirmed).
			WithTheme(ui.GetTheme()).
			Run()
		if err != nil {
			ui.PrintError(err,
				"Failed to read confirmation prompt",
				"  • Check terminal is interactive (TTY)\n"+
					"  • Try running with --force to skip confirmation")
			return err
		}

		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Update lifecycle to active
	m.Lifecycle = ""

	// Write manifest to Dolt (automatic UpdatedAt)
	if err := adapter.UpdateSession(m); err != nil {
		ui.PrintError(err, "Failed to update session in Dolt",
			"  • Check database connection\n"+
				"  • Verify Dolt server is running")
		return err
	}

	// Auto-commit manifest change if in git repo
	_ = git.CommitManifest(archived.ManifestPath, "unarchive", archived.Name) // Errors logged internally

	// Check if session is in archive directory and needs to be moved
	sessionDir := filepath.Dir(archived.ManifestPath)
	archiveDir := filepath.Join(cfg.SessionsDir, ".archive-old-format")
	inArchiveDir := filepath.Dir(sessionDir) == archiveDir

	if inArchiveDir {
		// Move directory back to active sessions
		activeDir := filepath.Join(cfg.SessionsDir, filepath.Base(sessionDir))

		// Check for conflict and auto-rename if needed
		originalTargetName := filepath.Base(activeDir)
		if _, err := os.Stat(activeDir); err == nil {
			// Conflict detected
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

		ui.PrintSuccess(fmt.Sprintf("Restored session: %s", m.Name))
		fmt.Printf("\nSession moved to: %s\n", activeDir)
		fmt.Printf("\nThe session is now visible in 'agm session list'.\n")
	} else {
		// In-place restore
		ui.PrintSuccess(fmt.Sprintf("Restored session: %s", m.Name))
		fmt.Printf("\nLifecycle updated to active in: %s\n", sessionDir)
		fmt.Printf("\nThe session is now visible in 'agm session list' as active/stopped.\n")
	}

	return nil
}

func unarchiveCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Only complete first argument
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Get Dolt adapter
	adapter, err := getStorage()
	if err != nil {
		// Fail gracefully - return empty list if can't connect to Dolt
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}
	defer adapter.Close()

	// List archived sessions from Dolt
	filter := &dolt.SessionFilter{
		Lifecycle: manifest.LifecycleArchived,
	}
	archivedManifests, err := adapter.ListSessions(filter)
	if err != nil {
		// Fail gracefully - return empty list if query fails
		return []string{}, cobra.ShellCompDirectiveNoFileComp
	}

	// Build suggestions
	var suggestions []string
	for _, m := range archivedManifests {
		if m.Name != "" {
			suggestions = append(suggestions, m.Name)
		}
		suggestions = append(suggestions, m.SessionID)
	}

	return suggestions, cobra.ShellCompDirectiveNoFileComp
}

var unarchiveForce bool

func init() {
	unarchiveCmd.Flags().BoolVar(&unarchiveForce, "force", false, "Skip confirmation prompt (for non-interactive use)")
	sessionCmd.AddCommand(unarchiveCmd)
}
