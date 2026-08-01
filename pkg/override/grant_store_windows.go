//go:build windows

package override

import (
	"fmt"
	"os"
)

func prepareGrantDir(string) error {
	if !enforceOperatorOwnership {
		return os.MkdirAll(GrantDir(), 0o700)
	}
	return fmt.Errorf("%w: operator-owned grant storage is not implemented on Windows", ErrGrantUntrusted)
}

func grantFileMode() os.FileMode { return 0o600 }

func validateGrantPath(path string) error {
	if !enforceOperatorOwnership {
		_, err := os.Lstat(path)
		return err
	}
	return fmt.Errorf("%w: operator-owned grant storage is not implemented on Windows", ErrGrantUntrusted)
}
