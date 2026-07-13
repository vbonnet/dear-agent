package steps

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindBDDRepoRootFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/root\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nested := filepath.Join(root, "agm", "test", "bdd", "steps")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("create nested package: %v", err)
	}

	got, ok := findBDDRepoRoot(nested)
	if !ok {
		t.Fatal("findBDDRepoRoot did not find synthetic checkout")
	}
	if got != root {
		t.Fatalf("findBDDRepoRoot() = %q, want %q", got, root)
	}
}

func TestFindBDDRepoRootRejectsNonCheckout(t *testing.T) {
	if root, ok := findBDDRepoRoot(t.TempDir()); ok {
		t.Fatalf("findBDDRepoRoot unexpectedly found %q", root)
	}
}
