package commands

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/progress"
)

var startForce bool

var StartCmd = &cobra.Command{
	Use:   "start <feature-id>",
	Short: "Start working on a feature",
	Long: `Mark a feature as in_progress.

This command:
1. Validates the feature ID exists in the S7 plan
2. Checks that no other feature is currently in_progress
3. Updates status from 'failing' to 'in_progress'
4. Sets started_at timestamp

Example:
  wayfinder-features start auth-login
  wayfinder-features start auth-login --force  # Force even if already passing`,
	Args: cobra.ExactArgs(1),
	RunE: runStart,
}

func init() {
	StartCmd.Flags().BoolVar(&startForce, "force", false, "Start even if feature already passing")
}

func runStart(cmd *cobra.Command, args []string) error {
	featureID := args[0]
	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())

	// Find and read progress
	progressPath, err := progress.FindProgressFile()
	if err != nil {
		return fmt.Errorf("failed to find progress file: %w", err)
	}

	prog, err := progress.ReadProgress(progressPath)
	if err != nil {
		return fmt.Errorf("failed to read progress: %w", err)
	}

	// Find the feature
	feature, err := progress.FindFeature(prog, featureID)
	if err != nil {
		writer.Error(err.Error())
		fmt.Fprintf(cmd.OutOrStdout(), "\nAvailable features:\n")
		for _, f := range prog.Features {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s)\n", f.ID, f.Status)
		}
		return err
	}

	// Check if already passing
	if feature.Status == progress.StatusPassing && !startForce {
		writer.Error(fmt.Sprintf("Feature '%s' is already verified (passing)", featureID))
		writer.Info("Use --force to restart this feature")
		return fmt.Errorf("feature already passing")
	}

	// Check if another feature is in_progress
	for _, f := range prog.Features {
		if f.ID != featureID && f.Status == progress.StatusInProgress {
			writer.Error(fmt.Sprintf("Another feature is in_progress: %s", f.ID))
			writer.Info(fmt.Sprintf("Complete '%s' first or use: wayfinder-features verify %s", f.ID, f.ID))
			return fmt.Errorf("another feature in progress")
		}
	}

	// Update feature status
	now := time.Now()
	err = progress.UpdateFeature(prog, featureID, func(f *progress.Feature) {
		f.Status = progress.StatusInProgress
		f.StartedAt = &now
		// Reset verified_at if restarting
		if startForce {
			f.VerifiedAt = nil
		}
	})
	if err != nil {
		return fmt.Errorf("failed to update feature: %w", err)
	}

	// Write updated progress
	if err := progress.WriteProgress(progressPath, prog); err != nil {
		return fmt.Errorf("failed to update progress: %w", err)
	}

	writer.Success(fmt.Sprintf("Started: %s", featureID))
	fmt.Fprintf(cmd.OutOrStdout(), "Status: in_progress\n")
	fmt.Fprintf(cmd.OutOrStdout(), "\nNext steps:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  1. Implement the feature\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  2. Run tests to verify\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  3. wayfinder-features verify %s\n", featureID)

	return nil
}
