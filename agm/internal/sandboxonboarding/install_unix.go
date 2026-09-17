//go:build darwin || linux

package sandboxonboarding

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"golang.org/x/sys/unix"
)

const (
	privateDirectoryMode = 0o700
	privateFileMode      = 0o600
)

type fileIdentity struct {
	device string
	inode  string
}

type directoryHandle struct {
	file     *os.File
	parent   *directoryHandle
	name     string
	label    string
	identity fileIdentity
	created  bool
}

type temporaryFile struct {
	parent   *directoryHandle
	name     string
	identity fileIdentity
	content  []byte
}

type targetSnapshot struct {
	exists   bool
	identity fileIdentity
	mode     uint32
	uid      uint32
	gid      uint32
	links    uint64
	content  []byte
}

type outputTransaction struct {
	homePath    string
	directories []*directoryHandle
	created     []*directoryHandle
	temporary   *temporaryFile
	hooks       installHooks
	committed   bool
}

func requireSupportedPlatform() error { return nil }

func installOutput(homePath, projectName string, content []byte, hooks installHooks) (result error) {
	transaction, err := openOutputTransaction(homePath)
	if err != nil {
		return err
	}
	defer func() {
		if !transaction.committed {
			result = errors.Join(result, transaction.cleanup(), transaction.close())
			return
		}
		_ = transaction.close()
	}()
	transaction.hooks = hooks

	current := transaction.directories[0]
	for _, component := range []string{".claude", "projects", projectName} {
		current, err = transaction.openOrCreateDirectory(current, component)
		if err != nil {
			return err
		}
	}
	if err := transaction.verifyDirectoryChain(); err != nil {
		return err
	}

	original, err := readTarget(current)
	if err != nil {
		return err
	}
	installed := append([]byte(nil), content...)
	if original.exists {
		installed = append(installed, []byte("\n---\n\n")...)
		installed = append(installed, original.content...)
	}
	if err := transaction.stage(current, installed); err != nil {
		return err
	}
	if err := transaction.commit(current, original); err != nil {
		return err
	}
	return nil
}

func openOutputTransaction(homePath string) (*outputTransaction, error) {
	var before unix.Stat_t
	if err := unix.Lstat(homePath, &before); err != nil {
		return nil, fmt.Errorf("inspect retained HOME %q: %w", homePath, err)
	}
	if err := validateDirectory("retained HOME", &before, false); err != nil {
		return nil, err
	}
	fd, err := unix.Open(
		homePath,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open retained HOME %q without following links: %w", homePath, err)
	}
	file := os.NewFile(uintptr(fd), homePath)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open retained HOME %q: create directory handle", homePath)
	}
	var opened, visible unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return nil, errors.Join(fmt.Errorf("inspect opened retained HOME: %w", err), file.Close())
	}
	if err := unix.Lstat(homePath, &visible); err != nil {
		return nil, errors.Join(fmt.Errorf("reinspect retained HOME: %w", err), file.Close())
	}
	if err := validateSameDirectory("retained HOME", &before, &opened, &visible, false); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	root := &directoryHandle{
		file:     file,
		label:    "retained HOME",
		identity: identityFromStat(&opened),
	}
	return &outputTransaction{homePath: homePath, directories: []*directoryHandle{root}}, nil
}

func (transaction *outputTransaction) openOrCreateDirectory(
	parent *directoryHandle,
	name string,
) (*directoryHandle, error) {
	label := parent.label + "/" + name
	var observed unix.Stat_t
	created := false
	err := unix.Fstatat(int(parent.file.Fd()), name, &observed, unix.AT_SYMLINK_NOFOLLOW)
	if errors.Is(err, unix.ENOENT) {
		if mkdirErr := unix.Mkdirat(int(parent.file.Fd()), name, privateDirectoryMode); mkdirErr != nil {
			if !errors.Is(mkdirErr, unix.EEXIST) {
				return nil, fmt.Errorf("create onboarding directory %q: %w", label, mkdirErr)
			}
		} else {
			created = true
		}
		err = unix.Fstatat(int(parent.file.Fd()), name, &observed, unix.AT_SYMLINK_NOFOLLOW)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect onboarding directory %q: %w", label, err)
	}
	handle := &directoryHandle{
		parent:   parent,
		name:     name,
		label:    label,
		identity: identityFromStat(&observed),
		created:  created,
	}
	if created {
		transaction.created = append(transaction.created, handle)
	}
	if err := validateDirectory(label, &observed, created); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(
		int(parent.file.Fd()), name,
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open onboarding directory %q without following links: %w", label, err)
	}
	file := os.NewFile(uintptr(fd), label)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open onboarding directory %q: create directory handle", label)
	}
	var opened, visible unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return nil, errors.Join(fmt.Errorf("inspect opened onboarding directory %q: %w", label, err), file.Close())
	}
	if err := unix.Fstatat(int(parent.file.Fd()), name, &visible, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, errors.Join(fmt.Errorf("reinspect onboarding directory %q: %w", label, err), file.Close())
	}
	if err := validateSameDirectory(label, &observed, &opened, &visible, created); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	handle.file = file
	transaction.directories = append(transaction.directories, handle)
	return handle, nil
}

