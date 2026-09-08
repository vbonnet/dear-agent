//go:build darwin || linux

package steps

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const buildAuthoritySymlinkTargetLimit = 16 << 10

func buildAuthorityWaitOwnershipSourceReadsSupported() bool {
	return true
}

func openBuildAuthorityWaitOwnershipRoot(path string) (*os.File, fs.FileInfo, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return nil, nil, fmt.Errorf("secure source root is not absolute: %s", path)
	}
	current, currentInfo, symlink, err := openBuildAuthorityWaitOwnershipPath(
		unix.AT_FDCWD, string(filepath.Separator), true,
	)
	if err != nil {
		return nil, nil, err
	}
	if symlink {
		return nil, nil, fmt.Errorf("filesystem root is a symbolic link")
	}
	trimmed := strings.TrimPrefix(cleaned, string(filepath.Separator))
	if trimmed == "" {
		return current, currentInfo, nil
	}
	for component := range strings.SplitSeq(trimmed, string(filepath.Separator)) {
		next, nextInfo, componentSymlink, openErr := openBuildAuthorityWaitOwnershipPath(
			int(current.Fd()), component, true,
		)
		closeErr := current.Close()
		if openErr != nil {
			return nil, nil, errors.Join(openErr, closeErr)
		}
		if closeErr != nil {
			if next != nil {
				_ = next.Close()
			}
			return nil, nil, closeErr
		}
		if componentSymlink {
			return nil, nil, fmt.Errorf("secure source root component %q is a symbolic link", component)
		}
		current = next
		currentInfo = nextInfo
	}
	return current, currentInfo, nil
}

func openBuildAuthorityWaitOwnershipAt(
	parent *os.File,
	name string,
) (*os.File, fs.FileInfo, bool, error) {
	if parent == nil || name == "" || name == "." || name == ".." || strings.ContainsRune(name, filepath.Separator) {
		return nil, nil, false, fs.ErrInvalid
	}
	return openBuildAuthorityWaitOwnershipPath(int(parent.Fd()), name, false)
}

func openBuildAuthorityWaitOwnershipPath(
	directoryFD int,
	name string,
	directoryOnly bool,
) (*os.File, fs.FileInfo, bool, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK | buildAuthorityNoFollowOpenFlag
	if directoryOnly {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat(directoryFD, name, flags, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, nil, true, nil
		}
		return nil, nil, false, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		closeErr := unix.Close(fd)
		return nil, nil, false, errors.Join(fs.ErrInvalid, closeErr)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, false, errors.Join(err, file.Close())
	}
	if directoryOnly && !info.IsDir() {
		return nil, nil, false, errors.Join(
			&fs.PathError{Op: "open directory", Path: name, Err: fs.ErrInvalid},
			file.Close(),
		)
	}
	return file, info, false, nil
}

func readBuildAuthorityWaitOwnershipLink(parent *os.File, name string) (string, error) {
	buffer := make([]byte, buildAuthoritySymlinkTargetLimit)
	length, err := unix.Readlinkat(int(parent.Fd()), name, buffer)
	if err != nil {
		return "", err
	}
	if length == len(buffer) {
		return "", fmt.Errorf("symbolic-link target exceeds %d bytes", len(buffer)-1)
	}
	return string(buffer[:length]), nil
}
