package testcontext

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sys/unix"
)

type createdAuthNode struct {
	relativePath string
	identity     *os.File
}

type stagedAuthNode struct {
	base         string
	relativePath string
	info         os.FileInfo
	ledgerIndex  int
}

type authProjectionTransaction struct {
	home             string
	selectedHomeInfo os.FileInfo
	root             *os.Root
	lock             *os.File
	created          []createdAuthNode
	closed           bool
}

func newAuthProjectionTransaction(
	home string,
	expected os.FileInfo,
) (*authProjectionTransaction, error) {
	before, err := validatePreparedSelectedHome(home, expected)
	if err != nil {
		return nil, err
	}
	lock, err := openProjectionLock(home)
	if err != nil {
		return nil, err
	}
	root, err := openAuthenticatedProjectionRoot(home, before, lock)
	if err != nil {
		return nil, err
	}

	return &authProjectionTransaction{
		home:             home,
		selectedHomeInfo: expected,
		root:             root,
		lock:             lock,
	}, nil
}

func validatePreparedSelectedHome(home string, expected os.FileInfo) (os.FileInfo, error) {
	before, err := os.Lstat(home)
	if err != nil {
		return nil, authPathError("inspect selected home", ".", err)
	}
	if err := validateOwnedDirectoryInfo(before, true, "selected home"); err != nil {
		return nil, err
	}
	if !sameAuthFileState(expected, before) {
		return nil, errors.New("selected home changed after preparation")
	}
	return before, nil
}

func openProjectionLock(home string) (*os.File, error) {
	lock, err := os.OpenFile(home, os.O_RDONLY|unix.O_NONBLOCK|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, authPathError("open selected home", ".", err)
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = lock.Close()
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, errors.New("selected home already has an active auth projection")
		}
		return nil, authPathError("lock selected home", ".", err)
	}
	return lock, nil
}

func openAuthenticatedProjectionRoot(home string, before os.FileInfo, lock *os.File) (*os.Root, error) {
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, errors.Join(
			authPathError("open selected home root", ".", err),
			closeProjectionLock(lock),
		)
	}
	opened, err := lock.Stat()
	if err != nil {
		return nil, abortProjectionRootSetup(
			root,
			lock,
			authPathError("inspect opened selected home", ".", err),
		)
	}
	rooted, err := root.Stat(".")
	if err != nil {
		return nil, abortProjectionRootSetup(
			root,
			lock,
			authPathError("inspect selected home root", ".", err),
		)
	}
	after, err := os.Lstat(home)
	if err != nil {
		return nil, abortProjectionRootSetup(
			root,
			lock,
			authPathError("reinspect selected home", ".", err),
		)
	}
	for _, info := range []os.FileInfo{opened, rooted, after} {
		if err := validateOwnedDirectoryInfo(info, true, "selected home"); err != nil {
			return nil, abortProjectionRootSetup(root, lock, err)
		}
	}
	if !sameAuthFileState(before, opened) || !sameAuthFileState(opened, rooted) ||
		!sameAuthFileState(rooted, after) {
		return nil, abortProjectionRootSetup(
			root,
			lock,
			errors.New("selected home changed while opening transaction root"),
		)
	}
	return root, nil
}

func abortProjectionRootSetup(root *os.Root, lock *os.File, cause error) error {
	_ = root.Close()
	return errors.Join(cause, closeProjectionLock(lock))
}

func (tx *authProjectionTransaction) preflight(
	links []preparedCredentialLink,
	snapshots []preparedConfigSnapshot,
) ([]preparedAuthDirectory, error) {
	if err := tx.verifyRootVisible(); err != nil {
		return nil, err
	}
	preflighted := make([]preparedAuthDirectory, 0, len(authNamespacePaths))
	for _, relativePath := range authNamespacePaths {
		info, err := tx.root.Lstat(relativePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				preflighted = append(preflighted, preparedAuthDirectory{relativePath: relativePath})
				continue
			}
			return nil, authPathError("inspect destination directory", relativePath, err)
		}
		if err := validateOwnedDirectoryInfo(info, true, "destination directory "+relativePath); err != nil {
			return nil, err
		}
		preflighted = append(preflighted, preparedAuthDirectory{
			relativePath: relativePath,
			existingInfo: info,
		})
	}

	for _, relativePath := range allAuthLeafPaths() {
		_, err := tx.root.Lstat(relativePath)
		if err == nil {
			return nil, fmt.Errorf("destination leaf %s already exists", relativePath)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, authPathError("inspect destination leaf", relativePath, err)
		}
	}

	directories := make([]preparedAuthDirectory, 0, len(preflighted))
	for _, directory := range preflighted {
		if authDirectoryNeeded(directory.relativePath, links, snapshots) {
			directories = append(directories, directory)
		}
	}
	return directories, nil
}

