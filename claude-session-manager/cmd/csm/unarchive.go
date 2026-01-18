package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/session"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	forceUnarchive bool
)

var unarchiveCmd = &cobra.Command{
	Use:   "unarchive <pattern>",
	Short: "Restore archived sessions using glob patterns",
	Long: `Restore archived Claude sessions by pattern.

Supports glob patterns: *, ?, [abc]

The command will:
  1. Find archived sessions matching the pattern
  2. If multiple matches, show interactive selection menu
  3. Prompt for confirmation (unless --force is used)
  4. Restore the selected session (set lifecycle to active)
  5. Move from .archive-old-format/ if needed

Examples:
  # Exact match
  csm unarchive my-session

  # Any session with "[REDACTED_EMPLOYER]" in the name
  csm unarchive *[REDACTED_EMPLOYER]*

  # Wildcard year pattern
  csm unarchive session-202?-*

  # All archived sessions (interactive selection)
  csm unarchive "*"

  # Skip confirmation prompt
  csm unarchive my-session --force`,
	Args: cobra.ExactArgs(1),
	RunE: runUnarchive,
	ValidArgsFunction: unarchiveCompletion,
}

func runUnarchive(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	// Validate glob pattern
	if _, err := filepath.Match(pattern, ""); err != nil {
		return fmt.Errorf("invalid glob pattern: %w\nExamples:\n  *session*    Match any session with 'session'\n  my-?-work    ? matches one character\n  [abc]*       Match starting with a, b, or c", err)
	}

	// Find matching archived sessions
	matches, err := session.FindArchived(cfg.SessionsDir, pattern)
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
		fmt.Printf("  • List all archived sessions: csm list --all\n")
		fmt.Printf("  • Try a broader pattern: csm unarchive '*'\n")
		fmt.Printf("  • Use search for semantic matching: csm search \"<query>\"\n")
		return nil

	case 1:
		// Auto-restore single match
		matched := matches[0]
		fmt.Printf("Found 1 match: %s\n", matched.Name)
		return restoreArchivedSession(matched)

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

		return restoreArchivedSession(selected)
	}
}

// restoreArchivedSession restores a single archived session
func restoreArchivedSession(archived *session.ArchivedSession) error {
	// Read manifest
	m, err := manifest.Read(archived.ManifestPath)
	if err != nil {
		ui.PrintError(err, "Failed to read manifest",
			fmt.Sprintf("  • Path: %s\n"+
				"  • Check file permissions", archived.ManifestPath))
		return err
	}

	// Validate it's still archived (double-check)
	if m.Lifecycle != manifest.LifecycleArchived {
		ui.PrintWarning(fmt.Sprintf("Session '%s' is not archived", archived.Name))
		fmt.Printf("\nSession is already active.\n")
		return nil
	}

	// Show confirmation prompt (unless --force)
	if !forceUnarchive {
		fmt.Printf("Restore session: %s\n", ui.Bold(m.Name))
		fmt.Printf("  Location: %s\n", archived.ManifestPath)
		if m.Context.Project != "" {
			fmt.Printf("  Project: %s\n", m.Context.Project)
		}
		fmt.Printf("  Archived: %s\n", archived.ArchivedAt)
		fmt.Println("\nThis will restore the session and make it visible in 'csm list'.")
		fmt.Println()

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
				"  • Use --force flag to skip confirmation: csm unarchive "+archived.Name+" --force\n"+
					"  • Check terminal is interactive (TTY)\n"+
					"  • Try running outside tmux/screen if inside")
			return err
		}

		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Update lifecycle to active
	m.Lifecycle = ""

	// Write manifest (automatic backup + UpdatedAt)
	if err := manifest.Write(archived.ManifestPath, m); err != nil {
		ui.PrintError(err, "Failed to write manifest",
			"  • Check file permissions\n"+
				"  • Verify disk space")
		return err
	}

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
		fmt.Printf("\nThe session is now visible in 'csm list'.\n")
	} else {
		// In-place restore
		ui.PrintSuccess(fmt.Sprintf("Restored session: %s", m.Name))
		fmt.Printf("\nLifecycle updated to active in: %s\n", sessionDir)
		fmt.Printf("\nThe session is now visible in 'csm list' as active/stopped.\n")
	}

	return nil
}

func unarchiveCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
		allManifests = append(allManifests, manifests...)
	}

	// Build suggestions
	var suggestions []string
	for _, m := range allManifests {
		if m.Name != "" {
			suggestions = append(suggestions, m.Name)
		}
		suggestions = append(suggestions, m.SessionID)
	}

	return suggestions, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	unarchiveCmd.Flags().BoolVarP(&forceUnarchive, "force", "f", false,
		"Skip confirmation prompt")
	rootCmd.AddCommand(unarchiveCmd)
}
