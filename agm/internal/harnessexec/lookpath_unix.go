//go:build darwin || linux

package harnessexec

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// resolveExecutableInEnvironment applies Unix PATH lookup to the validated
// environment that will be given to the child. It must not consult the
// executor's ambient PATH: for Codex that ambient state can belong to a
// long-lived tmux server rather than the invoking AGM process.
func resolveExecutableInEnvironment(file string, environment []string) (string, error) {
	if strings.Contains(file, "/") {
		if err := environmentExecutable(file); err != nil {
			return "", &exec.Error{Name: file, Err: err}
		}
		return file, nil
	}
	path := environmentMap(environment)["PATH"]
	baseDir := environmentMap(environment)["PWD"]
	if baseDir == "" {
		baseDir, _ = os.Getwd()
	}
	if !filepath.IsAbs(baseDir) {
		if absoluteBase, err := filepath.Abs(baseDir); err == nil {
			baseDir = absoluteBase
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			dir = "."
		}
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(baseDir, dir)
		}
		candidate := filepath.Join(dir, file)
		if !filepath.IsAbs(candidate) {
			absoluteCandidate, err := filepath.Abs(candidate)
			if err != nil {
				continue
			}
			candidate = absoluteCandidate
		}
		if err := environmentExecutable(candidate); err != nil {
			continue
		}
		return candidate, nil
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

func environmentExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return syscall.EISDIR
	}
	err = unix.Access(path, unix.X_OK)
	if err == nil || (!errors.Is(err, syscall.ENOSYS) && !errors.Is(err, syscall.EPERM)) {
		return err
	}
	if info.Mode()&0111 != 0 {
		return nil
	}
	return fs.ErrPermission
}
