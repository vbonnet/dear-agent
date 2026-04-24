package taskmanager

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TaskManager handles task operations on WAYFINDER-STATUS.md V2 files
type TaskManager struct {
	statusFile string
	repoRoot   string // root directory for deliverable validation; empty disables validation
}

// New creates a new TaskManager for the given project directory.
// Uses projectDir as the repo root for deliverable validation.
func New(projectDir string) *TaskManager {
	return &TaskManager{
		statusFile: filepath.Join(projectDir, "WAYFINDER-STATUS.md"),
		repoRoot:   projectDir,
	}
}

// NewWithRepoRoot creates a TaskManager with an explicit repo root for
// deliverable validation. If repoRoot is empty, deliverable validation is skipped.
func NewWithRepoRoot(projectDir, repoRoot string) *TaskManager {
	return &TaskManager{
		statusFile: filepath.Join(projectDir, "WAYFINDER-STATUS.md"),
		repoRoot:   repoRoot,
	}
}

// AddTask adds a new task to the specified phase
func (tm *TaskManager) AddTask(phaseID, title string, opts *TaskOptions) (*status.Task, error) {
	// Load current status
	st, err := status.ParseV2(tm.statusFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load status file: %w", err)
	}

	// Validate phase ID
	if !isValidPhaseID(phaseID) {
		return nil, fmt.Errorf("invalid phase ID: %s (must be one of: W0, D1, D2, D3, D4, S6, S7, S8, S11)", phaseID)
	}

	// Initialize roadmap if nil
	if st.Roadmap == nil {
		st.Roadmap = &status.Roadmap{
			Phases: []status.RoadmapPhase{},
		}
	}

	// Find or create the phase
	phase := tm.findOrCreatePhase(st, phaseID)

	// Generate task ID
	taskID := generateTaskID(phase)

	// Create task
	task := &status.Task{
		ID:     taskID,
		Title:  title,
		Status: status.TaskStatusPending,
	}

	// Apply options
	if opts != nil {
		if opts.EffortDays > 0 {
			task.EffortDays = opts.EffortDays
		}
		if opts.Description != "" {
			task.Description = opts.Description
		}
		if opts.Priority != "" {
			if !isValidPriority(opts.Priority) {
				return nil, fmt.Errorf("invalid priority: %s (must be P0, P1, or P2)", opts.Priority)
			}
			task.Priority = opts.Priority
		}
		if len(opts.DependsOn) > 0 {
			task.DependsOn = opts.DependsOn
		}
		if len(opts.Deliverables) > 0 {
			task.Deliverables = opts.Deliverables
		}
		if len(opts.AcceptanceCriteria) > 0 {
			task.AcceptanceCriteria = opts.AcceptanceCriteria
		}
		if opts.AssignedTo != "" {
			task.AssignedTo = opts.AssignedTo
		}
		if opts.BeadID != "" {
			task.BeadID = opts.BeadID
		}
		if opts.Notes != "" {
			task.Notes = opts.Notes
		}
	}

	// Validate deliverable file references when a bead is being assigned
	if tm.repoRoot != "" && task.BeadID != "" && len(task.Deliverables) > 0 {
		if err := ValidateDeliverables(tm.repoRoot, task.Deliverables); err != nil {
			return nil, fmt.Errorf("deliverable validation failed: %w", err)
		}
	}

	// Validate dependencies exist
	if err := tm.validateDependencies(st, task.DependsOn); err != nil {
		return nil, err
	}

	// Check for cycles
	validator := NewDependencyValidator(st)
	if err := validator.ValidateTask(task); err != nil {
		return nil, fmt.Errorf("dependency validation failed: %w", err)
	}

	// Add task to phase
	phase.Tasks = append(phase.Tasks, *task)

	// Update timestamp
	st.UpdatedAt = time.Now()

	// Write back
	if err := status.WriteV2(st, tm.statusFile); err != nil {
		return nil, fmt.Errorf("failed to write status file: %w", err)
	}

	return task, nil
}

