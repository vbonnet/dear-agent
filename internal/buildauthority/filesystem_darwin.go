//go:build darwin && arm64

package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	platformModeTypeMask  = uint32(unix.S_IFMT)
	platformModeDirectory = uint32(unix.S_IFDIR)
	platformModeRegular   = uint32(unix.S_IFREG)
	platformModeSymlink   = uint32(unix.S_IFLNK)

	darwinACLDomain              = "darwin-filesec/v1"
	darwinFilesecMagic           = uint32(0x012cc16d)
	darwinFilesecNoACL           = ^uint32(0)
	darwinACLMaxEntries          = uint32(128)
	darwinFilesecHeaderSize      = 44
	darwinACERecordSize          = 24
	darwinAttrResultHeaderSize   = 12
	darwinACLAttributeBufferSize = darwinAttrResultHeaderSize + darwinFilesecHeaderSize + int(darwinACLMaxEntries)*darwinACERecordSize
	darwinFSOptReportFullSize    = uintptr(0x00000004)
	darwinSysFgetattrlist        = uintptr(228)
	darwinSysFreadlink           = uintptr(551)

	darwinACLDeferInherit = uint32(1 << 16)
	darwinACLNoInherit    = uint32(1 << 17)

	darwinACEPermit           = uint32(1)
	darwinACEDeny             = uint32(2)
	darwinACEInherited        = uint32(1 << 4)
	darwinACEFileInherit      = uint32(1 << 5)
	darwinACEDirectoryInherit = uint32(1 << 6)
	darwinACELimitInherit     = uint32(1 << 7)
	darwinACEOnlyInherit      = uint32(1 << 8)

	darwinVNodeSynchronize  = uint32(1 << 20)
	darwinACEGenericAll     = uint32(1 << 21)
	darwinACEGenericExecute = uint32(1 << 22)
	darwinACEGenericWrite   = uint32(1 << 23)
	darwinACEGenericRead    = uint32(1 << 24)
)

const (
	darwinAllowedACLFlags = darwinACLDeferInherit | darwinACLNoInherit
	darwinAllowedACEFlags = darwinACEPermit | darwinACEDeny |
		darwinACEInherited | darwinACEFileInherit | darwinACEDirectoryInherit |
		darwinACELimitInherit | darwinACEOnlyInherit
	darwinAllowedACERights = uint32(0x3ffe) | darwinVNodeSynchronize |
		darwinACEGenericAll | darwinACEGenericExecute | darwinACEGenericWrite |
		darwinACEGenericRead
	darwinMutationRights = uint32(1<<2) | // write data
		uint32(1<<4) | // delete
		uint32(1<<5) | // append data
		uint32(1<<6) | // delete child
		uint32(1<<8) | // write attributes
		uint32(1<<10) | // write extended attributes
		uint32(1<<12) | // write security
		uint32(1<<13) | // take ownership
		darwinACEGenericAll | darwinACEGenericWrite
)

func openAbsoluteNoFollow(path string, kind entryKind) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fs.ErrInvalid
	}
	flags, err := darwinOpenFlags(kind, true)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open(path, flags, 0)
	if err != nil {
		return nil, err
	}
	if err := requireDescriptorKind(fd, kind); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(unix.Close(fd)))
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		return nil, joinFilesystemFailures(fs.ErrInvalid, closeDescriptorFailure(unix.Close(fd)))
	}
	return file, nil
}