func validateSameDirectory(label string, expected, opened, visible *unix.Stat_t, exactMode bool) error {
	for _, stat := range []*unix.Stat_t{expected, opened, visible} {
		if err := validateDirectory(label, stat, exactMode); err != nil {
			return err
		}
	}
	identity := identityFromStat(expected)
	if identityFromStat(opened) != identity || identityFromStat(visible) != identity {
		return fmt.Errorf("onboarding directory %q changed identity while it was opened", label)
	}
	return nil
}

func validateDirectory(label string, stat *unix.Stat_t, exactMode bool) error {
	if uint32(stat.Mode)&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("onboarding directory %q is not a real directory", label)
	}
	if err := validateEffectiveOwner("onboarding directory", label, stat.Uid); err != nil {
		return err
	}
	permissions := uint32(stat.Mode) & 0o777
	if permissions&0o022 != 0 {
		return fmt.Errorf("onboarding directory %q is group- or world-writable (mode %04o)", label, permissions)
	}
	if exactMode && permissions != privateDirectoryMode {
		return fmt.Errorf("created onboarding directory %q has mode %04o, want %04o", label, permissions, privateDirectoryMode)
	}
	return nil
}

func (transaction *outputTransaction) verifyDirectoryChain() error {
	for index, directory := range transaction.directories {
		var opened unix.Stat_t
		if err := unix.Fstat(int(directory.file.Fd()), &opened); err != nil {
			return fmt.Errorf("reinspect opened onboarding directory %q: %w", directory.label, err)
		}
		if err := validateDirectory(directory.label, &opened, directory.created); err != nil {
			return err
		}
		if identityFromStat(&opened) != directory.identity {
			return fmt.Errorf("opened onboarding directory %q changed identity", directory.label)
		}

		var visible unix.Stat_t
		if index == 0 {
			if err := unix.Lstat(transaction.homePath, &visible); err != nil {
				return fmt.Errorf("reinspect visible retained HOME: %w", err)
			}
		} else if err := unix.Fstatat(
			int(directory.parent.file.Fd()), directory.name, &visible, unix.AT_SYMLINK_NOFOLLOW,
		); err != nil {
			return fmt.Errorf("reinspect visible onboarding directory %q: %w", directory.label, err)
		}
		if err := validateDirectory(directory.label, &visible, directory.created); err != nil {
			return err
		}
		if identityFromStat(&visible) != directory.identity {
			return fmt.Errorf("visible onboarding directory %q changed identity", directory.label)
		}
	}
	return nil
}

func readTarget(parent *directoryHandle) (targetSnapshot, error) {
	return readRegular(parent, outputName, false)
}

func readRegular(parent *directoryHandle, name string, exactPrivateMode bool) (targetSnapshot, error) {
	label := parent.label + "/" + name
	before, exists, err := inspectRegularEntry(parent, name, label, exactPrivateMode)
	if err != nil {
		return targetSnapshot{}, err
	}
	if !exists {
		return targetSnapshot{}, nil
	}
	file, err := openRegularEntry(parent, name, label, &before, exactPrivateMode)
	if err != nil {
		return targetSnapshot{}, err
	}
	content, openedAfter, visible, err := readOpenedRegular(parent, name, label, file)
	if err != nil {
		return targetSnapshot{}, err
	}
	if err := validateReadRegular(label, &before, &openedAfter, &visible, content, exactPrivateMode); err != nil {
		return targetSnapshot{}, err
	}
	return snapshotFromStat(&before, content), nil
}