func (tx *authProjectionTransaction) apply(plan authProjectionPlan, hook authInstallHook) error {
	if err := tx.verifyRootVisible(); err != nil {
		return err
	}
	for _, directory := range plan.directories {
		if err := tx.ensureDirectory(directory); err != nil {
			return err
		}
	}
	for _, link := range plan.links {
		if hook != nil {
			if err := hook(link.relativePath); err != nil {
				return fmt.Errorf("install hook %s: %w", link.relativePath, err)
			}
		}
		if err := tx.installCredentialLink(plan.hostHome, link); err != nil {
			return err
		}
	}
	for _, snapshot := range plan.snapshots {
		if hook != nil {
			if err := hook(snapshot.relativePath); err != nil {
				return fmt.Errorf("install hook %s: %w", snapshot.relativePath, err)
			}
		}
		if err := tx.installConfigSnapshot(snapshot); err != nil {
			return err
		}
	}
	return nil
}

func (tx *authProjectionTransaction) verifyRootVisible() error {
	rooted, err := tx.root.Stat(".")
	if err != nil {
		return authPathError("inspect selected home root", ".", err)
	}
	visible, err := os.Lstat(tx.home)
	if err != nil {
		return authPathError("inspect selected home", ".", err)
	}
	for _, info := range []os.FileInfo{rooted, visible} {
		if err := validateOwnedDirectoryInfo(info, true, "selected home"); err != nil {
			return err
		}
		if !os.SameFile(tx.selectedHomeInfo, info) {
			return errors.New("selected home changed after transaction root opened")
		}
	}
	if !os.SameFile(rooted, visible) {
		return errors.New("selected home path no longer names the transaction root")
	}
	return nil
}

func (tx *authProjectionTransaction) ensureDirectory(directory preparedAuthDirectory) error {
	if directory.existingInfo != nil {
		return tx.verifyExistingDirectory(directory)
	}
	return tx.createDirectory(directory.relativePath)
}

func (tx *authProjectionTransaction) verifyExistingDirectory(directory preparedAuthDirectory) error {
	info, err := tx.root.Lstat(directory.relativePath)
	if err != nil {
		return authPathError("reinspect destination directory", directory.relativePath, err)
	}
	if err := validateOwnedDirectoryInfo(info, true, "destination directory "+directory.relativePath); err != nil {
		return err
	}
	if !os.SameFile(directory.existingInfo, info) {
		return fmt.Errorf("destination directory %s changed after preflight", directory.relativePath)
	}
	return nil
}

func (tx *authProjectionTransaction) createDirectory(relativePath string) error {
	parentPath := filepath.Dir(relativePath)
	parent, err := tx.openDestinationParent(parentPath)
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := ensureAuthLeafAbsent(tx.root, relativePath); err != nil {
		return err
	}

	staged, err := tx.stageDirectory(parent, parentPath, relativePath)
	if err != nil {
		return err
	}
	return tx.installStagedDirectory(parent, relativePath, staged)
}

func (tx *authProjectionTransaction) stageDirectory(
	parent *os.File,
	parentPath, relativePath string,
) (stagedAuthNode, error) {
	stageBase, err := createStagedDirectory(parent, filepath.Base(relativePath))
	if err != nil {
		return stagedAuthNode{}, authPathError("stage destination directory", relativePath, err)
	}
	stagePath := filepath.Join(parentPath, stageBase)
	openedFD, err := unix.Openat(
		int(parent.Fd()),
		stageBase,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK|unix.O_DIRECTORY|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return stagedAuthNode{}, cleanupUntrackedAuthNode(
			parent,
			stageBase,
			true,
			authPathError("open staged destination directory", relativePath, err),
		)
	}
	opened := os.NewFile(uintptr(openedFD), relativePath)
	if opened == nil {
		_ = unix.Close(openedFD)
		return stagedAuthNode{}, cleanupUntrackedAuthNode(
			parent,
			stageBase,
			true,
			fmt.Errorf("open staged destination directory %s", relativePath),
		)
	}
	info, statErr := opened.Stat()
	if statErr != nil {
		closeErr := opened.Close()
		return stagedAuthNode{}, cleanupUntrackedAuthNode(
			parent,
			stageBase,
			true,
			errors.Join(
				authPathError("inspect staged destination directory", relativePath, statErr),
				sanitizeCloseError("close staged destination directory", relativePath, closeErr),
			),
		)
	}
	nodeIndex := len(tx.created)
	tx.created = append(tx.created, createdAuthNode{
		relativePath: stagePath,
		identity:     opened,
	})
	staged := stagedAuthNode{
		base:         stageBase,
		relativePath: stagePath,
		info:         info,
		ledgerIndex:  nodeIndex,
	}
	return staged, nil
}

