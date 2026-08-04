//go:build darwin

package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// installOperatorGrant asks macOS Authorization Services to open the fixed
// /etc grant path for writing. AGM itself stays unprivileged: no agent-writable
// binary is ever executed as root.
func installOperatorGrant(data []byte, path string) error {
	cmd := exec.Command("/usr/libexec/authopen", "-c", "-m", "0644", "-w", path)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install operator-owned override grant: %w: %s", err, strings.TrimSpace(string(output)))
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
