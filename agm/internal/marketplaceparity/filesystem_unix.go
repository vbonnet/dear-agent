//go:build darwin || linux

package marketplaceparity

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const maxAnchoredPathDepth = 256

type anchoredTreeEntry struct {
	Path      string
	Directory bool
	Data      []byte
}

type anchoredRoot struct {
	path      string
	directory *os.File
	identity  anchoredIdentity
}

type anchoredIdentity struct {
	device string
	inode  string
}

type anchoredStat struct {
	identity anchoredIdentity
	mode     uint32
	links    uint64
	size     int64
}

type openedDirectory struct {
	file       *os.File
	relative   string
	name       string
	opened     anchoredStat
	modifiedAt int64
}

func readAnchoredRegular(rootPath, relative string, maximum int64) (data []byte, result error) {
	if err := validateReadBound(maximum); err != nil {
		return nil, err
	}
	components, err := validateAnchoredRelative(relative)
	if err != nil {
		return nil, err
	}
	root, err := openMarketplaceRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, root.Close()) }()

	directories, err := root.openDirectoryChain(components[:len(components)-1])
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, closeOpenedDirectories(directories)) }()

	data, err = readStableRegularAt(
		directories[len(directories)-1].file,
		components[len(components)-1],
		relative,
		maximum,
	)
	if err != nil {
		return nil, err
	}
	if err := verifyDirectoryChain(directories); err != nil {
		return nil, err
	}
	if err := root.verifyVisible(); err != nil {
		return nil, err
	}
	return data, nil
}

func readAnchoredTree(rootPath, relative string, maximumEntries int, maximumFileBytes int64) (entries []anchoredTreeEntry, result error) {
	if maximumEntries <= 0 {
		return nil, fmt.Errorf("marketplace tree entry bound must be positive")
	}
	if err := validateReadBound(maximumFileBytes); err != nil {
		return nil, err
	}
	components, err := validateAnchoredRelative(relative)
	if err != nil {
		return nil, err
	}
	root, err := openMarketplaceRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, root.Close()) }()

	directories, err := root.openDirectoryChain(components)
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, closeOpenedDirectories(directories)) }()

	entries = make([]anchoredTreeEntry, 0, maximumEntries)
	if err := walkAnchoredTree(directories[len(directories)-1].file, "", maximumEntries, maximumFileBytes, &entries); err != nil {
		return nil, err
	}
	if err := verifyDirectoryChain(directories); err != nil {
		return nil, err
	}
	if err := root.verifyVisible(); err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(left, right anchoredTreeEntry) int {
		return strings.Compare(left.Path, right.Path)
	})
	return entries, nil
}

func validateReadBound(maximum int64) error {
	if maximum < 0 || maximum == int64(^uint64(0)>>1) {
		return fmt.Errorf("marketplace file byte bound must be between 0 and %d", int64(^uint64(0)>>1)-1)
	}
	return nil
}

func validateAnchoredRelative(relative string) ([]string, error) {
	if relative == "" || !utf8.ValidString(relative) || strings.ContainsRune(relative, '\x00') {
		return nil, fmt.Errorf("marketplace path must be nonempty valid UTF-8")
	}
	if strings.ContainsRune(relative, '\\') || path.IsAbs(relative) || filepath.IsAbs(relative) || path.Clean(relative) != relative {
		return nil, fmt.Errorf("marketplace path %q is noncanonical and may escape its anchored root; want a relative slash path", relative)
	}
	if filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative))) != relative || !filepath.IsLocal(filepath.FromSlash(relative)) {
		return nil, fmt.Errorf("marketplace path %q escapes its anchored root", relative)
	}
	components := strings.Split(relative, "/")
	if len(components) > maxAnchoredPathDepth {
		return nil, fmt.Errorf("marketplace path %q exceeds the %d-component bound", relative, maxAnchoredPathDepth)
	}
	for _, component := range components {
		if err := validateAnchoredName(component); err != nil {
			return nil, fmt.Errorf("marketplace path %q: %w", relative, err)
		}
	}
	return components, nil
}

func validateAnchoredName(name string) error {
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("invalid path component %q", name)
	}
	return nil
}

