package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/backup"
	"github.com/vbonnet/dear-agent/agm/internal/conversation"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// Command-specific flags (avoid global variable conflicts)
var (
	cleanupDryRun           bool
	cleanupMessageThreshold int
	cleanupPattern          string
)

// sessionCandidate represents a session that may be cleaned up
type sessionCandidate struct {
	SessionID    string
	Name         string
	MessageCount int
	LastActivity time.Time
	ManifestPath string
}

var cleanupTestSessionsCmd = &cobra.Command{
	Use:   "cleanup-test-sessions",
	Short: "Clean up test sessions from production workspace",
	Long: `Safely removes test-* sessions with trivial conversation history.

This command helps maintain a clean session workspace by identifying and
removing test sessions that have minimal conversation activity.

Algorithm:
1. Scan all sessions in the workspace
2. Filter by pattern (default: ^test-)
3. Analyze conversation.jsonl for message count
4. Interactively select sessions to delete
5. Backup each session before deletion
6. Delete selected session directories

Safety Features:
- Dry-run mode for preview (--dry-run)
- Backup before deletion (stored in ~/.agm/backups/sessions/)
- Interactive multi-select (pre-selects trivial sessions)
- Confirmation prompt before deletion

Examples:
  agm admin cleanup-test-sessions                           # Interactive cleanup
  agm admin cleanup-test-sessions --dry-run                 # Preview only
  agm admin cleanup-test-sessions --message-threshold=10    # Custom threshold
  agm admin cleanup-test-sessions --pattern="^demo-"        # Different pattern

Flags:
  --dry-run              Preview only, no deletions
  --message-threshold    Minimum messages to preserve (default: 5)
  --pattern              Regex pattern for session names (default: ^test-)`,
	RunE: runCleanupTestSessions,
}

func init() {
	adminCmd.AddCommand(cleanupTestSessionsCmd)

	cleanupTestSessionsCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Preview only, no deletions")
	cleanupTestSessionsCmd.Flags().IntVar(&cleanupMessageThreshold, "message-threshold", 5, "Minimum messages to preserve")
	cleanupTestSessionsCmd.Flags().StringVar(&cleanupPattern, "pattern", "^test-", "Regex pattern for session names")
}

func runCleanupTestSessions(cmd *cobra.Command, args []string) error {
	fmt.Println(ui.Blue("=== Test Session Cleanup ===\n"))

	// Get sessions directory from config
	sessionsDir := cfg.SessionsDir
	if sessionsDir == "" {
		homeDir, _ := os.UserHomeDir()
		sessionsDir = filepath.Join(homeDir, ".claude", "sessions")
	}

	fmt.Printf("Sessions directory: %s\n", sessionsDir)
	fmt.Printf("Pattern filter: %s\n", cleanupPattern)
	fmt.Printf("Message threshold: %d\n", cleanupMessageThreshold)
	if cleanupDryRun {
		fmt.Println(ui.Yellow("Mode: DRY-RUN (preview only, no deletions)"))
	}
	fmt.Println()

	// Compile regex pattern
	namePattern, err := regexp.Compile(cleanupPattern)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}

	// Step 1: List all sessions from Dolt
	fmt.Println("Scanning sessions...")
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer adapter.Close()

	manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	// Step 2: Filter by pattern and analyze conversation
	var candidates []sessionCandidate
	for _, m := range manifests {
		// Skip archived sessions
		if m.Lifecycle == "archived" {
			continue
		}

		// Filter by pattern
		if !namePattern.MatchString(m.Name) {
			continue
		}

		// Build conversation path
		conversationPath := filepath.Join(sessionsDir, m.SessionID, "conversation.jsonl")

		// Count messages
		messageCount, err := conversation.CountMessages(conversationPath)
		if err != nil {
			// Log warning but continue
			fmt.Fprintf(os.Stderr, "Warning: Failed to count messages for %s: %v\n", m.Name, err)
			messageCount = 0
		}

		candidates = append(candidates, sessionCandidate{
			SessionID:    m.SessionID,
			Name:         m.Name,
			MessageCount: messageCount,
			LastActivity: m.UpdatedAt,
			ManifestPath: filepath.Join(sessionsDir, m.SessionID),
		})
	}

	if len(candidates) == 0 {
		ui.PrintSuccess("No matching sessions found")
		return nil
	}

	fmt.Printf("Found %d matching session(s)\n\n", len(candidates))

	// Display preview table
	displayCandidatesTable(candidates)
	fmt.Println()

	// If dry-run, stop here
	if cleanupDryRun {
		fmt.Println(ui.Blue("Dry-run complete. No sessions were deleted."))
		fmt.Println("Run without --dry-run to perform actual cleanup.")
		return nil
	}

	// Step 3: Interactive multi-select
	selectedIDs, err := selectSessionsForDeletion(candidates)
	if err != nil {
		return fmt.Errorf("failed to select sessions: %w", err)
	}

	if len(selectedIDs) == 0 {
		fmt.Println("No sessions selected. Exiting.")
		return nil
	}

	// Step 4: Confirm deletion
	confirmed, err := confirmDeletion(selectedIDs)
	if err != nil {
		return fmt.Errorf("failed to confirm deletion: %w", err)
	}
	if !confirmed {
		fmt.Println("Deletion cancelled.")
		return nil
	}

	// Step 5: Delete selected sessions
	fmt.Println()
	fmt.Println(ui.Blue("Deleting selected sessions..."))
	deletedCount := 0
	backupPaths := []string{}

	for _, sessionID := range selectedIDs {
		// Find candidate info
		var candidate *sessionCandidate
		for i := range candidates {
			if candidates[i].SessionID == sessionID {
				candidate = &candidates[i]
				break
			}
		}
		if candidate == nil {
			continue
		}

		fmt.Printf("\n  • %s (ID: %s, %d messages)\n", candidate.Name, sessionID[:8], candidate.MessageCount)

		// Backup session
		fmt.Printf("    Backing up... ")
		backupPath, err := backup.BackupSessionFromDir(sessionID, sessionsDir)
		if err != nil {
			fmt.Printf("FAILED: %v\n", err)
			fmt.Printf("    Skipping deletion for safety\n")
			continue
		}
		fmt.Printf("✓ Backed up to %s\n", backupPath)
		backupPaths = append(backupPaths, backupPath)

		// Verify backup exists
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			fmt.Printf("    ERROR: Backup verification failed\n")
			fmt.Printf("    Skipping deletion for safety\n")
			continue
		}

		// Delete session directory
		fmt.Printf("    Deleting session directory... ")
		sessionDir := filepath.Join(sessionsDir, sessionID)
		if err := os.RemoveAll(sessionDir); err != nil {
			fmt.Printf("FAILED: %v\n", err)
			continue
		}
		fmt.Printf("✓ Deleted\n")
		deletedCount++
	}

	// Step 6: Report results
	fmt.Println()
	fmt.Println(ui.Blue("=== Cleanup Complete ==="))
	fmt.Printf("Sessions deleted: %d\n", deletedCount)
	fmt.Printf("Backups created: %d\n", len(backupPaths))

	if len(backupPaths) > 0 {
		fmt.Println("\nBackup locations:")
		for _, path := range backupPaths {
			fmt.Printf("  • %s\n", path)
		}
		fmt.Println("\nTo restore a session, use:")
		fmt.Println("  agm admin restore-backup <backup-path>")
	}

	return nil
}

