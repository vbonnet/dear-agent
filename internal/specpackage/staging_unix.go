//go:build darwin || linux

package specpackage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

type stagedOpenEntry struct {
	file      *os.File
	stat      unix.Stat_t
	directory bool
}

type stagedFilesystem struct {
	root        *anchoredRoot
	directories map[string]*os.File
	entries     map[string]stagedOpenEntry
}

var beforeStagedFileWrite = func(string) {}

func newStagedFilesystem(root *anchoredRoot, identity *stagedRootIdentity) (*stagedFilesystem, error) {
	if root == nil || root.directory == nil {
		return nil, fmt.Errorf("private staging root handle is not open")
	}
	if root.identity != identity.file {
		return nil, fmt.Errorf("private staging root handle changed before writing")
	}
	staged := &stagedFilesystem{
		root:        root,
		directories: map[string]*os.File{".": root.directory},
		entries:     make(map[string]stagedOpenEntry, len(payloadLayout)+len(expectedDirectories)),
	}
	if err := staged.verifyRoot(); err != nil {
		return nil, err
	}
	return staged, nil
}

func (staged *stagedFilesystem) Mkdir(relative string) error {
	if err := validatePackagePath(relative); err != nil {
		return err
	}
	if err := staged.verifyParentVisible(relative); err != nil {
		return err
	}
	parent, err := staged.parentDirectory(relative)
	if err != nil {
		return err
	}
	name := path.Base(relative)
	if err := unix.Mkdirat(int(parent.Fd()), name, uint32(privateDirectoryMode.Perm())); err != nil {
		return fmt.Errorf("create staged directory %q: %w", relative, err)
	}
	opened, openedStat, err := openCreatedStagedDirectory(parent, name, relative)
	if err != nil {
		return err
	}
	staged.directories[relative] = opened
	staged.entries[relative] = stagedOpenEntry{file: opened, stat: openedStat, directory: true}
	return nil
}

func openCreatedStagedDirectory(parent *os.File, name, relative string) (*os.File, unix.Stat_t, error) {
	var created unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &created, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, unix.Stat_t{}, fmt.Errorf("inspect created staged directory %q: %w", relative, err)
	}
	fd, err := unix.Openat(
		int(parent.Fd()), name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, unix.Stat_t{}, fmt.Errorf("open created staged directory %q: %w", relative, err)
	}
	opened := os.NewFile(uintptr(fd), relative)
	if opened == nil {
		_ = unix.Close(fd)
		return nil, unix.Stat_t{}, fmt.Errorf("open created staged directory %q: create file handle", relative)
	}
	var openedStat unix.Stat_t
	var visible unix.Stat_t
	if err := unix.Fstat(fd, &openedStat); err != nil {
		return nil, unix.Stat_t{}, errors.Join(
			fmt.Errorf("inspect opened staged directory %q: %w", relative, err), opened.Close(),
		)
	}
	if err := unix.Fstatat(int(parent.Fd()), name, &visible, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, unix.Stat_t{}, errors.Join(
			fmt.Errorf("reinspect staged directory %q: %w", relative, err), opened.Close(),
		)
	}
	if err := validateCreatedStagedDirectory(relative, &created, &openedStat, &visible); err != nil {
		return nil, unix.Stat_t{}, errors.Join(err, opened.Close())
	}
	return opened, openedStat, nil
}

func validateCreatedStagedDirectory(relative string, created, opened, visible *unix.Stat_t) error {
	if created.Mode&unix.S_IFMT != unix.S_IFDIR ||
		opened.Mode&unix.S_IFMT != unix.S_IFDIR ||
		visible.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("staged directory %q changed type while it was created", relative)
	}
	if identityFromUnixStat(created) != identityFromUnixStat(opened) ||
		identityFromUnixStat(opened) != identityFromUnixStat(visible) {
		return fmt.Errorf("staged directory %q changed identity while it was created", relative)
	}
	if fileModeFromUnix(uint32(opened.Mode)) != privateDirectoryMode ||
		fileModeFromUnix(uint32(visible.Mode)) != privateDirectoryMode {
		return fmt.Errorf("staged directory %q changed mode while it was created", relative)
	}
	return nil
}

