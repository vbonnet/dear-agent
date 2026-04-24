package cmd

import (
	"github.com/spf13/cobra"
	featurescmd "github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-features/commands"
)

var featuresCmd = &cobra.Command{
	Use:   "features",
	Short: "Track feature-level progress in S5 implementation",
	Long: `Track feature-level progress in S5 (Implementation) waypoint.

This tool helps manage multi-session implementation projects by tracking
which features are complete and verified. It integrates with the S7 plan
Feature Tracking table and maintains runtime progress in progress.json.

Commands:
  init                      Create progress.json from S7 plan
  status                    View feature progress
  start <feature>           Start working on a feature
  verify <feature>          Verify feature is complete

Example workflow:
  1. Define features in S7 plan Feature Tracking table
  2. wayfinder features init          # Create progress.json from S7 plan
  3. wayfinder features status        # View progress
  4. wayfinder features start auth-login
  5. wayfinder features verify auth-login
  6. wayfinder features status        # Shows auth-login as passing`,
}

func init() {
	// Add features subcommands from existing package
	featuresCmd.AddCommand(featurescmd.InitCmd)
	featuresCmd.AddCommand(featurescmd.StatusCmd)
	featuresCmd.AddCommand(featurescmd.StartCmd)
	featuresCmd.AddCommand(featurescmd.VerifyCmd)

	// Add features to root
	rootCmd.AddCommand(featuresCmd)
}
