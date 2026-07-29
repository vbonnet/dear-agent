//go:build darwin

package override

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// appendOperatorLedger asks Authorization Services to append the exact record
// to the fixed root-owned ledger. AGM remains unprivileged.
func appendOperatorLedger(data []byte, path string) error {
	cmd := exec.Command("/usr/libexec/authopen", "-c", "-m", "0644", "-w", "-a", path)
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("append operator-owned override ledger: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
