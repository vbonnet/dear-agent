package commands

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/taskmanager"
)

var (
	taskUpdateStatus             string
	taskUpdateTitle              string
	taskUpdateDescription        string
	taskUpdateEffort             float64
	taskUpdatePriority           string
	taskUpdateTestsStatus        string
	taskUpdateAssignedTo         string
	taskUpdateNotes              string
	taskUpdateBeadID             string
	taskUpdateDeliverables       []string
	taskUpdateAcceptanceCriteria []string
	taskUpdateDependsOn          []string
	taskUpdateVerifyCommand      string
	taskUpdateVerifyExpected     string
)

// TaskUpdateCmd updates an existing task
var TaskUpdateCmd = &cobra.Command{
	Use:   "update <task-id>",
	Short: "Update an existing task",
	Long: `Update an existing task's status, metadata, or other fields.

Examples:
  wayfinder session task update S8-1 --status in-progress
  wayfinder session task update S8-1 --status completed --tests-status passed
  wayfinder session task update S8-2 --priority P0 --effort 6
  wayfinder session task update S8-3 --assigned-to "claude"`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskUpdate,
}

func init() {
	TaskUpdateCmd.Flags().StringVar(&taskUpdateStatus, "status", "", "Task status (pending, in-progress, completed, blocked)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateTitle, "title", "", "Task title")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateDescription, "description", "", "Task description")
	TaskUpdateCmd.Flags().Float64Var(&taskUpdateEffort, "effort", 0, "Estimated effort in days")
	TaskUpdateCmd.Flags().StringVar(&taskUpdatePriority, "priority", "", "Priority level (P0, P1, P2)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateTestsStatus, "tests-status", "", "Test status (passed, failed, pending)")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateAssignedTo, "assigned-to", "", "Agent or developer name")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateNotes, "notes", "", "Task-specific notes")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateBeadID, "bead-id", "", "Associated bead ID")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateDeliverables, "deliverables", []string{}, "Files/artifacts created")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateAcceptanceCriteria, "acceptance-criteria", []string{}, "How to validate completion")
	TaskUpdateCmd.Flags().StringSliceVar(&taskUpdateDependsOn, "depends-on", []string{}, "Task IDs this task depends on")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateVerifyCommand, "verify-command", "", "Command to verify task completion (e.g., 'go test ./auth/...')")
	TaskUpdateCmd.Flags().StringVar(&taskUpdateVerifyExpected, "verify-expected", "", "Expected verification result (e.g., 'exit code 0, all tests pass')")
}

func runTaskUpdate(cmd *cobra.Command, args []string) error {
	projectDir := GetProjectDirectory()
	taskID := args[0]

	tm := taskmanager.New(projectDir)

	opts := &taskmanager.UpdateOptions{
		Status:             taskUpdateStatus,
		Title:              taskUpdateTitle,
		Description:        taskUpdateDescription,
		EffortDays:         taskUpdateEffort,
		Priority:           taskUpdatePriority,
		TestsStatus:        taskUpdateTestsStatus,
		AssignedTo:         taskUpdateAssignedTo,
		Notes:              taskUpdateNotes,
		BeadID:             taskUpdateBeadID,
		Deliverables:       taskUpdateDeliverables,
		AcceptanceCriteria: taskUpdateAcceptanceCriteria,
		DependsOn:          taskUpdateDependsOn,
		VerifyCommand:      taskUpdateVerifyCommand,
		VerifyExpected:     taskUpdateVerifyExpected,
	}

	task, err := tm.UpdateTask(taskID, opts)
	if err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	// Print success message
	fmt.Printf("Task updated successfully: %s\n\n", task.ID)
	fmt.Printf("ID:          %s\n", task.ID)
	fmt.Printf("Title:       %s\n", task.Title)
	fmt.Printf("Status:      %s\n", task.Status)
	if task.EffortDays > 0 {
		fmt.Printf("Effort:      %.1f days\n", task.EffortDays)
	}
	if task.Priority != "" {
		fmt.Printf("Priority:    %s\n", task.Priority)
	}
	if task.TestsStatus != nil {
		fmt.Printf("Tests:       %s\n", *task.TestsStatus)
	}
	if task.VerifyCommand != "" {
		fmt.Printf("Verify:      %s\n", task.VerifyCommand)
	}

	return nil
}