func openRelativeNoFollow(rootFD int, path string, kind entryKind) (*os.File, error) {
	components, err := relativePathComponents(path)
	if err != nil {
		return nil, err
	}
	parentFD := rootFD
	ownedParent := false
	for _, component := range components[:len(components)-1] {
		fd, openErr := unix.Openat(
			parentFD,
			component,
			unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW_ANY,
			0,
		)
		if openErr != nil {
			if !ownedParent {
				return nil, openErr
			}
			return nil, joinFilesystemFailures(openErr, closeDescriptorFailure(unix.Close(parentFD)))
		}
		if ownedParent {
			if closeErr := unix.Close(parentFD); closeErr != nil {
				return nil, joinFilesystemFailures(
					closeDescriptorFailure(closeErr),
					closeDescriptorFailure(unix.Close(fd)),
				)
			}
		}
		parentFD = fd
		ownedParent = true
	}

	flags, err := darwinOpenFlags(kind, kind != entrySymlink)
	if err != nil {
		if !ownedParent {
			return nil, err
		}
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(unix.Close(parentFD)))
	}
	leaf := components[len(components)-1]
	fd, openErr := unix.Openat(parentFD, leaf, flags, 0)
	var parentCloseErr error
	if ownedParent {
		parentCloseErr = closeDescriptorFailure(unix.Close(parentFD))
	}
	if openErr != nil {
		return nil, joinFilesystemFailures(openErr, parentCloseErr)
	}
	if parentCloseErr != nil {
		return nil, joinFilesystemFailures(parentCloseErr, closeDescriptorFailure(unix.Close(fd)))
	}
	if err := requireDescriptorKind(fd, kind); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(unix.Close(fd)))
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		return nil, joinFilesystemFailures(fs.ErrInvalid, closeDescriptorFailure(unix.Close(fd)))
	}
	return file, nil
}

func relativeEntryKindNoFollow(parentFD int, name string) (entryKind, error) {
	if name == "" || name == "." || name == ".." || strings.Contains(name, string(filepath.Separator)) {
		return 0, fs.ErrInvalid
	}
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return 0, err
	}
	return entryKindFromPlatformMode(uint32(stat.Mode))
}

func removeRetainedDirectoryAt(parentFD int, name string) error {
	return removeDarwinDirectoryAtWith(parentFD, name, unix.Unlinkat)
}

type darwinUnlinkatFunc func(int, string, int) error

func removeDarwinDirectoryAtWith(parentFD int, name string, unlink darwinUnlinkatFunc) error {
	if parentFD < 0 || unlink == nil {
		return fs.ErrInvalid
	}
	// Do not retry EINTR: a mutating unlinkat may already have completed.
	return unlink(parentFD, name, unix.AT_REMOVEDIR)
}

func retainedDescriptorPath(descriptor *os.File) (string, error) {
	return retainedDescriptorPathWith(descriptor, darwinFcntlGetPath)
}

type darwinFcntlGetPathFunc func(*os.File, []byte) error

func retainedDescriptorPathWith(
	descriptor *os.File,
	getPath darwinFcntlGetPathFunc,
) (string, error) {
	if descriptor == nil || getPath == nil {
		return "", fs.ErrInvalid
	}
	buffer := make([]byte, unix.PathMax)
	if err := getPath(descriptor, buffer); err != nil {
		return "", err
	}
	terminator := bytes.IndexByte(buffer, 0)
	if terminator <= 0 {
		return "", fs.ErrInvalid
	}
	return string(buffer[:terminator]), nil
}

func darwinFcntlGetPath(descriptor *os.File, buffer []byte) error {
	if descriptor == nil || len(buffer) != unix.PathMax {
		return fs.ErrInvalid
	}
	_, err := unix.FcntlInt(
		descriptor.Fd(),
		unix.F_GETPATH,
		int(uintptr(unsafe.Pointer(&buffer[0]))),
	)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(buffer)
	return err
}

func darwinOpenFlags(kind entryKind, noFollowAny bool) (int, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK
	if noFollowAny {
		flags |= unix.O_NOFOLLOW_ANY
	}
	switch kind {
	case entryDirectory:
		flags |= unix.O_DIRECTORY
	case entryRegular:
	case entrySymlink:
		flags |= unix.O_SYMLINK
	default:
		return 0, fs.ErrInvalid
	}
	return flags, nil
}

func relativePathComponents(path string) ([]string, error) {
	if path == "" || path == "." || filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fs.ErrInvalid
	}
	components := strings.Split(path, string(filepath.Separator))
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return nil, fs.ErrInvalid
		}
	}
	return components, nil
}

