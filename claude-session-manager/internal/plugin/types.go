package plugin

import "time"

// Task represents a task from any task manager (Claude tasks, beads, GitHub issues, etc.)
type Task struct {
	ID          string            // Unique task ID (e.g., "task-123", "oss-abc", "issue-456")
	Title       string            // Task title/subject
	Description string            // Detailed description
	Status      string            // Normalized status: "pending", "in_progress", "completed", "blocked", "cancelled"
	Phase       string            // Phase label (e.g., "phase-0", "phase-1")
	Labels      []string          // Additional labels
	Metadata    map[string]string // Plugin-specific metadata
	CreatedAt   time.Time         // Creation timestamp
	UpdatedAt   time.Time         // Last update timestamp
}

// PhaseStats represents aggregate statistics for a phase
type PhaseStats struct {
	Phase      string  // Phase identifier (e.g., "phase-0", "phase-1")
	Total      int     // Total tasks in phase
	Pending    int     // Tasks with status "pending"
	InProgress int     // Tasks with status "in_progress"
	Completed  int     // Tasks with status "completed"
	Blocked    int     // Tasks with status "blocked"
	Cancelled  int     // Tasks with status "cancelled"
	Percentage float64 // Completion percentage (completed/total * 100)
}

// PluginMetadata describes plugin information
type PluginMetadata struct {
	Name        string // Plugin name (e.g., "claude-tasks", "beads", "github-issues")
	Version     string // Plugin version (semver)
	Author      string // Plugin author
	Description string // Short description
}
