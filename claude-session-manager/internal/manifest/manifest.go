package manifest

import "time"

// Manifest represents a Claude session manifest (v2 schema)
type Manifest struct {
	SchemaVersion string    `yaml:"schema_version"`
	SessionID     string    `yaml:"session_id"`
	Name          string    `yaml:"name"`
	CreatedAt     time.Time `yaml:"created_at"`
	UpdatedAt     time.Time `yaml:"updated_at"`
	Lifecycle     string    `yaml:"lifecycle"` // "" (active/stopped) or "archived"
	Context       Context   `yaml:"context"`
	Claude        Claude    `yaml:"claude"`
	Tmux          Tmux      `yaml:"tmux"`
}

// Context holds session context information
type Context struct {
	Project string   `yaml:"project"`
	Purpose string   `yaml:"purpose,omitempty"`
	Tags    []string `yaml:"tags,omitempty"`
	Notes   string   `yaml:"notes,omitempty"`
}

// Claude represents Claude session metadata
type Claude struct {
	UUID string `yaml:"uuid,omitempty"` // Claude session UUID (required for resume)
}

// Tmux represents tmux session metadata
type Tmux struct {
	SessionName string `yaml:"session_name"`
}

// ManifestV1 represents the legacy v1 manifest schema (for migration)
type ManifestV1 struct {
	SchemaVersion string     `yaml:"schema_version"`
	SessionID     string     `yaml:"session_id"`
	Status        string     `yaml:"status"`
	CreatedAt     time.Time  `yaml:"created_at"`
	LastActivity  time.Time  `yaml:"last_activity"`
	Worktree      WorktreeV1 `yaml:"worktree"`
	Claude        ClaudeV1   `yaml:"claude"`
	Tmux          TmuxV1     `yaml:"tmux"`
}

// WorktreeV1 represents the working directory for a Claude session (v1)
type WorktreeV1 struct {
	Path string `yaml:"path"`
}

// ClaudeV1 represents Claude session metadata (v1)
type ClaudeV1 struct {
	SessionID       string    `yaml:"session_id"`
	SessionEnvPath  string    `yaml:"session_env_path"`
	FileHistoryPath string    `yaml:"file_history_path"`
	StartedAt       time.Time `yaml:"started_at"`
	LastActivity    time.Time `yaml:"last_activity"`
}

// TmuxV1 represents tmux session metadata (v1)
type TmuxV1 struct {
	SessionName string    `yaml:"session_name"`
	WindowName  string    `yaml:"window_name"`
	CreatedAt   time.Time `yaml:"created_at"`
}

// Status constants (v1 - deprecated)
const (
	StatusActive     = "active"
	StatusDiscovered = "discovered"
	StatusStale      = "stale"
	StatusArchived   = "archived"
)
