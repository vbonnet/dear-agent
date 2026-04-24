package hash

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCalculateFileHash tests the happy path with known hash values
func TestCalculateFileHash(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		expectedHash string
	}{
		{
			name:         "empty file",
			content:      "",
			expectedHash: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:         "hello newline",
			content:      "hello\n",
			expectedHash: "sha256:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
		},
		{
			name:         "single character",
			content:      "a",
			expectedHash: "sha256:ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "test.txt")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Calculate hash
			hash, err := CalculateFileHash(tmpFile)
			if err != nil {
				t.Fatalf("CalculateFileHash() error = %v", err)
			}

			// Verify hash
			if hash != tt.expectedHash {
				t.Errorf("CalculateFileHash() = %v, want %v", hash, tt.expectedHash)
			}
		})
	}
}

// TestCalculateFileHash_TildeExpansion tests that tilde expansion works
func TestCalculateFileHash_TildeExpansion(t *testing.T) {
	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	// Create test file in home directory
	tmpFile := filepath.Join(home, "test-hash.txt")
	content := "test content\n"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	defer os.Remove(tmpFile)

	// Test with tilde path
	hash1, err := CalculateFileHash("~/test-hash.txt")
	if err != nil {
		t.Fatalf("CalculateFileHash() with tilde error = %v", err)
	}

	// Test with absolute path
	hash2, err := CalculateFileHash(tmpFile)
	if err != nil {
		t.Fatalf("CalculateFileHash() with absolute path error = %v", err)
	}

	// Both should produce same hash
	if hash1 != hash2 {
		t.Errorf("Tilde expansion hash mismatch: got %v, want %v", hash1, hash2)
	}
}

// TestCalculateFileHash_Errors tests error cases
func TestCalculateFileHash_Errors(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "file not found",
			path:    "/nonexistent/file.txt",
			wantErr: true,
		},
		{
			name:    "directory instead of file",
			path:    os.TempDir(),
			wantErr: true,
		},
		{
			name:    "unsupported tilde format",
			path:    "~otheruser/file.txt",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := CalculateFileHash(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateFileHash() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestExpandPath tests path expansion functionality
func TestExpandPath(t *testing.T) {
	// Get home directory for comparisons
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name:    "tilde only",
			path:    "~",
			want:    home,
			wantErr: false,
		},
		{
			name:    "tilde with path",
			path:    "~/Documents",
			want:    filepath.Join(home, "Documents"),
			wantErr: false,
		},
		{
			name:    "tilde with nested path",
			path:    "~/Documents/test.txt",
			want:    filepath.Join(home, "Documents/test.txt"),
			wantErr: false,
		},
		{
			name:    "unsupported user tilde",
			path:    "~otheruser/file",
			want:    "",
			wantErr: true,
		},
		{
			name:    "absolute path",
			path:    "/tmp/test.txt",
			want:    "/tmp/test.txt",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ExpandPath() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestExpandPath_RelativePath tests that relative paths are converted to absolute
func TestExpandPath_RelativePath(t *testing.T) {
	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}

	got, err := ExpandPath("test.txt")
	if err != nil {
		t.Fatalf("ExpandPath() error = %v", err)
	}

	want := filepath.Join(cwd, "test.txt")
	if got != want {
		t.Errorf("ExpandPath() = %v, want %v", got, want)
	}
}
