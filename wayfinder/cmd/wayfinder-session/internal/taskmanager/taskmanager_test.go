package taskmanager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// createTestStatusFile creates a test WAYFINDER-STATUS.md file
func createTestStatusFile(t *testing.T) (string, *TaskManager) {
	t.Helper()

	tempDir := t.TempDir()
	statusFile := filepath.Join(tempDir, "WAYFINDER-STATUS.md")

	// Create a basic V2 status file
	st := status.NewStatusV2("Test Project", status.ProjectTypeFeature, status.RiskLevelM)
	if err := status.WriteV2(st, statusFile); err != nil {
		t.Fatalf("failed to create test status file: %v", err)
	}

	return tempDir, New(tempDir)
}

func TestAddTask(t *testing.T) {
	_, tm := createTestStatusFile(t)

	tests := []struct {
		name      string
		phaseID   string
		title     string
		opts      *TaskOptions
		wantErr   bool
		errString string
	}{
		{
			name:    "basic task",
			phaseID: "S8",
			title:   "Implement feature",
			opts:    nil,
			wantErr: false,
		},
		{
			name:    "task with effort",
			phaseID: "S8",
			title:   "Another task",
			opts: &TaskOptions{
				EffortDays: 3.5,
				Priority:   status.PriorityP0,
			},
			wantErr: false,
		},
		{
			name:      "invalid phase",
			phaseID:   "X99",
			title:     "Invalid",
			opts:      nil,
			wantErr:   true,
			errString: "invalid phase ID",
		},
		{
			name:    "invalid priority",
			phaseID: "S8",
			title:   "Task",
			opts: &TaskOptions{
				Priority: "PX",
			},
			wantErr:   true,
			errString: "invalid priority",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := tm.AddTask(tt.phaseID, tt.title, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errString)
					return
				}
				if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expected error containing %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if task.Title != tt.title {
				t.Errorf("expected title %q, got %q", tt.title, task.Title)
			}

			if task.Status != status.TaskStatusPending {
				t.Errorf("expected status %q, got %q", status.TaskStatusPending, task.Status)
			}

			if tt.opts != nil && tt.opts.EffortDays > 0 {
				if task.EffortDays != tt.opts.EffortDays {
					t.Errorf("expected effort %.1f, got %.1f", tt.opts.EffortDays, task.EffortDays)
				}
			}
		})
	}
}

func TestUpdateTask(t *testing.T) {
	_, tm := createTestStatusFile(t)

	// Create a task first
	task, err := tm.AddTask("S8", "Test task", nil)
	if err != nil {
		t.Fatalf("failed to add task: %v", err)
	}

	tests := []struct {
		name      string
		taskID    string
		opts      *UpdateOptions
		wantErr   bool
		errString string
	}{
		{
			name:   "update status to in-progress",
			taskID: task.ID,
			opts: &UpdateOptions{
				Status: status.TaskStatusInProgress,
			},
			wantErr: false,
		},
		{
			name:   "update status to completed",
			taskID: task.ID,
			opts: &UpdateOptions{
				Status: status.TaskStatusCompleted,
			},
			wantErr: false,
		},
		{
			name:   "update priority",
			taskID: task.ID,
			opts: &UpdateOptions{
				Priority: status.PriorityP0,
			},
			wantErr: false,
		},
		{
			name:   "update effort",
			taskID: task.ID,
			opts: &UpdateOptions{
				EffortDays: 5.0,
			},
			wantErr: false,
		},
		{
			name:      "invalid task id",
			taskID:    "INVALID",
			opts:      &UpdateOptions{},
			wantErr:   true,
			errString: "task not found",
		},
		{
			name:   "invalid status",
			taskID: task.ID,
			opts: &UpdateOptions{
				Status: "invalid-status",
			},
			wantErr:   true,
			errString: "invalid status",
		},
		{
			name:   "update verify command",
			taskID: task.ID,
			opts: &UpdateOptions{
				VerifyCommand:  "go test ./auth/...",
				VerifyExpected: "exit code 0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated, err := tm.UpdateTask(tt.taskID, tt.opts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errString)
					return
				}
				if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expected error containing %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.opts.Status != "" && updated.Status != tt.opts.Status {
				t.Errorf("expected status %q, got %q", tt.opts.Status, updated.Status)
			}

			if tt.opts.Priority != "" && updated.Priority != tt.opts.Priority {
				t.Errorf("expected priority %q, got %q", tt.opts.Priority, updated.Priority)
			}

			if tt.opts.EffortDays > 0 && updated.EffortDays != tt.opts.EffortDays {
				t.Errorf("expected effort %.1f, got %.1f", tt.opts.EffortDays, updated.EffortDays)
			}

			if tt.opts.VerifyCommand != "" && updated.VerifyCommand != tt.opts.VerifyCommand {
				t.Errorf("expected verify_command %q, got %q", tt.opts.VerifyCommand, updated.VerifyCommand)
			}

			if tt.opts.VerifyExpected != "" && updated.VerifyExpected != tt.opts.VerifyExpected {
				t.Errorf("expected verify_expected %q, got %q", tt.opts.VerifyExpected, updated.VerifyExpected)
			}

			// Check timestamps
			if tt.opts.Status == status.TaskStatusInProgress && updated.StartedAt == nil {
				t.Error("expected StartedAt to be set for in-progress status")
			}

			if tt.opts.Status == status.TaskStatusCompleted && updated.CompletedAt == nil {
				t.Error("expected CompletedAt to be set for completed status")
			}
		})
	}
}

