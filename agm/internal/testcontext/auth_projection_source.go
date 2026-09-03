package testcontext

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

type snapshotReadHook func() error

func openApprovedAuthSource(
	hostHome, relativePath string,
	credential bool,
) (*os.File, os.FileInfo, bool, error) {
	root, err := openAuthenticatedRoot(hostHome, "host home", false)
	if err != nil {
		return nil, nil, false, err
	}

	parent := root
	parentPath := "."
	components := strings.Split(filepath.ToSlash(relativePath), "/")
	for _, component := range components[:len(components)-1] {
		if parentPath == "." {
			parentPath = component
		} else {
			parentPath = filepath.Join(parentPath, component)
		}
		next, present, openErr := openAuthenticatedDirectoryAt(parent, component, parentPath)
		closeErr := parent.Close()
		if openErr != nil {
			if next != nil {
				_ = next.Close()
			}
			return nil, nil, false, errors.Join(openErr, sanitizeCloseError("close source directory", parentPath, closeErr))
		}
		if closeErr != nil {
			if next != nil {
				_ = next.Close()
			}
			return nil, nil, false, authPathError("close source directory", parentPath, closeErr)
		}
		if !present {
			return nil, nil, false, nil
		}
		parent = next
	}

	leaf := components[len(components)-1]
	file, info, present, openErr := openAuthenticatedLeafAt(parent, leaf, relativePath, credential)
	closeErr := parent.Close()
	if openErr != nil {
		if file != nil {
			_ = file.Close()
		}
		return nil, nil, false, errors.Join(openErr, sanitizeCloseError("close source directory", relativePath, closeErr))
	}
	if closeErr != nil {
		if file != nil {
			_ = file.Close()
		}
		return nil, nil, false, authPathError("close source directory", relativePath, closeErr)
	}
	return file, info, present, nil
}

func openAuthenticatedRoot(path, label string, exactPrivate bool) (*os.Root, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, authPathError("inspect source directory", label, err)
	}
	if err := validateOwnedDirectoryInfo(before, exactPrivate, "source directory "+label); err != nil {
		return nil, err
	}

	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, authPathError("open source directory", label, err)
	}
	opened, err := root.Stat(".")
	if err != nil {
		_ = root.Close()
		return nil, authPathError("inspect opened source directory", label, err)
	}
	after, err := os.Lstat(path)
	if err != nil {
		_ = root.Close()
		return nil, authPathError("reinspect source directory", label, err)
	}
	for _, info := range []os.FileInfo{opened, after} {
		if err := validateOwnedDirectoryInfo(info, exactPrivate, "source directory "+label); err != nil {
			_ = root.Close()
			return nil, err
		}
	}
	if !sameAuthFileState(before, opened) || !sameAuthFileState(opened, after) {
		_ = root.Close()
		return nil, fmt.Errorf("source directory %s changed while opening", label)
	}
	return root, nil
}

func openAuthenticatedDirectoryAt(parent *os.Root, name, relativePath string) (*os.Root, bool, error) {
	before, err := parent.Lstat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, authPathError("inspect source directory", relativePath, err)
	}
	if err := validateOwnedDirectoryInfo(before, false, "source directory "+relativePath); err != nil {
		return nil, false, err
	}

	next, err := parent.OpenRoot(name)
	if err != nil {
		return nil, false, authPathError("open source directory", relativePath, err)
	}
	opened, err := next.Stat(".")
	if err != nil {
		_ = next.Close()
		return nil, false, authPathError("inspect opened source directory", relativePath, err)
	}
	after, err := parent.Lstat(name)
	if err != nil {
		_ = next.Close()
		return nil, false, authPathError("reinspect source directory", relativePath, err)
	}
	for _, info := range []os.FileInfo{opened, after} {
		if err := validateOwnedDirectoryInfo(info, false, "source directory "+relativePath); err != nil {
			_ = next.Close()
			return nil, false, err
		}
	}
	if !sameAuthFileState(before, opened) || !sameAuthFileState(opened, after) {
		_ = next.Close()
		return nil, false, fmt.Errorf("source directory %s changed while opening", relativePath)
	}
	return next, true, nil
}

