package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateBaselineFilePreservesSymlinkAndUpdatesTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "canonical-baseline.json")
	link := filepath.Join(dir, "baseline.json")
	initial := baseline{Version: baselineSchemaV1, Findings: keySet(map[string][]string{
		"dead-package": {"pkg/removed"},
	})}
	if err := os.WriteFile(target, mustMarshalBaseline(t, initial), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), link); err != nil {
		t.Skipf("create baseline symlink: %v", err)
	}

	plan, err := updateBaselineFile(link, findingSet(nil), updateRequest{})
	if err != nil {
		t.Fatalf("update through symlink: %v", err)
	}
	if !plan.Write {
		t.Fatal("symlinked baseline shrink did not write")
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("baseline symlink was replaced by a regular file")
	}
	updated, err := readBaseline(target)
	if err != nil {
		t.Fatalf("read symlink target: %v", err)
	}
	if keyMapCount(updated.Findings) != 0 {
		t.Fatalf("symlink target findings = %v, want empty", updated.Findings)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if targetInfo.Mode().Perm() != 0o640 {
		t.Fatalf("symlink target mode = %o, want 640", targetInfo.Mode().Perm())
	}
}

func TestUpdateBaselineFileRejectsBrokenSymlinkWithoutReplacingIt(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "baseline.json")
	if err := os.Symlink("missing-target.json", link); err != nil {
		t.Skipf("create broken baseline symlink: %v", err)
	}

	_, err := updateBaselineFile(link, findingSet(nil), updateRequest{})
	if err == nil || !strings.Contains(err.Error(), "resolve baseline symlink") {
		t.Fatalf("broken symlink update error = %v, want resolution failure", err)
	}
	info, lstatErr := os.Lstat(link)
	if lstatErr != nil {
		t.Fatal(lstatErr)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("broken baseline symlink was replaced")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "missing-target.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("broken symlink update created target: %v", statErr)
	}
}
