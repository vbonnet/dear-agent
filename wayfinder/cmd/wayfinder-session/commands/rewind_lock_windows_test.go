//go:build windows

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsExtendedPathPointerPreservesLongDiskPaths(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantPrefix string
	}{
		{
			name:       "drive path",
			path:       `C:\project\` + strings.Repeat(`nested\`, 40) + `rewind.lock`,
			wantPrefix: `\\?\C:\project\`,
		},
		{
			name:       "UNC path",
			path:       `\\server\share\project\` + strings.Repeat(`nested\`, 40) + `rewind.lock`,
			wantPrefix: `\\?\UNC\server\share\project\`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pointer, err := windowsExtendedPathPointer(test.path)
			if err != nil {
				t.Fatalf("normalize extended-length path: %v", err)
			}
			got := windows.UTF16PtrToString(pointer)
			if !strings.HasPrefix(got, test.wantPrefix) {
				t.Fatalf("extended path = %q, want prefix %q", got, test.wantPrefix)
			}
			if len(got) < 260 {
				t.Fatalf("fixture path length = %d, want a Windows long path", len(got))
			}
		})
	}
}

func TestRewindTransitionLockSupportsLongWindowsProjectPath(t *testing.T) {
	projectDir := t.TempDir()
	for len(filepath.Join(projectDir, ".wayfinder", "locks", rewindLockFilename)) < 300 {
		projectDir = filepath.Join(projectDir, "nested-project-segment")
		if err := os.Mkdir(projectDir, 0o700); err != nil {
			t.Fatalf("create long project path: %v", err)
		}
	}

	lock, err := acquireRewindTransitionLock(projectDir)
	if err != nil {
		t.Fatalf("acquire rewind lock at long project path: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatalf("release rewind lock at long project path: %v", err)
	}
}
