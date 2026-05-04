package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/tracker"
)

var sessionStatus string

// EndCmd is the cobra command that ends the current Wayfinder session.
var EndCmd = &cobra.Command{
	Use:   "end",
	Short: "End the current Wayfinder V2 session",
	Long: `Update WAYFINDER-STATUS.md and publish session.completed event.

Supported V2 statuses:
  completed  - Project achieved its goals
  abandoned  - Stopped before completion
  blocked    - Paused, may resume later

Example:
  wayfinder-session end --status completed`,
	RunE: runEnd,
}

func init() {
	EndCmd.Flags().StringVar(&sessionStatus, "status", "completed", "Session status (completed|abandoned|blocked)")
}

func runEnd(cmd *cobra.Command, args []string) error {
	// Get project directory
	projectDir := GetProjectDirectory()

	// Read existing V2 STATUS from project directory
	st, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read V2 STATUS file: %w", err)
	}

	// Validate status value
	validStatuses := map[string]bool{
		status.StatusV2Completed: true,
		status.StatusV2Abandoned: true,
		status.StatusV2Blocked:   true,
	}
	if !validStatuses[sessionStatus] {
		return fmt.Errorf("invalid status: %s (must be completed, abandoned, or blocked)", sessionStatus)
	}

	// Update session status
	now := time.Now()
	st.CompletionDate = &now
	st.Status = sessionStatus
	st.UpdatedAt = now

	// Initialize tracker (use project name as session ID)
	sessionID := fmt.Sprintf("session-%d", st.CreatedAt.Unix())
	tr, err := tracker.New(sessionID)
	if err != nil {
		return fmt.Errorf("failed to initialize tracker: %w", err)
	}
	defer tr.Close(context.Background())

	// Publish session.completed event
	if err := tr.EndSession(sessionStatus); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish session.completed event: %v\n", err)
	}

	// Validate V2 schema before writing
	if err := status.ValidateV2(st); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: validation errors found:\n%v\n", err)
	}

	// Write updated V2 STATUS to project directory
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	// Calculate duration
	duration := now.Sub(st.CreatedAt)

	// Success message
	fmt.Printf("✅ Wayfinder V2 session ended\n")
	fmt.Printf("Project: %s\n", st.ProjectName)
	fmt.Printf("Duration: %s\n", formatDuration(duration))
	fmt.Printf("Status: %s\n", sessionStatus)
	fmt.Printf("Phases completed: %d\n", countCompletedPhasesV2(st))

	return nil
}

func countCompletedPhasesV2(st *status.StatusV2) int {
	count := 0
	for _, phase := range st.WaypointHistory {
		if phase.Status == status.PhaseStatusV2Completed {
			count++
		}
	}
	return count
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}