func (tx *authProjectionTransaction) installStagedDirectory(
	parent *os.File,
	relativePath string,
	staged stagedAuthNode,
) error {
	if err := validateOwnedDirectoryInfo(staged.info, true, "destination directory "+relativePath); err != nil {
		return err
	}
	visible, err := tx.root.Lstat(staged.relativePath)
	if err != nil {
		return authPathError("inspect staged destination directory", relativePath, err)
	}
	if !os.SameFile(staged.info, visible) {
		return fmt.Errorf("staged destination directory %s changed before installation", relativePath)
	}

	if err := renameNoReplace(
		int(parent.Fd()), staged.base,
		int(parent.Fd()), filepath.Base(relativePath),
	); err != nil {
		return authPathError("install destination directory", relativePath, err)
	}
	tx.created[staged.ledgerIndex].relativePath = relativePath
	current, err := tx.root.Lstat(relativePath)
	if err != nil {
		return authPathError("verify destination directory", relativePath, err)
	}
	if !os.SameFile(staged.info, current) {
		return fmt.Errorf("destination directory %s changed during installation", relativePath)
	}
	return validateOwnedDirectoryInfo(current, true, "destination directory "+relativePath)
}

func (tx *authProjectionTransaction) installCredentialLink(
	hostHome string,
	link preparedCredentialLink,
) error {
	sourcePath, err := validatePreparedCredentialSource(hostHome, link)
	if err != nil {
		return err
	}

	parentPath := filepath.Dir(link.relativePath)
	parent, err := tx.openDestinationParent(parentPath)
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := ensureAuthLeafAbsent(tx.root, link.relativePath); err != nil {
		return err
	}

	staged, err := tx.stageCredentialLink(parent, parentPath, link.relativePath, sourcePath)
	if err != nil {
		return err
	}
	return tx.installStagedCredentialLink(parent, link.relativePath, sourcePath, staged)
}

func validatePreparedCredentialSource(hostHome string, link preparedCredentialLink) (string, error) {
	file, current, present, err := openApprovedAuthSource(hostHome, link.relativePath, true)
	if err != nil {
		return "", err
	}
	if !present {
		return "", fmt.Errorf("credential source %s changed after preparation", link.relativePath)
	}
	if err := file.Close(); err != nil {
		return "", authPathError("close credential source", link.relativePath, err)
	}
	if !sameAuthFileState(link.sourceInfo, current) {
		return "", fmt.Errorf("credential source %s changed after preparation", link.relativePath)
	}
	return filepath.Join(hostHome, link.relativePath), nil
}

func (tx *authProjectionTransaction) stageCredentialLink(
	parent *os.File,
	parentPath, relativePath, sourcePath string,
) (stagedAuthNode, error) {
	stageBase, err := createStagedSymlink(parent, filepath.Base(relativePath), sourcePath)
	if err != nil {
		return stagedAuthNode{}, authPathError("stage credential link", relativePath, err)
	}
	stagePath := filepath.Join(parentPath, stageBase)
	opened, err := openSymlinkAt(int(parent.Fd()), stageBase)
	if err != nil {
		return stagedAuthNode{}, cleanupUntrackedAuthNode(
			parent,
			stageBase,
			false,
			authPathError("open staged credential link", relativePath, err),
		)
	}
	info, statErr := opened.Stat()
	if statErr != nil {
		closeErr := opened.Close()
		return stagedAuthNode{}, cleanupUntrackedAuthNode(
			parent,
			stageBase,
			false,
			errors.Join(
				authPathError("inspect staged credential link", relativePath, statErr),
				sanitizeCloseError("close staged credential link", relativePath, closeErr),
			),
		)
	}
	nodeIndex := len(tx.created)
	tx.created = append(tx.created, createdAuthNode{
		relativePath: stagePath,
		identity:     opened,
	})
	staged := stagedAuthNode{
		base:         stageBase,
		relativePath: stagePath,
		info:         info,
		ledgerIndex:  nodeIndex,
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return staged, fmt.Errorf("staged credential destination %s is not a link", relativePath)
	}
	visible, err := tx.root.Lstat(stagePath)
	if err != nil {
		return staged, authPathError("inspect staged credential link", relativePath, err)
	}
	if !os.SameFile(info, visible) {
		return staged, fmt.Errorf("staged credential destination %s changed before installation", relativePath)
	}
	target, err := tx.root.Readlink(stagePath)
	if err != nil {
		return staged, authPathError("read staged credential link", relativePath, err)
	}
	if target != sourcePath {
		return staged, fmt.Errorf("staged credential destination %s has an unexpected target", relativePath)
	}
	return staged, nil
}

