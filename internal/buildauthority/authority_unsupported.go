//go:build !darwin || !arm64

package buildauthority

import (
	"context"
	"os"
)

func openFixedNullDeviceNoFollow(int) (*os.File, error) {
	return nil, unsupportedFilesystemError()
}

func validatePlatformNullDeviceSnapshot(fileSnapshot) error {
	return unsupportedFilesystemError()
}

func inspectNullAuthorityDescriptorContext(context.Context, *os.File) (fileSnapshot, mountSnapshot, Digest, error) {
	return fileSnapshot{}, mountSnapshot{}, Digest{}, unsupportedFilesystemError()
}
