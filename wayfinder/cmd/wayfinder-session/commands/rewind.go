package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/archive"
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
	Short: "Rewind to a previous phase in V2 sequence",
	Long: `Rewind the V2 session to a previously completed phase.

V2 Phase Sequence:
  CHARTER → PROBLEM → RESEARCH → DESIGN → SPEC → PLAN → SETUP → BUILD → RETRO

This will:
1. Archive the current session state
2. Mark all phases after the target phase as pending
3. Set the current phase to the target phase
4. Log rewind event to retrospective (with optional prompting)

Examples:
  wayfinder-session rewind-to RESEARCH
  wayfinder-session rewind-to PLAN --no-prompt
  wayfinder-session rewind-to DESIGN --reason "Design was too complex"`,
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

	// Read existing V2 STATUS from project directory
	st, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read V2 STATUS file: %w", err)
	}

	// Get all V2 phases
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

	// Mark all phases after target as pending in phase history
	for i := range st.WaypointHistory {
		phaseData := &st.WaypointHistory[i]
		// Find this phase's index in allPhases
		phaseIdx := -1
		for j, p := range allPhases {
			if p == phaseData.Name {
				phaseIdx = j
				break
			}
		}

		// If phase is after target, mark as pending
		if phaseIdx > targetIdx {
			phaseData.Status = status.PhaseStatusV2Pending
			phaseData.CompletedAt = nil
			phaseData.Outcome = nil
		}
	}

	// Update roadmap phases if present
	if st.Roadmap != nil {
		for i := range st.Roadmap.Phases {
			roadmapPhase := &st.Roadmap.Phases[i]
			// Find this phase's index in allPhases
			phaseIdx := -1
			for j, p := range allPhases {
				if p == roadmapPhase.ID {
					phaseIdx = j
					break
				}
			}

			// If phase is after target, mark as pending
			if phaseIdx > targetIdx {
				roadmapPhase.Status = status.PhaseStatusV2Pending
				roadmapPhase.CompletedAt = nil
			}
		}
	}

	// Update current phase
	st.CurrentWaypoint = targetPhase
	st.UpdatedAt = time.Now()

	// Write updated V2 STATUS to project directory
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

	fmt.Printf("⏪ Rewound to phase %s\n", targetPhase)
	fmt.Println("ℹ️  Phases after", targetPhase, "have been reset to pending")
	return nil
}
