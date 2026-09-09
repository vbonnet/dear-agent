// Package gclog provides append-only JSONL logging for GC operations.
package gclog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/pkg/diskledger"
)

// Entry represents a single GC log entry.
type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Operation string    `json:"operation"`
	// Source names who ran this GC, when the runner declares itself. It exists
	// because a reader that infers "the scheduled reaper is alive" from any
	// completion record cannot tell a scheduled sweep from one the reader's own
	// remediation just triggered — and a watchdog that answers its own liveness
	// question manufactures proof of life for a schedule that is dead. Empty
	// means an undeclared runner (a manual invocation, or an agm predating the
	// tag); readers must not treat empty as "scheduled".
	Source         string   `json:"source,omitempty"`
	SessionID      string   `json:"session_id,omitempty"`
	SessionName    string   `json:"session_name,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	SandboxRemoved string   `json:"sandbox_removed,omitempty"`
	WorktreesPaths []string `json:"worktrees_removed,omitempty"`
	BytesReclaimed int64    `json:"bytes_reclaimed,omitempty"`
	DryRun         bool     `json:"dry_run,omitempty"`
	Error          string   `json:"error,omitempty"`
	// Errors counts sub-operations that were attempted and failed within a
	// single sweep. A sweep can return success overall while individual
	// deletions fail, so readers that treat a completion record as evidence of
	// health must be able to see that count rather than infer it from Reason.
	Errors int `json:"errors,omitempty"`
	// ProbeFailures counts entries a sweep could not EVALUATE (a safety check
	// like lsof or the mount table itself failed to run) rather than entries
	// it correctly found in use. A sweep can report Errors == 0 while every
	// entry was actually unevaluated, which reads as healthy unless this
	// count is checked too.
	ProbeFailures int `json:"probe_failures,omitempty"`
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

// DirSize computes the disk a directory tree actually occupies, in bytes.
// Returns 0 if the path doesn't exist or on error.
//
// This reports ALLOCATED blocks over distinct inodes, not the sum of file
// lengths. The distinction matters because DirSize feeds Entry.BytesReclaimed,
// which the reclaim-health check reads to decide whether the collector is
// doing anything at all. Summing lengths charges a hard-linked or reflinked
// tree once per name, so a sweep would report more bytes reclaimed than the
// filesystem returned -- overstating exactly in the direction that makes a
// broken collector look healthy. Sandboxes here are provisioned with APFS
// clonefile, so this is the common case and not a corner one.
func DirSize(path string) int64 {
	// A partial walk yields a lower bound rather than an error: the callers
	// are all best-effort accounting and a hard failure here would abort a
	// sweep over one unreadable file.
	u, _ := diskledger.Measure(path)
	return u.AllocatedBytes
}