func displayCandidatesTable(candidates []sessionCandidate) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header
	fmt.Fprintln(w, "Session Name\tMessages\tLast Activity\tSession ID")
	fmt.Fprintln(w, "------------\t--------\t-------------\t----------")

	// Rows
	for _, c := range candidates {
		sessionID := c.SessionID
		if len(sessionID) > 8 {
			sessionID = sessionID[:8] + "..."
		}

		lastActivity := c.LastActivity.Format("2006-01-02 15:04")
		if c.LastActivity.IsZero() {
			lastActivity = "(unknown)"
		}

		// Highlight trivial sessions (below threshold)
		nameDisplay := c.Name
		if c.MessageCount < cleanupMessageThreshold {
			nameDisplay = ui.Yellow(c.Name)
		}

		fmt.Fprintf(w, "%s\t%d\t%s\t%s\n",
			nameDisplay, c.MessageCount, lastActivity, sessionID)
	}
}

func selectSessionsForDeletion(candidates []sessionCandidate) ([]string, error) {
	fmt.Println(ui.Blue("Select sessions to delete:"))
	fmt.Println("(Sessions with < " + fmt.Sprintf("%d", cleanupMessageThreshold) + " messages are pre-selected)")
	fmt.Println()

	// Build options with pre-selection for trivial sessions
	var options []huh.Option[string]
	var preSelected []string

	for _, c := range candidates {
		label := fmt.Sprintf("%s (%d messages, updated %s)",
			c.Name,
			c.MessageCount,
			c.LastActivity.Format("2006-01-02"))

		options = append(options, huh.NewOption(label, c.SessionID))

		// Pre-select if below threshold
		if c.MessageCount < cleanupMessageThreshold {
			preSelected = append(preSelected, c.SessionID)
		}
	}

	// Interactive multi-select
	var selected []string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Sessions to delete:").
				Options(options...).
				Value(&selected).
				Height(15),
		),
	)

	// Set initial value to pre-selected sessions
	selected = preSelected

	if err := form.Run(); err != nil {
		return nil, err
	}

	return selected, nil
}

func confirmDeletion(sessionIDs []string) (bool, error) {
	fmt.Println()
	fmt.Println(ui.Yellow(fmt.Sprintf("⚠️  You are about to delete %d session(s)", len(sessionIDs))))
	fmt.Println("Backups will be created before deletion.")
	fmt.Println()

	var confirmed bool
	err := huh.NewConfirm().
		Title("Proceed with deletion?").
		Affirmative("Yes, delete selected sessions").
		Negative("No, cancel").
		Value(&confirmed).
		Run()

	return confirmed, err
}
