package commands

import (
	"github.com/spf13/cobra"
)

// TaskCmd is the parent command for task operations
var TaskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks in the roadmap",
	Long: `Manage tasks in the WAYFINDER-STATUS.md roadmap.

Task management commands allow you to:
  - Add new tasks to phases
  - Update task status and metadata
  - List tasks with filtering
  - Show detailed task information
  - Delete tasks

Examples:
  wayfinder session task add BUILD "Implement OAuth2 endpoint" --effort 4
  wayfinder session task update BUILD-1 --status in-progress
  wayfinder session task list --phase BUILD
  wayfinder session task show BUILD-1
  wayfinder session task delete BUILD-1`,
}

func init() {
	// Add task subcommands
	TaskCmd.AddCommand(TaskAddCmd)
	TaskCmd.AddCommand(TaskUpdateCmd)
	TaskCmd.AddCommand(TaskListCmd)
	TaskCmd.AddCommand(TaskShowCmd)
	TaskCmd.AddCommand(TaskDeleteCmd)
}