// UpdateTask updates an existing task
func (tm *TaskManager) UpdateTask(taskID string, opts *UpdateOptions) (*status.Task, error) {
	// Load current status
	st, err := status.ParseV2(tm.statusFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load status file: %w", err)
	}

	// Find the task
	task, phase := tm.findTask(st, taskID)
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	// Apply updates
	if opts.Status != "" {
		if !isValidTaskStatus(opts.Status) {
			return nil, fmt.Errorf("invalid status: %s (must be pending, in-progress, completed, or blocked)", opts.Status)
		}
		task.Status = opts.Status

		// Set timestamps based on status
		now := time.Now()
		if opts.Status == status.TaskStatusInProgress && task.StartedAt == nil {
			task.StartedAt = &now
		}
		if opts.Status == status.TaskStatusCompleted && task.CompletedAt == nil {
			task.CompletedAt = &now
		}
	}

	if opts.Title != "" {
		task.Title = opts.Title
	}

	if opts.Description != "" {
		task.Description = opts.Description
	}

	if opts.EffortDays > 0 {
		task.EffortDays = opts.EffortDays
	}

	if opts.Priority != "" {
		if !isValidPriority(opts.Priority) {
			return nil, fmt.Errorf("invalid priority: %s (must be P0, P1, or P2)", opts.Priority)
		}
		task.Priority = opts.Priority
	}

	if opts.TestsStatus != "" {
		task.TestsStatus = &opts.TestsStatus
	}

	if opts.AssignedTo != "" {
		task.AssignedTo = opts.AssignedTo
	}

	if opts.Notes != "" {
		task.Notes = opts.Notes
	}

	if opts.BeadID != "" {
		task.BeadID = opts.BeadID
	}

	if len(opts.Deliverables) > 0 {
		task.Deliverables = opts.Deliverables
	}

	if len(opts.AcceptanceCriteria) > 0 {
		task.AcceptanceCriteria = opts.AcceptanceCriteria
	}

	// Validate deliverable file references when a bead is assigned
	if tm.repoRoot != "" && task.BeadID != "" && len(task.Deliverables) > 0 {
		if err := ValidateDeliverables(tm.repoRoot, task.Deliverables); err != nil {
			return nil, fmt.Errorf("deliverable validation failed: %w", err)
		}
	}

	if len(opts.DependsOn) > 0 {
		task.DependsOn = opts.DependsOn
		// Validate dependencies
		if err := tm.validateDependencies(st, task.DependsOn); err != nil {
			return nil, err
		}
		// Check for cycles
		validator := NewDependencyValidator(st)
		if err := validator.ValidateTask(task); err != nil {
			return nil, fmt.Errorf("dependency validation failed: %w", err)
		}
	}

	if opts.VerifyCommand != "" {
		task.VerifyCommand = opts.VerifyCommand
	}

	if opts.VerifyExpected != "" {
		task.VerifyExpected = opts.VerifyExpected
	}

	// Update the task in the phase
	for i := range phase.Tasks {
		if phase.Tasks[i].ID == taskID {
			phase.Tasks[i] = *task
			break
		}
	}

	// Update timestamp
	st.UpdatedAt = time.Now()

	// Write back
	if err := status.WriteV2(st, tm.statusFile); err != nil {
		return nil, fmt.Errorf("failed to write status file: %w", err)
	}

	return task, nil
}

