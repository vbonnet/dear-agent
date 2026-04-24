// Package context provides session context persistence for Claude Code sessions.
// It tracks active workers, open beads, current operations, and task lists to
// prevent context loss when sessions are interrupted or forget their state.
package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WorkerInfo represents a worker session spawned by a coordinator.
type WorkerInfo struct {
	SessionName string    `json:"session_name"`
	Status      string    `json:"status"` // "active", "completed", "failed"
	Task        string    `json:"task"`   // what the worker is doing
	StartedAt   time.Time `json:"started_at"`
}

// BeadInfo represents a bead being tracked by the session.
type BeadInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`   // "open", "in_progress", "closed"
	Priority string `json:"priority"` // "P0", "P1", "P2", "P3"
}

// SessionContext holds the full operational state of a Claude Code session.
type SessionContext struct {
	SessionID     string       `json:"session_id"`
	SessionName   string       `json:"session_name,omitempty"`
	UpdatedAt     time.Time    `json:"updated_at"`
	ActiveWorkers []WorkerInfo `json:"active_workers,omitempty"`
	OpenBeads     []BeadInfo   `json:"open_beads,omitempty"`
	CurrentSwarm  string       `json:"current_swarm,omitempty"`
	CurrentPhase  string       `json:"current_phase,omitempty"`
	ActiveTasks   []string     `json:"active_tasks,omitempty"`
	LastOperation string       `json:"last_operation,omitempty"`
	Notes         []string     `json:"notes,omitempty"`
}

// maxNotes is the cap on free-form context notes.
const maxNotes = 20

// DefaultContextDir returns the default directory for session context files.
func DefaultContextDir() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".claude", "session-context")
}

// Path returns the full file path for a session context file.
func Path(dir, sessionID string) string {
	return filepath.Join(dir, sessionID+".json")
}

// LoadContext reads a session context from disk.
// Returns an empty context (never nil) if the file doesn't exist or is invalid.
// This fail-open behavior is required for non-blocking hooks.
func LoadContext(path string) (*SessionContext, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return &SessionContext{}, nil //nolint:nilerr // fail-safe: return empty on error
	}
	var ctx SessionContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return &SessionContext{}, nil //nolint:nilerr // fail-safe: return empty on error
	}
	return &ctx, nil
}

// SaveContext writes a session context to disk, updating the timestamp.
// Creates parent directories if needed.
func SaveContext(path string, ctx *SessionContext) error {
	ctx.UpdatedAt = time.Now()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("creating context directory: %w", err)
	}

	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling context: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing context file: %w", err)
	}
	return nil
}

// AddWorker adds an active worker to the session context.
func (ctx *SessionContext) AddWorker(name, task string) {
	ctx.ActiveWorkers = append(ctx.ActiveWorkers, WorkerInfo{
		SessionName: name,
		Status:      "active",
		Task:        task,
		StartedAt:   time.Now(),
	})
}

// RemoveWorker marks a worker as completed by session name.
func (ctx *SessionContext) RemoveWorker(name string) {
	for i := range ctx.ActiveWorkers {
		if ctx.ActiveWorkers[i].SessionName == name {
			ctx.ActiveWorkers[i].Status = "completed"
			return
		}
	}
}

// UpdateBeads replaces the open beads list.
func (ctx *SessionContext) UpdateBeads(beads []BeadInfo) {
	ctx.OpenBeads = beads
}

// AddNote adds a free-form context note, capping at maxNotes.
// When at capacity, the oldest note is dropped.
func (ctx *SessionContext) AddNote(note string) {
	if len(ctx.Notes) >= maxNotes {
		ctx.Notes = ctx.Notes[1:]
	}
	ctx.Notes = append(ctx.Notes, note)
}

// Summary returns a human-readable summary of the session context,
// suitable for stderr output.
func (ctx *SessionContext) Summary() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Session: %s", ctx.SessionID)
	if ctx.SessionName != "" {
		fmt.Fprintf(&b, " (%s)", ctx.SessionName)
	}
	b.WriteString("\n")

	if ctx.CurrentSwarm != "" {
		fmt.Fprintf(&b, "Swarm: %s", ctx.CurrentSwarm)
		if ctx.CurrentPhase != "" {
			fmt.Fprintf(&b, " | Phase: %s", ctx.CurrentPhase)
		}
		b.WriteString("\n")
	}

	if ctx.LastOperation != "" {
		fmt.Fprintf(&b, "Last: %s\n", ctx.LastOperation)
	}

	activeCount := 0
	for _, w := range ctx.ActiveWorkers {
		if w.Status == "active" {
			activeCount++
		}
	}
	if activeCount > 0 {
		fmt.Fprintf(&b, "Workers: %d active\n", activeCount)
		for _, w := range ctx.ActiveWorkers {
			if w.Status == "active" {
				fmt.Fprintf(&b, "  - %s: %s\n", w.SessionName, w.Task)
			}
		}
	}

	if len(ctx.OpenBeads) > 0 {
		fmt.Fprintf(&b, "Beads: %d tracked\n", len(ctx.OpenBeads))
		for _, bd := range ctx.OpenBeads {
			fmt.Fprintf(&b, "  - %s [%s] %s (%s)\n", bd.ID, bd.Priority, bd.Title, bd.Status)
		}
	}

	if len(ctx.ActiveTasks) > 0 {
		fmt.Fprintf(&b, "Tasks: %d\n", len(ctx.ActiveTasks))
		for _, t := range ctx.ActiveTasks {
			fmt.Fprintf(&b, "  - %s\n", t)
		}
	}

	return b.String()
}

// PruneOldContexts deletes context files in dir older than maxAge,
// based on the file's modification time.
func PruneOldContexts(dir string, maxAge time.Duration) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading context directory: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, entry.Name())
			if removeErr := os.Remove(path); removeErr != nil {
				return fmt.Errorf("removing old context %s: %w", entry.Name(), removeErr)
			}
		}
	}
	return nil
}
