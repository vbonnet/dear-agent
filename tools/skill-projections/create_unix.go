//go:build darwin || linux

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type repositoryRoot struct {
	directory    *os.File
	path         string
	rootIdentity string
}

type markerScanEntry struct {
	name        string
	isDirectory bool
	isRegular   bool
}

type markerScanner struct {
	limits      markerScanLimits
	started     time.Time
	files       int
	directories int
	bytes       int64
	marked      []string
}

func openRepositoryRoot(path string) (*repositoryRoot, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open repository root without following a symlink: %w", err)
	}
	directory := os.NewFile(uintptr(fd), path)
	if directory == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("adopt repository root descriptor")
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = directory.Close()
		return nil, fmt.Errorf("stat repository root descriptor: %w", err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		_ = directory.Close()
		return nil, fmt.Errorf("repository root descriptor is not a directory")
	}
	identity := statIdentity(&stat)
	return &repositoryRoot{
		directory:    directory,
		path:         path,
		rootIdentity: identity,
	}, nil
}

func statIdentity(stat *unix.Stat_t) string {
	return fmt.Sprintf("dev=%v,ino=%v", stat.Dev, stat.Ino)
}

func (root *repositoryRoot) close() error {
	if root == nil || root.directory == nil {
		return nil
	}
	directory := root.directory
	root.directory = nil
	return directory.Close()
}

func (root *repositoryRoot) identity() string {
	if root == nil {
		return "unavailable"
	}
	return root.rootIdentity
}

func (root *repositoryRoot) verifyPathIdentity(path, label string) error {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return fmt.Errorf("bind %s to retained caller root %s: inspect path identity without following a final symlink: %w", label, root.identity(), err)
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("bind %s to retained caller root %s: reported path is not a directory", label, root.identity())
	}
	reportedIdentity := statIdentity(&stat)
	if reportedIdentity != root.rootIdentity {
		return fmt.Errorf("bind %s identity %s to retained caller root identity %s: identities do not match", label, reportedIdentity, root.identity())
	}
	return nil
}

func (root *repositoryRoot) duplicateDirectory() (*os.File, error) {
	if root == nil || root.directory == nil {
		return nil, fmt.Errorf("retained repository root is closed")
	}
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Openat(int(root.directory.Fd()), ".", flags, 0)
	if err != nil {
		return nil, fmt.Errorf("duplicate retained repository root %s without reopening %s: %w", root.identity(), root.path, err)
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("stat duplicate retained repository root: %w", err)
	}
	identity := statIdentity(&stat)
	if identity != root.rootIdentity {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("duplicate repository root identity changed from %s to %s", root.identity(), identity)
	}
	directory := os.NewFile(uintptr(fd), root.path)
	if directory == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("adopt duplicate retained repository root descriptor")
	}
	return directory, nil
}

func (root *repositoryRoot) readRegular(relative, label string, limit int64) ([]byte, error) {
	return root.readRegularAfterParents(relative, label, limit, nil)
}

func (root *repositoryRoot) readRegularAfterParents(
	relative, label string,
	limit int64,
	afterParents func() error,
) ([]byte, error) {
	parent, name, closeDirectories, err := root.openParent(relative, false)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	defer closeDirectories()
	if afterParents != nil {
		if err := afterParents(); err != nil {
			return nil, fmt.Errorf("read %s: race hook: %w", label, err)
		}
	}
	data, err := readRegularFileAt(parent, name, label, limit)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", label, err)
	}
	return data, nil
}

func (root *repositoryRoot) createExclusive(relative string, content []byte) (bool, error) {
	if len(content) > maxDelegateBytes {
		return false, fmt.Errorf("delegate exceeds %d-byte limit", maxDelegateBytes)
	}
	parent, name, closeDirectories, err := root.openParent(relative, true)
	if err != nil {
		return false, err
	}
	defer closeDirectories()

	flags := unix.O_WRONLY | unix.O_CREAT | unix.O_EXCL | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Openat(int(parent.Fd()), name, flags, 0o644)
	if err != nil {
		return false, fmt.Errorf("exclusively create regular delegate: %w", err)
	}
	file := os.NewFile(uintptr(fd), relative)
	if file == nil {
		_ = unix.Close(fd)
		return true, fmt.Errorf("adopt exclusively created delegate descriptor")
	}
	return true, finishDelegateFile(file, content)
}

