package security

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePath(t *testing.T) {
	// Create temp directory for testing
	tmpDir := t.TempDir()

	t.Run("allows path within base", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "test.txt")
		if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}

		validated, err := ValidatePath(testFile, tmpDir, false)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if !strings.Contains(validated, "test.txt") {
			t.Errorf("expected validated path to contain test.txt, got %s", validated)
		}
	})

	t.Run("blocks path traversal with ..", func(t *testing.T) {
		maliciousPath := filepath.Join(tmpDir, "..", "..", "etc", "passwd")

		_, err := ValidatePath(maliciousPath, tmpDir, false)
		if err == nil {
			t.Error("expected path traversal error")
		}

		var pathErr *PathTraversalError
		if !errors.As(err, &pathErr) {
			t.Errorf("expected PathTraversalError, got %T", err)
		}
	})

	t.Run("blocks symlinks when followSymlinks is false", func(t *testing.T) {
		targetFile := filepath.Join(tmpDir, "target.txt")
		linkFile := filepath.Join(tmpDir, "link.txt")

		if err := os.WriteFile(targetFile, []byte("target"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(targetFile, linkFile); err != nil {
			t.Skip("symlinks not supported on this system")
		}

		_, err := ValidatePath(linkFile, tmpDir, false)
		if err == nil {
			t.Error("expected symlink error")
		}

		var symlinkErr *SymlinkError
		if !errors.As(err, &symlinkErr) {
			t.Errorf("expected SymlinkError, got %T", err)
		}
	})

	t.Run("allows symlinks when followSymlinks is true", func(t *testing.T) {
		targetFile := filepath.Join(tmpDir, "target2.txt")
		linkFile := filepath.Join(tmpDir, "link2.txt")

		if err := os.WriteFile(targetFile, []byte("target"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(targetFile, linkFile); err != nil {
			t.Skip("symlinks not supported on this system")
		}

		_, err := ValidatePath(linkFile, tmpDir, true)
		if err != nil {
			t.Errorf("expected no error when following symlinks, got %v", err)
		}
	})

	t.Run("returns error when base directory does not exist", func(t *testing.T) {
		nonExistentBase := filepath.Join(tmpDir, "nonexistent")

		_, err := ValidatePath("/tmp/test.txt", nonExistentBase, false)
		if err == nil {
			t.Error("expected error for nonexistent base")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Errorf("expected 'does not exist' error, got %v", err)
		}
	})

	t.Run("allows non-existent paths within base", func(t *testing.T) {
		nonExistentPath := filepath.Join(tmpDir, "future", "file.txt")

		validated, err := ValidatePath(nonExistentPath, tmpDir, false)
		if err != nil {
			t.Errorf("expected no error for non-existent path within base, got %v", err)
		}
		if !strings.Contains(validated, "future") {
			t.Errorf("expected validated path to contain future, got %s", validated)
		}
	})
}

func TestValidateDiagramPath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("allows valid diagram extensions", func(t *testing.T) {
		validExtensions := []string{".d2", ".mmd", ".mermaid", ".puml", ".plantuml"}

		for _, ext := range validExtensions {
			testFile := filepath.Join(tmpDir, "diagram"+ext)
			if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := ValidateDiagramPath(testFile, tmpDir)
			if err != nil {
				t.Errorf("expected no error for %s, got %v", ext, err)
			}
		}
	})

	t.Run("blocks invalid diagram extensions", func(t *testing.T) {
		invalidFile := filepath.Join(tmpDir, "malicious.exe")
		if err := os.WriteFile(invalidFile, []byte("malware"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := ValidateDiagramPath(invalidFile, tmpDir)
		if err == nil {
			t.Error("expected invalid extension error")
		}

		var extErr *InvalidExtensionError
		if !errors.As(err, &extErr) {
			t.Errorf("expected InvalidExtensionError, got %T", err)
		}
	})

	t.Run("blocks path traversal in diagram paths", func(t *testing.T) {
		maliciousPath := filepath.Join(tmpDir, "..", "..", "etc", "passwd.d2")

		_, err := ValidateDiagramPath(maliciousPath, tmpDir)
		if err == nil {
			t.Error("expected path traversal error")
		}

		var pathErr *PathTraversalError
		if !errors.As(err, &pathErr) {
			t.Errorf("expected PathTraversalError, got %T", err)
		}
	})
}

func TestValidateOutputPath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("allows valid output extensions", func(t *testing.T) {
		validExtensions := []string{".svg", ".png", ".pdf", ".json", ".txt", ".md"}

		for _, ext := range validExtensions {
			testFile := filepath.Join(tmpDir, "output"+ext)
			if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
				t.Fatal(err)
			}

			_, err := ValidateOutputPath(testFile, tmpDir)
			if err != nil {
				t.Errorf("expected no error for %s, got %v", ext, err)
			}
		}
	})

	t.Run("allows files without extension", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "outputfile")
		if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := ValidateOutputPath(testFile, tmpDir)
		if err != nil {
			t.Errorf("expected no error for extensionless file, got %v", err)
		}
	})

	t.Run("blocks invalid output extensions", func(t *testing.T) {
		invalidFile := filepath.Join(tmpDir, "malicious.sh")
		if err := os.WriteFile(invalidFile, []byte("#!/bin/sh"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := ValidateOutputPath(invalidFile, tmpDir)
		if err == nil {
			t.Error("expected invalid extension error")
		}

		var extErr *InvalidExtensionError
		if !errors.As(err, &extErr) {
			t.Errorf("expected InvalidExtensionError, got %T", err)
		}
	})
}

func TestSafeReadFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("reads valid file", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "test.txt")
		expectedContent := "test content"
		if err := os.WriteFile(testFile, []byte(expectedContent), 0644); err != nil {
			t.Fatal(err)
		}

		content, err := SafeReadFile(testFile, 0)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if content != expectedContent {
			t.Errorf("expected %q, got %q", expectedContent, content)
		}
	})

	t.Run("blocks oversized files", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "large.txt")
		largeContent := strings.Repeat("x", 1000)
		if err := os.WriteFile(testFile, []byte(largeContent), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := SafeReadFile(testFile, 100) // Max 100 bytes
		if err == nil {
			t.Error("expected file size error")
		}

		var sizeErr *FileSizeError
		if !errors.As(err, &sizeErr) {
			t.Errorf("expected FileSizeError, got %T", err)
		}
		if sizeErr.Size != 1000 {
			t.Errorf("expected size 1000, got %d", sizeErr.Size)
		}
		if sizeErr.MaxSize != 100 {
			t.Errorf("expected max size 100, got %d", sizeErr.MaxSize)
		}
	})

	t.Run("returns error for nonexistent file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "nonexistent.txt")

		_, err := SafeReadFile(nonExistentFile, 0)
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
		if !os.IsNotExist(err) {
			t.Errorf("expected not exist error, got %v", err)
		}
	})

	t.Run("uses default max size when maxSize is 0", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "medium.txt")
		content := strings.Repeat("x", 100)
		if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := SafeReadFile(testFile, 0) // Should use MaxDiagramSize
		if err != nil {
			t.Errorf("expected no error with default max size, got %v", err)
		}
	})
}

func TestSafeCreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("creates valid directory", func(t *testing.T) {
		newDir := filepath.Join(tmpDir, "newdir")

		created, err := SafeCreateDirectory(newDir, tmpDir, 0755)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		info, err := os.Stat(created)
		if err != nil {
			t.Errorf("directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected created path to be a directory")
		}
	})

	t.Run("creates nested directories", func(t *testing.T) {
		nestedDir := filepath.Join(tmpDir, "output", "diagrams", "nested")

		created, err := SafeCreateDirectory(nestedDir, tmpDir, 0755)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}

		info, err := os.Stat(created)
		if err != nil {
			t.Errorf("nested directory not created: %v", err)
		}
		if !info.IsDir() {
			t.Error("expected created path to be a directory")
		}
	})

	t.Run("blocks path traversal", func(t *testing.T) {
		maliciousDir := filepath.Join(tmpDir, "..", "..", "etc", "malicious")

		_, err := SafeCreateDirectory(maliciousDir, tmpDir, 0755)
		if err == nil {
			t.Error("expected path traversal error")
		}

		var pathErr *PathTraversalError
		if !errors.As(err, &pathErr) {
			t.Errorf("expected PathTraversalError, got %T", err)
		}
	})

	t.Run("blocks symlinks", func(t *testing.T) {
		targetDir := filepath.Join(tmpDir, "target")
		linkDir := filepath.Join(tmpDir, "link")

		if err := os.Mkdir(targetDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(targetDir, linkDir); err != nil {
			t.Skip("symlinks not supported on this system")
		}

		_, err := SafeCreateDirectory(linkDir, tmpDir, 0755)
		if err == nil {
			t.Error("expected symlink error")
		}

		var symlinkErr *SymlinkError
		if !errors.As(err, &symlinkErr) {
			t.Errorf("expected SymlinkError, got %T", err)
		}
	})

	t.Run("succeeds for existing directory", func(t *testing.T) {
		existingDir := filepath.Join(tmpDir, "existing")
		if err := os.Mkdir(existingDir, 0755); err != nil {
			t.Fatal(err)
		}

		_, err := SafeCreateDirectory(existingDir, tmpDir, 0755)
		if err != nil {
			t.Errorf("expected no error for existing directory, got %v", err)
		}
	})
}

