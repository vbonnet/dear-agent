package progress

import "time"

// Progress represents the S5 feature tracking state
type Progress struct {
	SchemaVersion string    `json:"schema_version"`
	Project       string    `json:"project"`
	Waypoint      string    `json:"waypoint"`
	CreatedAt     time.Time `json:"created_at"`
	LastUpdated   time.Time `json:"last_updated"`
	Features      []Feature `json:"features"`
}

// Feature represents a single feature being tracked
type Feature struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"` // failing, in_progress, passing
	StartedAt  *time.Time `json:"started_at,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	GitCommits []string   `json:"git_commits,omitempty"`
}

// Status constants
const (
	StatusFailing    = "failing"
	StatusInProgress = "in_progress"
	StatusPassing    = "passing"
)

// Waypoint constant
const (
	WaypointS5 = "S5"
)

// SchemaVersion is the progress.json schema version.
const SchemaVersion = "1.0"

// DefaultProgressFile is the standard location for progress.json
const DefaultProgressFile = "S5-implementation/progress.json"

// ValidateStatus checks if a status string is valid
func ValidateStatus(status string) bool {
	return status == StatusFailing || status == StatusInProgress || status == StatusPassing
}

// NewProgress creates a new Progress instance with defaults
func NewProgress(project string, features []Feature) *Progress {
	now := time.Now()
	return &Progress{
		SchemaVersion: SchemaVersion,
		Project:       project,
		Waypoint:      WaypointS5,
		CreatedAt:     now,
		LastUpdated:   now,
		Features:      features,
	}
}
