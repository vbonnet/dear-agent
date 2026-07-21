package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/tracker"
)

var (
	sessionStatus        string
	sessionBlockedReason string
)

// EndCmd is the cobra command that ends the current Wayfinder session.
var EndCmd = &cobra.Command{
	Use:   "end",
	Short: "End the current canonical Wayfinder session",
	Long: `Update WAYFINDER-STATUS.md and publish session.completed event.

Supported statuses:
  completed  - Project achieved its goals
  abandoned  - Stopped before completion
  blocked    - Paused, may resume later; requires --reason

Example:
  wayfinder session end --status completed`,
	RunE: runEnd,
}

func init() {
	EndCmd.Flags().StringVar(&sessionStatus, "status", "completed", "Session status (completed|abandoned|blocked)")
	EndCmd.Flags().StringVar(&sessionBlockedReason, "reason", "", "Reason the session is blocked (required with --status blocked)")
}

func runEnd(cmd *cobra.Command, args []string) error {
	return runEndV2(GetProjectDirectory(), sessionStatus, sessionBlockedReason)
}

func runEndInDir(projectDir, newStatus string) error {
	return runEndV2(projectDir, newStatus, "")
}

// runEndV2 handles canonical WAYFINDER-STATUS.md files.
func runEndV2(projectDir, newStatus, blockedReason string) error {
	validStatuses := map[string]bool{
		status.StatusV2Completed: true,
		status.StatusV2Abandoned: true,
		status.StatusV2Blocked:   true,
	}
	if !validStatuses[newStatus] {
		return fmt.Errorf("invalid status: %s (must be completed, abandoned, or blocked)", newStatus)
	}
	if newStatus == status.StatusV2Blocked {
		blockedReason = strings.TrimSpace(blockedReason)
		if blockedReason == "" {
			return fmt.Errorf("blocked status requires --reason")
		}
	}

	st, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read canonical status file: %w", err)
	}
	if newStatus == status.StatusV2Completed {
		if err := status.ValidateSessionCompletion(st); err != nil {
			return err
		}
	}

	now := time.Now()
	switch newStatus {
	case status.StatusV2Completed:
		applyLifecycleState(st, status.LifecycleCompleted, "", "", "", now)
	case status.StatusV2Abandoned:
		applyLifecycleState(st, status.LifecycleCanceled, "", "", "", now)
	case status.StatusV2Blocked:
		st.Status = status.StatusV2Blocked
		st.LifecycleState = ""
		st.BlockedReason = blockedReason
		st.BlockedOn = ""
		st.ErrorMessage = ""
		st.InputNeeded = ""
		st.CompletionDate = nil
		st.UpdatedAt = now
	}

	// Guard against zero CreatedAt — use UpdatedAt as the session start if unset.
	startedAt := st.CreatedAt
	if startedAt.IsZero() {
		startedAt = now
	}

	sessionID := fmt.Sprintf("session-%d", startedAt.Unix())
	tr, err := tracker.New(sessionID)
	if err != nil {
		return fmt.Errorf("failed to initialize tracker: %w", err)
	}
	defer func() { _ = tr.Close(context.Background()) }()

	if err := tr.EndSession(newStatus); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish session.completed event: %v\n", err)
	}

	if err := status.ValidateV2(st); err != nil {
		return fmt.Errorf("invalid completed session status: %w", err)
	}

	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	duration := now.Sub(startedAt)

	fmt.Printf("✅ Wayfinder session ended\n")
	fmt.Printf("Project: %s\n", st.ProjectName)
	fmt.Printf("Duration: %s\n", formatDuration(duration))
	fmt.Printf("Status: %s\n", newStatus)
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