func openAuthenticatedLeafAt(
	parent *os.Root,
	name, relativePath string,
	credential bool,
) (*os.File, os.FileInfo, bool, error) {
	before, err := parent.Lstat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, false, nil
		}
		return nil, nil, false, authPathError("inspect auth source", relativePath, err)
	}
	if err := validateAuthLeafInfo(before, credential, relativePath); err != nil {
		return nil, nil, false, err
	}

	file, err := parent.OpenFile(name, os.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, false, authPathError("open auth source", relativePath, err)
	}
	opened, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, false, authPathError("inspect opened auth source", relativePath, err)
	}
	after, err := parent.Lstat(name)
	if err != nil {
		_ = file.Close()
		return nil, nil, false, authPathError("reinspect auth source", relativePath, err)
	}
	for _, info := range []os.FileInfo{opened, after} {
		if err := validateAuthLeafInfo(info, credential, relativePath); err != nil {
			_ = file.Close()
			return nil, nil, false, err
		}
	}
	if !sameAuthFileState(before, opened) || !sameAuthFileState(opened, after) {
		_ = file.Close()
		return nil, nil, false, fmt.Errorf("auth source %s changed while opening", relativePath)
	}
	return file, opened, true, nil
}

func prepareConfigSnapshotWithHook(
	hostHome, relativePath string,
	hook snapshotReadHook,
) ([]byte, bool, error) {
	file, before, present, err := openApprovedAuthSource(hostHome, relativePath, false)
	if err != nil || !present {
		return nil, present, err
	}
	declaredSize := before.Size()
	if declaredSize < 0 {
		_ = file.Close()
		return nil, false, fmt.Errorf("configuration source %s reported a negative size", relativePath)
	}
	if declaredSize > maxAuthConfigBytes {
		_ = file.Close()
		return nil, false, fmt.Errorf("configuration source %s exceeds %d bytes", relativePath, maxAuthConfigBytes)
	}

	first, readErr := readBoundedAuthSnapshot(file)
	middle, statErr := file.Stat()
	if readErr == nil && statErr == nil && hook != nil {
		readErr = hook()
	}
	if readErr == nil && statErr == nil {
		_, readErr = file.Seek(0, io.SeekStart)
	}
	var second []byte
	if readErr == nil && statErr == nil {
		second, readErr = readBoundedAuthSnapshot(file)
	}
	after, afterErr := file.Stat()
	closeErr := file.Close()
	if readErr != nil {
		return nil, false, authPathError("read configuration source", relativePath, readErr)
	}
	if statErr != nil {
		return nil, false, authPathError("reinspect configuration source", relativePath, statErr)
	}
	if afterErr != nil {
		return nil, false, authPathError("reinspect configuration source", relativePath, afterErr)
	}
	if closeErr != nil {
		return nil, false, authPathError("close configuration source", relativePath, closeErr)
	}
	for _, info := range []os.FileInfo{middle, after} {
		if err := validateAuthLeafInfo(info, false, relativePath); err != nil {
			return nil, false, err
		}
	}
	if len(first) > int(maxAuthConfigBytes) || len(second) > int(maxAuthConfigBytes) {
		return nil, false, fmt.Errorf("configuration source %s exceeds %d bytes", relativePath, maxAuthConfigBytes)
	}
	if int64(len(first)) != declaredSize || !bytes.Equal(first, second) ||
		!sameAuthFileState(before, middle) || !sameAuthFileState(middle, after) {
		return nil, false, fmt.Errorf("configuration source %s changed while reading", relativePath)
	}

	currentFile, current, currentPresent, err := openApprovedAuthSource(hostHome, relativePath, false)
	if err != nil {
		return nil, false, err
	}
	if !currentPresent {
		return nil, false, fmt.Errorf("configuration source %s changed while reading", relativePath)
	}
	if err := currentFile.Close(); err != nil {
		return nil, false, authPathError("close configuration source", relativePath, err)
	}
	if !sameAuthFileState(before, current) {
		return nil, false, fmt.Errorf("configuration source %s changed while reading", relativePath)
	}
	return first, true, nil
}

func readBoundedAuthSnapshot(file *os.File) ([]byte, error) {
	return io.ReadAll(io.LimitReader(file, maxAuthConfigBytes+1))
}

func sanitizeCloseError(operation, relativePath string, err error) error {
	if err == nil {
		return nil
	}
	return authPathError(operation, relativePath, err)
}
