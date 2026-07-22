//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNativeOverlayFSRequestPrioritizesMatchedLowerDir(t *testing.T) {
	firstLowerDir := t.TempDir()
	requestedLowerDir := t.TempDir()
	requestedWorkingDir := filepath.Join(requestedLowerDir, "nested")
	if err := os.MkdirAll(requestedWorkingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mergedDir := filepath.Join(t.TempDir(), "merged")

	workingDir, lowerDirs, err := mapNativeOverlayFSRequest(SandboxRequest{
		LowerDirs:  []string{firstLowerDir, requestedLowerDir},
		WorkingDir: requestedWorkingDir,
	}, mergedDir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(mergedDir, "nested"); workingDir != want {
		t.Fatalf("working directory = %q, want %q", workingDir, want)
	}
	if want := []string{requestedLowerDir, firstLowerDir}; !reflect.DeepEqual(lowerDirs, want) {
		t.Fatalf("ordered lower directories = %v, want %v", lowerDirs, want)
	}
}