func openMarketplaceRoot(rootPath string) (*anchoredRoot, error) {
	if rootPath == "" || !utf8.ValidString(rootPath) || strings.ContainsRune(rootPath, '\x00') {
		return nil, fmt.Errorf("marketplace root must be a nonempty valid UTF-8 path")
	}
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve marketplace root %q: %w", rootPath, err)
	}
	absolute = filepath.Clean(absolute)
	var visible unix.Stat_t
	if err := unix.Lstat(absolute, &visible); err != nil {
		return nil, fmt.Errorf("inspect marketplace root %q: %w", absolute, err)
	}
	if err := requireDirectoryStat(absolute, &visible); err != nil {
		return nil, err
	}
	fd, err := unix.Open(
		absolute,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open marketplace root %q without following links: %w", absolute, err)
	}
	directory, err := fileFromDescriptor(fd, absolute)
	if err != nil {
		return nil, err
	}
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return nil, errors.Join(fmt.Errorf("inspect opened marketplace root %q: %w", absolute, err), directory.Close())
	}
	if err := requireDirectoryStat(absolute, &opened); err != nil {
		return nil, errors.Join(err, directory.Close())
	}
	if !sameAnchoredObject(&visible, &opened) {
		return nil, errors.Join(fmt.Errorf("marketplace root %q changed while it was opened", absolute), directory.Close())
	}
	return &anchoredRoot{
		path:      absolute,
		directory: directory,
		identity:  identityFromAnchoredStat(&opened),
	}, nil
}

func fileFromDescriptor(fd int, name string) (*os.File, error) {
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("create file handle for %q", name)
	}
	return file, nil
}

func (root *anchoredRoot) Close() error {
	if root == nil || root.directory == nil {
		return nil
	}
	return root.directory.Close()
}

func (root *anchoredRoot) verifyVisible() error {
	if root == nil || root.directory == nil {
		return fmt.Errorf("marketplace root handle is not open")
	}
	var visible unix.Stat_t
	if err := unix.Lstat(root.path, &visible); err != nil {
		return fmt.Errorf("reinspect marketplace root %q: %w", root.path, err)
	}
	if err := requireDirectoryStat(root.path, &visible); err != nil {
		return err
	}
	if identityFromAnchoredStat(&visible) != root.identity {
		return fmt.Errorf("marketplace root %q changed while it was in use", root.path)
	}
	return nil
}

func (root *anchoredRoot) openDirectoryChain(components []string) ([]openedDirectory, error) {
	rootFD, err := unix.Openat(
		int(root.directory.Fd()),
		".",
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("reopen marketplace root: %w", err)
	}
	rootDirectory, err := fileFromDescriptor(rootFD, root.path)
	if err != nil {
		return nil, err
	}
	rootStamp, err := inspectOpenedDirectory(rootDirectory, ".")
	if err != nil {
		return nil, errors.Join(err, rootDirectory.Close())
	}
	directories := []openedDirectory{rootStamp}
	for index, component := range components {
		relative := strings.Join(components[:index+1], "/")
		child, err := openChildDirectory(directories[len(directories)-1].file, component, relative)
		if err != nil {
			return nil, errors.Join(err, closeOpenedDirectories(directories))
		}
		directories = append(directories, child)
	}
	return directories, nil
}

func inspectOpenedDirectory(directory *os.File, relative string) (openedDirectory, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(int(directory.Fd()), &stat); err != nil {
		return openedDirectory{}, fmt.Errorf("inspect opened marketplace directory %q: %w", relative, err)
	}
	if err := requireDirectoryStat(relative, &stat); err != nil {
		return openedDirectory{}, err
	}
	info, err := directory.Stat()
	if err != nil {
		return openedDirectory{}, fmt.Errorf("read opened marketplace directory state %q: %w", relative, err)
	}
	return openedDirectory{
		file:       directory,
		relative:   relative,
		opened:     snapshotAnchoredStat(&stat),
		modifiedAt: info.ModTime().UnixNano(),
	}, nil
}

func openChildDirectory(parent *os.File, name, relative string) (openedDirectory, error) {
	var before unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return openedDirectory{}, fmt.Errorf("inspect marketplace directory %q: %w", relative, err)
	}
	if err := requireDirectoryStat(relative, &before); err != nil {
		return openedDirectory{}, err
	}
	fd, err := unix.Openat(
		int(parent.Fd()),
		name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return openedDirectory{}, fmt.Errorf("open marketplace directory %q without following links: %w", relative, err)
	}
	directory, err := fileFromDescriptor(fd, relative)
	if err != nil {
		return openedDirectory{}, err
	}
	opened, err := inspectOpenedDirectory(directory, relative)
	if err != nil {
		return openedDirectory{}, errors.Join(err, directory.Close())
	}
	opened.name = name
	if !sameStableAnchoredStat(snapshotAnchoredStat(&before), opened.opened) {
		return openedDirectory{}, errors.Join(fmt.Errorf("marketplace directory %q changed while it was opened", relative), directory.Close())
	}
	return opened, nil
}

