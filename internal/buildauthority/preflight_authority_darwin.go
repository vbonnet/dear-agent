//go:build darwin && arm64

package buildauthority

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
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
		brackets: preflightAuthorityBracketPrimitives{
			probeSentinelAbsence:         probePreflightAuthoritySentinelAbsence,
			openAbsoluteRoot:             openPreflightAuthorityAbsoluteRoot,
			openRootHandle:               openPreflightAuthorityRootHandle,
			openNullRelativeNoFollow:     openPreflightAuthorityNullRelativeNoFollow,
			probeRelativeKind:            probePreflightAuthorityRelativeKind,
			openTypedRelativeNoFollow:    openPreflightAuthorityTypedRelativeNoFollow,
			readDirectory:                readPreflightAuthorityDirectory,
			readSymlinkForWalk:           readPreflightAuthoritySymlinkForWalk,
			hashDescriptor:               hashPreflightAuthorityDescriptor,
			parseMachO:                   parsePreflightAuthorityMachO,
			hashManifest:                 hashPreflightAuthorityManifest,
			acquireRawNullACL:            acquirePreflightAuthorityRawNullACL,
			parseRawACLForComparison:     parsePreflightAuthorityRawACLForComparison,
			parseRawNullACLForComparison: parsePreflightAuthorityRawNullACLForComparison,
		},
	}, nil
}

func probePreflightAuthoritySentinelAbsence(
	ctx context.Context,
	parent *os.File,
	name string,
	mode authoritySentinelAbsenceMode,
) *authorityPrimitiveFailure {
	if parent == nil || name == "" || name == "." || name == ".." ||
		strings.Contains(name, string(filepath.Separator)) || !mode.valid() {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	for {
		if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return failure
		}
		var stat unix.Stat_t
		err := unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
		runtime.KeepAlive(parent)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return classifyPreflightAuthoritySentinelAbsence(ctx, mode, err)
	}
}

func classifyPreflightAuthoritySentinelAbsence(
	ctx context.Context,
	mode authoritySentinelAbsenceMode,
	err error,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return failure
	}
	if !mode.valid() {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if err == nil {
		if mode == authorityInitialSentinelAbsence {
			return newAuthorityPrimitiveFailure(OperationValidate, CauseUnsupported)
		}
		return newAuthorityPrimitiveFailure(OperationCompare, CauseUnstable)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return newAuthorityPrimitiveFailure(OperationProbe, CausePermission)
	}
	return newAuthorityPrimitiveFailure(OperationProbe, CauseUnstable)
}

func openPreflightAuthorityAbsoluteRoot(
	ctx context.Context,
) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
	if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return nil, failure
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK |
		unix.O_DIRECTORY | unix.O_NOFOLLOW_ANY
	for {
		fd, err := unix.Open(physicalRootPath, flags, 0)
		if errors.Is(err, unix.EINTR) {
			if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
				return nil, failure
			}
			continue
		}
		if err != nil {
			return nil, classifyPreflightAuthorityAcquisitionFailure(
				ctx,
				authorityExpectedPresentOpen,
				err,
			)
		}
		file := os.NewFile(uintptr(fd), physicalRootPath)
		if file == nil {
			return &authorityDescriptorAcquisition{
				close: func() error { return unix.Close(fd) },
			}, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		acquisition := &authorityDescriptorAcquisition{file: file, close: file.Close}
		if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
			return acquisition, failure
		}
		return acquisition, nil
	}
}

func openPreflightAuthorityRootHandle(
	ctx context.Context,
	root *os.Root,
) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
	if root == nil {
		return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return nil, failure
	}
	file, err := root.Open(".")
	if err != nil {
		return nil, classifyPreflightAuthorityAcquisitionFailure(
			ctx,
			authorityExpectedPresentOpen,
			err,
		)
	}
	acquisition := &authorityDescriptorAcquisition{file: file, close: file.Close}
	if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
		return acquisition, failure
	}
	return acquisition, nil
}

func probePreflightAuthorityRelativeKind(
	ctx context.Context,
	parent *os.File,
	name string,
	mode authorityRequiredOpenMode,
) (entryKind, *authorityPrimitiveFailure) {
	if parent == nil || name == "" || name == "." || name == ".." ||
		strings.Contains(name, string(filepath.Separator)) || !mode.valid() {
		return 0, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	for {
		if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return 0, failure
		}
		var stat unix.Stat_t
		err := unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
		runtime.KeepAlive(parent)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, classifyPreflightAuthorityPresenceFailure(ctx, mode, err)
		}
		kind, kindFailure := classifyPreflightAuthorityObservedKind(mode, uint32(stat.Mode))
		if kindFailure != nil {
			return 0, kindFailure
		}
		if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return 0, failure
		}
		return kind, nil
	}
}

