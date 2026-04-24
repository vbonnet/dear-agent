package ops

import (
	"strings"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseTasks(t *testing.T) {
	// Create a temporary backlog file
	tmpDir := t.TempDir()
	backlogPath := filepath.Join(tmpDir, "backlog.md")

	content := `## Backlog
- [ ] #1 [P0] Implement agm task CLI | status:queued
- [x] #2 [P1] Fix sycophancy hook | status:done | completed:2026-04-12
- [ ] #3 [P2] EventBus PLAN phase | status:in-progress
`

	if err := os.WriteFile(backlogPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test backlog: %v", err)
	}

	tasks, err := parseTasks(backlogPath)
	if err != nil {
		t.Fatalf("Failed to parse tasks: %v", err)
	}

	if len(tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(tasks))
	}

	// Check first task
	if tasks[0].ID != "1" || tasks[0].Status != "queued" || tasks[0].Priority != "P0" {
		t.Errorf("Task 1 parsed incorrectly: %+v", tasks[0])
	}

	// Check second task (completed)
	if tasks[1].ID != "2" || tasks[1].Status != "done" || tasks[1].Completed.IsZero() {
		t.Errorf("Task 2 parsed incorrectly: %+v", tasks[1])
	}

	// Check third task
	if tasks[2].ID != "3" || tasks[2].Status != "in-progress" || tasks[2].Priority != "P2" {
		t.Errorf("Task 3 parsed incorrectly: %+v", tasks[2])
	}
}

func TestListTasks(t *testing.T) {
	tmpDir := t.TempDir()
	backlogPath := filepath.Join(tmpDir, "backlog.md")

	content := `## Backlog
- [ ] #1 [P0] Task 1 | status:queued
- [ ] #2 [P1] Task 2 | status:in-progress
- [x] #3 [P2] Task 3 | status:done
`

	if err := os.WriteFile(backlogPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test backlog: %v", err)
	}

	// Test listing all tasks
	result, err := ListTasks(&OpContext{}, &TaskListRequest{FilePath: backlogPath})
	if err != nil {
		t.Fatalf("Failed to list tasks: %v", err)
	}

	if result.Total != 3 {
		t.Errorf("Expected 3 total tasks, got %d", result.Total)
	}

	// Test filtering by status
	result, err = ListTasks(&OpContext{}, &TaskListRequest{
		FilePath: backlogPath,
		Status:   "queued",
	})
	if err != nil {
		t.Fatalf("Failed to filter tasks: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("Expected 1 queued task, got %d", result.Total)
	}
}

func TestTaskNext(t *testing.T) {
	tmpDir := t.TempDir()
	backlogPath := filepath.Join(tmpDir, "backlog.md")

	content := `## Backlog
- [x] #1 [P0] Task 1 | status:done
- [ ] #2 [P1] Task 2 | status:queued
- [ ] #3 [P2] Task 3 | status:in-progress
`

	if err := os.WriteFile(backlogPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test backlog: %v", err)
	}

	result, err := TaskNext(&OpContext{}, &TaskNextRequest{FilePath: backlogPath})
	if err != nil {
		t.Fatalf("Failed to get next task: %v", err)
	}

	if result.Task == nil || result.Task.ID != "2" {
		t.Errorf("Expected task #2, got %v", result.Task)
	}
}

func TestTaskComplete(t *testing.T) {
	tmpDir := t.TempDir()
	backlogPath := filepath.Join(tmpDir, "backlog.md")

	content := `## Backlog
- [ ] #1 [P0] Task 1 | status:queued
- [ ] #2 [P1] Task 2 | status:in-progress
`

	if err := os.WriteFile(backlogPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test backlog: %v", err)
	}

	result, err := TaskComplete(&OpContext{}, &TaskCompleteRequest{
		ID:       "2",
		FilePath: backlogPath,
	})
	if err != nil {
		t.Fatalf("Failed to complete task: %v", err)
	}

	if result.Task.Status != "done" || result.Task.Completed.IsZero() {
		t.Errorf("Task not completed correctly: %+v", result.Task)
	}

	// Verify persistence
	tasks, err := parseTasks(backlogPath)
	if err != nil {
		t.Fatalf("Failed to parse tasks: %v", err)
	}

	found := false
	for _, task := range tasks {
		if task.ID == "2" && task.Status == "done" && !task.Completed.IsZero() {
			found = true
			break
		}
	}

	if !found {
		t.Error("Completed task not persisted correctly")
	}
}

func TestTaskUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	backlogPath := filepath.Join(tmpDir, "backlog.md")

	content := `## Backlog
- [ ] #1 [P0] Task 1 | status:queued
- [ ] #2 [P1] Task 2 | status:queued
`

	if err := os.WriteFile(backlogPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test backlog: %v", err)
	}

	result, err := TaskUpdate(&OpContext{}, &TaskUpdateRequest{
		ID:       "1",
		Status:   "in-progress",
		FilePath: backlogPath,
	})
	if err != nil {
		t.Fatalf("Failed to update task: %v", err)
	}

	if result.Task.Status != "in-progress" {
		t.Errorf("Task status not updated: %+v", result.Task)
	}

	// Verify persistence
	tasks, err := parseTasks(backlogPath)
	if err != nil {
		t.Fatalf("Failed to parse tasks: %v", err)
	}

	if tasks[0].Status != "in-progress" {
		t.Error("Task update not persisted correctly")
	}
}

func TestWriteTasks(t *testing.T) {
	tmpDir := t.TempDir()
	backlogPath := filepath.Join(tmpDir, "backlog.md")

	tasks := []Task{
		{
			ID:          "1",
			Priority:    "P0",
			Status:      "queued",
			Description: "Task 1",
		},
		{
			ID:          "2",
			Priority:    "P1",
			Status:      "done",
			Description: "Task 2",
			Completed:   time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC),
		},
	}

	if err := writeTasks(backlogPath, tasks); err != nil {
		t.Fatalf("Failed to write tasks: %v", err)
	}

	// Verify by reading back
	parsed, err := parseTasks(backlogPath)
	if err != nil {
		t.Fatalf("Failed to parse tasks: %v", err)
	}

	if len(parsed) != 2 {
		t.Errorf("Expected 2 tasks after write, got %d", len(parsed))
	}

	if parsed[1].Completed.IsZero() {
		t.Error("Completion timestamp not preserved")
	}
}

func TestTaskNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	backlogPath := filepath.Join(tmpDir, "backlog.md")

	content := `## Backlog
- [ ] #1 [P0] Task 1 | status:queued
`

	if err := os.WriteFile(backlogPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test backlog: %v", err)
	}

	_, err := TaskComplete(&OpContext{}, &TaskCompleteRequest{
		ID:       "999",
		FilePath: backlogPath,
	})

	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected not found error, got: %v", err)
	}
}

