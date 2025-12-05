package manifest

import "time"

// Manifest represents a Claude session manifest
type Manifest struct {
	SchemaVersion string    `yaml:"schema_version"`
	SessionID     string    `yaml:"session_id"`
	Status        string    `yaml:"status"`
	CreatedAt     time.Time `yaml:"created_at"`
	LastActivity  time.Time `yaml:"last_activity"`
	Worktree      Worktree  `yaml:"worktree"`
	Claude        Claude    `yaml:"claude"`
	Tmux          Tmux      `yaml:"tmux"`
}

// Worktree represents git worktree metadata
type Worktree struct {
	Path     string `yaml:"path"`
	Branch   string `yaml:"branch"`
	Repo     string `yaml:"repo"`
	Upstream string `yaml:"upstream"`
}

// Claude represents Claude session metadata
type Claude struct {
	SessionID       string    `yaml:"session_id"`
	SessionEnvPath  string    `yaml:"session_env_path"`
	FileHistoryPath string    `yaml:"file_history_path"`
	StartedAt       time.Time `yaml:"started_at"`
	LastActivity    time.Time `yaml:"last_activity"`
}

// Tmux represents tmux session metadata
type Tmux struct {
	SessionName string    `yaml:"session_name"`
	WindowName  string    `yaml:"window_name"`
	CreatedAt   time.Time `yaml:"created_at"`
}

// Status constants
const (
	StatusActive     = "active"
	StatusDiscovered = "discovered"
	StatusStale      = "stale"
	StatusArchived   = "archived"
)

// SchemaVersion is the current manifest schema version
const SchemaVersion = "1.0"
