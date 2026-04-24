package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/git"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/history"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/tracker"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/validator"
)

var (
	allowDirty bool
)

var StartPhaseCmd = &cobra.Command{
	Use:   "start-phase <phase-name>",
	Short: "Mark a phase as started",
	Long: `Update WAYFINDER-STATUS.md and publish phase.started event.

Example:
  wayfinder-session start-phase PROBLEM
  wayfinder-session start-phase BUILD --allow-dirty`,
	Args: cobra.ExactArgs(1),
	RunE: runStartPhase,
}

func init() {
	StartPhaseCmd.Flags().BoolVar(&allowDirty, "allow-dirty", false, "Allow phase transition with uncommitted files in project directory")
}

func runStartPhase(cmd *cobra.Command, args []string) error {
	phaseName := args[0]

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

	// Read existing STATUS from project directory (version-aware)
	st, err := status.LoadAnyVersion(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read STATUS file: %w (run 'wayfinder-session start' first)", err)
	}

	// Initialize history logger
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

	// Update phase status
	st.UpdatePhase(phaseName, status.PhaseStatusInProgress, "")
	st.SetCurrentPhase(phaseName)

	// Initialize tracker
	tr, err := tracker.New(st.GetSessionID())
	if err != nil {
		return fmt.Errorf("failed to initialize tracker: %w", err)
	}
	defer tr.Close(context.Background())

	// Publish phase.started event
	if err := tr.StartPhase(phaseName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish phase.started event: %v\n", err)
	}

	// Log phase started to history
	if err := hist.AppendEvent(history.EventTypePhaseStarted, phaseName, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to log phase.started to history: %v\n", err)
	}

	// Write updated STATUS to project directory
	if err := st.WriteTo(projectDir); err != nil {
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	fmt.Printf("✅ Phase %s started\n", phaseName)
	return nil
}
