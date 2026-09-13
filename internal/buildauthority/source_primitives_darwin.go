//go:build darwin && arm64

package buildauthority

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

// darwinSourcePrimitives is deliberately stateless. Long-lived source owners
// contain only their concrete os handles and lifecycle state; this adapter is
// supplied to one source transaction and is never retained by an owner.
type darwinSourcePrimitives struct{}

var _ sourcePrimitives = darwinSourcePrimitives{}

// invalidateDarwinSourceFD retires the raw integer slot immediately after an
// os.File assumes responsibility for closing the descriptor.
func invalidateDarwinSourceFD(fd *int) bool {
	if fd == nil || *fd < 0 {
		return false
	}
	*fd = -1
	return true
}

func platformSourcePrimitives() (sourcePrimitives, *sourcePrimitiveFailure) {
	return darwinSourcePrimitives{}, nil
}

func (darwinSourcePrimitives) privateSourcePrimitives() {}

func (darwinSourcePrimitives) openRepositoryRoot(
	ctx context.Context,
	locator sourceRepositoryLocator,
) (*ownedSourceRoot, *sourcePrimitiveFailure) {
	if !locator.valid() {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return nil, failure
	}
	owner := &ownedSourceRoot{state: sourceHandleOpen}
	var err error
	owner.root, err = os.OpenRoot(locator.path)
	if err != nil {
		if owner.root != nil {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		return nil, classifyDarwinSourceOpenFailure(ctx, sourceInitialRequired, err)
	}
	if owner.root == nil {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return owner, failure
	}
	return owner, nil
}

func (darwinSourcePrimitives) openPhysicalRootDescriptor(
	ctx context.Context,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return nil, failure
	}
	flags, flagsErr := darwinOpenFlags(entryDirectory, true)
	if flagsErr != nil {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	for {
		fd, err := unix.Open(physicalRootPath, flags, 0)
		if err == unix.EINTR {
			if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
				return nil, failure
			}
			continue
		}
		if err != nil {
			return nil, classifyDarwinSourceOpenFailure(ctx, sourceInitialRequired, err)
		}
		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), physicalRootPath),
			kind:  sourceObservedDirectory,
			state: sourceHandleOpen,
		}
		if owner.file == nil {
			closeErr := unix.Close(fd)
			if closeErr != nil {
				return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		if !invalidateDarwinSourceFD(&fd) {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
			return owner, failure
		}
		return owner, nil
	}
}

func (darwinSourcePrimitives) probeRelativeKind(
	ctx context.Context,
	parent *ownedSourceDescriptor,
	name string,
	mode sourcePresenceMode,
) (sourceObservedKind, bool, *sourcePrimitiveFailure) {
	if !parent.validOpen() || parent.kind != sourceObservedDirectory ||
		!validDarwinSourceName(name) || !mode.valid() {
		return 0, false, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	for {
		if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return 0, false, failure
		}
		var stat unix.Stat_t
		err := unix.Fstatat(int(parent.file.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
		runtime.KeepAlive(parent.file)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return classifyDarwinSourcePresenceFailure(ctx, mode, err)
		}
		if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return 0, false, failure
		}
		if mode == sourceRevalidateAbsent {
			return 0, true, newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
		entry, kindErr := entryKindFromPlatformMode(uint32(stat.Mode))
		kind := observedSourceKind(entry)
		if kindErr != nil {
			kind = sourceObservedSpecial
		}
		if mode == sourceInitialForbidden {
			return kind, true, newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)
		}
		return kind, true, nil
	}
}

