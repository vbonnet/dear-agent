// Package binstamp identifies which build of an executable is currently
// running, so a long-lived daemon can notice that it has been replaced on
// disk and exit for a supervisor to restart into the new image.
//
// A KeepAlive LaunchAgent restarts a daemon that crashes, never one whose
// binary was merely reinstalled: replacing the file leaves the running
// process on the old inode indefinitely. That is how `agm watch-stalled`
// spent eight days serving pre-fix completion routing while main, and
// ~/go/bin/agm, both carried the fix.
package binstamp

import (
	"fmt"
	"os"
	"time"
)

// Stamp identifies one build of a file on disk. Size and modification time
// come from the file, inode from the filesystem: `go install` writes a
// temporary file and renames it over the target, so the path keeps
// resolving while the inode behind it changes. Comparing inode alone would
// also miss an in-place rewrite, so all three are compared.
type Stamp struct {
	Size    int64
	ModTime time.Time
	Inode   uint64
}

// Of stamps the file at path.
func Of(path string) (Stamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Stamp{}, fmt.Errorf("stat %s: %w", path, err)
	}
	stamp := Stamp{Size: info.Size(), ModTime: info.ModTime(), Inode: getInode(info)}
	return stamp, nil
}

// OfRunning stamps the currently running executable and reports its path.
func OfRunning() (Stamp, string, error) {
	path, err := os.Executable()
	if err != nil {
		return Stamp{}, "", fmt.Errorf("resolve executable: %w", err)
	}
	stamp, err := Of(path)
	if err != nil {
		return Stamp{}, path, err
	}
	return stamp, path, nil
}

// Differs reports whether other is a different build from s.
func (s Stamp) Differs(other Stamp) bool {
	return s.Size != other.Size ||
		!s.ModTime.Equal(other.ModTime) ||
		s.Inode != other.Inode
}

// Watcher answers one question on each poll: has the executable I am
// running been replaced since I started?
//
// A failed stat is deliberately not a replacement. An install is not
// atomic from the observer's side, and the file can be absent or
// unreadable for the moment between the old build being removed and the
// new one appearing. Treating that instant as a replacement would exit the
// daemon so it could be restarted onto a binary that is not there yet,
// which is a worse outcome than waiting one more poll for a stat that
// succeeds.
type Watcher struct {
	path     string
	baseline Stamp
	ok       bool
}

// NewWatcher stamps the running executable as the baseline. A watcher that
// could not read its own executable never reports a replacement, so an
// unstampable binary leaves the daemon running rather than restarting it
// on every poll.
func NewWatcher() *Watcher {
	stamp, path, err := OfRunning()
	if err != nil {
		return &Watcher{path: path}
	}
	return &Watcher{path: path, baseline: stamp, ok: true}
}

// Path is the executable being watched.
func (w *Watcher) Path() string { return w.path }

// Replaced reports whether the executable now on disk is a different build
// from the one this process started as.
func (w *Watcher) Replaced() bool {
	if !w.ok {
		return false
	}
	current, err := Of(w.path)
	if err != nil {
		return false
	}
	return w.baseline.Differs(current)
}
