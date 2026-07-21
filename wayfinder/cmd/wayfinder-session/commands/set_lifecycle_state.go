package commands

import (
	"fmt"
	"slices"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// setLifecycleStateCmd represents the set-lifecycle-state command
var setLifecycleStateCmd = &cobra.Command{
	Use:   "set-lifecycle-state <state>",
	Short: "Set the lifecycle state for observability",
	Long: `Update lifecycle_state field in WAYFINDER-STATUS.md for enhanced swarm observability.

Lifecycle states (A2A-compatible 7-state model):
  working            - Agent actively executing (default)
  input-required     - Blocked on user input (AskUserQuestion)
  dependency-blocked - Waiting for another agent/task
  validating         - Running BUILD validation or quality gates
  completed          - Task successfully finished
  failed             - Error encountered, cannot proceed
  canceled           - Task abandoned or superseded

Optional flags:
  --blocked-on       - Agent/task ID causing block (for input-required, dependency-blocked)
  --error-message    - Error details (for failed state)
  --input-needed     - Description of needed input (for input-required state)

Examples:
  wayfinder session set-lifecycle-state working
  wayfinder session set-lifecycle-state input-required --input-needed "Design decision needed"
  wayfinder session set-lifecycle-state dependency-blocked --blocked-on oss-vnfl
  wayfinder session set-lifecycle-state failed --error-message "Compilation failed: undefined variable"
  wayfinder session set-lifecycle-state completed
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := GetProjectDirectory()
		lifecycleState := args[0]
		blockedOn, _ := cmd.Flags().GetString("blocked-on")
		errorMessage, _ := cmd.Flags().GetString("error-message")
		inputNeeded, _ := cmd.Flags().GetString("input-needed")

		// Validate lifecycle state
		validStates := []string{
			status.LifecycleWorking,
			status.LifecycleInputRequired,
			status.LifecycleDependencyBlocked,
			status.LifecycleValidating,
			status.LifecycleCompleted,
			status.LifecycleFailed,
			status.LifecycleCanceled,
		}

		if !slices.Contains(validStates, lifecycleState) {
			return fmt.Errorf("invalid lifecycle state: %s (valid: working, input-required, dependency-blocked, validating, completed, failed, canceled)", lifecycleState)
		}
		if err := validateLifecycleMetadata(lifecycleState, blockedOn, errorMessage, inputNeeded); err != nil {
			return err
		}

		st, err := status.ParseV2FromDir(projectDir)
		if err != nil {
			return fmt.Errorf("failed to read status: %w", err)
		}

		// Validate state transition
		if st.LifecycleState != "" {
			if err := validateTransition(st.LifecycleState, lifecycleState); err != nil {
				return fmt.Errorf("invalid state transition: %w", err)
			}
		}
		if err := validateLifecycleCompletion(st, lifecycleState); err != nil {
			return err
		}

		applyLifecycleState(st, lifecycleState, blockedOn, errorMessage, inputNeeded, time.Now())
		if err := status.ValidateV2(st); err != nil {
			return fmt.Errorf("invalid lifecycle update: %w", err)
		}

		// Write updated status
		if err := st.WriteTo(projectDir); err != nil {
			return fmt.Errorf("failed to write status: %w", err)
		}

		fmt.Printf("Lifecycle state updated: %s\n", lifecycleState)
		if blockedOn != "" {
			fmt.Printf("  Blocked on: %s\n", blockedOn)
		}
		if errorMessage != "" {
			fmt.Printf("  Error: %s\n", errorMessage)
		}
		if inputNeeded != "" {
			fmt.Printf("  Input needed: %s\n", inputNeeded)
		}

		return nil
	},
}

func validateLifecycleCompletion(st *status.StatusV2, lifecycleState string) error {
	if lifecycleState != status.LifecycleCompleted {
		return nil
	}
	return status.ValidateSessionCompletion(st)
}

func applyLifecycleState(st *status.StatusV2, lifecycleState, blockedOn, errorMessage, inputNeeded string, now time.Time) {
	st.LifecycleState = lifecycleState
	st.BlockedOn = blockedOn
	st.ErrorMessage = errorMessage
	st.InputNeeded = inputNeeded
	st.BlockedReason = ""
	st.CompletionDate = nil
	st.UpdatedAt = now

	switch lifecycleState {
	case status.LifecycleWorking, status.LifecycleValidating:
		st.Status = status.StatusV2InProgress
	case status.LifecycleInputRequired:
		st.Status = status.StatusV2Blocked
		st.BlockedReason = inputNeeded
	case status.LifecycleDependencyBlocked:
		st.Status = status.StatusV2Blocked
		st.BlockedReason = "blocked on " + blockedOn
	case status.LifecycleFailed:
		st.Status = status.StatusV2Blocked
		st.BlockedReason = errorMessage
	case status.LifecycleCompleted:
		st.Status = status.StatusV2Completed
		st.CompletionDate = &now
	case status.LifecycleCanceled:
		st.Status = status.StatusV2Abandoned
	}

	if lifecycleState == status.LifecycleWorking || lifecycleState == status.LifecycleCompleted || lifecycleState == status.LifecycleCanceled {
		st.BlockedOn = ""
		st.ErrorMessage = ""
		st.InputNeeded = ""
	}
}

func validateLifecycleMetadata(state, blockedOn, errorMessage, inputNeeded string) error {
	switch state {
	case status.LifecycleInputRequired:
		if inputNeeded == "" {
			return fmt.Errorf("input-required requires --input-needed")
		}
	case status.LifecycleDependencyBlocked:
		if blockedOn == "" {
			return fmt.Errorf("dependency-blocked requires --blocked-on")
		}
	case status.LifecycleFailed:
		if errorMessage == "" {
			return fmt.Errorf("failed requires --error-message")
		}
	}
	return nil
}

// GetLifecycleStateCmd returns the lifecycle-state command for registration.
func GetLifecycleStateCmd() *cobra.Command {
	return setLifecycleStateCmd
}

// validateTransition checks if a lifecycle state transition is valid
func validateTransition(from, to string) error {
	// Terminal states cannot transition (except manual intervention)
	if from == status.LifecycleCompleted || from == status.LifecycleCanceled {
		return fmt.Errorf("cannot transition from terminal state %s to %s (requires manual restart)", from, to)
	}

	// Failed can only go to working (with fix) or canceled
	if from == status.LifecycleFailed && to != status.LifecycleWorking && to != status.LifecycleCanceled {
		return fmt.Errorf("failed state can only transition to working (after fix) or canceled, not %s", to)
	}

	// Conflicting blocking states
	if (from == status.LifecycleInputRequired && to == status.LifecycleDependencyBlocked) ||
		(from == status.LifecycleDependencyBlocked && to == status.LifecycleInputRequired) {
		return fmt.Errorf("cannot transition between conflicting blocking states: %s → %s", from, to)
	}

	// Validating cannot pause for blocking states
	if from == status.LifecycleValidating && (to == status.LifecycleInputRequired || to == status.LifecycleDependencyBlocked) {
		return fmt.Errorf("validation cannot transition to blocking state %s", to)
	}

	// All other transitions are valid (working is hub state)
	return nil
}

func init() {
	setLifecycleStateCmd.Flags().String("blocked-on", "", "Agent/task ID causing block")
	setLifecycleStateCmd.Flags().String("error-message", "", "Error details (for failed state)")
	setLifecycleStateCmd.Flags().String("input-needed", "", "Description of needed input (for input-required state)")
}