func (root *repositoryRoot) targetAbsent(relative string) (bool, error) {
	cleanRelative, err := cleanRootRelativePath(relative)
	if err != nil {
		return false, err
	}
	directory, err := root.duplicateDirectory()
	if err != nil {
		return false, err
	}
	directories := []*os.File{directory}
	defer func() {
		for _, opened := range slices.Backward(directories) {
			_ = opened.Close()
		}
	}()

	segments := strings.Split(cleanRelative, string(filepath.Separator))
	for _, segment := range segments[:len(segments)-1] {
		next, openErr := openExistingDirectoryWithoutSymlink(directories[len(directories)-1], segment)
		if errors.Is(openErr, fs.ErrNotExist) {
			return true, nil
		}
		if openErr != nil {
			return false, openErr
		}
		directories = append(directories, next)
	}
	var stat unix.Stat_t
	err = unix.Fstatat(
		int(directories[len(directories)-1].Fd()),
		segments[len(segments)-1],
		&stat,
		unix.AT_SYMLINK_NOFOLLOW,
	)
	if errors.Is(err, fs.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect final target without following a symlink: %w", err)
	}
	return false, nil
}

func (root *repositoryRoot) openParent(relative string, create bool) (*os.File, string, func(), error) {
	cleanRelative, err := cleanRootRelativePath(relative)
	if err != nil {
		return nil, "", func() {}, err
	}
	directory, err := root.duplicateDirectory()
	if err != nil {
		return nil, "", func() {}, err
	}
	directories := []*os.File{directory}
	closeDirectories := closeDirectoryFiles(directories)
	segments := strings.Split(cleanRelative, string(filepath.Separator))
	for _, segment := range segments[:len(segments)-1] {
		var next *os.File
		if create {
			next, err = openOrCreateDirectoryWithoutSymlink(directories[len(directories)-1], segment)
		} else {
			next, err = openExistingDirectoryWithoutSymlink(directories[len(directories)-1], segment)
		}
		if err != nil {
			closeDirectories()
			return nil, "", func() {}, err
		}
		directories = append(directories, next)
		closeDirectories = closeDirectoryFiles(directories)
	}
	return directories[len(directories)-1], segments[len(segments)-1], closeDirectories, nil
}

func cleanRootRelativePath(relative string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path must stay beneath the repository root")
	}
	return cleaned, nil
}

func openExistingDirectoryWithoutSymlink(parent *os.File, name string) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Openat(int(parent.Fd()), name, flags, 0)
	if err != nil {
		return nil, fmt.Errorf("refusing to traverse symlinked parent or non-directory %q; open without following symlinks: %w", name, err)
	}
	directory := os.NewFile(uintptr(fd), name)
	if directory == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("adopt existing parent %q descriptor", name)
	}
	return directory, nil
}

func openOrCreateDirectoryWithoutSymlink(parent *os.File, name string) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW
	fd, err := unix.Openat(int(parent.Fd()), name, flags, 0)
	if errors.Is(err, fs.ErrNotExist) {
		if mkdirErr := unix.Mkdirat(int(parent.Fd()), name, 0o755); mkdirErr != nil && !errors.Is(mkdirErr, fs.ErrExist) {
			return nil, fmt.Errorf("create delegate parent %q: %w", name, mkdirErr)
		}
		fd, err = unix.Openat(int(parent.Fd()), name, flags, 0)
	}
	if err != nil {
		return nil, fmt.Errorf("refusing to traverse symlinked parent or non-directory %q; open without following symlinks: %w", name, err)
	}
	directory := os.NewFile(uintptr(fd), name)
	if directory == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("adopt delegate parent %q descriptor", name)
	}
	return directory, nil
}

func readRegularFileAt(parent *os.File, name, label string, limit int64) ([]byte, error) {
	if limit < 0 {
		return nil, fmt.Errorf("invalid negative byte limit")
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	fd, err := unix.Openat(int(parent.Fd()), name, flags, 0)
	if err != nil {
		return nil, fmt.Errorf("open regular file without following a symlink: %w", err)
	}
	file := os.NewFile(uintptr(fd), label)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("adopt regular file descriptor")
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat opened file: %w", statErr)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("file must be a regular file")
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d-byte limit", limit)
	}
	return data, nil
}