func closeOpenedDirectories(directories []openedDirectory) error {
	var result error
	for _, directory := range slices.Backward(directories) {
		if directory.file != nil {
			result = errors.Join(result, directory.file.Close())
		}
	}
	return result
}

func verifyDirectoryChain(directories []openedDirectory) error {
	for index, directory := range slices.Backward(directories) {
		var visibleParent *os.File
		if index > 0 {
			visibleParent = directories[index-1].file
		}
		if err := verifyOpenedDirectory(directory, visibleParent); err != nil {
			return err
		}
	}
	return nil
}

func verifyOpenedDirectory(directory openedDirectory, parent *os.File) error {
	var after unix.Stat_t
	if err := unix.Fstat(int(directory.file.Fd()), &after); err != nil {
		return fmt.Errorf("reinspect opened marketplace directory %q: %w", directory.relative, err)
	}
	if err := requireDirectoryStat(directory.relative, &after); err != nil {
		return err
	}
	info, err := directory.file.Stat()
	if err != nil {
		return fmt.Errorf("read final marketplace directory state %q: %w", directory.relative, err)
	}
	if !sameStableAnchoredStat(directory.opened, snapshotAnchoredStat(&after)) || info.ModTime().UnixNano() != directory.modifiedAt {
		return fmt.Errorf("marketplace directory %q changed while it was read", directory.relative)
	}
	if parent == nil {
		return nil
	}
	var visible unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), directory.name, &visible, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("reinspect marketplace directory %q: %w", directory.relative, err)
	}
	if err := requireDirectoryStat(directory.relative, &visible); err != nil {
		return err
	}
	if !sameStableAnchoredStat(directory.opened, snapshotAnchoredStat(&visible)) {
		return fmt.Errorf("marketplace directory %q changed while it was visible", directory.relative)
	}
	return nil
}

func walkAnchoredTree(directory *os.File, relative string, maximumEntries int, maximumFileBytes int64, result *[]anchoredTreeEntry) error {
	remaining := maximumEntries - len(*result)
	if remaining <= 0 {
		return fmt.Errorf("marketplace tree exceeds the %d-entry bound", maximumEntries)
	}
	children, err := directory.ReadDir(remaining + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read marketplace directory %q: %w", relative, err)
	}
	if len(children) > remaining {
		return fmt.Errorf("marketplace tree exceeds the %d-entry bound", maximumEntries)
	}
	slices.SortFunc(children, func(left, right os.DirEntry) int {
		return strings.Compare(left.Name(), right.Name())
	})
	for _, child := range children {
		if err := walkAnchoredTreeEntry(directory, relative, child.Name(), maximumEntries, maximumFileBytes, result); err != nil {
			return err
		}
	}
	return nil
}

func walkAnchoredTreeEntry(parent *os.File, relative, name string, maximumEntries int, maximumFileBytes int64, result *[]anchoredTreeEntry) error {
	if err := validateAnchoredName(name); err != nil {
		return fmt.Errorf("marketplace directory %q: %w", relative, err)
	}
	entryPath := name
	if relative != "" {
		entryPath = path.Join(relative, name)
	}
	if len(*result) >= maximumEntries {
		return fmt.Errorf("marketplace tree exceeds the %d-entry bound", maximumEntries)
	}
	var before unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("inspect marketplace tree entry %q: %w", entryPath, err)
	}
	switch before.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
		child, err := openChildDirectory(parent, name, entryPath)
		if err != nil {
			return err
		}
		*result = append(*result, anchoredTreeEntry{Path: entryPath, Directory: true})
		walkErr := walkAnchoredTree(child.file, entryPath, maximumEntries, maximumFileBytes, result)
		verifyErr := verifyOpenedDirectory(child, parent)
		closeErr := child.file.Close()
		return errors.Join(walkErr, verifyErr, closeErr)
	case unix.S_IFREG:
		data, err := readStableRegularAt(parent, name, entryPath, maximumFileBytes)
		if err != nil {
			return err
		}
		*result = append(*result, anchoredTreeEntry{Path: entryPath, Data: data})
		return nil
	case unix.S_IFLNK:
		return fmt.Errorf("marketplace tree entry %q escapes its anchored root through a symbolic link", entryPath)
	case unix.S_IFIFO:
		return fmt.Errorf("marketplace tree entry %q is a FIFO", entryPath)
	case unix.S_IFSOCK:
		return fmt.Errorf("marketplace tree entry %q is a socket", entryPath)
	default:
		return fmt.Errorf("marketplace tree entry %q is a device or unsupported special file", entryPath)
	}
}