func inspectRegularEntry(
	parent *directoryHandle,
	name, label string,
	exactPrivateMode bool,
) (unix.Stat_t, bool, error) {
	var before unix.Stat_t
	if err := unix.Fstatat(int(parent.file.Fd()), name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return unix.Stat_t{}, false, nil
		}
		return unix.Stat_t{}, false, fmt.Errorf("inspect onboarding file %q: %w", label, err)
	}
	if err := validateRegular(label, &before, exactPrivateMode); err != nil {
		return unix.Stat_t{}, false, err
	}
	return before, true, nil
}

func openRegularEntry(
	parent *directoryHandle,
	name, label string,
	before *unix.Stat_t,
	exactPrivateMode bool,
) (*os.File, error) {
	fd, err := unix.Openat(
		int(parent.file.Fd()), name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open onboarding file %q without following links: %w", label, err)
	}
	file := os.NewFile(uintptr(fd), label)
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open onboarding file %q: create file handle", label)
	}
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return nil, errors.Join(fmt.Errorf("inspect opened onboarding file %q: %w", label, err), file.Close())
	}
	if err := validateRegular(label, &opened, exactPrivateMode); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if !sameRegularState(before, &opened) {
		return nil, errors.Join(fmt.Errorf("onboarding file %q changed while it was opened", label), file.Close())
	}
	return file, nil
}

func readOpenedRegular(
	parent *directoryHandle,
	name, label string,
	file *os.File,
) ([]byte, unix.Stat_t, unix.Stat_t, error) {
	content, readErr := io.ReadAll(file)
	var openedAfter, visible unix.Stat_t
	openedErr := unix.Fstat(int(file.Fd()), &openedAfter)
	visibleErr := unix.Fstatat(int(parent.file.Fd()), name, &visible, unix.AT_SYMLINK_NOFOLLOW)
	closeErr := file.Close()
	err := errors.Join(
		wrapError("read onboarding file "+label, readErr),
		wrapError("reinspect opened onboarding file "+label, openedErr),
		wrapError("reinspect visible onboarding file "+label, visibleErr),
		closeErr,
	)
	if err != nil {
		return nil, unix.Stat_t{}, unix.Stat_t{}, err
	}
	return content, openedAfter, visible, nil
}

func validateReadRegular(
	label string,
	before, openedAfter, visible *unix.Stat_t,
	content []byte,
	exactPrivateMode bool,
) error {
	if err := validateRegular(label, openedAfter, exactPrivateMode); err != nil {
		return err
	}
	if err := validateRegular(label, visible, exactPrivateMode); err != nil {
		return err
	}
	if !sameRegularState(before, openedAfter) {
		return fmt.Errorf("onboarding file %q changed while it was read", label)
	}
	if !sameRegularState(before, visible) {
		return fmt.Errorf("onboarding file %q changed while it was read", label)
	}
	if before.Size != int64(len(content)) {
		return fmt.Errorf("onboarding file %q changed while it was read", label)
	}
	return nil
}

func validateRegular(label string, stat *unix.Stat_t, exactPrivateMode bool) error {
	if uint32(stat.Mode)&unix.S_IFMT != unix.S_IFREG {
		return fmt.Errorf("onboarding file %q is not a regular file", label)
	}
	if err := validateEffectiveOwner("onboarding file", label, stat.Uid); err != nil {
		return err
	}
	if uint64(stat.Nlink) != 1 {
		return fmt.Errorf("onboarding file %q has %d hard links, want 1", label, stat.Nlink)
	}
	mode := uint32(stat.Mode)
	permissions := mode & 0o777
	if permissions&0o022 != 0 {
		return fmt.Errorf("onboarding file %q is group- or world-writable (mode %04o)", label, permissions)
	}
	if mode&(unix.S_ISUID|unix.S_ISGID|unix.S_ISVTX) != 0 {
		return fmt.Errorf("onboarding file %q has unsafe special mode bits", label)
	}
	if exactPrivateMode && permissions != privateFileMode {
		return fmt.Errorf("onboarding file %q has mode %04o, want %04o", label, permissions, privateFileMode)
	}
	return nil
}

func (transaction *outputTransaction) stage(parent *directoryHandle, content []byte) error {
	for range 100 {
		random := make([]byte, 12)
		if _, err := rand.Read(random); err != nil {
			return fmt.Errorf("generate onboarding temporary filename: %w", err)
		}
		name := ".CLAUDE.md.agm-" + hex.EncodeToString(random) + ".tmp"
		fd, err := unix.Openat(
			int(parent.file.Fd()), name,
			unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
			privateFileMode,
		)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return fmt.Errorf("create onboarding temporary file: %w", err)
		}
		file := os.NewFile(uintptr(fd), name)
		if file == nil {
			_ = unix.Close(fd)
			return fmt.Errorf("create onboarding temporary file: create file handle")
		}
		if err := transaction.authenticateTemporary(parent, name, file, content); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("allocate a unique onboarding temporary filename")
}

