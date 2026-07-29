//go:build !darwin && !windows

package override

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// appendOperatorLedger crosses only the fixed system tee boundary. The
// invoking AGM binary is never elevated.
func appendOperatorLedger(data []byte, path string) error {
	cmd := exec.Command("/usr/bin/sudo", "/usr/bin/tee", "-a", path)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("append operator-owned override ledger: %w: %s", err, strings.TrimSpace(string(output)))
	}
	output, err = exec.Command("/usr/bin/sudo", "/bin/chmod", "0644", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("secure operator-owned override ledger: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