func readStableRegularAt(parent *os.File, name, relative string, maximum int64) ([]byte, error) {
	var before unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, fmt.Errorf("inspect marketplace file %q: %w", relative, err)
	}
	if err := requireRegularStat(relative, &before, maximum); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(
		int(parent.Fd()),
		name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open marketplace file %q without following links: %w", relative, err)
	}
	file, err := fileFromDescriptor(fd, relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return nil, fmt.Errorf("inspect opened marketplace file %q: %w", relative, err)
	}
	if err := requireRegularStat(relative, &opened, maximum); err != nil {
		return nil, err
	}
	if !sameStableAnchoredStat(snapshotAnchoredStat(&before), snapshotAnchoredStat(&opened)) {
		return nil, fmt.Errorf("marketplace file %q changed while it was opened", relative)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("read opened marketplace file state %q: %w", relative, err)
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, fmt.Errorf("read marketplace file %q: %w", relative, err)
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("marketplace file %q exceeds the %d-byte bound", relative, maximum)
	}
	if err := verifyStableRegularAfterRead(parent, file, name, relative, snapshotAnchoredStat(&opened), openedInfo.ModTime().UnixNano(), data, maximum); err != nil {
		return nil, err
	}
	return data, nil
}

func verifyStableRegularAfterRead(
	parent, file *os.File,
	name, relative string,
	opened anchoredStat,
	openedModifiedAt int64,
	data []byte,
	maximum int64,
) error {
	var finalOpened unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &finalOpened); err != nil {
		return fmt.Errorf("reinspect opened marketplace file %q: %w", relative, err)
	}
	if err := requireRegularStat(relative, &finalOpened, maximum); err != nil {
		return err
	}
	finalInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("read final marketplace file state %q: %w", relative, err)
	}
	var visible unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &visible, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("reinspect marketplace file %q: %w", relative, err)
	}
	if err := requireRegularStat(relative, &visible, maximum); err != nil {
		return err
	}
	if !sameStableAnchoredStat(opened, snapshotAnchoredStat(&finalOpened)) ||
		!sameStableAnchoredStat(opened, snapshotAnchoredStat(&visible)) ||
		opened.size != int64(len(data)) ||
		finalInfo.ModTime().UnixNano() != openedModifiedAt {
		return fmt.Errorf("marketplace file %q changed while it was read", relative)
	}
	return nil
}

func requireDirectoryStat(relative string, stat *unix.Stat_t) error {
	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFDIR:
		return nil
	case unix.S_IFLNK:
		return fmt.Errorf("marketplace directory %q escapes its anchored root through a symbolic link", relative)
	case unix.S_IFIFO:
		return fmt.Errorf("marketplace directory %q is a FIFO", relative)
	case unix.S_IFSOCK:
		return fmt.Errorf("marketplace directory %q is a socket", relative)
	default:
		return fmt.Errorf("marketplace path %q is not a directory", relative)
	}
}

func requireRegularStat(relative string, stat *unix.Stat_t, maximum int64) error {
	switch stat.Mode & unix.S_IFMT {
	case unix.S_IFREG:
	case unix.S_IFLNK:
		return fmt.Errorf("marketplace file %q escapes its anchored root through a symbolic link", relative)
	case unix.S_IFIFO:
		return fmt.Errorf("marketplace file %q is a FIFO", relative)
	case unix.S_IFSOCK:
		return fmt.Errorf("marketplace file %q is a socket", relative)
	default:
		return fmt.Errorf("marketplace file %q is not a regular file", relative)
	}
	if uint64(stat.Nlink) != 1 {
		return fmt.Errorf("marketplace file %q has %d hard links, want 1", relative, stat.Nlink)
	}
	if stat.Size < 0 || stat.Size > maximum {
		return fmt.Errorf("marketplace file %q size %d exceeds the %d-byte bound", relative, stat.Size, maximum)
	}
	return nil
}

func snapshotAnchoredStat(stat *unix.Stat_t) anchoredStat {
	return anchoredStat{
		identity: identityFromAnchoredStat(stat),
		mode:     uint32(stat.Mode),
		links:    uint64(stat.Nlink),
		size:     stat.Size,
	}
}

func identityFromAnchoredStat(stat *unix.Stat_t) anchoredIdentity {
	return anchoredIdentity{device: fmt.Sprint(stat.Dev), inode: fmt.Sprint(stat.Ino)}
}

func sameAnchoredObject(left, right *unix.Stat_t) bool {
	return identityFromAnchoredStat(left) == identityFromAnchoredStat(right) &&
		left.Mode&unix.S_IFMT == right.Mode&unix.S_IFMT
}

func sameStableAnchoredStat(left, right anchoredStat) bool {
	return left == right
}