// GetTask retrieves a task by ID
func (tm *TaskManager) GetTask(taskID string) (*status.Task, error) {
	st, err := status.ParseV2(tm.statusFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load status file: %w", err)
	}

	task, _ := tm.findTask(st, taskID)
	if task == nil {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	return task, nil
}

// ListTasks lists tasks with optional filtering
func (tm *TaskManager) ListTasks(filter *TaskFilter) ([]TaskWithPhase, error) {
	st, err := status.ParseV2(tm.statusFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load status file: %w", err)
	}

	var results []TaskWithPhase

	if st.Roadmap == nil {
		return results, nil
	}

	for _, phase := range st.Roadmap.Phases {
		// Filter by phase if specified
		if filter != nil && filter.PhaseID != "" && phase.ID != filter.PhaseID {
			continue
		}

		for _, task := range phase.Tasks {
			// Filter by status if specified
			if filter != nil && filter.Status != "" && task.Status != filter.Status {
				continue
			}

			results = append(results, TaskWithPhase{
				Task:    task,
				PhaseID: phase.ID,
			})
		}
	}

	return results, nil
}

// DeleteTask removes a task from the roadmap
func (tm *TaskManager) DeleteTask(taskID string) error {
	st, err := status.ParseV2(tm.statusFile)
	if err != nil {
		return fmt.Errorf("failed to load status file: %w", err)
	}

	// Find and remove the task
	found := false
	if st.Roadmap != nil {
		for i := range st.Roadmap.Phases {
			phase := &st.Roadmap.Phases[i]
			for j, task := range phase.Tasks {
				if task.ID == taskID {
					// Check if other tasks depend on this one
					if err := tm.checkTaskReferences(st, taskID); err != nil {
						return err
					}

					// Remove task
					phase.Tasks = append(phase.Tasks[:j], phase.Tasks[j+1:]...)
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	if !found {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Update timestamp
	st.UpdatedAt = time.Now()

	// Write back
	if err := status.WriteV2(st, tm.statusFile); err != nil {
		return fmt.Errorf("failed to write status file: %w", err)
	}

	return nil
}

// Helper functions

func (tm *TaskManager) findOrCreatePhase(st *status.StatusV2, phaseID string) *status.RoadmapPhase {
	// Find existing phase
	for i := range st.Roadmap.Phases {
		if st.Roadmap.Phases[i].ID == phaseID {
			return &st.Roadmap.Phases[i]
		}
	}

	// Create new phase
	phaseName := getPhaseNameByID(phaseID)
	newPhase := status.RoadmapPhase{
		ID:     phaseID,
		Name:   phaseName,
		Status: status.PhaseStatusV2Pending,
		Tasks:  []status.Task{},
	}

	st.Roadmap.Phases = append(st.Roadmap.Phases, newPhase)
	return &st.Roadmap.Phases[len(st.Roadmap.Phases)-1]
}

func (tm *TaskManager) findTask(st *status.StatusV2, taskID string) (*status.Task, *status.RoadmapPhase) {
	if st.Roadmap == nil {
		return nil, nil
	}

	for i := range st.Roadmap.Phases {
		phase := &st.Roadmap.Phases[i]
		for j := range phase.Tasks {
			if phase.Tasks[j].ID == taskID {
				return &phase.Tasks[j], phase
			}
		}
	}

	return nil, nil
}

func (tm *TaskManager) validateDependencies(st *status.StatusV2, deps []string) error {
	if len(deps) == 0 {
		return nil
	}

	if st.Roadmap == nil {
		return fmt.Errorf("no tasks exist to depend on")
	}

	// Build set of existing task IDs
	existing := make(map[string]bool)
	for _, phase := range st.Roadmap.Phases {
		for _, task := range phase.Tasks {
			existing[task.ID] = true
		}
	}

	// Check each dependency
	for _, dep := range deps {
		if !existing[dep] {
			return fmt.Errorf("dependency task not found: %s", dep)
		}
	}

	return nil
}

func (tm *TaskManager) checkTaskReferences(st *status.StatusV2, taskID string) error {
	if st.Roadmap == nil {
		return nil
	}

	var references []string
	for _, phase := range st.Roadmap.Phases {
		for _, task := range phase.Tasks {
			for _, dep := range task.DependsOn {
				if dep == taskID {
					references = append(references, task.ID)
				}
			}
		}
	}

	if len(references) > 0 {
		return fmt.Errorf("cannot delete task %s: it is referenced by %v", taskID, references)
	}

	return nil
}

func generateTaskID(phase *status.RoadmapPhase) string {
	// Generate task ID in format: phase-N (e.g., "S8-1", "S8-2")
	maxNum := 0
	prefix := phase.ID + "-"

	for _, task := range phase.Tasks {
		var num int
		if _, err := fmt.Sscanf(task.ID, prefix+"%d", &num); err == nil {
			if num > maxNum {
				maxNum = num
			}
		}
	}

	return fmt.Sprintf("%s-%d", phase.ID, maxNum+1)
}

func isValidPhaseID(phaseID string) bool {
	validPhases := map[string]bool{
		"W0": true, "D1": true, "D2": true, "D3": true,
		"D4": true, "S6": true, "S7": true, "S8": true, "S11": true,
	}
	return validPhases[phaseID]
}

func isValidTaskStatus(taskStatus string) bool {
	return taskStatus == status.TaskStatusPending ||
		taskStatus == status.TaskStatusInProgress ||
		taskStatus == status.TaskStatusCompleted ||
		taskStatus == status.TaskStatusBlocked
}

func isValidPriority(priority string) bool {
	return priority == status.PriorityP0 ||
		priority == status.PriorityP1 ||
		priority == status.PriorityP2
}

func getPhaseNameByID(phaseID string) string {
	names := map[string]string{
		"W0":  "Intake & Waypoint",
		"D1":  "Discovery & Context",
		"D2":  "Investigation & Options",
		"D3":  "Architecture & Design Spec",
		"D4":  "Solution Requirements",
		"S6":  "Design",
		"S7":  "Planning & Task Breakdown",
		"S8":  "BUILD Loop",
		"S11": "Closure & Retrospective",
	}
	if name, ok := names[phaseID]; ok {
		return name
	}
	return phaseID
}
