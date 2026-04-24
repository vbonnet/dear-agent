package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// StatusCmd displays the current Wayfinder session status
var StatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Wayfinder session status",
	Long: `Show the current Wayfinder session status by scanning phase files on disk.

This command detects phase status from the filesystem (stateless mode):
- Scans for W*.md, D*.md, S*.md files
- Checks validation signatures to determine completion
- Shows current phase and progress

If WAYFINDER-STATUS.md exists, it will be used as fallback.

Flags:
  --force-fs    Force filesystem detection even if STATUS file exists

Example:
  wayfinder-session status
  wayfinder-session status --force-fs`,
	RunE: runStatus,
}

func init() {
	StatusCmd.Flags().Bool("force-fs", false, "Force filesystem detection (ignore STATUS file)")
}

func runStatus(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()

	forceFs, _ := cmd.Flags().GetBool("force-fs")

	var currentStatus *status.Status
	var err error
	var source string

	// Try filesystem detection first if --force-fs, otherwise try STATUS file
	if forceFs {
		currentStatus, err = status.DetectFromFilesystem(projectDir)
		source = "filesystem"
	} else {
		// Try STATUS file first (backward compatibility)
		currentStatus, err = status.ReadFrom(projectDir)
		if err != nil {
			// Fallback to filesystem detection
			currentStatus, err = status.DetectFromFilesystem(projectDir)
			source = "filesystem"
		} else {
			source = "STATUS file"
		}
	}

	if err != nil {
		return fmt.Errorf("failed to read status: %w", err)
	}

	// Display status
	fmt.Printf("Wayfinder Session Status\n")
	fmt.Printf("=========================\n\n")

	fmt.Printf("Source: %s\n", source)
	if currentStatus.ProjectPath != "" {
		fmt.Printf("Project: %s\n", currentStatus.ProjectPath)
	}
	if currentStatus.SessionID != "" {
		fmt.Printf("Session ID: %s\n", currentStatus.SessionID)
	}

	// Display version information
	version := currentStatus.GetVersion()
	phaseSchema := "W0-W12"
	if version == status.WayfinderV2 {
		phaseSchema = "dot-notation"
	}
	fmt.Printf("Version: %s (%s phases)\n", version, phaseSchema)

	if !currentStatus.StartedAt.IsZero() {
		fmt.Printf("Started: %s\n", currentStatus.StartedAt.Format("2006-01-02 15:04 MST"))
	}
	if currentStatus.EndedAt != nil {
		fmt.Printf("Ended: %s\n", currentStatus.EndedAt.Format("2006-01-02 15:04 MST"))
	}
	fmt.Printf("Status: %s\n", currentStatus.Status)
	fmt.Printf("Current Phase: %s\n", currentStatus.CurrentPhase)
	fmt.Printf("\n")

	// Display phase progress
	fmt.Printf("Phase Progress:\n")
	fmt.Printf("---------------\n")

	if len(currentStatus.Phases) == 0 {
		fmt.Printf("  (no phases started)\n")
	} else {
		for _, phase := range currentStatus.Phases {
			symbol := getPhaseSymbol(phase.Status, currentStatus.CurrentPhase == phase.Name)
			suffix := getPhaseStatusText(phase.Status, currentStatus.CurrentPhase == phase.Name)

			fmt.Printf("%s %s %s\n", symbol, phase.Name, suffix)
		}
	}

	// Show remaining phases
	fmt.Printf("\nRemaining Phases:\n")
	fmt.Printf("-----------------\n")

	existingPhases := make(map[string]bool)
	for _, phase := range currentStatus.Phases {
		existingPhases[phase.Name] = true
	}

	hasRemaining := false
	for _, phaseName := range status.AllPhases(currentStatus.GetVersion()) {
		if !existingPhases[phaseName] {
			fmt.Printf("  %s (pending)\n", phaseName)
			hasRemaining = true
		}
	}

	if !hasRemaining {
		fmt.Printf("  (all phases started)\n")
	}

	return nil
}

// getPhaseSymbol returns the display symbol for a phase
func getPhaseSymbol(phaseStatus string, isCurrent bool) string {
	switch phaseStatus {
	case status.PhaseStatusCompleted:
		return "✓"
	case status.PhaseStatusInProgress:
		if isCurrent {
			return "→"
		}
		return "○"
	case status.PhaseStatusSkipped:
		return "⊘"
	default:
		return "○"
	}
}

// getPhaseStatusText returns the status text for a phase
func getPhaseStatusText(phaseStatus string, isCurrent bool) string {
	switch phaseStatus {
	case status.PhaseStatusCompleted:
		return "(validated)"
	case status.PhaseStatusInProgress:
		if isCurrent {
			return "(in progress - current)"
		}
		return "(in progress - no signature)"
	case status.PhaseStatusSkipped:
		return "(skipped)"
	default:
		return "(pending)"
	}
}