func classifyPreflightAuthorityObservedKind(
	mode authorityRequiredOpenMode,
	platformMode uint32,
) (entryKind, *authorityPrimitiveFailure) {
	if !mode.valid() {
		return 0, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	kind, err := entryKindFromPlatformMode(platformMode)
	if err == nil {
		return kind, nil
	}
	if mode == authorityExpectedPresentOpen {
		return 0, newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
	}
	return 0, newAuthorityPrimitiveFailure(OperationValidate, CauseUnsupported)
}

func classifyPreflightAuthorityPresenceFailure(
	ctx context.Context,
	mode authorityRequiredOpenMode,
	err error,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
		return failure
	}
	if !mode.valid() || err == nil {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if errors.Is(err, fs.ErrNotExist) {
		return requiredAuthorityOpenNotFound(mode)
	}
	if errors.Is(err, fs.ErrPermission) {
		return newAuthorityPrimitiveFailure(OperationProbe, CausePermission)
	}
	return newAuthorityPrimitiveFailure(OperationProbe, CauseUnstable)
}

func classifyPreflightAuthorityWalkFailure(
	ctx context.Context,
	err error,
) *authorityPrimitiveFailure {
	if failure := authorityContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return failure
	}
	if err == nil {
		return newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if errors.Is(err, fs.ErrPermission) {
		return newAuthorityPrimitiveFailure(OperationWalk, CausePermission)
	}
	return newAuthorityPrimitiveFailure(OperationWalk, CauseUnstable)
}

func openPreflightAuthorityTypedRelativeNoFollow(
	ctx context.Context,
	parent *os.File,
	name string,
	kind entryKind,
	mode authorityRequiredOpenMode,
) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
	if parent == nil || name == "" || name == "." || name == ".." ||
		strings.Contains(name, string(filepath.Separator)) || !mode.valid() {
		return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	flags, err := darwinOpenFlags(kind, kind != entrySymlink)
	if err != nil {
		return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
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
			return nil, classifyPreflightAuthorityAcquisitionFailure(ctx, mode, openErr)
		}
		file := os.NewFile(uintptr(fd), name)
		if file == nil {
			return &authorityDescriptorAcquisition{
				close: func() error { return unix.Close(fd) },
			}, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		acquisition := &authorityDescriptorAcquisition{file: file, close: file.Close}
		if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
			return acquisition, failure
		}
		return acquisition, nil
	}
}

func openPreflightAuthorityNullRelativeNoFollow(
	ctx context.Context,
	parent *os.File,
	name string,
	mode authorityRequiredOpenMode,
) (*authorityDescriptorAcquisition, *authorityPrimitiveFailure) {
	if parent == nil || name == "" || name == "." || name == ".." ||
		strings.Contains(name, string(filepath.Separator)) || !mode.valid() {
		return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
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
			return nil, classifyPreflightAuthorityAcquisitionFailure(ctx, mode, openErr)
		}
		file := os.NewFile(uintptr(fd), name)
		if file == nil {
			return &authorityDescriptorAcquisition{
				close: func() error { return unix.Close(fd) },
			}, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
		}
		acquisition := &authorityDescriptorAcquisition{file: file, close: file.Close}
		if failure := authorityContextPrimitiveFailure(ctx, OperationOpen); failure != nil {
			return acquisition, failure
		}
		return acquisition, nil
	}
}

func readPreflightAuthorityDirectory(
	ctx context.Context,
	descriptor *os.File,
	limit int,
) ([]string, bool, *authorityPrimitiveFailure) {
	if descriptor == nil || limit <= 0 || limit > directoryReadBatchSize {
		return nil, false, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return nil, false, failure
	}
	entries, err := descriptor.ReadDir(limit)
	if failure := authorityContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
		return nil, false, failure
	}
	done := errors.Is(err, io.EOF)
	if err != nil && !done {
		return nil, false, classifyPreflightAuthorityWalkFailure(ctx, err)
	}
	if len(entries) > limit {
		return nil, false, newAuthorityPrimitiveFailure(OperationWalk, CauseLimit)
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		if entry == nil {
			return nil, false, newAuthorityPrimitiveFailure(
				OperationValidate,
				CauseInternalInvariant,
			)
		}
		names[index] = entry.Name()
	}
	return names, done, nil
}

