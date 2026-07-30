//go:build !darwin && !windows

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// installOperatorGrant performs the complete write and mode change in one
// fixed privileged command. The elevated shell program is a literal compiled
// into AGM; only the already-validated destination path is positional. sudo -k
// requires fresh authentication and, when used with a command, deliberately
// does not create or refresh a timestamp another same-user process could reuse.
func installOperatorGrant(data []byte, path string) error {
	passwordless, err := sudoValidationIsPasswordless()
	if err != nil {
		return err
	}
	if passwordless {
		return errors.New("fresh operator authentication is unavailable: passwordless sudo cannot approve a dangerous override")
	}
	cmd := exec.Command("/usr/bin/sudo", operatorGrantInstallArgs(path)...)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install operator-owned override grant: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
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

func removeOperatorGrant(path string) error {
	output, err := exec.Command("/usr/bin/sudo", "-k", "/bin/rm", "-f", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove operator-owned override grant: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