func TestGetTask(t *testing.T) {
	_, tm := createTestStatusFile(t)

	// Add a test task
	added, err := tm.AddTask("S8", "Test task", &TaskOptions{
		EffortDays: 3.0,
		Priority:   status.PriorityP1,
	})
	if err != nil {
		t.Fatalf("failed to add task: %v", err)
	}

	tests := []struct {
		name      string
		taskID    string
		wantErr   bool
		errString string
	}{
		{
			name:    "existing task",
			taskID:  added.ID,
			wantErr: false,
		},
		{
			name:      "non-existent task",
			taskID:    "INVALID",
			wantErr:   true,
			errString: "task not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := tm.GetTask(tt.taskID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errString)
					return
				}
				if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expected error containing %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if task.ID != tt.taskID {
				t.Errorf("expected ID %q, got %q", tt.taskID, task.ID)
			}
		})
	}
}

func TestListTasks(t *testing.T) {
	_, tm := createTestStatusFile(t)

	// Add multiple tasks
	_, err := tm.AddTask("S8", "Task 1", &TaskOptions{Priority: status.PriorityP0})
	if err != nil {
		t.Fatalf("failed to add task 1: %v", err)
	}

	_, err = tm.AddTask("S8", "Task 2", &TaskOptions{Priority: status.PriorityP1})
	if err != nil {
		t.Fatalf("failed to add task 2: %v", err)
	}

	_, err = tm.AddTask("S7", "Task 3", &TaskOptions{Priority: status.PriorityP0})
	if err != nil {
		t.Fatalf("failed to add task 3: %v", err)
	}

	// Update one task status
	_, err = tm.UpdateTask("S8-1", &UpdateOptions{Status: status.TaskStatusInProgress})
	if err != nil {
		t.Fatalf("failed to update task: %v", err)
	}

	tests := []struct {
		name       string
		filter     *TaskFilter
		wantCount  int
		wantPhases []string
	}{
		{
			name:       "all tasks",
			filter:     nil,
			wantCount:  3,
			wantPhases: []string{"S8", "S8", "S7"},
		},
		{
			name:       "filter by phase S8",
			filter:     &TaskFilter{PhaseID: "S8"},
			wantCount:  2,
			wantPhases: []string{"S8", "S8"},
		},
		{
			name:       "filter by phase S7",
			filter:     &TaskFilter{PhaseID: "S7"},
			wantCount:  1,
			wantPhases: []string{"S7"},
		},
		{
			name:       "filter by status pending",
			filter:     &TaskFilter{Status: status.TaskStatusPending},
			wantCount:  2,
			wantPhases: []string{"S8", "S7"},
		},
		{
			name:       "filter by status in-progress",
			filter:     &TaskFilter{Status: status.TaskStatusInProgress},
			wantCount:  1,
			wantPhases: []string{"S8"},
		},
		{
			name:       "filter by phase and status",
			filter:     &TaskFilter{PhaseID: "S8", Status: status.TaskStatusPending},
			wantCount:  1,
			wantPhases: []string{"S8"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := tm.ListTasks(tt.filter)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(tasks) != tt.wantCount {
				t.Errorf("expected %d tasks, got %d", tt.wantCount, len(tasks))
			}

			for i, task := range tasks {
				if i < len(tt.wantPhases) && task.PhaseID != tt.wantPhases[i] {
					t.Errorf("task %d: expected phase %q, got %q", i, tt.wantPhases[i], task.PhaseID)
				}
			}
		})
	}
}

