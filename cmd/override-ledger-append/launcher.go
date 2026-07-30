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
	companionIdentityPath    = "/usr/local/libexec/dear-agent-override-ledger-agm-mcp-server.identity"
	maxLauncherIdentityBytes = 256
)

type processParentLookup func(int) (int, error)
type processPathLookup func(int) (string, error)

// authenticateLauncher authenticates the unprivileged AGM process that invoked
// sudo. Capability issuance may also originate in the separately attested,
// co-installed MCP companion because that process prepares launches directly.
// Ledger appends and capability consumption remain AGM-only.
func authenticateLauncher(launcherPID int, allowCompanion bool) error {
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
	expectedAGM, err := loadLauncherIdentity(launcherIdentityPath)
	if err != nil {
		return err
	}
	expected := []string{expectedAGM}
	if allowCompanion {
		companion, companionErr := loadLauncherIdentity(companionIdentityPath)
		if companionErr != nil {
			return companionErr
		}
		expected = append(expected, companion)
	}
	actual, err := processCodeIdentity(launcherPID)
	if err != nil {
		return fmt.Errorf("inspect running launcher code identity: %w", err)
	}
	if !launcherIdentityMatches(actual, expected) {
		return errors.New(
			"running launcher identity does not match any operator-approved identity",
		)
	}
	return nil
}

func launcherIdentityMatches(actual string, expected []string) bool {
	matched := 0
	for _, identity := range expected {
		matched |= subtle.ConstantTimeCompare([]byte(identity), []byte(actual))
	}
	return matched == 1
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

func loadLauncherIdentity(path string) (string, error) {
	if err := validateRootOwnedPath(filepath.Dir(path), true); err != nil {
		return "", fmt.Errorf("validate launcher policy directory: %w", err)
	}
	if err := validateRootOwnedPath(path, false); err != nil {
		return "", fmt.Errorf("validate root-owned launcher identity: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return parseLauncherIdentity(data)
}

func parseLauncherIdentity(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxLauncherIdentityBytes {
		return "", errors.New("root-owned launcher identity has an invalid size")
	}
	raw := string(data)
	identity := strings.TrimSuffix(raw, "\n")
	if identity == "" ||
		(raw != identity && raw != identity+"\n") ||
		strings.ContainsAny(identity, " \t\r\n") {
		return "", errors.New("root-owned launcher identity is not canonical")
	}
	prefix, digest, ok := strings.Cut(identity, ":")
	if !ok || prefix != codeIdentityAlgorithm() {
		return "", fmt.Errorf("root-owned launcher identity uses unsupported algorithm %q", prefix)
	}
	if len(digest) != codeIdentityHexLength() {
		return "", fmt.Errorf("root-owned launcher identity has invalid %s digest length", prefix)
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return "", fmt.Errorf("root-owned launcher identity has invalid %s digest: %w", prefix, err)
	}
	return identity, nil
}
