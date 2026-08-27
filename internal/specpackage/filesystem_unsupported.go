//go:build !darwin && !linux

package specpackage

import (
	"context"
	"fmt"
)

type anchoredRoot struct{}
type stagedRootIdentity struct {
	file fileIdentity
}

func openAnchoredRoot(string) (*anchoredRoot, error) {
	return nil, fmt.Errorf("portable SPEC package validation is unsupported on this platform; requires handle-anchored filesystem support on darwin or linux")
}

func (*anchoredRoot) Close() error { return nil }

func (*anchoredRoot) verifyVisible() error {
	return fmt.Errorf("portable SPEC package validation is unsupported on this platform")
}

func (*anchoredRoot) readTree(context.Context, string) ([]treeEntry, error) {
	return nil, fmt.Errorf("portable SPEC package validation is unsupported on this platform")
}

func (*anchoredRoot) readRegular(context.Context, string, int64) (fileSnapshot, error) {
	return fileSnapshot{}, fmt.Errorf("portable SPEC package validation is unsupported on this platform")
}

func readStandaloneRegular(context.Context, string, int64) (fileSnapshot, error) {
	return fileSnapshot{}, fmt.Errorf("portable SPEC package validation is unsupported on this platform")
}

func validateStagingParentOutsideSource(*anchoredRoot, *anchoredRoot) error {
	return fmt.Errorf("portable SPEC package staging is unsupported on this platform")
}

func createPrivateStagingRoot(*anchoredRoot, *anchoredRoot) (string, stagedRootIdentity, *anchoredRoot, error) {
	return "", stagedRootIdentity{}, nil, fmt.Errorf("portable SPEC package staging is unsupported on this platform")
}

func sameStagedRoot(string, stagedRootIdentity) (bool, error) {
	return false, fmt.Errorf("portable SPEC package staging is unsupported on this platform")
}