func (tx *authProjectionTransaction) installStagedCredentialLink(
	parent *os.File,
	relativePath, sourcePath string,
	staged stagedAuthNode,
) error {
	if err := renameNoReplace(
		int(parent.Fd()), staged.base,
		int(parent.Fd()), filepath.Base(relativePath),
	); err != nil {
		return authPathError("install credential link", relativePath, err)
	}
	tx.created[staged.ledgerIndex].relativePath = relativePath
	currentLink, err := tx.root.Lstat(relativePath)
	if err != nil {
		return authPathError("verify credential link", relativePath, err)
	}
	if !os.SameFile(staged.info, currentLink) || currentLink.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("credential destination %s changed during installation", relativePath)
	}
	target, err := tx.root.Readlink(relativePath)
	if err != nil {
		return authPathError("read credential link", relativePath, err)
	}
	if target != sourcePath {
		return fmt.Errorf("credential destination %s has an unexpected target", relativePath)
	}
	return nil
}

func (tx *authProjectionTransaction) installConfigSnapshot(snapshot preparedConfigSnapshot) error {
	parentPath := filepath.Dir(snapshot.relativePath)
	parent, err := tx.openDestinationParent(parentPath)
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := ensureAuthLeafAbsent(tx.root, snapshot.relativePath); err != nil {
		return err
	}

	fd, err := unix.Openat(
		int(parent.Fd()),
		filepath.Base(snapshot.relativePath),
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0600,
	)
	if err != nil {
		return authPathError("create configuration snapshot", snapshot.relativePath, err)
	}
	file := os.NewFile(uintptr(fd), snapshot.relativePath)
	if file == nil {
		_ = unix.Close(fd)
		return cleanupUntrackedAuthNode(
			parent,
			filepath.Base(snapshot.relativePath),
			false,
			fmt.Errorf("create configuration snapshot %s", snapshot.relativePath),
		)
	}
	info, err := file.Stat()
	if err != nil {
		closeErr := file.Close()
		return cleanupUntrackedAuthNode(
			parent,
			filepath.Base(snapshot.relativePath),
			false,
			errors.Join(
				authPathError("inspect configuration snapshot", snapshot.relativePath, err),
				sanitizeCloseError("close configuration snapshot", snapshot.relativePath, closeErr),
			),
		)
	}
	tx.created = append(tx.created, createdAuthNode{
		relativePath: snapshot.relativePath,
		identity:     file,
	})
	if !info.Mode().IsRegular() {
		return fmt.Errorf("configuration snapshot %s is not a regular file", snapshot.relativePath)
	}
	if err := file.Chmod(0600); err != nil {
		return authPathError("secure configuration snapshot", snapshot.relativePath, err)
	}
	if _, err := file.Write(snapshot.data); err != nil {
		return authPathError("write configuration snapshot", snapshot.relativePath, err)
	}
	if err := file.Sync(); err != nil {
		return authPathError("sync configuration snapshot", snapshot.relativePath, err)
	}

	visible, err := tx.root.Lstat(snapshot.relativePath)
	if err != nil {
		return authPathError("verify configuration snapshot", snapshot.relativePath, err)
	}
	if !os.SameFile(info, visible) || !visible.Mode().IsRegular() || visible.Mode().Perm() != 0600 {
		return fmt.Errorf("configuration snapshot %s failed identity or mode verification", snapshot.relativePath)
	}
	return nil
}

