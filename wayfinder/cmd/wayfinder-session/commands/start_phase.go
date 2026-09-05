package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/beads"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/git"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/tracker"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/validator"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var (
	allowDirty bool
)

// StartPhaseCmd is the cobra command that marks a phase as started.
var StartPhaseCmd = &cobra.Command{
	Use:   "start-phase <phase-name>",
	Short: "Mark a phase as started",
	Long: `Update WAYFINDER-STATUS.md and publish wayfinder.phase.started event.

Example:
  wayfinder session start-phase PROBLEM
  wayfinder session start-phase BUILD --allow-dirty`,
	Args: cobra.ExactArgs(1),
	RunE: runStartPhase,
}

func init() {
	StartPhaseCmd.Flags().BoolVar(&allowDirty, "allow-dirty", false, "Allow phase transition with uncommitted files in project directory")
}

func runStartPhase(cmd *cobra.Command, args []string) (retErr error) {
	phaseName := args[0]

	// Trace the phase transition. cmd.Context() is always non-nil (cobra
	// defaults to context.Background()); the span is a no-op unless a collector
	// is configured. Span name keeps the SDLC phase transition legible in Jaeger.
	_, span := otel.Tracer("wayfinder").Start(cmd.Context(), "wayfinder.phase.start",
		trace.WithAttributes(attribute.String("phase.name", phaseName)))
	defer func() {
		if retErr != nil {
			span.SetStatus(codes.Error, retErr.Error())
		}
		span.End()
	}()

	// Get project directory
	projectDir := GetProjectDirectory()

	// Check for uncommitted files in project directory (unless --allow-dirty)
	if !allowDirty {
		gitIntegrator := git.New(projectDir)
		uncommittedFiles, err := gitIntegrator.GetUncommittedFilesInProjectDir()
		if err != nil {
			return fmt.Errorf("failed to check git status: %w", err)
		}
		if len(uncommittedFiles) > 0 {
			return fmt.Errorf("uncommitted files detected in project directory:\n  %s\n\nCommit your changes before transitioning phases, or use --allow-dirty to override",
				strings.Join(uncommittedFiles, "\n  "))
		}
	}

	st, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read canonical status file: %w", err)
	}

	hist := history.New(projectDir)

	// Validate phase can be started
	v := validator.NewValidator(st)
	if err := v.CanStartPhase(phaseName, projectDir); err != nil {
		// Log validation failure
		failureData := map[string]interface{}{
			"error": err.Error(),
		}
		if logErr := hist.AppendEvent(history.EventTypeValidationFailed, phaseName, failureData); logErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to log validation failure: %v\n", logErr)
		}
		return fmt.Errorf("validation failed: %w", err)
	}

	// Initialize tracker
	tr, err := tracker.New(st.GetSessionID())
	if err != nil {
		return fmt.Errorf("failed to initialize tracker: %w", err)
	}
	defer func() { _ = tr.Close(context.Background()) }()

	if err := hist.EnsureCurrentFile(); err != nil {
		return fmt.Errorf("prepare history for phase transition: %w", err)
	}

	// Update phase status
	st.UpdatePhase(phaseName, status.PhaseStatusInProgress, "")
	st.SetCurrentPhase(phaseName)

	// Guarantee the session is backed by a tracking bead when task execution
	// begins. SETUP normally owns this transition; --skip-roadmap and explicit
	// phase profiles may skip SETUP, so BUILD must cover that path as well.
	if shouldEnsureSessionBead(st, phaseName) {
		ensureSessionBead(cmd.Context(), st)
	}

	// Publish wayfinder.phase.started event
	if err := tr.StartPhase(phaseName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish wayfinder.phase.started event: %v\n", err)
	}

	// Log phase started to history
	if err := hist.AppendEvent(history.EventTypePhaseStarted, phaseName, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log wayfinder.phase.started to history: %v\n", err)
	}

	// Write updated STATUS to project directory
	if err := st.WriteTo(projectDir); err != nil {
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	// Auto-commit the marker files written above so the worktree stays clean.
	// Without this, the next start-phase call sees WAYFINDER-STATUS.md and
	// WAYFINDER-HISTORY.jsonl as uncommitted and refuses (ce-fvkz recurrence).
	// This mirrors the auto-commit that complete-phase already performs.
	gitIntegrator := git.New(projectDir)
	if gitIntegrator.IsGitRepo() {
		committed, err := gitIntegrator.CommitPhaseStart(phaseName)
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "Warning: failed to create git commit: %v\n", err)
		case committed:
			fmt.Println("📝 Git commit created")
		default:
			fmt.Println("📝 No git commit: the repository ignores the Wayfinder markers")
		}
	}

	fmt.Printf("✅ Phase %s started\n", phaseName)
	return nil
}

func shouldEnsureSessionBead(st *status.StatusV2, phaseName string) bool {
	if strings.EqualFold(phaseName, status.WaypointV2Setup) {
		return true
	}
	return strings.EqualFold(phaseName, status.WaypointV2Build) && st.IsPhaseSkipped(status.WaypointV2Setup)
}

// ensureSessionBead files a tracking bead for the session if it has none yet,
// recording the new id on the status so it is persisted by the WriteTo that
// follows. Bead tracking lives in canonical status (StatusV2.Beads); for any other
// status version this is a no-op. Failures (bd absent, create error) warn and
// continue — a missing tracker must never block a phase transition.
func ensureSessionBead(ctx context.Context, st *status.StatusV2) {
	if len(st.Beads) > 0 {
		return
	}
	if !beads.Available() {
		fmt.Fprintf(os.Stderr, "Warning: bd CLI not available; skipping auto-bead creation\n")
		return
	}
	title := strings.TrimSpace(st.ProjectName)
	if title == "" {
		title = "wayfinder session"
	}
	id, err := beads.Create(ctx, beads.DefaultDB(), title)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-create bead: %v\n", err)
		return
	}
	st.Beads = append(st.Beads, id)
	fmt.Printf("🔗 Auto-created bead %s for this task\n", id)
}
