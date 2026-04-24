// Package devlog provides devlog functionality.
package devlog

import (
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/cliframe"
)

var (
	// commonFlags holds standard CLI flags from cliframe
	commonFlags *cliframe.CommonFlags
)

var rootCmd = &cobra.Command{
	Use:   "devlog",
	Short: "Manage devlog workspaces with bare repos and worktrees",
	Long: `devlog is a CLI tool for managing development workspaces that use
bare git repositories with multiple worktrees.

It simplifies syncing repos across machines and managing worktrees
for parallel development workflows.`,
}

// Execute runs the root command and returns any error encountered.
// The caller is responsible for handling the error and exiting with
// appropriate exit codes.
func Execute() error {
	return rootCmd.Execute()
}

// GetCommonFlags returns the shared cliframe flags.
func GetCommonFlags() *cliframe.CommonFlags {
	return commonFlags
}

func init() {
	// Add standard cliframe flags
	commonFlags = cliframe.AddStandardFlags(rootCmd)
}
