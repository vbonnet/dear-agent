//go:build darwin || linux

package specpackage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

type anchoredRoot struct {
	path      string
	directory *os.File
	identity  fileIdentity
}

type stagedRootIdentity struct {
	file fileIdentity
}

var (
	afterRegularFilePreInspection = func(string) {}
	afterRegularFileOpen          = func(string) {}
	afterPrivateStagingRootMkdir  = func(string) {}
)

func openAnchoredRoot(rootPath string) (*anchoredRoot, error) {
	rootPath, err := cleanAbsolutePath(rootPath, "root")
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("inspect root %q: %w", rootPath, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("root %q is not a nonsymlink directory", rootPath)
	}
	fd, err := unix.Open(rootPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open anchored root %q: %w", rootPath, err)
	}
	directory := os.NewFile(uintptr(fd), rootPath)
	if directory == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open anchored root %q: create file handle", rootPath)
	}
	opened, err := directory.Stat()
	if err != nil {
		_ = directory.Close()
		return nil, fmt.Errorf("inspect opened root %q: %w", rootPath, err)
	}
	if !os.SameFile(info, opened) {
		_ = directory.Close()
		return nil, fmt.Errorf("root %q changed while it was opened", rootPath)
	}
	identity, _, err := identityFromFileInfo(opened)
	if err != nil {
		_ = directory.Close()
		return nil, fmt.Errorf("identify opened root %q: %w", rootPath, err)
	}
	return &anchoredRoot{path: rootPath, directory: directory, identity: identity}, nil
}

func (root *anchoredRoot) Close() error {
	if root == nil || root.directory == nil {
		return nil
	}
	return root.directory.Close()
}

func (root *anchoredRoot) verifyVisible() error {
	if root == nil || root.directory == nil {
		return fmt.Errorf("anchored root is not open")
	}
	info, err := os.Lstat(root.path)
	if err != nil {
		return fmt.Errorf("reinspect anchored root %q: %w", root.path, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("anchored root %q is no longer the visible nonsymlink directory", root.path)
	}
	identity, _, err := identityFromFileInfo(info)
	if err != nil {
		return fmt.Errorf("identify visible anchored root %q: %w", root.path, err)
	}
	if identity != root.identity {
		return fmt.Errorf("anchored root %q changed while it was in use", root.path)
	}
	return nil
}

func (root *anchoredRoot) openDirectory(relative string) (*os.File, error) {
	if relative == "" || relative == "." {
		fd, err := unix.Openat(
			int(root.directory.Fd()), ".",
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
			0,
		)
		if err != nil {
			return nil, fmt.Errorf("reopen root directory handle: %w", err)
		}
		return os.NewFile(uintptr(fd), root.path), nil
	}
	if err := validatePackagePath(relative); err != nil {
		return nil, fmt.Errorf("open directory %q: %w", relative, err)
	}
	fd, err := unix.Openat(
		int(root.directory.Fd()), ".",
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("reopen root directory handle: %w", err)
	}
	current := os.NewFile(uintptr(fd), root.path)
	for component := range strings.SplitSeq(relative, "/") {
		nextFD, openErr := unix.Openat(
			int(current.Fd()), component,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
			0,
		)
		closeErr := current.Close()
		if openErr != nil {
			return nil, errors.Join(fmt.Errorf("open directory component %q: %w", component, openErr), closeErr)
		}
		if closeErr != nil {
			_ = unix.Close(nextFD)
			return nil, fmt.Errorf("close parent directory while opening %q: %w", relative, closeErr)
		}
		current = os.NewFile(uintptr(nextFD), component)
	}
	return current, nil
}

func (root *anchoredRoot) readTree(ctx context.Context, relative string) ([]treeEntry, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	directory, err := root.openDirectory(relative)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries := make([]treeEntry, 0, len(payloadLayout)+len(expectedDirectories))
	rootInfo, err := directory.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect package directory %q: %w", relative, err)
	}
	rootIdentity, _, err := identityFromFileInfo(rootInfo)
	if err != nil {
		return nil, fmt.Errorf("identify package directory %q: %w", relative, err)
	}
	entries = append(entries, treeEntry{
		path:      relative,
		directory: true,
		mode:      rootInfo.Mode() & (fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky),
		identity:  rootIdentity,
		state:     fmt.Sprintf("%v:%d:%d", rootInfo.Mode(), rootInfo.Size(), rootInfo.ModTime().UnixNano()),
	})
	if err := readDirectoryTree(ctx, directory, relative, &entries); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, entry.path)
	}
	if err := validateNoPathAliases(paths); err != nil {
		return nil, err
	}
	if err := root.verifyVisible(); err != nil {
		return nil, err
	}
	return entries, nil
}