func readPreflightAuthoritySymlinkForWalk(
	ctx context.Context,
	descriptor *os.File,
	expectedSize int64,
	limit uint64,
) (string, *authorityPrimitiveFailure) {
	return readPreflightAuthoritySymlinkForWalkWith(
		ctx,
		descriptor,
		expectedSize,
		limit,
		darwinFreadlink,
	)
}

func readPreflightAuthoritySymlinkForWalkWith(
	ctx context.Context,
	descriptor *os.File,
	expectedSize int64,
	limit uint64,
	readLink darwinFreadlinkFunc,
) (string, *authorityPrimitiveFailure) {
	if descriptor == nil || readLink == nil {
		return "", newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	bufferSize, expected, failure := preflightAuthoritySymlinkReadSize(expectedSize, limit)
	if failure != nil {
		return "", failure
	}
	buffer := make([]byte, bufferSize)
	for {
		if failure := authorityContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
			return "", failure
		}
		count, err := readLink(descriptor, buffer)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return "", classifyPreflightAuthorityWalkFailure(ctx, err)
		}
		if failure := authorityContextPrimitiveFailure(ctx, OperationWalk); failure != nil {
			return "", failure
		}
		if failure := validatePreflightAuthoritySymlinkRead(
			count,
			expected,
			limit,
			len(buffer),
		); failure != nil {
			return "", failure
		}
		return string(buffer[:count]), nil
	}
}

func preflightAuthoritySymlinkReadSize(
	expectedSize int64,
	limit uint64,
) (int, uint64, *authorityPrimitiveFailure) {
	maximumInt := uint64(^uint(0) >> 1)
	expected, err := nonnegativeByteSize(expectedSize)
	if err != nil || limit == 0 || limit >= maximumInt || expected > limit {
		return 0, 0, newAuthorityPrimitiveFailure(OperationWalk, CauseLimit)
	}
	if expected == 0 {
		return 0, 0, newAuthorityPrimitiveFailure(OperationWalk, CauseUnstable)
	}
	// expected is bounded strictly below the platform maximum int above.
	return int(expected) + 1, expected, nil //nolint:gosec // Explicit platform-int bound precedes conversion.
}

func validatePreflightAuthoritySymlinkRead(
	count uintptr,
	expected uint64,
	limit uint64,
	bufferSize int,
) *authorityPrimitiveFailure {
	if count > uintptr(limit) {
		return newAuthorityPrimitiveFailure(OperationWalk, CauseLimit)
	}
	if count != uintptr(expected) || count > uintptr(bufferSize) {
		return newAuthorityPrimitiveFailure(OperationWalk, CauseUnstable)
	}
	return nil
}

func acquirePreflightAuthorityRawNullACL(
	ctx context.Context,
	descriptor *os.File,
) ([]byte, *authorityPrimitiveFailure) {
	if descriptor == nil {
		return nil, newAuthorityPrimitiveFailure(OperationValidate, CauseInternalInvariant)
	}
	standardAttributes := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr:  unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	standard := make([]byte, darwinACLAttributeBufferSize)
	for {
		if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return nil, failure
		}
		err := darwinFgetattrlist(int(descriptor.Fd()), &standardAttributes, standard)
		runtime.KeepAlive(descriptor)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err == nil {
			if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
				return nil, failure
			}
			return append([]byte{0}, standard...), nil
		}
		if !errors.Is(err, unix.EINVAL) {
			if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
				return nil, failure
			}
			return nil, newAuthorityPrimitiveFailure(
				OperationProbe,
				authorityPrimitiveIOCause(err),
			)
		}
		break
	}
	fallbackAttributes := unix.Attrlist{
		Bitmapcount: unix.ATTR_BIT_MAP_COUNT,
		Commonattr: unix.ATTR_CMN_RETURNED_ATTRS |
			unix.ATTR_CMN_EXTENDED_SECURITY,
	}
	fallback := make([]byte, darwinNullACLResultSize)
	for {
		if failure := authorityContextPrimitiveFailure(ctx, OperationProbe); failure != nil {
			return nil, failure
		}
		err := darwinFgetattrlistWithOptions(
			int(descriptor.Fd()),
			&fallbackAttributes,
			fallback,
			darwinFSOptReportFullSize|unix.FSOPT_PACK_INVAL_ATTRS,
		)
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
		return append([]byte{1}, fallback...), nil
	}
}

func parsePreflightAuthorityRawNullACL(
	ctx context.Context,
	raw []byte,
) (Digest, *authorityPrimitiveFailure) {
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return Digest{}, failure
	}
	if len(raw) < 2 {
		return Digest{}, newAuthorityPrimitiveFailure(OperationParse, CauseMalformed)
	}
	var (
		digest Digest
		err    error
	)
	switch raw[0] {
	case 0:
		digest, err = digestExtendedSecurityResult(raw[1:])
	case 1:
		digest, err = digestNullDeviceACLResult(raw[1:])
	default:
		err = fs.ErrInvalid
	}
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