func (root *repositoryRoot) scanGeneratedMarkers(limits markerScanLimits) ([]string, error) {
	if limits.now == nil {
		limits.now = func(_ string) time.Time { return time.Now() }
	}
	if limits.maxFiles <= 0 {
		return nil, fmt.Errorf("marker scan file limit must be positive")
	}
	if limits.maxDirectories <= 0 {
		return nil, fmt.Errorf("marker scan directory limit must be positive")
	}
	if limits.maxDepth < 0 {
		return nil, fmt.Errorf("marker scan depth limit must be non-negative")
	}
	if limits.maxBytes <= 0 {
		return nil, fmt.Errorf("marker scan byte limit must be positive")
	}
	if limits.elapsedBudget <= 0 {
		return nil, fmt.Errorf("marker scan checked elapsed-time budget must be positive")
	}
	scanner := markerScanner{
		limits:      limits,
		started:     limits.now("start marker scan"),
		directories: 1,
	}
	directory, err := root.duplicateDirectory()
	if err != nil {
		return nil, err
	}
	if err := scanner.checkTime("after opening the retained scan root"); err != nil {
		return nil, errors.Join(err, directory.Close())
	}
	scanErr := scanner.scanDirectory(directory, "", 0)
	closeErr := directory.Close()
	if scanErr != nil || closeErr != nil {
		return nil, errors.Join(scanErr, closeErr)
	}
	sort.Strings(scanner.marked)
	if err := scanner.checkTime("after sorting marked paths and closing the retained scan root"); err != nil {
		return nil, err
	}
	return scanner.marked, nil
}

func (scanner *markerScanner) scanDirectory(directory *os.File, relative string, depth int) error {
	if err := scanner.checkTime("before scanning directory " + scanPathLabel(relative)); err != nil {
		return err
	}
	entries, err := scanner.readDirectoryEntries(directory, relative, depth)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := scanner.scanEntry(directory, relative, depth, entry); err != nil {
			return err
		}
	}
	return scanner.checkTime("after scanning directory " + scanPathLabel(relative))
}

func (scanner *markerScanner) scanEntry(directory *os.File, relative string, depth int, entry markerScanEntry) error {
	if err := scanner.checkTime("before scanning entry " + joinScanPath(relative, entry.name)); err != nil {
		return err
	}
	path := joinScanPath(relative, entry.name)
	if entry.isDirectory {
		child, err := openExistingDirectoryWithoutSymlink(directory, entry.name)
		if err != nil {
			return fmt.Errorf("scan directory %s: %w", path, err)
		}
		scanErr := scanner.scanDirectory(child, path, depth+1)
		closeErr := child.Close()
		return errors.Join(scanErr, closeErr)
	}
	if entry.name != "SKILL.md" || !entry.isRegular {
		return nil
	}
	remaining := scanner.limits.maxBytes - scanner.bytes
	readLimit := min(int64(maxCanonicalSkillBytes), remaining)
	data, readErr := readRegularFileAt(directory, entry.name, path, readLimit)
	if err := scanner.checkTime("after reading marker file " + path); err != nil {
		return err
	}
	if readErr != nil {
		if strings.Contains(readErr.Error(), "file exceeds") {
			return fmt.Errorf("marker scan byte limit exceeded while reading %s: %w", path, readErr)
		}
		return fmt.Errorf("scan marker file %s: %w", path, readErr)
	}
	scanner.bytes += int64(len(data))
	if scanner.bytes > scanner.limits.maxBytes {
		return fmt.Errorf("marker scan byte limit exceeded: read %d bytes, maximum %d", scanner.bytes, scanner.limits.maxBytes)
	}
	if bytes.Contains(data, []byte(generatedMarker)) {
		scanner.marked = append(scanner.marked, filepath.ToSlash(path))
	}
	return nil
}

