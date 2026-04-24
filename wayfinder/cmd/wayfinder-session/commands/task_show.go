package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/taskmanager"
)

// TaskShowCmd displays detailed information about a task
var TaskShowCmd = &cobra.Command{
	Use:   "show <task-id>",
	Short: "Show detailed information about a task",
	Long: `Display detailed information about a specific task including all metadata,
dependencies, deliverables, and acceptance criteria.

Examples:
  wayfinder session task show S8-1
  wayfinder session task show D4-2`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskShow,
}

func runTaskShow(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()
	taskID := args[0]

	tm := taskmanager.New(projectDir)

	task, err := tm.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Print task details
	fmt.Printf("Task: %s\n", task.ID)
	fmt.Println("================================================================================")
	fmt.Printf("\n")
	fmt.Printf("Title:       %s\n", task.Title)
	fmt.Printf("Status:      %s\n", task.Status)

	if task.Description != "" {
		fmt.Printf("\nDescription:\n%s\n", task.Description)
	}

	fmt.Printf("\nMetadata:\n")
	if task.EffortDays > 0 {
		fmt.Printf("  Effort:         %.1f days\n", task.EffortDays)
	}
	if task.Priority != "" {
		fmt.Printf("  Priority:       %s\n", task.Priority)
	}
	if task.AssignedTo != "" {
		fmt.Printf("  Assigned to:    %s\n", task.AssignedTo)
	}
	if task.BeadID != "" {
		fmt.Printf("  Bead ID:        %s\n", task.BeadID)
	}
	if task.TestsStatus != nil {
		fmt.Printf("  Tests status:   %s\n", *task.TestsStatus)
	}

	if len(task.DependsOn) > 0 {
		fmt.Printf("\nDependencies:\n")
		for _, dep := range task.DependsOn {
			fmt.Printf("  - %s\n", dep)
		}
	}

	if len(task.Deliverables) > 0 {
		fmt.Printf("\nDeliverables:\n")
		for _, d := range task.Deliverables {
			fmt.Printf("  - %s\n", d)
		}
	}

	if len(task.AcceptanceCriteria) > 0 {
		fmt.Printf("\nAcceptance Criteria:\n")
		for _, ac := range task.AcceptanceCriteria {
			fmt.Printf("  - %s\n", ac)
		}
	}

	if task.StartedAt != nil {
		fmt.Printf("\nStarted at:  %s\n", task.StartedAt.Format("2006-01-02 15:04:05"))
	}
	if task.CompletedAt != nil {
		fmt.Printf("Completed at: %s\n", task.CompletedAt.Format("2006-01-02 15:04:05"))
	}

	if task.Notes != "" {
		fmt.Printf("\nNotes:\n%s\n", task.Notes)
	}

	return nil
}