func requireDescriptorKind(fd int, expected entryKind) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	actual, err := entryKindFromPlatformMode(uint32(stat.Mode))
	if err != nil {
		return err
	}
	if actual != expected {
		return errAuthorityKindMismatch
	}
	return nil
}

func entryKindFromPlatformMode(mode uint32) (entryKind, error) {
	switch mode & platformModeTypeMask {
	case platformModeDirectory:
		return entryDirectory, nil
	case platformModeRegular:
		return entryRegular, nil
	case platformModeSymlink:
		return entrySymlink, nil
	default:
		return 0, errUnsupportedAuthorityEntry
	}
}

type darwinSymlinkReadFunc func(context.Context, *os.File, []byte) (uintptr, error)
type darwinFreadlinkFunc func(*os.File, []byte) (uintptr, error)

func readOpenedSymlinkContext(ctx context.Context, descriptor *os.File, limit uint64) (string, error) {
	return readOpenedSymlinkWith(ctx, descriptor, limit, readDarwinSymlink)
}

func readOpenedSymlinkWith(
	ctx context.Context,
	descriptor *os.File,
	limit uint64,
	readLink darwinSymlinkReadFunc,
) (string, error) {
	if descriptor == nil {
		return "", fail(CauseInternalInvariant, "missing authority symlink descriptor")
	}
	if readLink == nil {
		return "", fail(CauseInternalInvariant, "missing authority symlink reader")
	}
	if err := checkContext(ctx, "read authority symlink"); err != nil {
		return "", err
	}
	maxInt := uint64(^uint(0) >> 1)
	if limit >= maxInt {
		return "", fail(CauseLimit, "authority symlink bound refused")
	}
	buffer := make([]byte, int(limit)+1)
	count, err := readLink(ctx, descriptor, buffer)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return "", failWithFilesystemCauses(contextErr, contextCause(contextErr), "read authority symlink")
		}
		return "", failWith(err, CauseUnstable, "read authority symlink")
	}
	if err := checkContext(ctx, "read authority symlink"); err != nil {
		return "", err
	}
	if count == 0 || count > uintptr(limit) || count > uintptr(len(buffer)) {
		return "", fail(CauseLimit, "authority symlink text refused")
	}
	return string(buffer[:count]), nil
}

func readDarwinSymlink(ctx context.Context, descriptor *os.File, buffer []byte) (uintptr, error) {
	return readDarwinSymlinkWith(ctx, descriptor, buffer, darwinFreadlink)
}

func readDarwinSymlinkWith(
	ctx context.Context,
	descriptor *os.File,
	buffer []byte,
	readLink darwinFreadlinkFunc,
) (uintptr, error) {
	if descriptor == nil || len(buffer) == 0 || readLink == nil {
		return 0, fail(CauseInternalInvariant, "invalid authority symlink syscall request")
	}
	for {
		if err := checkContext(ctx, "read authority symlink"); err != nil {
			return 0, err
		}
		count, err := readLink(descriptor, buffer)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		return count, nil
	}
}

func darwinFreadlink(descriptor *os.File, buffer []byte) (uintptr, error) {
	count, _, errno := unix.Syscall6(
		darwinSysFreadlink,
		descriptor.Fd(),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		0,
		0,
		0,
	)
	runtime.KeepAlive(descriptor)
	runtime.KeepAlive(buffer)
	if errno != 0 {
		return 0, errno
	}
	return count, nil
}

func inspectDescriptorContext(
	ctx context.Context,
	descriptor *os.File,
) (fileSnapshot, mountSnapshot, Digest, error) {
	return inspectDescriptorContextWithACLReader(ctx, descriptor, darwinFgetattrlist)
}