func (darwinSourcePrimitives) openRelativeNoFollow(
	ctx context.Context,
	parent *ownedSourceDescriptor,
	name string,
	kind sourceObservedKind,
	mode sourcePresenceMode,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	openKind, kindOK := kind.openKind()
	if !parent.validOpen() || parent.kind != sourceObservedDirectory ||
		!validDarwinSourceName(name) || !mode.valid() || !kindOK {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if mode == sourceInitialForbidden || mode == sourceRevalidateAbsent {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	flags, err := darwinOpenFlags(openKind, openKind != entrySymlink)
	if err != nil {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	return openDarwinSourceRelativeDescriptor(ctx, parent, name, kind, openKind, mode, flags)
}

//nolint:gocyclo // Keep the linear raw-FD ownership and validation state machine visibly contiguous.
func openDarwinSourceRelativeDescriptor(
	ctx context.Context,
	parent *ownedSourceDescriptor,
	name string,
	kind sourceObservedKind,
	openKind entryKind,
	mode sourcePresenceMode,
	flags int,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	if !validDarwinSourceName(name) {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	for {
		if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
			return nil, failure
		}
		fd, openErr := unix.Openat(int(parent.file.Fd()), name, flags, 0)
		runtime.KeepAlive(parent.file)
		if openErr == unix.EINTR {
			continue
		}
		if openErr != nil {
			return nil, classifyDarwinSourceOpenFailure(ctx, mode, openErr)
		}
		owner := &ownedSourceDescriptor{
			file:  os.NewFile(uintptr(fd), name),
			kind:  kind,
			state: sourceHandleOpen,
		}
		if owner.file == nil {
			closeErr := unix.Close(fd)
			if closeErr != nil {
				return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
			}
			return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		if !invalidateDarwinSourceFD(&fd) {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
			return owner, failure
		}
		if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return owner, failure
		}
		kindErr := requireDescriptorKind(int(owner.file.Fd()), openKind)
		runtime.KeepAlive(owner.file)
		if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return owner, failure
		}
		if kindErr != nil {
			//nolint:errorlint // requireDescriptorKind returns these exact private sentinels.
			if kindErr == errAuthorityKindMismatch ||
				kindErr == errUnsupportedAuthorityEntry {
				operation := OperationOpen
				if mode == sourceRevalidatePresent {
					operation = OperationCompare
				}
				return owner, newSourcePrimitiveFailure(operation, CauseIdentity)
			}
			return owner, newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(kindErr))
		}
		return owner, nil
	}
}

func (darwinSourcePrimitives) openChildRoot(
	ctx context.Context,
	parent *ownedSourceRoot,
	name string,
) (*ownedSourceRoot, *sourcePrimitiveFailure) {
	if !parent.validOpen() || !validDarwinSourceName(name) {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return nil, failure
	}
	owner := &ownedSourceRoot{state: sourceHandleOpen}
	var err error
	owner.root, err = parent.root.OpenRoot(name)
	runtime.KeepAlive(parent.root)
	if err != nil {
		if owner.root != nil {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		return nil, classifyDarwinSourceOpenFailure(ctx, sourceInitialRequired, err)
	}
	if owner.root == nil {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return owner, failure
	}
	return owner, nil
}

func (darwinSourcePrimitives) openRootDirectoryDescriptor(
	ctx context.Context,
	root *ownedSourceRoot,
) (*ownedSourceDescriptor, *sourcePrimitiveFailure) {
	if !root.validOpen() {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return nil, failure
	}
	owner := &ownedSourceDescriptor{
		kind:  sourceObservedDirectory,
		state: sourceHandleOpen,
	}
	var err error
	owner.file, err = root.root.Open(".")
	runtime.KeepAlive(root.root)
	if err != nil {
		if owner.file != nil {
			return owner, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		return nil, classifyDarwinSourceOpenFailure(ctx, sourceInitialRequired, err)
	}
	if owner.file == nil {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return owner, failure
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return owner, failure
	}
	kindErr := requireDescriptorKind(int(owner.file.Fd()), entryDirectory)
	runtime.KeepAlive(owner.file)
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return owner, failure
	}
	if kindErr != nil {
		//nolint:errorlint // requireDescriptorKind returns these exact private sentinels.
		if kindErr == errAuthorityKindMismatch ||
			kindErr == errUnsupportedAuthorityEntry {
			return owner, newSourcePrimitiveFailure(OperationOpen, CauseIdentity)
		}
		return owner, newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(kindErr))
	}
	return owner, nil
}

func (darwinSourcePrimitives) readDirectoryBatch(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) ([]string, bool, *sourcePrimitiveFailure) {
	if !owner.validOpen() || owner.kind != sourceObservedDirectory {
		return nil, false, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return nil, false, failure
	}
	entries, err := owner.file.ReadDir(sourceDirectoryReadBatchSize)
	runtime.KeepAlive(owner.file)
	return normalizeDarwinSourceDirectoryBatch(ctx, entries, err)
}

func normalizeDarwinSourceDirectoryBatch(
	ctx context.Context,
	entries []os.DirEntry,
	err error,
) ([]string, bool, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return nil, false, failure
	}
	done := err == io.EOF //nolint:errorlint // os.File.ReadDir returns io.EOF directly for this terminal batch contract.
	if err != nil && !done {
		return nil, false, classifyDarwinSourceWalkFailure(ctx, err)
	}
	if len(entries) > sourceDirectoryReadBatchSize {
		return nil, false, newSourcePrimitiveFailure(OperationWalk, CauseLimit)
	}
	if len(entries) == 0 && !done {
		return nil, false, newSourcePrimitiveFailure(OperationWalk, CauseUnstable)
	}

	names := make([]string, len(entries))
	for index, entry := range entries {
		if entry == nil {
			return nil, false, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		name := entry.Name()
		if !validDarwinSourceName(name) {
			return nil, false, newSourcePrimitiveFailure(OperationWalk, CauseUnstable)
		}
		if len(name) > maxPathComponentBytes {
			return nil, false, newSourcePrimitiveFailure(OperationWalk, CauseLimit)
		}
		names[index] = name
	}
	return names, done, nil
}

func (darwinSourcePrimitives) statDescriptor(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) (fileSnapshot, *sourcePrimitiveFailure) {
	if !owner.validOpen() {
		return fileSnapshot{}, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return fileSnapshot{}, failure
	}
	var stat unix.Stat_t
	err := unix.Fstat(int(owner.file.Fd()), &stat)
	runtime.KeepAlive(owner.file)
	if err != nil {
		if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return fileSnapshot{}, failure
		}
		return fileSnapshot{}, newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(err))
	}
	snapshot := fileSnapshot{
		identity: FileIdentity{
			Device: uint64(uint32(stat.Dev)), //nolint:gosec // Preserve Darwin dev_t's raw 32-bit identity.
			Inode:  stat.Ino,
			UID:    stat.Uid,
			Mode:   uint32(stat.Mode),
		},
		linkCount:  uint64(stat.Nlink),
		gid:        stat.Gid,
		rdev:       uint64(uint32(stat.Rdev)), //nolint:gosec // Preserve Darwin dev_t's raw 32-bit identity.
		size:       stat.Size,
		mtimeSec:   stat.Mtim.Sec,
		mtimeNsec:  stat.Mtim.Nsec,
		ctimeSec:   stat.Ctim.Sec,
		ctimeNsec:  stat.Ctim.Nsec,
		birthSec:   stat.Btim.Sec,
		birthNsec:  stat.Btim.Nsec,
		flags:      stat.Flags,
		generation: stat.Gen,
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return fileSnapshot{}, failure
	}
	return snapshot, nil
}

