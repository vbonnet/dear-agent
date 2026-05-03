// Package gclog provides append-only JSONL logging for GC operations.
package gclog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Entry represents a single GC log entry.
type Entry struct {
	Timestamp      time.Time `json:"timestamp"`
	Operation      string    `json:"operation"`
	SessionID      string    `json:"session_id,omitempty"`
	SessionName    string    `json:"session_name,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	SandboxRemoved string    `json:"sandbox_removed,omitempty"`
	WorktreesPaths []string  `json:"worktrees_removed,omitempty"`
	BytesReclaimed int64     `json:"bytes_reclaimed,omitempty"`
	DryRun         bool      `json:"dry_run,omitempty"`
	Error          string    `json:"error,omitempty"`
}

// DefaultPath returns the default gc.jsonl path (~/.agm/logs/gc.jsonl).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agm", "logs", "gc.jsonl"), nil
}

// Logger writes GC entries to a JSONL file.
type Logger struct {
	path string
}

// New creates a Logger that writes to the given path.
// It creates the parent directory if needed.
func New(path string) (*Logger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create gc log dir: %w", err)
	}
	return &Logger{path: path}, nil
}

// NewDefault creates a Logger using the default path.
func NewDefault() (*Logger, error) {
	p, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return New(p)
}

// Log appends an entry to the JSONL file.
func (l *Logger) Log(entry Entry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal gc log entry: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open gc log: %w", err)
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// Path returns the log file path.
func (l *Logger) Path() string {
	return l.path
}

// DirSize computes the total size of a directory tree in bytes.
// Returns 0 if the path doesn't exist or on error.
func DirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