func (scanner *markerScanner) readDirectoryEntries(directory *os.File, relative string, depth int) ([]markerScanEntry, error) {
	entries := make([]markerScanEntry, 0)
	for {
		if err := scanner.checkTime("before reading directory " + scanPathLabel(relative)); err != nil {
			return nil, err
		}
		batch, readErr := directory.ReadDir(128)
		if err := scanner.checkTime("after reading directory " + scanPathLabel(relative)); err != nil {
			return nil, err
		}
		for _, entry := range batch {
			inspected, include, err := scanner.inspectDirectoryEntry(directory, relative, depth, entry)
			if err != nil {
				return nil, err
			}
			if include {
				entries = append(entries, inspected)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("enumerate marker scan directory %s: %w", scanPathLabel(relative), readErr)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	if err := scanner.checkTime("after sorting directory entries for " + scanPathLabel(relative)); err != nil {
		return nil, err
	}
	return entries, nil
}

func (scanner *markerScanner) inspectDirectoryEntry(
	directory *os.File,
	relative string,
	depth int,
	entry fs.DirEntry,
) (markerScanEntry, bool, error) {
	if relative == "" && entry.Name() == ".git" {
		return markerScanEntry{}, false, nil
	}
	path := joinScanPath(relative, entry.Name())
	if err := scanner.checkTime("before inspecting entry " + path); err != nil {
		return markerScanEntry{}, false, err
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(int(directory.Fd()), entry.Name(), &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return markerScanEntry{}, false, fmt.Errorf("inspect marker scan entry %s without following a symlink: %w", path, err)
	}
	entryType := stat.Mode & unix.S_IFMT
	isDirectory := entryType == unix.S_IFDIR
	if isDirectory {
		if depth+1 > scanner.limits.maxDepth {
			return markerScanEntry{}, false, fmt.Errorf("marker scan depth limit exceeded at %s: depth %d, maximum %d", path, depth+1, scanner.limits.maxDepth)
		}
		scanner.directories++
		if scanner.directories > scanner.limits.maxDirectories {
			return markerScanEntry{}, false, fmt.Errorf("marker scan directory limit exceeded: visited more than %d directories", scanner.limits.maxDirectories)
		}
	} else {
		scanner.files++
		if scanner.files > scanner.limits.maxFiles {
			return markerScanEntry{}, false, fmt.Errorf("marker scan file limit exceeded: visited more than %d files", scanner.limits.maxFiles)
		}
	}
	return markerScanEntry{
		name:        entry.Name(),
		isDirectory: isDirectory,
		isRegular:   entryType == unix.S_IFREG,
	}, true, nil
}

func (scanner *markerScanner) checkTime(checkpoint string) error {
	elapsed := scanner.limits.now(checkpoint).Sub(scanner.started)
	if elapsed > scanner.limits.elapsedBudget {
		return fmt.Errorf(
			"marker scan checked elapsed-time budget exceeded at %s: elapsed %s, budget %s; result rejected at a cooperative checkpoint because synchronous filesystem calls are not interruptible",
			checkpoint,
			elapsed,
			scanner.limits.elapsedBudget,
		)
	}
	return nil
}

func joinScanPath(parent, name string) string {
	if parent == "" {
		return filepath.ToSlash(name)
	}
	return filepath.ToSlash(parent + "/" + name)
}

func scanPathLabel(relative string) string {
	if relative == "" {
		return "."
	}
	return filepath.ToSlash(relative)
}

func closeDirectoryFiles(directories []*os.File) func() {
	return func() {
		for _, directory := range slices.Backward(directories) {
			_ = directory.Close()
		}
	}
}

func finishDelegateFile(file *os.File, content []byte) error {
	var result error
	for len(content) > 0 && result == nil {
		written, writeErr := file.Write(content)
		if writeErr != nil {
			result = fmt.Errorf("write exact delegate bytes: %w", writeErr)
			break
		}
		if written == 0 {
			result = ioErrNoProgress{}
			break
		}
		content = content[written:]
	}
	if chmodErr := file.Chmod(0o644); chmodErr != nil {
		result = errors.Join(result, fmt.Errorf("set delegate mode 0644: %w", chmodErr))
	}
	if syncErr := file.Sync(); syncErr != nil {
		result = errors.Join(result, fmt.Errorf("sync delegate bytes: %w", syncErr))
	}
	if closeErr := file.Close(); closeErr != nil {
		result = errors.Join(result, fmt.Errorf("close delegate: %w", closeErr))
	}
	return result
}

type ioErrNoProgress struct{}

func (ioErrNoProgress) Error() string { return "write exact delegate bytes: no progress" }
