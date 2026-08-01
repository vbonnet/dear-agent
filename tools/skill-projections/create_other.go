//go:build !darwin && !linux

package main

import (
	"fmt"
	"runtime"
)

type repositoryRoot struct{}

func openRepositoryRoot(_ string) (*repositoryRoot, error) {
	return nil, fmt.Errorf("race-safe retained-root operations are unavailable on %s", runtime.GOOS)
}

func (root *repositoryRoot) close() error { return nil }

func (root *repositoryRoot) identity() string { return "unavailable" }

func (root *repositoryRoot) verifyPathIdentity(_ string, label string) error {
	return fmt.Errorf("bind %s: race-safe retained-root identity checks are unavailable on %s", label, runtime.GOOS)
}

func (root *repositoryRoot) readRegular(_ string, label string, _ int64) ([]byte, error) {
	return nil, fmt.Errorf("read %s: race-safe retained-root operations are unavailable on %s", label, runtime.GOOS)
}

func (root *repositoryRoot) createExclusive(_ string, _ []byte) (bool, error) {
	return false, fmt.Errorf("race-safe retained-root write mode is unavailable on %s", runtime.GOOS)
}

func (root *repositoryRoot) targetAbsent(_ string) (bool, error) {
	return false, fmt.Errorf("race-safe retained-root target inspection is unavailable on %s", runtime.GOOS)
}

func (root *repositoryRoot) scanGeneratedMarkers(_ markerScanLimits) ([]string, error) {
	return nil, fmt.Errorf("race-safe retained-root marker scanning is unavailable on %s", runtime.GOOS)
}