func (darwinSourcePrimitives) statFilesystem(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) (mountSnapshot, *sourcePrimitiveFailure) {
	if !owner.validOpen() {
		return mountSnapshot{}, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return mountSnapshot{}, failure
	}
	var filesystem unix.Statfs_t
	err := unix.Fstatfs(int(owner.file.Fd()), &filesystem)
	runtime.KeepAlive(owner.file)
	if err != nil {
		if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return mountSnapshot{}, failure
		}
		return mountSnapshot{}, newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(err))
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return mountSnapshot{}, failure
	}
	return mountSnapshot{
		filesystem: filesystem.Fsid.Val,
		flags:      filesystem.Flags & unix.MNT_VISFLAGMASK,
	}, nil
}

func (darwinSourcePrimitives) validateFilesystem(
	ctx context.Context,
	mount mountSnapshot,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	if mount.flags&unix.MNT_LOCAL == 0 || mount.flags&unix.MNT_IGNORE_OWNERSHIP != 0 {
		return newSourcePrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	return nil
}

func (darwinSourcePrimitives) acquireRawACL(
	ctx context.Context,
	owner *ownedSourceDescriptor,
) ([]byte, *sourcePrimitiveFailure) {
	if !owner.validOpen() {
		return nil, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	attributes := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, darwinACLAttributeBufferSize)
	for {
		if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return nil, failure
		}
		err := darwinFgetattrlist(int(owner.file.Fd()), &attributes, buffer)
		runtime.KeepAlive(owner.file)
		if err == unix.EINTR { //nolint:errorlint // darwinFgetattrlist returns the raw syscall sentinel without wrapping.
			continue
		}
		if err != nil {
			if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
				return nil, failure
			}
			return nil, newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(err))
		}
		if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return nil, failure
		}
		return buffer, nil
	}
}

