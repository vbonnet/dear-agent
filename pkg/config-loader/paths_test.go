package configloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePath(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	tests := []struct {
		name    string
		path    string
		baseDir string
		want    string
		wantErr bool
	}{
		{
			name:    "absolute path",
			path:    "/etc/config.yaml",
			baseDir: "/home/user",
			want:    "/etc/config.yaml",
		},
		{
			name:    "tilde path",
			path:    "~/.config/app.yaml",
			baseDir: "/tmp/workspace",
			want:    filepath.Join(homeDir, ".config/app.yaml"),
		},
		{
			name:    "relative path with baseDir",
			path:    "config/app.yaml",
			baseDir: "/tmp/workspace",
			want:    "/tmp/workspace/config/app.yaml",
		},
		{
			name:    "relative path with empty baseDir",
			path:    "config.yaml",
			baseDir: "",
			want:    filepath.Join(cwd, "config.yaml"),
		},
		{
			name:    "empty path",
			path:    "",
			baseDir: "/home/user",
			wantErr: true,
		},
		{
			name:    "baseDir with tilde",
			path:    "config.yaml",
			baseDir: "~/workspace",
			want:    filepath.Join(homeDir, "workspace/config.yaml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolvePath(tt.path, tt.baseDir)

			if tt.wantErr {
				if err == nil {
					t.Error("ResolvePath() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Fatalf("ResolvePath() unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("ResolvePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvePathWithDefaults(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		baseDir string
	}{
		{
			name:    "valid path",
			path:    "config.yaml",
			baseDir: "/home/user",
		},
		{
			name:    "empty path returns original",
			path:    "",
			baseDir: "/home/user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePathWithDefaults(tt.path, tt.baseDir)
			// Should never panic or return empty string
			if got == "" && tt.path != "" {
				t.Error("ResolvePathWithDefaults() returned empty string for non-empty input")
			}
		})
	}
}

func TestResolvePathWithDefaults_ErrorCases(t *testing.T) {
	// Test that ResolvePathWithDefaults returns original path on error
	result := ResolvePathWithDefaults("", "/some/path")
	if result != "" {
		t.Errorf("ResolvePathWithDefaults() = %q, want empty string", result)
	}
}

func TestFindFile(t *testing.T) {
	// Create temp directories and files
	tmpDir := t.TempDir()

	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	os.MkdirAll(dir1, 0755)
	os.MkdirAll(dir2, 0755)

	// Create test file in dir2
	testFile := filepath.Join(dir2, "config.yaml")
	os.WriteFile(testFile, []byte("test"), 0644)

	tests := []struct {
		name        string
		filename    string
		searchPaths []string
		want        string
		wantErr     bool
	}{
		{
			name:     "file found in second directory",
			filename: "config.yaml",
			searchPaths: []string{
				dir1,
				dir2,
			},
			want: testFile,
		},
		{
			name:     "file not found",
			filename: "missing.yaml",
			searchPaths: []string{
				dir1,
				dir2,
			},
			wantErr: true,
		},
		{
			name:        "empty filename",
			filename:    "",
			searchPaths: []string{dir1},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindFile(tt.filename, tt.searchPaths)

			if tt.wantErr {
				if err == nil {
					t.Error("FindFile() error = nil, want error")
				}
				return
			}

			if err != nil {
				t.Fatalf("FindFile() unexpected error: %v", err)
			}

			if got != tt.want {
				t.Errorf("FindFile() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindFile_ExactPath(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "exact.yaml")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// FindFile should find exact path first
	found, err := FindFile(testFile, []string{"/nonexistent"})
	if err != nil {
		t.Fatalf("FindFile() unexpected error: %v", err)
	}
	if found != testFile {
		t.Errorf("FindFile() = %q, want %q", found, testFile)
	}
}

func TestFindFile_WithTildePath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	// Create temp file in home directory
	testFile := filepath.Join(homeDir, ".test-config-loader-findfile.yaml")
	defer os.Remove(testFile)

	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test with tilde path in search paths
	tmpDir := t.TempDir()
	found, err := FindFile(".test-config-loader-findfile.yaml", []string{tmpDir, "~"})
	if err != nil {
		t.Fatalf("FindFile() unexpected error: %v", err)
	}
	if found != testFile {
		t.Errorf("FindFile() = %q, want %q", found, testFile)
	}
}
