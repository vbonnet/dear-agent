//go:build !windows

package override

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func prepareGrantDir(dir string) error {
	if !enforceOperatorOwnership {
		return os.MkdirAll(dir, 0o700)
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("%w: %s must be created by the operator-authorized installer", ErrGrantUntrusted, dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return validateRootOwnedPath(dir, true)
}

func grantFileMode() os.FileMode {
	if !enforceOperatorOwnership {
		return 0o600
	}
	// Every AGM process must be able to read the grant, while only root may
	// replace it. The directory and file ownership checks enforce that split.
	return 0o644
}

func validateGrantPath(path string) error {
	if !enforceOperatorOwnership {
		_, err := os.Lstat(path)
		return err
	}
	if err := validateRootOwnedPath(GrantDir(), true); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrGrantUntrusted, err)
	}
	if err := validateRootOwnedPath(path, false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrGrantUntrusted, err)
	}
	return nil
}

func validateRootOwnedPath(path string, wantDir bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", path)
	}
	if wantDir && !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if !wantDir && !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("cannot verify owner of %s", path)
	}
	if stat.Uid != 0 {
		return fmt.Errorf("%s is owned by uid %d, want root", path, stat.Uid)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s is writable by group or others (mode %04o)", path, info.Mode().Perm())
	}
	return nil
}
