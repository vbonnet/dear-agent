package fsguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Violation is a single blocked write, recorded for DEAR-retro analysis: it is
// the raw material for spotting which protected paths agents keep trying to
// write to, and why. One Violation is one JSON Lines record.
type Violation struct {
	// Time is RFC3339 UTC; set by the Recorder when empty.
	Time string `json:"time"`
	// Tool is the originating tool name (Edit, Write, MultiEdit, Bash).
	Tool string `json:"tool"`
	// Path is the raw write target for file tools (empty for Bash).
	Path string `json:"path,omitempty"`
	// Command is the raw command for the Bash tool (empty for file tools).
	Command string `json:"command,omitempty"`
	// CWD is the working directory the tool ran in.
	CWD string `json:"cwd,omitempty"`
	// Resolved is the absolute, symlink-resolved target classification judged
	// (file tools only). It makes symlink-escape attempts visible in the log.
	Resolved string `json:"resolved,omitempty"`
	// Reason is the policy guidance message explaining the block.
	Reason string `json:"reason"`
}

// Recorder persists violations. Implementations must be safe for concurrent use
// because multiple hook processes can record at once.
type Recorder interface {
	Record(Violation) error
}

// NopRecorder discards violations. Used when logging is disabled.
type NopRecorder struct{}

// Record implements Recorder.
func (NopRecorder) Record(Violation) error { return nil }

// FileRecorder appends violations as JSON Lines to a file, creating the parent
// directory on first write. Each record is marshalled to a single line and
// written with one O_APPEND write so concurrent hook processes interleave whole
// records rather than corrupting each other.
type FileRecorder struct {
	Path string

	mu         sync.Mutex
	now        func() time.Time // injectable clock for tests
	dirCreated bool             // memoizes the one-time parent MkdirAll
}

// NewFileRecorder returns a FileRecorder writing to path.
func NewFileRecorder(path string) *FileRecorder {
	return &FileRecorder{Path: path, now: time.Now}
}

// Record appends v to the log file. A best-effort timestamp is stamped when v
// has none. Errors (e.g. unwritable directory) are returned but, by convention,
// callers treat logging failures as non-fatal — enforcement does not depend on
// the log succeeding.
func (r *FileRecorder) Record(v Violation) error {
	if v.Time == "" {
		clock := r.now
		if clock == nil {
			clock = time.Now
		}
		v.Time = clock().UTC().Format(time.RFC3339)
	}
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.dirCreated {
		if dir := filepath.Dir(r.Path); dir != "" {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return err
			}
		}
		r.dirCreated = true
	}
	f, err := os.OpenFile(r.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(line)
	return err
}