func (staged *stagedFilesystem) WriteFile(relative string, content []byte, mode fs.FileMode) error {
	if err := validatePackagePath(relative); err != nil {
		return err
	}
	beforeStagedFileWrite(relative)
	if err := staged.verifyParentVisible(relative); err != nil {
		return err
	}
	parent, err := staged.parentDirectory(relative)
	if err != nil {
		return err
	}
	name := path.Base(relative)
	fd, err := unix.Openat(
		int(parent.Fd()), name,
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0o600,
	)
	if err != nil {
		return fmt.Errorf("create staged file %q: %w", relative, err)
	}
	file := os.NewFile(uintptr(fd), relative)
	if file == nil {
		_ = unix.Close(fd)
		return fmt.Errorf("create staged file %q: create file handle", relative)
	}
	if err := writeOpenedStagedFile(file, relative, content, mode); err != nil {
		return errors.Join(err, file.Close())
	}
	var opened unix.Stat_t
	var visible unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return errors.Join(fmt.Errorf("inspect opened staged file %q: %w", relative, err), file.Close())
	}
	if err := unix.Fstatat(int(parent.Fd()), name, &visible, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return errors.Join(fmt.Errorf("reinspect staged file %q: %w", relative, err), file.Close())
	}
	if err := validateRegularStat(relative, &opened); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := validateRegularStat(relative, &visible); err != nil {
		return errors.Join(err, file.Close())
	}
	if !sameStableRegularStat(&opened, &visible) || fileModeFromUnix(uint32(opened.Mode)) != mode {
		return errors.Join(fmt.Errorf("staged file %q changed while it was created", relative), file.Close())
	}
	staged.entries[relative] = stagedOpenEntry{file: file, stat: opened}
	return nil
}

