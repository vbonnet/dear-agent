//go:build !windows

package override

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func validateLedgerPath(path string) error {
	if !enforceOperatorLedger {
		_, err := os.Lstat(path)
		return err
	}
	if err := validateRootOwnedPath(filepath.Dir(path), true); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrLedgerUntrusted, err)
	}
	if err := validateRootOwnedPath(path, false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrLedgerUntrusted, err)
	}
	return nil
}
