//go:build !windows

package override

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const operatorLedgerAppendHelper = "/usr/local/libexec/dear-agent-override-ledger-append"

// appendOperatorLedger crosses only the fixed, root-owned helper boundary.
// The helper accepts no arguments, validates and bounds one canonical
// transaction, and owns the fixed destination. The invoking AGM binary is
// never elevated.
func appendOperatorLedger(uses []Use, path string) error {
	if path != operatorLedgerPath {
		return fmt.Errorf("%w: privileged append destination is %q, want fixed %q",
			ErrLedgerUntrusted, path, operatorLedgerPath)
	}
	if err := validateRootOwnedPath(filepath.Dir(operatorLedgerAppendHelper), true); err != nil {
		return fmt.Errorf(
			"%w: ledger helper parent is not installed securely at %s: %w",
			ErrLedgerUntrusted, filepath.Dir(operatorLedgerAppendHelper), err,
		)
	}
	if err := validateRootOwnedPath(operatorLedgerAppendHelper, false); err != nil {
		return fmt.Errorf(
			"%w: ledger helper is not installed securely at %s: %w (run the operator ledger-helper installation before approving overrides)",
			ErrLedgerUntrusted, operatorLedgerAppendHelper, err,
		)
	}
	data, err := EncodePrivilegedAppendRequest(uses, os.Getpid())
	if err != nil {
		return err
	}
	cmd := exec.Command("/usr/bin/sudo", "-n", operatorLedgerAppendHelper)
	cmd.Stdin = strings.NewReader(string(data))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf(
			"append operator-owned override ledger through fixed helper: %w: %s (verify its exact NOPASSWD sudoers rule)",
			err, strings.TrimSpace(string(output)),
		)
	}
	return nil
}
