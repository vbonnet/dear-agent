package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/archive"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/git"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/retrospective"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

var (
	// Rewind command flags
	rewindNoPrompt  bool
	rewindReason    string
	rewindLearnings string
)

// RewindCmd is the cobra command that rewinds the session to a previous phase.
var RewindCmd = &cobra.Command{
	Use:   "rewind-to <phase-name>",
	Short: "Rewind to a previous phase in the canonical sequence",
	Long: `Rewind the session to a previously completed phase.

Phase sequence:
  CHARTER → PROBLEM → RESEARCH → DESIGN → SPEC → PLAN → SETUP → BUILD → RETRO

This will:
1. Archive the current session state
2. Mark all phases after the target phase as pending
3. Set the current phase to the target phase
4. Log rewind event to retrospective (with optional prompting)
5. Commit canonical rewind markers when the project is a Git repository

Examples:
  wayfinder session rewind-to RESEARCH
  wayfinder session rewind-to PLAN --no-prompt
  wayfinder session rewind-to DESIGN --reason "Design was too complex"`,
	Args: cobra.ExactArgs(1),
	RunE: runRewind,
}

func init() {
	RewindCmd.Flags().BoolVar(&rewindNoPrompt, "no-prompt", false, "Skip prompting for reason/learnings")
	RewindCmd.Flags().StringVar(&rewindReason, "reason", "", "Pre-provide rewind reason (bypasses prompt)")
	RewindCmd.Flags().StringVar(&rewindLearnings, "learnings", "", "Pre-provide learnings (bypasses prompt)")
}

//nolint:gocyclo // reason: linear CLI driver covering many rewind targets
func runRewind(cmd *cobra.Command, args []string) error {
	targetPhase := args[0]

	// Get project directory
	projectDir := GetProjectDirectory()

	// Read existing canonical status from the project directory.
	st, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read canonical status file: %w", err)
	}

	// Get the canonical phase sequence.
	allPhases := status.AllPhasesV2Schema()

	// Find target phase index
	targetIdx := -1
	for i, phase := range allPhases {
		if phase == targetPhase {
			targetIdx = i
			break
		}
	}

	if targetIdx == -1 {
		return fmt.Errorf("invalid target phase: %s (valid phases: CHARTER, PROBLEM, RESEARCH, DESIGN, SPEC, PLAN, SETUP, BUILD, RETRO)", targetPhase)
	}

	// Validate that target phase has been completed
	targetHistory := st.GetPhaseHistory(targetPhase)
	if targetHistory == nil || (targetHistory.Status != status.PhaseStatusV2Completed && targetHistory.Status != status.PhaseStatusV2Skipped) {
		return fmt.Errorf("cannot rewind to phase %s: phase has not been completed yet", targetPhase)
	}

	// Archive current state before rewinding
	archiver := archive.New(projectDir)
	if err := archiver.ArchivePhase(st.CurrentWaypoint); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to archive current state: %v\n", err)
	} else {
		fmt.Println("📦 Current state archived")
	}

	// Capture fromPhase BEFORE updating (needed for retrospective logging)
	fromPhase := st.CurrentWaypoint

	resetForRewind(st, allPhases, targetIdx)
	resetLifecycleForRewind(st, time.Now())

	// Update current phase
	st.CurrentWaypoint = targetPhase

	// Write updated canonical status to the project directory.
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	// Log rewind event to retrospective (dual logging: JSON + markdown)
	// Errors are non-blocking (logged to stderr)
	flags := retrospective.RewindFlags{
		NoPrompt:  rewindNoPrompt,
		Reason:    rewindReason,
		Learnings: rewindLearnings,
	}
	if err := retrospective.LogRewindEvent(projectDir, fromPhase, targetPhase, flags); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: retrospective logging failed: %v\n", err)
	}
	gitIntegrator := git.New(projectDir)
	if gitIntegrator.IsGitRepo() {
		if err := gitIntegrator.CommitRewind(fromPhase, targetPhase); err != nil {
			return fmt.Errorf("commit rewind state before restarting %s: %w", targetPhase, err)
		}
		fmt.Println("📝 Rewind state committed")
	}

	fmt.Printf("⏪ Rewound to phase %s\n", targetPhase)
	fmt.Println("ℹ️  Phases after", targetPhase, "have been reset to pending")
	return nil
}

func resetLifecycleForRewind(st *status.StatusV2, now time.Time) {
	applyLifecycleState(st, status.LifecycleWorking, "", "", "", now)
}

func resetForRewind(st *status.StatusV2, allPhases []string, targetIdx int) {
	positions := make(map[string]int, len(allPhases))
	for index, phase := range allPhases {
		positions[phase] = index
	}
	retainedHistory := st.WaypointHistory[:0]
	for _, phase := range st.WaypointHistory {
		if positions[phase.Name] < targetIdx {
			retainedHistory = append(retainedHistory, phase)
		}
	}
	st.WaypointHistory = retainedHistory
	if st.Roadmap == nil {
		return
	}
	for index := range st.Roadmap.Phases {
		phase := &st.Roadmap.Phases[index]
		if positions[phase.ID] < targetIdx {
			continue
		}
		phase.Status = status.PhaseStatusV2Pending
		phase.StartedAt = nil
		phase.CompletedAt = nil
	}
}
