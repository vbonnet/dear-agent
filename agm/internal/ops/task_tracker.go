package ops

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Task represents a single task in the backlog.
type Task struct {
	ID          string    `json:"id"`
	Priority    string    `json:"priority"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	Completed   time.Time `json:"completed,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
}

// TaskListRequest defines the input for listing tasks.
type TaskListRequest struct {
	// Status filters by status: "queued", "in-progress", "blocked", "done", or "" for all.
	Status string `json:"status,omitempty"`
	// FilePath is the path to the backlog markdown file (default: .agm/backlog.md).
	FilePath string `json:"file_path,omitempty"`
}

// TaskListResult is the output of listing tasks.
type TaskListResult struct {
	Tasks []Task `json:"tasks"`
	Total int    `json:"total"`
}

// TaskNextRequest defines the input for getting the next task.
type TaskNextRequest struct {
	// FilePath is the path to the backlog markdown file (default: .agm/backlog.md).
	FilePath string `json:"file_path,omitempty"`
}

// TaskNextResult is the output of getting the next task.
type TaskNextResult struct {
	Task *Task `json:"task"`
}

// TaskCompleteRequest defines the input for completing a task.
type TaskCompleteRequest struct {
	ID       string `json:"id"`
	FilePath string `json:"file_path,omitempty"`
}

// TaskCompleteResult is the output of completing a task.
type TaskCompleteResult struct {
	Task *Task `json:"task"`
}

// TaskUpdateRequest defines the input for updating a task.
type TaskUpdateRequest struct {
	ID       string `json:"id"`
	Status   string `json:"status,omitempty"`
	Note     string `json:"note,omitempty"`
	FilePath string `json:"file_path,omitempty"`
}

// TaskUpdateResult is the output of updating a task.
type TaskUpdateResult struct {
	Task *Task `json:"task"`
}

// getBacklogPath returns the path to the backlog file, defaulting to .agm/backlog.md.
func getBacklogPath(filePath string) string {
	if filePath != "" {
		return filePath
	}
	wd, err := os.Getwd()
	if err != nil {
		return ".agm/backlog.md"
	}
	return filepath.Join(wd, ".agm", "backlog.md")
}

// ListTasks returns all tasks from the backlog, optionally filtered by status.
func ListTasks(ctx *OpContext, req *TaskListRequest) (*TaskListResult, error) {
	if req == nil {
		req = &TaskListRequest{}
	}

	backlogPath := getBacklogPath(req.FilePath)
	tasks, err := parseTasks(backlogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse backlog: %w", err)
	}

	// Filter by status if provided
	var filtered []Task
	if req.Status == "" {
		filtered = tasks
	} else {
		for _, t := range tasks {
			if t.Status == req.Status {
				filtered = append(filtered, t)
			}
		}
	}

	return &TaskListResult{
		Tasks: filtered,
		Total: len(filtered),
	}, nil
}

// TaskNext returns the first task with status != "done".
func TaskNext(ctx *OpContext, req *TaskNextRequest) (*TaskNextResult, error) {
	if req == nil {
		req = &TaskNextRequest{}
	}

	backlogPath := getBacklogPath(req.FilePath)
	tasks, err := parseTasks(backlogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse backlog: %w", err)
	}

	// Return the first unfinished task
	for _, t := range tasks {
		if t.Status != "done" {
			return &TaskNextResult{Task: &t}, nil
		}
	}

	// No unfinished tasks
	return &TaskNextResult{Task: nil}, nil
}

// TaskComplete marks a task as done and records the completion time.
func TaskComplete(ctx *OpContext, req *TaskCompleteRequest) (*TaskCompleteResult, error) {
	if req == nil {
		req = &TaskCompleteRequest{}
	}

	if req.ID == "" {
		return nil, ErrInvalidInput("id", "Task ID is required.")
	}

	backlogPath := getBacklogPath(req.FilePath)
	tasks, err := parseTasks(backlogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse backlog: %w", err)
	}

	// Find and update the task
	var found *Task
	for i := range tasks {
		if tasks[i].ID == req.ID {
			tasks[i].Status = "done"
			tasks[i].Completed = time.Now()
			found = &tasks[i]
			break
		}
	}

	if found == nil {
		return nil, fmt.Errorf("task %s not found", req.ID)
	}

	// Write the updated tasks back to the file
	if err := writeTasks(backlogPath, tasks); err != nil {
		return nil, fmt.Errorf("failed to write backlog: %w", err)
	}

	return &TaskCompleteResult{Task: found}, nil
}

