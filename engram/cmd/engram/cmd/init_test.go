package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectEngramRepo(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() string // Returns temp dir path
		cleanup func(string)
		wantErr bool
	}{
		{
			name: "valid repo structure",
			setup: func() string {
				tmpDir := t.TempDir()

				// Create characteristic files
				os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Engram"), 0644)
				os.MkdirAll(filepath.Join(tmpDir, "core"), 0755)
				os.MkdirAll(filepath.Join(tmpDir, "engrams"), 0755)
				os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0755)

				return tmpDir
			},
			cleanup: func(path string) {
				os.RemoveAll(path)
			},
			wantErr: false,
		},
		{
			name: "missing core directory",
			setup: func() string {
				tmpDir := t.TempDir()

				os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Engram"), 0644)
				os.MkdirAll(filepath.Join(tmpDir, "engrams"), 0755)
				os.MkdirAll(filepath.Join(tmpDir, "plugins"), 0755)

				return tmpDir
			},
			cleanup: func(path string) {
				os.RemoveAll(path)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := tt.setup()
			defer tt.cleanup(tmpDir)

			valid := isEngramRepo(tmpDir)
			if valid && tt.wantErr {
				t.Errorf("isEngramRepo() returned true, expected false")
			}
			if !valid && !tt.wantErr {
				t.Errorf("isEngramRepo() returned false, expected true")
			}
		})
	}
}

func TestRunInit(t *testing.T) {
	// Save original HOME

	// Create temporary home directory
	tmpHome := t.TempDir()

	// Set HOME to temp directory
	t.Setenv("HOME", tmpHome)

	// Run init command
	err := runInit(nil, []string{})
	if err != nil {
		t.Fatalf("runInit() failed: %v", err)
	}

	// Verify workspace structure
	workspaceDir := filepath.Join(tmpHome, ".engram")
	userDir := filepath.Join(workspaceDir, "user")
	logsDir := filepath.Join(workspaceDir, "logs")
	cacheDir := filepath.Join(workspaceDir, "cache")
	userConfig := filepath.Join(userDir, "config.yaml")

	// Check directories
	dirs := map[string]string{
		"workspace": workspaceDir,
		"user":      userDir,
		"logs":      logsDir,
		"cache":     cacheDir,
	}

	for name, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("%s directory not created: %v", name, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s path exists but is not a directory", name)
		}
	}

	// Check user config
	if _, err := os.Stat(userConfig); err != nil {
		t.Errorf("user config not created: %v", err)
	}

	// Test idempotency - run init again
	err = runInit(nil, []string{})
	if err != nil {
		t.Errorf("runInit() should be idempotent, but failed on second run: %v", err)
	}

	// Verify directories still exist
	for name, dir := range dirs {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s directory missing after second init: %v", name, err)
		}
	}
}

func TestRunInit_NoHome(t *testing.T) {
	// Save original HOME

	// Unset HOME
	t.Setenv("HOME", "") // restored on test cleanup
	os.Unsetenv("HOME")

	// Run init command - should fail
	err := runInit(nil, []string{})
	if err == nil {
		t.Error("runInit() should fail when HOME is not set")
	}
}
