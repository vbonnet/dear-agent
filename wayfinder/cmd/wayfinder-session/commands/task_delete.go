package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/taskmanager"
)

// TaskDeleteCmd deletes a task from the roadmap
var TaskDeleteCmd = &cobra.Command{
	Use:   "delete <task-id>",
	Short: "Delete a task from the roadmap",
	Long: `Delete a task from the roadmap. This will fail if other tasks depend on it.

Examples:
  wayfinder session task delete S8-1
  wayfinder session task delete D4-2`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskDelete,
}

func runTaskDelete(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()
	taskID := args[0]

	tm := taskmanager.New(projectDir)

	if err := tm.DeleteTask(taskID); err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	fmt.Printf("Task deleted successfully: %s\n", taskID)

	return nil
}
