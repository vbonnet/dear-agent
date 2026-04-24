package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/taskmanager"
)

var (
	taskListPhase  string
	taskListStatus string
)

// TaskListCmd lists tasks with optional filtering
var TaskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks with optional filtering",
	Long: `List all tasks in the roadmap with optional filtering by phase or status.

Examples:
  wayfinder session task list
  wayfinder session task list --phase S8
  wayfinder session task list --status pending
  wayfinder session task list --phase S8 --status in-progress`,
	RunE: runTaskList,
}

func init() {
	TaskListCmd.Flags().StringVar(&taskListPhase, "phase", "", "Filter by phase ID (W0, D1, D2, D3, D4, S6, S7, S8, S11)")
	TaskListCmd.Flags().StringVar(&taskListStatus, "status", "", "Filter by status (pending, in-progress, completed, blocked)")
}

func runTaskList(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()

	tm := taskmanager.New(projectDir)

	filter := &taskmanager.TaskFilter{
		PhaseID: taskListPhase,
		Status:  taskListStatus,
	}

	tasks, err := tm.ListTasks(filter)
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	// Print header
	fmt.Println("Tasks:")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-10s %-8s %-12s %-8s %-50s\n", "ID", "Phase", "Status", "Priority", "Title")
	fmt.Println(strings.Repeat("-", 100))

	// Print tasks
	for _, t := range tasks {
		priority := t.Task.Priority
		if priority == "" {
			priority = "-"
		}

		// Truncate title if too long
		title := t.Task.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}

		fmt.Printf("%-10s %-8s %-12s %-8s %-50s\n",
			t.Task.ID,
			t.PhaseID,
			t.Task.Status,
			priority,
			title,
		)

		// Show dependencies if any
		if len(t.Task.DependsOn) > 0 {
			fmt.Printf("           └─ Depends on: %v\n", t.Task.DependsOn)
		}
	}

	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("Total: %d tasks\n", len(tasks))

	return nil
}