func (darwinSourcePrimitives) parseRawACL(
	ctx context.Context,
	raw []byte,
) (parsedSourceACL, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return parsedSourceACL{}, failure
	}
	observation, err := observeExtendedSecurityResult(raw)
	if err != nil {
		return parsedSourceACL{}, sourcePrimitiveFailureFromError(OperationParse, err, CauseMalformed)
	}
	disposition := sourceACLAdmitted
	if observation.policyErr != nil {
		causes := privateCauses(observation.policyErr, CauseInternalInvariant)
		if len(causes) != 1 || causes[0] != CausePermission {
			return parsedSourceACL{}, newSourcePrimitiveFailure(
				OperationValidate,
				CauseInternalInvariant,
			)
		}
		disposition = sourceACLMutationPermitting
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return parsedSourceACL{}, failure
	}
	return parsedSourceACL{
		digest:      observation.digest,
		disposition: disposition,
	}, nil
}

func (darwinSourcePrimitives) validateACL(
	ctx context.Context,
	acl parsedSourceACL,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	if !acl.valid() {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if acl.disposition == sourceACLMutationPermitting {
		return newSourcePrimitiveFailure(OperationValidate, CausePermission)
	}
	return nil
}

func (darwinSourcePrimitives) readExactForParse(
	ctx context.Context,
	owner *ownedSourceDescriptor,
	content []byte,
) *sourcePrimitiveFailure {
	if !owner.validOpen() || content == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return failure
	}
	read, err := owner.file.ReadAt(content, 0)
	runtime.KeepAlive(owner.file)
	if failure := sourceContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return failure
	}
	if read == len(content) && (err == nil || err == io.EOF) {
		return nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return newSourcePrimitiveFailure(OperationParse, CausePermission)
	}
	return newSourcePrimitiveFailure(OperationParse, CauseUnstable)
}

func (darwinSourcePrimitives) hashBytes(
	ctx context.Context,
	content []byte,
) (Digest, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return Digest{}, failure
	}
	digest := Digest(sha256.Sum256(content))
	if failure := sourceContextPrimitiveFailure(ctx, OperationHash); failure != nil {
		return Digest{}, failure
	}
	return digest, nil
}

func (darwinSourcePrimitives) compareRootAndDescriptor(
	ctx context.Context,
	root *ownedSourceRoot,
	descriptor *ownedSourceDescriptor,
) *sourcePrimitiveFailure {
	if !root.validOpen() || !descriptor.validOpen() ||
		descriptor.kind != sourceObservedDirectory {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return failure
	}
	rootInfo, rootErr := root.root.Lstat(".")
	runtime.KeepAlive(root.root)
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return failure
	}
	if rootErr != nil {
		return newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(rootErr))
	}
	descriptorInfo, descriptorErr := descriptor.file.Stat()
	runtime.KeepAlive(descriptor.file)
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return failure
	}
	if descriptorErr != nil {
		return newSourcePrimitiveFailure(OperationProbe, darwinSourceIOCause(descriptorErr))
	}
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	same := os.SameFile(rootInfo, descriptorInfo)
	if failure := sourceContextPrimitiveFailure(ctx, OperationCompare); failure != nil {
		return failure
	}
	if !same {
		return newSourcePrimitiveFailure(OperationCompare, CauseIdentity)
	}
	return nil
}

