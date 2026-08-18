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

// installOperatorGrant performs the complete write and mode change in one
// fixed privileged command. The elevated shell program is a literal compiled
// into AGM; only the already-validated destination path is positional. sudo -k
// requires fresh authentication and, when used with a command, deliberately
// does not create or refresh a timestamp another same-user process could reuse.
func installOperatorGrant(data []byte, path string) error {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlinked operator grant target %q", path)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect operator grant target %q: %w", path, err)
	}
	passwordless, err := sudoInstallerIsPasswordless(path)
	if err != nil {
		return err
	}
	if passwordless {
		return errors.New("fresh operator authentication is unavailable: passwordless sudo cannot approve a dangerous override")
	}
	cmd := exec.Command("/usr/bin/sudo", operatorGrantInstallArgs(path, false)...)
	cmd.Stdin = bytes.NewReader(append([]byte(unixOperatorGrantInstallInput), data...))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install operator-owned override grant: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func sudoInstallerIsPasswordless(path string) (bool, error) {
	// Probe the exact privileged command that will perform the installation.
	// sudoers command matching sees the same executable and arguments in the
	// probe and real calls; only sudo's -n flag and stdin differ. If that exact
	// command is NOPASSWD, the fixed shell consumes the probe marker, performs
	// no write, and returns the dedicated status below. Otherwise -n fails
	// before the shell runs, and the real sudo -k call must authenticate.
	cmd := exec.Command("/usr/bin/sudo", operatorGrantInstallArgs(path, true)...)
	cmd.Stdin = strings.NewReader(unixOperatorGrantProbeInput)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return false, errors.New("probe privileged override installer: installer probe returned success unexpectedly")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ExitCode() == unixOperatorGrantProbeExitCode {
			return true, nil
		}
		return false, nil
	}
	return false, fmt.Errorf("probe privileged override installer: %w: %s", err, strings.TrimSpace(string(output)))
}

func removeOperatorGrant(path string) error {
	output, err := exec.Command("/usr/bin/sudo", "-k", "/bin/rm", "-f", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("remove operator-owned override grant: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
