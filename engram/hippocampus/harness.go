package hippocampus

import (
	"context"
	"time"
)

// HarnessAdapter discovers sessions and reads transcripts from an AI coding harness.
// Implementations exist for Claude Code, Codex CLI, Gemini CLI, etc.
type HarnessAdapter interface {
	// Name returns the harness identifier (e.g., "claude-code").
	Name() string

	// DiscoverSessions finds sessions for a project since a given time.
	// Returns sessions sorted by start time (oldest first).
	DiscoverSessions(ctx context.Context, projectPath string, since time.Time) ([]SessionInfo, error)

	// ReadTranscript reads a session's user+assistant text content.
	// Tool results and progress events are skipped to reduce noise.
	ReadTranscript(ctx context.Context, session SessionInfo) (string, error)

	// GetMemoryDir returns the auto-memory directory for a project.
	// For Claude Code: ~/.claude/projects/<project-key>/memory/
	GetMemoryDir(projectPath string) (string, error)
}

// SessionInfo describes a discovered session.
type SessionInfo struct {
	ID        string
	StartTime time.Time
	EndTime   time.Time
	Project   string
	FilePath  string // path to session transcript file
}
