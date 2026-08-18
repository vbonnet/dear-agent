//go:build !darwin && !linux && !windows

package main

import "errors"

func validateProcessImage(int) error {
	return errors.New("non-injectable process authentication is unsupported on this platform")
}

func processParentPID(int) (int, error) {
	return 0, errors.New("launch-bound process authentication is unsupported on this platform")
}

func processExecutableSHA256(int) (string, error) {
	return "", errors.New("launch-bound process authentication is unsupported on this platform")
}

func processExecutablePath(int) (string, error) {
	return "", errors.New("launch-bound process authentication is unsupported on this platform")
}

func processCodeIdentity(int) (string, error) {
	return "", errors.New("launch-bound process authentication is unsupported on this platform")
}

func codeIdentityAlgorithm() string { return "unsupported" }
func codeIdentityHexLength() int    { return 0 }