// TaskUpdate updates a task's status and optionally adds a note.
func TaskUpdate(ctx *OpContext, req *TaskUpdateRequest) (*TaskUpdateResult, error) {
	if req == nil {
		req = &TaskUpdateRequest{}
	}

	if req.ID == "" {
		return nil, ErrInvalidInput("id", "Task ID is required.")
	}

	backlogPath := getBacklogPath(req.FilePath)
	tasks, err := parseTasks(backlogPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse backlog: %w", err)
	}

	// Find and update the task
	var found *Task
	for i := range tasks {
		if tasks[i].ID == req.ID {
			if req.Status != "" {
				tasks[i].Status = req.Status
			}
			found = &tasks[i]
			break
		}
	}

	if found == nil {
		return nil, fmt.Errorf("task %s not found", req.ID)
	}

	// Write the updated tasks back to the file
	if err := writeTasks(backlogPath, tasks); err != nil {
		return nil, fmt.Errorf("failed to write backlog: %w", err)
	}

	return &TaskUpdateResult{Task: found}, nil
}

// parseTasks reads and parses the backlog markdown file.
// Format:
//   - [ ] #1 [P0] Task description | status:queued
//   - [x] #2 [P1] Another task | status:done | completed:2026-04-12
func parseTasks(backlogPath string) ([]Task, error) {
	file, err := os.Open(backlogPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty list if file doesn't exist yet
			return []Task{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var tasks []Task
	scanner := bufio.NewScanner(file)

	// Regex to match task lines: - [ ] #ID [PRIORITY] Description | status:... | completed:...
	taskRegex := regexp.MustCompile(`^\s*-\s+\[([ x])\]\s+#(\d+)\s+\[([^\]]+)\]\s+([^|]+)(.*)$`)

	for scanner.Scan() {
		line := scanner.Text()
		matches := taskRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		checked := matches[1] == "x"
		id := matches[2]
		priority := matches[3]
		description := strings.TrimSpace(matches[4])
		metadata := matches[5]

		status := "queued" // default
		var completed time.Time

		// Parse metadata fields like | status:done | completed:2026-04-12
		if metadata != "" {
			metaFields := strings.Split(metadata, "|")
			for _, field := range metaFields {
				field = strings.TrimSpace(field)
				if strings.HasPrefix(field, "status:") {
					status = strings.TrimPrefix(field, "status:")
					status = strings.TrimSpace(status)
				} else if strings.HasPrefix(field, "completed:") {
					dateStr := strings.TrimPrefix(field, "completed:")
					dateStr = strings.TrimSpace(dateStr)
					if t, err := time.Parse("2006-01-02", dateStr); err == nil {
						completed = t
					}
				}
			}
		}

		// Override status based on checkbox if not explicitly set
		if checked && status == "queued" {
			status = "done"
		}

		tasks = append(tasks, Task{
			ID:          id,
			Priority:    priority,
			Status:      status,
			Description: description,
			Completed:   completed,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Sort by ID to maintain consistent ordering
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	return tasks, nil
}

// writeTasks writes tasks back to the backlog markdown file.
func writeTasks(backlogPath string, tasks []Task) error {
	// Ensure directory exists
	dir := filepath.Dir(backlogPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(backlogPath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	// Write header
	if _, err := writer.WriteString("## Backlog\n"); err != nil {
		return err
	}

	// Sort tasks by ID for consistent output
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	// Write each task
	for _, t := range tasks {
		checkbox := "[ ]"
		if t.Status == "done" {
			checkbox = "[x]"
		}

		line := fmt.Sprintf("- %s #%s [%s] %s | status:%s",
			checkbox, t.ID, t.Priority, t.Description, t.Status)

		if !t.Completed.IsZero() {
			line += fmt.Sprintf(" | completed:%s", t.Completed.Format("2006-01-02"))
		}

		if _, err := writer.WriteString(line + "\n"); err != nil {
			return err
		}
	}

	return writer.Flush()
}