func TestIsSafePath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("returns true for safe path", func(t *testing.T) {
		safePath := filepath.Join(tmpDir, "safe.txt")
		if err := os.WriteFile(safePath, []byte("safe"), 0644); err != nil {
			t.Fatal(err)
		}

		if !IsSafePath(safePath, tmpDir) {
			t.Error("expected true for safe path")
		}
	})

	t.Run("returns false for unsafe path", func(t *testing.T) {
		unsafePath := filepath.Join(tmpDir, "..", "..", "etc", "passwd")

		if IsSafePath(unsafePath, tmpDir) {
			t.Error("expected false for unsafe path")
		}
	})

	t.Run("returns false for symlinks", func(t *testing.T) {
		targetFile := filepath.Join(tmpDir, "target3.txt")
		linkFile := filepath.Join(tmpDir, "link3.txt")

		if err := os.WriteFile(targetFile, []byte("target"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(targetFile, linkFile); err != nil {
			t.Skip("symlinks not supported on this system")
		}

		if IsSafePath(linkFile, tmpDir) {
			t.Error("expected false for symlink")
		}
	})
}

func TestGetSafeTempDir(t *testing.T) {
	t.Run("returns non-empty path", func(t *testing.T) {
		tempDir := GetSafeTempDir()
		if tempDir == "" {
			t.Error("expected non-empty temporary directory")
		}
	})

	t.Run("returns existing directory", func(t *testing.T) {
		tempDir := GetSafeTempDir()

		info, err := os.Stat(tempDir)
		if err != nil {
			t.Errorf("temp directory should exist: %v", err)
		}
		if !info.IsDir() {
			t.Error("temp path should be a directory")
		}
	})
}

func TestErrorTypes(t *testing.T) {
	t.Run("PathTraversalError unwraps to ErrPathTraversal", func(t *testing.T) {
		err := &PathTraversalError{Path: "/malicious", AllowedBase: "/safe"}

		if !errors.Is(err, ErrPathTraversal) {
			t.Error("PathTraversalError should unwrap to ErrPathTraversal")
		}
	})

	t.Run("SymlinkError unwraps to ErrSymlink", func(t *testing.T) {
		err := &SymlinkError{Path: "/link"}

		if !errors.Is(err, ErrSymlink) {
			t.Error("SymlinkError should unwrap to ErrSymlink")
		}
	})

	t.Run("FileSizeError unwraps to ErrFileSize", func(t *testing.T) {
		err := &FileSizeError{Size: 1000, MaxSize: 100}

		if !errors.Is(err, ErrFileSize) {
			t.Error("FileSizeError should unwrap to ErrFileSize")
		}
	})

	t.Run("InvalidExtensionError unwraps to ErrInvalidExtension", func(t *testing.T) {
		err := &InvalidExtensionError{Extension: ".exe", Allowed: AllowedDiagramExtensions}

		if !errors.Is(err, ErrInvalidExtension) {
			t.Error("InvalidExtensionError should unwrap to ErrInvalidExtension")
		}
	})
}
