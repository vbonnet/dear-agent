//go:build !darwin || !arm64

package buildauthority

import (
	"context"
	"os"
)

const (
	platformModeTypeMask  = uint32(0xf000)
	platformModeDirectory = uint32(0x4000)
	platformModeRegular   = uint32(0x8000)
	platformModeSymlink   = uint32(0xa000)
)

func openAbsoluteNoFollow(string, entryKind) (*os.File, error) {
	return nil, unsupportedFilesystemError()
}

func openRelativeNoFollow(int, string, entryKind) (*os.File, error) {
	return nil, unsupportedFilesystemError()
}

func relativeEntryKindNoFollow(int, string) (entryKind, error) {
	return 0, unsupportedFilesystemError()
}

func removeRetainedDirectoryAt(int, string) error {
	return unsupportedFilesystemError()
}

func retainedDescriptorPath(*os.File) (string, error) {
	return "", unsupportedFilesystemError()
}

func readOpenedSymlinkContext(context.Context, *os.File, uint64) (string, error) {
	return "", unsupportedFilesystemError()
}

func inspectDescriptorContext(context.Context, *os.File) (fileSnapshot, mountSnapshot, Digest, error) {
	return fileSnapshot{}, mountSnapshot{}, Digest{}, unsupportedFilesystemError()
}

func unsupportedFilesystemError() error {
	return fail(CauseUnsupported, "build authority requires darwin/arm64")
}
