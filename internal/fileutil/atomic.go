// Package fileutil provides shared file-writing helpers for dear-agent.
//
// It lives at the module root (rather than under any one command's internal
// tree) so every subpackage — wayfinder, agm, engram — can reuse the same
// crash-safe write path instead of reinventing temp-file-then-rename locally.
package fileutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type atomicWriteFile interface {
	Write([]byte) (int, error)
	Chmod(os.FileMode) error
	Sync() error
	Close() error
	Name() string
}

type atomicWriteDir interface {
	Sync() error
	Close() error
}

// atomicWriteOps is an internal seam for verifying the durability protocol.
// AtomicWrite deliberately keeps its public interface small; callers do not
// need to know which filesystem operations make the replacement durable.
type atomicWriteOps struct {
	mkdirAll     func(string, os.FileMode) error
	evalSymlinks func(string) (string, error)
	createTemp   func(string, string) (atomicWriteFile, error)
	rename       func(string, string) error
	openDir      func(string) (atomicWriteDir, error)
	remove       func(string) error
}

var osAtomicWriteOps = atomicWriteOps{
	mkdirAll:     os.MkdirAll,
	evalSymlinks: filepath.EvalSymlinks,
	createTemp: func(dir, pattern string) (atomicWriteFile, error) {
		return os.CreateTemp(dir, pattern)
	},
	rename: os.Rename,
	openDir: func(path string) (atomicWriteDir, error) {
		return os.Open(path)
	},
	remove: os.Remove,
}

// AtomicWrite writes data to path atomically: it writes to a temp file in the
// same directory, applies the requested permissions, fsyncs the complete file,
// then renames it over the destination and fsyncs the physical parent
// directory. POSIX guarantees rename is atomic, so a reader never observes a
// half-written file — it sees either the old contents or the complete new
// contents, never a truncated mix. A nil return also means the replacement's
// data, metadata, and directory entry have passed the filesystem durability
// barriers available through os.File.Sync.
//
// Use this for state, config, and plan files where a partial write on crash
// would corrupt the artifact. It is not meant for append-only logs or streams.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	return atomicWrite(path, data, perm, osAtomicWriteOps)
}

func atomicWrite(path string, data []byte, perm os.FileMode, ops atomicWriteOps) (retErr error) {
	physicalDir, destination, err := resolveAtomicWriteDestination(path, ops)
	if err != nil {
		return err
	}
	// Create the temp file in the SAME directory as the destination so the
	// final rename stays within one filesystem (cross-device rename is not
	// atomic and fails with EXDEV).
	tmp, err := ops.createTemp(physicalDir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	publication := atomicWritePublication{file: tmp, path: tmp.Name(), ops: ops, open: true}
	defer publication.cleanup(&retErr)
	if err := publication.writeAndClose(data, perm); err != nil {
		return err
	}
	if err := publication.publish(destination); err != nil {
		return err
	}
	return syncAtomicWriteParent(physicalDir, ops)
}

type atomicWritePublication struct {
	file      atomicWriteFile
	path      string
	ops       atomicWriteOps
	open      bool
	published bool
}

func resolveAtomicWriteDestination(path string, ops atomicWriteOps) (string, string, error) {
	dir := filepath.Dir(path)
	if err := ops.mkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("create dir %s: %w", dir, err)
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", "", fmt.Errorf("resolve absolute parent dir %s: %w", dir, err)
	}
	physicalDir, err := ops.evalSymlinks(absDir)
	if err != nil {
		return "", "", fmt.Errorf("resolve physical parent dir %s: %w", absDir, err)
	}
	return physicalDir, filepath.Join(physicalDir, filepath.Base(path)), nil
}

func (p *atomicWritePublication) cleanup(returnErr *error) {
	// Before publication, surface cleanup errors alongside the primary failure.
	// After rename, never remove the visible destination even if fsync fails.
	if p.published {
		return
	}
	if p.open {
		if err := p.file.Close(); err != nil {
			*returnErr = errors.Join(*returnErr, fmt.Errorf("close temp file during cleanup: %w", err))
		}
	}
	if err := p.ops.remove(p.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		*returnErr = errors.Join(*returnErr, fmt.Errorf("remove temp file during cleanup: %w", err))
	}
}

func (p *atomicWritePublication) writeAndClose(data []byte, perm os.FileMode) error {
	n, err := p.file.Write(data)
	if err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("write temp file: wrote %d of %d bytes: %w", n, len(data), io.ErrShortWrite)
	}
	if err := p.file.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := p.file.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}
	err = p.file.Close()
	p.open = false
	if err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	return nil
}

func (p *atomicWritePublication) publish(destination string) error {
	if err := p.ops.rename(p.path, destination); err != nil {
		return fmt.Errorf("rename temp file into place: %w", err)
	}
	p.published = true
	return nil
}

func syncAtomicWriteParent(physicalDir string, ops atomicWriteOps) error {
	parent, err := ops.openDir(physicalDir)
	if err != nil {
		return fmt.Errorf("open physical parent dir for sync: %w", err)
	}
	syncErr := parent.Sync()
	closeErr := parent.Close()
	var errs []error
	if syncErr != nil {
		errs = append(errs, fmt.Errorf("sync physical parent dir: %w", syncErr))
	}
	if closeErr != nil {
		errs = append(errs, fmt.Errorf("close physical parent dir: %w", closeErr))
	}
	return errors.Join(errs...)
}
