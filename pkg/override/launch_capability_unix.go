//go:build !windows

package override

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func issueLaunchCapability(capability LaunchCapability) error {
	data, err := EncodePrivilegedLaunchCapabilityRequest(capability, os.Getpid())
	if err != nil {
		return err
	}
	return invokeLaunchCapabilityHelper(data, "issue root-attested launch capability")
}

// ConsumeLaunchCapability atomically invalidates a root-attested sidecar.
// The helper compares the complete canonical capability before unlinking it,
// so a copied private handoff cannot replay the same parent authorization.
func ConsumeLaunchCapability(capability LaunchCapability) error {
	data, err := EncodePrivilegedConsumeLaunchCapabilityRequest(capability, os.Getpid())
	if err != nil {
		return err
	}
	return invokeLaunchCapabilityHelper(data, "consume root-attested launch capability")
}

func invokeLaunchCapabilityHelper(data []byte, action string) error {
	if err := validateRootOwnedPath(filepath.Dir(operatorLedgerAppendHelper), true); err != nil {
		return fmt.Errorf(
			"%w: ledger helper parent is not installed securely at %s: %w",
			ErrLedgerUntrusted, filepath.Dir(operatorLedgerAppendHelper), err,
		)
	}
	if err := validateRootOwnedPath(operatorLedgerAppendHelper, false); err != nil {
		return fmt.Errorf(
			"%w: ledger helper is not installed securely at %s: %w",
			ErrLedgerUntrusted, operatorLedgerAppendHelper, err,
		)
	}
	cmd := exec.Command("/usr/bin/sudo", "-n", operatorLedgerAppendHelper)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"%s through fixed helper: %w: %s",
			action, err, strings.TrimSpace(string(output)),
		)
	}
	return nil
}

// LoadLaunchCapability reads and authenticates one fixed root-owned sidecar.
func LoadLaunchCapability(id string) (LaunchCapability, error) {
	path, err := LaunchCapabilityPath(id)
	if err != nil {
		return LaunchCapability{}, err
	}
	if err := validateRootOwnedPath(LaunchCapabilityDir(), true); err != nil {
		return LaunchCapability{}, fmt.Errorf("validate launch capability directory: %w", err)
	}
	if err := validateRootOwnedPath(path, false); err != nil {
		return LaunchCapability{}, fmt.Errorf("validate launch capability sidecar: %w", err)
	}
	file, err := os.Open(path)
	if err != nil {
		return LaunchCapability{}, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, MaxLaunchCapabilityBytes+1))
	if err != nil {
		return LaunchCapability{}, err
	}
	if len(data) > MaxLaunchCapabilityBytes {
		return LaunchCapability{}, errors.New("launch capability sidecar exceeds its size limit")
	}
	capability, err := DecodeLaunchCapability(data)
	if err != nil {
		return LaunchCapability{}, err
	}
	if capability.ID != id || filepath.Base(path) != id+".json" {
		return LaunchCapability{}, errors.New("launch capability sidecar identity does not match its path")
	}
	return capability, nil
}