func (tx *authProjectionTransaction) openDestinationParent(relativePath string) (*os.File, error) {
	if err := tx.verifyRootVisible(); err != nil {
		return nil, err
	}
	return tx.openRootedDestinationParent(relativePath)
}

func (tx *authProjectionTransaction) openRootedDestinationParent(relativePath string) (*os.File, error) {
	if relativePath != "." {
		current := ""
		for component := range strings.SplitSeq(filepath.ToSlash(relativePath), "/") {
			current = filepath.Join(current, component)
			info, err := tx.root.Lstat(current)
			if err != nil {
				return nil, authPathError("inspect destination directory", relativePath, err)
			}
			if err := validateOwnedDirectoryInfo(info, true, "destination directory "+current); err != nil {
				return nil, err
			}
		}
	}

	parent, err := tx.root.Open(relativePath)
	if err != nil {
		return nil, authPathError("open destination directory", relativePath, err)
	}
	opened, err := parent.Stat()
	if err != nil {
		_ = parent.Close()
		return nil, authPathError("inspect opened destination directory", relativePath, err)
	}
	label := "destination directory " + relativePath
	if relativePath == "." {
		label = "selected home"
	}
	if err := validateOwnedDirectoryInfo(opened, true, label); err != nil {
		_ = parent.Close()
		return nil, err
	}
	visible, err := tx.root.Lstat(relativePath)
	if err != nil {
		_ = parent.Close()
		return nil, authPathError("reinspect destination directory", relativePath, err)
	}
	if err := validateOwnedDirectoryInfo(visible, true, label); err != nil {
		_ = parent.Close()
		return nil, err
	}
	if !os.SameFile(opened, visible) {
		_ = parent.Close()
		return nil, fmt.Errorf("destination directory %s changed while opening", relativePath)
	}
	return parent, nil
}

func ensureAuthLeafAbsent(root *os.Root, relativePath string) error {
	_, err := root.Lstat(relativePath)
	if err == nil {
		return fmt.Errorf("destination leaf %s already exists", relativePath)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return authPathError("inspect destination leaf", relativePath, err)
	}
	return nil
}

func createStagedDirectory(parent *os.File, destinationBase string) (string, error) {
	return allocateStagedName(destinationBase, func(stageBase string) error {
		return unix.Mkdirat(int(parent.Fd()), stageBase, 0700)
	})
}

func createStagedSymlink(parent *os.File, destinationBase, target string) (string, error) {
	return allocateStagedName(destinationBase, func(stageBase string) error {
		return unix.Symlinkat(target, int(parent.Fd()), stageBase)
	})
}

func allocateStagedName(destinationBase string, create func(string) error) (string, error) {
	for range 8 {
		stageBase, err := randomAuthNodeName(destinationBase, "stage")
		if err != nil {
			return "", err
		}
		if err := create(stageBase); err == nil {
			return stageBase, nil
		} else if !errors.Is(err, unix.EEXIST) {
			return "", err
		}
	}
	return "", errors.New("could not allocate a unique staged auth node")
}

func randomAuthNodeName(base, purpose string) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate %s name: %w", purpose, err)
	}
	return "." + base + ".agm-auth-" + purpose + "-" + hex.EncodeToString(entropy[:]), nil
}

func openSymlinkAt(directoryFD int, name string) (*os.File, error) {
	fd, err := unix.Openat(directoryFD, name, symlinkOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, unix.EBADF
	}
	return file, nil
}

func cleanupUntrackedAuthNode(
	parent *os.File,
	base string,
	directory bool,
	cause error,
) error {
	flags := 0
	if directory {
		flags = unix.AT_REMOVEDIR
	}
	removeErr := unix.Unlinkat(int(parent.Fd()), base, flags)
	if errors.Is(removeErr, unix.ENOENT) {
		removeErr = nil
	}
	if removeErr != nil {
		return errors.Join(cause, authPathError("remove untracked staged auth node", ".", removeErr))
	}
	return cause
}

