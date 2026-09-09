//go:build darwin && arm64

package buildauthority

import (
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

const (
	platformModeCharacter = uint32(unix.S_IFCHR)
	darwinNullDeviceMajor = uint64(3)
	darwinNullDeviceMinor = uint64(2)

	darwinNullACLResultSize = 32
)

func openFixedNullDeviceNoFollow(parentFD int) (*os.File, error) {
	if parentFD < 0 {
		return nil, fs.ErrInvalid
	}
	fd, err := unix.Openat(
		parentFD,
		"null",
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK|unix.O_NOFOLLOW_ANY,
		0,
	)
	if err != nil {
		return nil, err
	}
	if err := requireDescriptorPlatformMode(fd, platformModeCharacter); err != nil {
		return nil, joinFilesystemFailures(err, closeDescriptorFailure(unix.Close(fd)))
	}
	file := os.NewFile(uintptr(fd), nullDevicePath)
	if file == nil {
		return nil, joinFilesystemFailures(fs.ErrInvalid, closeDescriptorFailure(unix.Close(fd)))
	}
	return file, nil
}

func requireDescriptorPlatformMode(fd int, expected uint32) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if uint32(stat.Mode)&platformModeTypeMask != expected {
		return errAuthorityKindMismatch
	}
	return nil
}

func validatePlatformNullDeviceSnapshot(snapshot fileSnapshot) error {
	if snapshot.identity.Mode&platformModeTypeMask != platformModeCharacter {
		return fail(CauseUnsupported, "retained null authority is not a character device")
	}
	if snapshot.identity.UID != 0 {
		return fail(CausePermission, "retained null authority owner refused")
	}
	if snapshot.identity.Mode != platformModeCharacter|0o666 {
		return fail(CausePermission, "retained null authority mode refused")
	}
	if snapshot.linkCount != 1 {
		return fail(CauseIdentity, "retained null authority link count refused")
	}
	major, minor := darwinDeviceNumbers(snapshot.rdev)
	if major != darwinNullDeviceMajor || minor != darwinNullDeviceMinor {
		return fail(CauseIdentity, "retained null authority device number refused")
	}
	return nil
}

func darwinDeviceNumbers(raw uint64) (uint64, uint64) {
	return uint64(unix.Major(raw)), uint64(unix.Minor(raw))
}

func inspectNullAuthorityDescriptorContext(
	ctx context.Context,
	descriptor *os.File,
) (fileSnapshot, mountSnapshot, Digest, error) {
	return inspectDescriptorContextWithACLDigest(ctx, descriptor, nullAuthorityACLDigestContext)
}

func nullAuthorityACLDigestContext(ctx context.Context, fd int) (Digest, error) {
	digest, err := descriptorACLDigestWith(ctx, fd, darwinFgetattrlist)
	if err == nil || !errors.Is(err, unix.EINVAL) {
		return digest, err
	}
	return nullDeviceACLDigestContext(ctx, fd)
}

// nullDeviceACLDigestContext asks Darwin to report which requested attributes
// are valid and to pack a canonical default for an invalid attribute. devfs
// rejects a bare ATTR_CMN_EXTENDED_SECURITY request with EINVAL; the returned
// attribute set is the descriptor-bound proof that extended security is absent
// on this object rather than an unchecked interpretation of that errno.
func nullDeviceACLDigestContext(ctx context.Context, fd int) (Digest, error) {
	attributes := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_RETURNED_ATTRS | unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	buffer := make([]byte, darwinNullACLResultSize)
	for {
		if err := checkContext(ctx, "inspect null-device ACL"); err != nil {
			return Digest{}, err
		}
		err := darwinFgetattrlistWithOptions(
			fd,
			&attributes,
			buffer,
			darwinFSOptReportFullSize|unix.FSOPT_PACK_INVAL_ATTRS,
		)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return Digest{}, failWith(err, CauseUnsupported, "inspect null-device ACL")
		}
		if err := checkContext(ctx, "inspect null-device ACL"); err != nil {
			return Digest{}, err
		}
		return digestNullDeviceACLResult(buffer)
	}
}

func digestNullDeviceACLResult(buffer []byte) (Digest, error) {
	if len(buffer) != darwinNullACLResultSize ||
		binary.LittleEndian.Uint32(buffer[0:4]) != darwinNullACLResultSize {
		return Digest{}, fail(CauseMalformed, "null-device ACL result length refused")
	}
	if binary.LittleEndian.Uint32(buffer[4:8]) != unix.ATTR_CMN_RETURNED_ATTRS {
		return Digest{}, fail(CauseUnsupported, "null-device extended security attribute present")
	}
	for offset := 8; offset < 24; offset += 4 {
		if binary.LittleEndian.Uint32(buffer[offset:offset+4]) != 0 {
			return Digest{}, fail(CauseMalformed, "null-device ACL returned-attribute set refused")
		}
	}
	if binary.LittleEndian.Uint32(buffer[24:28]) != 8 ||
		binary.LittleEndian.Uint32(buffer[28:32]) != 0 {
		return Digest{}, fail(CauseMalformed, "null-device ACL default reference refused")
	}
	return digestDarwinFilesec(nil)
}
