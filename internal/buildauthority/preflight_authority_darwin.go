//go:build darwin && arm64

package buildauthority

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

func platformPreflightAuthorityPrimitives() (
	preflightAuthorityPrimitives,
	*FailureRecord,
) {
	return preflightAuthorityPrimitives{
		openRelativeNoFollow: openPreflightAuthorityRelativeNoFollow,
		statDescriptor:       statPreflightAuthorityDescriptor,
		statFilesystem:       statPreflightAuthorityFilesystem,
		validateFilesystem:   validatePreflightAuthorityFilesystem,
		acquireRawACL:        acquirePreflightAuthorityRawACL,
		parseRawACL:          parsePreflightAuthorityRawACL,
		readExactForParse:    readExactAuthorityForParse,
		hashBytes:            hashAuthorityBytes,
	}, nil
}

// openPreflightAuthorityRelativeNoFollow performs acquisition only. In
// particular, it does not call requireDescriptorKind and never closes a file
// descriptor returned by openat; the common owner is installed first.
func openPreflightAuthorityRelativeNoFollow(
	ctx context.Context,
	parent *os.File,
	name string,
	mode authorityRequiredOpenMode,
) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
	if parent == nil || name == "" || name == "." || name == ".." ||
		strings.Contains(name, string(filepath.Separator)) {
		return nil, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if !mode.valid() {
		return nil, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK | unix.O_NOFOLLOW_ANY
	for {
		if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
			return nil, failure
		}
		fd, openErr := unix.Openat(int(parent.Fd()), name, flags, 0)
		runtime.KeepAlive(parent)
		if errors.Is(openErr, unix.EINTR) {
			continue
		}
		if openErr != nil {
			return nil, classifyPreflightAuthorityOpenFailure(ctx, mode, openErr)
		}
		file := os.NewFile(uintptr(fd), name)
		if file == nil {
			return &authorityDescriptorAcquisition{
				close: func() error { return unix.Close(fd) },
			}, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		acquisition := &authorityDescriptorAcquisition{
			file:  file,
			close: file.Close,
		}
		if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
			return acquisition, failure
		}
		return acquisition, nil
	}
}

func classifyPreflightAuthorityOpenFailure(
	ctx context.Context,
	mode authorityRequiredOpenMode,
	err error,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return failure
	}
	if !mode.valid() || err == nil {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return requiredAuthorityOpenNotFound(mode)
	case errors.Is(err, fs.ErrPermission):
		return newAuthorityPrimitiveFailure(OperationProbe, CausePermission)
	default:
		return newAuthorityPrimitiveFailure(OperationProbe, CauseUnstable)
	}
}

func statPreflightAuthorityDescriptor(
	ctx context.Context,
	descriptor *os.File,
) (fileSnapshot, *authorityPrimitiveFailure) {
	if descriptor == nil {
		return fileSnapshot{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return fileSnapshot{}, failure
	}
	fd := int(descriptor.Fd())
	var stat unix.Stat_t
	err := unix.Fstat(fd, &stat)
	runtime.KeepAlive(descriptor)
	if err != nil {
		if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return fileSnapshot{}, failure
		}
		return fileSnapshot{}, newAuthorityPrimitiveFailure(
			OperationProbe,
			authorityPrimitiveIOCause(err),
		)
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
	if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return fileSnapshot{}, failure
	}
	return snapshot, nil
}

func statPreflightAuthorityFilesystem(
	ctx context.Context,
	descriptor *os.File,
) (mountSnapshot, *authorityPrimitiveFailure) {
	if descriptor == nil {
		return mountSnapshot{}, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return mountSnapshot{}, failure
	}
	var filesystem unix.Statfs_t
	err := unix.Fstatfs(int(descriptor.Fd()), &filesystem)
	runtime.KeepAlive(descriptor)
	if err != nil {
		if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return mountSnapshot{}, failure
		}
		return mountSnapshot{}, newAuthorityPrimitiveFailure(
			OperationProbe,
			authorityPrimitiveIOCause(err),
		)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return mountSnapshot{}, failure
	}
	return normalizePreflightAuthorityMount(filesystem), nil
}

func normalizePreflightAuthorityMount(filesystem unix.Statfs_t) mountSnapshot {
	return mountSnapshot{
		filesystem: filesystem.Fsid.Val,
		flags:      filesystem.Flags & unix.MNT_VISFLAGMASK,
	}
}

func validatePreflightAuthorityFilesystem(
	ctx context.Context,
	mount mountSnapshot,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationValidate); failure != nil {
		return failure
	}
	if mount.flags&unix.MNT_LOCAL == 0 || mount.flags&unix.MNT_IGNORE_OWNERSHIP != 0 {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseUnsupported)
	}
	return nil
}

func acquirePreflightAuthorityRawACL(
	ctx context.Context,
	descriptor *os.File,
) ([]byte, *authorityPrimitiveFailure) {
	if descriptor == nil {
		return nil, newAuthorityPrimitiveFailure(
			OperationValidate,
			CauseInternalInvariant,
		)
	}
	attributes := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, darwinACLAttributeBufferSize)
	for {
		if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return nil, failure
		}
		err := darwinFgetattrlist(int(descriptor.Fd()), &attributes, buffer)
		runtime.KeepAlive(descriptor)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
				return nil, failure
			}
			return nil, newAuthorityPrimitiveFailure(
				OperationProbe,
				authorityPrimitiveIOCause(err),
			)
		}
		if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return nil, failure
		}
		return buffer, nil
	}
}

func parsePreflightAuthorityRawACL(
	ctx context.Context,
	raw []byte,
) (Digest, *authorityPrimitiveFailure) {
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return Digest{}, failure
	}
	digest, err := digestExtendedSecurityResult(raw)
	if err != nil {
		return Digest{}, authorityPrimitiveFailureFromError(
			OperationParse,
			err,
			CauseMalformed,
		)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return Digest{}, failure
	}
	return digest, nil
}

func authorityPrimitiveIOCause(
	err error,
) CauseCode {
	if errors.Is(err, fs.ErrPermission) {
		return CausePermission
	}
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
		return CauseUnsupported
	}
	return CauseUnstable
}