func (transaction *outputTransaction) authenticateTemporary(
	parent *directoryHandle,
	name string,
	file *os.File,
	content []byte,
) error {
	if err := transaction.registerTemporary(parent, name, file, content); err != nil {
		return err
	}
	if err := writeTemporary(file, content); err != nil {
		return errors.Join(err, file.Close())
	}
	return transaction.finishTemporary(parent, name, file, content)
}

func (transaction *outputTransaction) registerTemporary(
	parent *directoryHandle,
	name string,
	file *os.File,
	content []byte,
) error {
	fd := int(file.Fd())
	var opened, visible unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		return errors.Join(fmt.Errorf("inspect onboarding temporary file: %w", err), file.Close())
	}
	transaction.temporary = &temporaryFile{
		parent:   parent,
		name:     name,
		identity: identityFromStat(&opened),
		content:  append([]byte(nil), content...),
	}
	if err := unix.Fstatat(int(parent.file.Fd()), name, &visible, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return errors.Join(fmt.Errorf("reinspect onboarding temporary file: %w", err), file.Close())
	}
	if err := validateRegular("temporary "+name, &opened, false); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := validateRegular("temporary "+name, &visible, false); err != nil {
		return errors.Join(err, file.Close())
	}
	if !sameRegularState(&opened, &visible) {
		return errors.Join(fmt.Errorf("onboarding temporary file changed while it was created"), file.Close())
	}
	return nil
}

func writeTemporary(file *os.File, content []byte) error {
	if err := unix.Fchmod(int(file.Fd()), privateFileMode); err != nil {
		return fmt.Errorf("set onboarding temporary file mode: %w", err)
	}
	if err := writeAll(file, content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync onboarding temporary file: %w", err)
	}
	return nil
}

func (transaction *outputTransaction) finishTemporary(
	parent *directoryHandle,
	name string,
	file *os.File,
	content []byte,
) error {
	fd := int(file.Fd())
	var finalOpened, finalVisible unix.Stat_t
	openedErr := unix.Fstat(fd, &finalOpened)
	visibleErr := unix.Fstatat(int(parent.file.Fd()), name, &finalVisible, unix.AT_SYMLINK_NOFOLLOW)
	closeErr := file.Close()
	err := errors.Join(
		wrapError("reinspect opened onboarding temporary file", openedErr),
		wrapError("reinspect visible onboarding temporary file", visibleErr),
		closeErr,
	)
	if err != nil {
		return err
	}
	if err := validateRegular("temporary "+name, &finalOpened, true); err != nil {
		return err
	}
	if err := validateRegular("temporary "+name, &finalVisible, true); err != nil {
		return err
	}
	if identityFromStat(&finalOpened) != transaction.temporary.identity {
		return fmt.Errorf("onboarding temporary file changed while it was written")
	}
	if !sameRegularState(&finalOpened, &finalVisible) {
		return fmt.Errorf("onboarding temporary file changed while it was written")
	}
	if finalOpened.Size != int64(len(content)) {
		return fmt.Errorf("onboarding temporary file changed while it was written")
	}
	return nil
}

