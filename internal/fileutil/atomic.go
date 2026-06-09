// Package fileutil provides shared file-writing helpers for dear-agent.
//
// It lives at the module root (rather than under any one command's internal
// tree) so every subpackage — wayfinder, agm, engram — can reuse the same
// crash-safe write path instead of reinventing temp-file-then-rename locally.
package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to path atomically: it writes to a temp file in the
// same directory, fsyncs it to durable storage, then renames it over the
// destination. POSIX guarantees rename is atomic, so a reader (or a crash)
// never observes a half-written file — it sees either the old contents or the
// complete new contents, never a truncated mix.
//
// Use this for state, config, and plan files where a partial write on crash
// would corrupt the artifact. It is not meant for append-only logs or streams.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	// Ensure the parent directory exists; CreateTemp below needs it, and the
	// rename target must share a filesystem with the temp file.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	// Create the temp file in the SAME directory as the destination so the
	// final rename stays within one filesystem (cross-device rename is not
	// atomic and fails with EXDEV).
	tmp, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// On any error path, clean up the temp file. Set tmp=nil on success so
	// this becomes a no-op once the rename has happened.
	defer func() {
		if tmp != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	// fsync before rename: without it, the rename can be persisted while the
	// data blocks are not, leaving an empty file after a power loss.
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// CreateTemp makes the file 0600; apply the caller's requested perm before
	// it becomes visible at the destination path.
	if err := os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file into place: %w", err)
	}

	tmp = nil // rename succeeded; suppress deferred cleanup
	return nil
}
