package supervisorheartbeat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const heartbeatFilename = "heartbeat.json"

// Record is the authoritative heartbeat state for one supervisor.
type Record struct {
	ID          string    `json:"id"`
	PrimaryFor  string    `json:"primary_for,omitempty"`
	TertiaryFor string    `json:"tertiary_for,omitempty"`
	LastBeatUTC time.Time `json:"last_beat_utc"`
	PID         int       `json:"pid,omitempty"`
	TmuxSession string    `json:"tmux_session,omitempty"`
}

// Store persists authoritative heartbeat records beneath one state root.
type Store struct {
	root string
}

// New returns a Store rooted at root.
func New(root string) Store {
	return Store{root: root}
}

// Read returns the latest heartbeat for id. A missing record is not an error.
func (s Store) Read(id string) (*Record, error) {
	path := filepath.Join(s.root, id, heartbeatFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read supervisor heartbeat %q: %w", id, err)
	}

	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal supervisor heartbeat %q: %w", id, err)
	}
	return &rec, nil
}

// Write atomically replaces rec's authoritative heartbeat file.
func (s Store) Write(rec Record) error {
	dir := filepath.Join(s.root, rec.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create supervisor heartbeat directory %q: %w", rec.ID, err)
	}

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal supervisor heartbeat %q: %w", rec.ID, err)
	}

	path := filepath.Join(dir, heartbeatFilename)
	tmp, err := os.CreateTemp(dir, "."+heartbeatFilename+".*.tmp")
	if err != nil {
		return fmt.Errorf("create supervisor heartbeat temporary file %q: %w", rec.ID, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write supervisor heartbeat temporary file %q: %w", rec.ID, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close supervisor heartbeat temporary file %q: %w", rec.ID, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace supervisor heartbeat %q: %w", rec.ID, err)
	}
	return nil
}
