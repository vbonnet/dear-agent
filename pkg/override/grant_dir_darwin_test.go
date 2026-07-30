//go:build darwin

package override

import (
	"os"
	"testing"
)

func TestOperatorGrantDirUsesCanonicalDarwinPath(t *testing.T) {
	if operatorGrantDir != "/private/etc" {
		t.Fatalf("operatorGrantDir = %q, want canonical /private/etc", operatorGrantDir)
	}
	if got := GrantDir(); got != operatorGrantDir {
		t.Fatalf("GrantDir = %q, want canonical %q", got, operatorGrantDir)
	}
	info, err := os.Lstat(operatorGrantDir)
	if err != nil {
		t.Fatalf("lstat canonical operator grant directory: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("canonical operator grant directory %q is a symlink", operatorGrantDir)
	}
	if err := validateRootOwnedPath(operatorGrantDir, true); err != nil {
		t.Fatalf("validate canonical operator grant directory: %v", err)
	}
}
