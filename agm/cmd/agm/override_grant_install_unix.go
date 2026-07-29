//go:build !darwin && !windows

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// installOperatorGrant streams the confirmed bytes directly across the sudo
// boundary. There is no same-user staging file for an unattended agent to race
// between confirmation and installation, and AGM itself is never elevated.
func installOperatorGrant(data []byte, path string) error {
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
