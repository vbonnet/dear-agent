package commands

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/progress"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/internal/s7"
)

var initForce bool

var InitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize feature tracking from S7 plan",
	Long: `Create S5-implementation/progress.json from the Feature Tracking table in your S7 plan.

This command:
1. Finds your S7 plan (S7-plan.md or plan.md)
2. Parses the Feature Tracking table
3. Creates S5-implementation/progress.json with all features in 'failing' status

Example:
  wayfinder-features init
  wayfinder-features init --force  # Overwrite existing progress.json`,
	RunE: runInit,
}

func init() {
	InitCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing progress.json")
}

func runInit(cmd *cobra.Command, args []string) error {
	writer := cliframe.NewWriter(cmd.OutOrStdout(), cmd.ErrOrStderr())

	// Find S7 plan
	planPath, err := s7.FindS7Plan()
	if err != nil {
		return fmt.Errorf("failed to find S7 plan: %w", err)
	}

	writer.Info(fmt.Sprintf("Found S7 plan: %s", planPath))

	// Parse S7 plan
	projectName, features, err := s7.ParseS7Plan(planPath)
	if err != nil {
		return fmt.Errorf("failed to parse S7 plan: %w", err)
	}

	writer.Info(fmt.Sprintf("Found %d features from S7 plan", len(features)))

	// Convert to progress features
	var progFeatures []progress.Feature
	for _, f := range features {
		progFeatures = append(progFeatures, progress.Feature{
			ID:     f.ID,
			Status: progress.StatusFailing,
		})
	}

	// Create progress
	prog := progress.NewProgress(projectName, progFeatures)

	// Determine output path
	progressPath := filepath.Join(filepath.Dir(planPath), progress.DefaultProgressFile)

	// Check if already exists
	if _, err := progress.ReadProgress(progressPath); err == nil && !initForce {
		writer.Error(fmt.Sprintf("progress.json already exists at %s", progressPath))
		writer.Info("Use --force to overwrite")
		return fmt.Errorf("progress file already exists")
	}

	// Write progress file
	if err := progress.WriteProgress(progressPath, prog); err != nil {
		return fmt.Errorf("failed to write progress file: %w", err)
	}

	writer.Success("Initialized S5 feature tracking")
	fmt.Fprintf(cmd.OutOrStdout(), "Created: %s\n", progressPath)
	fmt.Fprintf(cmd.OutOrStdout(), "\nNext steps:\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  wayfinder-features status        # View progress\n")
	fmt.Fprintf(cmd.OutOrStdout(), "  wayfinder-features start <id>    # Start a feature\n")

	return nil
}
