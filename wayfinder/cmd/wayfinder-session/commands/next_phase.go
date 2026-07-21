package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// NextPhaseCmd prints the next phase in the Wayfinder sequence.
var NextPhaseCmd = &cobra.Command{
	Use:   "next-phase",
	Short: "Get the next phase in the sequence",
	Long: `Read WAYFINDER-STATUS.md and output the next phase.

Works with the canonical sequence (CHARTER through RETRO).
Returns the current phase if it is not yet completed.
Returns an error if already at the final phase.

Example:
  wayfinder session next-phase
  wayfinder -C wf/my-project session next-phase`,
	Args: cobra.NoArgs,
	RunE: runNextPhase,
}

func runNextPhase(cmd *cobra.Command, args []string) error {
	return runNextPhaseInDir(GetProjectDirectory())
}

func runNextPhaseInDir(projectDir string) error {
	st, err := status.ParseV2FromDir(projectDir)
	if err != nil {
		return fmt.Errorf("failed to parse canonical STATUS: %w (run 'wayfinder session start' or use -C <project-dir>)", err)
	}
	nextPhase, err := st.NextPhase()
	if err != nil {
		return fmt.Errorf("failed to get next phase: %w", err)
	}

	fmt.Println(nextPhase)
	return nil
}
