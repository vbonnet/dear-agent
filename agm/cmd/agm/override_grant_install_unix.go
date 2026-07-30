//go:build !darwin && !windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// installOperatorGrant streams the confirmed bytes directly across the sudo
// boundary. There is no same-user staging file for an unattended agent to race
// between confirmation and installation, and AGM itself is never elevated.
func installOperatorGrant(data []byte, path string) error {
	if err := requireFreshSudoAuthentication(); err != nil {
		return err
	}
	installErr := writeOperatorGrant(data, path)
	return errors.Join(installErr, invalidateSudoAuthentication())
}

func requireFreshSudoAuthentication() error {
	return requireFreshAuthentication(
		invalidateSudoAuthentication,
		sudoValidationIsPasswordless,
		promptForSudoAuthentication,
	)
}

func sudoValidationIsPasswordless() (bool, error) {
	// Probe a harmless command that is deliberately outside the installed
	// NOPASSWD ledger-helper rule. sudo's generic -v pseudocommand may succeed
	// when any matching NOPASSWD rule exists, which would make the prerequisite
	// helper installation disable every later approval.
	err := exec.Command("/usr/bin/sudo", freshUnixAuthenticationArgs(true)...).Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, fmt.Errorf("probe passwordless sudo validation: %w", err)
}

func promptForSudoAuthentication() error {
	// Authenticate this command specifically so the ledger helper's narrow
	// NOPASSWD rule cannot satisfy the fresh human challenge.
	cmd := exec.Command("/usr/bin/sudo", freshUnixAuthenticationArgs(false)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func invalidateSudoAuthentication() error {
	output, err := exec.Command("/usr/bin/sudo", "-k").CombinedOutput()
	if err != nil {
		return fmt.Errorf("invalidate cached sudo authentication: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func writeOperatorGrant(data []byte, path string) error {
	cmd := exec.Command("/usr/bin/sudo", "/usr/bin/tee", path)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install operator-owned override grant: %w: %s", err, strings.TrimSpace(string(output)))
	}
	output, err = exec.Command("/usr/bin/sudo", "/bin/chmod", "0644", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("secure operator-owned override grant: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func removeOperatorGrant(path string) error {
	output, err := exec.Command("/usr/bin/sudo", "/bin/rm", "-f", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove operator-owned override grant: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