func (darwinSourcePrimitives) closeRoot(owner *ownedSourceRoot) bool {
	if owner == nil {
		return true
	}
	return owner.closeDirect()
}

func (darwinSourcePrimitives) closeDescriptor(owner *ownedSourceDescriptor) bool {
	if owner == nil {
		return true
	}
	return owner.closeDirect()
}

func validDarwinSourceName(name string) bool {
	return name != "" && name != "." && name != ".." && strings.IndexByte(name, 0) < 0 &&
		!strings.Contains(name, string(filepath.Separator))
}

func classifyDarwinSourcePresenceFailure(
	ctx context.Context,
	mode sourcePresenceMode,
	err error,
) (sourceObservedKind, bool, *sourcePrimitiveFailure) {
	if failure := sourceContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return 0, false, failure
	}
	if !mode.valid() || err == nil {
		return 0, false, newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if errors.Is(err, fs.ErrNotExist) {
		switch mode {
		case sourceInitialRequired:
			return 0, false, newSourcePrimitiveFailure(OperationOpen, CauseNotFound)
		case sourceInitialOptional, sourceInitialForbidden, sourceRevalidateAbsent:
			return 0, false, nil
		case sourceInitialWalkPresent:
			return 0, false, newSourcePrimitiveFailure(OperationProbe, CauseUnstable)
		case sourceRevalidatePresent:
			return 0, false, newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
	}
	if errors.Is(err, fs.ErrPermission) {
		return 0, false, newSourcePrimitiveFailure(OperationProbe, CausePermission)
	}
	return 0, false, newSourcePrimitiveFailure(OperationProbe, CauseUnstable)
}

func classifyDarwinSourceOpenFailure(
	ctx context.Context,
	mode sourcePresenceMode,
	err error,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return failure
	}
	if !mode.valid() || err == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if errors.Is(err, fs.ErrNotExist) {
		if mode == sourceInitialRequired {
			return newSourcePrimitiveFailure(OperationOpen, CauseNotFound)
		}
		if mode == sourceInitialWalkPresent {
			return newSourcePrimitiveFailure(OperationOpen, CauseUnstable)
		}
		if mode == sourceInitialOptional {
			return newSourcePrimitiveFailure(OperationProbe, CauseUnstable)
		}
		if mode == sourceRevalidatePresent {
			return newSourcePrimitiveFailure(OperationCompare, CauseUnstable)
		}
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
		if mode == sourceRevalidatePresent {
			return newSourcePrimitiveFailure(OperationCompare, CauseIdentity)
		}
		return newSourcePrimitiveFailure(OperationOpen, CauseIdentity)
	}
	return newSourcePrimitiveFailure(OperationOpen, darwinSourceIOCause(err))
}

func classifyDarwinSourceWalkFailure(
	ctx context.Context,
	err error,
) *sourcePrimitiveFailure {
	if failure := sourceContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return failure
	}
	if err == nil {
		return newSourcePrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if errors.Is(err, context.Canceled) {
		return newSourcePrimitiveFailure(OperationWalk, CauseCanceled)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return newSourcePrimitiveFailure(OperationWalk, CauseDeadline)
	}
	if errors.Is(err, fs.ErrPermission) {
		return newSourcePrimitiveFailure(OperationWalk, CausePermission)
	}
	return newSourcePrimitiveFailure(OperationWalk, CauseUnstable)
}

func darwinSourceIOCause(err error) CauseCode {
	if errors.Is(err, fs.ErrPermission) {
		return CausePermission
	}
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
		return CauseUnsupported
	}
	return CauseUnstable
}
