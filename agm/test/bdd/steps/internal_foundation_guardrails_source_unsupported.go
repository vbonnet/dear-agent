//go:build !darwin && !linux

package steps

import (
	"context"
	"fmt"
	"io/fs"
	"os"
)

func buildAuthorityWaitOwnershipSourceReadsSupported() bool {
	return false
}

func scanBuildAuthorityWaitOwnershipSecure(
	context.Context,
	string,
	buildAuthorityWaitOwnershipLimits,
	buildAuthorityWaitOwnershipHooks,
) (buildAuthorityWaitOwnershipScan, error) {
	return buildAuthorityWaitOwnershipScan{},
		fmt.Errorf("secure production-source reads are unsupported on this platform")
}

func openBuildAuthorityWaitOwnershipRoot(string) (*os.File, fs.FileInfo, error) {
	return nil, nil, fs.ErrInvalid
}

func openBuildAuthorityWaitOwnershipAt(*os.File, string) (*os.File, fs.FileInfo, bool, error) {
	return nil, nil, false, fs.ErrInvalid
}

func readBuildAuthorityWaitOwnershipLink(*os.File, string) (string, error) {
	return "", fs.ErrInvalid
}

func buildAuthorityWaitOwnershipSameChangeTime(fs.FileInfo, fs.FileInfo) bool {
	return false
}
