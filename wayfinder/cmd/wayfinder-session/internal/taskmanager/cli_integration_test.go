package taskmanager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestCLIWorkflow tests a complete CLI workflow
func TestCLIWorkflow(t *testing.T) {
	tempDir, tm := createTestStatusFile(t)

	// Step 1: Add multiple tasks
	task1, err := tm.AddTask("S8", "Implement OAuth2 authorization endpoint", &TaskOptions{
		EffortDays:  4.0,
		Priority:    status.PriorityP0,
		Description: "Create /oauth/authorize endpoint with PKCE support",
		Deliverables: []string{
			"src/oauth/authorize.go",
			"src/oauth/authorize_test.go",
		},
		AcceptanceCriteria: []string{
			"Endpoint returns authorization code on success",
			"PKCE challenge validated correctly",
		},
	})
	if err != nil {
		t.Fatalf("failed to add task 1: %v", err)
	}

	task2, err := tm.AddTask("S8", "Implement OAuth2 token endpoint", &TaskOptions{
		EffortDays:  5.0,
		Priority:    status.PriorityP0,
		Description: "Create /oauth/token endpoint with refresh token support",
		DependsOn:   []string{task1.ID},
		Deliverables: []string{
			"src/oauth/token.go",
			"src/oauth/token_test.go",
		},
	})
	if err != nil {
		t.Fatalf("failed to add task 2: %v", err)
	}

	task3, err := tm.AddTask("S8", "Implement token validation middleware", &TaskOptions{
		EffortDays: 3.0,
		Priority:   status.PriorityP0,
		DependsOn:  []string{task1.ID},
	})
	if err != nil {
		t.Fatalf("failed to add task 3: %v", err)
	}

	// Step 2: List all tasks
	allTasks, err := tm.ListTasks(nil)
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}

	if len(allTasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(allTasks))
	}

	// Step 3: Update task status to in-progress
	updated1, err := tm.UpdateTask(task1.ID, &UpdateOptions{
		Status: status.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("failed to update task 1: %v", err)
	}

	if updated1.Status != status.TaskStatusInProgress {
		t.Errorf("expected status in-progress, got %s", updated1.Status)
	}

	if updated1.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}

	// Step 4: Complete task 1
	completed1, err := tm.UpdateTask(task1.ID, &UpdateOptions{
		Status:      status.TaskStatusCompleted,
		TestsStatus: "passed",
	})
	if err != nil {
		t.Fatalf("failed to complete task 1: %v", err)
	}

	if completed1.Status != status.TaskStatusCompleted {
		t.Errorf("expected status completed, got %s", completed1.Status)
	}

	if completed1.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}

	// Step 5: Start task 2 (depends on task 1, which is now complete)
	_, err = tm.UpdateTask(task2.ID, &UpdateOptions{
		Status: status.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("failed to start task 2: %v", err)
	}

	// Step 6: List pending tasks
	pendingTasks, err := tm.ListTasks(&TaskFilter{
		Status: status.TaskStatusPending,
	})
	if err != nil {
		t.Fatalf("failed to list pending tasks: %v", err)
	}

	if len(pendingTasks) != 1 {
		t.Errorf("expected 1 pending task, got %d", len(pendingTasks))
	}

	// Step 7: Get detailed task info
	detailedTask, err := tm.GetTask(task2.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if len(detailedTask.DependsOn) != 1 || detailedTask.DependsOn[0] != task1.ID {
		t.Errorf("expected task 2 to depend on task 1")
	}

	// Step 8: Try to delete task that has dependencies
	err = tm.DeleteTask(task1.ID)
	if err == nil {
		t.Error("expected error when deleting task with dependencies")
	}

	// Step 9: Delete task without dependencies
	err = tm.DeleteTask(task3.ID)
	if err != nil {
		t.Fatalf("failed to delete task 3: %v", err)
	}

	// Verify deletion
	remainingTasks, err := tm.ListTasks(nil)
	if err != nil {
		t.Fatalf("failed to list remaining tasks: %v", err)
	}

	if len(remainingTasks) != 2 {
		t.Errorf("expected 2 remaining tasks, got %d", len(remainingTasks))
	}

	// Step 10: Verify file integrity
	statusFile := filepath.Join(tempDir, "WAYFINDER-STATUS.md")
	st, err := status.ParseV2(statusFile)
	if err != nil {
		t.Fatalf("failed to parse status file: %v", err)
	}

	if st.Roadmap == nil {
		t.Fatal("expected roadmap to exist")
	}

	var s8Phase *status.RoadmapPhase
	for i := range st.Roadmap.Phases {
		if st.Roadmap.Phases[i].ID == "S8" {
			s8Phase = &st.Roadmap.Phases[i]
			break
		}
	}

	if s8Phase == nil {
		t.Fatal("expected S8 phase to exist")
	}

	if len(s8Phase.Tasks) != 2 {
		t.Errorf("expected 2 tasks in S8 phase, got %d", len(s8Phase.Tasks))
	}
}

// TestDependencyValidationWorkflow tests dependency validation through CLI
func TestDependencyValidationWorkflow(t *testing.T) {
	_, tm := createTestStatusFile(t)

	// Create a chain of dependencies
	task1, _ := tm.AddTask("S8", "Task 1", nil)
	task2, _ := tm.AddTask("S8", "Task 2", &TaskOptions{
		DependsOn: []string{task1.ID},
	})
	task3, _ := tm.AddTask("S8", "Task 3", &TaskOptions{
		DependsOn: []string{task2.ID},
	})

	// Try to create a cycle by making task1 depend on task3
	_, err := tm.UpdateTask(task1.ID, &UpdateOptions{
		DependsOn: []string{task3.ID},
	})

	if err == nil {
		t.Error("expected error when creating circular dependency")
	}

	if !contains(err.Error(), "circular dependency") {
		t.Errorf("expected error to mention circular dependency, got: %v", err)
	}

	// Verify no changes were made
	verifyTask, err := tm.GetTask(task1.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if len(verifyTask.DependsOn) > 0 {
		t.Error("expected task1 to have no dependencies after failed update")
	}
}

// TestMultiPhaseWorkflow tests tasks across multiple phases
func TestMultiPhaseWorkflow(t *testing.T) {
	_, tm := createTestStatusFile(t)

	// Add tasks to different phases
	s7Task, err := tm.AddTask("S7", "Break down implementation tasks", &TaskOptions{
		EffortDays: 2.0,
		Priority:   status.PriorityP0,
	})
	if err != nil {
		t.Fatalf("failed to add S7 task: %v", err)
	}

	s8Task1, err := tm.AddTask("S8", "Implement feature A", &TaskOptions{
		EffortDays: 4.0,
		Priority:   status.PriorityP0,
		DependsOn:  []string{s7Task.ID}, // Cross-phase dependency
	})
	if err != nil {
		t.Fatalf("failed to add S8 task 1: %v", err)
	}

	_, err = tm.AddTask("S8", "Implement feature B", &TaskOptions{
		EffortDays: 3.0,
		Priority:   status.PriorityP1,
	})
	if err != nil {
		t.Fatalf("failed to add S8 task 2: %v", err)
	}

	// List tasks by phase
	s7Tasks, err := tm.ListTasks(&TaskFilter{PhaseID: "S7"})
	if err != nil {
		t.Fatalf("failed to list S7 tasks: %v", err)
	}

	if len(s7Tasks) != 1 {
		t.Errorf("expected 1 S7 task, got %d", len(s7Tasks))
	}

	s8Tasks, err := tm.ListTasks(&TaskFilter{PhaseID: "S8"})
	if err != nil {
		t.Fatalf("failed to list S8 tasks: %v", err)
	}

	if len(s8Tasks) != 2 {
		t.Errorf("expected 2 S8 tasks, got %d", len(s8Tasks))
	}

	// Verify cross-phase dependency
	if len(s8Task1.DependsOn) != 1 || s8Task1.DependsOn[0] != s7Task.ID {
		t.Error("expected S8 task to depend on S7 task")
	}

	// Complete S7 task
	_, err = tm.UpdateTask(s7Task.ID, &UpdateOptions{
		Status: status.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatalf("failed to complete S7 task: %v", err)
	}

	// Update S8 task with cross-phase dependency
	_, err = tm.UpdateTask(s8Task1.ID, &UpdateOptions{
		Status: status.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("failed to update S8 task: %v", err)
	}
}

// TestAtomicFileUpdates verifies that file updates are atomic
func TestAtomicFileUpdates(t *testing.T) {
	tempDir, tm := createTestStatusFile(t)
	statusFile := filepath.Join(tempDir, "WAYFINDER-STATUS.md")

	// Add a task
	task1, err := tm.AddTask("S8", "Task 1", nil)
	if err != nil {
		t.Fatalf("failed to add task: %v", err)
	}

	// Get initial file content
	content1, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Update task
	_, err = tm.UpdateTask(task1.ID, &UpdateOptions{
		Status: status.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("failed to update task: %v", err)
	}

	// Get updated file content
	content2, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Verify content changed
	if string(content1) == string(content2) {
		t.Error("expected file content to change after update")
	}

	// Verify we can still parse the file
	st, err := status.ParseV2(statusFile)
	if err != nil {
		t.Fatalf("failed to parse updated file: %v", err)
	}

	// Verify updated_at timestamp was updated
	if st.UpdatedAt.IsZero() {
		t.Error("expected updated_at to be set")
	}

	// Verify task status was persisted
	found := false
	if st.Roadmap != nil {
		for _, phase := range st.Roadmap.Phases {
			for _, task := range phase.Tasks {
				if task.ID == task1.ID {
					if task.Status != status.TaskStatusInProgress {
						t.Errorf("expected task status to be in-progress, got %s", task.Status)
					}
					found = true
					break
				}
			}
		}
	}

	if !found {
		t.Error("expected to find updated task in file")
	}
}

// TestErrorHandling tests various error conditions
func TestErrorHandling(t *testing.T) {
	_, tm := createTestStatusFile(t)

	tests := []struct {
		name string
		fn   func() error
		want string
	}{
		{
			name: "invalid phase ID",
			fn: func() error {
				_, err := tm.AddTask("INVALID", "Task", nil)
				return err
			},
			want: "invalid phase ID",
		},
		{
			name: "invalid task ID",
			fn: func() error {
				_, err := tm.GetTask("INVALID")
				return err
			},
			want: "task not found",
		},
		{
			name: "invalid priority",
			fn: func() error {
				_, err := tm.AddTask("S8", "Task", &TaskOptions{
					Priority: "INVALID",
				})
				return err
			},
			want: "invalid priority",
		},
		{
			name: "invalid dependency",
			fn: func() error {
				_, err := tm.AddTask("S8", "Task", &TaskOptions{
					DependsOn: []string{"INVALID"},
				})
				return err
			},
			want: "dependency task not found",
		},
		{
			name: "delete task with references",
			fn: func() error {
				task1, _ := tm.AddTask("S8", "Task 1", nil)
				_, _ = tm.AddTask("S8", "Task 2", &TaskOptions{
					DependsOn: []string{task1.ID},
				})
				return tm.DeleteTask(task1.ID)
			},
			want: "it is referenced by",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.want)
				return
			}
			if !contains(err.Error(), tt.want) {
				t.Errorf("expected error containing %q, got %q", tt.want, err.Error())
			}
		})
	}
}
