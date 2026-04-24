package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FileState is a file-backed implementation of the State interface. It
// persists Snapshots as JSON using an atomic temp-file + rename pattern so
// a crash mid-write never leaves a corrupt checkpoint file.
//
// Usage:
//
//	r := workflow.NewRunner(ai)
//	r.State = &workflow.FileState{Path: "/tmp/mywf.snap.json"}
//	rep, err := r.Run(ctx, wf, inputs)
//
// To resume after a crash:
//
//	r.State = &workflow.FileState{Path: "/tmp/mywf.snap.json"}
//	rep, err := r.Resume(ctx, wf, r.State)
type FileState struct {
	// Path is the file where the snapshot is persisted. The file is
	// created atomically; its parent directory must exist.
	Path string
}

// Save marshals snap to JSON and writes it atomically to fs.Path.
// The write goes to a sibling temp file; on success the temp file is
// renamed over Path. A crash between the write and the rename leaves
// the previous snapshot intact.
func (fs *FileState) Save(_ context.Context, snap Snapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("state_file: marshal: %w", err)
	}

	dir := filepath.Dir(fs.Path)
	tmp, err := os.CreateTemp(dir, ".workflow-snap-*.json.tmp")
	if err != nil {
		return fmt.Errorf("state_file: create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, werr := tmp.Write(data); werr != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("state_file: write temp: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("state_file: close temp: %w", cerr)
	}
	if rerr := os.Rename(tmpName, fs.Path); rerr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("state_file: rename: %w", rerr)
	}
	return nil
}

// Load reads and unmarshals the snapshot from fs.Path. Returns (nil, nil)
// if the file does not exist (i.e. no checkpoint yet).
func (fs *FileState) Load(_ context.Context) (*Snapshot, error) {
	data, err := os.ReadFile(fs.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("state_file: read: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("state_file: unmarshal: %w", err)
	}
	return &snap, nil
}
