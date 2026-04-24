package taskmanager

import "github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"

// TaskOptions contains optional parameters for creating a task
type TaskOptions struct {
	EffortDays         float64
	Description        string
	Priority           string
	DependsOn          []string
	Deliverables       []string
	AcceptanceCriteria []string
	AssignedTo         string
	BeadID             string
	Notes              string
}

// UpdateOptions contains fields that can be updated on a task
type UpdateOptions struct {
	Status             string
	Title              string
	Description        string
	EffortDays         float64
	Priority           string
	TestsStatus        string
	AssignedTo         string
	Notes              string
	BeadID             string
	Deliverables       []string
	AcceptanceCriteria []string
	DependsOn          []string
	VerifyCommand      string
	VerifyExpected     string
}

// TaskFilter contains filtering options for listing tasks
type TaskFilter struct {
	PhaseID string
	Status  string
}

// TaskWithPhase wraps a task with its phase information
type TaskWithPhase struct {
	Task    status.Task
	PhaseID string
}