func readDirectoryTree(ctx context.Context, directory *os.File, relative string, result *[]treeEntry) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	children, err := readBoundedDirectoryEntries(directory, relative, len(*result))
	if err != nil {
		return err
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
	for _, child := range children {
		if err := checkContext(ctx); err != nil {
			return err
		}
		if err := readDirectoryTreeEntry(ctx, directory, relative, child.Name(), result); err != nil {
			return err
		}
	}
	return nil
}

func readBoundedDirectoryEntries(directory *os.File, relative string, currentCount int) ([]os.DirEntry, error) {
	remaining := maxPackageTreeEntries - currentCount
	if remaining <= 0 {
		return nil, fmt.Errorf("package tree exceeds the %d-entry bound", maxPackageTreeEntries)
	}
	children, err := directory.ReadDir(remaining + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read package directory %q: %w", relative, err)
	}
	if len(children) > remaining {
		return nil, fmt.Errorf("package tree exceeds the %d-entry bound", maxPackageTreeEntries)
	}
	return children, nil
}

func readDirectoryTreeEntry(ctx context.Context, directory *os.File, relative, name string, result *[]treeEntry) error {
	packagePath, err := packageChildPath(relative, name)
	if err != nil {
		return err
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(int(directory.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("inspect package entry %q: %w", packagePath, err)
	}
	entry := treeEntry{
		path:     packagePath,
		mode:     fileModeFromUnix(uint32(stat.Mode)),
		identity: identityFromUnixStat(&stat),
		state:    stableStatToken(&stat),
	}
	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
		entry.directory = true
		*result = append(*result, entry)
		return walkTreeDirectory(ctx, directory, name, packagePath, entry.identity, result)
	case unix.S_IFREG:
		if uint64(stat.Nlink) != 1 {
			return fmt.Errorf("package file %q has %d hard links, want 1", packagePath, stat.Nlink)
		}
		*result = append(*result, entry)
		return nil
	case unix.S_IFLNK:
		return fmt.Errorf("package entry %q is a symbolic link", packagePath)
	case unix.S_IFIFO:
		return fmt.Errorf("package entry %q is a FIFO", packagePath)
	case unix.S_IFSOCK:
		return fmt.Errorf("package entry %q is a socket", packagePath)
	default:
		return fmt.Errorf("package entry %q is a device or unsupported special file", packagePath)
	}
}

func packageChildPath(relative, name string) (string, error) {
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\\x00") {
		return "", fmt.Errorf("package directory %q contains an invalid entry name", relative)
	}
	packagePath := path.Join(relative, name)
	if relative == "." || relative == "" {
		packagePath = name
	}
	if err := validatePackagePath(packagePath); err != nil {
		return "", fmt.Errorf("inspect package entry %q: %w", packagePath, err)
	}
	return packagePath, nil
}