func writeOpenedStagedFile(file *os.File, relative string, content []byte, mode fs.FileMode) error {
	written, err := file.Write(content)
	if err != nil {
		return fmt.Errorf("write staged file %q: %w", relative, err)
	}
	if written != len(content) {
		return fmt.Errorf("write staged file %q: wrote %d of %d bytes", relative, written, len(content))
	}
	if err := file.Chmod(mode); err != nil {
		return fmt.Errorf("set staged file %q mode: %w", relative, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync staged file %q: %w", relative, err)
	}
	return nil
}

func (staged *stagedFilesystem) Sync() error {
	directories := make([]string, 0, len(staged.directories))
	for relative := range staged.directories {
		directories = append(directories, relative)
	}
	sort.Slice(directories, func(i, j int) bool {
		leftDepth := strings.Count(directories[i], "/")
		rightDepth := strings.Count(directories[j], "/")
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return directories[i] > directories[j]
	})
	for _, relative := range directories {
		if err := staged.directories[relative].Sync(); err != nil {
			return fmt.Errorf("sync staged directory %q: %w", relative, err)
		}
	}
	return nil
}

func (staged *stagedFilesystem) Verify(ctx context.Context) error {
	if err := staged.verifyRoot(); err != nil {
		return err
	}
	if err := staged.verifyExactTree(ctx); err != nil {
		return err
	}
	paths := make([]string, 0, len(staged.entries))
	for relative := range staged.entries {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	for _, relative := range paths {
		if err := staged.verifyEntry(relative, staged.entries[relative]); err != nil {
			return err
		}
	}
	return staged.verifyRoot()
}

func (staged *stagedFilesystem) verifyRoot() error {
	if staged == nil || staged.root == nil || staged.root.directory == nil {
		return fmt.Errorf("private staging root handle is not open")
	}
	var opened unix.Stat_t
	var visible unix.Stat_t
	if err := unix.Fstat(int(staged.root.directory.Fd()), &opened); err != nil {
		return fmt.Errorf("reinspect opened staged root: %w", err)
	}
	if err := unix.Lstat(staged.root.path, &visible); err != nil {
		return fmt.Errorf("reinspect visible staged root: %w", err)
	}
	if opened.Mode&unix.S_IFMT != unix.S_IFDIR || visible.Mode&unix.S_IFMT != unix.S_IFDIR ||
		identityFromUnixStat(&opened) != staged.root.identity ||
		identityFromUnixStat(&visible) != staged.root.identity {
		return fmt.Errorf("staged root changed identity or type")
	}
	if fileModeFromUnix(uint32(opened.Mode)) != privateDirectoryMode ||
		fileModeFromUnix(uint32(visible.Mode)) != privateDirectoryMode {
		return fmt.Errorf("staged root changed mode")
	}
	return nil
}

func (staged *stagedFilesystem) verifyExactTree(ctx context.Context) error {
	tree, err := staged.root.readTree(ctx, ".")
	if err != nil {
		return err
	}
	directories := make([]string, 0, len(expectedDirectories))
	files := make([]string, 0, len(payloadLayout)+1)
	for _, entry := range tree {
		if entry.directory {
			directories = append(directories, entry.path)
		} else {
			files = append(files, entry.path)
		}
		if entry.path == "." {
			if entry.identity != staged.root.identity || entry.mode != privateDirectoryMode {
				return fmt.Errorf("staged root changed identity or mode")
			}
			continue
		}
		expected, ok := staged.entries[entry.path]
		if !ok || entry.identity != identityFromUnixStat(&expected.stat) || entry.directory != expected.directory {
			return fmt.Errorf("staged tree entry %q does not match its retained identity", entry.path)
		}
	}
	expectedDirectories, expectedFiles := expectedPackagePaths()
	if err := equalSortedPaths(directories, expectedDirectories, "directory"); err != nil {
		return err
	}
	if err := equalSortedPaths(files, expectedFiles, "file"); err != nil {
		return err
	}
	return nil
}

func (staged *stagedFilesystem) verifyEntry(relative string, expected stagedOpenEntry) error {
	parent, err := staged.parentDirectory(relative)
	if err != nil {
		return err
	}
	var opened unix.Stat_t
	var visible unix.Stat_t
	if err := unix.Fstat(int(expected.file.Fd()), &opened); err != nil {
		return fmt.Errorf("reinspect opened staged entry %q: %w", relative, err)
	}
	if err := unix.Fstatat(int(parent.Fd()), path.Base(relative), &visible, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("reinspect visible staged entry %q: %w", relative, err)
	}
	if identityFromUnixStat(&opened) != identityFromUnixStat(&expected.stat) ||
		identityFromUnixStat(&visible) != identityFromUnixStat(&expected.stat) {
		return fmt.Errorf("staged entry %q changed identity", relative)
	}
	if expected.directory {
		if opened.Mode&unix.S_IFMT != unix.S_IFDIR ||
			visible.Mode&unix.S_IFMT != unix.S_IFDIR ||
			fileModeFromUnix(uint32(opened.Mode)) != privateDirectoryMode ||
			fileModeFromUnix(uint32(visible.Mode)) != privateDirectoryMode {
			return fmt.Errorf("staged directory %q changed metadata", relative)
		}
		return nil
	}
	if !sameStableRegularStat(&expected.stat, &opened) || !sameStableRegularStat(&expected.stat, &visible) {
		return fmt.Errorf("staged file %q changed metadata or content state", relative)
	}
	return nil
}

func (staged *stagedFilesystem) parentDirectory(relative string) (*os.File, error) {
	parentPath := path.Dir(relative)
	parent, ok := staged.directories[parentPath]
	if !ok {
		return nil, fmt.Errorf("staged parent directory %q is not retained", parentPath)
	}
	return parent, nil
}

func (staged *stagedFilesystem) verifyParentVisible(relative string) error {
	parentPath := path.Dir(relative)
	ancestors := make([]string, 0, maxPackagePathDepth)
	for current := parentPath; current != "."; current = path.Dir(current) {
		ancestors = append(ancestors, current)
	}
	if err := staged.root.verifyVisible(); err != nil {
		return fmt.Errorf("verify staged root before mutating %q: %w", relative, err)
	}
	for _, ancestor := range slices.Backward(ancestors) {
		expected, ok := staged.entries[ancestor]
		if !ok || !expected.directory {
			return fmt.Errorf("staged ancestor directory %q has no retained identity", ancestor)
		}
		if err := staged.verifyEntry(ancestor, expected); err != nil {
			return fmt.Errorf("verify staged ancestor before mutating %q: %w", relative, err)
		}
	}
	return nil
}

func (staged *stagedFilesystem) Close() error {
	if staged == nil || staged.root == nil {
		return nil
	}
	paths := make([]string, 0, len(staged.entries))
	for relative := range staged.entries {
		paths = append(paths, relative)
	}
	sort.Slice(paths, func(i, j int) bool {
		leftDepth := strings.Count(paths[i], "/")
		rightDepth := strings.Count(paths[j], "/")
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return paths[i] > paths[j]
	})
	var result error
	for _, relative := range paths {
		entry := staged.entries[relative]
		if entry.file == nil {
			continue
		}
		if err := entry.file.Close(); err != nil {
			result = errors.Join(result, fmt.Errorf("close retained staged entry %q: %w", relative, err))
		}
		entry.file = nil
		staged.entries[relative] = entry
	}
	if err := staged.root.Close(); err != nil {
		result = errors.Join(result, fmt.Errorf("close private staging root: %w", err))
	}
	staged.root = nil
	return result
}
