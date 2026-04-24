// Package delegation tracks outbound task delegations between AGM sessions.
//
// When a session sends an implementation task to another session via
// "agm send msg", a delegation record is created. Sessions cannot archive
// cleanly while they have unresolved delegations — this prevents premature
// exits where research sessions archive before verifying delegated work.
//
// Delegation records are stored as JSONL files per-session at:
//
//	~/.agm/delegations/{session-name}.jsonl
package delegation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Status constants for delegation lifecycle
const (
	StatusPending   = "pending"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)

// Delegation represents a tracked outbound task delegation.
type Delegation struct {
	MessageID   string    `json:"message_id"`
	From        string    `json:"from"`
	To          string    `json:"to"`
	TaskSummary string    `json:"task_summary"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}

// Tracker manages delegation records for sessions.
type Tracker struct {
	dir string
	mu  sync.Mutex
}

// NewTracker creates a Tracker that stores records under dir.
// dir is typically ~/.agm/delegations/.
func NewTracker(dir string) (*Tracker, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("delegation: create dir: %w", err)
	}
	return &Tracker{dir: dir}, nil
}

// DefaultDir returns the default delegations directory (~/.agm/delegations).
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agm", "delegations"), nil
}

// Record appends a new pending delegation for the sender session.
func (t *Tracker) Record(d *Delegation) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	d.Status = StatusPending
	d.CreatedAt = time.Now().UTC()

	return t.appendEntry(d.From, d)
}

// Resolve marks a delegation as completed or cancelled.
func (t *Tracker) Resolve(sessionName, messageID, status string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries, err := t.readEntries(sessionName)
	if err != nil {
		return err
	}

	found := false
	now := time.Now().UTC()
	for i, e := range entries {
		if e.MessageID == messageID {
			entries[i].Status = status
			entries[i].ResolvedAt = &now
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("delegation: message %s not found for session %s", messageID, sessionName)
	}

	return t.writeEntries(sessionName, entries)
}

// Pending returns all unresolved delegations for a session.
func (t *Tracker) Pending(sessionName string) ([]*Delegation, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	entries, err := t.readEntries(sessionName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var pending []*Delegation
	for _, e := range entries {
		if e.Status == StatusPending {
			pending = append(pending, e)
		}
	}
	return pending, nil
}

// filePath returns the JSONL file path for a session.
func (t *Tracker) filePath(sessionName string) string {
	return filepath.Join(t.dir, sessionName+".jsonl")
}

// appendEntry appends a single delegation entry to the session's JSONL file.
func (t *Tracker) appendEntry(sessionName string, d *Delegation) error {
	f, err := os.OpenFile(t.filePath(sessionName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("delegation: open file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(d)
	if err != nil {
		return fmt.Errorf("delegation: marshal: %w", err)
	}
	data = append(data, '\n')

	_, err = f.Write(data)
	return err
}

// readEntries reads all delegation entries for a session.
func (t *Tracker) readEntries(sessionName string) ([]*Delegation, error) {
	data, err := os.ReadFile(t.filePath(sessionName))
	if err != nil {
		return nil, err
	}

	var entries []*Delegation
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var d Delegation
		if err := json.Unmarshal(line, &d); err != nil {
			continue // skip malformed entries
		}
		entries = append(entries, &d)
	}
	return entries, nil
}

// writeEntries rewrites the entire JSONL file for a session (used for updates).
func (t *Tracker) writeEntries(sessionName string, entries []*Delegation) error {
	f, err := os.CreateTemp(t.dir, "delegation-*.tmp")
	if err != nil {
		return fmt.Errorf("delegation: create temp: %w", err)
	}
	tmpPath := f.Name()

	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			f.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("delegation: marshal: %w", err)
		}
		data = append(data, '\n')
		if _, err := f.Write(data); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return err
		}
	}
	f.Close()

	return os.Rename(tmpPath, t.filePath(sessionName))
}

// splitLines splits byte data into lines.
func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