func walkTreeDirectory(
	ctx context.Context,
	parent *os.File,
	name string,
	packagePath string,
	expected fileIdentity,
	result *[]treeEntry,
) error {
	childFD, err := unix.Openat(
		int(parent.Fd()), name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return fmt.Errorf("open package directory %q: %w", packagePath, err)
	}
	opened := os.NewFile(uintptr(childFD), packagePath)
	openedInfo, statErr := opened.Stat()
	if statErr != nil {
		_ = opened.Close()
		return fmt.Errorf("inspect opened package directory %q: %w", packagePath, statErr)
	}
	openedIdentity, _, identityErr := identityFromFileInfo(openedInfo)
	if identityErr != nil || openedIdentity != expected {
		_ = opened.Close()
		return errors.Join(fmt.Errorf("package directory %q changed while it was opened", packagePath), identityErr)
	}
	walkErr := readDirectoryTree(ctx, opened, packagePath, result)
	closeErr := opened.Close()
	return errors.Join(walkErr, closeErr)
}

func (root *anchoredRoot) readRegular(ctx context.Context, relative string, maximum int64) (fileSnapshot, error) {
	if err := checkContext(ctx); err != nil {
		return fileSnapshot{}, err
	}
	if err := validatePackagePath(relative); err != nil {
		return fileSnapshot{}, fmt.Errorf("read package file %q: %w", relative, err)
	}
	parent, err := root.openDirectory(path.Dir(relative))
	if err != nil {
		return fileSnapshot{}, err
	}
	defer parent.Close()
	file, opened, err := openStableRegular(parent, relative, maximum)
	if err != nil {
		return fileSnapshot{}, err
	}
	defer file.Close()
	afterRegularFileOpen(relative)
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("read package file %q: %w", relative, err)
	}
	if int64(len(data)) > maximum {
		return fileSnapshot{}, fmt.Errorf("package file %q exceeds the %d-byte bound", relative, maximum)
	}
	if err := verifyRegularAfterRead(parent, file, relative, &opened, data); err != nil {
		return fileSnapshot{}, err
	}
	if err := root.verifyVisible(); err != nil {
		return fileSnapshot{}, err
	}
	return fileSnapshot{data: data, mode: fileModeFromUnix(uint32(opened.Mode))}, nil
}

func openStableRegular(parent *os.File, relative string, maximum int64) (*os.File, unix.Stat_t, error) {
	name := path.Base(relative)
	var before unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, unix.Stat_t{}, fmt.Errorf("inspect package file %q: %w", relative, err)
	}
	if err := validateRegularStat(relative, &before); err != nil {
		return nil, unix.Stat_t{}, err
	}
	if before.Size < 0 || before.Size > maximum {
		return nil, unix.Stat_t{}, fmt.Errorf("package file %q size %d exceeds the %d-byte bound", relative, before.Size, maximum)
	}
	afterRegularFilePreInspection(relative)
	fd, err := unix.Openat(
		int(parent.Fd()), name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, unix.Stat_t{}, fmt.Errorf("open package file %q without following links: %w", relative, err)
	}
	file := os.NewFile(uintptr(fd), relative)
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return nil, unix.Stat_t{}, errors.Join(fmt.Errorf("inspect opened package file %q: %w", relative, err), file.Close())
	}
	if err := validateRegularStat(relative, &opened); err != nil {
		return nil, unix.Stat_t{}, errors.Join(err, file.Close())
	}
	if !sameStableRegularStat(&before, &opened) {
		return nil, unix.Stat_t{}, errors.Join(fmt.Errorf("package file %q changed while it was opened", relative), file.Close())
	}
	return file, opened, nil
}

func verifyRegularAfterRead(parent, file *os.File, relative string, opened *unix.Stat_t, data []byte) error {
	name := path.Base(relative)
	var finalOpened unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &finalOpened); err != nil {
		return fmt.Errorf("reinspect opened package file %q: %w", relative, err)
	}
	var after unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("reinspect package file %q: %w", relative, err)
	}
	if err := validateRegularStat(relative, &finalOpened); err != nil {
		return err
	}
	if err := validateRegularStat(relative, &after); err != nil {
		return err
	}
	if !sameStableRegularStat(opened, &finalOpened) ||
		!sameStableRegularStat(opened, &after) ||
		opened.Size != int64(len(data)) {
		return fmt.Errorf("package file %q changed while it was read", relative)
	}
	return nil
}

