package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var (
	taskBacklogPath string
	taskStatus      string
	taskNote        string
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage orchestrator task backlog",
	Long: `Manage tasks in a backend-agnostic task backlog for orchestrator loops.

Tasks are stored in a markdown file (.agm/backlog.md by default) with
status tracking and completion timestamps. Use this to coordinate work
across parallel worker sessions.

Subcommands:
  agm task next      - Get the highest-priority unstarted task
  agm task list      - List all tasks
  agm task complete  - Mark a task as done
  agm task update    - Update task status`,
	RunE: groupRunE,
}

var taskNextCmd = &cobra.Command{
	Use:   "next",
	Short: "Get the next unstarted task",
	Long: `Return the first task with status != "done" as JSON.
Useful for orchestrator loops to fetch the next item of work.

Output format:
  {
    "task": {
      "id": "1",
      "priority": "P0",
      "status": "queued",
      "description": "Implement feature X",
      "completed": null
    }
  }

Returns null task if all tasks are done.`,
	RunE: runTaskNext,
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tasks",
	Long: `List all tasks from the backlog, optionally filtered by status.

By default returns all tasks as JSON. Use --status to filter:
  agm task list --status=queued
  agm task list --status=in-progress
  agm task list --status=blocked
  agm task list --status=done`,
	RunE: runTaskList,
}

var taskCompleteCmd = &cobra.Command{
	Use:   "complete <id>",
	Short: "Mark a task as done",
	Long: `Mark a task as complete and record the completion timestamp.

Example:
  agm task complete 1
  agm task complete 2 -C /path/to/workspace`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskComplete,
}

var taskUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update task status",
	Long: `Update a task's status and optionally add notes.

Valid statuses: queued, in-progress, blocked, done

Example:
  agm task update 1 --status in-progress
  agm task update 2 --status blocked --note "Waiting for API response"`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskUpdate,
}

func init() {
	// Register task command group
	rootCmd.AddCommand(taskCmd)

	// Register subcommands
	taskCmd.AddCommand(taskNextCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskCompleteCmd)
	taskCmd.AddCommand(taskUpdateCmd)

	// Add flags for backlog path (available on all subcommands)
	taskNextCmd.Flags().StringVar(&taskBacklogPath, "backlog", "", "Path to backlog markdown file (default: .agm/backlog.md)")
	taskListCmd.Flags().StringVar(&taskBacklogPath, "backlog", "", "Path to backlog markdown file (default: .agm/backlog.md)")
	taskListCmd.Flags().StringVar(&taskStatus, "status", "", "Filter by status: queued, in-progress, blocked, done")
	taskCompleteCmd.Flags().StringVar(&taskBacklogPath, "backlog", "", "Path to backlog markdown file (default: .agm/backlog.md)")
	taskUpdateCmd.Flags().StringVar(&taskBacklogPath, "backlog", "", "Path to backlog markdown file (default: .agm/backlog.md)")
	taskUpdateCmd.Flags().StringVar(&taskStatus, "status", "", "New status: queued, in-progress, blocked, done")
	taskUpdateCmd.Flags().StringVar(&taskNote, "note", "", "Optional note to record with the update")
}

func runTaskNext(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return handleError(err)
	}
	defer cleanup()

	result, err := ops.TaskNext(opCtx, &ops.TaskNextRequest{
		FilePath: taskBacklogPath,
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		if result.Task == nil {
			fmt.Println("No pending tasks")
		} else {
			fmt.Printf("Task #%s [%s] %s (%s)\n",
				result.Task.ID, result.Task.Priority, result.Task.Description, result.Task.Status)
		}
	})
}

func runTaskList(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return handleError(err)
	}
	defer cleanup()

	result, err := ops.ListTasks(opCtx, &ops.TaskListRequest{
		FilePath: taskBacklogPath,
		Status:   taskStatus,
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		if len(result.Tasks) == 0 {
			fmt.Println("No tasks")
			return
		}
		fmt.Printf("Tasks (%d):\n", result.Total)
		for _, task := range result.Tasks {
			status := task.Status
			if status == "done" {
				status = "✓ done"
			}
			fmt.Printf("  #%s [%s] %s (%s)\n", task.ID, task.Priority, task.Description, status)
		}
	})
}

func runTaskComplete(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return handleError(err)
	}
	defer cleanup()

	result, err := ops.TaskComplete(opCtx, &ops.TaskCompleteRequest{
		ID:       args[0],
		FilePath: taskBacklogPath,
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		fmt.Printf("✓ Task #%s marked as done (%s)\n", result.Task.ID, result.Task.Completed.Format("2006-01-02"))
	})
}

func runTaskUpdate(cmd *cobra.Command, args []string) error {
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return handleError(err)
	}
	defer cleanup()

	result, err := ops.TaskUpdate(opCtx, &ops.TaskUpdateRequest{
		ID:       args[0],
		Status:   taskStatus,
		Note:     taskNote,
		FilePath: taskBacklogPath,
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		fmt.Printf("Task #%s updated: %s (%s)\n", result.Task.ID, result.Task.Description, result.Task.Status)
	})
}
