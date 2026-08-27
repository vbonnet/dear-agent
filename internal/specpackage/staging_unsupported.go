//go:build !darwin && !linux

package specpackage

import (
	"context"
	"fmt"
	"io/fs"
)

type stagedFilesystem struct{}

func newStagedFilesystem(*anchoredRoot, *stagedRootIdentity) (*stagedFilesystem, error) {
	return nil, fmt.Errorf("portable SPEC package staging is unsupported on this platform")
}

func (*stagedFilesystem) Mkdir(string) error {
	return fmt.Errorf("portable SPEC package staging is unsupported on this platform")
}

func (*stagedFilesystem) WriteFile(string, []byte, fs.FileMode) error {
	return fmt.Errorf("portable SPEC package staging is unsupported on this platform")
}

func (*stagedFilesystem) Sync() error {
	return fmt.Errorf("portable SPEC package staging is unsupported on this platform")
}

func (*stagedFilesystem) Verify(context.Context) error {
	return fmt.Errorf("portable SPEC package staging is unsupported on this platform")
}

func (*stagedFilesystem) Close() error { return nil }