func validateRegularStat(relative string, stat *unix.Stat_t) error {
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("package file %q is not a regular file", relative)
	}
	if uint64(stat.Nlink) != 1 {
		return fmt.Errorf("package file %q has %d hard links, want 1", relative, stat.Nlink)
	}
	return nil
}

func identityFromUnixStat(stat *unix.Stat_t) fileIdentity {
	return fileIdentity{device: fmt.Sprint(stat.Dev), inode: fmt.Sprint(stat.Ino)}
}

func identityFromFileInfo(info os.FileInfo) (fileIdentity, uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}, 0, fmt.Errorf("unsupported file identity type %T", info.Sys())
	}
	return fileIdentity{device: fmt.Sprint(stat.Dev), inode: fmt.Sprint(stat.Ino)}, uint64(stat.Nlink), nil
}

func fileModeFromUnix(mode uint32) fs.FileMode {
	result := fs.FileMode(mode & 0o777)
	if mode&unix.S_ISUID != 0 {
		result |= fs.ModeSetuid
	}
	if mode&unix.S_ISGID != 0 {
		result |= fs.ModeSetgid
	}
	if mode&unix.S_ISVTX != 0 {
		result |= fs.ModeSticky
	}
	return result
}

func readStandaloneRegular(ctx context.Context, absolutePath string, maximum int64) (fileSnapshot, error) {
	absolutePath, err := cleanAbsolutePath(absolutePath, "artifact")
	if err != nil {
		return fileSnapshot{}, err
	}
	root, err := openAnchoredRoot(filepath.Dir(absolutePath))
	if err != nil {
		return fileSnapshot{}, err
	}
	defer root.Close()
	return root.readRegular(ctx, filepath.Base(absolutePath), maximum)
}

//nolint:gocyclo // The bounded handle-ancestry walk keeps every fail-closed identity checkpoint explicit.
func validateStagingParentOutsideSource(source, stagingParent *anchoredRoot) (result error) {
	if source == nil || source.directory == nil {
		return fmt.Errorf("source root handle is not open")
	}
	if stagingParent == nil || stagingParent.directory == nil {
		return fmt.Errorf("staging parent handle is not open")
	}
	fd, err := unix.Openat(
		int(stagingParent.directory.Fd()), ".",
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return fmt.Errorf("open staging parent ancestry: %w", err)
	}
	current := os.NewFile(uintptr(fd), stagingParent.path)
	if current == nil {
		_ = unix.Close(fd)
		return fmt.Errorf("open staging parent ancestry: create file handle")
	}
	defer func() { result = errors.Join(result, current.Close()) }()

	const maximumAncestryDepth = 1024
	for depth := range maximumAncestryDepth {
		var currentStat unix.Stat_t
		if err := unix.Fstat(int(current.Fd()), &currentStat); err != nil {
			return fmt.Errorf("inspect staging parent ancestry at depth %d: %w", depth, err)
		}
		currentIdentity := identityFromUnixStat(&currentStat)
		if currentIdentity == source.identity {
			return fmt.Errorf("staging parent must not be the source root or a source descendant")
		}

		parentFD, err := unix.Openat(
			int(current.Fd()), "..",
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
			0,
		)
		if err != nil {
			return fmt.Errorf("open staging parent ancestor at depth %d: %w", depth+1, err)
		}
		parent := os.NewFile(uintptr(parentFD), "..")
		if parent == nil {
			_ = unix.Close(parentFD)
			return fmt.Errorf("open staging parent ancestor at depth %d: create file handle", depth+1)
		}
		var parentStat unix.Stat_t
		if err := unix.Fstat(parentFD, &parentStat); err != nil {
			return errors.Join(
				fmt.Errorf("inspect staging parent ancestor at depth %d: %w", depth+1, err),
				parent.Close(),
			)
		}
		if identityFromUnixStat(&parentStat) == currentIdentity {
			if err := parent.Close(); err != nil {
				return fmt.Errorf("close filesystem-root ancestry handle: %w", err)
			}
			if err := source.verifyVisible(); err != nil {
				return fmt.Errorf("reinspect source root after staging-parent ancestry check: %w", err)
			}
			if err := stagingParent.verifyVisible(); err != nil {
				return fmt.Errorf("reinspect staging parent after ancestry check: %w", err)
			}
			return nil
		}
		if err := current.Close(); err != nil {
			_ = parent.Close()
			return fmt.Errorf("close staging parent ancestry handle at depth %d: %w", depth, err)
		}
		current = parent
	}
	return fmt.Errorf("staging parent ancestry exceeds the %d-directory bound", maximumAncestryDepth)
}

