package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	deverrors "github.com/vbonnet/dear-agent/tools/devlog/internal/errors"
)

func TestLoad_FileSizeLimit(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Create a config file larger than MaxConfigSize
	largeContent := make([]byte, MaxConfigSize+1)
	for i := range largeContent {
		largeContent[i] = 'a'
	}

	if err := os.WriteFile(configPath, largeContent, 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want error for file size limit")
	}

	if !errors.Is(err, deverrors.ErrConfigInvalid) {
		t.Errorf("error = %v, want ErrConfigInvalid", err)
	}
}

func TestLoad_PermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configYAML := `name: test
repos: []
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(configPath, 0600)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("Load() error = nil, want permission error")
	}

	// Should get a stat or read error
	var devErr *deverrors.DevlogError
	if !errors.As(err, &devErr) {
		t.Errorf("error type = %T, want *DevlogError", err)
	}
}

func TestWorktree_Validate_ComprehensivePathTraversal(t *testing.T) {
	tests := []struct {
		name    string
		wtName  string
		wantErr bool
	}{
		{"backslash Windows", "path\\to\\wt", true},
		{"null byte", "test\x00name", true},
		{"absolute path Unix", "/absolute/path", true},
		{"absolute path Windows", "C:\\path", true},
		{"clean valid", "my-feature", false},
		{"underscore valid", "feat_123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wt := &Worktree{
				Name:   tt.wtName,
				Branch: "main",
			}

			err := wt.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate() error = nil for name %q, want error", tt.wtName)
				}
			} else {
				if err != nil {
					t.Errorf("Validate() error = %v for name %q, want nil", err, tt.wtName)
				}
			}
		})
	}
}

func TestValidateGitURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// Valid HTTPS
		{"https valid", "https://github.com/user/repo.git", false},
		{"https no .git", "https://github.com/user/repo", false},

		// Valid SSH
		{"ssh valid", "git@github.com:user/repo.git", false},
		{"ssh no .git", "git@github.com:user/repo", false},

		// Invalid HTTPS
		{"https no host", "https://", true},
		{"https malformed", "https://[invalid", true},

		// Invalid SSH
		{"ssh malformed", "git@", true},
		{"ssh no colon", "git@github.com/user/repo", true},

		// Forbidden schemes
		{"file scheme", "file:///path/to/repo", true},
		{"http scheme", "http://github.com/user/repo.git", true},
		{"local path", "/local/path/to/repo", true},
		{"relative path", "../relative/path", true},

		// Edge cases
		{"empty", "", true},
		{"too long", string(make([]byte, MaxGitURLLength+1)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGitURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateGitURL(%q) error = nil, want error", tt.url)
				}
			} else {
				if err != nil {
					t.Errorf("validateGitURL(%q) error = %v, want nil", tt.url, err)
				}
			}
		})
	}
}
