//go:build !windows

package main

import (
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxLauncherAncestryDepth = 4
	installedSudoPath        = "/usr/bin/sudo"
	launcherIdentityPath     = "/usr/local/libexec/dear-agent-override-ledger-agm.identity"
	maxLauncherIdentityBytes = 256
)

type processParentLookup func(int) (int, error)
type processPathLookup func(int) (string, error)

// authenticateLauncher authenticates the unprivileged AGM process that invoked
// sudo. A PID supplied by an unrelated shell cannot pass: it must be the first
// non-sudo ancestor of this fixed helper, and its kernel-backed executable
// identity must match the policy installed with the reviewed helper bytes.
// This excludes agent shells that merely descend from a long-running AGM
// supervisor.
func authenticateLauncher(launcherPID int) error {
	if launcherPID <= 1 {
		return errors.New("launcher PID is invalid")
	}
	if err := validateRootOwnedPath(filepath.Dir(installedSudoPath), true); err != nil {
		return fmt.Errorf("validate sudo parent directory: %w", err)
	}
	if err := validateRootOwnedPath(installedSudoPath, false); err != nil {
		return fmt.Errorf("validate sudo executable: %w", err)
	}
	if err := validateLauncherChain(
		os.Getppid(),
		launcherPID,
		processParentPID,
		processExecutablePath,
	); err != nil {
		return err
	}
	expected, err := loadLauncherIdentity()
	if err != nil {
		return err
	}
	actual, err := processCodeIdentity(launcherPID)
	if err != nil {
		return fmt.Errorf("inspect running AGM code identity: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
		return fmt.Errorf(
			"running AGM identity %q does not match operator-approved identity %q",
			actual, expected,
		)
	}
	return nil
}

func validateLauncherChain(
	current int,
	launcherPID int,
	parent processParentLookup,
	executablePath processPathLookup,
) error {
	for range maxLauncherAncestryDepth {
		if current == launcherPID {
			return nil
		}
		if current <= 1 {
			break
		}
		path, err := executablePath(current)
		if err != nil {
			return fmt.Errorf("inspect privileged intermediary PID %d: %w", current, err)
		}
		if path != installedSudoPath {
			return fmt.Errorf(
				"PID %d (%s) is between the privileged helper and claimed AGM launcher PID %d",
				current, path, launcherPID,
			)
		}
		parentPID, err := parent(current)
		if err != nil {
			return fmt.Errorf("inspect launcher ancestry at PID %d: %w", current, err)
		}
		if parentPID == current {
			break
		}
		current = parentPID
	}
	return fmt.Errorf(
		"PID %d is not the immediate authenticated AGM caller within %d sudo hops",
		launcherPID, maxLauncherAncestryDepth,
	)
}

func loadLauncherIdentity() (string, error) {
	if err := validateRootOwnedPath(filepath.Dir(launcherIdentityPath), true); err != nil {
		return "", fmt.Errorf("validate AGM caller policy directory: %w", err)
	}
	if err := validateRootOwnedPath(launcherIdentityPath, false); err != nil {
		return "", fmt.Errorf("validate root-owned AGM caller identity: %w", err)
	}
	data, err := os.ReadFile(launcherIdentityPath)
	if err != nil {
		return "", err
	}
	return parseLauncherIdentity(data)
}

func parseLauncherIdentity(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxLauncherIdentityBytes {
		return "", errors.New("root-owned AGM caller identity has an invalid size")
	}
	raw := string(data)
	identity := strings.TrimSuffix(raw, "\n")
	if identity == "" ||
		(raw != identity && raw != identity+"\n") ||
		strings.ContainsAny(identity, " \t\r\n") {
		return "", errors.New("root-owned AGM caller identity is not canonical")
	}
	prefix, digest, ok := strings.Cut(identity, ":")
	if !ok || prefix != codeIdentityAlgorithm() {
		return "", fmt.Errorf("root-owned AGM caller identity uses unsupported algorithm %q", prefix)
	}
	if len(digest) != codeIdentityHexLength() {
		return "", fmt.Errorf("root-owned AGM caller identity has invalid %s digest length", prefix)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", fmt.Errorf("root-owned AGM caller identity has invalid %s digest: %w", prefix, err)
	}
	return identity, nil
}
