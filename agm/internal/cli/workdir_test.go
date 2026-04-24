package cli

import (
	"sync"
	"testing"
)

func TestGetProjectDirectory_Default(t *testing.T) {
	// Reset state
	projectDirMutex.Lock()
	old := projectDirectory
	projectDirectory = ""
	projectDirMutex.Unlock()
	defer func() {
		projectDirMutex.Lock()
		projectDirectory = old
		projectDirMutex.Unlock()
	}()

	got := GetProjectDirectory()
	if got != "." {
		t.Errorf("GetProjectDirectory() with empty = %q, want %q", got, ".")
	}
}

func TestSetAndGetProjectDirectory(t *testing.T) {
	// Reset state
	projectDirMutex.Lock()
	old := projectDirectory
	projectDirMutex.Unlock()
	defer func() {
		projectDirMutex.Lock()
		projectDirectory = old
		projectDirMutex.Unlock()
	}()

	tests := []struct {
		name string
		dir  string
		want string
	}{
		{
			name: "absolute path",
			dir:  "~/projects/my-app",
			want: "~/projects/my-app",
		},
		{
			name: "relative path",
			dir:  "relative/path",
			want: "relative/path",
		},
		{
			name: "dot path",
			dir:  ".",
			want: ".",
		},
		{
			name: "empty resets to default",
			dir:  "",
			want: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetProjectDirectory(tt.dir)
			got := GetProjectDirectory()
			if got != tt.want {
				t.Errorf("GetProjectDirectory() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProjectDirectory_Concurrency(t *testing.T) {
	// Reset state
	projectDirMutex.Lock()
	old := projectDirectory
	projectDirMutex.Unlock()
	defer func() {
		projectDirMutex.Lock()
		projectDirectory = old
		projectDirMutex.Unlock()
	}()

	var wg sync.WaitGroup
	dirs := []string{"/a", "/b", "/c", "/d", "/e"}

	// Concurrent writers
	for _, d := range dirs {
		wg.Add(1)
		go func(dir string) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				SetProjectDirectory(dir)
			}
		}(d)
	}

	// Concurrent readers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				got := GetProjectDirectory()
				if got == "" {
					t.Error("GetProjectDirectory() returned empty string (should never happen)")
				}
			}
		}()
	}

	wg.Wait()
}
