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
	Long: `Show the canonical Wayfinder session status.

Example:
	wayfinder session status`,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()
	currentStatus, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read canonical status: %w", err)
	}

	// Display status
	fmt.Printf("Wayfinder Session Status\n")
	fmt.Printf("=========================\n\n")

	fmt.Printf("Source: %s\n", status.StatusFilename)
	fmt.Printf("Project: %s\n", currentStatus.ProjectName)
	fmt.Printf("Schema: %s (canonical 9 phases)\n", status.SchemaVersion)
	if !currentStatus.CreatedAt.IsZero() {
		fmt.Printf("Started: %s\n", currentStatus.CreatedAt.Format("2006-01-02 15:04 MST"))
	}
	if currentStatus.CompletionDate != nil {
		fmt.Printf("Ended: %s\n", currentStatus.CompletionDate.Format("2006-01-02 15:04 MST"))
	}
	fmt.Printf("Status: %s\n", currentStatus.Status)
	fmt.Printf("Current Phase: %s\n", currentStatus.CurrentWaypoint)
	fmt.Printf("\n")

	// Display phase progress
	fmt.Printf("Phase Progress:\n")
	fmt.Printf("---------------\n")

	if len(currentStatus.WaypointHistory) == 0 {
		fmt.Printf("  (no phases started)\n")
	} else {
		for _, phase := range currentStatus.WaypointHistory {
			symbol := getPhaseSymbol(phase.Status, currentStatus.CurrentWaypoint == phase.Name)
			suffix := getPhaseStatusText(phase.Status, currentStatus.CurrentWaypoint == phase.Name)

			fmt.Printf("%s %s %s\n", symbol, phase.Name, suffix)
		}
	}

	// Show remaining phases
	fmt.Printf("\nRemaining Phases:\n")
	fmt.Printf("-----------------\n")

	existingPhases := make(map[string]bool)
	for _, phase := range currentStatus.WaypointHistory {
		existingPhases[phase.Name] = true
	}

	hasRemaining := false
	for _, phaseName := range status.AllPhasesV2() {
		if !existingPhases[phaseName] {
			fmt.Printf("  %s %s\n", phaseName, remainingPhaseStatus(currentStatus, phaseName))
			hasRemaining = true
		}
	}

	if !hasRemaining {
		fmt.Printf("  (all phases started)\n")
	}

	return nil
}

func remainingPhaseStatus(currentStatus *status.StatusV2, phaseName string) string {
	if currentStatus.IsPhaseSkipped(phaseName) {
		return "(skipped)"
	}
	return "(pending)"
}

// getPhaseSymbol returns the display symbol for a phase
func getPhaseSymbol(phaseStatus string, isCurrent bool) string {
	switch phaseStatus {
	case status.WaypointStatusV2Completed:
		return "✓"
	case status.WaypointStatusV2InProgress:
		if isCurrent {
			return "→"
		}
		return "○"
	case status.WaypointStatusV2Skipped:
		return "⊘"
	default:
		return "○"
	}
}

// getPhaseStatusText returns the status text for a phase
func getPhaseStatusText(phaseStatus string, isCurrent bool) string {
	switch phaseStatus {
	case status.WaypointStatusV2Completed:
		return "(validated)"
	case status.WaypointStatusV2InProgress:
		if isCurrent {
			return "(in progress - current)"
		}
		return "(in progress - no signature)"
	case status.WaypointStatusV2Skipped:
		return "(skipped)"
	default:
		return "(pending)"
	}
}
