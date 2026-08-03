package commands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/archive"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/git"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
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
func runRewind(cmd *cobra.Command, args []string) (runErr error) {
	targetPhase := args[0]

	// Get project directory
	projectDir := GetProjectDirectory()
	transitionLock, err := acquireRewindTransitionLock(projectDir)
	if err != nil {
		return fmt.Errorf("acquire rewind transition lock: %w", err)
	}
	defer func() {
		if closeErr := transitionLock.Close(); closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("release rewind transition lock: %w", closeErr))
		}
	}()

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

	if err := validateRewindTarget(st, targetPhase); err != nil {
		return err
	}

	// Migrate a legacy history filename only after every rewind precondition
	// holds. A rejected rewind must leave the project byte-for-byte unchanged;
	// renaming first strands an uncommitted rename that the next start-phase
	// refuses to migrate because its dirty-worktree check runs first.
	if err := history.New(projectDir).EnsureCurrentFile(); err != nil {
		return fmt.Errorf("prepare history for rewind: %w", err)
	}

	// Archive current state before rewinding. Archive publication is required so
	// a reset status never exists without its pre-rewind snapshot.
	archiver := archive.New(projectDir)
	archiveRef, err := archiver.ArchivePhase(st.CurrentWaypoint)
	if err != nil {
		return fmt.Errorf("archive current state: %w", err)
	}
	fmt.Println("📦 Current state archived")

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

	// Log rewind event to retrospective (dual logging: JSON + markdown). These
	// writes are required evidence for every accepted rewind, including a
	// same-phase replay with magnitude zero.
	flags := retrospective.RewindFlags{
		NoPrompt:  rewindNoPrompt,
		Reason:    rewindReason,
		Learnings: rewindLearnings,
	}
	if err := retrospective.LogRewindEvent(projectDir, fromPhase, targetPhase, flags); err != nil {
		return fmt.Errorf("persist rewind trace: %w", err)
	}
	gitIntegrator := git.New(projectDir)
	if err := commitRewindState(gitIntegrator, fromPhase, targetPhase, archiveRef, os.Stdout); err != nil {
		return err
	}

	fmt.Printf("⏪ Rewound to phase %s\n", targetPhase)
	fmt.Println("ℹ️  Phases after", targetPhase, "have been reset to pending")
	return nil
}

type rewindCommitter interface {
	IsGitRepo() bool
	CommitRewind(fromPhase, toPhase string, archiveRef archive.ArchiveRef) error
}

func commitRewindState(integrator rewindCommitter, fromPhase, targetPhase string, archiveRef archive.ArchiveRef, stdout io.Writer) error {
	if !integrator.IsGitRepo() {
		return nil
	}
	if err := integrator.CommitRewind(fromPhase, targetPhase, archiveRef); err != nil {
		return fmt.Errorf("commit rewind trace: %w", err)
	}
	fmt.Fprintln(stdout, "📝 Rewind state committed")
	return nil
}

func validateRewindTarget(st *status.StatusV2, targetPhase string) error {
	targetHistory := st.GetPhaseHistory(targetPhase)
	if targetHistory == nil || (targetHistory.Status != status.PhaseStatusV2Completed && targetHistory.Status != status.PhaseStatusV2Skipped) {
		return fmt.Errorf("cannot rewind to phase %s: phase has not been completed yet", targetPhase)
	}
	if st.IsPhaseSkipped(targetPhase) {
		return fmt.Errorf("cannot rewind to phase %s: phase is configured to be skipped", targetPhase)
	}
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