func parsePreflightAuthorityRawNullACLForComparison(
	ctx context.Context,
	raw []byte,
) (authorityACLComparisonObservation, *authorityPrimitiveFailure) {
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return authorityACLComparisonObservation{}, failure
	}
	if len(raw) < 2 {
		return authorityACLComparisonObservation{}, newAuthorityPrimitiveFailure(
			OperationParse,
			CauseMalformed,
		)
	}
	var (
		observation darwinACLObservation
		err         error
	)
	switch raw[0] {
	case 0:
		observation, err = observeExtendedSecurityResult(raw[1:])
	case 1:
		observation, err = observePreflightAuthorityNullACLResult(raw[1:])
	default:
		err = fs.ErrInvalid
	}
	if err != nil {
		return authorityACLComparisonObservation{}, authorityPrimitiveFailureFromError(
			OperationParse,
			err,
			CauseMalformed,
		)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return authorityACLComparisonObservation{}, failure
	}
	return authorityACLComparisonObservation{
		digest: observation.digest,
		policyFailure: authorityPrimitiveFailureFromError(
			OperationValidate,
			observation.policyErr,
			CausePermission,
		),
	}, nil
}

func observePreflightAuthorityNullACLResult(
	raw []byte,
) (darwinACLObservation, error) {
	digest, err := digestNullDeviceACLResult(raw)
	if err == nil {
		return darwinACLObservation{digest: digest}, nil
	}
	if !preflightAuthorityNullACLReportsExtendedSecurity(raw) {
		return darwinACLObservation{}, err
	}
	hasher := sha256.New()
	writeLengthPrefixed(hasher, []byte("darwin-null-acl-rejected/v1"))
	writeLengthPrefixed(hasher, raw)
	var rejected Digest
	copy(rejected[:], hasher.Sum(nil))
	return darwinACLObservation{
		digest: rejected,
		policyErr: fail(
			CauseUnsupported,
			"null-device extended security attribute present",
		),
	}, nil
}

func preflightAuthorityNullACLReportsExtendedSecurity(raw []byte) bool {
	if len(raw) != darwinNullACLResultSize ||
		binary.LittleEndian.Uint32(raw[0:4]) < darwinNullACLResultSize ||
		binary.LittleEndian.Uint32(raw[4:8]) !=
			unix.ATTR_CMN_RETURNED_ATTRS|unix.ATTR_CMN_EXTENDED_SECURITY {
		return false
	}
	for offset := 8; offset < 24; offset += 4 {
		if binary.LittleEndian.Uint32(raw[offset:offset+4]) != 0 {
			return false
		}
	}
	return true
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

// classifyPreflightAuthorityAcquisitionFailure is A2's classifier for a
// distinct descriptor-acquisition primitive. The legacy A1 classifier above
// also owns a lookup observation and therefore remains unchanged.
func classifyPreflightAuthorityAcquisitionFailure(
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
	if errors.Is(err, unix.ENOENT) {
		return requiredAuthorityOpenNotFound(mode)
	}
	if errors.Is(err, unix.ELOOP) || errors.Is(err, unix.ENOTDIR) {
		if mode == authorityExpectedPresentOpen {
			return newAuthorityPrimitiveFailure(OperationCompare, CauseIdentity)
		}
		return newAuthorityPrimitiveFailure(OperationOpen, CauseIdentity)
	}
	return newAuthorityPrimitiveFailure(OperationOpen, authorityPrimitiveIOCause(err))
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

func parsePreflightAuthorityRawACLForComparison(
	ctx context.Context,
	raw []byte,
) (authorityACLComparisonObservation, *authorityPrimitiveFailure) {
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return authorityACLComparisonObservation{}, failure
	}
	observation, err := observeExtendedSecurityResult(raw)
	if err != nil {
		return authorityACLComparisonObservation{}, authorityPrimitiveFailureFromError(
			OperationParse,
			err,
			CauseMalformed,
		)
	}
	if failure := authorityContextPrimitiveFailure(ctx, OperationParse); failure != nil {
		return authorityACLComparisonObservation{}, failure
	}
	return authorityACLComparisonObservation{
		digest: observation.digest,
		policyFailure: authorityPrimitiveFailureFromError(
			OperationValidate,
			observation.policyErr,
			CausePermission,
		),
	}, nil
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
