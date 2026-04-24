package resume

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateProjectName_Valid(t *testing.T) {
	validNames := []string{
		"myproject",
		"my-project",
		"my_project",
		"project123",
		"MY-PROJECT",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			err := validateProjectName(name)
			if err != nil {
				t.Errorf("validateProjectName(%q) returned error: %v, want nil", name, err)
			}
		})
	}
}

func TestValidateProjectName_PathTraversal(t *testing.T) {
	invalidNames := []string{
		"../etc/passwd",
		"..\\windows\\system32",
		"project/../../../etc",
		"..",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			err := validateProjectName(name)
			if err == nil {
				t.Errorf("validateProjectName(%q) returned nil, want error for path traversal", name)
			}
		})
	}
}

func TestValidateProjectName_PathSeparators(t *testing.T) {
	invalidNames := []string{
		"/absolute/path",
		"relative/path",
		"windows\\path",
		"mixed/path\\styles",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			err := validateProjectName(name)
			if err == nil {
				t.Errorf("validateProjectName(%q) returned nil, want error for path separators", name)
			}
		})
	}
}

func TestValidateProjectName_Empty(t *testing.T) {
	emptyNames := []string{
		"",
		"   ",
		"\t",
		"\n",
	}

	for _, name := range emptyNames {
		t.Run("empty", func(t *testing.T) {
			err := validateProjectName(name)
			if err == nil {
				t.Errorf("validateProjectName(%q) returned nil, want error for empty name", name)
			}
		})
	}
}

func TestAbort(t *testing.T) {
	err := abort()

	if !errors.Is(err, ErrUserAborted) {
		t.Errorf("abort() returned %v, want ErrUserAborted", err)
	}
}

func TestResume_W0Only(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create W0 file
	w0Path := filepath.Join(tmpDir, "W0-charter.md")
	if err := os.WriteFile(w0Path, []byte("# W0"), 0644); err != nil {
		t.Fatalf("Failed to create W0 file: %v", err)
	}

	// Test resume
	err := resume(tmpDir, StateW0Only)
	if err != nil {
		t.Fatalf("resume() failed: %v", err)
	}

	// Verify STATUS file was created
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if _, err := os.Stat(statusPath); os.IsNotExist(err) {
		t.Error("Expected STATUS file to be created")
	}
}

func TestResume_Empty(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Test resume on empty directory
	err := resume(tmpDir, StateEmpty)
	if err != nil {
		t.Fatalf("resume() failed: %v", err)
	}

	// Verify STATUS file was created
	statusPath := filepath.Join(tmpDir, "WAYFINDER-STATUS.md")
	if _, err := os.Stat(statusPath); os.IsNotExist(err) {
		t.Error("Expected STATUS file to be created")
	}
}

func TestIsSafeToWrite_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent.md")

	err := isSafeToWrite(path)
	if err != nil {
		t.Errorf("isSafeToWrite(nonexistent) = %v, want nil", err)
	}
}

func TestIsSafeToWrite_RegularFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "regular.md")

	// Create regular file
	if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	err := isSafeToWrite(path)
	if err != nil {
		t.Errorf("isSafeToWrite(regular file) = %v, want nil", err)
	}
}

func TestIsSafeToWrite_Symlink(t *testing.T) {
	if os.Getenv("SKIP_SYMLINK_TESTS") != "" {
		t.Skip("Symlink tests skipped (may require permissions)")
	}

	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "target.md")
	symlinkPath := filepath.Join(tmpDir, "symlink.md")

	// Create target file
	if err := os.WriteFile(targetPath, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}

	// Create symlink
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	err := isSafeToWrite(symlinkPath)
	if err == nil {
		t.Error("isSafeToWrite(symlink) returned nil, want error")
	}
}