func (tx *authProjectionTransaction) rollback() error {
	var rollbackErr error
	for _, node := range slices.Backward(tx.created) {
		identity, err := node.retainedIdentity()
		if err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
			continue
		}
		if identity.IsDir() {
			empty, err := tx.rollbackDirectoryEmpty(node, identity)
			if err != nil {
				rollbackErr = errors.Join(rollbackErr, err)
				continue
			}
			if !empty {
				rollbackErr = errors.Join(rollbackErr, errors.New("preserved non-empty rollback directory"))
				continue
			}
		}
		if err := tx.quarantineAndRemove(node, identity); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

func (node createdAuthNode) retainedIdentity() (os.FileInfo, error) {
	if node.identity == nil {
		return nil, fmt.Errorf("created auth node %s has no retained identity", node.relativePath)
	}
	info, err := node.identity.Stat()
	if err != nil {
		return nil, authPathError("inspect retained created auth node", node.relativePath, err)
	}
	return info, nil
}

func (tx *authProjectionTransaction) rollbackDirectoryEmpty(
	node createdAuthNode,
	identity os.FileInfo,
) (bool, error) {
	current, err := tx.root.Lstat(node.relativePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, authPathError("inspect rollback directory", ".", err)
	}
	if !os.SameFile(identity, current) {
		return false, errors.New("preserved rollback node whose identity changed")
	}
	directory, err := tx.root.Open(node.relativePath)
	if err != nil {
		return false, authPathError("open rollback directory", ".", err)
	}
	opened, err := directory.Stat()
	if err != nil {
		_ = directory.Close()
		return false, authPathError("inspect opened rollback directory", ".", err)
	}
	if !os.SameFile(identity, opened) {
		_ = directory.Close()
		return false, errors.New("preserved rollback node whose identity changed")
	}
	_, readErr := directory.Readdirnames(1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return false, errors.Join(
			authPathError("read rollback directory", ".", readErr),
			sanitizeCloseError("close rollback directory", ".", closeErr),
		)
	}
	if closeErr != nil {
		return false, authPathError("close rollback directory", ".", closeErr)
	}
	return errors.Is(readErr, io.EOF), nil
}

func (tx *authProjectionTransaction) quarantineAndRemove(
	node createdAuthNode,
	identity os.FileInfo,
) error {
	parentPath := filepath.Dir(node.relativePath)
	parent, err := tx.openRootedDestinationParent(parentPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer parent.Close()

	var quarantine string
	for range 8 {
		quarantine, err = randomAuthNodeName("rollback", "quarantine")
		if err != nil {
			return err
		}
		err = renameNoReplace(
			int(parent.Fd()), filepath.Base(node.relativePath),
			int(tx.lock.Fd()), quarantine,
		)
		if err == nil {
			break
		}
		if errors.Is(err, unix.ENOENT) {
			return nil
		}
		if !errors.Is(err, unix.EEXIST) {
			return authPathError("quarantine rollback node", ".", err)
		}
	}
	if err != nil {
		return errors.New("could not allocate a unique rollback quarantine")
	}

	moved, err := tx.root.Lstat(quarantine)
	if err != nil {
		return authPathError("inspect quarantined rollback node", ".", err)
	}
	if !os.SameFile(identity, moved) {
		restoreErr := renameNoReplace(
			int(tx.lock.Fd()), quarantine,
			int(parent.Fd()), filepath.Base(node.relativePath),
		)
		if restoreErr != nil {
			return errors.Join(
				errors.New("preserved rollback node whose identity changed in quarantine"),
				authPathError("restore changed rollback node", ".", restoreErr),
			)
		}
		return errors.New("preserved rollback node whose identity changed")
	}
	if err := tx.root.Remove(quarantine); err != nil {
		return authPathError("remove quarantined rollback node", ".", err)
	}
	return nil
}

func (tx *authProjectionTransaction) close() error {
	if tx == nil || tx.closed {
		return nil
	}
	tx.closed = true
	createdErr := tx.closeCreatedIdentities()
	rootErr := tx.root.Close()
	lockErr := closeProjectionLock(tx.lock)
	return errors.Join(
		createdErr,
		sanitizeCloseError("close selected home root", ".", rootErr),
		lockErr,
	)
}

func (tx *authProjectionTransaction) closeCreatedIdentities() error {
	var closeErr error
	for _, node := range slices.Backward(tx.created) {
		if node.identity == nil {
			continue
		}
		closeErr = errors.Join(
			closeErr,
			sanitizeCloseError("close created auth node", node.relativePath, node.identity.Close()),
		)
	}
	return closeErr
}

func closeProjectionLock(lock *os.File) error {
	if lock == nil {
		return nil
	}
	return sanitizeCloseError("close selected home", ".", lock.Close())
}
