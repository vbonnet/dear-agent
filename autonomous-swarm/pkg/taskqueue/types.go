package taskqueue

import "time"

// TaskQueue represents the TASK-QUEUE.yaml structure
type TaskQueue struct {
	SchemaVersion string    `yaml:"schema_version" json:"schema_version"`
	LastUpdated   time.Time `yaml:"last_updated" json:"last_updated"`
	Ready         []Bead    `yaml:"ready" json:"ready"`
	InProgress    []Bead    `yaml:"in_progress" json:"in_progress"`
	Blocked       []Bead    `yaml:"blocked" json:"blocked"`
	Completed     []Bead    `yaml:"completed" json:"completed"`
}

// Bead represents a task unit in the queue
type Bead struct {
	ID        string       `yaml:"id" json:"id"`
	Tier      int          `yaml:"tier" json:"tier"`
	Title     string       `yaml:"title" json:"title"`
	Phase     string       `yaml:"phase,omitempty" json:"phase,omitempty"`
	DependsOn []string     `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Prompts   BeadPrompts  `yaml:"prompts" json:"prompts"`
	Metadata  BeadMetadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// BeadPrompts contains prompts for each stage of bead execution
type BeadPrompts struct {
	Start    string `yaml:"start" json:"start"`
	VerifyS8 string `yaml:"verify_s8,omitempty" json:"verify_s8,omitempty"`
	VerifyS9 string `yaml:"verify_s9,omitempty" json:"verify_s9,omitempty"`
	Done     string `yaml:"done,omitempty" json:"done,omitempty"`
}

// BeadMetadata tracks execution state and history
type BeadMetadata struct {
	SessionName      string    `yaml:"session_name,omitempty" json:"session_name,omitempty"`
	Iterations       int       `yaml:"iterations,omitempty" json:"iterations,omitempty"`
	LastAttempt      time.Time `yaml:"last_attempt,omitempty" json:"last_attempt,omitempty"`
	EscalationReason string    `yaml:"escalation_reason,omitempty" json:"escalation_reason,omitempty"`
}