func TestDeleteTask(t *testing.T) {
	_, tm := createTestStatusFile(t)

	// Add tasks
	task1, _ := tm.AddTask("S8", "Task 1", nil)
	task2, _ := tm.AddTask("S8", "Task 2", &TaskOptions{DependsOn: []string{task1.ID}})

	tests := []struct {
		name      string
		taskID    string
		wantErr   bool
		errString string
	}{
		{
			name:      "cannot delete task with dependencies",
			taskID:    task1.ID,
			wantErr:   true,
			errString: "it is referenced by",
		},
		{
			name:    "delete task without dependencies",
			taskID:  task2.ID,
			wantErr: false,
		},
		{
			name:      "non-existent task",
			taskID:    "INVALID",
			wantErr:   true,
			errString: "task not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tm.DeleteTask(tt.taskID)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errString)
					return
				}
				if tt.errString != "" && !contains(err.Error(), tt.errString) {
					t.Errorf("expected error containing %q, got %q", tt.errString, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify task is deleted
			_, err = tm.GetTask(tt.taskID)
			if err == nil {
				t.Error("expected task to be deleted")
			}
		})
	}
}

func TestTaskDependencies(t *testing.T) {
	_, tm := createTestStatusFile(t)

	// Add base task
	task1, err := tm.AddTask("S8", "Task 1", nil)
	if err != nil {
		t.Fatalf("failed to add task 1: %v", err)
	}

	// Add dependent task
	task2, err := tm.AddTask("S8", "Task 2", &TaskOptions{
		DependsOn: []string{task1.ID},
	})
	if err != nil {
		t.Fatalf("failed to add task 2: %v", err)
	}

	if len(task2.DependsOn) != 1 || task2.DependsOn[0] != task1.ID {
		t.Errorf("expected task 2 to depend on task 1")
	}

	// Try to add task with invalid dependency
	_, err = tm.AddTask("S8", "Task 3", &TaskOptions{
		DependsOn: []string{"INVALID"},
	})
	if err == nil {
		t.Error("expected error for invalid dependency")
	}
}

func TestTaskIDGeneration(t *testing.T) {
	_, tm := createTestStatusFile(t)

	// Add multiple tasks to same phase
	task1, _ := tm.AddTask("S8", "Task 1", nil)
	task2, _ := tm.AddTask("S8", "Task 2", nil)
	task3, _ := tm.AddTask("S8", "Task 3", nil)

	if task1.ID != "S8-1" {
		t.Errorf("expected task1 ID to be S8-1, got %s", task1.ID)
	}
	if task2.ID != "S8-2" {
		t.Errorf("expected task2 ID to be S8-2, got %s", task2.ID)
	}
	if task3.ID != "S8-3" {
		t.Errorf("expected task3 ID to be S8-3, got %s", task3.ID)
	}

	// Add task to different phase
	task4, _ := tm.AddTask("S7", "Task 4", nil)
	if task4.ID != "S7-1" {
		t.Errorf("expected task4 ID to be S7-1, got %s", task4.ID)
	}
}

func TestAtomicUpdates(t *testing.T) {
	tempDir, tm := createTestStatusFile(t)

	// Add a task
	_, err := tm.AddTask("S8", "Task 1", nil)
	if err != nil {
		t.Fatalf("failed to add task: %v", err)
	}

	// Verify file was updated
	statusFile := filepath.Join(tempDir, "WAYFINDER-STATUS.md")
	stat1, err := os.Stat(statusFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Update the task
	_, err = tm.UpdateTask("S8-1", &UpdateOptions{
		Status: status.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("failed to update task: %v", err)
	}

	// Verify file was updated again
	stat2, err := os.Stat(statusFile)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	if !stat2.ModTime().After(stat1.ModTime()) {
		t.Error("expected file modification time to be updated")
	}

	// Verify we can parse the updated file
	st, err := status.ParseV2(statusFile)
	if err != nil {
		t.Fatalf("failed to parse updated file: %v", err)
	}

	if st.Roadmap == nil || len(st.Roadmap.Phases) == 0 {
		t.Error("expected roadmap to be present")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