func inspectDescriptorContextWithACLReader(
	ctx context.Context,
	descriptor *os.File,
	readAttributes darwinFgetattrlistFunc,
) (fileSnapshot, mountSnapshot, Digest, error) {
	if descriptor == nil {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, fail(CauseInternalInvariant, "missing authority descriptor")
	}
	if err := checkContext(ctx, "inspect authority descriptor"); err != nil {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, err
	}
	fd := int(descriptor.Fd())
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, failWith(err, CauseUnstable, "stat authority descriptor")
	}
	if stat.Size < 0 {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, fail(CauseMalformed, "negative authority size")
	}

	var filesystem unix.Statfs_t
	if err := unix.Fstatfs(fd, &filesystem); err != nil {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, failWith(err, CauseUnsupported, "inspect authority filesystem")
	}
	mount, err := darwinMountSnapshot(filesystem)
	if err != nil {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, err
	}
	aclDigest, err := descriptorACLDigestWith(ctx, fd, readAttributes)
	if err != nil {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, err
	}
	snapshot := fileSnapshot{
		identity: FileIdentity{
			Device:     uint64(uint32(stat.Dev)), //nolint:gosec // Preserve Darwin dev_t's raw 32-bit identity.
			Inode:      stat.Ino,
			UID:        stat.Uid,
			Mode:       uint32(stat.Mode),
			Filesystem: mount.filesystem,
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
	runtime.KeepAlive(descriptor)
	return snapshot, mount, aclDigest, nil
}

func darwinMountSnapshot(filesystem unix.Statfs_t) (mountSnapshot, error) {
	if filesystem.Flags&unix.MNT_LOCAL == 0 {
		return mountSnapshot{}, fail(CauseUnsupported, "nonlocal authority filesystem refused")
	}
	if filesystem.Flags&unix.MNT_IGNORE_OWNERSHIP != 0 {
		return mountSnapshot{}, fail(CauseUnsupported, "ownership-ignoring authority filesystem refused")
	}
	return mountSnapshot{
		filesystem: filesystem.Fsid.Val,
		flags:      filesystem.Flags & unix.MNT_VISFLAGMASK,
	}, nil
}

type darwinFgetattrlistFunc func(int, *unix.Attrlist, []byte) error

func descriptorACLDigestWith(
	ctx context.Context,
	fd int,
	readAttributes darwinFgetattrlistFunc,
) (Digest, error) {
	if readAttributes == nil {
		return Digest{}, fail(CauseInternalInvariant, "missing authority ACL reader")
	}
	attributes := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, darwinACLAttributeBufferSize)
	for {
		if err := checkContext(ctx, "inspect authority ACL"); err != nil {
			return Digest{}, err
		}
		err := readAttributes(fd, &attributes, buffer)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return Digest{}, failWith(err, CauseUnsupported, "inspect authority ACL")
		}
		if err := checkContext(ctx, "inspect authority ACL"); err != nil {
			return Digest{}, err
		}
		return digestExtendedSecurityResult(buffer)
	}
}