func writeAll(file *os.File, content []byte) error {
	for len(content) > 0 {
		written, err := file.Write(content)
		if err != nil {
			return fmt.Errorf("write onboarding temporary file: %w", err)
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		content = content[written:]
	}
	return nil
}

func (transaction *outputTransaction) commit(parent *directoryHandle, original targetSnapshot) error {
	if transaction.hooks.beforeCommit != nil {
		transaction.hooks.beforeCommit()
	}
	if err := transaction.verifyDirectoryChain(); err != nil {
		return err
	}
	current, err := readTarget(parent)
	if err != nil {
		return err
	}
	if !sameSnapshot(original, current) {
		return fmt.Errorf("onboarding target changed before commit")
	}
	staged, err := readRegular(parent, transaction.temporary.name, true)
	if err != nil {
		return err
	}
	if !staged.exists || staged.identity != transaction.temporary.identity ||
		!bytes.Equal(staged.content, transaction.temporary.content) {
		return fmt.Errorf("onboarding temporary file changed before commit")
	}
	if err := unix.Renameat(
		int(parent.file.Fd()), transaction.temporary.name,
		int(parent.file.Fd()), outputName,
	); err != nil {
		return fmt.Errorf("atomically install onboarding file: %w", err)
	}
	transaction.temporary = nil
	transaction.committed = true
	return nil
}

func sameSnapshot(left, right targetSnapshot) bool {
	if left.exists != right.exists {
		return false
	}
	if !left.exists {
		return true
	}
	return left.identity == right.identity && left.mode == right.mode &&
		left.uid == right.uid && left.gid == right.gid && left.links == right.links &&
		bytes.Equal(left.content, right.content)
}

func snapshotFromStat(stat *unix.Stat_t, content []byte) targetSnapshot {
	return targetSnapshot{
		exists:   true,
		identity: identityFromStat(stat),
		mode:     uint32(stat.Mode),
		uid:      stat.Uid,
		gid:      stat.Gid,
		links:    uint64(stat.Nlink),
		content:  append([]byte(nil), content...),
	}
}

func sameRegularState(left, right *unix.Stat_t) bool {
	return identityFromStat(left) == identityFromStat(right) &&
		uint32(left.Mode) == uint32(right.Mode) && left.Uid == right.Uid && left.Gid == right.Gid &&
		uint64(left.Nlink) == uint64(right.Nlink) && left.Size == right.Size
}

func identityFromStat(stat *unix.Stat_t) fileIdentity {
	return fileIdentity{device: fmt.Sprint(stat.Dev), inode: fmt.Sprint(stat.Ino)}
}

func effectiveUID() uint32 {
	// #nosec G115 -- Geteuid carries the platform uid_t bits through int; on
	// 32-bit Unix, valid high UIDs appear negative and must retain those bits.
	return uint32(os.Geteuid())
}

func validateEffectiveOwner(kind, label string, actual uint32) error {
	expected := effectiveUID()
	if actual != expected {
		return fmt.Errorf("%s %q is owned by uid %d, want effective uid %d", kind, label, actual, expected)
	}
	return nil
}

func wrapError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func (transaction *outputTransaction) cleanup() error {
	var cleanupErr error
	if transaction.temporary != nil {
		cleanupErr = errors.Join(cleanupErr, transaction.cleanupTemporary())
	}
	for _, directory := range slices.Backward(transaction.created) {
		cleanupErr = errors.Join(cleanupErr, cleanupCreatedDirectory(directory))
	}
	return cleanupErr
}

func (transaction *outputTransaction) cleanupTemporary() error {
	temporary := transaction.temporary
	var visible unix.Stat_t
	if err := unix.Fstatat(
		int(temporary.parent.file.Fd()), temporary.name, &visible, unix.AT_SYMLINK_NOFOLLOW,
	); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return fmt.Errorf("cleanup onboarding temporary file: inspect: %w", err)
	}
	if !matchesCreatedEntry(&visible, temporary.identity, unix.S_IFREG) {
		return fmt.Errorf("cleanup onboarding temporary file: visible entry no longer matches the created file")
	}
	if err := unix.Unlinkat(int(temporary.parent.file.Fd()), temporary.name, 0); err != nil {
		return fmt.Errorf("cleanup onboarding temporary file: %w", err)
	}
	transaction.temporary = nil
	return nil
}

func cleanupCreatedDirectory(directory *directoryHandle) error {
	var visible unix.Stat_t
	if err := unix.Fstatat(
		int(directory.parent.file.Fd()), directory.name, &visible, unix.AT_SYMLINK_NOFOLLOW,
	); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		return fmt.Errorf("cleanup onboarding directory %q: inspect: %w", directory.label, err)
	}
	if !matchesCreatedEntry(&visible, directory.identity, unix.S_IFDIR) {
		return fmt.Errorf("cleanup onboarding directory %q: visible entry no longer matches the created directory", directory.label)
	}
	if err := unix.Unlinkat(int(directory.parent.file.Fd()), directory.name, unix.AT_REMOVEDIR); err != nil {
		return fmt.Errorf("cleanup onboarding directory %q: %w", directory.label, err)
	}
	return nil
}

func (transaction *outputTransaction) close() error {
	var closeErr error
	for _, directory := range slices.Backward(transaction.directories) {
		if directory.file != nil {
			closeErr = errors.Join(closeErr, directory.file.Close())
		}
	}
	return closeErr
}

func matchesCreatedEntry(stat *unix.Stat_t, identity fileIdentity, fileType uint32) bool {
	return identityFromStat(stat) == identity &&
		uint32(stat.Mode)&unix.S_IFMT == fileType && stat.Uid == effectiveUID()
}
