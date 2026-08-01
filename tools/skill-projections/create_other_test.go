//go:build !darwin && !linux

package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUnsupportedPlatformCheckAndWriteFailClosedWithoutMutation(t *testing.T) {
	repo := t.TempDir()
	sentinel := filepath.Join(repo, "sentinel")
	if err := os.WriteFile(sentinel, []byte("maintainer-owned\n"), 0o640); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	want := snapshotTree(t, repo)

	for _, args := range [][]string{
		{"-repo", repo},
		{"-repo", repo, "-write"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 1 {
			t.Fatalf("run(%v) = %d, stdout=%q, stderr=%q", args, code, stdout.String(), stderr.String())
		}
		if !strings.Contains(stderr.String(), "retained-root operations are unavailable") {
			t.Fatalf("run(%v) did not report the fail-closed platform seam: %q", args, stderr.String())
		}
		if got := snapshotTree(t, repo); !reflect.DeepEqual(got, want) {
			t.Fatalf("run(%v) mutated tree\ngot:  %#v\nwant: %#v", args, got, want)
		}
	}

	if _, err := openRepositoryRoot(repo); err == nil || !strings.Contains(err.Error(), "retained-root operations are unavailable") {
		t.Fatalf("openRepositoryRoot error = %v", err)
	}
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%s:%04o", info.Mode().Type(), info.Mode().Perm())
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += ":" + string(data)
		}
		snapshot[filepath.ToSlash(relative)] = value
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot tree: %v", err)
	}
	return snapshot
}