func darwinFgetattrlist(fd int, attributes *unix.Attrlist, buffer []byte) error {
	_, _, errno := unix.Syscall6(
		darwinSysFgetattrlist,
		uintptr(fd),
		uintptr(unsafe.Pointer(attributes)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		darwinFSOptReportFullSize,
		0,
	)
	runtime.KeepAlive(attributes)
	runtime.KeepAlive(buffer)
	if errno != 0 {
		return errno
	}
	return nil
}

func digestExtendedSecurityResult(buffer []byte) (Digest, error) {
	if len(buffer) < darwinAttrResultHeaderSize {
		return Digest{}, fail(CauseMalformed, "short authority ACL result")
	}
	reported := uint64(binary.LittleEndian.Uint32(buffer[:4]))
	if reported < darwinAttrResultHeaderSize || reported > uint64(len(buffer)) {
		return Digest{}, fail(CauseMalformed, "authority ACL result length refused")
	}
	rawReferenceOffset := binary.LittleEndian.Uint32(buffer[4:8])
	referenceOffset := int64(rawReferenceOffset)
	if rawReferenceOffset&(1<<31) != 0 {
		referenceOffset -= 1 << 32
	}
	attributeLength := uint64(binary.LittleEndian.Uint32(buffer[8:12]))
	attributeStart := int64(4) + referenceOffset
	if attributeStart != darwinAttrResultHeaderSize {
		return Digest{}, fail(CauseMalformed, "noncanonical authority ACL reference")
	}
	attributeEnd := uint64(attributeStart) + attributeLength
	if attributeEnd < uint64(attributeStart) || attributeEnd > reported {
		return Digest{}, fail(CauseMalformed, "authority ACL extent refused")
	}
	if attributeEnd != reported {
		return Digest{}, fail(CauseMalformed, "noncanonical authority ACL extent")
	}
	if attributeLength == 0 {
		return digestDarwinFilesec(nil)
	}
	return digestDarwinFilesec(buffer[attributeStart:attributeEnd])
}

func digestDarwinFilesec(attribute []byte) (Digest, error) {
	canonical := bytes.NewBuffer(make([]byte, 0, 8+len(darwinACLDomain)+1+len(attribute)))
	writeLengthPrefixed(canonical, []byte(darwinACLDomain))
	if len(attribute) == 0 {
		canonical.WriteByte(0)
		return sha256.Sum256(canonical.Bytes()), nil
	}
	if len(attribute) < darwinFilesecHeaderSize {
		return Digest{}, fail(CauseMalformed, "short authority filesec record")
	}

	magic := binary.LittleEndian.Uint32(attribute[0:4])
	entryCount := binary.LittleEndian.Uint32(attribute[36:40])
	aclFlags := binary.LittleEndian.Uint32(attribute[40:44])
	if magic != darwinFilesecMagic {
		return Digest{}, fail(CauseMalformed, "authority filesec magic refused")
	}
	if aclFlags&^darwinAllowedACLFlags != 0 {
		return Digest{}, fail(CauseMalformed, "authority ACL flags refused")
	}
	expectedSize := darwinFilesecHeaderSize
	if entryCount != darwinFilesecNoACL {
		if entryCount > darwinACLMaxEntries {
			return Digest{}, fail(CauseLimit, "authority ACL entry count refused")
		}
		expectedSize += int(entryCount) * darwinACERecordSize
	}
	if len(attribute) != expectedSize {
		return Digest{}, fail(CauseMalformed, "authority filesec length refused")
	}

	canonical.WriteByte(1)
	writeUint32(canonical, magic)
	canonical.Write(attribute[4:36])
	writeUint32(canonical, entryCount)
	writeUint32(canonical, aclFlags)
	if entryCount == darwinFilesecNoACL {
		return sha256.Sum256(canonical.Bytes()), nil
	}
	for index := range int(entryCount) {
		offset := darwinFilesecHeaderSize + index*darwinACERecordSize
		qualifier := attribute[offset : offset+16]
		flags := binary.LittleEndian.Uint32(attribute[offset+16 : offset+20])
		rights := binary.LittleEndian.Uint32(attribute[offset+20 : offset+24])
		if err := validateDarwinACE(flags, rights); err != nil {
			return Digest{}, err
		}
		canonical.Write(qualifier)
		writeUint32(canonical, flags)
		writeUint32(canonical, rights)
	}
	return sha256.Sum256(canonical.Bytes()), nil
}

func validateDarwinACE(flags, rights uint32) error {
	kind := flags & 0xf
	if (kind != darwinACEPermit && kind != darwinACEDeny) || flags&^darwinAllowedACEFlags != 0 {
		return fail(CauseMalformed, "authority ACE flags refused")
	}
	if rights&^darwinAllowedACERights != 0 {
		return fail(CauseMalformed, "authority ACE rights refused")
	}
	if kind == darwinACEPermit && rights&darwinMutationRights != 0 {
		return fail(CausePermission, "mutation-permitting authority ACE refused")
	}
	return nil
}
