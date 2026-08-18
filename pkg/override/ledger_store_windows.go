//go:build windows

package override

import (
	"fmt"
	"os"
)

func appendOperatorLedger([]Use, string) error {
	return fmt.Errorf("%w: operator-owned ledger storage is not implemented on Windows", ErrLedgerUntrusted)
}

func validateLedgerPath(path string) error {
	if !enforceOperatorLedger {
		_, err := os.Lstat(path)
		return err
	}
	return fmt.Errorf("%w: operator-owned ledger storage is not implemented on Windows", ErrLedgerUntrusted)
}
