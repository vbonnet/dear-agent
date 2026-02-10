package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/detection"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/fix"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/history"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
)

var (
	fixAll     bool
	fixClear   bool
	fixSession string
)

var fixCmd = &cobra.Command{
	Use:   "fix [session-name]",
	Short: "Fix UUID associations for sessions",
	Long: `Manually associate or fix Claude UUIDs for sessions.

Modes:
  csm fix                  # Scan for unassociated sessions, show suggestions
  csm fix my-session       # Fix specific session with UUID suggestions
  csm fix --all            # Auto-detect and fix all sessions with high confidence
  csm fix --clear my-session  # Clear UUID association

The fix command helps resolve UUID issues:
  • Unassociated sessions (no UUID)
  • Broken associations (UUID not in history)
  • Wrong associations (UUID mismatch)

Examples:
  csm fix                  # Interactive fix for all unassociated
  csm fix my-project       # Fix specific session
  csm fix --all            # Auto-fix all with high confidence
  csm fix --clear old-session  # Remove UUID association`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Create detector and associator
		historyPath := "" // Use default ~/.claude/history.jsonl
		detector := detection.NewDetector(historyPath, 5*time.Minute)
		parser := history.NewParser(historyPath)
		associator := fix.NewAssociator(detector, parser)

		// Handle --clear flag
		if fixClear {
			if len(args) == 0 {
				return fmt.Errorf("--clear requires a session name")
			}
			return clearUUID(args[0], associator)
		}

		// Handle --all flag
		if fixAll {
			return autoFixAll(detector, associator)
		}

		// Handle specific session
		if len(args) > 0 {
			return fixSpecificSession(args[0], associator)
		}

		// Default: scan and show unassociated
		return scanAndFix(associator)
	},
}

func clearUUID(sessionName string, associator *fix.Associator) error {
	// Find session
	m, manifestPath, err := findSession(sessionName)
	if err != nil {
		return err
	}

	if m.Claude.UUID == "" {
		fmt.Printf("Session '%s' has no UUID association.\n", sessionName)
		return nil
	}

	fmt.Printf("Clearing UUID association for '%s'\n", sessionName)
	fmt.Printf("  Current UUID: %s\n", m.Claude.UUID)

	if err := associator.Clear(m, manifestPath); err != nil {
		return fmt.Errorf("failed to clear UUID: %w", err)
	}

	ui.PrintSuccess("UUID association cleared")
	return nil
}

func autoFixAll(detector *detection.Detector, associator *fix.Associator) error {
	fmt.Println("Scanning for sessions to auto-fix...")

	// Get all sessions
	manifests, err := manifest.List(cfg.SessionsDir)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	fixed := 0
	skipped := 0

	for _, m := range manifests {
		// Skip if already has UUID
		if m.Claude.UUID != "" {
			continue
		}

		manifestPath := getManifestPath(m.SessionID)

		// Try auto-detection and association
		associated, err := detector.DetectAndAssociate(m, manifestPath, true)
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("%s: failed to detect UUID: %v", m.Name, err))
			skipped++
			continue
		}

		if associated {
			ui.PrintSuccess(fmt.Sprintf("%s: auto-associated UUID %s", m.Name, m.Claude.UUID))
			fixed++
		} else {
			skipped++
		}
	}

	fmt.Println()
	ui.PrintSuccess(fmt.Sprintf("Auto-fix complete: %d fixed, %d skipped (low confidence or no match)", fixed, skipped))
	return nil
}

func fixSpecificSession(sessionName string, associator *fix.Associator) error {
	// Find session
	m, manifestPath, err := findSession(sessionName)
	if err != nil {
		return err
	}

	fmt.Printf("Fixing UUID association for: %s\n", sessionName)
	if m.Claude.UUID != "" {
		fmt.Printf("  Current UUID: %s\n", m.Claude.UUID)
	} else {
		fmt.Println("  Current UUID: (none)")
	}

	// Get suggestions
	suggestions, err := associator.GetSuggestions(m, 5)
	if err != nil {
		return fmt.Errorf("failed to get suggestions: %w", err)
	}

	if len(suggestions) == 0 {
		fmt.Println()
		ui.PrintWarning("No UUID suggestions available.")
		fmt.Println("Try one of:")
		fmt.Println("  • Send a message in a Claude session for this project")
		fmt.Println("  • Check if ~/.claude/history.jsonl exists and has entries")
		fmt.Println("  • Manually enter UUID if you know it")
		return nil
	}

	// Show suggestions
	fmt.Println("\nUUID Suggestions:")
	for i, sug := range suggestions {
		fmt.Printf("  %d. %s (%s confidence) - %s\n", i+1, sug.UUID, sug.Confidence, sug.Context)
	}
	fmt.Println("  0. Enter UUID manually")
	fmt.Println("  c. Cancel")

	// Prompt for choice
	choice := ""
	fmt.Print("\nSelect option: ")
	fmt.Scanln(&choice)

	switch choice {
	case "c", "C":
		fmt.Println("Cancelled.")
		return nil
	case "0":
		// Manual entry
		fmt.Print("Enter UUID: ")
		var uuid string
		fmt.Scanln(&uuid)
		if uuid == "" {
			return fmt.Errorf("UUID cannot be empty")
		}
		if err := associator.Associate(m, manifestPath, uuid); err != nil {
			return err
		}
		ui.PrintSuccess(fmt.Sprintf("Associated UUID: %s", uuid))
	default:
		// Parse choice
		var idx int
		if _, err := fmt.Sscanf(choice, "%d", &idx); err != nil || idx < 1 || idx > len(suggestions) {
			return fmt.Errorf("invalid choice")
		}
		selectedUUID := suggestions[idx-1].UUID
		if err := associator.Associate(m, manifestPath, selectedUUID); err != nil {
			return err
		}
		ui.PrintSuccess(fmt.Sprintf("Associated UUID: %s", selectedUUID))
	}

	return nil
}

func scanAndFix(associator *fix.Associator) error {
	fmt.Println("Scanning for unassociated sessions...")

	unassociated, err := associator.ScanUnassociated(cfg.SessionsDir)
	if err != nil {
		return fmt.Errorf("failed to scan: %w", err)
	}

	if len(unassociated) == 0 {
		ui.PrintSuccess("All sessions have UUID associations!")
		return nil
	}

	fmt.Printf("\nFound %d unassociated sessions:\n", len(unassociated))
	for i, m := range unassociated {
		fmt.Printf("  %d. %s (project: %s)\n", i+1, m.Name, m.Context.Project)
	}

	fmt.Println("\nOptions:")
	fmt.Println("  • Run 'csm fix <session-name>' to fix a specific session")
	fmt.Println("  • Run 'csm fix --all' to auto-fix all with high confidence")

	return nil
}

func findSession(sessionName string) (*manifest.Manifest, string, error) {
	manifests, err := manifest.List(cfg.SessionsDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed to list sessions: %w", err)
	}

	for _, m := range manifests {
		if m.Name == sessionName {
			manifestPath := getManifestPath(m.SessionID)
			return m, manifestPath, nil
		}
	}

	return nil, "", fmt.Errorf("session '%s' not found", sessionName)
}

func init() {
	fixCmd.Flags().BoolVar(&fixAll, "all", false, "Auto-fix all sessions with high confidence")
	fixCmd.Flags().BoolVar(&fixClear, "clear", false, "Clear UUID association")
	adminCmd.AddCommand(fixCmd)
}