//nolint:gocyclo // Allocation keeps each post-mkdir identity failure explicit because no path may be auto-unlinked.
func createPrivateStagingRoot(source, parent *anchoredRoot) (string, stagedRootIdentity, *anchoredRoot, error) {
	if parent == nil || parent.directory == nil {
		return "", stagedRootIdentity{}, nil, fmt.Errorf("staging parent handle is not open")
	}
	for range 100 {
		random := make([]byte, 12)
		if _, err := rand.Read(random); err != nil {
			return "", stagedRootIdentity{}, nil, fmt.Errorf("generate private staging name: %w", err)
		}
		name := ".spec-governance-stage-" + hex.EncodeToString(random)
		// Keep the final ancestry observation immediately adjacent to mkdirat.
		// POSIX still cannot make this check and mutation atomic against a
		// hostile process with same-UID namespace control.
		if err := validateStagingParentOutsideSource(source, parent); err != nil {
			return "", stagedRootIdentity{}, nil, fmt.Errorf("verify staging parent before allocation: %w", err)
		}
		if err := unix.Mkdirat(int(parent.directory.Fd()), name, 0o700); err != nil {
			if errors.Is(err, unix.EEXIST) {
				continue
			}
			return "", stagedRootIdentity{}, nil, fmt.Errorf("create private staging root: %w", err)
		}
		absolute := filepath.Join(parent.path, name)
		afterPrivateStagingRootMkdir(absolute)
		fd, err := unix.Openat(
			int(parent.directory.Fd()), name,
			unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
			0,
		)
		if err != nil {
			return absolute, stagedRootIdentity{}, nil, fmt.Errorf("open private staging root: %w", err)
		}
		created := os.NewFile(uintptr(fd), name)
		if created == nil {
			_ = unix.Close(fd)
			return absolute, stagedRootIdentity{}, nil, fmt.Errorf("open private staging root: create file handle")
		}
		info, statErr := created.Stat()
		if statErr != nil {
			return absolute, stagedRootIdentity{}, nil, errors.Join(statErr, created.Close())
		}
		identity, links, err := identityFromFileInfo(info)
		if err != nil || links < 1 {
			return absolute, stagedRootIdentity{}, nil, errors.Join(
				fmt.Errorf("identify private staging root"), err, created.Close(),
			)
		}
		stagedIdentity := stagedRootIdentity{file: identity}
		openedRoot := &anchoredRoot{path: absolute, directory: created, identity: identity}
		visible, err := os.Lstat(absolute)
		if err != nil {
			return absolute, stagedIdentity, nil, errors.Join(
				fmt.Errorf("reinspect private staging root: %w", err), openedRoot.Close(),
			)
		}
		visibleIdentity, _, err := identityFromFileInfo(visible)
		if err != nil || visibleIdentity != identity {
			return absolute, stagedIdentity, nil, errors.Join(
				fmt.Errorf("staging parent changed while the private root was created"), err, openedRoot.Close(),
			)
		}
		return absolute, stagedIdentity, openedRoot, nil
	}
	return "", stagedRootIdentity{}, nil, fmt.Errorf("could not allocate a unique private staging root")
}

func sameStagedRoot(path string, expected stagedRootIdentity) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	actual, _, err := identityFromFileInfo(info)
	return actual == expected.file, err
}
