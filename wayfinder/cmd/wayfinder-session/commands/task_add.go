package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/taskmanager"
)

var (
	taskAddEffort             float64
	taskAddDescription        string
	taskAddPriority           string
	taskAddDependsOn          []string
	taskAddDeliverables       []string
	taskAddAcceptanceCriteria []string
	taskAddAssignedTo         string
	taskAddBeadID             string
	taskAddNotes              string
)

// TaskAddCmd adds a new task to a phase
var TaskAddCmd = &cobra.Command{
	Use:   "add <phase-id> <title>",
	Short: "Add a new task to a phase",
	Long: `Add a new task to the specified phase in WAYFINDER-STATUS.md.

The task will be created with a unique ID (e.g., S8-1, S8-2) and pending status.

Examples:
  wayfinder session task add S8 "Implement OAuth2 authorization endpoint" --effort 4
  wayfinder session task add S8 "Create token endpoint" --effort 5 --priority P0 --depends-on S8-1
  wayfinder session task add S7 "Break down implementation tasks" --effort 2`,
	Args: cobra.ExactArgs(2),
	RunE: runTaskAdd,
}

func init() {
	TaskAddCmd.Flags().Float64Var(&taskAddEffort, "effort", 0, "Estimated effort in days")
	TaskAddCmd.Flags().StringVar(&taskAddDescription, "description", "", "Detailed task description")
	TaskAddCmd.Flags().StringVar(&taskAddPriority, "priority", "", "Priority level (P0, P1, P2)")
	TaskAddCmd.Flags().StringSliceVar(&taskAddDependsOn, "depends-on", []string{}, "Task IDs this task depends on")
	TaskAddCmd.Flags().StringSliceVar(&taskAddDeliverables, "deliverables", []string{}, "Files/artifacts to be created")
	TaskAddCmd.Flags().StringSliceVar(&taskAddAcceptanceCriteria, "acceptance-criteria", []string{}, "How to validate completion")
	TaskAddCmd.Flags().StringVar(&taskAddAssignedTo, "assigned-to", "", "Agent or developer name")
	TaskAddCmd.Flags().StringVar(&taskAddBeadID, "bead-id", "", "Associated bead ID")
	TaskAddCmd.Flags().StringVar(&taskAddNotes, "notes", "", "Task-specific notes")
}

func runTaskAdd(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()
	phaseID := args[0]
	title := args[1]

	tm := taskmanager.New(projectDir)

	opts := &taskmanager.TaskOptions{
		EffortDays:         taskAddEffort,
		Description:        taskAddDescription,
		Priority:           taskAddPriority,
		DependsOn:          taskAddDependsOn,
		Deliverables:       taskAddDeliverables,
		AcceptanceCriteria: taskAddAcceptanceCriteria,
		AssignedTo:         taskAddAssignedTo,
		BeadID:             taskAddBeadID,
		Notes:              taskAddNotes,
	}

	task, err := tm.AddTask(phaseID, title, opts)
	if err != nil {
		return fmt.Errorf("failed to add task: %w", err)
	}

	// Print success message
	fmt.Printf("Task created successfully: %s\n\n", task.ID)
	fmt.Printf("ID:          %s\n", task.ID)
	fmt.Printf("Title:       %s\n", task.Title)
	fmt.Printf("Status:      %s\n", task.Status)
	if task.EffortDays > 0 {
		fmt.Printf("Effort:      %.1f days\n", task.EffortDays)
	}
	if task.Priority != "" {
		fmt.Printf("Priority:    %s\n", task.Priority)
	}
	if len(task.DependsOn) > 0 {
		fmt.Printf("Depends on:  %v\n", task.DependsOn)
	}

	return nil
}
