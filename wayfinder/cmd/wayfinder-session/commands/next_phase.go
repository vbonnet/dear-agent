package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// NextPhaseCmd is the cobra command that prints the next phase in the V2 sequence.
var NextPhaseCmd = &cobra.Command{
	Use:   "next-phase",
	Short: "Get the next phase in the V2 sequence",
	Long: `Read WAYFINDER-STATUS.md and output the next phase in the 9-phase sequence.

V2 Phase Sequence:
  W0  → D1 → D2 → D3 → D4 → S6 → S7 → S8 → S11

Returns current phase if not yet completed.
Returns error if already at final phase (S11).

Example:
  wayfinder-session next-phase`,
	Args: cobra.NoArgs,
	RunE: runNextPhase,
}

func runNextPhase(cmd *cobra.Command, args []string) error {
	// Get project directory
	projectDir := GetProjectDirectory()

	// Try to read V2 status file
	st, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read V2 STATUS file: %w (run 'wayfinder-session start' first)", err)
	}

	// Get next phase using V2 navigation
	nextPhase, err := st.NextPhase()
	if err != nil {
		return fmt.Errorf("failed to get next phase: %w", err)
	}

	// Output just the phase name (for easy parsing)
	fmt.Println(nextPhase)
	return nil
}
