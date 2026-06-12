package commands

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// NextPhaseCmd prints the next phase in the Wayfinder sequence.
var NextPhaseCmd = &cobra.Command{
	Use:   "next-phase",
	Short: "Get the next phase in the sequence",
	Long: `Read WAYFINDER-STATUS.md and output the next phase.

Works for both V1 (W0/D1-D4/S4-S11) and V2 (CHARTER/PROBLEM/…/RETRO) files.
Returns the current phase if it is not yet completed.
Returns an error if already at the final phase.

Example:
  wayfinder session next-phase
  wayfinder -C wf/my-project session next-phase`,
	Args: cobra.NoArgs,
	RunE: runNextPhase,
}

func runNextPhase(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()

	statusPath := filepath.Join(projectDir, status.StatusFilename)
	schemaVersion, err := status.DetectSchemaVersion(statusPath)
	if err != nil {
		return fmt.Errorf("failed to read STATUS file: %w (run 'wayfinder session start' or use -C <project-dir>)", err)
	}

	var nextPhase string
	if schemaVersion == status.SchemaVersionV2 {
		st, err := status.ParseV2FromDir(projectDir)
		if err != nil {
			return fmt.Errorf("failed to parse V2 STATUS: %w", err)
		}
		nextPhase, err = st.NextPhase()
		if err != nil {
			return fmt.Errorf("failed to get next phase: %w", err)
		}
	} else {
		// V1 (legacy 12-ID model)
		st, err := status.ReadFrom(projectDir)
		if err != nil {
			return fmt.Errorf("failed to parse V1 STATUS: %w", err)
		}
		nextPhase, err = st.NextPhase()
		if err != nil {
			return fmt.Errorf("failed to get next phase: %w", err)
		}
	}

	fmt.Println(nextPhase)
	return nil
}
