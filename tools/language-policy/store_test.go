package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRootForTest resolves the repository root from this package's directory.
func repoRootForTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return filepath.Join(wd, "..", "..")
}

// TestCommittedStoreIsValid pins the real, committed waiver store. Because it
// is an ordinary Go test, `go test ./...` and `make preflight` enforce the
// store's shape everywhere — locally and in CI — without any extra wiring.
func TestCommittedStoreIsValid(t *testing.T) {
	root := repoRootForTest(t)
	raw, err := os.ReadFile(filepath.Join(root, storePath))
	if err != nil {
		t.Fatalf("reading %s: %v", storePath, err)
	}
	if bytes.IndexByte(raw, 0) >= 0 {
		t.Fatalf("%s contains NUL bytes; the waiver store must stay text so each waiver is blameable", storePath)
	}
	store, err := LoadStore(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("%s: %v", storePath, err)
	}
	if err := CheckSorted(store.All); err != nil {
		t.Fatalf("%s: %v", storePath, err)
	}
	if len(store.All) == 0 {
		t.Fatalf("%s parsed to zero waivers; a store that silently reads as empty would fail every waived script at once", storePath)
	}
	// Canonical form: rewriting the parsed store must reproduce the file byte
	// for byte, so hand edits cannot drift into a shape that produces noisy
	// diffs on the next machine write.
	var buf bytes.Buffer
	if err := WriteStore(&buf, store.All); err != nil {
		t.Fatalf("WriteStore: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), raw) {
		t.Errorf("%s is not in canonical form; regenerate it so entries stay sorted and compact", storePath)
	}
}

// TestNoBinaryWaiverStore is the guard against reintroducing the SQLite
// database this store replaced. A binary store cannot be reviewed in a diff
// and gives no per-waiver `git blame`.
func TestNoBinaryWaiverStore(t *testing.T) {
	root := repoRootForTest(t)
	entries, err := os.ReadDir(filepath.Join(root, storeDir))
	if err != nil {
		t.Fatalf("reading %s: %v", storeDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, bad := range binaryStoreExtensions {
			if ext == bad {
				t.Errorf("binary waiver store %s/%s must not be committed; add waivers to %s instead",
					storeDir, e.Name(), storePath)
			}
		}
	}
}
